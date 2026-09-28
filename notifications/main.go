package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
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
)

type Notification struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	Message   string `json:"message"`
	CreatedAt string `json:"created_at"`
}

type notifyRequest struct {
	Username string `json:"username"`
	Message  string `json:"message"`
}

// store is an in-memory record of received notifications.
type store struct {
	mu    sync.Mutex
	items []Notification
	seq   int
}

func (s *store) add(username, message string) Notification {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	n := Notification{
		ID:        fmt.Sprintf("notif-%d", s.seq),
		Username:  username,
		Message:   message,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	s.items = append(s.items, n)
	return n
}

func (s *store) all() []Notification {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Notification, len(s.items))
	copy(out, s.items)
	return out
}

func main() {
	ctx := context.Background()

	shutdownTracing, err := setupTracing(ctx, "notifications")
	if err != nil {
		log.Printf("tracing setup failed, continuing without it: %v", err)
		shutdownTracing = func(context.Context) error { return nil }
	}
	defer shutdownTracing(ctx)

	port := getenv("PORT", "8080")
	st := &store{}

	mux := http.NewServeMux()

	healthHandler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}

	notifyHandler := func(w http.ResponseWriter, r *http.Request) {
		var req notifyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Username == "" || req.Message == "" {
			http.Error(w, "username and message are required", http.StatusBadRequest)
			return
		}
		n := st.add(req.Username, req.Message)
		log.Printf("recorded notification %s for user=%s", n.ID, n.Username)
		writeJSON(w, http.StatusCreated, n)
	}

	listHandler := func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, st.all())
	}

	// Bare paths serve internal Service Connect traffic from Orders;
	// /api/notifications/-prefixed paths serve requests forwarded by the
	// ALB, which does not strip the path prefix.
	mux.HandleFunc("GET /health", healthHandler)
	mux.HandleFunc("GET /api/notifications/health", healthHandler)
	mux.HandleFunc("POST /notify", notifyHandler)
	mux.HandleFunc("POST /api/notifications/notify", notifyHandler)
	mux.HandleFunc("GET /notifications", listHandler)
	mux.HandleFunc("GET /api/notifications/notifications", listHandler)

	srv := &http.Server{Addr: ":" + port, Handler: corsMiddleware(otelhttp.NewHandler(mux, "notifications"))}
	runWithGracefulShutdown(srv, "notifications")
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
