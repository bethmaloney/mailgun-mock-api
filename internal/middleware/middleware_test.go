package middleware_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bethmaloney/mailgun-mock-api/internal/database"
	"github.com/bethmaloney/mailgun-mock-api/internal/middleware"
	"github.com/go-chi/chi/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupTestDB creates an in-memory SQLite database for testing.
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}
	if err := db.AutoMigrate(&database.Domain{}); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}
	return db
}

// basicAuthHeader builds an HTTP Basic Auth header value from username and password.
func basicAuthHeader(username, password string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password))
}

// okHandler is a simple handler that returns 200 with a JSON body. It is used
// as the "next" handler when testing middleware.
func okHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// newAuthConfig returns an AuthConfig with the given mode and signing key.
// The returned closures capture the pointer values so mutations are visible
// immediately (matching real runtime behaviour where the config can change
// between requests).
func newAuthConfig(mode, signingKey *string) *middleware.AuthConfig {
	return &middleware.AuthConfig{
		GetAuthMode:   func() string { return *mode },
		GetSigningKey: func() string { return *signingKey },
	}
}

// parseErrorBody is a small helper that decodes the JSON error response
// returned by the middleware on rejection.
func parseErrorBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode error response body: %v", err)
	}
	return body
}

// ---------------------------------------------------------------------------
// BasicAuth middleware tests — accept_any mode
// ---------------------------------------------------------------------------

func TestBasicAuth_AcceptAny_ValidAuth(t *testing.T) {
	mode := "accept_any"
	key := ""
	cfg := newAuthConfig(&mode, &key)
	handler := middleware.BasicAuth(cfg)(http.HandlerFunc(okHandler))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", basicAuthHeader("api", "key-abc123"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

func TestBasicAuth_AcceptAny_MissingAuth(t *testing.T) {
	mode := "accept_any"
	key := ""
	cfg := newAuthConfig(&mode, &key)
	handler := middleware.BasicAuth(cfg)(http.HandlerFunc(okHandler))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	// No Authorization header set.
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	t.Run("returns 401 status", func(t *testing.T) {
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401, got %d", rec.Code)
		}
	})

	t.Run("returns Forbidden message", func(t *testing.T) {
		body := parseErrorBody(t, rec)
		if body["message"] != "Forbidden" {
			t.Errorf("expected message %q, got %q", "Forbidden", body["message"])
		}
	})
}

func TestBasicAuth_AcceptAny_WrongUsername(t *testing.T) {
	mode := "accept_any"
	key := ""
	cfg := newAuthConfig(&mode, &key)
	handler := middleware.BasicAuth(cfg)(http.HandlerFunc(okHandler))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", basicAuthHeader("notapi", "key-abc123"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rec.Code)
	}

	body := parseErrorBody(t, rec)
	if body["message"] != "Forbidden" {
		t.Errorf("expected message %q, got %q", "Forbidden", body["message"])
	}
}

func TestBasicAuth_AcceptAny_EmptyPassword(t *testing.T) {
	mode := "accept_any"
	key := ""
	cfg := newAuthConfig(&mode, &key)
	handler := middleware.BasicAuth(cfg)(http.HandlerFunc(okHandler))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", basicAuthHeader("api", ""))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rec.Code)
	}

	body := parseErrorBody(t, rec)
	if body["message"] != "Forbidden" {
		t.Errorf("expected message %q, got %q", "Forbidden", body["message"])
	}
}

func TestBasicAuth_AcceptAny_NonBasicScheme(t *testing.T) {
	mode := "accept_any"
	key := ""
	cfg := newAuthConfig(&mode, &key)
	handler := middleware.BasicAuth(cfg)(http.HandlerFunc(okHandler))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer some-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rec.Code)
	}

	body := parseErrorBody(t, rec)
	if body["message"] != "Forbidden" {
		t.Errorf("expected message %q, got %q", "Forbidden", body["message"])
	}
}

// ---------------------------------------------------------------------------
// BasicAuth middleware tests — none mode
// ---------------------------------------------------------------------------

