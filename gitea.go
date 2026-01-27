package main

import (
	"encoding/base64"
	"errors"
	"fmt"

	"code.gitea.io/sdk/gitea"
)

// ErrFileAlreadyExists is returned when attempting to create a file that already exists.
// This enables callers to handle conflict scenarios (e.g., concurrent lock creation).
var ErrFileAlreadyExists = errors.New("file already exists")

type GiteaClient struct {
	client *gitea.Client
	owner  string
	repo   string
	branch string
}

func NewGiteaClient(cfg *Config) (*GiteaClient, error) {
	client, err := gitea.NewClient(cfg.GiteaURL, gitea.SetToken(cfg.GiteaToken))
	if err != nil {
		return nil, fmt.Errorf("failed to create gitea client: %w", err)
	}

	return &GiteaClient{
		client: client,
		owner:  cfg.GiteaOwner,
		repo:   cfg.GiteaRepo,
		branch: cfg.GiteaBranch,
	}, nil
}

// GetFile retrieves a file's content and SHA from the repository.
// Returns content, SHA, and error. If file doesn't exist, returns nil content with no error.
func (g *GiteaClient) GetFile(path string) ([]byte, string, error) {
	content, resp, err := g.client.GetContents(g.owner, g.repo, g.branch, path)
	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			return nil, "", nil // File doesn't exist
		}
		return nil, "", fmt.Errorf("failed to get file %s: %w", path, err)
	}

	if content == nil {
		return nil, "", nil
	}

	// Content is base64 encoded
	decoded, err := base64.StdEncoding.DecodeString(*content.Content)
	if err != nil {
		return nil, "", fmt.Errorf("failed to decode file content: %w", err)
	}

	return decoded, content.SHA, nil
}

// FileExists checks if a file exists and returns its SHA if it does.
func (g *GiteaClient) FileExists(path string) (bool, string, error) {
	content, sha, err := g.GetFile(path)
	if err != nil {
		return false, "", err
	}
	return content != nil, sha, nil
}

// CreateFile creates a new file in the repository.
// Returns ErrFileAlreadyExists if the file already exists (HTTP 422 from Gitea).
func (g *GiteaClient) CreateFile(path string, content []byte, message string) error {
	_, resp, err := g.client.CreateFile(g.owner, g.repo, path, gitea.CreateFileOptions{
		FileOptions: gitea.FileOptions{
			Message:    message,
			BranchName: g.branch,
		},
		Content: base64.StdEncoding.EncodeToString(content),
	})
	if err != nil {
		// Gitea returns 422 Unprocessable Entity when file already exists
		if resp != nil && resp.StatusCode == 422 {
			return ErrFileAlreadyExists
		}
		return fmt.Errorf("failed to create file %s: %w", path, err)
	}
	return nil
}

// UpdateFile updates an existing file in the repository.
func (g *GiteaClient) UpdateFile(path string, content []byte, sha string, message string) error {
	_, _, err := g.client.UpdateFile(g.owner, g.repo, path, gitea.UpdateFileOptions{
		FileOptions: gitea.FileOptions{
			Message:    message,
			BranchName: g.branch,
		},
		SHA:     sha,
		Content: base64.StdEncoding.EncodeToString(content),
	})
	if err != nil {
		return fmt.Errorf("failed to update file %s: %w", path, err)
	}
	return nil
}

// DeleteFile deletes a file from the repository.
func (g *GiteaClient) DeleteFile(path string, sha string, message string) error {
	_, err := g.client.DeleteFile(g.owner, g.repo, path, gitea.DeleteFileOptions{
		FileOptions: gitea.FileOptions{
			Message:    message,
			BranchName: g.branch,
		},
		SHA: sha,
	})
	if err != nil {
		return fmt.Errorf("failed to delete file %s: %w", path, err)
	}
	return nil
}

// CreateOrUpdateFile creates a file if it doesn't exist, or updates it if it does.
func (g *GiteaClient) CreateOrUpdateFile(path string, content []byte, message string) error {
	exists, sha, err := g.FileExists(path)
	if err != nil {
		return err
	}

	if exists {
		return g.UpdateFile(path, content, sha, message)
	}
	return g.CreateFile(path, content, message)
}
