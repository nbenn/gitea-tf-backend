package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// MockStorage implements StateStorage for testing.
type MockStorage struct {
	files map[string][]byte
}

func NewMockStorage() *MockStorage {
	return &MockStorage{
		files: make(map[string][]byte),
	}
}

func (m *MockStorage) GetFile(path string) ([]byte, string, error) {
	content, exists := m.files[path]
	if !exists {
		return nil, "", nil
	}
	return content, "sha-" + path, nil
}

func (m *MockStorage) CreateOrUpdateFile(path string, content []byte, _ string) error {
	m.files[path] = content
	return nil
}

// Test helpers

func newTestHandler() (*StateHandler, *MockStorage) {
	mock := NewMockStorage()
	handler := NewStateHandler(mock)
	return handler, mock
}

// Tests for StateHandler

func TestServeHTTP_EmptyStateName(t *testing.T) {
	handler, _ := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestServeHTTP_MethodNotAllowed(t *testing.T) {
	handler, _ := newTestHandler()

	req := httptest.NewRequest(http.MethodDelete, "/myproject", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestGetState_NotFound(t *testing.T) {
	handler, _ := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/myproject", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestGetState_Found(t *testing.T) {
	handler, mock := newTestHandler()

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

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}
}

func TestPostState_NoLock(t *testing.T) {
	handler, mock := newTestHandler()

	stateData := []byte(`{"version":4,"terraform_version":"1.0.0"}`)
	req := httptest.NewRequest(http.MethodPost, "/myproject", bytes.NewReader(stateData))
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
	handler, _ := newTestHandler()

	// Create a lock
	handler.locks["myproject"] = LockInfo{ID: "lock-123", Operation: "apply"}

	stateData := []byte(`{"version":4}`)
	req := httptest.NewRequest(http.MethodPost, "/myproject", bytes.NewReader(stateData))
	req.Header.Set("Lock-Id", "lock-123")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestPostState_WithMatchingLockQueryParam(t *testing.T) {
	handler, _ := newTestHandler()

	// Create a lock
	handler.locks["myproject"] = LockInfo{ID: "lock-123", Operation: "apply"}

	stateData := []byte(`{"version":4}`)
	req := httptest.NewRequest(http.MethodPost, "/myproject?ID=lock-123", bytes.NewReader(stateData))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestPostState_WithWrongLock(t *testing.T) {
	handler, _ := newTestHandler()

	// Create a lock
	handler.locks["myproject"] = LockInfo{ID: "lock-123", Operation: "apply"}

	stateData := []byte(`{"version":4}`)
	req := httptest.NewRequest(http.MethodPost, "/myproject", bytes.NewReader(stateData))
	req.Header.Set("Lock-Id", "wrong-lock")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusLocked {
		t.Errorf("expected status 423, got %d", w.Code)
	}
}

func TestLock_Success(t *testing.T) {
	handler, _ := newTestHandler()

	lockInfo := LockInfo{ID: "lock-123", Operation: "apply", Who: "user@host"}
	lockJSON, _ := json.Marshal(lockInfo)

	req := httptest.NewRequest("LOCK", "/myproject", bytes.NewReader(lockJSON))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if _, exists := handler.locks["myproject"]; !exists {
		t.Error("lock was not created")
	}

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}
}

func TestLock_InvalidJSON(t *testing.T) {
	handler, _ := newTestHandler()

	req := httptest.NewRequest("LOCK", "/myproject", bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestLock_AlreadyLocked(t *testing.T) {
	handler, _ := newTestHandler()

	// Create existing lock
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
	_ = json.NewDecoder(w.Body).Decode(&returnedLock)
	if returnedLock.ID != "existing-lock" {
		t.Errorf("expected existing lock ID, got %s", returnedLock.ID)
	}
}

func TestLock_Idempotent(t *testing.T) {
	handler, _ := newTestHandler()

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
	handler, _ := newTestHandler()

	// Create existing lock
	handler.locks["myproject"] = LockInfo{ID: "lock-123", Operation: "apply"}

	lockInfo := LockInfo{ID: "lock-123"}
	lockJSON, _ := json.Marshal(lockInfo)

	req := httptest.NewRequest("UNLOCK", "/myproject", bytes.NewReader(lockJSON))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if _, exists := handler.locks["myproject"]; exists {
		t.Error("lock was not deleted")
	}
}

func TestUnlock_InvalidJSON(t *testing.T) {
	handler, _ := newTestHandler()

	req := httptest.NewRequest("UNLOCK", "/myproject", bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestUnlock_WrongID(t *testing.T) {
	handler, _ := newTestHandler()

	// Create existing lock
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

	if _, exists := handler.locks["myproject"]; !exists {
		t.Error("lock should not be deleted")
	}
}

func TestUnlock_NoLock(t *testing.T) {
	handler, _ := newTestHandler()

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
	handler, _ := newTestHandler()

	// Create existing lock
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

	if _, exists := handler.locks["myproject"]; exists {
		t.Error("lock should be deleted on force unlock")
	}
}

// Tests for utility functions

func TestStatePath(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{"myproject", "states/myproject/terraform.tfstate"},
		{"org/project", "states/org/project/terraform.tfstate"},
	}

	for _, tt := range tests {
		result := statePath(tt.name)
		if result != tt.expected {
			t.Errorf("statePath(%q) = %q, expected %q", tt.name, result, tt.expected)
		}
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
		{"/", ""},
		{"", ""},
	}

	for _, tt := range tests {
		result := extractStateName(tt.path)
		if result != tt.expected {
			t.Errorf("extractStateName(%q) = %q, expected %q", tt.path, result, tt.expected)
		}
	}
}