func TestBasicAuth_None_NoHeader(t *testing.T) {
	mode := "none"
	key := ""
	cfg := newAuthConfig(&mode, &key)
	handler := middleware.BasicAuth(cfg)(http.HandlerFunc(okHandler))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	// No Authorization header — should still be allowed.
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 (none mode), got %d", rec.Code)
	}
}

func TestBasicAuth_None_WithHeader(t *testing.T) {
	mode := "none"
	key := ""
	cfg := newAuthConfig(&mode, &key)
	handler := middleware.BasicAuth(cfg)(http.HandlerFunc(okHandler))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", basicAuthHeader("api", "key-abc123"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 (none mode with header), got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// BasicAuth middleware tests — strict mode
// ---------------------------------------------------------------------------

func TestBasicAuth_Strict_CorrectKey(t *testing.T) {
	mode := "strict"
	signingKey := "key-master-secret-000000000000"
	cfg := newAuthConfig(&mode, &signingKey)
	handler := middleware.BasicAuth(cfg)(http.HandlerFunc(okHandler))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", basicAuthHeader("api", "key-master-secret-000000000000"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

func TestBasicAuth_Strict_WrongKey(t *testing.T) {
	mode := "strict"
	signingKey := "key-master-secret-000000000000"
	cfg := newAuthConfig(&mode, &signingKey)
	handler := middleware.BasicAuth(cfg)(http.HandlerFunc(okHandler))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", basicAuthHeader("api", "key-wrong-key"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rec.Code)
	}

	body := parseErrorBody(t, rec)
	if body["message"] != "Forbidden" {
		t.Errorf("expected message %q, got %q", "Forbidden", body["message"])
	}
}

func TestBasicAuth_Strict_MissingAuth(t *testing.T) {
	mode := "strict"
	signingKey := "key-master-secret-000000000000"
	cfg := newAuthConfig(&mode, &signingKey)
	handler := middleware.BasicAuth(cfg)(http.HandlerFunc(okHandler))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	// No Authorization header.
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rec.Code)
	}

	body := parseErrorBody(t, rec)
	if body["message"] != "Forbidden" {
		t.Errorf("expected message %q, got %q", "Forbidden", body["message"])
	}
}

// ---------------------------------------------------------------------------
// BasicAuth — context propagation & dynamic config
// ---------------------------------------------------------------------------

