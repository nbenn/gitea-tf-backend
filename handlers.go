package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

// LockInfo represents the Terraform lock information structure.
type LockInfo struct {
	ID        string `json:"ID"`
	Operation string `json:"Operation"`
	Info      string `json:"Info"`
	Who       string `json:"Who"`
	Version   string `json:"Version"`
	Created   string `json:"Created"`
	Path      string `json:"Path"`
}

type StateHandler struct {
	gitea *GiteaClient
}

func NewStateHandler(gitea *GiteaClient) *StateHandler {
	return &StateHandler{gitea: gitea}
}

// statePath returns the path to the state file for a given state name.
func statePath(name string) string {
	return fmt.Sprintf("states/%s/terraform.tfstate", name)
}

// lockPath returns the path to the lock file for a given state name.
func lockPath(name string) string {
	return fmt.Sprintf("states/%s/.lock", name)
}

// extractStateName extracts the state name from the URL path.
func extractStateName(path string) string {
	// Remove leading slash and any trailing slashes
	name := strings.Trim(path, "/")
	return name
}

// ServeHTTP handles all state-related requests.
func (h *StateHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := extractStateName(r.URL.Path)
	if name == "" {
		http.Error(w, "state name required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.handleGet(w, r, name)
	case http.MethodPost:
		h.handlePost(w, r, name)
	case "LOCK":
		h.handleLock(w, r, name)
	case "UNLOCK":
		h.handleUnlock(w, r, name)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleGet retrieves the current state.
func (h *StateHandler) handleGet(w http.ResponseWriter, r *http.Request, name string) {
	content, _, err := h.gitea.GetFile(statePath(name))
	if err != nil {
		log.Printf("Error getting state %s: %v", name, err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if content == nil {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(content)
}

// handlePost saves the state.
func (h *StateHandler) handlePost(w http.ResponseWriter, r *http.Request, name string) {
	// Check if there's a lock and validate the lock ID
	lockContent, _, err := h.gitea.GetFile(lockPath(name))
	if err != nil {
		log.Printf("Error checking lock for %s: %v", name, err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// If locked, verify the lock ID matches
	if lockContent != nil {
		lockID := r.Header.Get("Lock-Id")
		if lockID == "" {
			// Terraform may also send it as a query param
			lockID = r.URL.Query().Get("ID")
		}

		var existingLock LockInfo
		if err := json.Unmarshal(lockContent, &existingLock); err != nil {
			log.Printf("Error parsing lock for %s: %v", name, err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		if lockID != existingLock.ID {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusLocked)
			json.NewEncoder(w).Encode(existingLock)
			return
		}
	}

	// Read the state body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error reading body for %s: %v", name, err)
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	// Save the state
	err = h.gitea.CreateOrUpdateFile(statePath(name), body, fmt.Sprintf("Update state: %s", name))
	if err != nil {
		log.Printf("Error saving state %s: %v", name, err)
		http.Error(w, "failed to save state", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// handleLock acquires a lock for the state.
func (h *StateHandler) handleLock(w http.ResponseWriter, r *http.Request, name string) {
	// Read the lock info from the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error reading lock body for %s: %v", name, err)
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	var lockInfo LockInfo
	if err := json.Unmarshal(body, &lockInfo); err != nil {
		log.Printf("Error parsing lock body for %s: %v", name, err)
		http.Error(w, "invalid lock info", http.StatusBadRequest)
		return
	}

	// Check if already locked
	existingContent, sha, err := h.gitea.GetFile(lockPath(name))
	if err != nil {
		log.Printf("Error checking existing lock for %s: %v", name, err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if existingContent != nil {
		// Already locked - return the existing lock info
		var existingLock LockInfo
		if err := json.Unmarshal(existingContent, &existingLock); err != nil {
			log.Printf("Error parsing existing lock for %s: %v", name, err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		// If it's the same lock ID, consider it a re-lock (idempotent)
		if existingLock.ID == lockInfo.ID {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(existingLock)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusLocked)
		json.NewEncoder(w).Encode(existingLock)
		return
	}

	// Create the lock file
	lockJSON, err := json.Marshal(lockInfo)
	if err != nil {
		log.Printf("Error marshaling lock for %s: %v", name, err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Use CreateFile directly to avoid race conditions
	// If someone else creates the lock between our check and create, this will fail
	if sha == "" {
		err = h.gitea.CreateFile(lockPath(name), lockJSON, fmt.Sprintf("Lock state: %s", name))
	} else {
		// This shouldn't happen since we checked existingContent == nil, but handle it
		err = h.gitea.UpdateFile(lockPath(name), lockJSON, sha, fmt.Sprintf("Lock state: %s", name))
	}

	if err != nil {
		// Could be a race condition - check if lock exists now
		existingContent, _, _ := h.gitea.GetFile(lockPath(name))
		if existingContent != nil {
			var existingLock LockInfo
			json.Unmarshal(existingContent, &existingLock)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusLocked)
			json.NewEncoder(w).Encode(existingLock)
			return
		}
		log.Printf("Error creating lock for %s: %v", name, err)
		http.Error(w, "failed to create lock", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(lockInfo)
}

// handleUnlock releases a lock for the state.
func (h *StateHandler) handleUnlock(w http.ResponseWriter, r *http.Request, name string) {
	// Read the lock info from the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error reading unlock body for %s: %v", name, err)
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	var unlockInfo LockInfo
	if err := json.Unmarshal(body, &unlockInfo); err != nil {
		log.Printf("Error parsing unlock body for %s: %v", name, err)
		http.Error(w, "invalid lock info", http.StatusBadRequest)
		return
	}

	// Get the existing lock
	existingContent, sha, err := h.gitea.GetFile(lockPath(name))
	if err != nil {
		log.Printf("Error checking lock for unlock %s: %v", name, err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if existingContent == nil {
		// No lock exists - success (idempotent)
		w.WriteHeader(http.StatusOK)
		return
	}

	var existingLock LockInfo
	if err := json.Unmarshal(existingContent, &existingLock); err != nil {
		log.Printf("Error parsing existing lock for unlock %s: %v", name, err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Verify the lock ID matches (unless force unlock with empty ID)
	if unlockInfo.ID != "" && unlockInfo.ID != existingLock.ID {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(existingLock)
		return
	}

	// Delete the lock file
	err = h.gitea.DeleteFile(lockPath(name), sha, fmt.Sprintf("Unlock state: %s", name))
	if err != nil {
		log.Printf("Error deleting lock for %s: %v", name, err)
		http.Error(w, "failed to delete lock", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
