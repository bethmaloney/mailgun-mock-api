package tag_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bethmaloney/mailgun-mock-api/internal/domain"
	"github.com/bethmaloney/mailgun-mock-api/internal/event"
	"github.com/bethmaloney/mailgun-mock-api/internal/message"
	"github.com/bethmaloney/mailgun-mock-api/internal/mock"
	"github.com/bethmaloney/mailgun-mock-api/internal/tag"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// V1 / Singular-Path Test Helpers
// ---------------------------------------------------------------------------

// setupV1Router creates a router with existing v3 routes, the new singular
// /v3/{domain_name}/tag routes, and the v1 analytics endpoints registered.
func setupV1Router(db *gorm.DB, cfg *mock.MockConfig) http.Handler {
	dh := domain.NewHandlers(db, cfg)
	tgh := tag.NewHandlers(db)
	eh := event.NewHandlers(db, cfg)
	mh := message.NewHandlers(db, cfg)
	r := chi.NewRouter()

	// Domain creation
	r.Route("/v4/domains", func(r chi.Router) {
		r.Post("/", dh.CreateDomain)
	})

	// Existing plural tag CRUD + stats routes
	r.Route("/v3/{domain_name}/tags", func(r chi.Router) {
		r.Get("/", tgh.ListTags)
		r.Get("/{tag}", tgh.GetTag)
		r.Put("/{tag}", tgh.UpdateTag)
		r.Delete("/{tag}", tgh.DeleteTag)

		r.Get("/{tag}/stats", tgh.GetTagStats)
		r.Get("/{tag}/stats/aggregates/countries", tgh.GetTagStatsCountries)
		r.Get("/{tag}/stats/aggregates/providers", tgh.GetTagStatsProviders)
		r.Get("/{tag}/stats/aggregates/devices", tgh.GetTagStatsDevices)
	})

	// Tag limits route
	r.Get("/v3/domains/{domain_name}/limits/tag", tgh.GetTagLimits)

	// Domain-level stats
	r.Get("/v3/{domain_name}/stats/total", tgh.GetDomainStats)

	// Singular tag paths (OpenAPI spec style — new handlers)
	r.Route("/v3/{domain_name}/tag", func(r chi.Router) {
		r.Get("/", tgh.GetTagByQuery)
		r.Get("/stats", tgh.GetTagStatsByQuery)
		r.Get("/stats/aggregates/countries", tgh.GetTagStatsCountriesByQuery)
		r.Get("/stats/aggregates/providers", tgh.GetTagStatsProvidersByQuery)
		r.Get("/stats/aggregates/devices", tgh.GetTagStatsDevicesByQuery)
	})

	// v1 Analytics Tags API (account-level, not domain-scoped)
	r.Route("/v1/analytics/tags", func(r chi.Router) {
		r.Post("/", tgh.V1ListTags)
		r.Put("/", tgh.V1UpdateTag)
		r.Delete("/", tgh.V1DeleteTag)
		r.Get("/limits", tgh.V1GetTagLimits)
	})

	// Message sending (for tag auto-creation)
	r.Route("/v3/{domain_name}/messages", func(r chi.Router) {
		r.Post("/", mh.SendMessage)
	})

	// Suppress linter for eh
	_ = eh

	return r
}

// setupV1 sets up the v1 test router and creates the test domain.
func setupV1(t *testing.T) http.Handler {
	t.Helper()
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupV1Router(db, cfg)

	// Create the test domain
	rec := doRequest(t, router, "POST", "/v4/domains", map[string]string{
		"name": testDomain,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to create test domain: %s", rec.Body.String())
	}

	return router
}

// doJSONRequest sends a request with a JSON body and returns the response recorder.
func doJSONRequest(t *testing.T, router http.Handler, method, url string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("failed to encode request body: %v", err)
		}
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, url, &buf)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	return rec
}

// =========================================================================
// Feature 1: Singular Path Discrepancy Tests
// =========================================================================

// ---------------------------------------------------------------------------
// 1. GET /v3/{domain}/tag?tag=tagname — Success
// ---------------------------------------------------------------------------