func TestBasicAuth_APIKeyInContext(t *testing.T) {
	mode := "accept_any"
	key := ""
	cfg := newAuthConfig(&mode, &key)

	const expectedKey = "key-my-test-key-12345"

	// Use a custom handler that reads the API key from context.
	var capturedKey string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedKey = middleware.GetAPIKey(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	handler := middleware.BasicAuth(cfg)(inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", basicAuthHeader("api", expectedKey))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	if capturedKey != expectedKey {
		t.Errorf("expected API key %q in context, got %q", expectedKey, capturedKey)
	}
}

func TestBasicAuth_ConfigChangesTakeEffect(t *testing.T) {
	mode := "accept_any"
	signingKey := "key-master-secret-000000000000"
	cfg := newAuthConfig(&mode, &signingKey)
	handler := middleware.BasicAuth(cfg)(http.HandlerFunc(okHandler))

	t.Run("accept_any allows any non-empty key", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", basicAuthHeader("api", "key-some-random-key"))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200 in accept_any mode, got %d", rec.Code)
		}
	})

	// Dynamically switch to strict mode between requests.
	mode = "strict"

	t.Run("strict rejects wrong key after config change", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", basicAuthHeader("api", "key-some-random-key"))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401 in strict mode after config change, got %d", rec.Code)
		}
	})

	t.Run("strict accepts correct key after config change", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", basicAuthHeader("api", "key-master-secret-000000000000"))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200 in strict mode with correct key, got %d", rec.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// RequireDomain middleware tests
// ---------------------------------------------------------------------------

func TestRequireDomain_DomainExists(t *testing.T) {
	db := setupTestDB(t)

	// Seed a domain into the database.
	domain := database.Domain{Name: "example.com"}
	if err := db.Create(&domain).Error; err != nil {
		t.Fatalf("failed to seed domain: %v", err)
	}

	// Build a chi-routed request so that URLParam("domain") is populated.
	r := chi.NewRouter()
	r.Route("/v3/{domain}", func(r chi.Router) {
		r.Use(middleware.RequireDomain(db))
		r.Get("/*", okHandler)
	})

	req := httptest.NewRequest(http.MethodGet, "/v3/example.com/messages", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestRequireDomain_DomainNotFound(t *testing.T) {
	db := setupTestDB(t)
	// Do NOT seed any domain — the lookup must fail.

	r := chi.NewRouter()
	r.Route("/v3/{domain}", func(r chi.Router) {
		r.Use(middleware.RequireDomain(db))
		r.Get("/*", okHandler)
	})

	req := httptest.NewRequest(http.MethodGet, "/v3/nonexistent.com/messages", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	t.Run("returns 404 status", func(t *testing.T) {
		if rec.Code != http.StatusNotFound {
			t.Errorf("expected status 404, got %d", rec.Code)
		}
	})

	t.Run("returns Domain not found message", func(t *testing.T) {
		body := parseErrorBody(t, rec)
		if body["message"] != "Domain not found" {
			t.Errorf("expected message %q, got %q", "Domain not found", body["message"])
		}
	})
}

func TestRequireDomain_DomainNameInContext(t *testing.T) {
	db := setupTestDB(t)

	// Seed a domain.
	domain := database.Domain{Name: "mg.example.org"}
	if err := db.Create(&domain).Error; err != nil {
		t.Fatalf("failed to seed domain: %v", err)
	}

	var capturedDomain string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedDomain = middleware.GetDomainName(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	r := chi.NewRouter()
	r.Route("/v3/{domain}", func(r chi.Router) {
		r.Use(middleware.RequireDomain(db))
		r.Get("/*", inner)
	})

	req := httptest.NewRequest(http.MethodGet, "/v3/mg.example.org/messages", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	if capturedDomain != "mg.example.org" {
		t.Errorf("expected domain name %q in context, got %q", "mg.example.org", capturedDomain)
	}
}

func TestRequireDomain_DomainNameParam(t *testing.T) {
	db := setupTestDB(t)

	domain := database.Domain{Name: "mail.example.org"}
	if err := db.Create(&domain).Error; err != nil {
		t.Fatalf("failed to seed domain: %v", err)
	}

	var capturedDomain string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedDomain = middleware.GetDomainName(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	r := chi.NewRouter()
	r.Route("/v3/domains/{domain_name}", func(r chi.Router) {
		r.Use(middleware.RequireDomain(db))
		r.Get("/*", inner)
	})

	req := httptest.NewRequest(http.MethodGet, "/v3/domains/mail.example.org/tracking", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	if capturedDomain != "mail.example.org" {
		t.Errorf("expected domain name %q in context, got %q", "mail.example.org", capturedDomain)
	}
}

func TestRequireDomain_NameParam(t *testing.T) {
	db := setupTestDB(t)

	domain := database.Domain{Name: "test.mailgun.org"}
	if err := db.Create(&domain).Error; err != nil {
		t.Fatalf("failed to seed domain: %v", err)
	}

	var capturedDomain string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedDomain = middleware.GetDomainName(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	r := chi.NewRouter()
	r.Route("/v4/domains/{name}", func(r chi.Router) {
		r.Use(middleware.RequireDomain(db))
		r.Get("/*", inner)
	})

	req := httptest.NewRequest(http.MethodGet, "/v4/domains/test.mailgun.org/verify", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	if capturedDomain != "test.mailgun.org" {
		t.Errorf("expected domain name %q in context, got %q", "test.mailgun.org", capturedDomain)
	}
}

// ---------------------------------------------------------------------------
// GetAPIKey / GetDomainName with empty context (zero-value safety)
// ---------------------------------------------------------------------------

func TestGetAPIKey_EmptyContext(t *testing.T) {
	key := middleware.GetAPIKey(context.Background())
	if key != "" {
		t.Errorf("expected empty string from background context, got %q", key)
	}
}

func TestGetDomainName_EmptyContext(t *testing.T) {
	name := middleware.GetDomainName(context.Background())
	if name != "" {
		t.Errorf("expected empty string from background context, got %q", name)
	}
}
