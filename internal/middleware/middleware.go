package middleware

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"

	"github.com/bethmaloney/mailgun-mock-api/internal/apikey"
	"github.com/bethmaloney/mailgun-mock-api/internal/auth"
	"github.com/bethmaloney/mailgun-mock-api/internal/mock"
	"github.com/bethmaloney/mailgun-mock-api/internal/response"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// contextKey is an unexported type used for context value keys to avoid collisions.
type contextKey string

const domainContextKey contextKey = "domain"
const subaccountContextKey contextKey = "subaccount"

// SubaccountFromContext returns the subaccount ID stored in the request context
// by the SubaccountScoping middleware, or an empty string if not set.
func SubaccountFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(subaccountContextKey).(string); ok {
		return v
	}
	return ""
}

// subaccountLookup is a minimal struct used to query the subaccounts table without
// importing the subaccount package (which would create a circular dependency).
type subaccountLookup struct {
	SubaccountID string
	Status       string
}

func (subaccountLookup) TableName() string { return "subaccounts" }

// SubaccountScoping returns a chi-compatible middleware that extracts the
// X-Mailgun-On-Behalf-Of header from the request. If the header is present,
// the middleware validates the subaccount exists and is not disabled, then
// stores the subaccount ID in the request context. If absent, the request
// proceeds with no subaccount context.
func SubaccountScoping(db *gorm.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			saHeader := r.Header.Get("X-Mailgun-On-Behalf-Of")
			if saHeader == "" {
				next.ServeHTTP(w, r)
				return
			}

			var sa subaccountLookup
			if err := db.Where("subaccount_id = ?", saHeader).First(&sa).Error; err != nil {
				response.RespondError(w, http.StatusBadRequest, "Invalid subaccount")
				return
			}

			if sa.Status == "disabled" {
				response.RespondError(w, http.StatusForbidden, "Subaccount is disabled")
				return
			}

			ctx := context.WithValue(r.Context(), subaccountContextKey, sa.SubaccountID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// DomainFromContext returns the domain name stored in the request context
// by the RequireDomain middleware, or an empty string if not set.
func DomainFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(domainContextKey).(string); ok {
		return v
	}
	return ""
}

const (
	wwwAuthBearer = `Bearer realm="mailgun-mock-api"`
	wwwAuthBasic  = `Basic realm="mailgun-mock-api"`
)

func unauthorized(w http.ResponseWriter, challenge, msg string) {
	w.Header().Set("WWW-Authenticate", challenge)
	response.RespondError(w, http.StatusUnauthorized, msg)
}

func providerUnavailable(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer error="temporarily_unavailable"`)
	response.RespondError(w, http.StatusServiceUnavailable, "Auth provider unavailable")
}

func extractBearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "Bearer ") {
		return h[7:]
	}
	return ""
}

// APIAuth returns a chi-compatible middleware that enforces authentication.
//
// It supports three authentication paths:
//  1. Bearer token (JWT) — if a Bearer token is present and v is non-nil, validate via Entra ID
//  2. Entra-mode Basic — if no Bearer token and v is non-nil, validate Basic auth against managed API keys
//  3. Disabled-mode Basic — if no Bearer token and v is nil, use configPtr.Authentication.AuthMode switch
func APIAuth(configPtr *mock.MockConfig, db *gorm.DB, v *auth.Validator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Arm 1: Bearer token
			if bearer := extractBearer(r); bearer != "" {
				if v == nil {
					unauthorized(w, wwwAuthBasic, "Forbidden")
					return
				}
				_, err := v.Validate(r.Context(), bearer)
				if err != nil {
					if errors.Is(err, auth.ErrProviderUnavailable) {
						providerUnavailable(w)
						return
					}
					unauthorized(w, wwwAuthBearer, "Invalid token")
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			// Arm 2: Entra-mode Basic (v != nil, no Bearer)
			if v != nil {
				username, password, ok := r.BasicAuth()
				if !ok || username != "api" || password == "" {
					unauthorized(w, wwwAuthBasic, "Forbidden")
					return
				}
				var key apikey.ManagedAPIKey
				if err := db.Where("key_value = ?", password).First(&key).Error; err != nil {
					unauthorized(w, wwwAuthBasic, "Forbidden")
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			// Arm 3: Disabled-mode Basic (v == nil, no Bearer)
			authMode := configPtr.Authentication.AuthMode
			signingKey := configPtr.Authentication.SigningKey

			switch authMode {
			case "accept_any":
				next.ServeHTTP(w, r)

			case "single":
				username, password, ok := r.BasicAuth()
				if !ok || username != "api" || password == "" {
					unauthorized(w, wwwAuthBasic, "Forbidden")
					return
				}
				if subtle.ConstantTimeCompare([]byte(password), []byte(signingKey)) != 1 {
					unauthorized(w, wwwAuthBasic, "Forbidden")
					return
				}
				next.ServeHTTP(w, r)

			case "managed_keys":
				username, password, ok := r.BasicAuth()
				if !ok || username != "api" || password == "" {
					unauthorized(w, wwwAuthBasic, "Forbidden")
					return
				}
				if db == nil {
					response.RespondError(w, http.StatusInternalServerError, "managed_keys mode requires a database connection")
					return
				}
				var key apikey.ManagedAPIKey
				if err := db.Where("key_value = ?", password).First(&key).Error; err != nil {
					unauthorized(w, wwwAuthBasic, "Forbidden")
					return
				}
				next.ServeHTTP(w, r)

			default:
				// "full" or any other value: require valid Basic Auth format.
				username, password, ok := r.BasicAuth()
				if !ok || username != "api" || password == "" {
					unauthorized(w, wwwAuthBasic, "Forbidden")
					return
				}
				next.ServeHTTP(w, r)
			}
		})
	}
}

// BasicAuth is a backward-compatible wrapper around APIAuth for call sites
// and tests that have not yet been updated to the new signature.
// Deprecated: Use APIAuth instead.
func BasicAuth(configPtr *mock.MockConfig, dbs ...*gorm.DB) func(http.Handler) http.Handler {
	var db *gorm.DB
	if len(dbs) > 0 {
		db = dbs[0]
	}
	return APIAuth(configPtr, db, nil)
}

// EntraRequired returns a chi-compatible middleware that enforces Entra ID
// JWT authentication on /mock/* routes. When v is nil (AUTH_MODE=disabled),
// all requests pass through. Otherwise, a valid JWT must be present either
// as a Bearer token in the Authorization header or as an access_token query
// parameter (for WebSocket connections).
func EntraRequired(v *auth.Validator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if v == nil {
				next.ServeHTTP(w, r)
				return
			}
			token := extractBearer(r)
			if token == "" {
				token = r.URL.Query().Get("access_token") // WebSocket path
			}
			if token == "" {
				unauthorized(w, wwwAuthBearer, "Unauthenticated")
				return
			}
			if _, err := v.Validate(r.Context(), token); err != nil {
				if errors.Is(err, auth.ErrProviderUnavailable) {
					providerUnavailable(w)
					return
				}
				unauthorized(w, wwwAuthBearer, "Invalid token")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// domainLookup is a minimal struct used to query the domains table without
// importing the domain package (which would create a circular dependency).
type domainLookup struct {
	Name string
}

func (domainLookup) TableName() string { return "domains" }

// RequireDomain returns a chi-compatible middleware that extracts a domain name
// from the URL and validates it exists in the database. It checks chi URL params
// in order: "domain", "domain_name", "name". On success, the domain name is
// stored in the request context and can be retrieved with DomainFromContext.
func RequireDomain(db *gorm.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			domainName := chi.URLParam(r, "domain")
			if domainName == "" {
				domainName = chi.URLParam(r, "domain_name")
			}
			if domainName == "" {
				domainName = chi.URLParam(r, "name")
			}

			if domainName == "" {
				response.RespondError(w, http.StatusNotFound, "Domain not found")
				return
			}

			if err := db.Where("name = ?", domainName).First(&domainLookup{}).Error; err != nil {
				response.RespondError(w, http.StatusNotFound, "Domain not found")
				return
			}

			ctx := context.WithValue(r.Context(), domainContextKey, domainName)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
