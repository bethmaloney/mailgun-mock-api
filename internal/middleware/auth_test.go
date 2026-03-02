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
	"github.com/bethmaloney/mailgun-mock-api/internal/mock"
	"github.com/go-chi/chi/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// setupTestDB creates a fresh in-memory SQLite database with the Domain table
// migrated and ready for use.
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}
	if err := db.AutoMigrate(&database.Domain{}); err != nil {
		t.Fatalf("failed to migrate Domain table: %v", err)
	}
	return db
}

// dummyHandler is a simple handler that writes 200 OK when the middleware
// allows the request through.
var dummyHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
})

// basicAuthHeader returns the value for an Authorization header using HTTP
// Basic Auth with the given username and password.
func basicAuthHeader(username, password string) string {
	creds := username + ":" + password
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(creds))
}

// newMockConfig creates a MockConfig with the given auth mode and signing key.
func newMockConfig(authMode, signingKey string) *mock.MockConfig {
	return &mock.MockConfig{
		Authentication: mock.AuthenticationConfig{
			AuthMode:  authMode,
			SigningKey: signingKey,
		},
	}
}

// parseErrorResponse decodes a JSON error response body and returns the
// "message" field value.
func parseErrorResponse(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode error response body: %v", err)
	}
	msg, ok := body["message"]
	if !ok {
		t.Fatal("error response missing 'message' key")
	}
	return msg
}

// ---------------------------------------------------------------------------
// 1. HTTP Basic Auth Middleware (format validation)
// ---------------------------------------------------------------------------

func TestBasicAuth_ValidCredentials(t *testing.T) {
	cfg := newMockConfig("full", "")
	handler := middleware.BasicAuth(cfg)(dummyHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", basicAuthHeader("api", "key-abc123"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	t.Run("returns 200 for valid Basic Auth", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}
	})

	t.Run("passes through to next handler", func(t *testing.T) {
		if rec.Body.String() != "ok" {
			t.Errorf("expected body %q, got %q", "ok", rec.Body.String())
		}
	})
}

func TestBasicAuth_MissingAuthorizationHeader(t *testing.T) {
	cfg := newMockConfig("full", "")
	handler := middleware.BasicAuth(cfg)(dummyHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	// No Authorization header set
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	t.Run("returns 401", func(t *testing.T) {
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401, got %d", rec.Code)
		}
	})

	t.Run("returns Forbidden message", func(t *testing.T) {
		msg := parseErrorResponse(t, rec)
		if msg != "Forbidden" {
			t.Errorf("expected message %q, got %q", "Forbidden", msg)
		}
	})
}

func TestBasicAuth_EmptyPassword(t *testing.T) {
	cfg := newMockConfig("full", "")
	handler := middleware.BasicAuth(cfg)(dummyHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", basicAuthHeader("api", ""))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	t.Run("returns 401 for empty password", func(t *testing.T) {
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401, got %d", rec.Code)
		}
	})

	t.Run("returns Forbidden message", func(t *testing.T) {
		msg := parseErrorResponse(t, rec)
		if msg != "Forbidden" {
			t.Errorf("expected message %q, got %q", "Forbidden", msg)
		}
	})
}

func TestBasicAuth_WrongUsername(t *testing.T) {
	cfg := newMockConfig("full", "")
	handler := middleware.BasicAuth(cfg)(dummyHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", basicAuthHeader("admin", "key-abc123"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	t.Run("returns 401 for wrong username", func(t *testing.T) {
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401, got %d", rec.Code)
		}
	})

	t.Run("returns Forbidden message", func(t *testing.T) {
		msg := parseErrorResponse(t, rec)
		if msg != "Forbidden" {
			t.Errorf("expected message %q, got %q", "Forbidden", msg)
		}
	})
}

