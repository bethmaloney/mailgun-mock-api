// Package pagination_test contains tests for the three reusable pagination
// patterns used across Mailgun API endpoints:
//
//  1. Cursor/URL-based pagination — used by events, suppressions, templates,
//     tags, and mailing lists. Returns opaque paging URLs with pivot cursors.
//  2. Skip/Limit offset pagination — used by domains, mailing list members,
//     routes, and SMTP credentials. Uses skip and limit query parameters with
//     a total_count response field.
//  3. Token-based pagination — used by v1 analytics endpoints. Returns a
//     base64-encoded cursor string in the response body.
package pagination_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/bethmaloney/mailgun-mock-api/internal/pagination"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newRequestWithQuery creates an HTTP GET request with the given query params.
func newRequestWithQuery(t *testing.T, path string, params map[string]string) *http.Request {
	t.Helper()
	u, err := url.Parse(path)
	if err != nil {
		t.Fatalf("failed to parse path %q: %v", path, err)
	}
	q := u.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	return httptest.NewRequest(http.MethodGet, u.String(), nil)
}

// ---------------------------------------------------------------------------
// 1. Cursor/URL-based Pagination
// ---------------------------------------------------------------------------

func TestCursorPagination_DefaultLimit(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v3/example.com/bounces", nil)
	params := pagination.ParseCursorParams(req)

	t.Run("default limit is 100", func(t *testing.T) {
		if params.Limit != 100 {
			t.Errorf("expected default limit 100, got %d", params.Limit)
		}
	})

	t.Run("default page is empty", func(t *testing.T) {
		if params.Page != "" {
			t.Errorf("expected empty page, got %q", params.Page)
		}
	})

	t.Run("default pivot is empty", func(t *testing.T) {
		if params.Pivot != "" {
			t.Errorf("expected empty pivot, got %q", params.Pivot)
		}
	})
}

func TestCursorPagination_CustomLimit(t *testing.T) {
	req := newRequestWithQuery(t, "/v3/example.com/bounces", map[string]string{
		"limit": "50",
	})
	params := pagination.ParseCursorParams(req)

	t.Run("respects custom limit", func(t *testing.T) {
		if params.Limit != 50 {
			t.Errorf("expected limit 50, got %d", params.Limit)
		}
	})
}

func TestCursorPagination_MaxLimit(t *testing.T) {
	tests := []struct {
		name     string
		limit    string
		expected int
	}{
		{"exactly at max", "1000", 1000},
		{"exceeds max", "1500", 1000},
		{"far exceeds max", "99999", 1000},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := newRequestWithQuery(t, "/v3/example.com/bounces", map[string]string{
				"limit": tc.limit,
			})
			params := pagination.ParseCursorParams(req)

			if params.Limit != tc.expected {
				t.Errorf("expected limit %d, got %d", tc.expected, params.Limit)
			}
		})
	}
}

func TestCursorPagination_InvalidLimit(t *testing.T) {
	tests := []struct {
		name  string
		limit string
	}{
		{"non-numeric", "abc"},
		{"negative", "-5"},
		{"zero", "0"},
		{"float", "10.5"},
		{"empty string", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := newRequestWithQuery(t, "/v3/example.com/bounces", map[string]string{
				"limit": tc.limit,
			})
			params := pagination.ParseCursorParams(req)

			if params.Limit != 100 {
				t.Errorf("expected default limit 100 for invalid limit %q, got %d", tc.limit, params.Limit)
			}
		})
	}
}

func TestCursorPagination_FirstPage(t *testing.T) {
	baseURL := "http://localhost:8025/v3/example.com/bounces"

	t.Run("first page has no meaningful previous URL", func(t *testing.T) {
		// On the first page there is no pivot, so previous should either be
		// empty or point back to the first page itself.
		paging := pagination.GeneratePagingURLs(baseURL, 100, "", true)

		// The previous URL should either be empty or equal to the first URL,
		// since there is no page before the first one.
		if paging.Previous != "" && paging.Previous != paging.First {
			t.Errorf("expected previous URL to be empty or equal to first URL, got %q", paging.Previous)
		}
	})

	t.Run("first URL is always present", func(t *testing.T) {
		paging := pagination.GeneratePagingURLs(baseURL, 100, "", true)

		if paging.First == "" {
			t.Error("expected non-empty first URL")
		}
	})

	t.Run("last URL is always present", func(t *testing.T) {
		paging := pagination.GeneratePagingURLs(baseURL, 100, "", true)

		if paging.Last == "" {
			t.Error("expected non-empty last URL")
		}
	})
}