func TestSingularPath_GetTag(t *testing.T) {
	router := setupV1(t)

	// Create a tag via message send
	rec := sendMessage(t, router, "newsletter")
	assertStatus(t, rec, http.StatusOK)

	// GET via the singular path with query parameter
	singularURL := fmt.Sprintf("/v3/%s/tag?tag=newsletter", testDomain)
	rec = doGET(t, router, singularURL)
	assertStatus(t, rec, http.StatusOK)

	var singularResp map[string]interface{}
	decodeJSON(t, rec, &singularResp)

	// Verify the tag name matches
	if singularResp["tag"] != "newsletter" {
		t.Errorf("expected tag %q, got %v", "newsletter", singularResp["tag"])
	}

	// GET via the plural path for comparison
	pluralURL := fmt.Sprintf("/v3/%s/tags/newsletter", testDomain)
	rec = doGET(t, router, pluralURL)
	assertStatus(t, rec, http.StatusOK)

	var pluralResp map[string]interface{}
	decodeJSON(t, rec, &pluralResp)

	// Verify both responses return the same tag name
	if singularResp["tag"] != pluralResp["tag"] {
		t.Errorf("tag mismatch: singular=%v, plural=%v", singularResp["tag"], pluralResp["tag"])
	}

	// Verify both responses include first-seen
	if singularResp["first-seen"] == nil {
		t.Error("singular path response missing 'first-seen' field")
	}
	if singularResp["description"] == nil && pluralResp["description"] != nil {
		t.Error("singular path response missing 'description' field present in plural path response")
	}
}

// ---------------------------------------------------------------------------
// 2. GET /v3/{domain}/tag?tag=nonexistent — Not Found
// ---------------------------------------------------------------------------

