package apikey_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bethmaloney/mailgun-mock-api/internal/apikey"
	"github.com/go-chi/chi/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// Test Helpers
// ---------------------------------------------------------------------------

// setupTestDB creates an in-memory SQLite database for testing with the
// APIKey table migrated.
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}
	if err := db.AutoMigrate(&apikey.APIKey{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

// setupRouter creates a chi router with API key routes registered.
// API keys are account-level (not domain-scoped), registered under /v1/keys.
func setupRouter(db *gorm.DB) http.Handler {
	kh := apikey.NewHandlers(db)
	r := chi.NewRouter()
	r.Route("/v1/keys", func(r chi.Router) {
		r.Get("/", kh.ListKeys)
		r.Post("/", kh.CreateKey)
		r.Get("/public", kh.GetPublicKey)
		r.Delete("/{id}", kh.DeleteKey)
		r.Post("/{id}/regenerate", kh.RegenerateKey)
	})
	return r
}

// newMultipartRequest creates an HTTP request with multipart/form-data body.
func newMultipartRequest(t *testing.T, method, url string, fields map[string]string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for key, val := range fields {
		if err := writer.WriteField(key, val); err != nil {
			t.Fatalf("failed to write field %q: %v", key, err)
		}
	}
	writer.Close()
	req := httptest.NewRequest(method, url, &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

// decodeJSON unmarshals the response body into the provided destination.
func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder, dest interface{}) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dest); err != nil {
		t.Fatalf("failed to decode response (body=%q): %v", rec.Body.String(), err)
	}
}

// createKey is a convenience helper that creates an API key and returns
// the recorder for inspection.
func createKey(t *testing.T, router http.Handler, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := newMultipartRequest(t, http.MethodPost, "/v1/keys", fields)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// listKeys is a convenience helper that lists API keys with optional query params.
func listKeys(t *testing.T, router http.Handler, queryParams string) *httptest.ResponseRecorder {
	t.Helper()
	url := "/v1/keys"
	if queryParams != "" {
		url += "?" + queryParams
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// deleteKey is a convenience helper that deletes an API key by ID.
func deleteKey(t *testing.T, router http.Handler, id string) *httptest.ResponseRecorder {
	t.Helper()
	url := fmt.Sprintf("/v1/keys/%s", id)
	req := httptest.NewRequest(http.MethodDelete, url, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// regenerateKey is a convenience helper that regenerates an API key's secret.
func regenerateKey(t *testing.T, router http.Handler, id string) *httptest.ResponseRecorder {
	t.Helper()
	url := fmt.Sprintf("/v1/keys/%s/regenerate", id)
	req := httptest.NewRequest(http.MethodPost, url, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// getPublicKey is a convenience helper that fetches the public verification key.
func getPublicKey(t *testing.T, router http.Handler) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/keys/public", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// ---------------------------------------------------------------------------
// Response Structs for Assertions
// ---------------------------------------------------------------------------

type keyJSON struct {
	ID             string  `json:"id"`
	Description    string  `json:"description"`
	Kind           string  `json:"kind"`
	Role           string  `json:"role"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
	ExpiresAt      *string `json:"expires_at"`
	IsDisabled     bool    `json:"is_disabled"`
	DisabledReason *string `json:"disabled_reason"`
	DomainName     *string `json:"domain_name"`
	Requestor      *string `json:"requestor"`
	UserName       *string `json:"user_name"`
	Secret         string  `json:"secret,omitempty"` // Only present on create/regenerate
}

type listKeysResponse struct {
	TotalCount int       `json:"total_count"`
	Items      []keyJSON `json:"items"`
}

type createKeyResponse struct {
	Message string  `json:"message"`
	Key     keyJSON `json:"key"`
}

type messageResponse struct {
	Message string `json:"message"`
}

type regenerateKeyResponse struct {
	Message string  `json:"message"`
	Key     keyJSON `json:"key"`
}

type publicKeyResponse struct {
	Key     string `json:"key"`
	Message string `json:"message"`
}

type errorResponse struct {
	Message string `json:"message"`
}

// ---------------------------------------------------------------------------
// POST /v1/keys -- Create API Key
// ---------------------------------------------------------------------------

func TestCreateKey_HappyPath(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	rec := createKey(t, router, map[string]string{"role": "admin"})

	t.Run("returns 200", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp createKeyResponse
	decodeJSON(t, rec, &resp)

	t.Run("returns success message", func(t *testing.T) {
		if resp.Message != "great success" {
			t.Errorf("expected %q, got %q", "great success", resp.Message)
		}
	})

	t.Run("secret starts with key-", func(t *testing.T) {
		if !strings.HasPrefix(resp.Key.Secret, "key-") {
			t.Errorf("expected secret to start with 'key-', got %q", resp.Key.Secret)
		}
	})

	t.Run("secret is non-empty after prefix", func(t *testing.T) {
		if len(resp.Key.Secret) <= len("key-") {
			t.Errorf("expected secret to have hex chars after 'key-' prefix, got %q", resp.Key.Secret)
		}
	})

	t.Run("id is non-empty", func(t *testing.T) {
		if resp.Key.ID == "" {
			t.Error("expected non-empty id")
		}
	})

	t.Run("role matches request", func(t *testing.T) {
		if resp.Key.Role != "admin" {
			t.Errorf("expected role %q, got %q", "admin", resp.Key.Role)
		}
	})

	t.Run("kind defaults to user", func(t *testing.T) {
		if resp.Key.Kind != "user" {
			t.Errorf("expected default kind %q, got %q", "user", resp.Key.Kind)
		}
	})

	t.Run("is_disabled defaults to false", func(t *testing.T) {
		if resp.Key.IsDisabled != false {
			t.Errorf("expected is_disabled=false, got %v", resp.Key.IsDisabled)
		}
	})

	t.Run("created_at is non-empty", func(t *testing.T) {
		if resp.Key.CreatedAt == "" {
			t.Error("expected non-empty created_at")
		}
	})

	t.Run("updated_at is non-empty", func(t *testing.T) {
		if resp.Key.UpdatedAt == "" {
			t.Error("expected non-empty updated_at")
		}
	})

	t.Run("created_at is ISO 8601 UTC format", func(t *testing.T) {
		if resp.Key.CreatedAt == "" {
			t.Skip("created_at is empty, skipping format check")
		}
		_, err := time.Parse(time.RFC3339, resp.Key.CreatedAt)
		if err != nil {
			t.Errorf("expected created_at in ISO 8601 (RFC3339) format, got %q: %v", resp.Key.CreatedAt, err)
		}
	})
}

func TestCreateKey_WithAllFields(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	domainName := "example.com"
	rec := createKey(t, router, map[string]string{
		"role":        "sending",
		"kind":        "domain",
		"description": "Production sending key",
		"domain_name": domainName,
		"expiration":  "3600",
	})

	t.Run("returns 200", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp createKeyResponse
	decodeJSON(t, rec, &resp)

	t.Run("role matches", func(t *testing.T) {
		if resp.Key.Role != "sending" {
			t.Errorf("expected role %q, got %q", "sending", resp.Key.Role)
		}
	})

	t.Run("kind matches", func(t *testing.T) {
		if resp.Key.Kind != "domain" {
			t.Errorf("expected kind %q, got %q", "domain", resp.Key.Kind)
		}
	})

	t.Run("description matches", func(t *testing.T) {
		if resp.Key.Description != "Production sending key" {
			t.Errorf("expected description %q, got %q", "Production sending key", resp.Key.Description)
		}
	})

	t.Run("domain_name matches", func(t *testing.T) {
		if resp.Key.DomainName == nil || *resp.Key.DomainName != domainName {
			got := "<nil>"
			if resp.Key.DomainName != nil {
				got = *resp.Key.DomainName
			}
			t.Errorf("expected domain_name %q, got %q", domainName, got)
		}
	})

	t.Run("secret is returned", func(t *testing.T) {
		if !strings.HasPrefix(resp.Key.Secret, "key-") {
			t.Errorf("expected secret to start with 'key-', got %q", resp.Key.Secret)
		}
	})
}

func TestCreateKey_MissingRole(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	rec := createKey(t, router, map[string]string{
		"description": "A key without a role",
	})

	t.Run("returns 400", func(t *testing.T) {
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("body contains error message", func(t *testing.T) {
		var resp errorResponse
		decodeJSON(t, rec, &resp)
		if resp.Message == "" {
			t.Error("expected non-empty error message")
		}
	})
}

func TestCreateKey_InvalidRole(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	rec := createKey(t, router, map[string]string{
		"role": "superuser",
	})

	t.Run("returns 400", func(t *testing.T) {
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("body contains error message", func(t *testing.T) {
		var resp errorResponse
		decodeJSON(t, rec, &resp)
		if resp.Message == "" {
			t.Error("expected non-empty error message")
		}
	})
}

func TestCreateKey_DefaultKindIsUser(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	// Create a key without specifying kind
	rec := createKey(t, router, map[string]string{"role": "basic"})

	t.Run("returns 200", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp createKeyResponse
	decodeJSON(t, rec, &resp)

	t.Run("kind defaults to user", func(t *testing.T) {
		if resp.Key.Kind != "user" {
			t.Errorf("expected default kind %q, got %q", "user", resp.Key.Kind)
		}
	})
}

func TestCreateKey_DomainKey(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	rec := createKey(t, router, map[string]string{
		"role":        "sending",
		"kind":        "domain",
		"domain_name": "example.com",
	})

	t.Run("returns 200", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp createKeyResponse
	decodeJSON(t, rec, &resp)

	t.Run("kind is domain", func(t *testing.T) {
		if resp.Key.Kind != "domain" {
			t.Errorf("expected kind %q, got %q", "domain", resp.Key.Kind)
		}
	})

	t.Run("role is sending", func(t *testing.T) {
		if resp.Key.Role != "sending" {
			t.Errorf("expected role %q, got %q", "sending", resp.Key.Role)
		}
	})

	t.Run("domain_name matches", func(t *testing.T) {
		if resp.Key.DomainName == nil || *resp.Key.DomainName != "example.com" {
			got := "<nil>"
			if resp.Key.DomainName != nil {
				got = *resp.Key.DomainName
			}
			t.Errorf("expected domain_name %q, got %q", "example.com", got)
		}
	})

	t.Run("secret starts with key-", func(t *testing.T) {
		if !strings.HasPrefix(resp.Key.Secret, "key-") {
			t.Errorf("expected secret to start with 'key-', got %q", resp.Key.Secret)
		}
	})
}

func TestCreateKey_WithUserName(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	rec := createKey(t, router, map[string]string{
		"role":      "basic",
		"user_name": "alice@example.com",
	})

	t.Run("returns 200", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp createKeyResponse
	decodeJSON(t, rec, &resp)

	t.Run("user_name matches", func(t *testing.T) {
		if resp.Key.UserName == nil || *resp.Key.UserName != "alice@example.com" {
			got := "<nil>"
			if resp.Key.UserName != nil {
				got = *resp.Key.UserName
			}
			t.Errorf("expected user_name %q, got %q", "alice@example.com", got)
		}
	})
}

func TestCreateKey_AllValidRoles(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	validRoles := []string{"admin", "basic", "sending", "developer"}
	for _, role := range validRoles {
		t.Run(fmt.Sprintf("role=%s succeeds", role), func(t *testing.T) {
			rec := createKey(t, router, map[string]string{"role": role})
			if rec.Code != http.StatusOK {
				t.Errorf("expected 200 for role %q, got %d (body: %s)", role, rec.Code, rec.Body.String())
			}
			var resp createKeyResponse
			decodeJSON(t, rec, &resp)
			if resp.Key.Role != role {
				t.Errorf("expected role %q, got %q", role, resp.Key.Role)
			}
		})
	}
}

func TestCreateKey_ZeroExpirationMeansNoExpiry(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	rec := createKey(t, router, map[string]string{
		"role":       "admin",
		"expiration": "0",
	})

	t.Run("returns 200", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp createKeyResponse
	decodeJSON(t, rec, &resp)

	t.Run("expires_at is null for zero expiration", func(t *testing.T) {
		if resp.Key.ExpiresAt != nil {
			t.Errorf("expected expires_at to be nil for expiration=0, got %q", *resp.Key.ExpiresAt)
		}
	})
}

func TestCreateKey_WithExpiration(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	beforeCreate := time.Now().UTC()

	rec := createKey(t, router, map[string]string{
		"role":       "basic",
		"expiration": "7200", // 2 hours
	})

	t.Run("returns 200", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp createKeyResponse
	decodeJSON(t, rec, &resp)

	t.Run("expires_at is set", func(t *testing.T) {
		if resp.Key.ExpiresAt == nil {
			t.Fatal("expected expires_at to be set, got nil")
		}
	})

	t.Run("expires_at is in the future", func(t *testing.T) {
		if resp.Key.ExpiresAt == nil {
			t.Skip("expires_at is nil, skipping")
		}
		expiresAt, err := time.Parse(time.RFC3339, *resp.Key.ExpiresAt)
		if err != nil {
			t.Fatalf("failed to parse expires_at %q: %v", *resp.Key.ExpiresAt, err)
		}
		// Should be at least 2 hours from before the request was made
		expectedMinimum := beforeCreate.Add(7200 * time.Second)
		if expiresAt.Before(expectedMinimum.Add(-5 * time.Second)) {
			t.Errorf("expected expires_at to be at least %v, got %v", expectedMinimum, expiresAt)
		}
	})

	t.Run("expires_at is approximately 2 hours from now", func(t *testing.T) {
		if resp.Key.ExpiresAt == nil {
			t.Skip("expires_at is nil, skipping")
		}
		expiresAt, err := time.Parse(time.RFC3339, *resp.Key.ExpiresAt)
		if err != nil {
			t.Fatalf("failed to parse expires_at %q: %v", *resp.Key.ExpiresAt, err)
		}
		expectedApprox := beforeCreate.Add(7200 * time.Second)
		diff := expiresAt.Sub(expectedApprox)
		if diff < -10*time.Second || diff > 10*time.Second {
			t.Errorf("expected expires_at ~%v, got %v (diff: %v)", expectedApprox, expiresAt, diff)
		}
	})
}

// ---------------------------------------------------------------------------
// GET /v1/keys -- List API Keys
// ---------------------------------------------------------------------------

func TestListKeys_ReturnsAllWithCorrectFormat(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	// Create multiple keys with different roles
	roles := []string{"admin", "basic", "sending"}
	for _, role := range roles {
		rec := createKey(t, router, map[string]string{"role": role})
		if rec.Code != http.StatusOK {
			t.Fatalf("failed to create key with role %q: status=%d body=%s", role, rec.Code, rec.Body.String())
		}
	}

	rec := listKeys(t, router, "")

	t.Run("returns 200", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp listKeysResponse
	decodeJSON(t, rec, &resp)

	t.Run("total_count matches number of keys", func(t *testing.T) {
		if resp.TotalCount != 3 {
			t.Errorf("expected total_count=3, got %d", resp.TotalCount)
		}
	})

	t.Run("items length matches total_count", func(t *testing.T) {
		if len(resp.Items) != 3 {
			t.Errorf("expected 3 items, got %d", len(resp.Items))
		}
	})

	t.Run("secret is NOT returned in list responses", func(t *testing.T) {
		body := rec.Body.String()
		if strings.Contains(body, "key-") {
			t.Errorf("list response should not contain secret (key- prefix found), body: %s", body)
		}
		for i, item := range resp.Items {
			if item.Secret != "" {
				t.Errorf("item[%d] has non-empty secret %q in list response", i, item.Secret)
			}
		}
	})

	t.Run("each item has correct fields", func(t *testing.T) {
		for _, item := range resp.Items {
			if item.ID == "" {
				t.Error("expected non-empty id")
			}
			if item.Role == "" {
				t.Error("expected non-empty role")
			}
			if item.CreatedAt == "" {
				t.Error("expected non-empty created_at")
			}
			if item.UpdatedAt == "" {
				t.Error("expected non-empty updated_at")
			}
		}
	})

	t.Run("timestamps are ISO 8601 UTC format", func(t *testing.T) {
		for _, item := range resp.Items {
			if item.CreatedAt != "" {
				_, err := time.Parse(time.RFC3339, item.CreatedAt)
				if err != nil {
					t.Errorf("expected created_at in ISO 8601 (RFC3339) format, got %q: %v", item.CreatedAt, err)
				}
			}
			if item.UpdatedAt != "" {
				_, err := time.Parse(time.RFC3339, item.UpdatedAt)
				if err != nil {
					t.Errorf("expected updated_at in ISO 8601 (RFC3339) format, got %q: %v", item.UpdatedAt, err)
				}
			}
		}
	})
}

func TestListKeys_FilterByKind(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	// Create keys with different kinds
	kindsAndRoles := []struct {
		kind string
		role string
	}{
		{"user", "admin"},
		{"user", "basic"},
		{"domain", "sending"},
		{"web", "developer"},
	}
	for _, kr := range kindsAndRoles {
		rec := createKey(t, router, map[string]string{
			"role": kr.role,
			"kind": kr.kind,
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("failed to create key kind=%q role=%q: status=%d body=%s", kr.kind, kr.role, rec.Code, rec.Body.String())
		}
	}

	t.Run("filter by kind=user returns only user keys", func(t *testing.T) {
		rec := listKeys(t, router, "kind=user")
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
		var resp listKeysResponse
		decodeJSON(t, rec, &resp)
		if resp.TotalCount != 2 {
			t.Errorf("expected total_count=2 for kind=user, got %d", resp.TotalCount)
		}
		for _, item := range resp.Items {
			if item.Kind != "user" {
				t.Errorf("expected all items to have kind=user, got %q", item.Kind)
			}
		}
	})

	t.Run("filter by kind=domain returns only domain keys", func(t *testing.T) {
		rec := listKeys(t, router, "kind=domain")
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
		var resp listKeysResponse
		decodeJSON(t, rec, &resp)
		if resp.TotalCount != 1 {
			t.Errorf("expected total_count=1 for kind=domain, got %d", resp.TotalCount)
		}
		if len(resp.Items) > 0 && resp.Items[0].Kind != "domain" {
			t.Errorf("expected kind=domain, got %q", resp.Items[0].Kind)
		}
	})

	t.Run("filter by kind=web returns only web keys", func(t *testing.T) {
		rec := listKeys(t, router, "kind=web")
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
		var resp listKeysResponse
		decodeJSON(t, rec, &resp)
		if resp.TotalCount != 1 {
			t.Errorf("expected total_count=1 for kind=web, got %d", resp.TotalCount)
		}
		if len(resp.Items) > 0 && resp.Items[0].Kind != "web" {
			t.Errorf("expected kind=web, got %q", resp.Items[0].Kind)
		}
	})
}

func TestListKeys_FilterByDomainName(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	// Create keys with different domain names
	createKey(t, router, map[string]string{
		"role":        "sending",
		"kind":        "domain",
		"domain_name": "alpha.com",
	})
	createKey(t, router, map[string]string{
		"role":        "sending",
		"kind":        "domain",
		"domain_name": "beta.com",
	})
	createKey(t, router, map[string]string{
		"role":        "sending",
		"kind":        "domain",
		"domain_name": "alpha.com",
	})

	t.Run("filter by domain_name=alpha.com returns matching keys", func(t *testing.T) {
		rec := listKeys(t, router, "domain_name=alpha.com")
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
		var resp listKeysResponse
		decodeJSON(t, rec, &resp)
		if resp.TotalCount != 2 {
			t.Errorf("expected total_count=2 for domain_name=alpha.com, got %d", resp.TotalCount)
		}
		for _, item := range resp.Items {
			if item.DomainName == nil || *item.DomainName != "alpha.com" {
				got := "<nil>"
				if item.DomainName != nil {
					got = *item.DomainName
				}
				t.Errorf("expected domain_name=alpha.com, got %q", got)
			}
		}
	})

	t.Run("filter by domain_name=beta.com returns matching keys", func(t *testing.T) {
		rec := listKeys(t, router, "domain_name=beta.com")
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
		var resp listKeysResponse
		decodeJSON(t, rec, &resp)
		if resp.TotalCount != 1 {
			t.Errorf("expected total_count=1 for domain_name=beta.com, got %d", resp.TotalCount)
		}
	})

	t.Run("filter by nonexistent domain returns empty", func(t *testing.T) {
		rec := listKeys(t, router, "domain_name=nosuch.com")
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
		var resp listKeysResponse
		decodeJSON(t, rec, &resp)
		if resp.TotalCount != 0 {
			t.Errorf("expected total_count=0 for domain_name=nosuch.com, got %d", resp.TotalCount)
		}
	})
}

func TestListKeys_Empty(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	rec := listKeys(t, router, "")

	t.Run("returns 200", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp listKeysResponse
	decodeJSON(t, rec, &resp)

	t.Run("total_count is 0", func(t *testing.T) {
		if resp.TotalCount != 0 {
			t.Errorf("expected total_count=0, got %d", resp.TotalCount)
		}
	})

	t.Run("items is empty array not null", func(t *testing.T) {
		raw := rec.Body.String()
		if !strings.Contains(raw, `"items":[]`) && !strings.Contains(raw, `"items": []`) {
			if strings.Contains(raw, `"items":null`) || strings.Contains(raw, `"items": null`) {
				t.Errorf("expected items to be empty array, not null (body: %s)", raw)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// DELETE /v1/keys/{id} -- Delete API Key
// ---------------------------------------------------------------------------

func TestDeleteKey_HappyPath(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	// Create a key first
	createRec := createKey(t, router, map[string]string{"role": "admin"})
	if createRec.Code != http.StatusOK {
		t.Fatalf("failed to create key: status=%d body=%s", createRec.Code, createRec.Body.String())
	}

	var createResp createKeyResponse
	decodeJSON(t, createRec, &createResp)
	keyID := createResp.Key.ID

	// Delete it
	rec := deleteKey(t, router, keyID)

	t.Run("returns 200", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp messageResponse
	decodeJSON(t, rec, &resp)

	t.Run("returns correct message", func(t *testing.T) {
		if resp.Message != "key deleted" {
			t.Errorf("expected %q, got %q", "key deleted", resp.Message)
		}
	})

	// Verify it is gone from the list
	t.Run("key is no longer listed", func(t *testing.T) {
		listRec := listKeys(t, router, "")
		var listResp listKeysResponse
		decodeJSON(t, listRec, &listResp)
		if listResp.TotalCount != 0 {
			t.Errorf("expected total_count=0 after deletion, got %d", listResp.TotalCount)
		}
		for _, item := range listResp.Items {
			if item.ID == keyID {
				t.Errorf("deleted key %q still appears in list", keyID)
			}
		}
	})
}

func TestDeleteKey_NotFound(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	rec := deleteKey(t, router, "00000000-0000-0000-0000-000000000000")

	t.Run("returns 404", func(t *testing.T) {
		if rec.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})
}

// ---------------------------------------------------------------------------
// POST /v1/keys/{id}/regenerate -- Regenerate Key Secret
// ---------------------------------------------------------------------------

func TestRegenerateKey_HappyPath(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	// Create a key first
	createRec := createKey(t, router, map[string]string{"role": "admin"})
	if createRec.Code != http.StatusOK {
		t.Fatalf("failed to create key: status=%d body=%s", createRec.Code, createRec.Body.String())
	}

	var createResp createKeyResponse
	decodeJSON(t, createRec, &createResp)
	keyID := createResp.Key.ID
	originalSecret := createResp.Key.Secret

	// Regenerate the key
	rec := regenerateKey(t, router, keyID)

	t.Run("returns 200", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp regenerateKeyResponse
	decodeJSON(t, rec, &resp)

	t.Run("returns correct message", func(t *testing.T) {
		if resp.Message != "key regenerated" {
			t.Errorf("expected %q, got %q", "key regenerated", resp.Message)
		}
	})

	t.Run("returns full key object", func(t *testing.T) {
		if resp.Key.ID != keyID {
			t.Errorf("expected id %q, got %q", keyID, resp.Key.ID)
		}
		if resp.Key.Role != "admin" {
			t.Errorf("expected role %q, got %q", "admin", resp.Key.Role)
		}
	})

	t.Run("new secret starts with key-", func(t *testing.T) {
		if !strings.HasPrefix(resp.Key.Secret, "key-") {
			t.Errorf("expected new secret to start with 'key-', got %q", resp.Key.Secret)
		}
	})

	t.Run("new secret differs from original", func(t *testing.T) {
		if resp.Key.Secret == originalSecret {
			t.Errorf("expected new secret to differ from original, but both are %q", originalSecret)
		}
	})
}

func TestRegenerateKey_NotFound(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	rec := regenerateKey(t, router, "00000000-0000-0000-0000-000000000000")

	t.Run("returns 404", func(t *testing.T) {
		if rec.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})
}

// ---------------------------------------------------------------------------
// GET /v1/keys/public -- Get Public Verification Key
// ---------------------------------------------------------------------------

func TestGetPublicKey(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	rec := getPublicKey(t, router)

	t.Run("returns 200", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp publicKeyResponse
	decodeJSON(t, rec, &resp)

	t.Run("returns message", func(t *testing.T) {
		if resp.Message != "public key" {
			t.Errorf("expected message %q, got %q", "public key", resp.Message)
		}
	})

	t.Run("key starts with pubkey-", func(t *testing.T) {
		if !strings.HasPrefix(resp.Key, "pubkey-") {
			t.Errorf("expected key to start with 'pubkey-', got %q", resp.Key)
		}
	})

	t.Run("key is non-empty after prefix", func(t *testing.T) {
		if len(resp.Key) <= len("pubkey-") {
			t.Errorf("expected key to have content after 'pubkey-' prefix, got %q", resp.Key)
		}
	})
}

// ---------------------------------------------------------------------------
// Full Lifecycle Test
// ---------------------------------------------------------------------------

func TestKeyLifecycle(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	// Step 1: Create a key
	var keyID string
	var originalSecret string
	t.Run("create key", func(t *testing.T) {
		rec := createKey(t, router, map[string]string{
			"role":        "sending",
			"kind":        "domain",
			"description": "Lifecycle test key",
			"domain_name": "lifecycle.com",
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("create failed: status=%d body=%s", rec.Code, rec.Body.String())
		}
		var resp createKeyResponse
		decodeJSON(t, rec, &resp)
		if resp.Message != "great success" {
			t.Errorf("expected %q, got %q", "great success", resp.Message)
		}
		if !strings.HasPrefix(resp.Key.Secret, "key-") {
			t.Errorf("expected secret to start with 'key-', got %q", resp.Key.Secret)
		}
		keyID = resp.Key.ID
		originalSecret = resp.Key.Secret
	})

	// Step 2: List keys and verify the created one appears
	t.Run("list shows created key", func(t *testing.T) {
		rec := listKeys(t, router, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("list failed: status=%d body=%s", rec.Code, rec.Body.String())
		}
		var resp listKeysResponse
		decodeJSON(t, rec, &resp)
		if resp.TotalCount != 1 {
			t.Fatalf("expected total_count=1, got %d", resp.TotalCount)
		}
		item := resp.Items[0]
		if item.ID != keyID {
			t.Errorf("expected id %q, got %q", keyID, item.ID)
		}
		if item.Description != "Lifecycle test key" {
			t.Errorf("expected description %q, got %q", "Lifecycle test key", item.Description)
		}
		if item.Kind != "domain" {
			t.Errorf("expected kind %q, got %q", "domain", item.Kind)
		}
		if item.Role != "sending" {
			t.Errorf("expected role %q, got %q", "sending", item.Role)
		}
	})

	// Step 3: Verify secret is NOT in list response
	t.Run("list does not expose secret", func(t *testing.T) {
		rec := listKeys(t, router, "")
		body := rec.Body.String()
		if strings.Contains(body, originalSecret) {
			t.Errorf("list response should not contain the secret, got: %s", body)
		}
		if strings.Contains(body, "key-") {
			t.Errorf("list response should not contain any 'key-' prefixed secret, got: %s", body)
		}
	})

	// Step 4: Regenerate the key's secret
	var newSecret string
	t.Run("regenerate key", func(t *testing.T) {
		rec := regenerateKey(t, router, keyID)
		if rec.Code != http.StatusOK {
			t.Fatalf("regenerate failed: status=%d body=%s", rec.Code, rec.Body.String())
		}
		var resp regenerateKeyResponse
		decodeJSON(t, rec, &resp)
		if resp.Message != "key regenerated" {
			t.Errorf("expected %q, got %q", "key regenerated", resp.Message)
		}
		if resp.Key.Secret == originalSecret {
			t.Error("expected new secret to differ from original")
		}
		if !strings.HasPrefix(resp.Key.Secret, "key-") {
			t.Errorf("expected new secret to start with 'key-', got %q", resp.Key.Secret)
		}
		newSecret = resp.Key.Secret
		_ = newSecret // used for verification
	})

	// Step 5: List again -- verify key is still there with same metadata
	t.Run("list after regenerate still shows key", func(t *testing.T) {
		rec := listKeys(t, router, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("list failed: status=%d body=%s", rec.Code, rec.Body.String())
		}
		var resp listKeysResponse
		decodeJSON(t, rec, &resp)
		if resp.TotalCount != 1 {
			t.Fatalf("expected total_count=1, got %d", resp.TotalCount)
		}
		if resp.Items[0].ID != keyID {
			t.Errorf("expected id %q, got %q", keyID, resp.Items[0].ID)
		}
		// Secret should still not be in list response
		body := rec.Body.String()
		if strings.Contains(body, "key-") {
			t.Errorf("list response after regenerate should not contain secret, got: %s", body)
		}
	})

	// Step 6: Create a second key
	var secondKeyID string
	t.Run("create second key", func(t *testing.T) {
		rec := createKey(t, router, map[string]string{
			"role":        "admin",
			"description": "Second lifecycle key",
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("create failed: status=%d body=%s", rec.Code, rec.Body.String())
		}
		var resp createKeyResponse
		decodeJSON(t, rec, &resp)
		secondKeyID = resp.Key.ID
	})

	// Step 7: List should now have 2
	t.Run("list shows 2 keys", func(t *testing.T) {
		rec := listKeys(t, router, "")
		var resp listKeysResponse
		decodeJSON(t, rec, &resp)
		if resp.TotalCount != 2 {
			t.Errorf("expected total_count=2, got %d", resp.TotalCount)
		}
	})

	// Step 8: Delete the first key
	t.Run("delete first key", func(t *testing.T) {
		rec := deleteKey(t, router, keyID)
		if rec.Code != http.StatusOK {
			t.Fatalf("delete failed: status=%d body=%s", rec.Code, rec.Body.String())
		}
		var resp messageResponse
		decodeJSON(t, rec, &resp)
		if resp.Message != "key deleted" {
			t.Errorf("expected %q, got %q", "key deleted", resp.Message)
		}
	})

	// Step 9: Verify first key is gone but second remains
	t.Run("first key is gone but second remains", func(t *testing.T) {
		rec := listKeys(t, router, "")
		var resp listKeysResponse
		decodeJSON(t, rec, &resp)
		if resp.TotalCount != 1 {
			t.Fatalf("expected total_count=1, got %d", resp.TotalCount)
		}
		if resp.Items[0].ID != secondKeyID {
			t.Errorf("expected remaining key id %q, got %q", secondKeyID, resp.Items[0].ID)
		}
	})

	// Step 10: Delete the second key
	t.Run("delete second key", func(t *testing.T) {
		rec := deleteKey(t, router, secondKeyID)
		if rec.Code != http.StatusOK {
			t.Fatalf("delete failed: status=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	// Step 11: Verify list is empty
	t.Run("list is empty after all deletions", func(t *testing.T) {
		rec := listKeys(t, router, "")
		var resp listKeysResponse
		decodeJSON(t, rec, &resp)
		if resp.TotalCount != 0 {
			t.Errorf("expected total_count=0, got %d", resp.TotalCount)
		}
	})
}