func TestCursorPagination_GeneratesNextURL(t *testing.T) {
	baseURL := "http://localhost:8025/v3/example.com/bounces"
	pivotValue := "abc123"

	paging := pagination.GeneratePagingURLs(baseURL, 100, pivotValue, true)

	t.Run("next URL is not empty when there are more results", func(t *testing.T) {
		if paging.Next == "" {
			t.Error("expected non-empty next URL when hasMore is true")
		}
	})

	t.Run("next URL contains pivot parameter", func(t *testing.T) {
		parsed, err := url.Parse(paging.Next)
		if err != nil {
			t.Fatalf("failed to parse next URL %q: %v", paging.Next, err)
		}

		pivot := parsed.Query().Get("pivot")
		if pivot != pivotValue {
			t.Errorf("expected pivot=%q in next URL, got %q", pivotValue, pivot)
		}
	})

	t.Run("next URL contains page=next parameter", func(t *testing.T) {
		parsed, err := url.Parse(paging.Next)
		if err != nil {
			t.Fatalf("failed to parse next URL %q: %v", paging.Next, err)
		}

		page := parsed.Query().Get("page")
		if page != "next" {
			t.Errorf("expected page=next in next URL, got %q", page)
		}
	})

	t.Run("next URL contains limit parameter", func(t *testing.T) {
		parsed, err := url.Parse(paging.Next)
		if err != nil {
			t.Fatalf("failed to parse next URL %q: %v", paging.Next, err)
		}

		limit := parsed.Query().Get("limit")
		if limit != "100" {
			t.Errorf("expected limit=100 in next URL, got %q", limit)
		}
	})
}

func TestCursorPagination_GeneratesPreviousURL(t *testing.T) {
	baseURL := "http://localhost:8025/v3/example.com/bounces"
	pivotValue := "xyz789"

	paging := pagination.GeneratePagingURLs(baseURL, 100, pivotValue, true)

	t.Run("previous URL is not empty when pivot is provided", func(t *testing.T) {
		if paging.Previous == "" {
			t.Error("expected non-empty previous URL when pivot is provided")
		}
	})

	t.Run("previous URL contains pivot parameter", func(t *testing.T) {
		parsed, err := url.Parse(paging.Previous)
		if err != nil {
			t.Fatalf("failed to parse previous URL %q: %v", paging.Previous, err)
		}

		pivot := parsed.Query().Get("pivot")
		if pivot != pivotValue {
			t.Errorf("expected pivot=%q in previous URL, got %q", pivotValue, pivot)
		}
	})

	t.Run("previous URL contains page=prev parameter", func(t *testing.T) {
		parsed, err := url.Parse(paging.Previous)
		if err != nil {
			t.Fatalf("failed to parse previous URL %q: %v", paging.Previous, err)
		}

		page := parsed.Query().Get("page")
		if page != "prev" {
			t.Errorf("expected page=prev in previous URL, got %q", page)
		}
	})
}

func TestCursorPagination_EmptyResult(t *testing.T) {
	baseURL := "http://localhost:8025/v3/example.com/bounces"

	// No pivot, no more results — simulates an empty collection.
	paging := pagination.GeneratePagingURLs(baseURL, 100, "", false)

	t.Run("paging object is still returned", func(t *testing.T) {
		// Even with no items, the paging structure should exist.
		// At minimum, first and last URLs should be present.
		if paging.First == "" {
			t.Error("expected non-empty first URL even for empty result")
		}
		if paging.Last == "" {
			t.Error("expected non-empty last URL even for empty result")
		}
	})

	t.Run("paging object serializes to JSON correctly", func(t *testing.T) {
		result := pagination.CursorResult{Paging: paging}
		data, err := json.Marshal(result)
		if err != nil {
			t.Fatalf("failed to marshal CursorResult: %v", err)
		}
		if !json.Valid(data) {
			t.Errorf("CursorResult produced invalid JSON: %s", string(data))
		}

		// Verify the paging keys are present in JSON output.
		var decoded map[string]map[string]string
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("failed to unmarshal CursorResult: %v", err)
		}
		pagingMap, ok := decoded["paging"]
		if !ok {
			t.Fatal("expected 'paging' key in serialized CursorResult")
		}
		for _, key := range []string{"first", "last", "next", "previous"} {
			if _, exists := pagingMap[key]; !exists {
				t.Errorf("expected %q key in paging object", key)
			}
		}
	})
}

