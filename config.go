package main

import (
	"fmt"
	"os"
	"strconv"
)

// Default maximum request body size (50 MB).
const DefaultMaxBodySize = 50 << 20

type Config struct {
	GiteaURL    string
	GiteaToken  string
	GiteaOwner  string
	GiteaRepo   string
	GiteaBranch string
	ListenAddr  string
	AuthToken   string // Optional - if empty, no auth required
	MaxBodySize int64  // Maximum request body size in bytes
}

func LoadConfig() (*Config, error) {
	cfg := &Config{
		GiteaURL:    os.Getenv("GITEA_URL"),
		GiteaToken:  os.Getenv("GITEA_TOKEN"),
		GiteaOwner:  os.Getenv("GITEA_OWNER"),
		GiteaRepo:   os.Getenv("GITEA_REPO"),
		GiteaBranch: os.Getenv("GITEA_BRANCH"),
		ListenAddr:  os.Getenv("LISTEN_ADDR"),
		AuthToken:   os.Getenv("AUTH_TOKEN"),
	}

	// Set defaults
	if cfg.GiteaBranch == "" {
		cfg.GiteaBranch = "main"
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":8080"
	}

	// Parse max body size (in MB)
	cfg.MaxBodySize = DefaultMaxBodySize
	if maxBodyMB := os.Getenv("MAX_BODY_SIZE_MB"); maxBodyMB != "" {
		mb, err := strconv.ParseInt(maxBodyMB, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("MAX_BODY_SIZE_MB must be a valid integer: %w", err)
		}
		if mb <= 0 {
			return nil, fmt.Errorf("MAX_BODY_SIZE_MB must be positive")
		}
		cfg.MaxBodySize = mb << 20 // Convert MB to bytes
	}

	// Validate required fields
	if cfg.GiteaURL == "" {
		return nil, fmt.Errorf("GITEA_URL is required")
	}
	if cfg.GiteaToken == "" {
		return nil, fmt.Errorf("GITEA_TOKEN is required")
	}
	if cfg.GiteaOwner == "" {
		return nil, fmt.Errorf("GITEA_OWNER is required")
	}
	if cfg.GiteaRepo == "" {
		return nil, fmt.Errorf("GITEA_REPO is required")
	}

	return cfg, nil
}