func TestSingularPath_GetTag_NotFound(t *testing.T) {
	router := setupV1(t)

	url := fmt.Sprintf("/v3/%s/tag?tag=nonexistent", testDomain)
	rec := doGET(t, router, url)
	assertStatus(t, rec, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// 3. GET /v3/{domain}/tag (no ?tag= query param) — Bad Request
// ---------------------------------------------------------------------------

func TestSingularPath_GetTag_MissingQueryParam(t *testing.T) {
	router := setupV1(t)

	url := fmt.Sprintf("/v3/%s/tag", testDomain)
	rec := doGET(t, router, url)
	assertStatus(t, rec, http.StatusBadRequest)
}

// ---------------------------------------------------------------------------
// 4. GET /v3/{domain}/tag/stats?tag=tagname&event=accepted — Tag Stats
// ---------------------------------------------------------------------------

func TestSingularPath_GetTagStats(t *testing.T) {
	router := setupV1(t)

	// Create a tag via message send
	rec := sendMessage(t, router, "statstag")
	assertStatus(t, rec, http.StatusOK)

	// GET stats via the singular path
	singularURL := fmt.Sprintf("/v3/%s/tag/stats?tag=statstag&event=accepted", testDomain)
	rec = doGET(t, router, singularURL)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	// Verify tag name in response
	if body["tag"] != "statstag" {
		t.Errorf("expected tag %q, got %v", "statstag", body["tag"])
	}

	// Verify stats array exists
	stats, ok := body["stats"].([]interface{})
	if !ok {
		t.Fatalf("expected stats array, got %T: %v", body["stats"], body["stats"])
	}
	if len(stats) == 0 {
		t.Fatal("expected at least one stats time bucket")
	}

	// Verify resolution field
	if _, ok := body["resolution"]; !ok {
		t.Error("expected 'resolution' field in response")
	}
}

// ---------------------------------------------------------------------------
// 5. GET /v3/{domain}/tag/stats/aggregates/countries?tag=tagname
// ---------------------------------------------------------------------------

func TestSingularPath_GetTagStatsCountries(t *testing.T) {
	router := setupV1(t)

	rec := sendMessage(t, router, "countries-singular")
	assertStatus(t, rec, http.StatusOK)

	url := fmt.Sprintf("/v3/%s/tag/stats/aggregates/countries?tag=countries-singular", testDomain)
	rec = doGET(t, router, url)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	if body["tag"] != "countries-singular" {
		t.Errorf("expected tag %q, got %v", "countries-singular", body["tag"])
	}

	countries, ok := body["countries"]
	if !ok {
		t.Fatal("expected 'countries' field in response")
	}
	countriesMap, ok := countries.(map[string]interface{})
	if !ok {
		t.Fatalf("expected countries to be a map, got %T", countries)
	}
	if len(countriesMap) != 0 {
		t.Errorf("expected empty countries map, got %d entries", len(countriesMap))
	}
}

// ---------------------------------------------------------------------------
// 6. GET /v3/{domain}/tag/stats/aggregates/providers?tag=tagname
// ---------------------------------------------------------------------------

func TestSingularPath_GetTagStatsProviders(t *testing.T) {
	router := setupV1(t)

	rec := sendMessage(t, router, "providers-singular")
	assertStatus(t, rec, http.StatusOK)

	url := fmt.Sprintf("/v3/%s/tag/stats/aggregates/providers?tag=providers-singular", testDomain)
	rec = doGET(t, router, url)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	if body["tag"] != "providers-singular" {
		t.Errorf("expected tag %q, got %v", "providers-singular", body["tag"])
	}

	providers, ok := body["providers"]
	if !ok {
		t.Fatal("expected 'providers' field in response")
	}
	providersMap, ok := providers.(map[string]interface{})
	if !ok {
		t.Fatalf("expected providers to be a map, got %T", providers)
	}
	if len(providersMap) != 0 {
		t.Errorf("expected empty providers map, got %d entries", len(providersMap))
	}
}

// ---------------------------------------------------------------------------
// 7. GET /v3/{domain}/tag/stats/aggregates/devices?tag=tagname
// ---------------------------------------------------------------------------

func TestSingularPath_GetTagStatsDevices(t *testing.T) {
	router := setupV1(t)

	rec := sendMessage(t, router, "devices-singular")
	assertStatus(t, rec, http.StatusOK)

	url := fmt.Sprintf("/v3/%s/tag/stats/aggregates/devices?tag=devices-singular", testDomain)
	rec = doGET(t, router, url)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	if body["tag"] != "devices-singular" {
		t.Errorf("expected tag %q, got %v", "devices-singular", body["tag"])
	}

	devices, ok := body["devices"]
	if !ok {
		t.Fatal("expected 'devices' field in response")
	}
	devicesMap, ok := devices.(map[string]interface{})
	if !ok {
		t.Fatalf("expected devices to be a map, got %T", devices)
	}
	if len(devicesMap) != 0 {
		t.Errorf("expected empty devices map, got %d entries", len(devicesMap))
	}
}

// =========================================================================
// Feature 2: v1 Analytics Tags API Tests
// =========================================================================

// ---------------------------------------------------------------------------
// 8. POST /v1/analytics/tags — Empty (no tags exist)
// ---------------------------------------------------------------------------

func TestV1ListTags_Empty(t *testing.T) {
	router := setupV1(t)

	rec := doJSONRequest(t, router, "POST", "/v1/analytics/tags", map[string]interface{}{})
	assertStatus(t, rec, http.StatusOK)

	var body struct {
		Items      []interface{}          `json:"items"`
		Pagination map[string]interface{} `json:"pagination"`
	}
	decodeJSON(t, rec, &body)

	if len(body.Items) != 0 {
		t.Errorf("expected 0 items, got %d", len(body.Items))
	}

	if body.Pagination == nil {
		t.Error("expected pagination object in response")
	}
}

// ---------------------------------------------------------------------------
// 9. POST /v1/analytics/tags — With Tags
// ---------------------------------------------------------------------------

func TestV1ListTags_WithTags(t *testing.T) {
	router := setupV1(t)

	// Create tags via message sends
	for _, tagName := range []string{"newsletter", "promo"} {
		rec := sendMessage(t, router, tagName)
		assertStatus(t, rec, http.StatusOK)
	}

	rec := doJSONRequest(t, router, "POST", "/v1/analytics/tags", map[string]interface{}{})
	assertStatus(t, rec, http.StatusOK)

	var body struct {
		Items      []map[string]interface{} `json:"items"`
		Pagination map[string]interface{}   `json:"pagination"`
	}
	decodeJSON(t, rec, &body)

	if len(body.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(body.Items))
	}

	// Verify items have snake_case fields (first_seen, last_seen)
	for _, item := range body.Items {
		tagName, ok := item["tag"].(string)
		if !ok || tagName == "" {
			t.Errorf("expected non-empty 'tag' field, got: %v", item["tag"])
		}

		if _, ok := item["first_seen"]; !ok {
			t.Errorf("expected 'first_seen' (snake_case) field for tag %q", tagName)
		}
		if _, ok := item["last_seen"]; !ok {
			t.Errorf("expected 'last_seen' (snake_case) field for tag %q", tagName)
		}
	}
}

// ---------------------------------------------------------------------------
// 10. POST /v1/analytics/tags — Filter by Tag Name
// ---------------------------------------------------------------------------

func TestV1ListTags_FilterByTag(t *testing.T) {
	router := setupV1(t)

	// Create multiple tags
	for _, tagName := range []string{"newsletter", "promo", "alerts"} {
		rec := sendMessage(t, router, tagName)
		assertStatus(t, rec, http.StatusOK)
	}

	// Filter to only "newsletter"
	rec := doJSONRequest(t, router, "POST", "/v1/analytics/tags", map[string]interface{}{
		"tag": "newsletter",
	})
	assertStatus(t, rec, http.StatusOK)

	var body struct {
		Items []map[string]interface{} `json:"items"`
	}
	decodeJSON(t, rec, &body)

	if len(body.Items) != 1 {
		t.Fatalf("expected 1 item matching filter, got %d", len(body.Items))
	}

	got, _ := body.Items[0]["tag"].(string)
	if got != "newsletter" {
		t.Errorf("expected tag %q, got %q", "newsletter", got)
	}
}

// ---------------------------------------------------------------------------
// 11. POST /v1/analytics/tags — Pagination
// ---------------------------------------------------------------------------

func TestV1ListTags_Pagination(t *testing.T) {
	router := setupV1(t)

	// Create several tags
	for _, tagName := range []string{"alpha", "bravo", "charlie", "delta", "echo"} {
		rec := sendMessage(t, router, tagName)
		assertStatus(t, rec, http.StatusOK)
	}

	// Request with limit=2
	rec := doJSONRequest(t, router, "POST", "/v1/analytics/tags", map[string]interface{}{
		"pagination": map[string]interface{}{
			"limit":         2,
			"include_total": true,
		},
	})
	assertStatus(t, rec, http.StatusOK)

	var body struct {
		Items      []map[string]interface{} `json:"items"`
		Pagination map[string]interface{}   `json:"pagination"`
	}
	decodeJSON(t, rec, &body)

	if len(body.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(body.Items))
	}

	// Verify pagination response fields
	if body.Pagination == nil {
		t.Fatal("expected pagination object in response")
	}

	limitVal, ok := body.Pagination["limit"].(float64)
	if !ok {
		t.Fatalf("expected 'limit' field in pagination, got: %v", body.Pagination["limit"])
	}
	if limitVal != 2 {
		t.Errorf("expected pagination limit 2, got %v", limitVal)
	}

	// Verify total is present and correct (5 tags created)
	totalVal, ok := body.Pagination["total"].(float64)
	if !ok {
		t.Fatalf("expected 'total' field in pagination, got: %v", body.Pagination["total"])
	}
	if totalVal != 5 {
		t.Errorf("expected pagination total 5, got %v", totalVal)
	}
}

