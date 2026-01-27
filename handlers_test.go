package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// MockGiteaClient implements file operations in memory for testing.
type MockGiteaClient struct {
	files map[string][]byte
}

func NewMockGiteaClient() *MockGiteaClient {
	return &MockGiteaClient{
		files: make(map[string][]byte),
	}
}

func (m *MockGiteaClient) GetFile(path string) ([]byte, string, error) {
	content, exists := m.files[path]
	if !exists {
		return nil, "", nil
	}
	return content, "sha-" + path, nil
}

func (m *MockGiteaClient) FileExists(path string) (bool, string, error) {
	content, sha, err := m.GetFile(path)
	if err != nil {
		return false, "", err
	}
	return content != nil, sha, nil
}

func (m *MockGiteaClient) CreateFile(path string, content []byte, message string) error {
	m.files[path] = content
	return nil
}

func (m *MockGiteaClient) UpdateFile(path string, content []byte, sha string, message string) error {
	m.files[path] = content
	return nil
}

func (m *MockGiteaClient) DeleteFile(path string, sha string, message string) error {
	delete(m.files, path)
	return nil
}

func (m *MockGiteaClient) CreateOrUpdateFile(path string, content []byte, message string) error {
	m.files[path] = content
	return nil
}

// GiteaFileClient interface for dependency injection
type GiteaFileClient interface {
	GetFile(path string) ([]byte, string, error)
	FileExists(path string) (bool, string, error)
	CreateFile(path string, content []byte, message string) error
	UpdateFile(path string, content []byte, sha string, message string) error
	DeleteFile(path string, sha string, message string) error
	CreateOrUpdateFile(path string, content []byte, message string) error
}

// TestableStateHandler is like StateHandler but accepts a GiteaFileClient interface
// for testing with mocks.
type TestableStateHandler struct {
	client GiteaFileClient
	mu     sync.RWMutex
	locks  map[string]LockInfo
}

func NewTestableStateHandler(client GiteaFileClient) *TestableStateHandler {
	return &TestableStateHandler{
		client: client,
		locks:  make(map[string]LockInfo),
	}
}

