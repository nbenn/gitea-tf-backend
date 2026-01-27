package main

import (
	"log"
	"net/http"
	"strings"
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
	stateHandler := NewStateHandler(giteaClient)

	// Create the main handler with optional auth middleware
	var handler http.Handler = stateHandler
	if cfg.AuthToken != "" {
		handler = authMiddleware(cfg.AuthToken, stateHandler)
		log.Printf("Authentication enabled")
	} else {
		log.Printf("WARNING: Authentication disabled - AUTH_TOKEN not set")
	}

	// Add logging middleware
	handler = loggingMiddleware(handler)

	// Start the server
	log.Printf("Starting server on %s", cfg.ListenAddr)
	log.Printf("Gitea: %s/%s/%s (branch: %s)", cfg.GiteaURL, cfg.GiteaOwner, cfg.GiteaRepo, cfg.GiteaBranch)

	if err := http.ListenAndServe(cfg.ListenAddr, handler); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
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

		if providedToken != token {
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