func TestCursorPagination_LastPage(t *testing.T) {
	baseURL := "http://localhost:8025/v3/example.com/bounces"
	pivotValue := "last-item-id"

	// hasMore=false indicates this is the last page.
	paging := pagination.GeneratePagingURLs(baseURL, 100, pivotValue, false)

	t.Run("next URL is empty or not meaningful on last page", func(t *testing.T) {
		// When there are no more results, the next URL should either be
		// empty or point to a page that would return no items.
		if paging.Next != "" {
			// If next is present, it should still have a valid URL structure
			// but ideally the caller would know not to follow it.
			_, err := url.Parse(paging.Next)
			if err != nil {
				t.Errorf("next URL on last page is not a valid URL: %v", err)
			}
		}
	})

	t.Run("previous URL is still present on last page", func(t *testing.T) {
		if paging.Previous == "" {
			t.Error("expected non-empty previous URL on last page when pivot is provided")
		}
	})

	t.Run("first and last URLs remain present", func(t *testing.T) {
		if paging.First == "" {
			t.Error("expected non-empty first URL on last page")
		}
		if paging.Last == "" {
			t.Error("expected non-empty last URL on last page")
		}
	})
}

func TestCursorPagination_URLBase(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
	}{
		{"localhost with custom port", "http://localhost:8025/v3/example.com/bounces"},
		{"production-like URL", "https://api.mailgun.net/v3/example.com/bounces"},
		{"custom host", "http://mock-mailgun.local/v3/example.com/bounces"},
		{"with path prefix", "http://localhost:9090/api/v3/example.com/bounces"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			paging := pagination.GeneratePagingURLs(tc.baseURL, 100, "pivot123", true)

			// All generated URLs should use the same base (scheme + host + path)
			// as the provided baseURL.
			parsedBase, err := url.Parse(tc.baseURL)
			if err != nil {
				t.Fatalf("failed to parse base URL: %v", err)
			}

			for _, urlStr := range []string{paging.First, paging.Last, paging.Next, paging.Previous} {
				if urlStr == "" {
					continue
				}
				parsed, err := url.Parse(urlStr)
				if err != nil {
					t.Errorf("failed to parse generated URL %q: %v", urlStr, err)
					continue
				}

				if parsed.Scheme != parsedBase.Scheme {
					t.Errorf("expected scheme %q, got %q in URL %q", parsedBase.Scheme, parsed.Scheme, urlStr)
				}
				if parsed.Host != parsedBase.Host {
					t.Errorf("expected host %q, got %q in URL %q", parsedBase.Host, parsed.Host, urlStr)
				}
				if !strings.HasPrefix(parsed.Path, parsedBase.Path) {
					t.Errorf("expected path to start with %q, got %q in URL %q", parsedBase.Path, parsed.Path, urlStr)
				}
			}
		})
	}
}

func TestCursorPagination_ParsesPageParam(t *testing.T) {
	pages := []string{"next", "prev", "first", "last"}

	for _, page := range pages {
		t.Run("parses page="+page, func(t *testing.T) {
			req := newRequestWithQuery(t, "/v3/example.com/bounces", map[string]string{
				"page":  page,
				"pivot": "some-cursor",
			})
			params := pagination.ParseCursorParams(req)

			if params.Page != page {
				t.Errorf("expected page %q, got %q", page, params.Page)
			}
			if params.Pivot != "some-cursor" {
				t.Errorf("expected pivot %q, got %q", "some-cursor", params.Pivot)
			}
		})
	}
}