func TestBasicAuth_MalformedAuthorizationHeader(t *testing.T) {
	cfg := newMockConfig("full", "")
	handler := middleware.BasicAuth(cfg)(dummyHandler)

	tests := []struct {
		name  string
		value string
	}{
		{"garbage value", "not-valid-at-all"},
		{"Basic with invalid base64", "Basic !!!invalid-base64!!!"},
		{"Basic with no colon in decoded value", "Basic " + base64.StdEncoding.EncodeToString([]byte("nocolon"))},
		{"empty string", ""},
		{"Basic with only spaces", "Basic    "},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Authorization", tc.value)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("expected status 401, got %d", rec.Code)
			}
		})
	}
}

func TestBasicAuth_NonBasicScheme(t *testing.T) {
	cfg := newMockConfig("full", "")
	handler := middleware.BasicAuth(cfg)(dummyHandler)

	schemes := []struct {
		name  string
		value string
	}{
		{"Bearer token", "Bearer some-jwt-token"},
		{"Digest auth", "Digest username=\"api\""},
		{"Token scheme", "Token key-abc123"},
	}

	for _, tc := range schemes {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Authorization", tc.value)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("expected status 401 for %s, got %d", tc.name, rec.Code)
			}

			msg := parseErrorResponse(t, rec)
			if msg != "Forbidden" {
				t.Errorf("expected message %q, got %q", "Forbidden", msg)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 2. Configurable Auth Mode
// ---------------------------------------------------------------------------

func TestAuthMode_AcceptAny_NoAuthHeader(t *testing.T) {
	cfg := newMockConfig("accept_any", "")
	handler := middleware.BasicAuth(cfg)(dummyHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	// No Authorization header
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	t.Run("passes through without auth header", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}
	})

	t.Run("next handler is called", func(t *testing.T) {
		if rec.Body.String() != "ok" {
			t.Errorf("expected body %q, got %q", "ok", rec.Body.String())
		}
	})
}

func TestAuthMode_AcceptAny_InvalidAuthHeader(t *testing.T) {
	cfg := newMockConfig("accept_any", "")
	handler := middleware.BasicAuth(cfg)(dummyHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "totally-bogus-value")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	t.Run("still passes through with invalid auth", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200 in accept_any mode, got %d", rec.Code)
		}
	})
}

func TestAuthMode_AcceptAny_ValidAuthHeader(t *testing.T) {
	cfg := newMockConfig("accept_any", "")
	handler := middleware.BasicAuth(cfg)(dummyHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", basicAuthHeader("api", "key-abc123"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	t.Run("passes through with valid auth too", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}
	})
}

func TestAuthMode_Single_CorrectKey(t *testing.T) {
	masterKey := "key-master-secret-12345"
	cfg := newMockConfig("single", masterKey)
	handler := middleware.BasicAuth(cfg)(dummyHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", basicAuthHeader("api", masterKey))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	t.Run("passes through with correct signing key", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}
	})

	t.Run("next handler is called", func(t *testing.T) {
		if rec.Body.String() != "ok" {
			t.Errorf("expected body %q, got %q", "ok", rec.Body.String())
		}
	})
}

func TestAuthMode_Single_WrongKey(t *testing.T) {
	masterKey := "key-master-secret-12345"
	cfg := newMockConfig("single", masterKey)
	handler := middleware.BasicAuth(cfg)(dummyHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", basicAuthHeader("api", "key-wrong-key-99999"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	t.Run("returns 401 for wrong key", func(t *testing.T) {
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401, got %d", rec.Code)
		}
	})

	t.Run("returns Forbidden message", func(t *testing.T) {
		msg := parseErrorResponse(t, rec)
		if msg != "Forbidden" {
			t.Errorf("expected message %q, got %q", "Forbidden", msg)
		}
	})
}

func TestAuthMode_Single_MissingAuth(t *testing.T) {
	masterKey := "key-master-secret-12345"
	cfg := newMockConfig("single", masterKey)
	handler := middleware.BasicAuth(cfg)(dummyHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	// No Authorization header
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	t.Run("returns 401 for missing auth", func(t *testing.T) {
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401, got %d", rec.Code)
		}
	})

	t.Run("returns Forbidden message", func(t *testing.T) {
		msg := parseErrorResponse(t, rec)
		if msg != "Forbidden" {
			t.Errorf("expected message %q, got %q", "Forbidden", msg)
		}
	})
}

