package tag_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bethmaloney/mailgun-mock-api/internal/domain"
	"github.com/bethmaloney/mailgun-mock-api/internal/event"
	"github.com/bethmaloney/mailgun-mock-api/internal/message"
	"github.com/bethmaloney/mailgun-mock-api/internal/mock"
	"github.com/bethmaloney/mailgun-mock-api/internal/tag"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// Stats Test Helpers
// ---------------------------------------------------------------------------

// setupStatsRouter creates a router with all tag CRUD routes, tag stats routes,
// and domain stats routes registered.
func setupStatsRouter(db *gorm.DB, cfg *mock.MockConfig) http.Handler {
	domain.ResetForTests(db)
	event.ResetForTests(db)
	dh := domain.NewHandlers(db, cfg)
	tgh := tag.NewHandlers(db)
	eh := event.NewHandlers(db, cfg)
	mh := message.NewHandlers(db, cfg)
	r := chi.NewRouter()

	// Domain creation
	r.Route("/v4/domains", func(r chi.Router) {
		r.Post("/", dh.CreateDomain)
	})

	// Tag CRUD routes
	r.Route("/v3/{domain_name}/tags", func(r chi.Router) {
		r.Get("/", tgh.ListTags)
		r.Get("/{tag}", tgh.GetTag)
		r.Put("/{tag}", tgh.UpdateTag)
		r.Delete("/{tag}", tgh.DeleteTag)

		// Tag stats routes
		r.Get("/{tag}/stats", tgh.GetTagStats)
		r.Get("/{tag}/stats/aggregates/countries", tgh.GetTagStatsCountries)
		r.Get("/{tag}/stats/aggregates/providers", tgh.GetTagStatsProviders)
		r.Get("/{tag}/stats/aggregates/devices", tgh.GetTagStatsDevices)
	})

	// Tag limits route
	r.Get("/v3/domains/{domain_name}/limits/tag", tgh.GetTagLimits)

	// Domain-level stats
	r.Get("/v3/{domain_name}/stats/total", tgh.GetDomainStats)

	// Message sending (for event generation)
	r.Route("/v3/{domain_name}/messages", func(r chi.Router) {
		r.Post("/", mh.SendMessage)
	})

	// Suppress linter for eh
	_ = eh

	return r
}

