package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
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

// StateStorage defines the interface for state file operations.
type StateStorage interface {
	GetFile(path string) ([]byte, string, error)
	CreateOrUpdateFile(path string, content []byte, message string) error
}

// StateHandler handles Terraform state HTTP requests.
// Locks are held in-memory for simplicity (single-instance deployment).
type StateHandler struct {
	storage StateStorage

	mu    sync.RWMutex
	locks map[string]LockInfo // keyed by state name
}

// NewStateHandler creates a new StateHandler with the given storage backend.
func NewStateHandler(storage StateStorage) *StateHandler {
	return &StateHandler{
		storage: storage,
		locks:   make(map[string]LockInfo),
	}
}

// statePath returns the path to the state file for a given state name.
func statePath(name string) string {
	return fmt.Sprintf("states/%s/terraform.tfstate", name)
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
	content, _, err := h.storage.GetFile(statePath(name))
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
	_, _ = w.Write(content)
}

// handlePost saves the state.
func (h *StateHandler) handlePost(w http.ResponseWriter, r *http.Request, name string) {
	// Check if there's a lock and validate the lock ID
	h.mu.RLock()
	existingLock, locked := h.locks[name]
	h.mu.RUnlock()

	if locked {
		lockID := r.Header.Get("Lock-Id")
		if lockID == "" {
			// Terraform may also send it as a query param
			lockID = r.URL.Query().Get("ID")
		}

		if lockID != existingLock.ID {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusLocked)
			_ = json.NewEncoder(w).Encode(existingLock)
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
	err = h.storage.CreateOrUpdateFile(statePath(name), body, fmt.Sprintf("Update state: %s", name))
	if err != nil {
		log.Printf("Error saving state %s: %v", name, err)
		http.Error(w, "failed to save state", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// handleLock acquires a lock for the state.
func (h *StateHandler) handleLock(w http.ResponseWriter, r *http.Request, name string) {
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

	h.mu.Lock()
	defer h.mu.Unlock()

	if existingLock, locked := h.locks[name]; locked {
		if existingLock.ID == lockInfo.ID {
			// Same lock ID - idempotent success
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(existingLock)
			return
		}
		// Different lock - return 423 Locked
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusLocked)
		_ = json.NewEncoder(w).Encode(existingLock)
		return
	}

	// Acquire the lock
	h.locks[name] = lockInfo

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(lockInfo)
}

// handleUnlock releases a lock for the state.
func (h *StateHandler) handleUnlock(w http.ResponseWriter, r *http.Request, name string) {
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

	h.mu.Lock()
	defer h.mu.Unlock()

	existingLock, locked := h.locks[name]
	if !locked {
		// No lock exists - success (idempotent)
		w.WriteHeader(http.StatusOK)
		return
	}

	// Verify the lock ID matches (unless force unlock with empty ID)
	if unlockInfo.ID != "" && unlockInfo.ID != existingLock.ID {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(existingLock)
		return
	}

	// Release the lock
	delete(h.locks, name)

	w.WriteHeader(http.StatusOK)
}
