package auth

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
)

// Claims holds the extracted claims from a validated JWT.
type Claims struct {
	OID   string `json:"oid"`
	Email string `json:"preferred_username"`
	Name  string `json:"name"`
	Scope string `json:"scp"` // space-delimited list of granted scopes
}

// ErrProviderUnavailable signals that token validation could not complete
// because the identity provider (JWKS / discovery) was unreachable.
var ErrProviderUnavailable = errors.New("auth: identity provider unavailable")

// Validator validates Entra ID JWTs.
type Validator struct {
	verifier      *oidc.IDTokenVerifier
	requiredScope string
}

// NewValidator creates a Validator using OIDC discovery against Entra ID.
func NewValidator(ctx context.Context, tenantID, expectedAud, requiredScope string) (*Validator, error) {
	issuer := fmt.Sprintf("https://login.microsoftonline.com/%s/v2.0", tenantID)
	return newValidatorForIssuer(ctx, issuer, expectedAud, requiredScope)
}

// newValidatorForIssuer is an internal helper for tests that need to override the issuer URL.
func newValidatorForIssuer(ctx context.Context, issuerURL, expectedAud, requiredScope string) (*Validator, error) {
	provider, err := oidc.NewProvider(ctx, issuerURL)
	if err != nil {
		return nil, err
	}
	verifier := provider.Verifier(&oidc.Config{ClientID: expectedAud})
	return &Validator{verifier: verifier, requiredScope: requiredScope}, nil
}

// Validate validates a raw JWT string and returns the extracted claims.
func (v *Validator) Validate(ctx context.Context, raw string) (*Claims, error) {
	tok, err := v.verifier.Verify(ctx, raw)
	if err != nil {
		if isNetworkError(err) {
			return nil, fmt.Errorf("%w: %v", ErrProviderUnavailable, err)
		}
		return nil, err
	}

	var c Claims
	if err := tok.Claims(&c); err != nil {
		return nil, err
	}

	if v.requiredScope != "" {
		scopes := strings.Fields(c.Scope)
		found := false
		for _, s := range scopes {
			if s == v.requiredScope {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("token missing required scope %q", v.requiredScope)
		}
	}

	return &c, nil
}

// isNetworkError checks whether err (or any error in its chain) is a network
// or URL-fetching error. go-oidc doesn't always use %w, so we also do a
// recursive string-free unwrap check via Unwrap() interface.
func isNetworkError(err error) bool {
	var urlErr *url.Error
	var netErr net.Error
	if errors.As(err, &urlErr) || errors.As(err, &netErr) {
		return true
	}
	// go-oidc may wrap errors without %w; walk the chain via strings as last resort.
	if strings.Contains(err.Error(), "connect: connection refused") ||
		strings.Contains(err.Error(), "dial tcp") ||
		strings.Contains(err.Error(), "no such host") {
		return true
	}
	return false
}
