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

// orderStore is a tiny in-memory store. The point of this project is the
// infrastructure around the service, not a real persistence layer -- swap
// this out for DynamoDB/RDS later if you want, but it isn't required to hit
// any of the ECS/networking/CI-CD learning outcomes.
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
	port := getenv("PORT", "8080")
	redisAddr := getenv("REDIS_ADDR", "localhost:6379")
	// In docker-compose this is the compose service name (http://notifications:8080).
	// On ECS this becomes the Cloud Map DNS name (e.g. http://notifications.internal:8080).
	notificationsURL := getenv("NOTIFICATIONS_URL", "http://localhost:8083")

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	defer rdb.Close()

	store := newOrderStore()
	httpClient := &http.Client{Timeout: 3 * time.Second}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	mux.HandleFunc("POST /orders", func(w http.ResponseWriter, r *http.Request) {
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
	})

	mux.HandleFunc("GET /orders", func(w http.ResponseWriter, r *http.Request) {
		username, ok := authenticate(r, rdb)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		writeJSON(w, http.StatusOK, store.list(username))
	})

	srv := &http.Server{Addr: ":" + port, Handler: mux}
	runWithGracefulShutdown(srv, "orders")
}

// authenticate looks the bearer token up directly in the shared Redis cache.
// Orders never calls Auth over the network to check a session -- it just
// reads the cache Auth already wrote to. That's the whole point of putting
// ElastiCache in front of stateless containers.
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

// notify calls the Notifications service by its DNS name. It logs and moves
// on if that call fails -- a notification failure shouldn't fail the order.
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