// ---------------------------------------------------------------------------
// 12. POST /v1/analytics/tags — Snake Case Fields
// ---------------------------------------------------------------------------

func TestV1ListTags_SnakeCaseFields(t *testing.T) {
	router := setupV1(t)

	rec := sendMessage(t, router, "snake-check")
	assertStatus(t, rec, http.StatusOK)

	rec = doJSONRequest(t, router, "POST", "/v1/analytics/tags", map[string]interface{}{})
	assertStatus(t, rec, http.StatusOK)

	// Check the raw JSON for snake_case keys
	raw := rec.Body.String()

	// Must have snake_case first_seen and last_seen
	if !strings.Contains(raw, `"first_seen"`) {
		t.Errorf("response should contain 'first_seen' (snake_case), got: %s", raw)
	}
	if !strings.Contains(raw, `"last_seen"`) {
		t.Errorf("response should contain 'last_seen' (snake_case), got: %s", raw)
	}

	// Must NOT have hyphenated first-seen / last-seen (v1 uses snake_case)
	if strings.Contains(raw, `"first-seen"`) {
		t.Errorf("v1 response should NOT contain hyphenated 'first-seen': %s", raw)
	}
	if strings.Contains(raw, `"last-seen"`) {
		t.Errorf("v1 response should NOT contain hyphenated 'last-seen': %s", raw)
	}

	// Verify additional v1-specific fields exist in items
	var body struct {
		Items []map[string]interface{} `json:"items"`
	}
	decodeJSON(t, rec, &body)

	if len(body.Items) == 0 {
		t.Fatal("expected at least one item")
	}

	item := body.Items[0]

	// Verify account_id, parent_account_id, account_name, and metrics fields
	for _, field := range []string{"account_id", "parent_account_id", "account_name", "metrics"} {
		if _, ok := item[field]; !ok {
			t.Errorf("expected field %q in v1 tag item", field)
		}
	}
}

// ---------------------------------------------------------------------------
// 13. PUT /v1/analytics/tags — Update Tag Success
// ---------------------------------------------------------------------------