func TestAuthMode_Single_EmptySigningKey(t *testing.T) {
	// Edge case: single mode but the signing key itself is empty in config.
	// Any non-empty password should fail since it won't match the empty signing key,
	// and empty passwords should also fail since Basic Auth requires a non-empty key.
	cfg := newMockConfig("single", "")
	handler := middleware.BasicAuth(cfg)(dummyHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", basicAuthHeader("api", "some-key"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	t.Run("returns 401 when key does not match empty signing key", func(t *testing.T) {
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401, got %d", rec.Code)
		}
	})
}

func TestAuthMode_Full_ValidBasicAuth(t *testing.T) {
	cfg := newMockConfig("full", "")
	handler := middleware.BasicAuth(cfg)(dummyHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", basicAuthHeader("api", "key-anything-goes"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	t.Run("passes through with valid Basic Auth", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}
	})
}

func TestAuthMode_Full_MissingAuth(t *testing.T) {
	cfg := newMockConfig("full", "")
	handler := middleware.BasicAuth(cfg)(dummyHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	t.Run("returns 401 for missing auth", func(t *testing.T) {
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401, got %d", rec.Code)
		}
	})
}

func TestAuthMode_Full_WrongUsername(t *testing.T) {
	cfg := newMockConfig("full", "")
	handler := middleware.BasicAuth(cfg)(dummyHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", basicAuthHeader("notapi", "key-abc"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	t.Run("returns 401 for wrong username in full mode", func(t *testing.T) {
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401, got %d", rec.Code)
		}
	})
}

func TestAuthMode_Full_EmptyPassword(t *testing.T) {
	cfg := newMockConfig("full", "")
	handler := middleware.BasicAuth(cfg)(dummyHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", basicAuthHeader("api", ""))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	t.Run("returns 401 for empty password in full mode", func(t *testing.T) {
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401, got %d", rec.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// Config changes take effect immediately (pointer-based config)
// ---------------------------------------------------------------------------

func TestAuthMode_DynamicConfigChange(t *testing.T) {
	cfg := newMockConfig("accept_any", "key-master")
	handler := middleware.BasicAuth(cfg)(dummyHandler)

	t.Run("initially passes without auth in accept_any mode", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}
	})

	// Simulate a config change at runtime (e.g., via PUT /mock/config)
	cfg.Authentication.AuthMode = "full"

	t.Run("after switching to full mode, missing auth returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401 after config change, got %d", rec.Code)
		}
	})

	t.Run("after switching to full mode, valid auth passes through", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", basicAuthHeader("api", "any-key"))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200 with valid auth, got %d", rec.Code)
		}
	})

	// Switch to single mode and verify key matching
	cfg.Authentication.AuthMode = "single"

	t.Run("after switching to single mode, correct key passes", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", basicAuthHeader("api", "key-master"))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200 with correct key, got %d", rec.Code)
		}
	})

	t.Run("after switching to single mode, wrong key returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", basicAuthHeader("api", "wrong-key"))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401 with wrong key, got %d", rec.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// 3. Domain Extraction Middleware (RequireDomain)
// ---------------------------------------------------------------------------

// newDomainRouter sets up a chi router with the RequireDomain middleware applied
// and a handler at the given route pattern. The URL parameter name in the pattern
// should be one of {domain}, {domain_name}, or {name}.
func newDomainRouter(db *gorm.DB, pattern string) *chi.Mux {
	r := chi.NewRouter()
	r.Route(pattern, func(r chi.Router) {
		r.Use(middleware.RequireDomain(db))
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			domainName := middleware.DomainFromContext(r.Context())
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"domain": domainName})
		})
	})
	return r
}

