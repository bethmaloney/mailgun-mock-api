package middleware

import (
	"context"
	"crypto/subtle"
	"net/http"

	"github.com/bethmaloney/mailgun-mock-api/internal/database"
	"github.com/bethmaloney/mailgun-mock-api/internal/response"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// AuthConfig holds functions that return the current authentication settings.
// Using closures allows the configuration to change dynamically between requests.
type AuthConfig struct {
	GetAuthMode   func() string // returns "none", "accept_any", or "strict"
	GetSigningKey func() string // returns the master signing key for strict mode
}

// contextKey is an unexported type used for context value keys to avoid collisions.
type contextKey string

const (
	apiKeyContextKey     contextKey = "apiKey"
	domainNameContextKey contextKey = "domainName"
)

// GetAPIKey returns the API key stored in the context, or an empty string if not set.
func GetAPIKey(ctx context.Context) string {
	if v, ok := ctx.Value(apiKeyContextKey).(string); ok {
		return v
	}
	return ""
}

// GetDomainName returns the domain name stored in the context, or an empty string if not set.
func GetDomainName(ctx context.Context) string {
	if v, ok := ctx.Value(domainNameContextKey).(string); ok {
		return v
	}
	return ""
}

// DomainFromContext is an alias for GetDomainName. It returns the domain name
// stored in the request context by the RequireDomain middleware.
func DomainFromContext(ctx context.Context) string {
	return GetDomainName(ctx)
}

// BasicAuth returns a chi-compatible middleware that enforces HTTP Basic Auth.
//
// Auth modes:
//   - "none": pass all requests through (still extract API key if present)
//   - "accept_any": require valid Basic Auth (username "api", non-empty password)
//   - "strict": like accept_any, plus the password must match the signing key
func BasicAuth(config *AuthConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authMode := config.GetAuthMode()
			signingKey := config.GetSigningKey()

			switch authMode {
			case "none":
				// Pass all requests through. Still extract the API key if present.
				username, password, ok := r.BasicAuth()
				if ok && username == "api" && password != "" {
					ctx := context.WithValue(r.Context(), apiKeyContextKey, password)
					r = r.WithContext(ctx)
				}
				next.ServeHTTP(w, r)

			case "strict":
				username, password, ok := r.BasicAuth()
				if !ok || username != "api" || password == "" {
					response.RespondError(w, http.StatusUnauthorized, "Forbidden")
					return
				}
				if subtle.ConstantTimeCompare([]byte(password), []byte(signingKey)) != 1 {
					response.RespondError(w, http.StatusUnauthorized, "Forbidden")
					return
				}
				ctx := context.WithValue(r.Context(), apiKeyContextKey, password)
				next.ServeHTTP(w, r.WithContext(ctx))

			default:
				// "accept_any" or any other value: require valid Basic Auth format
				username, password, ok := r.BasicAuth()
				if !ok || username != "api" || password == "" {
					response.RespondError(w, http.StatusUnauthorized, "Forbidden")
					return
				}
				ctx := context.WithValue(r.Context(), apiKeyContextKey, password)
				next.ServeHTTP(w, r.WithContext(ctx))
			}
		})
	}
}

// RequireDomain returns a chi-compatible middleware that extracts a domain name
// from the URL and validates it exists in the database. It checks chi URL params
// in order: "domain", "domain_name", "name".
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

			var domain database.Domain
			if err := db.Where("name = ?", domainName).First(&domain).Error; err != nil {
				response.RespondError(w, http.StatusNotFound, "Domain not found")
				return
			}

			ctx := context.WithValue(r.Context(), domainNameContextKey, domainName)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
