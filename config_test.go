package main

import (
	"os"
	"testing"
)

func TestLoadConfig_Success(t *testing.T) {
	// Save current env and restore after test
	envVars := []string{"GITEA_URL", "GITEA_TOKEN", "GITEA_OWNER", "GITEA_REPO", "GITEA_BRANCH", "LISTEN_ADDR", "AUTH_TOKEN"}
	saved := make(map[string]string)
	for _, key := range envVars {
		saved[key] = os.Getenv(key)
	}
	defer func() {
		for key, val := range saved {
			if val == "" {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, val)
			}
		}
	}()

	// Set required env vars
	os.Setenv("GITEA_URL", "https://gitea.example.com")
	os.Setenv("GITEA_TOKEN", "test-token")
	os.Setenv("GITEA_OWNER", "testowner")
	os.Setenv("GITEA_REPO", "testrepo")
	os.Setenv("GITEA_BRANCH", "develop")
	os.Setenv("LISTEN_ADDR", ":9090")
	os.Setenv("AUTH_TOKEN", "auth-secret")

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
	// Save current env and restore after test
	envVars := []string{"GITEA_URL", "GITEA_TOKEN", "GITEA_OWNER", "GITEA_REPO", "GITEA_BRANCH", "LISTEN_ADDR", "AUTH_TOKEN"}
	saved := make(map[string]string)
	for _, key := range envVars {
		saved[key] = os.Getenv(key)
	}
	defer func() {
		for key, val := range saved {
			if val == "" {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, val)
			}
		}
	}()

	// Set only required env vars, leave optional ones unset
	os.Setenv("GITEA_URL", "https://gitea.example.com")
	os.Setenv("GITEA_TOKEN", "test-token")
	os.Setenv("GITEA_OWNER", "testowner")
	os.Setenv("GITEA_REPO", "testrepo")
	os.Unsetenv("GITEA_BRANCH")
	os.Unsetenv("LISTEN_ADDR")
	os.Unsetenv("AUTH_TOKEN")

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
}

func TestLoadConfig_MissingGiteaURL(t *testing.T) {
	// Save current env and restore after test
	envVars := []string{"GITEA_URL", "GITEA_TOKEN", "GITEA_OWNER", "GITEA_REPO"}
	saved := make(map[string]string)
	for _, key := range envVars {
		saved[key] = os.Getenv(key)
	}
	defer func() {
		for key, val := range saved {
			if val == "" {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, val)
			}
		}
	}()

	os.Unsetenv("GITEA_URL")
	os.Setenv("GITEA_TOKEN", "test-token")
	os.Setenv("GITEA_OWNER", "testowner")
	os.Setenv("GITEA_REPO", "testrepo")

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error for missing GITEA_URL")
	}
	if err.Error() != "GITEA_URL is required" {
		t.Errorf("expected error message %q, got %q", "GITEA_URL is required", err.Error())
	}
}

func TestLoadConfig_MissingGiteaToken(t *testing.T) {
	// Save current env and restore after test
	envVars := []string{"GITEA_URL", "GITEA_TOKEN", "GITEA_OWNER", "GITEA_REPO"}
	saved := make(map[string]string)
	for _, key := range envVars {
		saved[key] = os.Getenv(key)
	}
	defer func() {
		for key, val := range saved {
			if val == "" {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, val)
			}
		}
	}()

	os.Setenv("GITEA_URL", "https://gitea.example.com")
	os.Unsetenv("GITEA_TOKEN")
	os.Setenv("GITEA_OWNER", "testowner")
	os.Setenv("GITEA_REPO", "testrepo")

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error for missing GITEA_TOKEN")
	}
	if err.Error() != "GITEA_TOKEN is required" {
		t.Errorf("expected error message %q, got %q", "GITEA_TOKEN is required", err.Error())
	}
}

func TestLoadConfig_MissingGiteaOwner(t *testing.T) {
	// Save current env and restore after test
	envVars := []string{"GITEA_URL", "GITEA_TOKEN", "GITEA_OWNER", "GITEA_REPO"}
	saved := make(map[string]string)
	for _, key := range envVars {
		saved[key] = os.Getenv(key)
	}
	defer func() {
		for key, val := range saved {
			if val == "" {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, val)
			}
		}
	}()

	os.Setenv("GITEA_URL", "https://gitea.example.com")
	os.Setenv("GITEA_TOKEN", "test-token")
	os.Unsetenv("GITEA_OWNER")
	os.Setenv("GITEA_REPO", "testrepo")

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error for missing GITEA_OWNER")
	}
	if err.Error() != "GITEA_OWNER is required" {
		t.Errorf("expected error message %q, got %q", "GITEA_OWNER is required", err.Error())
	}
}

func TestLoadConfig_MissingGiteaRepo(t *testing.T) {
	// Save current env and restore after test
	envVars := []string{"GITEA_URL", "GITEA_TOKEN", "GITEA_OWNER", "GITEA_REPO"}
	saved := make(map[string]string)
	for _, key := range envVars {
		saved[key] = os.Getenv(key)
	}
	defer func() {
		for key, val := range saved {
			if val == "" {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, val)
			}
		}
	}()

	os.Setenv("GITEA_URL", "https://gitea.example.com")
	os.Setenv("GITEA_TOKEN", "test-token")
	os.Setenv("GITEA_OWNER", "testowner")
	os.Unsetenv("GITEA_REPO")

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error for missing GITEA_REPO")
	}
	if err.Error() != "GITEA_REPO is required" {
		t.Errorf("expected error message %q, got %q", "GITEA_REPO is required", err.Error())
	}
}
