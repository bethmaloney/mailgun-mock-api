package apikey

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// Test Helpers
// ---------------------------------------------------------------------------

func setupManagedTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}
	if err := db.AutoMigrate(&ManagedAPIKey{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

// managedKeyResponse is used to unmarshal JSON responses for a single managed key.
type managedKeyResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	KeyValue  string `json:"key_value"`
	Prefix    string `json:"prefix"`
	CreatedAt string `json:"created_at"`
}

// ---------------------------------------------------------------------------
// Handler Tests
// ---------------------------------------------------------------------------

func TestManagedList_EmptyReturnsEmptyArray(t *testing.T) {
	db := setupManagedTestDB(t)
	h := NewManagedHandlers(db)

	req := httptest.NewRequest(http.MethodGet, "/mock/api-keys", nil)
	rec := httptest.NewRecorder()

	h.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var result []managedKeyResponse
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if result == nil || len(result) != 0 {
		t.Fatalf("expected empty array, got %v", result)
	}
}

func TestManagedCreate_ReturnsKeyWithMockPrefix(t *testing.T) {
	db := setupManagedTestDB(t)
	h := NewManagedHandlers(db)

	body := `{"name":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/mock/api-keys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var result managedKeyResponse
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result.Name != "test" {
		t.Errorf("expected name %q, got %q", "test", result.Name)
	}
	if !strings.HasPrefix(result.KeyValue, "mock_") {
		t.Errorf("expected key_value to start with 'mock_', got %q", result.KeyValue)
	}
	if len(result.Prefix) != 13 {
		t.Errorf("expected prefix length 13, got %d (%q)", len(result.Prefix), result.Prefix)
	}
	if result.ID == "" {
		t.Error("expected non-empty id")
	}
}

func TestManagedCreate_EmptyNameReturns400(t *testing.T) {
	db := setupManagedTestDB(t)
	h := NewManagedHandlers(db)

	body := `{"name":""}`
	req := httptest.NewRequest(http.MethodPost, "/mock/api-keys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestManagedList_AfterCreateReturnsOneItem(t *testing.T) {
	db := setupManagedTestDB(t)
	h := NewManagedHandlers(db)

	// Create a key first.
	body := `{"name":"my-key"}`
	createReq := httptest.NewRequest(http.MethodPost, "/mock/api-keys", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	h.Create(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("create: expected status 201, got %d", createRec.Code)
	}

	var created managedKeyResponse
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}

	// List keys.
	listReq := httptest.NewRequest(http.MethodGet, "/mock/api-keys", nil)
	listRec := httptest.NewRecorder()
	h.List(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("list: expected status 200, got %d", listRec.Code)
	}

	var items []managedKeyResponse
	if err := json.NewDecoder(listRec.Body).Decode(&items); err != nil {
		t.Fatalf("failed to decode list response: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Name != "my-key" {
		t.Errorf("expected name %q, got %q", "my-key", items[0].Name)
	}
	if items[0].ID != created.ID {
		t.Errorf("expected id %q, got %q", created.ID, items[0].ID)
	}
}

func TestManagedDelete_Returns204(t *testing.T) {
	db := setupManagedTestDB(t)
	h := NewManagedHandlers(db)

	// Create a key.
	body := `{"name":"to-delete"}`
	createReq := httptest.NewRequest(http.MethodPost, "/mock/api-keys", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	h.Create(createRec, createReq)

	var created managedKeyResponse
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}

	// Delete by ID using chi URL param.
	deleteReq := httptest.NewRequest(http.MethodDelete, "/mock/api-keys/"+created.ID, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", created.ID)
	deleteReq = deleteReq.WithContext(context.WithValue(deleteReq.Context(), chi.RouteCtxKey, rctx))
	deleteRec := httptest.NewRecorder()

	h.Delete(deleteRec, deleteReq)

	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d; body: %s", deleteRec.Code, deleteRec.Body.String())
	}
}

func TestManagedList_AfterDeleteReturnsEmptyArray(t *testing.T) {
	db := setupManagedTestDB(t)
	h := NewManagedHandlers(db)

	// Create a key.
	body := `{"name":"ephemeral"}`
	createReq := httptest.NewRequest(http.MethodPost, "/mock/api-keys", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	h.Create(createRec, createReq)

	var created managedKeyResponse
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}

	// Delete.
	deleteReq := httptest.NewRequest(http.MethodDelete, "/mock/api-keys/"+created.ID, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", created.ID)
	deleteReq = deleteReq.WithContext(context.WithValue(deleteReq.Context(), chi.RouteCtxKey, rctx))
	deleteRec := httptest.NewRecorder()
	h.Delete(deleteRec, deleteReq)

	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("delete: expected status 204, got %d", deleteRec.Code)
	}

	// List should now be empty.
	listReq := httptest.NewRequest(http.MethodGet, "/mock/api-keys", nil)
	listRec := httptest.NewRecorder()
	h.List(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("list: expected status 200, got %d", listRec.Code)
	}

	var items []managedKeyResponse
	if err := json.NewDecoder(listRec.Body).Decode(&items); err != nil {
		t.Fatalf("failed to decode list response: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected empty array, got %d items", len(items))
	}
}

// ---------------------------------------------------------------------------
// Key Generator Tests
// ---------------------------------------------------------------------------

func TestGenerateManagedKeyValue_Format(t *testing.T) {
	value, prefix, err := generateManagedKeyValue()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Value must start with "mock_".
	if !strings.HasPrefix(value, "mock_") {
		t.Errorf("expected value to start with 'mock_', got %q", value)
	}

	// Prefix must be first 13 chars of value.
	if len(prefix) != 13 {
		t.Errorf("expected prefix length 13, got %d (%q)", len(prefix), prefix)
	}
	if len(value) >= 13 && prefix != value[:13] {
		t.Errorf("expected prefix %q to equal first 13 chars of value %q", prefix, value[:13])
	}
}

func TestGenerateManagedKeyValue_Uniqueness(t *testing.T) {
	// Generate multiple keys and verify they are unique.
	seen := make(map[string]bool)
	for i := 0; i < 50; i++ {
		value, _, err := generateManagedKeyValue()
		if err != nil {
			t.Fatalf("unexpected error on iteration %d: %v", i, err)
		}
		if seen[value] {
			t.Fatalf("duplicate key value on iteration %d: %q", i, value)
		}
		seen[value] = true
	}
}
