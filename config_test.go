package main

import (
	"testing"
)

func TestLoadConfig_Success(t *testing.T) {
	// t.Setenv automatically restores the original value after the test
	t.Setenv("GITEA_URL", "https://gitea.example.com")
	t.Setenv("GITEA_TOKEN", "test-token")
	t.Setenv("GITEA_OWNER", "testowner")
	t.Setenv("GITEA_REPO", "testrepo")
	t.Setenv("GITEA_BRANCH", "develop")
	t.Setenv("LISTEN_ADDR", ":9090")
	t.Setenv("AUTH_TOKEN", "auth-secret")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.GiteaURL != "https://gitea.example.com" {
		t.Errorf("expected GiteaURL %q, got %q", "https://gitea.example.com", cfg.GiteaURL)
	}
	if cfg.GiteaToken != "test-token" {
		t.Errorf("expected GiteaToken %q, got %q", "test-token", cfg.GiteaToken)
	}
	if cfg.GiteaOwner != "testowner" {
		t.Errorf("expected GiteaOwner %q, got %q", "testowner", cfg.GiteaOwner)
	}
	if cfg.GiteaRepo != "testrepo" {
		t.Errorf("expected GiteaRepo %q, got %q", "testrepo", cfg.GiteaRepo)
	}
	if cfg.GiteaBranch != "develop" {
		t.Errorf("expected GiteaBranch %q, got %q", "develop", cfg.GiteaBranch)
	}
	if cfg.ListenAddr != ":9090" {
		t.Errorf("expected ListenAddr %q, got %q", ":9090", cfg.ListenAddr)
	}
	if cfg.AuthToken != "auth-secret" {
		t.Errorf("expected AuthToken %q, got %q", "auth-secret", cfg.AuthToken)
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	// Set only required env vars, leave optional ones empty
	t.Setenv("GITEA_URL", "https://gitea.example.com")
	t.Setenv("GITEA_TOKEN", "test-token")
	t.Setenv("GITEA_OWNER", "testowner")
	t.Setenv("GITEA_REPO", "testrepo")
	t.Setenv("GITEA_BRANCH", "")
	t.Setenv("LISTEN_ADDR", "")
	t.Setenv("AUTH_TOKEN", "")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check defaults
	if cfg.GiteaBranch != "main" {
		t.Errorf("expected default GiteaBranch %q, got %q", "main", cfg.GiteaBranch)
	}
	if cfg.ListenAddr != ":8080" {
		t.Errorf("expected default ListenAddr %q, got %q", ":8080", cfg.ListenAddr)
	}
	if cfg.AuthToken != "" {
		t.Errorf("expected empty AuthToken, got %q", cfg.AuthToken)
	}
	if cfg.MaxBodySize != DefaultMaxBodySize {
		t.Errorf("expected default MaxBodySize %d, got %d", DefaultMaxBodySize, cfg.MaxBodySize)
	}
}

func TestLoadConfig_CustomMaxBodySize(t *testing.T) {
	t.Setenv("GITEA_URL", "https://gitea.example.com")
	t.Setenv("GITEA_TOKEN", "test-token")
	t.Setenv("GITEA_OWNER", "testowner")
	t.Setenv("GITEA_REPO", "testrepo")
	t.Setenv("MAX_BODY_SIZE_MB", "100")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := int64(100 << 20) // 100 MB
	if cfg.MaxBodySize != expected {
		t.Errorf("expected MaxBodySize %d, got %d", expected, cfg.MaxBodySize)
	}
}

func TestLoadConfig_InvalidMaxBodySize(t *testing.T) {
	t.Setenv("GITEA_URL", "https://gitea.example.com")
	t.Setenv("GITEA_TOKEN", "test-token")
	t.Setenv("GITEA_OWNER", "testowner")
	t.Setenv("GITEA_REPO", "testrepo")
	t.Setenv("MAX_BODY_SIZE_MB", "not-a-number")

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error for invalid MAX_BODY_SIZE_MB")
	}
}

func TestLoadConfig_NegativeMaxBodySize(t *testing.T) {
	t.Setenv("GITEA_URL", "https://gitea.example.com")
	t.Setenv("GITEA_TOKEN", "test-token")
	t.Setenv("GITEA_OWNER", "testowner")
	t.Setenv("GITEA_REPO", "testrepo")
	t.Setenv("MAX_BODY_SIZE_MB", "-5")

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error for negative MAX_BODY_SIZE_MB")
	}
}

func TestLoadConfig_MissingGiteaURL(t *testing.T) {
	t.Setenv("GITEA_URL", "")
	t.Setenv("GITEA_TOKEN", "test-token")
	t.Setenv("GITEA_OWNER", "testowner")
	t.Setenv("GITEA_REPO", "testrepo")

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error for missing GITEA_URL")
	}
	if err.Error() != "GITEA_URL is required" {
		t.Errorf("expected error message %q, got %q", "GITEA_URL is required", err.Error())
	}
}

func TestLoadConfig_MissingGiteaToken(t *testing.T) {
	t.Setenv("GITEA_URL", "https://gitea.example.com")
	t.Setenv("GITEA_TOKEN", "")
	t.Setenv("GITEA_OWNER", "testowner")
	t.Setenv("GITEA_REPO", "testrepo")

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error for missing GITEA_TOKEN")
	}
	if err.Error() != "GITEA_TOKEN is required" {
		t.Errorf("expected error message %q, got %q", "GITEA_TOKEN is required", err.Error())
	}
}

func TestLoadConfig_MissingGiteaOwner(t *testing.T) {
	t.Setenv("GITEA_URL", "https://gitea.example.com")
	t.Setenv("GITEA_TOKEN", "test-token")
	t.Setenv("GITEA_OWNER", "")
	t.Setenv("GITEA_REPO", "testrepo")

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error for missing GITEA_OWNER")
	}
	if err.Error() != "GITEA_OWNER is required" {
		t.Errorf("expected error message %q, got %q", "GITEA_OWNER is required", err.Error())
	}
}

func TestLoadConfig_MissingGiteaRepo(t *testing.T) {
	t.Setenv("GITEA_URL", "https://gitea.example.com")
	t.Setenv("GITEA_TOKEN", "test-token")
	t.Setenv("GITEA_OWNER", "testowner")
	t.Setenv("GITEA_REPO", "")

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error for missing GITEA_REPO")
	}
	if err.Error() != "GITEA_REPO is required" {
		t.Errorf("expected error message %q, got %q", "GITEA_REPO is required", err.Error())
	}
}