func TestCursorPagination_PagingURLsJSONTags(t *testing.T) {
	paging := pagination.PagingURLs{
		First:    "http://localhost/first",
		Last:     "http://localhost/last",
		Next:     "http://localhost/next",
		Previous: "http://localhost/prev",
	}

	data, err := json.Marshal(paging)
	if err != nil {
		t.Fatalf("failed to marshal PagingURLs: %v", err)
	}

	var decoded map[string]string
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal PagingURLs: %v", err)
	}

	t.Run("first JSON key is correct", func(t *testing.T) {
		if decoded["first"] != "http://localhost/first" {
			t.Errorf("expected first=%q, got %q", "http://localhost/first", decoded["first"])
		}
	})

	t.Run("last JSON key is correct", func(t *testing.T) {
		if decoded["last"] != "http://localhost/last" {
			t.Errorf("expected last=%q, got %q", "http://localhost/last", decoded["last"])
		}
	})

	t.Run("next JSON key is correct", func(t *testing.T) {
		if decoded["next"] != "http://localhost/next" {
			t.Errorf("expected next=%q, got %q", "http://localhost/next", decoded["next"])
		}
	})

	t.Run("previous JSON key is correct", func(t *testing.T) {
		if decoded["previous"] != "http://localhost/prev" {
			t.Errorf("expected previous=%q, got %q", "http://localhost/prev", decoded["previous"])
		}
	})
}

// ---------------------------------------------------------------------------
// 2. Skip/Limit Offset Pagination
// ---------------------------------------------------------------------------

func TestSkipLimit_DefaultValues(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v4/domains", nil)
	params := pagination.ParseSkipLimitParams(req, 100, 1000)

	t.Run("default skip is 0", func(t *testing.T) {
		if params.Skip != 0 {
			t.Errorf("expected default skip 0, got %d", params.Skip)
		}
	})

	t.Run("default limit is 100", func(t *testing.T) {
		if params.Limit != 100 {
			t.Errorf("expected default limit 100, got %d", params.Limit)
		}
	})
}

func TestSkipLimit_CustomValues(t *testing.T) {
	req := newRequestWithQuery(t, "/v4/domains", map[string]string{
		"skip":  "20",
		"limit": "10",
	})
	params := pagination.ParseSkipLimitParams(req, 100, 1000)

	t.Run("respects custom skip", func(t *testing.T) {
		if params.Skip != 20 {
			t.Errorf("expected skip 20, got %d", params.Skip)
		}
	})

	t.Run("respects custom limit", func(t *testing.T) {
		if params.Limit != 10 {
			t.Errorf("expected limit 10, got %d", params.Limit)
		}
	})
}

func TestSkipLimit_MaxLimit(t *testing.T) {
	tests := []struct {
		name     string
		limit    string
		maxLimit int
		expected int
	}{
		{"at max", "1000", 1000, 1000},
		{"exceeds default max", "1500", 1000, 1000},
		{"exceeds custom max", "200", 150, 150},
		{"far exceeds max", "99999", 1000, 1000},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := newRequestWithQuery(t, "/v4/domains", map[string]string{
				"limit": tc.limit,
			})
			params := pagination.ParseSkipLimitParams(req, 100, tc.maxLimit)

			if params.Limit != tc.expected {
				t.Errorf("expected limit %d, got %d", tc.expected, params.Limit)
			}
		})
	}
}

func TestSkipLimit_InvalidValues(t *testing.T) {
	tests := []struct {
		name          string
		skip          string
		limit         string
		expectedSkip  int
		expectedLimit int
	}{
		{"non-numeric skip", "abc", "10", 0, 10},
		{"non-numeric limit", "5", "xyz", 5, 100},
		{"both non-numeric", "abc", "xyz", 0, 100},
		{"negative skip", "-5", "10", 0, 10},
		{"negative limit", "5", "-10", 5, 100},
		{"both negative", "-5", "-10", 0, 100},
		{"zero limit", "0", "0", 0, 100},
		{"float skip", "1.5", "10", 0, 10},
		{"float limit", "5", "10.5", 5, 100},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := newRequestWithQuery(t, "/v4/domains", map[string]string{
				"skip":  tc.skip,
				"limit": tc.limit,
			})
			params := pagination.ParseSkipLimitParams(req, 100, 1000)

			if params.Skip != tc.expectedSkip {
				t.Errorf("expected skip %d, got %d", tc.expectedSkip, params.Skip)
			}
			if params.Limit != tc.expectedLimit {
				t.Errorf("expected limit %d, got %d", tc.expectedLimit, params.Limit)
			}
		})
	}
}