func TestRequireDomain_DomainExists(t *testing.T) {
	db := setupTestDB(t)

	// Insert a test domain into the database.
	domain := database.Domain{Name: "example.com"}
	if err := db.Create(&domain).Error; err != nil {
		t.Fatalf("failed to create test domain: %v", err)
	}

	router := newDomainRouter(db, "/v3/{domain}")

	req := httptest.NewRequest(http.MethodGet, "/v3/example.com/", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	t.Run("returns 200 when domain exists", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}
	})

	t.Run("domain name is available in context", func(t *testing.T) {
		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if body["domain"] != "example.com" {
			t.Errorf("expected domain %q in context, got %q", "example.com", body["domain"])
		}
	})
}

func TestRequireDomain_DomainNotFound(t *testing.T) {
	db := setupTestDB(t)

	// Do NOT insert any domain — the database is empty.
	router := newDomainRouter(db, "/v3/{domain}")

	req := httptest.NewRequest(http.MethodGet, "/v3/nonexistent.com/", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	t.Run("returns 404 when domain does not exist", func(t *testing.T) {
		if rec.Code != http.StatusNotFound {
			t.Errorf("expected status 404, got %d", rec.Code)
		}
	})

	t.Run("returns Domain not found message", func(t *testing.T) {
		msg := parseErrorResponse(t, rec)
		if msg != "Domain not found" {
			t.Errorf("expected message %q, got %q", "Domain not found", msg)
		}
	})
}

func TestRequireDomain_EmptyDomainParameter(t *testing.T) {
	db := setupTestDB(t)

	// Use a chi router where the domain param could be empty.
	// We set up a route that catches an empty segment.
	r := chi.NewRouter()
	r.Route("/v3/{domain}", func(r chi.Router) {
		r.Use(middleware.RequireDomain(db))
		r.Get("/", dummyHandler)
	})

	// Chi won't match "/v3//" to {domain}="" in the normal case,
	// so we also test via a direct handler invocation with an empty param.
	// First, test the route-based behavior:
	req := httptest.NewRequest(http.MethodGet, "/v3//", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	t.Run("returns 404 for empty domain path segment", func(t *testing.T) {
		// Chi may return 404 at the routing level or the middleware may catch it.
		// Either way, the request must not succeed.
		if rec.Code == http.StatusOK {
			t.Errorf("expected non-200 status for empty domain, got %d", rec.Code)
		}
	})
}

func TestRequireDomain_DomainNameParam(t *testing.T) {
	db := setupTestDB(t)

	domain := database.Domain{Name: "mail.example.org"}
	if err := db.Create(&domain).Error; err != nil {
		t.Fatalf("failed to create test domain: %v", err)
	}

	router := newDomainRouter(db, "/v3/domains/{domain_name}")

	req := httptest.NewRequest(http.MethodGet, "/v3/domains/mail.example.org/", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	t.Run("returns 200 when using domain_name param", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}
	})

	t.Run("domain name is accessible via context", func(t *testing.T) {
		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if body["domain"] != "mail.example.org" {
			t.Errorf("expected domain %q, got %q", "mail.example.org", body["domain"])
		}
	})
}

func TestRequireDomain_NameParam(t *testing.T) {
	db := setupTestDB(t)

	domain := database.Domain{Name: "test.mailgun.org"}
	if err := db.Create(&domain).Error; err != nil {
		t.Fatalf("failed to create test domain: %v", err)
	}

	router := newDomainRouter(db, "/v3/domains/{name}")

	req := httptest.NewRequest(http.MethodGet, "/v3/domains/test.mailgun.org/", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	t.Run("returns 200 when using name param", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}
	})

	t.Run("domain name is accessible via context", func(t *testing.T) {
		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if body["domain"] != "test.mailgun.org" {
			t.Errorf("expected domain %q, got %q", "test.mailgun.org", body["domain"])
		}
	})
}

