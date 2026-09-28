package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
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

// sessionTTL is the lifetime of a session record in Redis.
const sessionTTL = 30 * time.Minute

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token            string `json:"token"`
	ExpiresInSeconds int    `json:"expires_in_seconds"`
}

func main() {
	ctx := context.Background()

	shutdownTracing, err := setupTracing(ctx, "auth")
	if err != nil {
		log.Printf("tracing setup failed, continuing without it: %v", err)
		shutdownTracing = func(context.Context) error { return nil }
	}
	defer shutdownTracing(ctx)

	port := getenv("PORT", "8080")
	redisAddr := getenv("REDIS_ADDR", "localhost:6379")

	// API_KEY is injected by the container runtime from Secrets Manager.
	// Only its length is logged; the value itself is never written out.
	if apiKey := os.Getenv("API_KEY"); apiKey != "" {
		log.Printf("loaded API_KEY secret (%d chars)", len(apiKey))
	}

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	defer rdb.Close()

	mux := http.NewServeMux()

	healthHandler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}

	loginHandler := func(w http.ResponseWriter, r *http.Request) {
		var req loginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		req.Username = strings.TrimSpace(req.Username)
		if req.Username == "" || req.Password == "" {
			http.Error(w, "username and password are required", http.StatusBadRequest)
			return
		}

		// Credentials are not validated against a user store; any
		// non-empty username/password issues a session.
		token, err := generateToken()
		if err != nil {
			log.Printf("token generation failed: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if err := rdb.Set(ctx, sessionKey(token), req.Username, sessionTTL).Err(); err != nil {
			log.Printf("redis set failed: %v", err)
			http.Error(w, "session store unavailable", http.StatusServiceUnavailable)
			return
		}

		log.Printf("issued session for user=%s", req.Username)
		writeJSON(w, http.StatusOK, loginResponse{
			Token:            token,
			ExpiresInSeconds: int(sessionTTL.Seconds()),
		})
	}

	// Each route is registered under both its bare path and an
	// /api/auth/-prefixed path: the ALB forwards the full request path
	// unchanged, while local and internal callers use the bare path.
	mux.HandleFunc("GET /health", healthHandler)
	mux.HandleFunc("GET /api/auth/health", healthHandler)
	mux.HandleFunc("POST /login", loginHandler)
	mux.HandleFunc("POST /api/auth/login", loginHandler)

	srv := &http.Server{Addr: ":" + port, Handler: corsMiddleware(otelhttp.NewHandler(mux, "auth"))}
	runWithGracefulShutdown(srv, "auth")
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

func generateToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func sessionKey(token string) string {
	return fmt.Sprintf("session:%s", token)
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
