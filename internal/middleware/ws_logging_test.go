package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bethmaloney/mailgun-mock-api/internal/middleware"
)

func TestWSLogScrubber_RedactsAccessToken(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.URL.Query().Get("access_token")
		if got != "REDACTED" {
			t.Errorf("expected access_token=REDACTED, got %q", got)
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := middleware.WSLogScrubber()(inner)

	req := httptest.NewRequest(http.MethodGet, "/mock/ws?access_token=secret-jwt-value", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

func TestWSLogScrubber_LeavesOtherParams(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		if got := q.Get("access_token"); got != "REDACTED" {
			t.Errorf("expected access_token=REDACTED, got %q", got)
		}
		if got := q.Get("foo"); got != "bar" {
			t.Errorf("expected foo=bar, got %q", got)
		}
		if got := q.Get("baz"); got != "qux" {
			t.Errorf("expected baz=qux, got %q", got)
		}

		w.WriteHeader(http.StatusOK)
	})

	handler := middleware.WSLogScrubber()(inner)

	req := httptest.NewRequest(http.MethodGet, "/mock/ws?foo=bar&access_token=secret&baz=qux", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

func TestWSLogScrubber_NoQueryParams(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("access_token"); got != "" {
			t.Errorf("expected no access_token param, got %q", got)
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := middleware.WSLogScrubber()(inner)

	req := httptest.NewRequest(http.MethodGet, "/mock/ws", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}
