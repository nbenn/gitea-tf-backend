package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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
	// Use path as fake SHA for simplicity
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

// TestStateHandler wraps StateHandler for testing with mock client
type TestStateHandler struct {
	client GiteaFileClient
}

func NewTestStateHandler(client GiteaFileClient) *TestStateHandler {
	return &TestStateHandler{client: client}
}

func (h *TestStateHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

func (h *TestStateHandler) handleGet(w http.ResponseWriter, r *http.Request, name string) {
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
}

func (h *TestStateHandler) handlePost(w http.ResponseWriter, r *http.Request, name string) {
	lockContent, _, err := h.client.GetFile(lockPath(name))
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if lockContent != nil {
		lockID := r.Header.Get("Lock-Id")
		var existingLock LockInfo
		if err := json.Unmarshal(lockContent, &existingLock); err != nil {
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

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	err = h.client.CreateOrUpdateFile(statePath(name), body, "Update state: "+name)
	if err != nil {
		http.Error(w, "failed to save state", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *TestStateHandler) handleLock(w http.ResponseWriter, r *http.Request, name string) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	var lockInfo LockInfo
	if err := json.Unmarshal(body, &lockInfo); err != nil {
		http.Error(w, "invalid lock info", http.StatusBadRequest)
		return
	}

	existingContent, _, err := h.client.GetFile(lockPath(name))
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if existingContent != nil {
		var existingLock LockInfo
		if err := json.Unmarshal(existingContent, &existingLock); err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
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

	lockJSON, _ := json.Marshal(lockInfo)
	err = h.client.CreateFile(lockPath(name), lockJSON, "Lock state: "+name)
	if err != nil {
		http.Error(w, "failed to create lock", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(lockInfo)
}

func (h *TestStateHandler) handleUnlock(w http.ResponseWriter, r *http.Request, name string) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	var unlockInfo LockInfo
	if err := json.Unmarshal(body, &unlockInfo); err != nil {
		http.Error(w, "invalid lock info", http.StatusBadRequest)
		return
	}

	existingContent, sha, err := h.client.GetFile(lockPath(name))
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if existingContent == nil {
		w.WriteHeader(http.StatusOK)
		return
	}

	var existingLock LockInfo
	if err := json.Unmarshal(existingContent, &existingLock); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if unlockInfo.ID != "" && unlockInfo.ID != existingLock.ID {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(existingLock)
		return
	}

	err = h.client.DeleteFile(lockPath(name), sha, "Unlock state: "+name)
	if err != nil {
		http.Error(w, "failed to delete lock", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// Tests

func TestGetState_NotFound(t *testing.T) {
	mock := NewMockGiteaClient()
	handler := NewTestStateHandler(mock)

	req := httptest.NewRequest(http.MethodGet, "/myproject", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestGetState_Found(t *testing.T) {
	mock := NewMockGiteaClient()
	handler := NewTestStateHandler(mock)

	// Pre-populate state
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
	handler := NewTestStateHandler(mock)

	stateData := []byte(`{"version":4,"terraform_version":"1.0.0"}`)
	req := httptest.NewRequest(http.MethodPost, "/myproject", bytes.NewReader(stateData))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// Verify state was saved
	saved := mock.files["states/myproject/terraform.tfstate"]
	if !bytes.Equal(saved, stateData) {
		t.Errorf("state not saved correctly")
	}
}

func TestPostState_WithMatchingLock(t *testing.T) {
	mock := NewMockGiteaClient()
	handler := NewTestStateHandler(mock)

	// Create a lock
	lockInfo := LockInfo{ID: "lock-123", Operation: "apply"}
	lockJSON, _ := json.Marshal(lockInfo)
	mock.files["states/myproject/.lock"] = lockJSON

	stateData := []byte(`{"version":4}`)
	req := httptest.NewRequest(http.MethodPost, "/myproject", bytes.NewReader(stateData))
	req.Header.Set("Lock-Id", "lock-123")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestPostState_WithWrongLock(t *testing.T) {
	mock := NewMockGiteaClient()
	handler := NewTestStateHandler(mock)

	// Create a lock
	lockInfo := LockInfo{ID: "lock-123", Operation: "apply"}
	lockJSON, _ := json.Marshal(lockInfo)
	mock.files["states/myproject/.lock"] = lockJSON

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
	mock := NewMockGiteaClient()
	handler := NewTestStateHandler(mock)

	lockInfo := LockInfo{ID: "lock-123", Operation: "apply", Who: "user@host"}
	lockJSON, _ := json.Marshal(lockInfo)

	req := httptest.NewRequest("LOCK", "/myproject", bytes.NewReader(lockJSON))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// Verify lock was created
	if _, exists := mock.files["states/myproject/.lock"]; !exists {
		t.Error("lock file was not created")
	}
}

func TestLock_AlreadyLocked(t *testing.T) {
	mock := NewMockGiteaClient()
	handler := NewTestStateHandler(mock)

	// Create existing lock
	existingLock := LockInfo{ID: "existing-lock", Operation: "apply"}
	existingJSON, _ := json.Marshal(existingLock)
	mock.files["states/myproject/.lock"] = existingJSON

	// Try to acquire new lock
	newLock := LockInfo{ID: "new-lock", Operation: "apply"}
	newJSON, _ := json.Marshal(newLock)

	req := httptest.NewRequest("LOCK", "/myproject", bytes.NewReader(newJSON))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusLocked {
		t.Errorf("expected status 423, got %d", w.Code)
	}

	// Verify response contains existing lock info
	var returnedLock LockInfo
	json.NewDecoder(w.Body).Decode(&returnedLock)
	if returnedLock.ID != "existing-lock" {
		t.Errorf("expected existing lock ID, got %s", returnedLock.ID)
	}
}

func TestLock_Idempotent(t *testing.T) {
	mock := NewMockGiteaClient()
	handler := NewTestStateHandler(mock)

	// Create existing lock with same ID
	lockInfo := LockInfo{ID: "lock-123", Operation: "apply"}
	lockJSON, _ := json.Marshal(lockInfo)
	mock.files["states/myproject/.lock"] = lockJSON

	// Try to acquire same lock again
	req := httptest.NewRequest("LOCK", "/myproject", bytes.NewReader(lockJSON))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Should succeed (idempotent)
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200 for idempotent lock, got %d", w.Code)
	}
}

func TestUnlock_Success(t *testing.T) {
	mock := NewMockGiteaClient()
	handler := NewTestStateHandler(mock)

	// Create existing lock
	lockInfo := LockInfo{ID: "lock-123", Operation: "apply"}
	lockJSON, _ := json.Marshal(lockInfo)
	mock.files["states/myproject/.lock"] = lockJSON

	req := httptest.NewRequest("UNLOCK", "/myproject", bytes.NewReader(lockJSON))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// Verify lock was deleted
	if _, exists := mock.files["states/myproject/.lock"]; exists {
		t.Error("lock file was not deleted")
	}
}

func TestUnlock_WrongID(t *testing.T) {
	mock := NewMockGiteaClient()
	handler := NewTestStateHandler(mock)

	// Create existing lock
	existingLock := LockInfo{ID: "lock-123", Operation: "apply"}
	existingJSON, _ := json.Marshal(existingLock)
	mock.files["states/myproject/.lock"] = existingJSON

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
	if _, exists := mock.files["states/myproject/.lock"]; !exists {
		t.Error("lock file should not be deleted")
	}
}

func TestUnlock_NoLock(t *testing.T) {
	mock := NewMockGiteaClient()
	handler := NewTestStateHandler(mock)

	unlockInfo := LockInfo{ID: "any-id"}
	unlockJSON, _ := json.Marshal(unlockInfo)

	req := httptest.NewRequest("UNLOCK", "/myproject", bytes.NewReader(unlockJSON))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Should succeed (idempotent)
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200 for unlock with no lock, got %d", w.Code)
	}
}

func TestUnlock_ForceUnlock(t *testing.T) {
	mock := NewMockGiteaClient()
	handler := NewTestStateHandler(mock)

	// Create existing lock
	existingLock := LockInfo{ID: "lock-123", Operation: "apply"}
	existingJSON, _ := json.Marshal(existingLock)
	mock.files["states/myproject/.lock"] = existingJSON

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
	if _, exists := mock.files["states/myproject/.lock"]; exists {
		t.Error("lock file should be deleted on force unlock")
	}
}

func TestStatePath(t *testing.T) {
	path := statePath("myproject")
	expected := "states/myproject/terraform.tfstate"
	if path != expected {
		t.Errorf("expected %s, got %s", expected, path)
	}
}

func TestLockPath(t *testing.T) {
	path := lockPath("myproject")
	expected := "states/myproject/.lock"
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
