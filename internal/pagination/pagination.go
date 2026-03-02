package pagination

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// CursorParams holds the parsed query parameters for cursor/URL-based
// pagination (used by events, suppressions, templates, tags, mailing lists).
type CursorParams struct {
	Limit int    // page size
	Page  string // "next", "prev", "first", "last", or ""
	Pivot string // opaque cursor value identifying the pivot item
}

// PagingURLs is the paging object returned in cursor-paginated responses.
// Each field is a full URL (or empty string when the direction is unavailable).
type PagingURLs struct {
	First    string `json:"first"`
	Last     string `json:"last"`
	Next     string `json:"next"`
	Previous string `json:"previous"`
}

// CursorResult is a convenience wrapper for JSON responses that include a
// paging object.
type CursorResult struct {
	Paging PagingURLs `json:"paging"`
}

// SkipLimitParams holds the parsed skip/limit query parameters for
// offset-based pagination (used by domains, mailing list members, routes,
// SMTP credentials).
type SkipLimitParams struct {
	Skip  int
	Limit int
}

// OffsetResult holds the outcome of an offset-based (skip/limit) paginated
// query, including the total number of matching rows.
type OffsetResult struct {
	TotalCount int64       `json:"total_count"`
	Items      interface{} `json:"items"`
}

// TokenParams holds the parsed query parameters for token-based pagination
// (used by v1 analytics endpoints).
type TokenParams struct {
	Cursor string
	Limit  int
}

// TokenPaginationMeta is the pagination metadata block used by the v1
// analytics endpoints (token-based pagination).
type TokenPaginationMeta struct {
	Cursor string `json:"cursor"`
	Total  int64  `json:"total"`
	Sort   string `json:"sort"`
	Limit  int    `json:"limit"`
}

// ---------------------------------------------------------------------------
// Cursor/URL-based pagination
// ---------------------------------------------------------------------------

// ParseCursorParams extracts cursor pagination parameters (limit, page, pivot)
// from the HTTP request query string. The default limit is 100 and the maximum
// is 1000.
func ParseCursorParams(r *http.Request) CursorParams {
	q := r.URL.Query()

	limit := 100
	if v := q.Get("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > 1000 {
		limit = 1000
	}

	return CursorParams{
		Limit: limit,
		Page:  q.Get("page"),
		Pivot: q.Get("pivot"),
	}
}

// GeneratePagingURLs constructs a PagingURLs struct from the given base URL,
// limit, pivot value, and whether more results exist beyond the current page.
//
// The pivot parameter should be the cursor value for navigating to the next/previous
// page — typically the last item's ID from the current page results, NOT the incoming
// request's pivot. The caller is responsible for determining the appropriate pivot
// value after fetching results.
func GeneratePagingURLs(baseURL string, limit int, pivot string, hasMore bool) PagingURLs {
	limitStr := strconv.Itoa(limit)

	buildURL := func(page, pivotVal string) string {
		u, err := url.Parse(baseURL)
		if err != nil {
			return ""
		}
		q := u.Query()
		q.Set("page", page)
		q.Set("limit", limitStr)
		if pivotVal != "" {
			q.Set("pivot", pivotVal)
		}
		u.RawQuery = q.Encode()
		return u.String()
	}

	paging := PagingURLs{
		First: buildURL("first", ""),
		Last:  buildURL("last", ""),
	}

	if hasMore && pivot != "" {
		paging.Next = buildURL("next", pivot)
	}

	if pivot != "" {
		// "prev" (not "previous") is the correct query parameter value per the Mailgun API.
		paging.Previous = buildURL("prev", pivot)
	}

	return paging
}

// ---------------------------------------------------------------------------
// Token-based cursor encode/decode (used by analytics endpoints)
// ---------------------------------------------------------------------------

// EncodeCursor serialises the given key-value data into an opaque base64 token.
func EncodeCursor(data map[string]string) string {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(jsonBytes)
}

// DecodeCursor decodes an opaque base64 cursor token back into a key-value map.
// An empty token string returns an empty map with no error.
func DecodeCursor(token string) (map[string]string, error) {
	if token == "" {
		return map[string]string{}, nil
	}

	jsonBytes, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		// Try URL-safe encoding as fallback.
		jsonBytes, err = base64.URLEncoding.DecodeString(token)
		if err != nil {
			return nil, fmt.Errorf("invalid cursor: bad base64: %w", err)
		}
	}

	var result map[string]string
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		return nil, fmt.Errorf("invalid cursor: bad JSON: %w", err)
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// Skip/limit offset pagination
// ---------------------------------------------------------------------------

// ParseSkipLimitParams extracts "skip" and "limit" query parameters from the
// HTTP request. Missing or non-numeric values fall back to skip=0 and the
// provided defaultLimit. The limit is clamped to maxLimit.
func ParseSkipLimitParams(r *http.Request, defaultLimit, maxLimit int) SkipLimitParams {
	q := r.URL.Query()

	skip := 0
	if v := q.Get("skip"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			skip = parsed
		}
	}

	limit := defaultLimit
	if v := q.Get("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	return SkipLimitParams{
		Skip:  skip,
		Limit: limit,
	}
}

// ---------------------------------------------------------------------------
// Token-based pagination (v1 analytics)
// ---------------------------------------------------------------------------

// ParseTokenParams extracts "cursor" and "limit" query parameters from the
// HTTP request for token-based pagination. Invalid or missing limits fall back
// to defaultLimit. The limit is clamped to maxLimit.
func ParseTokenParams(r *http.Request, defaultLimit, maxLimit int) TokenParams {
	q := r.URL.Query()

	limit := defaultLimit
	if v := q.Get("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	return TokenParams{
		Cursor: q.Get("cursor"),
		Limit:  limit,
	}
}

// BuildTokenPaginationMeta constructs a TokenPaginationMeta value from the
// given parameters.
func BuildTokenPaginationMeta(cursor string, total int64, sort string, limit int) TokenPaginationMeta {
	return TokenPaginationMeta{
		Cursor: cursor,
		Total:  total,
		Sort:   sort,
		Limit:  limit,
	}
}
