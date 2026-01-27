package main

import (
	"fmt"
	"os"
)

type Config struct {
	GiteaURL    string
	GiteaToken  string
	GiteaOwner  string
	GiteaRepo   string
	GiteaBranch string
	ListenAddr  string
	AuthToken   string // Optional - if empty, no auth required
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
