package middleware_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bethmaloney/mailgun-mock-api/internal/apikey"
	"github.com/bethmaloney/mailgun-mock-api/internal/auth"
	"github.com/bethmaloney/mailgun-mock-api/internal/middleware"
	"github.com/bethmaloney/mailgun-mock-api/internal/mock"
	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// testDomain is a test-local struct that maps to the domains table.
// It avoids importing the domain package (which would create a circular
// dependency) while providing enough fields to set up test data.
type testDomain struct {
	Name string `gorm:"uniqueIndex"`
}

func (testDomain) TableName() string { return "domains" }

// setupTestDB creates a fresh in-memory SQLite database with the domains table
// migrated and ready for use.
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}
	if err := db.AutoMigrate(&testDomain{}, &apikey.ManagedAPIKey{}); err != nil {
		t.Fatalf("failed to migrate domains table: %v", err)
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
// Fake OIDC provider helpers (for Bearer / Entra ID tests)
// ---------------------------------------------------------------------------

// testOIDCProvider bundles an httptest.Server that serves OIDC discovery and
// JWKS endpoints, along with the RSA key pair used to sign test tokens.
type testOIDCProvider struct {
	Server *httptest.Server
	Key    *rsa.PrivateKey
}

const testAudience = "api://test-client"
const testScope = "access_as_user"

// newTestOIDCProvider starts a fake OIDC provider serving discovery and JWKS.
func newTestOIDCProvider(t *testing.T) *testOIDCProvider {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)

	// JWKS endpoint
	n := base64.RawURLEncoding.EncodeToString(key.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())
	jwksJSON := fmt.Sprintf(`{"keys":[{"kty":"RSA","kid":"test-key","use":"sig","alg":"RS256","n":"%s","e":"%s"}]}`, n, e)

	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, jwksJSON)
	})

	// OIDC discovery endpoint
	discoveryJSON := fmt.Sprintf(`{
		"issuer": %q,
		"authorization_endpoint": %q,
		"token_endpoint": %q,
		"jwks_uri": %q,
		"response_types_supported": ["code"],
		"subject_types_supported": ["public"],
		"id_token_signing_alg_values_supported": ["RS256"]
	}`, srv.URL, srv.URL+"/authorize", srv.URL+"/token", srv.URL+"/jwks")

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, discoveryJSON)
	})

	return &testOIDCProvider{
		Server: srv,
		Key:    key,
	}
}

// signTestToken signs a JWT with the given RSA key, setting the kid header.
func signTestToken(t *testing.T, key *rsa.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "test-key"
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}
	return signed
}

// newTestValidator creates a *auth.Validator backed by the given testOIDCProvider.
func newTestValidator(t *testing.T, tp *testOIDCProvider) *auth.Validator {
	t.Helper()
	ctx := context.Background()
	v, err := auth.NewValidatorForIssuer(ctx, tp.Server.URL, testAudience, testScope)
	if err != nil {
		t.Fatalf("failed to create test validator: %v", err)
	}
	return v
}

// validTokenClaims returns a jwt.MapClaims map for a valid token matching
// the test OIDC provider's issuer URL.
func validTokenClaims(issuerURL string) jwt.MapClaims {
	return jwt.MapClaims{
		"aud":                testAudience,
		"iss":                issuerURL,
		"scp":                testScope,
		"oid":                "user-object-id-123",
		"preferred_username": "alice@example.com",
		"name":               "Alice Smith",
		"exp":                time.Now().Add(time.Hour).Unix(),
		"iat":                time.Now().Unix(),
	}
}

// ---------------------------------------------------------------------------
// 1. HTTP Basic Auth Middleware (format validation) — disabled mode (v == nil)
// ---------------------------------------------------------------------------