// setupStats sets up a test router with stats routes and creates the test domain.
func setupStats(t *testing.T) http.Handler {
	t.Helper()
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupStatsRouter(db, cfg)

	// Create the test domain
	rec := doRequest(t, router, "POST", "/v4/domains", map[string]string{
		"name": testDomain,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to create test domain: %s", rec.Body.String())
	}

	return router
}

// doGET is a shorthand for making a GET request with query params baked into the URL.
func doGET(t *testing.T, router http.Handler, url string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", url, nil)
	router.ServeHTTP(rec, req)
	return rec
}

// =========================================================================
// Tag Stats Tests
// =========================================================================

// ---------------------------------------------------------------------------
// 1. Tag Stats — Basic: send messages, verify event counts
// ---------------------------------------------------------------------------

func TestTagStats_Basic(t *testing.T) {
	router := setupStats(t)

	// Send 3 messages with the "newsletter" tag (each produces accepted + delivered events)
	for i := 0; i < 3; i++ {
		rec := sendMessage(t, router, "newsletter")
		assertStatus(t, rec, http.StatusOK)
	}

	// GET tag stats for accepted and delivered events
	url := fmt.Sprintf("/v3/%s/tags/newsletter/stats?event=accepted&event=delivered", testDomain)
	rec := doGET(t, router, url)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	// Verify tag name is included
	if body["tag"] != "newsletter" {
		t.Errorf("expected tag %q, got %v", "newsletter", body["tag"])
	}

	// Verify stats array exists
	stats, ok := body["stats"].([]interface{})
	if !ok {
		t.Fatalf("expected stats array, got %T: %v", body["stats"], body["stats"])
	}
	if len(stats) == 0 {
		t.Fatal("expected at least one stats time bucket, got 0")
	}

	// Sum accepted and delivered totals across all time buckets
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

	// Each message sends to 1 recipient, so 3 accepted + 3 delivered
	if totalAccepted != 3 {
		t.Errorf("expected 3 total accepted events, got %v", totalAccepted)
	}
	if totalDelivered != 3 {
		t.Errorf("expected 3 total delivered events, got %v", totalDelivered)
	}
}

// ---------------------------------------------------------------------------
// 2. Tag Stats — Resolution Parameter (hour)
// ---------------------------------------------------------------------------

func TestTagStats_ResolutionHour(t *testing.T) {
	router := setupStats(t)

	rec := sendMessage(t, router, "hourly")
	assertStatus(t, rec, http.StatusOK)

	url := fmt.Sprintf("/v3/%s/tags/hourly/stats?event=accepted&resolution=hour", testDomain)
	rec = doGET(t, router, url)
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

	// RFC 2822 time should be parseable with time.RFC1123 or similar
	_, err := time.Parse(time.RFC1123, timeStr)
	if err != nil {
		_, err = time.Parse(time.RFC1123Z, timeStr)
		if err != nil {
			// Try the exact format used in the spec: "Mon, 16 Mar 2024 00:00:00 UTC"
			_, err = time.Parse("Mon, 02 Jan 2006 15:04:05 MST", timeStr)
			if err != nil {
				t.Errorf("time bucket %q is not a valid RFC 2822 timestamp: %v", timeStr, err)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// 3. Tag Stats — Default Resolution is "day"
// ---------------------------------------------------------------------------

func TestTagStats_DefaultResolutionDay(t *testing.T) {
	router := setupStats(t)

	rec := sendMessage(t, router, "daily")
	assertStatus(t, rec, http.StatusOK)

	// Do NOT pass resolution — should default to "day"
	url := fmt.Sprintf("/v3/%s/tags/daily/stats?event=accepted", testDomain)
	rec = doGET(t, router, url)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	if body["resolution"] != "day" {
		t.Errorf("expected default resolution %q, got %v", "day", body["resolution"])
	}
}

// ---------------------------------------------------------------------------
// 4. Tag Stats — Event Parameter Required (400)
// ---------------------------------------------------------------------------

func TestTagStats_EventParamRequired(t *testing.T) {
	router := setupStats(t)

	rec := sendMessage(t, router, "missing-event")
	assertStatus(t, rec, http.StatusOK)

	// GET stats without the event parameter
	url := fmt.Sprintf("/v3/%s/tags/missing-event/stats", testDomain)
	rec = doGET(t, router, url)
	assertStatus(t, rec, http.StatusBadRequest)
	assertMessage(t, rec, "event is required")
}

// ---------------------------------------------------------------------------
// 5. Tag Stats — Tag Not Found (404)
// ---------------------------------------------------------------------------

func TestTagStats_TagNotFound(t *testing.T) {
	router := setupStats(t)

	url := fmt.Sprintf("/v3/%s/tags/nonexistent/stats?event=accepted", testDomain)
	rec := doGET(t, router, url)
	assertStatus(t, rec, http.StatusNotFound)
	assertMessage(t, rec, "tag not found")
}

// ---------------------------------------------------------------------------
// 6. Tag Stats — Event Param is Repeatable
// ---------------------------------------------------------------------------

func TestTagStats_EventParamRepeatable(t *testing.T) {
	router := setupStats(t)

	// Send messages to generate events
	rec := sendMessage(t, router, "multi-event")
	assertStatus(t, rec, http.StatusOK)

	// Request only delivered and accepted events
	url := fmt.Sprintf("/v3/%s/tags/multi-event/stats?event=delivered&event=accepted", testDomain)
	rec = doGET(t, router, url)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	stats, ok := body["stats"].([]interface{})
	if !ok || len(stats) == 0 {
		t.Fatal("expected non-empty stats array")
	}

	// Verify accepted and delivered fields are populated
	firstBucket := stats[0].(map[string]interface{})

	if _, ok := firstBucket["accepted"]; !ok {
		t.Error("expected 'accepted' field in stats bucket")
	}
	if _, ok := firstBucket["delivered"]; !ok {
		t.Error("expected 'delivered' field in stats bucket")
	}
}

// ---------------------------------------------------------------------------
// 7. Tag Stats — Response Shape (nested structure)
// ---------------------------------------------------------------------------

func TestTagStats_ResponseShape(t *testing.T) {
	router := setupStats(t)

	rec := sendMessage(t, router, "shape-test")
	assertStatus(t, rec, http.StatusOK)

	url := fmt.Sprintf("/v3/%s/tags/shape-test/stats?event=accepted&event=delivered&event=failed&event=opened&event=clicked&event=unsubscribed&event=complained&event=stored", testDomain)
	rec = doGET(t, router, url)
	assertStatus(t, rec, http.StatusOK)

	// Parse into raw JSON to verify nested structure
	var body map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify top-level fields
	for _, field := range []string{"tag", "start", "end", "resolution", "stats"} {
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

	// Verify failed has temporary.espblock and permanent sub-fields
	if rawFailed, ok := bucket["failed"]; ok {
		var failed map[string]json.RawMessage
		if err := json.Unmarshal(rawFailed, &failed); err != nil {
			t.Fatalf("failed to decode failed: %v", err)
		}

		// Check temporary
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

		// Check permanent
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

	// Verify simple total fields: stored, opened, clicked, unsubscribed, complained
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

// ---------------------------------------------------------------------------
// 8. Tag Stats — Start/End Parameters
// ---------------------------------------------------------------------------

func TestTagStats_StartEndParameters(t *testing.T) {
	router := setupStats(t)

	rec := sendMessage(t, router, "timerange")
	assertStatus(t, rec, http.StatusOK)

	now := time.Now().UTC()
	start := now.Add(-24 * time.Hour)
	end := now.Add(24 * time.Hour)

	// Use Unix epoch floats for start/end to avoid URL encoding issues with spaces
	startStr := fmt.Sprintf("%.6f", float64(start.UnixMicro())/1e6)
	endStr := fmt.Sprintf("%.6f", float64(end.UnixMicro())/1e6)

	url := fmt.Sprintf("/v3/%s/tags/timerange/stats?event=accepted&start=%s&end=%s",
		testDomain, startStr, endStr)
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

	// Verify stats are present (events within the range should be included)
	stats, ok := body["stats"].([]interface{})
	if !ok {
		t.Fatal("expected stats array")
	}
	if len(stats) == 0 {
		t.Error("expected at least one stats bucket for the given time range")
	}
}

// ---------------------------------------------------------------------------
// 9. Tag Stats — Includes Tag Name and Description
// ---------------------------------------------------------------------------

func TestTagStats_IncludesTagAndDescription(t *testing.T) {
	router := setupStats(t)

	// Create a tag via message send
	rec := sendMessage(t, router, "described")
	assertStatus(t, rec, http.StatusOK)

	// Update the tag with a description
	rec = doRequest(t, router, "PUT", fmt.Sprintf("/v3/%s/tags/described", testDomain), map[string]string{
		"description": "A tag with a description",
	})
	assertStatus(t, rec, http.StatusOK)

	// GET stats
	url := fmt.Sprintf("/v3/%s/tags/described/stats?event=accepted", testDomain)
	rec = doGET(t, router, url)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	// Verify tag name
	if body["tag"] != "described" {
		t.Errorf("expected tag %q, got %v", "described", body["tag"])
	}

	// Verify description
	desc, ok := body["description"].(string)
	if !ok {
		t.Fatalf("expected description string, got %T: %v", body["description"], body["description"])
	}
	if desc != "A tag with a description" {
		t.Errorf("expected description %q, got %q", "A tag with a description", desc)
	}
}

// =========================================================================
// Domain Stats Tests
// =========================================================================

// ---------------------------------------------------------------------------
// 10. Domain Stats — Basic
// ---------------------------------------------------------------------------

func TestDomainStats_Basic(t *testing.T) {
	router := setupStats(t)

	// Send messages with different tags (events should be aggregated across all)
	rec := sendMessage(t, router, "tag-a")
	assertStatus(t, rec, http.StatusOK)
	rec = sendMessage(t, router, "tag-b")
	assertStatus(t, rec, http.StatusOK)

	// GET domain-level stats for accepted events
	url := fmt.Sprintf("/v3/%s/stats/total?event=accepted", testDomain)
	rec = doGET(t, router, url)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	// Verify stats array
	stats, ok := body["stats"].([]interface{})
	if !ok || len(stats) == 0 {
		t.Fatal("expected non-empty stats array")
	}

	// Sum accepted totals across all time buckets
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

	// 2 messages * 1 recipient = 2 accepted events total across all tags
	if totalAccepted != 2 {
		t.Errorf("expected 2 total accepted events across all tags, got %v", totalAccepted)
	}
}

// ---------------------------------------------------------------------------
// 11. Domain Stats — No Tag/Description Fields
// ---------------------------------------------------------------------------

func TestDomainStats_NoTagOrDescription(t *testing.T) {
	router := setupStats(t)

	rec := sendMessage(t, router, "domain-check")
	assertStatus(t, rec, http.StatusOK)

	url := fmt.Sprintf("/v3/%s/stats/total?event=accepted", testDomain)
	rec = doGET(t, router, url)
	assertStatus(t, rec, http.StatusOK)

	// Parse into a generic map to check field presence
	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	// Verify that tag and description fields are NOT present
	if _, ok := body["tag"]; ok {
		t.Error("domain stats response should NOT contain 'tag' field")
	}
	if _, ok := body["description"]; ok {
		t.Error("domain stats response should NOT contain 'description' field")
	}

	// Verify expected fields ARE present
	for _, field := range []string{"start", "end", "resolution", "stats"} {
		if _, ok := body[field]; !ok {
			t.Errorf("expected field %q in domain stats response", field)
		}
	}
}

// ---------------------------------------------------------------------------
// 12. Domain Stats — Event Param Required (400)
// ---------------------------------------------------------------------------

func TestDomainStats_EventParamRequired(t *testing.T) {
	router := setupStats(t)

	// GET domain stats without the event parameter
	url := fmt.Sprintf("/v3/%s/stats/total", testDomain)
	rec := doGET(t, router, url)
	assertStatus(t, rec, http.StatusBadRequest)
	assertMessage(t, rec, "event is required")
}

// =========================================================================
// Aggregate Stub Tests
// =========================================================================

// ---------------------------------------------------------------------------
// 13. Aggregate Stub — Countries
// ---------------------------------------------------------------------------

func TestTagStatsAggregates_Countries(t *testing.T) {
	router := setupStats(t)

	rec := sendMessage(t, router, "countries-tag")
	assertStatus(t, rec, http.StatusOK)

	url := fmt.Sprintf("/v3/%s/tags/countries-tag/stats/aggregates/countries", testDomain)
	rec = doGET(t, router, url)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	// Verify tag field
	if body["tag"] != "countries-tag" {
		t.Errorf("expected tag %q, got %v", "countries-tag", body["tag"])
	}

	// Verify countries field is present (empty map)
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
// 14. Aggregate Stub — Providers
// ---------------------------------------------------------------------------

func TestTagStatsAggregates_Providers(t *testing.T) {
	router := setupStats(t)

	rec := sendMessage(t, router, "providers-tag")
	assertStatus(t, rec, http.StatusOK)

	url := fmt.Sprintf("/v3/%s/tags/providers-tag/stats/aggregates/providers", testDomain)
	rec = doGET(t, router, url)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	// Verify tag field
	if body["tag"] != "providers-tag" {
		t.Errorf("expected tag %q, got %v", "providers-tag", body["tag"])
	}

	// Verify providers field is present (empty map)
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
// 15. Aggregate Stub — Devices
// ---------------------------------------------------------------------------

func TestTagStatsAggregates_Devices(t *testing.T) {
	router := setupStats(t)

	rec := sendMessage(t, router, "devices-tag")
	assertStatus(t, rec, http.StatusOK)

	url := fmt.Sprintf("/v3/%s/tags/devices-tag/stats/aggregates/devices", testDomain)
	rec = doGET(t, router, url)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	// Verify tag field
	if body["tag"] != "devices-tag" {
		t.Errorf("expected tag %q, got %v", "devices-tag", body["tag"])
	}

	// Verify devices field is present (empty map)
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

// ---------------------------------------------------------------------------
// 16. Aggregate — Tag Not Found (404)
// ---------------------------------------------------------------------------

func TestTagStatsAggregates_TagNotFound(t *testing.T) {
	router := setupStats(t)

	// Test all three aggregate endpoints with a nonexistent tag
	endpoints := []struct {
		name string
		path string
	}{
		{"countries", fmt.Sprintf("/v3/%s/tags/nonexistent/stats/aggregates/countries", testDomain)},
		{"providers", fmt.Sprintf("/v3/%s/tags/nonexistent/stats/aggregates/providers", testDomain)},
		{"devices", fmt.Sprintf("/v3/%s/tags/nonexistent/stats/aggregates/devices", testDomain)},
	}

	for _, ep := range endpoints {
		t.Run(ep.name, func(t *testing.T) {
			rec := doGET(t, router, ep.path)
			assertStatus(t, rec, http.StatusNotFound)
			assertMessage(t, rec, "tag not found")
		})
	}
}