func (h *TestableStateHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := extractStateName(r.URL.Path)
	if name == "" {
		http.Error(w, "state name required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		content, _, err := h.client.GetFile(statePath(name))
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if content == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(content)

	case http.MethodPost:
		h.mu.RLock()
		existingLock, locked := h.locks[name]
		h.mu.RUnlock()

		if locked {
			lockID := r.Header.Get("Lock-Id")
			if lockID != existingLock.ID {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusLocked)
				json.NewEncoder(w).Encode(existingLock)
				return
			}
		}

		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
		if err := h.client.CreateOrUpdateFile(statePath(name), body, "Update state"); err != nil {
			http.Error(w, "failed to save state", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)

	case "LOCK":
		var lockInfo LockInfo
		if err := json.NewDecoder(r.Body).Decode(&lockInfo); err != nil {
			http.Error(w, "invalid lock info", http.StatusBadRequest)
			return
		}

		h.mu.Lock()
		defer h.mu.Unlock()

		if existing, locked := h.locks[name]; locked {
			if existing.ID == lockInfo.ID {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(existing)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusLocked)
			json.NewEncoder(w).Encode(existing)
			return
		}

		h.locks[name] = lockInfo
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(lockInfo)

	case "UNLOCK":
		var unlockInfo LockInfo
		if err := json.NewDecoder(r.Body).Decode(&unlockInfo); err != nil {
			http.Error(w, "invalid lock info", http.StatusBadRequest)
			return
		}

		h.mu.Lock()
		defer h.mu.Unlock()

		existing, locked := h.locks[name]
		if !locked {
			w.WriteHeader(http.StatusOK)
			return
		}

		if unlockInfo.ID != "" && unlockInfo.ID != existing.ID {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(existing)
			return
		}

		delete(h.locks, name)
		w.WriteHeader(http.StatusOK)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// Tests

func TestGetState_NotFound(t *testing.T) {
	mock := NewMockGiteaClient()
	handler := NewTestableStateHandler(mock)

	req := httptest.NewRequest(http.MethodGet, "/myproject", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestGetState_Found(t *testing.T) {
	mock := NewMockGiteaClient()
	handler := NewTestableStateHandler(mock)

	stateData := []byte(`{"version":4,"terraform_version":"1.0.0"}`)
	mock.files["states/myproject/terraform.tfstate"] = stateData

	req := httptest.NewRequest(http.MethodGet, "/myproject", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if !bytes.Equal(w.Body.Bytes(), stateData) {
		t.Errorf("expected body %s, got %s", stateData, w.Body.Bytes())
	}
}

func TestPostState_NoLock(t *testing.T) {
	mock := NewMockGiteaClient()
	handler := NewTestableStateHandler(mock)

	stateData := []byte(`{"version":4,"terraform_version":"1.0.0"}`)
	req := httptest.NewRequest(http.MethodPost, "/myproject", bytes.NewReader(stateData))
	req.ContentLength = int64(len(stateData))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	saved := mock.files["states/myproject/terraform.tfstate"]
	if !bytes.Equal(saved, stateData) {
		t.Errorf("state not saved correctly")
	}
}

func TestPostState_WithMatchingLock(t *testing.T) {
	mock := NewMockGiteaClient()
	handler := NewTestableStateHandler(mock)

	// Create a lock in-memory
	handler.locks["myproject"] = LockInfo{ID: "lock-123", Operation: "apply"}

	stateData := []byte(`{"version":4}`)
	req := httptest.NewRequest(http.MethodPost, "/myproject", bytes.NewReader(stateData))
	req.ContentLength = int64(len(stateData))
	req.Header.Set("Lock-Id", "lock-123")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestPostState_WithWrongLock(t *testing.T) {
	mock := NewMockGiteaClient()
	handler := NewTestableStateHandler(mock)

	// Create a lock in-memory
	handler.locks["myproject"] = LockInfo{ID: "lock-123", Operation: "apply"}

	stateData := []byte(`{"version":4}`)
	req := httptest.NewRequest(http.MethodPost, "/myproject", bytes.NewReader(stateData))
	req.ContentLength = int64(len(stateData))
	req.Header.Set("Lock-Id", "wrong-lock")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusLocked {
		t.Errorf("expected status 423, got %d", w.Code)
	}
}

func TestLock_Success(t *testing.T) {
	mock := NewMockGiteaClient()
	handler := NewTestableStateHandler(mock)

	lockInfo := LockInfo{ID: "lock-123", Operation: "apply", Who: "user@host"}
	lockJSON, _ := json.Marshal(lockInfo)

	req := httptest.NewRequest("LOCK", "/myproject", bytes.NewReader(lockJSON))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// Verify lock was created in-memory
	if _, exists := handler.locks["myproject"]; !exists {
		t.Error("lock was not created")
	}
}

func TestLock_AlreadyLocked(t *testing.T) {
	mock := NewMockGiteaClient()
	handler := NewTestableStateHandler(mock)

	// Create existing lock in-memory
	handler.locks["myproject"] = LockInfo{ID: "existing-lock", Operation: "apply"}

	// Try to acquire new lock
	newLock := LockInfo{ID: "new-lock", Operation: "apply"}
	newJSON, _ := json.Marshal(newLock)

	req := httptest.NewRequest("LOCK", "/myproject", bytes.NewReader(newJSON))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusLocked {
		t.Errorf("expected status 423, got %d", w.Code)
	}

	var returnedLock LockInfo
	json.NewDecoder(w.Body).Decode(&returnedLock)
	if returnedLock.ID != "existing-lock" {
		t.Errorf("expected existing lock ID, got %s", returnedLock.ID)
	}
}

func TestLock_Idempotent(t *testing.T) {
	mock := NewMockGiteaClient()
	handler := NewTestableStateHandler(mock)

	// Create existing lock with same ID
	handler.locks["myproject"] = LockInfo{ID: "lock-123", Operation: "apply"}

	// Try to acquire same lock again
	lockInfo := LockInfo{ID: "lock-123", Operation: "apply"}
	lockJSON, _ := json.Marshal(lockInfo)

	req := httptest.NewRequest("LOCK", "/myproject", bytes.NewReader(lockJSON))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200 for idempotent lock, got %d", w.Code)
	}
}

func TestUnlock_Success(t *testing.T) {
	mock := NewMockGiteaClient()
	handler := NewTestableStateHandler(mock)

	// Create existing lock in-memory
	handler.locks["myproject"] = LockInfo{ID: "lock-123", Operation: "apply"}

	lockInfo := LockInfo{ID: "lock-123"}
	lockJSON, _ := json.Marshal(lockInfo)

	req := httptest.NewRequest("UNLOCK", "/myproject", bytes.NewReader(lockJSON))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// Verify lock was deleted
	if _, exists := handler.locks["myproject"]; exists {
		t.Error("lock was not deleted")
	}
}

func TestUnlock_WrongID(t *testing.T) {
	mock := NewMockGiteaClient()
	handler := NewTestableStateHandler(mock)

	// Create existing lock in-memory
	handler.locks["myproject"] = LockInfo{ID: "lock-123", Operation: "apply"}

	// Try to unlock with wrong ID
	wrongLock := LockInfo{ID: "wrong-id"}
	wrongJSON, _ := json.Marshal(wrongLock)

	req := httptest.NewRequest("UNLOCK", "/myproject", bytes.NewReader(wrongJSON))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected status 409, got %d", w.Code)
	}

	// Lock should still exist
	if _, exists := handler.locks["myproject"]; !exists {
		t.Error("lock should not be deleted")
	}
}

func TestUnlock_NoLock(t *testing.T) {
	mock := NewMockGiteaClient()
	handler := NewTestableStateHandler(mock)

	unlockInfo := LockInfo{ID: "any-id"}
	unlockJSON, _ := json.Marshal(unlockInfo)

	req := httptest.NewRequest("UNLOCK", "/myproject", bytes.NewReader(unlockJSON))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200 for unlock with no lock, got %d", w.Code)
	}
}

func TestUnlock_ForceUnlock(t *testing.T) {
	mock := NewMockGiteaClient()
	handler := NewTestableStateHandler(mock)

	// Create existing lock in-memory
	handler.locks["myproject"] = LockInfo{ID: "lock-123", Operation: "apply"}

	// Force unlock with empty ID
	forceLock := LockInfo{ID: ""}
	forceJSON, _ := json.Marshal(forceLock)

	req := httptest.NewRequest("UNLOCK", "/myproject", bytes.NewReader(forceJSON))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200 for force unlock, got %d", w.Code)
	}

	// Lock should be deleted
	if _, exists := handler.locks["myproject"]; exists {
		t.Error("lock should be deleted on force unlock")
	}
}

func TestStatePath(t *testing.T) {
	path := statePath("myproject")
	expected := "states/myproject/terraform.tfstate"
	if path != expected {
		t.Errorf("expected %s, got %s", expected, path)
	}
}

func TestExtractStateName(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"/myproject", "myproject"},
		{"/myproject/", "myproject"},
		{"myproject", "myproject"},
		{"/org/project", "org/project"},
	}

	for _, tt := range tests {
		result := extractStateName(tt.path)
		if result != tt.expected {
			t.Errorf("extractStateName(%q) = %q, expected %q", tt.path, result, tt.expected)
		}
	}
}
