package metrics_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bethmaloney/mailgun-mock-api/internal/domain"
	"github.com/bethmaloney/mailgun-mock-api/internal/event"
	"github.com/bethmaloney/mailgun-mock-api/internal/message"
	"github.com/bethmaloney/mailgun-mock-api/internal/metrics"
	"github.com/bethmaloney/mailgun-mock-api/internal/mock"
	"github.com/bethmaloney/mailgun-mock-api/internal/tag"
	"github.com/go-chi/chi/v5"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const (
	testDomain1 = "test1.example.com"
	testDomain2 = "test2.example.com"
)

// ---------------------------------------------------------------------------
// Test Helpers
// ---------------------------------------------------------------------------

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}
	if err := db.AutoMigrate(
		&domain.Domain{}, &domain.DNSRecord{},
		&event.Event{},
		&message.StoredMessage{},
		&tag.Tag{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

func defaultConfig() *mock.MockConfig {
	return &mock.MockConfig{
		DomainBehavior: mock.DomainBehaviorConfig{
			DomainAutoVerify: true,
			SandboxDomain:    "sandbox123.mailgun.org",
		},
		EventGeneration: mock.EventGenerationConfig{
			AutoDeliver:               true,
			DeliveryDelayMs:           0,
			DefaultDeliveryStatusCode: 250,
			AutoFailRate:              0.0,
		},
	}
}

func setupRouter(db *gorm.DB, cfg *mock.MockConfig) http.Handler {
	domain.ResetForTests(db)
	event.ResetForTests(db)
	dh := domain.NewHandlers(db, cfg)
	eh := event.NewHandlers(db, cfg)
	mh := message.NewHandlers(db, cfg)
	metricsH := metrics.NewHandlers(db)

	r := chi.NewRouter()

	// Domain creation
	r.Route("/v4/domains", func(r chi.Router) {
		r.Post("/", dh.CreateDomain)
	})

	// Message sending (for event generation)
	r.Route("/v3/{domain_name}/messages", func(r chi.Router) {
		r.Post("/", mh.SendMessage)
	})

	// Account-level stats (v3)
	r.Get("/v3/stats/total", metricsH.GetAccountStats)
	r.Get("/v3/stats/filter", metricsH.GetFilteredStats)
	r.Get("/v3/stats/total/domains", metricsH.GetDomainStatsSnapshot)

	// Domain aggregate stubs (v3)
	r.Get("/v3/{domain_name}/aggregates/providers", metricsH.GetDomainAggregateProviders)
	r.Get("/v3/{domain_name}/aggregates/devices", metricsH.GetDomainAggregateDevices)
	r.Get("/v3/{domain_name}/aggregates/countries", metricsH.GetDomainAggregateCountries)

	// v1 analytics metrics
	r.Post("/v1/analytics/metrics", metricsH.QueryMetrics)
	r.Post("/v1/analytics/usage/metrics", metricsH.QueryUsageMetrics)

	// v2 bounce classification
	r.Post("/v2/bounce-classification/metrics", metricsH.QueryBounceClassification)

	// Suppress linter for eh
	_ = eh

	return r
}

// setup creates a test router and the first test domain.
func setup(t *testing.T) http.Handler {
	t.Helper()
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	// Create the first test domain
	rec := doRequest(t, router, "POST", "/v4/domains", map[string]string{
		"name": testDomain1,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to create test domain %s: %s", testDomain1, rec.Body.String())
	}

	return router
}

// setupTwoDomains creates a test router with two domains pre-created.
func setupTwoDomains(t *testing.T) http.Handler {
	t.Helper()
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	// Create two test domains
	for _, d := range []string{testDomain1, testDomain2} {
		rec := doRequest(t, router, "POST", "/v4/domains", map[string]string{
			"name": d,
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("failed to create test domain %s: %s", d, rec.Body.String())
		}
	}

	return router
}

func newMultipartRequest(t *testing.T, method, url string, fields map[string]string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for key, val := range fields {
		if err := writer.WriteField(key, val); err != nil {
			t.Fatalf("failed to write field %q: %v", key, err)
		}
	}
	writer.Close()
	req := httptest.NewRequest(method, url, &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func doRequest(t *testing.T, router http.Handler, method, url string, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	var req *http.Request
	if fields != nil {
		req = newMultipartRequest(t, method, url, fields)
	} else {
		req = httptest.NewRequest(method, url, nil)
	}
	router.ServeHTTP(rec, req)
	return rec
}

func doJSONRequest(t *testing.T, router http.Handler, method, url string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		t.Fatalf("failed to encode JSON body: %v", err)
	}
	req := httptest.NewRequest(method, url, &buf)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	return rec
}

func doGET(t *testing.T, router http.Handler, url string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", url, nil)
	router.ServeHTTP(rec, req)
	return rec
}

func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder, dest interface{}) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dest); err != nil {
		t.Fatalf("failed to decode response (body=%q): %v", rec.Body.String(), err)
	}
}

func assertStatus(t *testing.T, rec *httptest.ResponseRecorder, expected int) {
	t.Helper()
	if rec.Code != expected {
		t.Errorf("expected status %d, got %d; body=%s", expected, rec.Code, rec.Body.String())
	}
}

func assertMessage(t *testing.T, rec *httptest.ResponseRecorder, expected string) {
	t.Helper()
	var body map[string]interface{}
	decodeJSON(t, rec, &body)
	msg, ok := body["message"].(string)
	if !ok {
		t.Fatalf("expected string 'message' field in response, got: %v", body)
	}
	if msg != expected {
		t.Errorf("expected message %q, got %q", expected, msg)
	}
}

// sendMessageToDomain sends a message to the specified domain and returns the
// response recorder.
func sendMessageToDomain(t *testing.T, router http.Handler, domainName string) *httptest.ResponseRecorder {
	t.Helper()
	return doRequest(t, router, "POST", fmt.Sprintf("/v3/%s/messages", domainName), map[string]string{
		"from":    fmt.Sprintf("sender@%s", domainName),
		"to":      "recipient@example.com",
		"subject": "Test message",
		"text":    "Hello, world!",
	})
}

// sendMessage sends a message to testDomain1 with no tag.
func sendMessage(t *testing.T, router http.Handler) *httptest.ResponseRecorder {
	t.Helper()
	return sendMessageToDomain(t, router, testDomain1)
}

// =========================================================================
// 1. Account Stats (v3) — Cross-domain aggregation
// =========================================================================

func TestAccountStats_CrossDomainAggregation(t *testing.T) {
	router := setupTwoDomains(t)

	// Send 2 messages to domain1 and 3 messages to domain2
	for i := 0; i < 2; i++ {
		rec := sendMessageToDomain(t, router, testDomain1)
		assertStatus(t, rec, http.StatusOK)
	}
	for i := 0; i < 3; i++ {
		rec := sendMessageToDomain(t, router, testDomain2)
		assertStatus(t, rec, http.StatusOK)
	}

	// GET /v3/stats/total?event=delivered
	rec := doGET(t, router, "/v3/stats/total?event=delivered")
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	// Verify stats array exists
	stats, ok := body["stats"].([]interface{})
	if !ok {
		t.Fatalf("expected stats array, got %T: %v", body["stats"], body["stats"])
	}
	if len(stats) == 0 {
		t.Fatal("expected at least one stats time bucket, got 0")
	}

	// Sum delivered totals across all time buckets
	var totalDelivered float64
	for _, bucket := range stats {
		b, ok := bucket.(map[string]interface{})
		if !ok {
			continue
		}
		if delivered, ok := b["delivered"].(map[string]interface{}); ok {
			if total, ok := delivered["total"].(float64); ok {
				totalDelivered += total
			}
		}
	}

	// 5 messages total (2 + 3) should produce 5 delivered events
	if totalDelivered != 5 {
		t.Errorf("expected 5 total delivered events across both domains, got %v", totalDelivered)
	}
}

// =========================================================================
// 2. Account Stats with Multiple Events
// =========================================================================

func TestAccountStats_MultipleEvents(t *testing.T) {
	router := setupTwoDomains(t)

	// Send messages to generate both accepted and delivered events
	for i := 0; i < 3; i++ {
		rec := sendMessageToDomain(t, router, testDomain1)
		assertStatus(t, rec, http.StatusOK)
	}

	// GET /v3/stats/total?event=delivered&event=accepted
	rec := doGET(t, router, "/v3/stats/total?event=delivered&event=accepted")
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	stats, ok := body["stats"].([]interface{})
	if !ok || len(stats) == 0 {
		t.Fatal("expected non-empty stats array")
	}

	// Sum totals across all time buckets
	var totalAccepted, totalDelivered float64
	for _, bucket := range stats {
		b, ok := bucket.(map[string]interface{})
		if !ok {
			continue
		}
		if accepted, ok := b["accepted"].(map[string]interface{}); ok {
			if total, ok := accepted["total"].(float64); ok {
				totalAccepted += total
			}
		}
		if delivered, ok := b["delivered"].(map[string]interface{}); ok {
			if total, ok := delivered["total"].(float64); ok {
				totalDelivered += total
			}
		}
	}

	// 3 messages: 3 accepted + 3 delivered
	if totalAccepted != 3 {
		t.Errorf("expected 3 total accepted events, got %v", totalAccepted)
	}
	if totalDelivered != 3 {
		t.Errorf("expected 3 total delivered events, got %v", totalDelivered)
	}
}

// =========================================================================
// 3. Account Stats with Resolution = "hour"
// =========================================================================

func TestAccountStats_ResolutionHour(t *testing.T) {
	router := setup(t)

	rec := sendMessage(t, router)
	assertStatus(t, rec, http.StatusOK)

	rec = doGET(t, router, "/v3/stats/total?event=accepted&resolution=hour")
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	// Verify resolution is "hour"
	if body["resolution"] != "hour" {
		t.Errorf("expected resolution %q, got %v", "hour", body["resolution"])
	}

	// Verify stats array exists
	stats, ok := body["stats"].([]interface{})
	if !ok || len(stats) == 0 {
		t.Fatal("expected non-empty stats array")
	}

	// Verify time bucket format (RFC 2822)
	firstBucket := stats[0].(map[string]interface{})
	timeStr, ok := firstBucket["time"].(string)
	if !ok || timeStr == "" {
		t.Fatal("expected non-empty 'time' field in stats bucket")
	}

	// RFC 2822 time should be parseable
	_, err := time.Parse(time.RFC1123, timeStr)
	if err != nil {
		_, err = time.Parse(time.RFC1123Z, timeStr)
		if err != nil {
			_, err = time.Parse("Mon, 02 Jan 2006 15:04:05 MST", timeStr)
			if err != nil {
				t.Errorf("time bucket %q is not a valid RFC 2822 timestamp: %v", timeStr, err)
			}
		}
	}
}

// =========================================================================
// 4. Account Stats with Date Range
// =========================================================================

func TestAccountStats_DateRange(t *testing.T) {
	router := setup(t)

	rec := sendMessage(t, router)
	assertStatus(t, rec, http.StatusOK)

	now := time.Now().UTC()
	start := now.Add(-24 * time.Hour)
	end := now.Add(24 * time.Hour)

	// Use Unix epoch floats for start/end
	startStr := fmt.Sprintf("%.6f", float64(start.UnixMicro())/1e6)
	endStr := fmt.Sprintf("%.6f", float64(end.UnixMicro())/1e6)

	url := fmt.Sprintf("/v3/stats/total?event=accepted&start=%s&end=%s", startStr, endStr)
	rec = doGET(t, router, url)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	// Verify start and end fields are present in the response
	if _, ok := body["start"]; !ok {
		t.Error("expected 'start' field in response")
	}
	if _, ok := body["end"]; !ok {
		t.Error("expected 'end' field in response")
	}

	// Verify stats are present
	stats, ok := body["stats"].([]interface{})
	if !ok {
		t.Fatal("expected stats array")
	}
	if len(stats) == 0 {
		t.Error("expected at least one stats bucket for the given time range")
	}

	// Sum accepted totals — should include our message
	var totalAccepted float64
	for _, bucket := range stats {
		b, ok := bucket.(map[string]interface{})
		if !ok {
			continue
		}
		if accepted, ok := b["accepted"].(map[string]interface{}); ok {
			if total, ok := accepted["total"].(float64); ok {
				totalAccepted += total
			}
		}
	}
	if totalAccepted != 1 {
		t.Errorf("expected 1 accepted event in date range, got %v", totalAccepted)
	}
}

// =========================================================================
// 5. Account Stats — Missing Event Param (400)
// =========================================================================

func TestAccountStats_MissingEventParam(t *testing.T) {
	router := setup(t)

	// GET /v3/stats/total without event parameter
	rec := doGET(t, router, "/v3/stats/total")
	assertStatus(t, rec, http.StatusBadRequest)
	assertMessage(t, rec, "event is required")
}

// =========================================================================
// 6. Account Stats — Invalid Event Type (400)
// =========================================================================

func TestAccountStats_InvalidEventType(t *testing.T) {
	router := setup(t)

	rec := doGET(t, router, "/v3/stats/total?event=bogus")
	assertStatus(t, rec, http.StatusBadRequest)
	assertMessage(t, rec, "invalid event type")
}

// =========================================================================
// 7. Account Stats — Invalid Resolution (400)
// =========================================================================

func TestAccountStats_InvalidResolution(t *testing.T) {
	router := setup(t)

	rec := doGET(t, router, "/v3/stats/total?event=accepted&resolution=minute")
	assertStatus(t, rec, http.StatusBadRequest)
	assertMessage(t, rec, "invalid resolution")
}

// =========================================================================
// 8. Account Stats — Response Shape
// =========================================================================

func TestAccountStats_ResponseShape(t *testing.T) {
	router := setup(t)

	rec := sendMessage(t, router)
	assertStatus(t, rec, http.StatusOK)

	rec = doGET(t, router, "/v3/stats/total?event=accepted&event=delivered&event=failed&event=opened&event=clicked&event=unsubscribed&event=complained&event=stored")
	assertStatus(t, rec, http.StatusOK)

	// Parse into raw JSON to verify nested structure
	var body map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify top-level fields
	for _, field := range []string{"start", "end", "resolution", "stats"} {
		if _, ok := body[field]; !ok {
			t.Errorf("expected top-level field %q in response", field)
		}
	}

	// Parse stats array
	var stats []map[string]json.RawMessage
	if err := json.Unmarshal(body["stats"], &stats); err != nil {
		t.Fatalf("failed to decode stats array: %v", err)
	}
	if len(stats) == 0 {
		t.Fatal("expected at least one stats time bucket")
	}

	bucket := stats[0]

	// Verify "time" field
	if _, ok := bucket["time"]; !ok {
		t.Error("expected 'time' field in stats bucket")
	}

	// Verify accepted has incoming/outgoing/total
	if rawAccepted, ok := bucket["accepted"]; ok {
		var accepted map[string]interface{}
		if err := json.Unmarshal(rawAccepted, &accepted); err != nil {
			t.Fatalf("failed to decode accepted: %v", err)
		}
		for _, key := range []string{"incoming", "outgoing", "total"} {
			if _, ok := accepted[key]; !ok {
				t.Errorf("expected accepted.%s field", key)
			}
		}
	} else {
		t.Error("expected 'accepted' field in stats bucket")
	}

	// Verify delivered has smtp/http/total
	if rawDelivered, ok := bucket["delivered"]; ok {
		var delivered map[string]interface{}
		if err := json.Unmarshal(rawDelivered, &delivered); err != nil {
			t.Fatalf("failed to decode delivered: %v", err)
		}
		for _, key := range []string{"smtp", "http", "total"} {
			if _, ok := delivered[key]; !ok {
				t.Errorf("expected delivered.%s field", key)
			}
		}
	} else {
		t.Error("expected 'delivered' field in stats bucket")
	}

	// Verify failed has temporary and permanent sub-fields
	if rawFailed, ok := bucket["failed"]; ok {
		var failed map[string]json.RawMessage
		if err := json.Unmarshal(rawFailed, &failed); err != nil {
			t.Fatalf("failed to decode failed: %v", err)
		}

		if rawTemp, ok := failed["temporary"]; ok {
			var temp map[string]interface{}
			if err := json.Unmarshal(rawTemp, &temp); err != nil {
				t.Fatalf("failed to decode failed.temporary: %v", err)
			}
			if _, ok := temp["espblock"]; !ok {
				t.Error("expected failed.temporary.espblock field")
			}
		} else {
			t.Error("expected 'temporary' field in failed")
		}

		if rawPerm, ok := failed["permanent"]; ok {
			var perm map[string]interface{}
			if err := json.Unmarshal(rawPerm, &perm); err != nil {
				t.Fatalf("failed to decode failed.permanent: %v", err)
			}
			for _, key := range []string{"suppress-bounce", "suppress-unsubscribe", "suppress-complaint", "bounce", "delayed-bounce", "total"} {
				if _, ok := perm[key]; !ok {
					t.Errorf("expected failed.permanent.%s field", key)
				}
			}
		} else {
			t.Error("expected 'permanent' field in failed")
		}
	} else {
		t.Error("expected 'failed' field in stats bucket")
	}

	// Verify simple total fields
	for _, eventType := range []string{"stored", "opened", "clicked", "unsubscribed", "complained"} {
		if rawField, ok := bucket[eventType]; ok {
			var field map[string]interface{}
			if err := json.Unmarshal(rawField, &field); err != nil {
				t.Fatalf("failed to decode %s: %v", eventType, err)
			}
			if _, ok := field["total"]; !ok {
				t.Errorf("expected %s.total field", eventType)
			}
		} else {
			t.Errorf("expected %q field in stats bucket", eventType)
		}
	}
}

// =========================================================================
// 9. Account Stats — Default Resolution is "day"
// =========================================================================

func TestAccountStats_DefaultResolutionDay(t *testing.T) {
	router := setup(t)

	rec := sendMessage(t, router)
	assertStatus(t, rec, http.StatusOK)

	// No resolution parameter — should default to "day"
	rec = doGET(t, router, "/v3/stats/total?event=accepted")
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	if body["resolution"] != "day" {
		t.Errorf("expected default resolution %q, got %v", "day", body["resolution"])
	}
}

// =========================================================================
// 10. Account Stats — Duration Parameter
// =========================================================================

func TestAccountStats_DurationParameter(t *testing.T) {
	router := setup(t)

	rec := sendMessage(t, router)
	assertStatus(t, rec, http.StatusOK)

	// Use duration=1m (1 month) to get a wide range
	rec = doGET(t, router, "/v3/stats/total?event=accepted&duration=1m")
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	// Verify start and end are present
	if _, ok := body["start"]; !ok {
		t.Error("expected 'start' field in response")
	}
	if _, ok := body["end"]; !ok {
		t.Error("expected 'end' field in response")
	}

	stats, ok := body["stats"].([]interface{})
	if !ok || len(stats) == 0 {
		t.Fatal("expected non-empty stats array")
	}
}

// =========================================================================
// 11. Filtered Stats — Same Shape as Account Stats
// =========================================================================

func TestFilteredStats_SameAsAccountStats(t *testing.T) {
	router := setup(t)

	rec := sendMessage(t, router)
	assertStatus(t, rec, http.StatusOK)

	// GET /v3/stats/filter?event=delivered
	rec = doGET(t, router, "/v3/stats/filter?event=delivered")
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	// Verify same response shape as /v3/stats/total
	for _, field := range []string{"start", "end", "resolution", "stats"} {
		if _, ok := body[field]; !ok {
			t.Errorf("expected field %q in filtered stats response", field)
		}
	}

	stats, ok := body["stats"].([]interface{})
	if !ok || len(stats) == 0 {
		t.Fatal("expected non-empty stats array in filtered stats")
	}
}

// =========================================================================
// 12. Filtered Stats — Missing Event Param (400)
// =========================================================================

func TestFilteredStats_MissingEventParam(t *testing.T) {
	router := setup(t)

	rec := doGET(t, router, "/v3/stats/filter")
	assertStatus(t, rec, http.StatusBadRequest)
	assertMessage(t, rec, "event is required")
}

// =========================================================================
// 13. Filtered Stats — Accepts Filter and Group Params
// =========================================================================

func TestFilteredStats_AcceptsFilterAndGroupParams(t *testing.T) {
	router := setup(t)

	rec := sendMessage(t, router)
	assertStatus(t, rec, http.StatusOK)

	// These params should be accepted but ignored in the mock
	rec = doGET(t, router, "/v3/stats/filter?event=delivered&filter=domain:test1.example.com&group=domain")
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	// Should still return valid response shape
	if _, ok := body["stats"]; !ok {
		t.Error("expected 'stats' field even with filter/group params")
	}
}

// =========================================================================
// 14. Per-Domain Stats Snapshot — GET /v3/stats/total/domains
// =========================================================================

func TestDomainStatsSnapshot(t *testing.T) {
	router := setupTwoDomains(t)

	// Send messages to both domains
	rec := sendMessageToDomain(t, router, testDomain1)
	assertStatus(t, rec, http.StatusOK)
	rec = sendMessageToDomain(t, router, testDomain2)
	assertStatus(t, rec, http.StatusOK)

	now := time.Now().UTC()
	timestampStr := fmt.Sprintf("%.6f", float64(now.UnixMicro())/1e6)

	// GET /v3/stats/total/domains?event=delivered&timestamp=<now>
	url := fmt.Sprintf("/v3/stats/total/domains?event=delivered&timestamp=%s", timestampStr)
	rec = doGET(t, router, url)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	// Verify response has stats
	if _, ok := body["stats"]; !ok {
		t.Error("expected 'stats' field in per-domain stats response")
	}
}

// =========================================================================
// 15. Domain Aggregate Providers — Stub
// =========================================================================

func TestDomainAggregateProviders(t *testing.T) {
	router := setup(t)

	url := fmt.Sprintf("/v3/%s/aggregates/providers", testDomain1)
	rec := doGET(t, router, url)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	// Verify items field is present and is an empty map
	items, ok := body["items"]
	if !ok {
		t.Fatal("expected 'items' field in response")
	}
	itemsMap, ok := items.(map[string]interface{})
	if !ok {
		t.Fatalf("expected items to be a map, got %T", items)
	}
	if len(itemsMap) != 0 {
		t.Errorf("expected empty items map, got %d entries", len(itemsMap))
	}
}

// =========================================================================
// 16. Domain Aggregate Devices — Stub
// =========================================================================

func TestDomainAggregateDevices(t *testing.T) {
	router := setup(t)

	url := fmt.Sprintf("/v3/%s/aggregates/devices", testDomain1)
	rec := doGET(t, router, url)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	// Verify items field is present and is an empty map
	items, ok := body["items"]
	if !ok {
		t.Fatal("expected 'items' field in response")
	}
	itemsMap, ok := items.(map[string]interface{})
	if !ok {
		t.Fatalf("expected items to be a map, got %T", items)
	}
	if len(itemsMap) != 0 {
		t.Errorf("expected empty items map, got %d entries", len(itemsMap))
	}
}

// =========================================================================
// 17. Domain Aggregate Countries — Stub
// =========================================================================

func TestDomainAggregateCountries(t *testing.T) {
	router := setup(t)

	url := fmt.Sprintf("/v3/%s/aggregates/countries", testDomain1)
	rec := doGET(t, router, url)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	// Verify items field is present and is an empty map
	items, ok := body["items"]
	if !ok {
		t.Fatal("expected 'items' field in response")
	}
	itemsMap, ok := items.(map[string]interface{})
	if !ok {
		t.Fatalf("expected items to be a map, got %T", items)
	}
	if len(itemsMap) != 0 {
		t.Errorf("expected empty items map, got %d entries", len(itemsMap))
	}
}

// =========================================================================
// 18. v1 Metrics Basic — Time-series items
// =========================================================================

func TestV1Metrics_Basic(t *testing.T) {
	router := setup(t)

	// Send messages to generate events
	for i := 0; i < 3; i++ {
		rec := sendMessage(t, router)
		assertStatus(t, rec, http.StatusOK)
	}

	now := time.Now().UTC()
	start := now.Add(-7 * 24 * time.Hour)

	reqBody := map[string]interface{}{
		"start":              start.Format("Mon, 02 Jan 2006 15:04:05 MST"),
		"end":                now.Format("Mon, 02 Jan 2006 15:04:05 MST"),
		"resolution":         "day",
		"dimensions":         []string{"time"},
		"metrics":            []string{"accepted_count", "delivered_count"},
		"include_aggregates": true,
	}

	rec := doJSONRequest(t, router, "POST", "/v1/analytics/metrics", reqBody)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	// Verify response shape
	for _, field := range []string{"start", "end", "resolution", "dimensions", "items"} {
		if _, ok := body[field]; !ok {
			t.Errorf("expected field %q in v1 metrics response", field)
		}
	}

	// Verify items is an array
	items, ok := body["items"].([]interface{})
	if !ok {
		t.Fatalf("expected items to be an array, got %T", body["items"])
	}

	// Verify each item has dimensions and metrics
	for i, item := range items {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			t.Fatalf("item[%d] is not a map", i)
			continue
		}
		if _, ok := itemMap["dimensions"]; !ok {
			t.Errorf("item[%d]: expected 'dimensions' field", i)
		}
		if _, ok := itemMap["metrics"]; !ok {
			t.Errorf("item[%d]: expected 'metrics' field", i)
		}
	}
}

// =========================================================================
// 19. v1 Metrics with Domain Filter
// =========================================================================

func TestV1Metrics_WithDomainFilter(t *testing.T) {
	router := setupTwoDomains(t)

	// Send messages to both domains
	for i := 0; i < 2; i++ {
		rec := sendMessageToDomain(t, router, testDomain1)
		assertStatus(t, rec, http.StatusOK)
	}
	rec := sendMessageToDomain(t, router, testDomain2)
	assertStatus(t, rec, http.StatusOK)

	now := time.Now().UTC()
	start := now.Add(-7 * 24 * time.Hour)

	reqBody := map[string]interface{}{
		"start":      start.Format("Mon, 02 Jan 2006 15:04:05 MST"),
		"end":        now.Format("Mon, 02 Jan 2006 15:04:05 MST"),
		"resolution": "day",
		"dimensions": []string{"time"},
		"metrics":    []string{"accepted_count", "delivered_count"},
		"filter": map[string]interface{}{
			"AND": []map[string]interface{}{
				{
					"attribute":  "domain",
					"comparator": "=",
					"values": []map[string]string{
						{"label": testDomain1, "value": testDomain1},
					},
				},
			},
		},
		"include_aggregates": true,
	}

	rec = doJSONRequest(t, router, "POST", "/v1/analytics/metrics", reqBody)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	// Verify response is valid
	if _, ok := body["items"]; !ok {
		t.Error("expected 'items' field in filtered v1 metrics response")
	}

	// If aggregates are present, check the accepted_count is scoped to domain1
	if aggregates, ok := body["aggregates"].(map[string]interface{}); ok {
		if metricsData, ok := aggregates["metrics"].(map[string]interface{}); ok {
			if acceptedCount, ok := metricsData["accepted_count"]; ok {
				// Should be scoped to domain1 only (2 messages)
				if count, ok := acceptedCount.(float64); ok && count > 0 {
					if count != 2 {
						t.Errorf("expected accepted_count=2 for filtered domain, got %v", count)
					}
				}
			}
		}
	}
}

// =========================================================================
// 20. v1 Metrics with Aggregates
// =========================================================================

func TestV1Metrics_WithAggregates(t *testing.T) {
	router := setup(t)

	// Send messages
	for i := 0; i < 5; i++ {
		rec := sendMessage(t, router)
		assertStatus(t, rec, http.StatusOK)
	}

	now := time.Now().UTC()
	start := now.Add(-7 * 24 * time.Hour)

	reqBody := map[string]interface{}{
		"start":              start.Format("Mon, 02 Jan 2006 15:04:05 MST"),
		"end":                now.Format("Mon, 02 Jan 2006 15:04:05 MST"),
		"resolution":         "day",
		"dimensions":         []string{"time"},
		"metrics":            []string{"accepted_count", "delivered_count"},
		"include_aggregates": true,
	}

	rec := doJSONRequest(t, router, "POST", "/v1/analytics/metrics", reqBody)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	// Verify aggregates field exists when include_aggregates is true
	aggregates, ok := body["aggregates"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'aggregates' field in response when include_aggregates=true")
	}

	// Verify metrics sub-object
	metricsData, ok := aggregates["metrics"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'metrics' field inside aggregates")
	}

	// Verify requested metrics are present in aggregates
	if _, ok := metricsData["accepted_count"]; !ok {
		t.Error("expected 'accepted_count' in aggregates.metrics")
	}
	if _, ok := metricsData["delivered_count"]; !ok {
		t.Error("expected 'delivered_count' in aggregates.metrics")
	}
}

// =========================================================================
// 21. v1 Metrics with Pagination
// =========================================================================

func TestV1Metrics_Pagination(t *testing.T) {
	router := setup(t)

	// Send messages
	for i := 0; i < 3; i++ {
		rec := sendMessage(t, router)
		assertStatus(t, rec, http.StatusOK)
	}

	now := time.Now().UTC()
	start := now.Add(-7 * 24 * time.Hour)

	reqBody := map[string]interface{}{
		"start":      start.Format("Mon, 02 Jan 2006 15:04:05 MST"),
		"end":        now.Format("Mon, 02 Jan 2006 15:04:05 MST"),
		"resolution": "day",
		"dimensions": []string{"time"},
		"metrics":    []string{"accepted_count"},
		"pagination": map[string]interface{}{
			"sort":  "time:asc",
			"skip":  0,
			"limit": 2,
		},
	}

	rec := doJSONRequest(t, router, "POST", "/v1/analytics/metrics", reqBody)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	// Verify pagination field exists in response
	pagination, ok := body["pagination"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'pagination' field in response")
	}

	// Verify pagination fields
	for _, field := range []string{"sort", "skip", "limit", "total"} {
		if _, ok := pagination[field]; !ok {
			t.Errorf("expected pagination.%s field", field)
		}
	}

	// Verify limit is respected
	if limit, ok := pagination["limit"].(float64); ok {
		if limit != 2 {
			t.Errorf("expected pagination.limit=2, got %v", limit)
		}
	}

	// Verify items array length is capped by limit
	items, ok := body["items"].([]interface{})
	if !ok {
		t.Fatal("expected 'items' array in response")
	}
	if len(items) > 2 {
		t.Errorf("expected at most 2 items (limit=2), got %d", len(items))
	}
}

// =========================================================================
// 22. v1 Metrics — Too Many Dimensions (400)
// =========================================================================

func TestV1Metrics_TooManyDimensions(t *testing.T) {
	router := setup(t)

	reqBody := map[string]interface{}{
		"start":      time.Now().UTC().Add(-7 * 24 * time.Hour).Format("Mon, 02 Jan 2006 15:04:05 MST"),
		"end":        time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 MST"),
		"resolution": "day",
		"dimensions": []string{"time", "domain", "provider", "country"}, // 4 > max 3
		"metrics":    []string{"accepted_count"},
	}

	rec := doJSONRequest(t, router, "POST", "/v1/analytics/metrics", reqBody)
	assertStatus(t, rec, http.StatusBadRequest)
	assertMessage(t, rec, "too many dimensions")
}

// =========================================================================
// 23. v1 Metrics — Too Many Metrics (400)
// =========================================================================

func TestV1Metrics_TooManyMetrics(t *testing.T) {
	router := setup(t)

	reqBody := map[string]interface{}{
		"start":      time.Now().UTC().Add(-7 * 24 * time.Hour).Format("Mon, 02 Jan 2006 15:04:05 MST"),
		"end":        time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 MST"),
		"resolution": "day",
		"dimensions": []string{"time"},
		"metrics": []string{
			"accepted_count", "delivered_count", "opened_count", "clicked_count",
			"unsubscribed_count", "complained_count", "stored_count", "failed_count",
			"opened_rate", "clicked_rate", "bounced_count",
		}, // 11 > max 10
	}

	rec := doJSONRequest(t, router, "POST", "/v1/analytics/metrics", reqBody)
	assertStatus(t, rec, http.StatusBadRequest)
	assertMessage(t, rec, "too many metrics")
}

// =========================================================================
// 24. v1 Metrics — Duration Parameter
// =========================================================================

func TestV1Metrics_DurationParameter(t *testing.T) {
	router := setup(t)

	rec := sendMessage(t, router)
	assertStatus(t, rec, http.StatusOK)

	reqBody := map[string]interface{}{
		"end":                time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 MST"),
		"duration":           "30d",
		"resolution":         "day",
		"dimensions":         []string{"time"},
		"metrics":            []string{"accepted_count"},
		"include_aggregates": true,
	}

	rec = doJSONRequest(t, router, "POST", "/v1/analytics/metrics", reqBody)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	// Verify duration is echoed back
	if dur, ok := body["duration"].(string); ok {
		if dur != "30d" {
			t.Errorf("expected duration %q, got %q", "30d", dur)
		}
	}
}

// =========================================================================
// 25. v1 Metrics — Rate Metrics are Strings
// =========================================================================

func TestV1Metrics_RateMetricsAreStrings(t *testing.T) {
	router := setup(t)

	// Send messages
	for i := 0; i < 3; i++ {
		rec := sendMessage(t, router)
		assertStatus(t, rec, http.StatusOK)
	}

	now := time.Now().UTC()
	start := now.Add(-7 * 24 * time.Hour)

	reqBody := map[string]interface{}{
		"start":              start.Format("Mon, 02 Jan 2006 15:04:05 MST"),
		"end":                now.Format("Mon, 02 Jan 2006 15:04:05 MST"),
		"resolution":         "day",
		"dimensions":         []string{"time"},
		"metrics":            []string{"accepted_count", "opened_rate"},
		"include_aggregates": true,
	}

	rec := doJSONRequest(t, router, "POST", "/v1/analytics/metrics", reqBody)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	// Check aggregates for rate metric type
	if aggregates, ok := body["aggregates"].(map[string]interface{}); ok {
		if metricsData, ok := aggregates["metrics"].(map[string]interface{}); ok {
			// accepted_count should be a number
			if acceptedCount, ok := metricsData["accepted_count"]; ok {
				if _, isNum := acceptedCount.(float64); !isNum {
					t.Errorf("expected accepted_count to be a number, got %T", acceptedCount)
				}
			}
			// opened_rate should be a string
			if openedRate, ok := metricsData["opened_rate"]; ok {
				if _, isStr := openedRate.(string); !isStr {
					t.Errorf("expected opened_rate to be a string, got %T: %v", openedRate, openedRate)
				}
			}
		}
	}

	// Also check items for rate metric type
	if items, ok := body["items"].([]interface{}); ok && len(items) > 0 {
		firstItem := items[0].(map[string]interface{})
		if itemMetrics, ok := firstItem["metrics"].(map[string]interface{}); ok {
			if openedRate, ok := itemMetrics["opened_rate"]; ok {
				if _, isStr := openedRate.(string); !isStr {
					t.Errorf("expected item opened_rate to be a string, got %T: %v", openedRate, openedRate)
				}
			}
		}
	}
}

// =========================================================================
// 26. v1 Metrics — Item Dimensions Shape
// =========================================================================

func TestV1Metrics_ItemDimensionsShape(t *testing.T) {
	router := setup(t)

	rec := sendMessage(t, router)
	assertStatus(t, rec, http.StatusOK)

	now := time.Now().UTC()
	start := now.Add(-7 * 24 * time.Hour)

	reqBody := map[string]interface{}{
		"start":      start.Format("Mon, 02 Jan 2006 15:04:05 MST"),
		"end":        now.Format("Mon, 02 Jan 2006 15:04:05 MST"),
		"resolution": "day",
		"dimensions": []string{"time"},
		"metrics":    []string{"accepted_count"},
	}

	rec = doJSONRequest(t, router, "POST", "/v1/analytics/metrics", reqBody)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	items, ok := body["items"].([]interface{})
	if !ok || len(items) == 0 {
		t.Fatal("expected non-empty items array")
	}

	// Verify the shape of the first item's dimensions
	firstItem := items[0].(map[string]interface{})
	dimensions, ok := firstItem["dimensions"].([]interface{})
	if !ok || len(dimensions) == 0 {
		t.Fatal("expected non-empty dimensions array in item")
	}

	firstDim, ok := dimensions[0].(map[string]interface{})
	if !ok {
		t.Fatal("expected dimension to be a map")
	}

	// Each dimension should have dimension, value, and display_value
	for _, key := range []string{"dimension", "value", "display_value"} {
		if _, ok := firstDim[key]; !ok {
			t.Errorf("expected dimension.%s field", key)
		}
	}

	// The dimension should be "time" since that's what we requested
	if dimName, ok := firstDim["dimension"].(string); ok {
		if dimName != "time" {
			t.Errorf("expected dimension name %q, got %q", "time", dimName)
		}
	}
}

// =========================================================================
// 27. v1 Metrics — include_subaccounts Accepted
// =========================================================================

func TestV1Metrics_IncludeSubaccountsAccepted(t *testing.T) {
	router := setup(t)

	rec := sendMessage(t, router)
	assertStatus(t, rec, http.StatusOK)

	now := time.Now().UTC()
	start := now.Add(-7 * 24 * time.Hour)

	reqBody := map[string]interface{}{
		"start":                start.Format("Mon, 02 Jan 2006 15:04:05 MST"),
		"end":                  now.Format("Mon, 02 Jan 2006 15:04:05 MST"),
		"resolution":           "day",
		"dimensions":           []string{"time"},
		"metrics":              []string{"accepted_count"},
		"include_subaccounts":  false,
	}

	rec = doJSONRequest(t, router, "POST", "/v1/analytics/metrics", reqBody)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	// Should return valid response regardless of include_subaccounts value
	if _, ok := body["items"]; !ok {
		t.Error("expected 'items' field in response")
	}
}

// =========================================================================
// 28. v1 Usage Metrics Stub
// =========================================================================

func TestV1UsageMetrics_Stub(t *testing.T) {
	router := setup(t)

	now := time.Now().UTC()
	start := now.Add(-7 * 24 * time.Hour)

	reqBody := map[string]interface{}{
		"start":      start.Format("Mon, 02 Jan 2006 15:04:05 MST"),
		"end":        now.Format("Mon, 02 Jan 2006 15:04:05 MST"),
		"resolution": "day",
		"dimensions": []string{"time"},
		"metrics":    []string{"accepted_count"},
	}

	rec := doJSONRequest(t, router, "POST", "/v1/analytics/usage/metrics", reqBody)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	// Verify response shape
	for _, field := range []string{"start", "end", "resolution", "dimensions", "items"} {
		if _, ok := body[field]; !ok {
			t.Errorf("expected field %q in usage metrics response", field)
		}
	}

	// Verify items is present (may be empty or contain zeroed entries)
	items, ok := body["items"].([]interface{})
	if !ok {
		t.Fatal("expected items to be an array")
	}

	// Usage metrics stub returns empty items
	if len(items) != 0 {
		t.Logf("note: usage metrics returned %d items (expected 0 for stub)", len(items))
	}
}

// =========================================================================
// 29. v2 Bounce Classification Stub
// =========================================================================

func TestV2BounceClassification_Stub(t *testing.T) {
	router := setup(t)

	now := time.Now().UTC()
	start := now.Add(-7 * 24 * time.Hour)

	reqBody := map[string]interface{}{
		"start":      start.Format("Mon, 02 Jan 2006 15:04:05 MST"),
		"end":        now.Format("Mon, 02 Jan 2006 15:04:05 MST"),
		"resolution": "day",
		"dimensions": []string{"time"},
		"metrics":    []string{"failed_count"},
	}

	rec := doJSONRequest(t, router, "POST", "/v2/bounce-classification/metrics", reqBody)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	// Verify response shape
	for _, field := range []string{"start", "end", "resolution", "dimensions", "items"} {
		if _, ok := body[field]; !ok {
			t.Errorf("expected field %q in bounce classification response", field)
		}
	}

	// Verify items is present (may be empty for stub)
	items, ok := body["items"].([]interface{})
	if !ok {
		t.Fatal("expected items to be an array")
	}

	// Bounce classification stub returns empty items
	if len(items) != 0 {
		t.Logf("note: bounce classification returned %d items (expected 0 for stub)", len(items))
	}
}

// =========================================================================
// 30. v1 Metrics — No Dimensions and Metrics Boundary (valid)
// =========================================================================

func TestV1Metrics_ExactlyThreeDimensions(t *testing.T) {
	router := setup(t)

	rec := sendMessage(t, router)
	assertStatus(t, rec, http.StatusOK)

	now := time.Now().UTC()
	start := now.Add(-7 * 24 * time.Hour)

	// Exactly 3 dimensions should be accepted
	reqBody := map[string]interface{}{
		"start":      start.Format("Mon, 02 Jan 2006 15:04:05 MST"),
		"end":        now.Format("Mon, 02 Jan 2006 15:04:05 MST"),
		"resolution": "day",
		"dimensions": []string{"time", "domain", "provider"},
		"metrics":    []string{"accepted_count"},
	}

	rec = doJSONRequest(t, router, "POST", "/v1/analytics/metrics", reqBody)
	assertStatus(t, rec, http.StatusOK)
}

// =========================================================================
// 31. v1 Metrics — Exactly Ten Metrics (boundary, valid)
// =========================================================================

func TestV1Metrics_ExactlyTenMetrics(t *testing.T) {
	router := setup(t)

	rec := sendMessage(t, router)
	assertStatus(t, rec, http.StatusOK)

	now := time.Now().UTC()
	start := now.Add(-7 * 24 * time.Hour)

	// Exactly 10 metrics should be accepted
	reqBody := map[string]interface{}{
		"start":      start.Format("Mon, 02 Jan 2006 15:04:05 MST"),
		"end":        now.Format("Mon, 02 Jan 2006 15:04:05 MST"),
		"resolution": "day",
		"dimensions": []string{"time"},
		"metrics": []string{
			"accepted_count", "delivered_count", "opened_count", "clicked_count",
			"unsubscribed_count", "complained_count", "stored_count", "failed_count",
			"opened_rate", "clicked_rate",
		},
	}

	rec = doJSONRequest(t, router, "POST", "/v1/analytics/metrics", reqBody)
	assertStatus(t, rec, http.StatusOK)
}

// =========================================================================
// 32. Account Stats — Resolution "month"
// =========================================================================

func TestAccountStats_ResolutionMonth(t *testing.T) {
	router := setup(t)

	rec := sendMessage(t, router)
	assertStatus(t, rec, http.StatusOK)

	rec = doGET(t, router, "/v3/stats/total?event=accepted&resolution=month")
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	if body["resolution"] != "month" {
		t.Errorf("expected resolution %q, got %v", "month", body["resolution"])
	}

	stats, ok := body["stats"].([]interface{})
	if !ok || len(stats) == 0 {
		t.Fatal("expected non-empty stats array with month resolution")
	}
}

// =========================================================================
// 33. Account Stats — No Tag or Description Fields
// =========================================================================

func TestAccountStats_NoTagOrDescription(t *testing.T) {
	router := setup(t)

	rec := sendMessage(t, router)
	assertStatus(t, rec, http.StatusOK)

	rec = doGET(t, router, "/v3/stats/total?event=accepted")
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	// Account stats should NOT contain tag or description fields
	if _, ok := body["tag"]; ok {
		t.Error("account stats response should NOT contain 'tag' field")
	}
	if _, ok := body["description"]; ok {
		t.Error("account stats response should NOT contain 'description' field")
	}
}

// =========================================================================
// 34. v1 Metrics — Multiple Domains Aggregated
// =========================================================================

func TestV1Metrics_MultipleDomains(t *testing.T) {
	router := setupTwoDomains(t)

	// Send messages to both domains
	for i := 0; i < 2; i++ {
		rec := sendMessageToDomain(t, router, testDomain1)
		assertStatus(t, rec, http.StatusOK)
	}
	for i := 0; i < 3; i++ {
		rec := sendMessageToDomain(t, router, testDomain2)
		assertStatus(t, rec, http.StatusOK)
	}

	now := time.Now().UTC()
	start := now.Add(-7 * 24 * time.Hour)

	reqBody := map[string]interface{}{
		"start":              start.Format("Mon, 02 Jan 2006 15:04:05 MST"),
		"end":                now.Format("Mon, 02 Jan 2006 15:04:05 MST"),
		"resolution":         "day",
		"dimensions":         []string{"time"},
		"metrics":            []string{"accepted_count"},
		"include_aggregates": true,
	}

	rec := doJSONRequest(t, router, "POST", "/v1/analytics/metrics", reqBody)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	// If aggregates are present, accepted_count should include all domains
	if aggregates, ok := body["aggregates"].(map[string]interface{}); ok {
		if metricsData, ok := aggregates["metrics"].(map[string]interface{}); ok {
			if acceptedCount, ok := metricsData["accepted_count"].(float64); ok {
				// 2 + 3 = 5 messages total
				if acceptedCount != 5 {
					t.Errorf("expected accepted_count=5 across both domains, got %v", acceptedCount)
				}
			}
		}
	}
}

// =========================================================================
// 35. Domain Aggregate Stubs — All Three Return Same Shape
// =========================================================================

func TestDomainAggregateStubs_ConsistentShape(t *testing.T) {
	router := setup(t)

	endpoints := []struct {
		name string
		path string
	}{
		{"providers", fmt.Sprintf("/v3/%s/aggregates/providers", testDomain1)},
		{"devices", fmt.Sprintf("/v3/%s/aggregates/devices", testDomain1)},
		{"countries", fmt.Sprintf("/v3/%s/aggregates/countries", testDomain1)},
	}

	for _, ep := range endpoints {
		t.Run(ep.name, func(t *testing.T) {
			rec := doGET(t, router, ep.path)
			assertStatus(t, rec, http.StatusOK)

			var body map[string]interface{}
			decodeJSON(t, rec, &body)

			// All should have "items" key with empty map
			items, ok := body["items"]
			if !ok {
				t.Fatalf("%s: expected 'items' field", ep.name)
			}
			itemsMap, ok := items.(map[string]interface{})
			if !ok {
				t.Fatalf("%s: expected items to be a map, got %T", ep.name, items)
			}
			if len(itemsMap) != 0 {
				t.Errorf("%s: expected empty items map, got %d entries", ep.name, len(itemsMap))
			}
		})
	}
}

// =========================================================================
// 36. v1 Usage Metrics — Same Request Shape as v1 Metrics
// =========================================================================

func TestV1UsageMetrics_SameRequestShape(t *testing.T) {
	router := setup(t)

	now := time.Now().UTC()
	start := now.Add(-7 * 24 * time.Hour)

	reqBody := map[string]interface{}{
		"start":              start.Format("Mon, 02 Jan 2006 15:04:05 MST"),
		"end":                now.Format("Mon, 02 Jan 2006 15:04:05 MST"),
		"resolution":         "day",
		"dimensions":         []string{"time"},
		"metrics":            []string{"accepted_count"},
		"include_aggregates": true,
		"pagination": map[string]interface{}{
			"sort":  "time:asc",
			"skip":  0,
			"limit": 10,
		},
	}

	rec := doJSONRequest(t, router, "POST", "/v1/analytics/usage/metrics", reqBody)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	// Should still have valid pagination in response
	if pagination, ok := body["pagination"].(map[string]interface{}); ok {
		for _, field := range []string{"sort", "skip", "limit", "total"} {
			if _, ok := pagination[field]; !ok {
				t.Errorf("expected pagination.%s in usage metrics response", field)
			}
		}
	}
}

// =========================================================================
// 37. v2 Bounce Classification — Same Request Shape
// =========================================================================

func TestV2BounceClassification_SameRequestShape(t *testing.T) {
	router := setup(t)

	now := time.Now().UTC()
	start := now.Add(-7 * 24 * time.Hour)

	reqBody := map[string]interface{}{
		"start":              start.Format("Mon, 02 Jan 2006 15:04:05 MST"),
		"end":                now.Format("Mon, 02 Jan 2006 15:04:05 MST"),
		"resolution":         "day",
		"dimensions":         []string{"time"},
		"metrics":            []string{"failed_count"},
		"include_aggregates": true,
		"pagination": map[string]interface{}{
			"sort":  "time:asc",
			"skip":  0,
			"limit": 10,
		},
	}

	rec := doJSONRequest(t, router, "POST", "/v2/bounce-classification/metrics", reqBody)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	// Should still have valid pagination in response
	if pagination, ok := body["pagination"].(map[string]interface{}); ok {
		for _, field := range []string{"sort", "skip", "limit", "total"} {
			if _, ok := pagination[field]; !ok {
				t.Errorf("expected pagination.%s in bounce classification response", field)
			}
		}
	}
}

// =========================================================================
// 38. Filtered Stats — Invalid Event Type (400)
// =========================================================================

func TestFilteredStats_InvalidEventType(t *testing.T) {
	router := setup(t)

	rec := doGET(t, router, "/v3/stats/filter?event=invalid_type")
	assertStatus(t, rec, http.StatusBadRequest)
	assertMessage(t, rec, "invalid event type")
}

// =========================================================================
// 39. Per-Domain Stats Snapshot — Missing Event Param (400)
// =========================================================================

func TestDomainStatsSnapshot_MissingEventParam(t *testing.T) {
	router := setup(t)

	now := time.Now().UTC()
	timestampStr := fmt.Sprintf("%.6f", float64(now.UnixMicro())/1e6)

	// Missing event param
	url := fmt.Sprintf("/v3/stats/total/domains?timestamp=%s", timestampStr)
	rec := doGET(t, router, url)
	assertStatus(t, rec, http.StatusBadRequest)
	assertMessage(t, rec, "event is required")
}
