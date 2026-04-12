package server_test

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
	"strings"
	"testing"
	"time"

	"github.com/bethmaloney/mailgun-mock-api/internal/auth"
	"github.com/bethmaloney/mailgun-mock-api/internal/config"
	"github.com/bethmaloney/mailgun-mock-api/internal/server"
	"github.com/golang-jwt/jwt/v5"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

const (
	testClientID = "test-client-id"
	testAudience = "api://" + testClientID
	testScope    = "access_as_user"
)

// startFakeOIDCProvider starts an httptest server that serves OIDC discovery
// and JWKS endpoints using the provided RSA key.
func startFakeOIDCProvider(t *testing.T, key *rsa.PrivateKey) *httptest.Server {
	t.Helper()

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

	return srv
}

// signTestToken creates a signed JWT with the given claims, setting sensible
// defaults for aud, iss, exp, and iat if not already present.
func signTestToken(t *testing.T, key *rsa.PrivateKey, issuer, audience string, claims jwt.MapClaims) string {
	t.Helper()

	if _, ok := claims["aud"]; !ok {
		claims["aud"] = audience
	}
	if _, ok := claims["iss"]; !ok {
		claims["iss"] = issuer
	}
	if _, ok := claims["exp"]; !ok {
		claims["exp"] = time.Now().Add(time.Hour).Unix()
	}
	if _, ok := claims["iat"]; !ok {
		claims["iat"] = time.Now().Unix()
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "test-key"
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}
	return signed
}

