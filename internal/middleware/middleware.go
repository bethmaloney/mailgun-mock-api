package middleware

import (
	"context"
	"crypto/subtle"
	"net/http"

	"github.com/bethmaloney/mailgun-mock-api/internal/mock"
	"github.com/bethmaloney/mailgun-mock-api/internal/response"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// contextKey is an unexported type used for context value keys to avoid collisions.
type contextKey string

const domainContextKey contextKey = "domain"

// DomainFromContext returns the domain name stored in the request context
// by the RequireDomain middleware, or an empty string if not set.
func DomainFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(domainContextKey).(string); ok {
		return v
	}
	return ""
}

// BasicAuth returns a chi-compatible middleware that enforces HTTP Basic Auth
// based on the mock config's auth mode.
//
// Auth modes (from configPtr.Authentication.AuthMode):
//   - "accept_any": Skip all auth checks — pass every request through
//   - "single": Validate HTTP Basic Auth format AND password matches SigningKey
//   - "full" (default): Validate HTTP Basic Auth format (username "api", non-empty password)
func BasicAuth(configPtr *mock.MockConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authMode := configPtr.Authentication.AuthMode
			signingKey := configPtr.Authentication.SigningKey

			switch authMode {
			case "accept_any":
				next.ServeHTTP(w, r)

			case "single":
				username, password, ok := r.BasicAuth()
				if !ok || username != "api" || password == "" {
					response.RespondError(w, http.StatusUnauthorized, "Forbidden")
					return
				}
				if subtle.ConstantTimeCompare([]byte(password), []byte(signingKey)) != 1 {
					response.RespondError(w, http.StatusUnauthorized, "Forbidden")
					return
				}
				next.ServeHTTP(w, r)

			default:
				// "full" or any other value: require valid Basic Auth format.
				username, password, ok := r.BasicAuth()
				if !ok || username != "api" || password == "" {
					response.RespondError(w, http.StatusUnauthorized, "Forbidden")
					return
				}
				next.ServeHTTP(w, r)
			}
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
