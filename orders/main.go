package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/redis/go-redis/v9"
)

type Order struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	Item      string `json:"item"`
	Quantity  int    `json:"quantity"`
	CreatedAt string `json:"created_at"`
}

type createOrderRequest struct {
	Item     string `json:"item"`
	Quantity int    `json:"quantity"`
}

// orderStore is an in-memory, per-username order store.
type orderStore struct {
	mu   sync.Mutex
	data map[string][]Order // keyed by username
	seq  int
}

func newOrderStore() *orderStore {
	return &orderStore{data: make(map[string][]Order)}
}

func (s *orderStore) add(username, item string, qty int) Order {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	o := Order{
		ID:        fmt.Sprintf("order-%d", s.seq),
		Username:  username,
		Item:      item,
		Quantity:  qty,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	s.data[username] = append(s.data[username], o)
	return o
}

func (s *orderStore) list(username string) []Order {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Order, len(s.data[username]))
	copy(out, s.data[username])
	return out
}

func main() {
	ctx := context.Background()

	shutdownTracing, err := setupTracing(ctx, "orders")
	if err != nil {
		log.Printf("tracing setup failed, continuing without it: %v", err)
		shutdownTracing = func(context.Context) error { return nil }
	}
	defer shutdownTracing(ctx)

	port := getenv("PORT", "8080")
	redisAddr := getenv("REDIS_ADDR", "localhost:6379")
	// Resolves to the notifications container locally, or its Service
	// Connect DNS name (notifications.internal) when running on ECS.
	notificationsURL := getenv("NOTIFICATIONS_URL", "http://localhost:8083")

	if apiKey := os.Getenv("API_KEY"); apiKey != "" {
		log.Printf("loaded API_KEY secret (%d chars)", len(apiKey))
	}

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	defer rdb.Close()

	store := newOrderStore()
	// otelhttp propagates trace context on the outbound call to
	// notifications so it appears as a child span.
	httpClient := &http.Client{
		Timeout:   3 * time.Second,
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}

	mux := http.NewServeMux()

	healthHandler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}

	createOrderHandler := func(w http.ResponseWriter, r *http.Request) {
		username, ok := authenticate(r, rdb)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		var req createOrderRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Item == "" || req.Quantity <= 0 {
			http.Error(w, "item and a positive quantity are required", http.StatusBadRequest)
			return
		}

		order := store.add(username, req.Item, req.Quantity)
		log.Printf("created order %s for user=%s", order.ID, username)

		notify(httpClient, notificationsURL, username,
			fmt.Sprintf("Order %s placed: %dx %s", order.ID, order.Quantity, order.Item))

		writeJSON(w, http.StatusCreated, order)
	}

	listOrdersHandler := func(w http.ResponseWriter, r *http.Request) {
		username, ok := authenticate(r, rdb)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		writeJSON(w, http.StatusOK, store.list(username))
	}

	// Each route is registered under both its bare path and an
	// /api/orders/-prefixed path: the ALB forwards the full request path
	// unchanged, while local and internal callers use the bare path.
	mux.HandleFunc("GET /health", healthHandler)
	mux.HandleFunc("GET /api/orders/health", healthHandler)
	mux.HandleFunc("POST /orders", createOrderHandler)
	mux.HandleFunc("POST /api/orders/orders", createOrderHandler)
	mux.HandleFunc("GET /orders", listOrdersHandler)
	mux.HandleFunc("GET /api/orders/orders", listOrdersHandler)

	srv := &http.Server{Addr: ":" + port, Handler: corsMiddleware(otelhttp.NewHandler(mux, "orders"))}
	runWithGracefulShutdown(srv, "orders")
}

// corsMiddleware permits cross-origin requests from the frontend, which is
// served from a different origin than this API.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// setupTracing configures an OTLP/HTTP exporter targeting the ADOT
// Collector sidecar. Tracing is disabled without interrupting request
// handling if the collector is unreachable.
func setupTracing(ctx context.Context, serviceName string) (func(context.Context) error, error) {
	endpoint := getenv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4318")

	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(endpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("creating OTLP exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(attribute.String("service.name", serviceName)),
	)
	if err != nil {
		return nil, fmt.Errorf("building resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	return tp.Shutdown, nil
}

// authenticate resolves a bearer token directly against the shared Redis
// cache rather than calling the Auth service.
func authenticate(r *http.Request, rdb *redis.Client) (string, bool) {
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return "", false
	}
	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token == "" {
		return "", false
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	username, err := rdb.Get(ctx, "session:"+token).Result()
	if err != nil {
		return "", false
	}
	return username, true
}

// notify calls the Notifications service by its DNS name. Failures are
// logged and otherwise ignored; a notification failure must not fail the
// order.
func notify(client *http.Client, baseURL, username, message string) {
	payload, _ := json.Marshal(map[string]string{"username": username, "message": message})
	resp, err := client.Post(baseURL+"/notify", "application/json", bytes.NewReader(payload))
	if err != nil {
		log.Printf("notify call failed (continuing anyway): %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		log.Printf("notify call returned status %d", resp.StatusCode)
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func runWithGracefulShutdown(srv *http.Server, name string) {
	go func() {
		log.Printf("%s service listening on %s", name, srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("%s: listen error: %v", name, err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Printf("%s service shutting down...", name)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("%s: shutdown error: %v", name, err)
	}
}
