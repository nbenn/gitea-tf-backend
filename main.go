package main

import (
	"context"
	"crypto/subtle"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	// Load configuration
	cfg, err := LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize Gitea client
	giteaClient, err := NewGiteaClient(cfg)
	if err != nil {
		log.Fatalf("Failed to create Gitea client: %v", err)
	}

	// Create state handler
	stateHandler := NewStateHandler(giteaClient, cfg.MaxBodySize)

	// Create the main handler with optional auth middleware
	var stateHandlerWithAuth http.Handler = stateHandler
	if cfg.AuthToken != "" {
		stateHandlerWithAuth = authMiddleware(cfg.AuthToken, stateHandler)
		log.Printf("Authentication enabled")
	} else {
		log.Printf("WARNING: Authentication disabled - AUTH_TOKEN not set")
	}

	// Set up routes
	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	mux.Handle("/metrics", MetricsHandler())
	mux.Handle("/", stateHandlerWithAuth)

	// Add middleware (metrics wraps logging wraps routes)
	handler := metricsMiddleware(loggingMiddleware(mux))

	// Configure server with timeouts
	server := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second, // Higher to allow for slow Gitea responses
		IdleTimeout:  120 * time.Second,
	}

	// Start the server in a goroutine
	log.Printf("Starting server on %s", cfg.ListenAddr)
	log.Printf("Gitea: %s/%s/%s (branch: %s)", cfg.GiteaURL, cfg.GiteaOwner, cfg.GiteaRepo, cfg.GiteaBranch)

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Give outstanding requests 30 seconds to complete
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server stopped")
}

// authMiddleware checks for a valid Bearer token.
func authMiddleware(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")

		// Support both "Bearer <token>" and basic auth (Terraform sends password as basic auth)
		var providedToken string

		if strings.HasPrefix(auth, "Bearer ") {
			providedToken = strings.TrimPrefix(auth, "Bearer ")
		} else if strings.HasPrefix(auth, "Basic ") {
			// Terraform's http backend sends the password as basic auth
			// The password is in the format "username:password" base64 encoded
			// We only care about the password part
			username, password, ok := r.BasicAuth()
			if ok {
				// Use password as the token (username is ignored)
				_ = username
				providedToken = password
			}
		}

		if subtle.ConstantTimeCompare([]byte(providedToken), []byte(token)) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="terraform-state"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// loggingMiddleware logs each request.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

// handleHealth responds to health check requests.
func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