func TestV1UpdateTag_Success(t *testing.T) {
	router := setupV1(t)

	// Create a tag via message send
	rec := sendMessage(t, router, "updatable")
	assertStatus(t, rec, http.StatusOK)

	// Update via v1 endpoint
	rec = doJSONRequest(t, router, "PUT", "/v1/analytics/tags", map[string]interface{}{
		"tag":         "updatable",
		"description": "Updated via v1",
	})
	assertStatus(t, rec, http.StatusOK)
	assertMessage(t, rec, "Tag updated")

	// Verify description was updated via the v3 GET endpoint
	rec = doGET(t, router, fmt.Sprintf("/v3/%s/tags/updatable", testDomain))
	assertStatus(t, rec, http.StatusOK)

	var tagResp map[string]interface{}
	decodeJSON(t, rec, &tagResp)

	desc, _ := tagResp["description"].(string)
	if desc != "Updated via v1" {
		t.Errorf("expected description %q, got %q", "Updated via v1", desc)
	}
}

// ---------------------------------------------------------------------------
// 14. PUT /v1/analytics/tags — Not Found
// ---------------------------------------------------------------------------

func TestV1UpdateTag_NotFound(t *testing.T) {
	router := setupV1(t)

	rec := doJSONRequest(t, router, "PUT", "/v1/analytics/tags", map[string]interface{}{
		"tag":         "nonexistent",
		"description": "should fail",
	})
	assertStatus(t, rec, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// 15. DELETE /v1/analytics/tags — Delete Tag Success
// ---------------------------------------------------------------------------

func TestV1DeleteTag_Success(t *testing.T) {
	router := setupV1(t)

	// Create a tag via message send
	rec := sendMessage(t, router, "deletable")
	assertStatus(t, rec, http.StatusOK)

	// Delete via v1 endpoint
	rec = doJSONRequest(t, router, "DELETE", "/v1/analytics/tags", map[string]interface{}{
		"tag": "deletable",
	})
	assertStatus(t, rec, http.StatusOK)
	assertMessage(t, rec, "Tag has been removed")

	// Verify tag is gone via v3 GET endpoint
	rec = doGET(t, router, fmt.Sprintf("/v3/%s/tags/deletable", testDomain))
	assertStatus(t, rec, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// 16. DELETE /v1/analytics/tags — Not Found
// ---------------------------------------------------------------------------

func TestV1DeleteTag_NotFound(t *testing.T) {
	router := setupV1(t)

	rec := doJSONRequest(t, router, "DELETE", "/v1/analytics/tags", map[string]interface{}{
		"tag": "nonexistent",
	})
	assertStatus(t, rec, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// 17. GET /v1/analytics/tags/limits — Empty (no tags)
// ---------------------------------------------------------------------------

func TestV1GetTagLimits_Empty(t *testing.T) {
	router := setupV1(t)

	rec := doGET(t, router, "/v1/analytics/tags/limits")
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	limitVal, ok := body["limit"].(float64)
	if !ok {
		t.Fatalf("expected 'limit' field as number, got: %v", body["limit"])
	}
	if limitVal != 5000 {
		t.Errorf("expected limit 5000, got %v", limitVal)
	}

	countVal, ok := body["count"].(float64)
	if !ok {
		t.Fatalf("expected 'count' field as number, got: %v", body["count"])
	}
	if countVal != 0 {
		t.Errorf("expected count 0, got %v", countVal)
	}

	limitReached, ok := body["limit_reached"].(bool)
	if !ok {
		t.Fatalf("expected 'limit_reached' field as bool, got: %v (%T)", body["limit_reached"], body["limit_reached"])
	}
	if limitReached {
		t.Error("expected limit_reached to be false")
	}
}

// ---------------------------------------------------------------------------
// 18. GET /v1/analytics/tags/limits — With Tags
// ---------------------------------------------------------------------------

func TestV1GetTagLimits_WithTags(t *testing.T) {
	router := setupV1(t)

	// Create some tags
	for _, tagName := range []string{"tag1", "tag2", "tag3"} {
		rec := sendMessage(t, router, tagName)
		assertStatus(t, rec, http.StatusOK)
	}

	rec := doGET(t, router, "/v1/analytics/tags/limits")
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	limitVal, ok := body["limit"].(float64)
	if !ok {
		t.Fatalf("expected 'limit' field as number, got: %v", body["limit"])
	}
	if limitVal != 5000 {
		t.Errorf("expected limit 5000, got %v", limitVal)
	}

	countVal, ok := body["count"].(float64)
	if !ok {
		t.Fatalf("expected 'count' field as number, got: %v", body["count"])
	}
	if countVal != 3 {
		t.Errorf("expected count 3, got %v", countVal)
	}

	limitReached, ok := body["limit_reached"].(bool)
	if !ok {
		t.Fatalf("expected 'limit_reached' field as bool, got: %v (%T)", body["limit_reached"], body["limit_reached"])
	}
	if limitReached {
		t.Error("expected limit_reached to be false")
	}
}