func TestSkipLimit_ParseFromRequest(t *testing.T) {
	t.Run("parses from standard HTTP request", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v4/domains?skip=30&limit=15", nil)
		params := pagination.ParseSkipLimitParams(req, 100, 1000)

		if params.Skip != 30 {
			t.Errorf("expected skip 30, got %d", params.Skip)
		}
		if params.Limit != 15 {
			t.Errorf("expected limit 15, got %d", params.Limit)
		}
	})

	t.Run("uses defaults when no query params present", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v4/domains", nil)
		params := pagination.ParseSkipLimitParams(req, 100, 1000)

		if params.Skip != 0 {
			t.Errorf("expected default skip 0, got %d", params.Skip)
		}
		if params.Limit != 100 {
			t.Errorf("expected default limit 100, got %d", params.Limit)
		}
	})

	t.Run("only skip present uses default limit", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v4/domains?skip=10", nil)
		params := pagination.ParseSkipLimitParams(req, 100, 1000)

		if params.Skip != 10 {
			t.Errorf("expected skip 10, got %d", params.Skip)
		}
		if params.Limit != 100 {
			t.Errorf("expected default limit 100, got %d", params.Limit)
		}
	})

	t.Run("only limit present uses default skip", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v4/domains?limit=25", nil)
		params := pagination.ParseSkipLimitParams(req, 100, 1000)

		if params.Skip != 0 {
			t.Errorf("expected default skip 0, got %d", params.Skip)
		}
		if params.Limit != 25 {
			t.Errorf("expected limit 25, got %d", params.Limit)
		}
	})
}

func TestSkipLimit_TotalCount(t *testing.T) {
	// total_count is independent of skip and limit — it represents the total
	// number of items in the collection. This test verifies the SkipLimitParams
	// struct does not influence or depend on total_count.

	t.Run("skip and limit do not affect total count semantics", func(t *testing.T) {
		// Parse with a large skip that exceeds a hypothetical total count.
		req := newRequestWithQuery(t, "/v4/domains", map[string]string{
			"skip":  "5000",
			"limit": "100",
		})
		params := pagination.ParseSkipLimitParams(req, 100, 1000)

		// The params should faithfully represent what was requested; it's up
		// to the handler to return an empty items array when skip > total_count.
		if params.Skip != 5000 {
			t.Errorf("expected skip 5000, got %d", params.Skip)
		}
		if params.Limit != 100 {
			t.Errorf("expected limit 100, got %d", params.Limit)
		}
	})

	t.Run("SkipLimitParams does not contain total count", func(t *testing.T) {
		// Verify that SkipLimitParams only has Skip and Limit fields.
		// total_count is a response field, not a request parameter.
		params := pagination.SkipLimitParams{Skip: 0, Limit: 100}
		if params.Skip != 0 {
			t.Errorf("expected skip 0, got %d", params.Skip)
		}
		if params.Limit != 100 {
			t.Errorf("expected limit 100, got %d", params.Limit)
		}
	})
}

func TestSkipLimit_CustomDefaults(t *testing.T) {
	// Different endpoints may use different default limits.
	t.Run("default limit of 50", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v4/routes", nil)
		params := pagination.ParseSkipLimitParams(req, 50, 500)

		if params.Limit != 50 {
			t.Errorf("expected default limit 50, got %d", params.Limit)
		}
	})

	t.Run("max limit of 500", func(t *testing.T) {
		req := newRequestWithQuery(t, "/v4/routes", map[string]string{
			"limit": "600",
		})
		params := pagination.ParseSkipLimitParams(req, 50, 500)

		if params.Limit != 500 {
			t.Errorf("expected limit capped at 500, got %d", params.Limit)
		}
	})
}

// ---------------------------------------------------------------------------
// 3. Token-based Pagination
// ---------------------------------------------------------------------------

func TestTokenPagination_EncodeCursor(t *testing.T) {
	t.Run("encodes data to a non-empty base64 string", func(t *testing.T) {
		data := map[string]string{"last_id": "abc123"}
		cursor := pagination.EncodeCursor(data)

		if cursor == "" {
			t.Fatal("expected non-empty cursor string")
		}

		// Verify it is valid base64.
		_, err := base64.StdEncoding.DecodeString(cursor)
		if err != nil {
			// Try URL-safe base64 as well.
			_, err = base64.URLEncoding.DecodeString(cursor)
			if err != nil {
				// Also try raw variants (no padding).
				_, err = base64.RawStdEncoding.DecodeString(cursor)
				if err != nil {
					_, err = base64.RawURLEncoding.DecodeString(cursor)
					if err != nil {
						t.Errorf("cursor %q is not valid base64: %v", cursor, err)
					}
				}
			}
		}
	})

	t.Run("encodes multiple key-value pairs", func(t *testing.T) {
		data := map[string]string{
			"last_id":   "def456",
			"timestamp": "2024-01-15T10:30:00Z",
		}
		cursor := pagination.EncodeCursor(data)

		if cursor == "" {
			t.Fatal("expected non-empty cursor for multi-key data")
		}
	})

	t.Run("empty data produces a cursor", func(t *testing.T) {
		cursor := pagination.EncodeCursor(map[string]string{})

		// An empty map should still produce a valid (though possibly empty)
		// encoded cursor that can be decoded back.
		decoded, err := pagination.DecodeCursor(cursor)
		if err != nil {
			t.Fatalf("expected no error decoding cursor from empty data, got: %v", err)
		}
		if len(decoded) != 0 {
			t.Errorf("expected empty map after decoding empty data cursor, got %v", decoded)
		}
	})
}