func TestBasicAuth_ValidCredentials(t *testing.T) {
	db := setupTestDB(t)
	cfg := newMockConfig("full", "")
	handler := middleware.APIAuth(cfg, db, nil)(dummyHandler)

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
	db := setupTestDB(t)
	cfg := newMockConfig("full", "")
	handler := middleware.APIAuth(cfg, db, nil)(dummyHandler)

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
	db := setupTestDB(t)
	cfg := newMockConfig("full", "")
	handler := middleware.APIAuth(cfg, db, nil)(dummyHandler)

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
	db := setupTestDB(t)
	cfg := newMockConfig("full", "")
	handler := middleware.APIAuth(cfg, db, nil)(dummyHandler)

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
	db := setupTestDB(t)
	cfg := newMockConfig("full", "")
	handler := middleware.APIAuth(cfg, db, nil)(dummyHandler)

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
	db := setupTestDB(t)
	cfg := newMockConfig("full", "")
	handler := middleware.APIAuth(cfg, db, nil)(dummyHandler)

	schemes := []struct {
		name  string
		value string
	}{
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
// 2. Configurable Auth Mode — disabled mode (v == nil)
// ---------------------------------------------------------------------------

func TestAuthMode_AcceptAny_NoAuthHeader(t *testing.T) {
	db := setupTestDB(t)
	cfg := newMockConfig("accept_any", "")
	handler := middleware.APIAuth(cfg, db, nil)(dummyHandler)

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
	db := setupTestDB(t)
	cfg := newMockConfig("accept_any", "")
	handler := middleware.APIAuth(cfg, db, nil)(dummyHandler)

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
	db := setupTestDB(t)
	cfg := newMockConfig("accept_any", "")
	handler := middleware.APIAuth(cfg, db, nil)(dummyHandler)

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
	db := setupTestDB(t)
	masterKey := "key-master-secret-12345"
	cfg := newMockConfig("single", masterKey)
	handler := middleware.APIAuth(cfg, db, nil)(dummyHandler)

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
	db := setupTestDB(t)
	masterKey := "key-master-secret-12345"
	cfg := newMockConfig("single", masterKey)
	handler := middleware.APIAuth(cfg, db, nil)(dummyHandler)

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
	db := setupTestDB(t)
	masterKey := "key-master-secret-12345"
	cfg := newMockConfig("single", masterKey)
	handler := middleware.APIAuth(cfg, db, nil)(dummyHandler)

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
	db := setupTestDB(t)
	// Edge case: single mode but the signing key itself is empty in config.
	// Any non-empty password should fail since it won't match the empty signing key,
	// and empty passwords should also fail since Basic Auth requires a non-empty key.
	cfg := newMockConfig("single", "")
	handler := middleware.APIAuth(cfg, db, nil)(dummyHandler)

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
	db := setupTestDB(t)
	cfg := newMockConfig("full", "")
	handler := middleware.APIAuth(cfg, db, nil)(dummyHandler)

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
	db := setupTestDB(t)
	cfg := newMockConfig("full", "")
	handler := middleware.APIAuth(cfg, db, nil)(dummyHandler)

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
	db := setupTestDB(t)
	cfg := newMockConfig("full", "")
	handler := middleware.APIAuth(cfg, db, nil)(dummyHandler)

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
	db := setupTestDB(t)
	cfg := newMockConfig("full", "")
	handler := middleware.APIAuth(cfg, db, nil)(dummyHandler)

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
	db := setupTestDB(t)
	cfg := newMockConfig("accept_any", "key-master")
	handler := middleware.APIAuth(cfg, db, nil)(dummyHandler)

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
	domain := testDomain{Name: "example.com"}
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

	domain := testDomain{Name: "mail.example.org"}
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

	domain := testDomain{Name: "test.mailgun.org"}
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
		d := testDomain{Name: name}
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
	db := setupTestDB(t)
	cfg := newMockConfig("full", "")
	handler := middleware.APIAuth(cfg, db, nil)(dummyHandler)

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
	db := setupTestDB(t)
	cfg := newMockConfig("full", "")
	handler := middleware.APIAuth(cfg, db, nil)(dummyHandler)

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

// ---------------------------------------------------------------------------
// Managed Keys auth mode — disabled mode (v == nil)
// ---------------------------------------------------------------------------

func TestBasicAuth_ManagedKeys_Valid(t *testing.T) {
	db := setupTestDB(t)
	cfg := newMockConfig("managed_keys", "")

	key := apikey.ManagedAPIKey{Name: "test-key", KeyValue: "mock_testkey123", Prefix: "mock_testkey1"}
	if err := db.Create(&key).Error; err != nil {
		t.Fatalf("failed to create managed API key: %v", err)
	}

	handler := middleware.APIAuth(cfg, db, nil)(dummyHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", basicAuthHeader("api", "mock_testkey123"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	t.Run("returns 200 for valid managed key", func(t *testing.T) {
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

func TestBasicAuth_ManagedKeys_Invalid(t *testing.T) {
	db := setupTestDB(t)
	cfg := newMockConfig("managed_keys", "")

	key := apikey.ManagedAPIKey{Name: "test-key", KeyValue: "mock_testkey123", Prefix: "mock_testkey1"}
	if err := db.Create(&key).Error; err != nil {
		t.Fatalf("failed to create managed API key: %v", err)
	}

	handler := middleware.APIAuth(cfg, db, nil)(dummyHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", basicAuthHeader("api", "wrong-key"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	t.Run("returns 401 for invalid managed key", func(t *testing.T) {
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

func TestBasicAuth_ManagedKeys_EmptyTable(t *testing.T) {
	db := setupTestDB(t)
	cfg := newMockConfig("managed_keys", "")

	// Do NOT insert any managed API keys.
	handler := middleware.APIAuth(cfg, db, nil)(dummyHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", basicAuthHeader("api", "some-key"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	t.Run("returns 401 when no managed keys exist", func(t *testing.T) {
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

// ===========================================================================
// APIAuth — Bearer arm tests (Entra ID JWT validation)
// ===========================================================================

// TestAPIAuth_Bearer_ValidToken verifies that a valid JWT presented via
// Bearer auth passes through when a validator is configured.
func TestAPIAuth_Bearer_ValidToken(t *testing.T) {
	tp := newTestOIDCProvider(t)
	defer tp.Server.Close()

	db := setupTestDB(t)
	cfg := newMockConfig("full", "")
	v := newTestValidator(t, tp)

	handler := middleware.APIAuth(cfg, db, v)(dummyHandler)

	raw := signTestToken(t, tp.Key, validTokenClaims(tp.Server.URL))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+raw)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("expected body %q, got %q", "ok", rec.Body.String())
	}
}

// TestAPIAuth_Bearer_InvalidToken_NoFallthrough verifies that an invalid JWT
// via Bearer returns 401 and does NOT fall through to Basic auth.
func TestAPIAuth_Bearer_InvalidToken_NoFallthrough(t *testing.T) {
	tp := newTestOIDCProvider(t)
	defer tp.Server.Close()

	db := setupTestDB(t)
	cfg := newMockConfig("accept_any", "") // accept_any would pass Basic, proving no fall-through
	v := newTestValidator(t, tp)

	handler := middleware.APIAuth(cfg, db, v)(dummyHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer invalid-jwt-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401 for invalid Bearer token (no fall-through), got %d", rec.Code)
	}
}

// TestAPIAuth_Bearer_DisabledValidator_401 verifies that when a Bearer token
// is presented but the validator is nil (Entra ID disabled), the middleware
// returns 401 with a Basic challenge.
func TestAPIAuth_Bearer_DisabledValidator_401(t *testing.T) {
	db := setupTestDB(t)
	cfg := newMockConfig("full", "")

	handler := middleware.APIAuth(cfg, db, nil)(dummyHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer some-jwt-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401 for Bearer with nil validator, got %d", rec.Code)
	}

	// Should tell the client to use Basic auth instead.
	wwwAuth := rec.Header().Get("WWW-Authenticate")
	expected := `Basic realm="mailgun-mock-api"`
	if wwwAuth != expected {
		t.Errorf("expected WWW-Authenticate %q, got %q", expected, wwwAuth)
	}
}

// ===========================================================================
// APIAuth — Entra-mode Basic arm tests (v != nil, no Bearer)
// ===========================================================================

// TestAPIAuth_EntraBasic_ValidManagedKey verifies that when the validator is
// present (Entra mode) and a request uses Basic auth with a valid managed key,
// the request passes through.
func TestAPIAuth_EntraBasic_ValidManagedKey(t *testing.T) {
	tp := newTestOIDCProvider(t)
	defer tp.Server.Close()

	db := setupTestDB(t)
	cfg := newMockConfig("full", "")
	v := newTestValidator(t, tp)

	// Insert a managed API key.
	key := apikey.ManagedAPIKey{Name: "entra-key", KeyValue: "mock_entrakey123", Prefix: "mock_entrak"}
	if err := db.Create(&key).Error; err != nil {
		t.Fatalf("failed to create managed API key: %v", err)
	}

	handler := middleware.APIAuth(cfg, db, v)(dummyHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", basicAuthHeader("api", "mock_entrakey123"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 for valid managed key in Entra mode, got %d", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("expected body %q, got %q", "ok", rec.Body.String())
	}
}

// TestAPIAuth_EntraBasic_InvalidKey_401 verifies that when the validator is
// present (Entra mode) and a request uses Basic auth with an unknown key,
// the request is rejected with 401.
func TestAPIAuth_EntraBasic_InvalidKey_401(t *testing.T) {
	tp := newTestOIDCProvider(t)
	defer tp.Server.Close()

	db := setupTestDB(t)
	cfg := newMockConfig("full", "")
	v := newTestValidator(t, tp)

	// Insert a managed API key (the request will use a different one).
	key := apikey.ManagedAPIKey{Name: "entra-key", KeyValue: "mock_entrakey123", Prefix: "mock_entrak"}
	if err := db.Create(&key).Error; err != nil {
		t.Fatalf("failed to create managed API key: %v", err)
	}

	handler := middleware.APIAuth(cfg, db, v)(dummyHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", basicAuthHeader("api", "unknown-key-value"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401 for unknown key in Entra mode, got %d", rec.Code)
	}
}

// TestAPIAuth_EntraBasic_IgnoresMockConfigFullMode is a regression test for
// the anti-footgun behavior: even when MockConfig says "full" (accept any
// well-formed Basic auth), Entra mode MUST reject keys not found in
// managed_api_keys. This prevents accidentally accepting arbitrary keys when
// Entra ID auth is enabled.
func TestAPIAuth_EntraBasic_IgnoresMockConfigFullMode(t *testing.T) {
	tp := newTestOIDCProvider(t)
	defer tp.Server.Close()

	db := setupTestDB(t)
	// "full" mode normally accepts any well-formed Basic auth.
	cfg := newMockConfig("full", "")
	v := newTestValidator(t, tp)

	// Do NOT insert any managed keys — the table is empty.
	handler := middleware.APIAuth(cfg, db, v)(dummyHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", basicAuthHeader("api", "any-random-key"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401 in Entra mode even with full auth_mode, got %d", rec.Code)
	}
}

// ===========================================================================
// APIAuth — WWW-Authenticate challenge header tests
// ===========================================================================

// TestAPIAuth_Bearer_Invalid_SetsBearerChallenge verifies that when a Bearer
// token fails validation, the 401 response includes the correct
// WWW-Authenticate: Bearer challenge header.
func TestAPIAuth_Bearer_Invalid_SetsBearerChallenge(t *testing.T) {
	tp := newTestOIDCProvider(t)
	defer tp.Server.Close()

	db := setupTestDB(t)
	cfg := newMockConfig("full", "")
	v := newTestValidator(t, tp)

	handler := middleware.APIAuth(cfg, db, v)(dummyHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer invalid-jwt-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rec.Code)
	}

	wwwAuth := rec.Header().Get("WWW-Authenticate")
	expected := `Bearer realm="mailgun-mock-api"`
	if wwwAuth != expected {
		t.Errorf("expected WWW-Authenticate %q, got %q", expected, wwwAuth)
	}
}

// TestAPIAuth_Basic_Invalid_SetsBasicChallenge verifies that when Basic auth
// fails (in either Entra or disabled mode), the 401 response includes the
// correct WWW-Authenticate: Basic challenge header.
func TestAPIAuth_Basic_Invalid_SetsBasicChallenge(t *testing.T) {
	tp := newTestOIDCProvider(t)
	defer tp.Server.Close()

	db := setupTestDB(t)
	cfg := newMockConfig("full", "")
	v := newTestValidator(t, tp)

	// Entra mode, no managed keys — Basic auth with any key should fail.
	handler := middleware.APIAuth(cfg, db, v)(dummyHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", basicAuthHeader("api", "unknown-key"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rec.Code)
	}

	wwwAuth := rec.Header().Get("WWW-Authenticate")
	expected := `Basic realm="mailgun-mock-api"`
	if wwwAuth != expected {
		t.Errorf("expected WWW-Authenticate %q, got %q", expected, wwwAuth)
	}
}

// ===========================================================================
// APIAuth — Provider unavailable (503)
// ===========================================================================

// TestAPIAuth_Bearer_ProviderUnavailable_503 verifies that when the JWKS
// endpoint is unreachable (provider down), the middleware returns 503 with
// the appropriate WWW-Authenticate header indicating temporary unavailability.
func TestAPIAuth_Bearer_ProviderUnavailable_503(t *testing.T) {
	tp := newTestOIDCProvider(t)

	db := setupTestDB(t)
	cfg := newMockConfig("full", "")

	// Create the validator while the provider is still running.
	v := newTestValidator(t, tp)

	// Sign a token that would be valid if the provider were reachable.
	raw := signTestToken(t, tp.Key, validTokenClaims(tp.Server.URL))

	// Stop the OIDC provider so JWKS fetch fails during verification.
	tp.Server.Close()

	handler := middleware.APIAuth(cfg, db, v)(dummyHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+raw)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503 for unavailable provider, got %d", rec.Code)
	}

	wwwAuth := rec.Header().Get("WWW-Authenticate")
	expected := `Bearer error="temporarily_unavailable"`
	if wwwAuth != expected {
		t.Errorf("expected WWW-Authenticate %q, got %q", expected, wwwAuth)
	}
}
