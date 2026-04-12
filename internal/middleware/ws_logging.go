package middleware

import "net/http"

// WSLogScrubber returns a middleware that replaces the access_token query
// parameter value with "REDACTED" in the request URL passed to downstream
// handlers, so that JWT tokens are not leaked into access logs. Intended
// for per-route use on /mock/ws only — do not apply globally.
func WSLogScrubber() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("access_token") == "" {
				next.ServeHTTP(w, r)
				return
			}

			scrubbedReq := *r
			u := *r.URL
			q := u.Query()
			q.Set("access_token", "REDACTED")
			u.RawQuery = q.Encode()
			scrubbedReq.URL = &u

			next.ServeHTTP(w, &scrubbedReq)
		})
	}
}