func TestTokenPagination_DecodeCursor(t *testing.T) {
	t.Run("round-trips single key-value pair", func(t *testing.T) {
		original := map[string]string{"last_id": "abc123"}
		cursor := pagination.EncodeCursor(original)
		decoded, err := pagination.DecodeCursor(cursor)

		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if decoded["last_id"] != "abc123" {
			t.Errorf("expected last_id=%q, got %q", "abc123", decoded["last_id"])
		}
	})

	t.Run("round-trips multiple key-value pairs", func(t *testing.T) {
		original := map[string]string{
			"last_id":   "item-42",
			"timestamp": "2024-06-01T00:00:00Z",
			"domain":    "example.com",
		}
		cursor := pagination.EncodeCursor(original)
		decoded, err := pagination.DecodeCursor(cursor)

		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		for k, v := range original {
			if decoded[k] != v {
				t.Errorf("expected %s=%q, got %q", k, v, decoded[k])
			}
		}
	})

	t.Run("round-trips empty map", func(t *testing.T) {
		original := map[string]string{}
		cursor := pagination.EncodeCursor(original)
		decoded, err := pagination.DecodeCursor(cursor)

		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(decoded) != 0 {
			t.Errorf("expected empty map, got %v", decoded)
		}
	})

	t.Run("round-trips values with special characters", func(t *testing.T) {
		original := map[string]string{
			"query": "status=active&type=bounce",
			"name":  "test user <admin@example.com>",
		}
		cursor := pagination.EncodeCursor(original)
		decoded, err := pagination.DecodeCursor(cursor)

		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		for k, v := range original {
			if decoded[k] != v {
				t.Errorf("expected %s=%q, got %q", k, v, decoded[k])
			}
		}
	})
}

func TestTokenPagination_InvalidCursor(t *testing.T) {
	tests := []struct {
		name   string
		cursor string
	}{
		{"not base64", "!!!not-valid-base64!!!"},
		{"valid base64 but not valid JSON", base64.StdEncoding.EncodeToString([]byte("not json"))},
		{"valid base64 but wrong JSON type", base64.StdEncoding.EncodeToString([]byte("[1,2,3]"))},
		{"truncated base64", "eyJsYXN0X2lkIjoiYW"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := pagination.DecodeCursor(tc.cursor)
			if err == nil {
				t.Errorf("expected error for invalid cursor %q, got nil", tc.cursor)
			}
		})
	}
}

func TestTokenPagination_EmptyCursor(t *testing.T) {
	t.Run("empty cursor string means first page", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/analytics", nil)
		params := pagination.ParseTokenParams(req, 10, 1000)

		if params.Cursor != "" {
			t.Errorf("expected empty cursor for request with no cursor param, got %q", params.Cursor)
		}
	})

	t.Run("explicit empty cursor string in query", func(t *testing.T) {
		req := newRequestWithQuery(t, "/v1/analytics", map[string]string{
			"cursor": "",
		})
		params := pagination.ParseTokenParams(req, 10, 1000)

		if params.Cursor != "" {
			t.Errorf("expected empty cursor for empty cursor param, got %q", params.Cursor)
		}
	})

	t.Run("uses default limit when cursor is empty", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/analytics", nil)
		params := pagination.ParseTokenParams(req, 10, 1000)

		if params.Limit != 10 {
			t.Errorf("expected default limit 10, got %d", params.Limit)
		}
	})
}