func TestServerEntraMode(t *testing.T) {
	// 1. Generate RSA key pair for signing tokens.
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}

	// 2. Start fake OIDC provider.
	oidcServer := startFakeOIDCProvider(t, key)
	defer oidcServer.Close()

	// 3. Create a validator pointed at the fake provider.
	ctx := context.Background()
	validator, err := auth.NewValidatorForIssuer(ctx, oidcServer.URL, testAudience, testScope)
	if err != nil {
		t.Fatalf("failed to create validator: %v", err)
	}

	// 4. Open an in-memory SQLite database.
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	// 5. Create config with entra mode enabled.
	cfg := &config.Config{
		AuthMode:         "entra",
		EntraTenantID:    "fake-tenant-id",
		EntraClientID:    testClientID,
		EntraAPIScope:    testScope,
		EntraRedirectURI: "http://localhost:3000/auth/callback",
	}

	// 6. Boot the full server.
	handler := server.New(ctx, db, cfg, validator)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	// Helper: make a request and return the response.
	doReq := func(method, path string, headers map[string]string) *http.Response {
		t.Helper()
		req, err := http.NewRequest(method, ts.URL+path, nil)
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		return resp
	}

	// Helper: sign a valid token with standard claims.
	validToken := func() string {
		return signTestToken(t, key, oidcServer.URL, testAudience, jwt.MapClaims{
			"scp":                testScope,
			"oid":                "test-user-oid",
			"preferred_username": "test@example.com",
			"name":               "Test User",
		})
	}

	// Case 1: GET /mock/health returns 200 without authentication.
	// Health endpoint must be accessible outside any auth middleware.
	t.Run("health_unauthenticated", func(t *testing.T) {
		resp := doReq("GET", "/mock/health", nil)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET /mock/health: status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
	})

	// Case 2: GET /mock/auth-config returns 200 with enabled=true, unauthenticated.
	t.Run("auth_config_unauthenticated", func(t *testing.T) {
		resp := doReq("GET", "/mock/auth-config", nil)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET /mock/auth-config: status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		var body map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode auth-config response: %v", err)
		}
		enabled, ok := body["enabled"].(bool)
		if !ok || !enabled {
			t.Errorf("GET /mock/auth-config: enabled = %v, want true", body["enabled"])
		}
		if clientID, _ := body["clientId"].(string); clientID != testClientID {
			t.Errorf("GET /mock/auth-config: clientId = %q, want %q", clientID, testClientID)
		}
	})

	// Case 3: GET /v4/domains with no auth returns 401 + WWW-Authenticate: Basic.
	t.Run("domains_no_auth_401_basic", func(t *testing.T) {
		resp := doReq("GET", "/v4/domains", nil)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("GET /v4/domains (no auth): status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
		}
		wwwAuth := resp.Header.Get("WWW-Authenticate")
		if !strings.HasPrefix(wwwAuth, "Basic") {
			t.Errorf("GET /v4/domains (no auth): WWW-Authenticate = %q, want Basic realm=...", wwwAuth)
		}
	})

	// Case 4: GET /v4/domains with expired Bearer returns 401 + WWW-Authenticate: Bearer.
	t.Run("domains_expired_bearer_401", func(t *testing.T) {
		expiredToken := signTestToken(t, key, oidcServer.URL, testAudience, jwt.MapClaims{
			"scp":                testScope,
			"oid":                "test-user-oid",
			"preferred_username": "test@example.com",
			"name":               "Test User",
			"exp":                time.Now().Add(-time.Hour).Unix(),
			"iat":                time.Now().Add(-2 * time.Hour).Unix(),
		})
		resp := doReq("GET", "/v4/domains", map[string]string{
			"Authorization": "Bearer " + expiredToken,
		})
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("GET /v4/domains (expired Bearer): status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
		}
		wwwAuth := resp.Header.Get("WWW-Authenticate")
		if !strings.HasPrefix(wwwAuth, "Bearer") {
			t.Errorf("GET /v4/domains (expired Bearer): WWW-Authenticate = %q, want Bearer realm=...", wwwAuth)
		}
	})

	// Case 5: GET /v4/domains with valid Bearer (correct aud + scp) returns 200.
	t.Run("domains_valid_bearer_200", func(t *testing.T) {
		resp := doReq("GET", "/v4/domains", map[string]string{
			"Authorization": "Bearer " + validToken(),
		})
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET /v4/domains (valid Bearer): status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
	})

	// Case 6: GET /v4/domains with Basic api:<unknown-key> returns 401.
	// In entra mode, unknown Basic credentials are rejected even though the mock
	// config defaults to "accept_any" — the entra-mode Basic arm only accepts
	// managed API keys.
	t.Run("domains_basic_unknown_key_401", func(t *testing.T) {
		resp := doReq("GET", "/v4/domains", map[string]string{
			"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte("api:unknown-key-value")),
		})
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("GET /v4/domains (Basic unknown key): status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
		}
	})

	// Case 7: POST /mock/api-keys creates a managed key; then GET /v4/domains
	// with Basic api:<that-key> returns 200.
	t.Run("managed_key_basic_auth_200", func(t *testing.T) {
		// Create a managed API key via the mock endpoint.
		createReq, err := http.NewRequest("POST", ts.URL+"/mock/api-keys", strings.NewReader(`{"name":"integration-test-key"}`))
		if err != nil {
			t.Fatalf("failed to create POST request: %v", err)
		}
		createReq.Header.Set("Content-Type", "application/json")
		createReq.Header.Set("Authorization", "Bearer "+validToken())
		createResp, err := http.DefaultClient.Do(createReq)
		if err != nil {
			t.Fatalf("POST /mock/api-keys failed: %v", err)
		}
		defer createResp.Body.Close()

		if createResp.StatusCode != http.StatusCreated {
			t.Fatalf("POST /mock/api-keys: status = %d, want %d", createResp.StatusCode, http.StatusCreated)
		}

		var keyResp struct {
			KeyValue string `json:"key_value"`
		}
		if err := json.NewDecoder(createResp.Body).Decode(&keyResp); err != nil {
			t.Fatalf("failed to decode api-keys response: %v", err)
		}
		if keyResp.KeyValue == "" {
			t.Fatal("POST /mock/api-keys: key_value is empty")
		}

		// Use the managed key for Basic auth.
		resp := doReq("GET", "/v4/domains", map[string]string{
			"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte("api:"+keyResp.KeyValue)),
		})
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET /v4/domains (Basic managed key): status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
	})

	// Case 8: GET /mock/dashboard with no token returns 401 + WWW-Authenticate: Bearer.
	// The /mock/* protected routes require Entra ID JWT when entra mode is enabled.
	t.Run("dashboard_no_token_401_bearer", func(t *testing.T) {
		resp := doReq("GET", "/mock/dashboard", nil)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("GET /mock/dashboard (no token): status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
		}
		wwwAuth := resp.Header.Get("WWW-Authenticate")
		if !strings.HasPrefix(wwwAuth, "Bearer") {
			t.Errorf("GET /mock/dashboard (no token): WWW-Authenticate = %q, want Bearer realm=...", wwwAuth)
		}
	})
}
