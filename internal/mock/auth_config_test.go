package mock_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bethmaloney/mailgun-mock-api/internal/config"
	"github.com/bethmaloney/mailgun-mock-api/internal/mock"
	"github.com/go-chi/chi/v5"
)

func TestGetAuthConfig_Disabled(t *testing.T) {
	db := setupTestDB(t)
	cfg := &config.Config{AuthMode: "disabled"}
	h := mock.NewHandlers(db, cfg)

	r := chi.NewRouter()
	r.Get("/mock/auth-config", h.GetAuthConfig)

	req := httptest.NewRequest(http.MethodGet, "/mock/auth-config", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	t.Run("returns 200 status", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}
	})

	t.Run("returns JSON content type", func(t *testing.T) {
		ct := rec.Header().Get("Content-Type")
		if ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %q", ct)
		}
	})

	t.Run("returns only enabled false", func(t *testing.T) {
		var body map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("failed to decode response body: %v", err)
		}

		enabled, ok := body["enabled"]
		if !ok {
			t.Fatal("response missing 'enabled' key")
		}
		if enabled != false {
			t.Errorf("expected enabled=false, got %v", enabled)
		}

		if len(body) != 1 {
			t.Errorf("expected exactly 1 top-level key, got %d: %v", len(body), body)
		}
	})
}

func TestGetAuthConfig_Enabled(t *testing.T) {
	db := setupTestDB(t)
	cfg := &config.Config{
		AuthMode:         "entra",
		EntraTenantID:    "test-tenant-id",
		EntraClientID:    "test-client-id",
		EntraAPIScope:    "access_as_user",
		EntraRedirectURI: "https://mock.example.com",
	}
	h := mock.NewHandlers(db, cfg)

	r := chi.NewRouter()
	r.Get("/mock/auth-config", h.GetAuthConfig)

	req := httptest.NewRequest(http.MethodGet, "/mock/auth-config", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	t.Run("returns 200 status", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}
	})

	t.Run("returns JSON content type", func(t *testing.T) {
		ct := rec.Header().Get("Content-Type")
		if ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %q", ct)
		}
	})

	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}

	t.Run("enabled is true", func(t *testing.T) {
		enabled, ok := body["enabled"]
		if !ok {
			t.Fatal("response missing 'enabled' key")
		}
		if enabled != true {
			t.Errorf("expected enabled=true, got %v", enabled)
		}
	})

	t.Run("tenantId is correct", func(t *testing.T) {
		tenantID, ok := body["tenantId"]
		if !ok {
			t.Fatal("response missing 'tenantId' key")
		}
		if tenantID != "test-tenant-id" {
			t.Errorf("expected tenantId=%q, got %q", "test-tenant-id", tenantID)
		}
	})

	t.Run("clientId is correct", func(t *testing.T) {
		clientID, ok := body["clientId"]
		if !ok {
			t.Fatal("response missing 'clientId' key")
		}
		if clientID != "test-client-id" {
			t.Errorf("expected clientId=%q, got %q", "test-client-id", clientID)
		}
	})

	t.Run("scopes is correct", func(t *testing.T) {
		rawScopes, ok := body["scopes"]
		if !ok {
			t.Fatal("response missing 'scopes' key")
		}
		scopes, ok := rawScopes.([]interface{})
		if !ok {
			t.Fatalf("expected scopes to be an array, got %T", rawScopes)
		}
		if len(scopes) != 1 {
			t.Fatalf("expected 1 scope, got %d", len(scopes))
		}
		expected := "api://test-client-id/access_as_user"
		if scopes[0] != expected {
			t.Errorf("expected scope %q, got %q", expected, scopes[0])
		}
	})

	t.Run("redirectUri is correct", func(t *testing.T) {
		redirectURI, ok := body["redirectUri"]
		if !ok {
			t.Fatal("response missing 'redirectUri' key")
		}
		if redirectURI != "https://mock.example.com" {
			t.Errorf("expected redirectUri=%q, got %q", "https://mock.example.com", redirectURI)
		}
	})

	t.Run("has exactly 5 top-level keys", func(t *testing.T) {
		if len(body) != 5 {
			t.Errorf("expected 5 top-level keys, got %d: %v", len(body), body)
		}
	})
}