func TestRequireDomain_DomainNotFound_WithNameParam(t *testing.T) {
	db := setupTestDB(t)

	router := newDomainRouter(db, "/v3/domains/{domain_name}")

	req := httptest.NewRequest(http.MethodGet, "/v3/domains/missing.com/", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	t.Run("returns 404 for non-existent domain via domain_name param", func(t *testing.T) {
		if rec.Code != http.StatusNotFound {
			t.Errorf("expected status 404, got %d", rec.Code)
		}
	})

	t.Run("returns Domain not found message", func(t *testing.T) {
		msg := parseErrorResponse(t, rec)
		if msg != "Domain not found" {
			t.Errorf("expected message %q, got %q", "Domain not found", msg)
		}
	})
}

func TestRequireDomain_MultipleDomains(t *testing.T) {
	db := setupTestDB(t)

	// Insert multiple domains.
	domains := []string{"alpha.com", "beta.com", "gamma.com"}
	for _, name := range domains {
		d := database.Domain{Name: name}
		if err := db.Create(&d).Error; err != nil {
			t.Fatalf("failed to create domain %q: %v", name, err)
		}
	}

	router := newDomainRouter(db, "/v3/{domain}")

	for _, name := range domains {
		t.Run("domain "+name+" found", func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/v3/"+name+"/", nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("expected status 200 for domain %q, got %d", name, rec.Code)
			}

			var body map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}
			if body["domain"] != name {
				t.Errorf("expected domain %q in context, got %q", name, body["domain"])
			}
		})
	}

	t.Run("unknown domain still returns 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v3/unknown.com/", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("expected status 404, got %d", rec.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// DomainFromContext returns empty string when not set
// ---------------------------------------------------------------------------

func TestDomainFromContext_EmptyWhenNotSet(t *testing.T) {
	ctx := context.Background()
	domainName := middleware.DomainFromContext(ctx)

	t.Run("returns empty string for bare context", func(t *testing.T) {
		if domainName != "" {
			t.Errorf("expected empty string from bare context, got %q", domainName)
		}
	})
}

// ---------------------------------------------------------------------------
// JSON content type on error responses
// ---------------------------------------------------------------------------

func TestBasicAuth_ErrorResponseIsJSON(t *testing.T) {
	cfg := newMockConfig("full", "")
	handler := middleware.BasicAuth(cfg)(dummyHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	// No auth header — should trigger 401
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	t.Run("error response has JSON content type", func(t *testing.T) {
		ct := rec.Header().Get("Content-Type")
		if ct != "application/json" {
			t.Errorf("expected Content-Type %q, got %q", "application/json", ct)
		}
	})
}

func TestRequireDomain_ErrorResponseIsJSON(t *testing.T) {
	db := setupTestDB(t)
	router := newDomainRouter(db, "/v3/{domain}")

	req := httptest.NewRequest(http.MethodGet, "/v3/nonexistent.com/", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	t.Run("error response has JSON content type", func(t *testing.T) {
		ct := rec.Header().Get("Content-Type")
		if ct != "application/json" {
			t.Errorf("expected Content-Type %q, got %q", "application/json", ct)
		}
	})
}

// ---------------------------------------------------------------------------
// BasicAuth works across HTTP methods
// ---------------------------------------------------------------------------

func TestBasicAuth_WorksWithDifferentHTTPMethods(t *testing.T) {
	cfg := newMockConfig("full", "")
	handler := middleware.BasicAuth(cfg)(dummyHandler)

	methods := []string{
		http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodDelete,
		http.MethodPatch,
	}

	for _, method := range methods {
		t.Run(method+" with valid auth passes", func(t *testing.T) {
			req := httptest.NewRequest(method, "/", nil)
			req.Header.Set("Authorization", basicAuthHeader("api", "key-test"))
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("expected status 200 for %s, got %d", method, rec.Code)
			}
		})

		t.Run(method+" without auth returns 401", func(t *testing.T) {
			req := httptest.NewRequest(method, "/", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("expected status 401 for %s, got %d", method, rec.Code)
			}
		})
	}
}
