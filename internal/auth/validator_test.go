package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// testProvider bundles an httptest.Server that serves OIDC discovery and JWKS
// endpoints, along with the RSA key pair used to sign test tokens.
type testProvider struct {
	Server *httptest.Server
	Key    *rsa.PrivateKey
}

// newTestProvider starts a fake OIDC provider serving discovery and JWKS.
func newTestProvider(t *testing.T) *testProvider {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}

	// We need the server URL in the responses, but don't know it until after
	// the server starts. Use a mux and capture the URL via closure.
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

	return &testProvider{
		Server: srv,
		Key:    key,
	}
}

// signToken signs a JWT with the test RSA key, setting the kid header.
func signToken(t *testing.T, key *rsa.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "test-key"
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}
	return signed
}

const testAudience = "test-client-id"
const testScope = "access_as_user"

func TestValidate(t *testing.T) {
	t.Run("ValidToken_WithRequiredScope", func(t *testing.T) {
		tp := newTestProvider(t)
		defer tp.Server.Close()

		ctx := context.Background()
		v, err := newValidatorForIssuer(ctx, tp.Server.URL, testAudience, testScope)
		if err != nil {
			t.Fatalf("newValidatorForIssuer: %v", err)
		}

		raw := signToken(t, tp.Key, jwt.MapClaims{
			"aud":                testAudience,
			"iss":                tp.Server.URL,
			"scp":                "access_as_user",
			"oid":                "user-object-id-123",
			"preferred_username": "alice@example.com",
			"name":               "Alice Smith",
			"exp":                time.Now().Add(time.Hour).Unix(),
			"iat":                time.Now().Unix(),
		})

		claims, err := v.Validate(ctx, raw)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if claims.OID != "user-object-id-123" {
			t.Errorf("OID = %q, want %q", claims.OID, "user-object-id-123")
		}
		if claims.Email != "alice@example.com" {
			t.Errorf("Email = %q, want %q", claims.Email, "alice@example.com")
		}
		if claims.Name != "Alice Smith" {
			t.Errorf("Name = %q, want %q", claims.Name, "Alice Smith")
		}
		if claims.Scope != "access_as_user" {
			t.Errorf("Scope = %q, want %q", claims.Scope, "access_as_user")
		}
	})

	t.Run("ValidToken_MissingRequiredScope", func(t *testing.T) {
		tp := newTestProvider(t)
		defer tp.Server.Close()

		ctx := context.Background()
		v, err := newValidatorForIssuer(ctx, tp.Server.URL, testAudience, testScope)
		if err != nil {
			t.Fatalf("newValidatorForIssuer: %v", err)
		}

		raw := signToken(t, tp.Key, jwt.MapClaims{
			"aud":                testAudience,
			"iss":                tp.Server.URL,
			"scp":                "other_scope",
			"oid":                "user-object-id-123",
			"preferred_username": "alice@example.com",
			"name":               "Alice Smith",
			"exp":                time.Now().Add(time.Hour).Unix(),
			"iat":                time.Now().Unix(),
		})

		_, err = v.Validate(ctx, raw)
		if err == nil {
			t.Fatal("expected error for missing required scope, got nil")
		}
		if got := err.Error(); !strings.Contains(got, "missing required scope") {
			t.Errorf("error = %q, want it to contain %q", got, "missing required scope")
		}
	})

	t.Run("ValidToken_ScopeAmongSeveral", func(t *testing.T) {
		tp := newTestProvider(t)
		defer tp.Server.Close()

		ctx := context.Background()
		v, err := newValidatorForIssuer(ctx, tp.Server.URL, testAudience, testScope)
		if err != nil {
			t.Fatalf("newValidatorForIssuer: %v", err)
		}

		raw := signToken(t, tp.Key, jwt.MapClaims{
			"aud":                testAudience,
			"iss":                tp.Server.URL,
			"scp":                "openid profile access_as_user",
			"oid":                "user-object-id-456",
			"preferred_username": "bob@example.com",
			"name":               "Bob Jones",
			"exp":                time.Now().Add(time.Hour).Unix(),
			"iat":                time.Now().Unix(),
		})

		claims, err := v.Validate(ctx, raw)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if claims.OID != "user-object-id-456" {
			t.Errorf("OID = %q, want %q", claims.OID, "user-object-id-456")
		}
		if claims.Email != "bob@example.com" {
			t.Errorf("Email = %q, want %q", claims.Email, "bob@example.com")
		}
		if claims.Name != "Bob Jones" {
			t.Errorf("Name = %q, want %q", claims.Name, "Bob Jones")
		}
		if claims.Scope != "openid profile access_as_user" {
			t.Errorf("Scope = %q, want %q", claims.Scope, "openid profile access_as_user")
		}
	})

	t.Run("ExpiredToken", func(t *testing.T) {
		tp := newTestProvider(t)
		defer tp.Server.Close()

		ctx := context.Background()
		v, err := newValidatorForIssuer(ctx, tp.Server.URL, testAudience, testScope)
		if err != nil {
			t.Fatalf("newValidatorForIssuer: %v", err)
		}

		raw := signToken(t, tp.Key, jwt.MapClaims{
			"aud":                testAudience,
			"iss":                tp.Server.URL,
			"scp":                "access_as_user",
			"oid":                "user-object-id-123",
			"preferred_username": "alice@example.com",
			"name":               "Alice Smith",
			"exp":                time.Now().Add(-time.Hour).Unix(),
			"iat":                time.Now().Add(-2 * time.Hour).Unix(),
		})

		_, err = v.Validate(ctx, raw)
		if err == nil {
			t.Fatal("expected error for expired token, got nil")
		}
		if errors.Is(err, ErrProviderUnavailable) {
			t.Error("expired token should not be classified as provider unavailable")
		}
	})

	t.Run("WrongAudience", func(t *testing.T) {
		tp := newTestProvider(t)
		defer tp.Server.Close()

		ctx := context.Background()
		v, err := newValidatorForIssuer(ctx, tp.Server.URL, testAudience, testScope)
		if err != nil {
			t.Fatalf("newValidatorForIssuer: %v", err)
		}

		raw := signToken(t, tp.Key, jwt.MapClaims{
			"aud":                "wrong-audience",
			"iss":                tp.Server.URL,
			"scp":                "access_as_user",
			"oid":                "user-object-id-123",
			"preferred_username": "alice@example.com",
			"name":               "Alice Smith",
			"exp":                time.Now().Add(time.Hour).Unix(),
			"iat":                time.Now().Unix(),
		})

		_, err = v.Validate(ctx, raw)
		if err == nil {
			t.Fatal("expected error for wrong audience, got nil")
		}
		if errors.Is(err, ErrProviderUnavailable) {
			t.Error("wrong audience should not be classified as provider unavailable")
		}
	})

	t.Run("WrongIssuer", func(t *testing.T) {
		tp := newTestProvider(t)
		defer tp.Server.Close()

		ctx := context.Background()
		v, err := newValidatorForIssuer(ctx, tp.Server.URL, testAudience, testScope)
		if err != nil {
			t.Fatalf("newValidatorForIssuer: %v", err)
		}

		// Sign the token with a different issuer than what the provider advertises.
		raw := signToken(t, tp.Key, jwt.MapClaims{
			"aud":                testAudience,
			"iss":                "https://wrong-issuer.example.com",
			"scp":                "access_as_user",
			"oid":                "user-object-id-123",
			"preferred_username": "alice@example.com",
			"name":               "Alice Smith",
			"exp":                time.Now().Add(time.Hour).Unix(),
			"iat":                time.Now().Unix(),
		})

		_, err = v.Validate(ctx, raw)
		if err == nil {
			t.Fatal("expected error for wrong issuer, got nil")
		}
		if errors.Is(err, ErrProviderUnavailable) {
			t.Error("wrong issuer should not be classified as provider unavailable")
		}
	})

	t.Run("MalformedToken", func(t *testing.T) {
		tp := newTestProvider(t)
		defer tp.Server.Close()

		ctx := context.Background()
		v, err := newValidatorForIssuer(ctx, tp.Server.URL, testAudience, testScope)
		if err != nil {
			t.Fatalf("newValidatorForIssuer: %v", err)
		}

		_, err = v.Validate(ctx, "not-a-jwt")
		if err == nil {
			t.Fatal("expected error for malformed token, got nil")
		}
		if errors.Is(err, ErrProviderUnavailable) {
			t.Error("malformed token should not be classified as provider unavailable")
		}
	})

	t.Run("JWKS_Unreachable", func(t *testing.T) {
		tp := newTestProvider(t)

		ctx := context.Background()
		v, err := newValidatorForIssuer(ctx, tp.Server.URL, testAudience, testScope)
		if err != nil {
			t.Fatalf("newValidatorForIssuer: %v", err)
		}

		raw := signToken(t, tp.Key, jwt.MapClaims{
			"aud":                testAudience,
			"iss":                tp.Server.URL,
			"scp":                "access_as_user",
			"oid":                "user-object-id-123",
			"preferred_username": "alice@example.com",
			"name":               "Alice Smith",
			"exp":                time.Now().Add(time.Hour).Unix(),
			"iat":                time.Now().Unix(),
		})

		// Stop the provider so JWKS is unreachable.
		tp.Server.Close()

		_, err = v.Validate(ctx, raw)
		if err == nil {
			t.Fatal("expected error when JWKS is unreachable, got nil")
		}
		if !errors.Is(err, ErrProviderUnavailable) {
			t.Errorf("expected errors.Is(err, ErrProviderUnavailable), got: %v", err)
		}
	})
}

