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

// store is a tiny in-memory record of notifications, kept only so /notifications
// has something to show. In a real system this would publish to SNS/SQS/email --
// out of scope for this project's stated learning outcomes.
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
	port := getenv("PORT", "8080")
	st := &store{}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	mux.HandleFunc("POST /notify", func(w http.ResponseWriter, r *http.Request) {
		var req notifyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Username == "" || req.Message == "" {
			http.Error(w, "username and message are required", http.StatusBadRequest)
			return
		}
		n := st.add(req.Username, req.Message)
		log.Printf("recorded notification %s for user=%s", n.ID, n.Username)
		writeJSON(w, http.StatusCreated, n)
	})

	mux.HandleFunc("GET /notifications", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, st.all())
	})

	srv := &http.Server{Addr: ":" + port, Handler: mux}
	runWithGracefulShutdown(srv, "notifications")
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