func TestTokenPagination_CustomLimit(t *testing.T) {
	t.Run("respects custom limit", func(t *testing.T) {
		req := newRequestWithQuery(t, "/v1/analytics", map[string]string{
			"limit": "50",
		})
		params := pagination.ParseTokenParams(req, 10, 1000)

		if params.Limit != 50 {
			t.Errorf("expected limit 50, got %d", params.Limit)
		}
	})

	t.Run("caps at max limit", func(t *testing.T) {
		req := newRequestWithQuery(t, "/v1/analytics", map[string]string{
			"limit": "2000",
		})
		params := pagination.ParseTokenParams(req, 10, 1000)

		if params.Limit != 1000 {
			t.Errorf("expected limit capped at 1000, got %d", params.Limit)
		}
	})

	t.Run("invalid limit falls back to default", func(t *testing.T) {
		req := newRequestWithQuery(t, "/v1/analytics", map[string]string{
			"limit": "abc",
		})
		params := pagination.ParseTokenParams(req, 10, 1000)

		if params.Limit != 10 {
			t.Errorf("expected default limit 10 for invalid limit, got %d", params.Limit)
		}
	})

	t.Run("negative limit falls back to default", func(t *testing.T) {
		req := newRequestWithQuery(t, "/v1/analytics", map[string]string{
			"limit": "-5",
		})
		params := pagination.ParseTokenParams(req, 10, 1000)

		if params.Limit != 10 {
			t.Errorf("expected default limit 10 for negative limit, got %d", params.Limit)
		}
	})

	t.Run("zero limit falls back to default", func(t *testing.T) {
		req := newRequestWithQuery(t, "/v1/analytics", map[string]string{
			"limit": "0",
		})
		params := pagination.ParseTokenParams(req, 10, 1000)

		if params.Limit != 10 {
			t.Errorf("expected default limit 10 for zero limit, got %d", params.Limit)
		}
	})

	t.Run("different default limits per endpoint", func(t *testing.T) {
		// Time-based analytics default: 1500
		req := httptest.NewRequest(http.MethodGet, "/v1/analytics/time-series", nil)
		params := pagination.ParseTokenParams(req, 1500, 5000)

		if params.Limit != 1500 {
			t.Errorf("expected default limit 1500, got %d", params.Limit)
		}
	})
}

func TestTokenPagination_ParsesCursorFromRequest(t *testing.T) {
	cursorValue := base64.StdEncoding.EncodeToString([]byte(`{"last_id":"abc123"}`))

	req := newRequestWithQuery(t, "/v1/analytics", map[string]string{
		"cursor": cursorValue,
		"limit":  "25",
	})
	params := pagination.ParseTokenParams(req, 10, 1000)

	t.Run("parses cursor from query", func(t *testing.T) {
		if params.Cursor != cursorValue {
			t.Errorf("expected cursor %q, got %q", cursorValue, params.Cursor)
		}
	})

	t.Run("parses limit alongside cursor", func(t *testing.T) {
		if params.Limit != 25 {
			t.Errorf("expected limit 25, got %d", params.Limit)
		}
	})
}

func TestBuildTokenPaginationMeta(t *testing.T) {
	meta := pagination.BuildTokenPaginationMeta("next-cursor-token", 150, "timestamp:desc", 100)

	t.Run("cursor matches", func(t *testing.T) {
		if meta.Cursor != "next-cursor-token" {
			t.Errorf("expected Cursor %q, got %q", "next-cursor-token", meta.Cursor)
		}
	})

	t.Run("total matches", func(t *testing.T) {
		if meta.Total != 150 {
			t.Errorf("expected Total 150, got %d", meta.Total)
		}
	})

	t.Run("sort matches", func(t *testing.T) {
		if meta.Sort != "timestamp:desc" {
			t.Errorf("expected Sort %q, got %q", "timestamp:desc", meta.Sort)
		}
	})

	t.Run("limit matches", func(t *testing.T) {
		if meta.Limit != 100 {
			t.Errorf("expected Limit 100, got %d", meta.Limit)
		}
	})

	t.Run("serializes to JSON with correct field names", func(t *testing.T) {
		data, err := json.Marshal(meta)
		if err != nil {
			t.Fatalf("failed to marshal TokenPaginationMeta: %v", err)
		}

		var decoded map[string]interface{}
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		for _, key := range []string{"cursor", "total", "sort", "limit"} {
			if _, ok := decoded[key]; !ok {
				t.Errorf("expected %q key in JSON output", key)
			}
		}
	})
}
