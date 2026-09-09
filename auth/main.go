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

	"github.com/redis/go-redis/v9"
)

// sessionTTL controls how long a login session lives in Redis before it
// expires on its own. Orders and Notifications never talk to Auth directly --
// they just read this same Redis instance, which is the "shared session
// cache across stateless containers" pattern this project is about.
const sessionTTL = 30 * time.Minute

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token           string `json:"token"`
	ExpiresInSeconds int    `json:"expires_in_seconds"`
}

func main() {
	port := getenv("PORT", "8080")
	redisAddr := getenv("REDIS_ADDR", "localhost:6379")

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	defer rdb.Close()

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	mux.HandleFunc("POST /login", func(w http.ResponseWriter, r *http.Request) {
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

		// NOTE: this is intentionally fake auth -- any non-empty username and
		// password is accepted, and nothing is checked against a real user
		// store. The learning goal of this project is ECS, service discovery,
		// and the shared cache, not building a real identity provider.
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
	})

	srv := &http.Server{Addr: ":" + port, Handler: mux}
	runWithGracefulShutdown(srv, "auth")
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
