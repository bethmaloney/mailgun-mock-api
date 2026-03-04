package integration

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/mailgun/mailgun-go/v5"
	"github.com/mailgun/mailgun-go/v5/mtypes"
)

func TestTags(t *testing.T) {
	resetServer(t)

	const domain = "tags-test.example.com"

	// Setup: create a domain
	resp, err := doFormRequest("POST", "/v4/domains", map[string]string{"name": domain})
	if err != nil {
		t.Fatalf("setup: create domain: %v", err)
	}
	resp.Body.Close()

	// Setup: send messages with tags to auto-create tags via EnsureTags
	for _, tagSet := range []struct {
		to   string
		tags []string
	}{
		{"user1@example.com", []string{"newsletter", "promo"}},
		{"user2@example.com", []string{"newsletter", "updates"}},
		{"user3@example.com", []string{"promo"}},
	} {
		// Send one message per tag. Since doFormRequest uses multipart with map keys,
		// each key can only appear once, so we send separate messages for each tag.
		for _, tag := range tagSet.tags {
			r, err := doFormRequest("POST", "/v3/"+domain+"/messages", map[string]string{
				"from":    "sender@" + domain,
				"to":      tagSet.to,
				"subject": "Tagged message for " + tag,
				"text":    "Body",
				"o:tag":   tag,
			})
			if err != nil {
				t.Fatalf("setup: send message with tag %q: %v", tag, err)
			}
			r.Body.Close()
		}
	}

	// -----------------------------------------------------------------------
	// 9.1 List Tags
	// -----------------------------------------------------------------------

	t.Run("SDK_ListTags", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		it := mg.ListTags(domain, &mailgun.ListTagOptions{Limit: 10})
		var page []mtypes.Tag
		found := it.Next(ctx, &page)
		if !found && it.Err() != nil {
			reporter.Record("Tags", "ListTags", "SDK", false, it.Err().Error())
			t.Fatalf("ListTags: %v", it.Err())
		}

		if len(page) < 3 {
			reporter.Record("Tags", "ListTags", "SDK", false, fmt.Sprintf("expected >= 3 tags, got %d", len(page)))
			t.Fatalf("expected >= 3 tags, got %d", len(page))
		}

		// Verify known tags exist
		tagNames := make(map[string]bool)
		for _, tag := range page {
			tagNames[tag.Value] = true
		}
		for _, expected := range []string{"newsletter", "promo", "updates"} {
			if !tagNames[expected] {
				t.Errorf("expected tag %q in results", expected)
			}
		}

		reporter.Record("Tags", "ListTags", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_ListTags", func(t *testing.T) {
		resp, err := doRequest("GET", "/v3/"+domain+"/tags", nil)
		if err != nil {
			reporter.Record("Tags", "ListTags", "HTTP", false, err.Error())
			t.Fatalf("GET /v3/%s/tags: %v", domain, err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result struct {
			Items  []map[string]interface{} `json:"items"`
			Paging map[string]interface{}   `json:"paging"`
		}
		readJSON(t, resp, &result)

		if len(result.Items) < 3 {
			t.Fatalf("expected >= 3 tags, got %d", len(result.Items))
		}

		// Verify items have expected keys (hyphenated)
		for _, item := range result.Items {
			if _, ok := item["tag"]; !ok {
				t.Error("tag item missing 'tag' key")
			}
			if _, ok := item["first-seen"]; !ok {
				t.Error("tag item missing 'first-seen' key")
			}
			if _, ok := item["last-seen"]; !ok {
				t.Error("tag item missing 'last-seen' key")
			}
		}

		reporter.Record("Tags", "ListTags", "HTTP", !t.Failed(), "")
	})

	// -----------------------------------------------------------------------
	// 9.2 Get Tag
	// -----------------------------------------------------------------------

	t.Run("SDK_GetTag", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		tag, err := mg.GetTag(ctx, domain, "newsletter")
		if err != nil {
			reporter.Record("Tags", "GetTag", "SDK", false, err.Error())
			t.Fatalf("GetTag: %v", err)
		}

		if tag.Value != "newsletter" {
			t.Errorf("expected tag value 'newsletter', got %q", tag.Value)
		}

		reporter.Record("Tags", "GetTag", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_GetTag", func(t *testing.T) {
		resp, err := doRequest("GET", "/v3/"+domain+"/tags/newsletter", nil)
		if err != nil {
			reporter.Record("Tags", "GetTag", "HTTP", false, err.Error())
			t.Fatalf("GET tag: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["tag"] != "newsletter" {
			t.Errorf("expected tag 'newsletter', got %v", result["tag"])
		}
		if _, ok := result["first-seen"]; !ok {
			t.Error("missing 'first-seen' key")
		}
		if _, ok := result["last-seen"]; !ok {
			t.Error("missing 'last-seen' key")
		}

		reporter.Record("Tags", "GetTag", "HTTP", !t.Failed(), "")
	})

	// -----------------------------------------------------------------------
	// 9.3 Update Tag Description
	// -----------------------------------------------------------------------

	t.Run("HTTP_UpdateTagDescription", func(t *testing.T) {
		resp, err := doFormRequest("PUT", "/v3/"+domain+"/tags/newsletter", map[string]string{
			"description": "Monthly newsletter tag",
		})
		if err != nil {
			reporter.Record("Tags", "UpdateTagDescription", "HTTP", false, err.Error())
			t.Fatalf("PUT tag: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "Tag updated" {
			t.Errorf("expected message 'Tag updated', got %v", result["message"])
		}

		// Verify the description was actually updated
		resp2, err := doRequest("GET", "/v3/"+domain+"/tags/newsletter", nil)
		if err != nil {
			t.Fatalf("GET tag after update: %v", err)
		}
		var tagResult map[string]interface{}
		readJSON(t, resp2, &tagResult)

		if tagResult["description"] != "Monthly newsletter tag" {
			t.Errorf("expected description 'Monthly newsletter tag', got %v", tagResult["description"])
		}

		reporter.Record("Tags", "UpdateTagDescription", "HTTP", !t.Failed(), "")
	})

	// -----------------------------------------------------------------------
	// 9.4 Delete Tag
	// -----------------------------------------------------------------------

	t.Run("SDK_DeleteTag", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		err := mg.DeleteTag(ctx, domain, "updates")
		if err != nil {
			reporter.Record("Tags", "DeleteTag", "SDK", false, err.Error())
			t.Fatalf("DeleteTag: %v", err)
		}

		// Verify the tag is gone
		_, err = mg.GetTag(ctx, domain, "updates")
		if err == nil {
			t.Error("expected error when getting deleted tag, got nil")
		}

		reporter.Record("Tags", "DeleteTag", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_DeleteTag", func(t *testing.T) {
		resp, err := doRequest("DELETE", "/v3/"+domain+"/tags/promo", nil)
		if err != nil {
			reporter.Record("Tags", "DeleteTag", "HTTP", false, err.Error())
			t.Fatalf("DELETE tag: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "Tag has been removed" {
			t.Errorf("expected message 'Tag has been removed', got %v", result["message"])
		}

		// Verify the tag is gone
		resp2, err := doRequest("GET", "/v3/"+domain+"/tags/promo", nil)
		if err != nil {
			t.Fatalf("GET deleted tag: %v", err)
		}
		if resp2.StatusCode != http.StatusNotFound {
			resp2.Body.Close()
			t.Errorf("expected 404 for deleted tag, got %d", resp2.StatusCode)
		} else {
			resp2.Body.Close()
		}

		reporter.Record("Tags", "DeleteTag", "HTTP", !t.Failed(), "")
	})

	// -----------------------------------------------------------------------
	// 9.5 Get Tag Stats
	// -----------------------------------------------------------------------

	t.Run("HTTP_GetTagStats", func(t *testing.T) {
		resp, err := doRequest("GET", "/v3/"+domain+"/tags/newsletter/stats?event=delivered&event=accepted", nil)
		if err != nil {
			reporter.Record("Tags", "GetTagStats", "HTTP", false, err.Error())
			t.Fatalf("GET tag stats: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["tag"] != "newsletter" {
			t.Errorf("expected tag 'newsletter', got %v", result["tag"])
		}
		if result["resolution"] == nil {
			t.Error("expected 'resolution' in response")
		}
		if result["start"] == nil {
			t.Error("expected 'start' in response")
		}
		if result["end"] == nil {
			t.Error("expected 'end' in response")
		}

		stats, ok := result["stats"].([]interface{})
		if !ok {
			t.Fatal("expected 'stats' to be an array")
		}
		if len(stats) == 0 {
			t.Error("expected at least one stats bucket")
		}

		// Verify each bucket has expected structure
		if len(stats) > 0 {
			bucket, ok := stats[0].(map[string]interface{})
			if !ok {
				t.Fatal("expected stats bucket to be a map")
			}
			if _, ok := bucket["time"]; !ok {
				t.Error("stats bucket missing 'time' key")
			}
			if _, ok := bucket["delivered"]; !ok {
				t.Error("stats bucket missing 'delivered' key")
			}
			if _, ok := bucket["accepted"]; !ok {
				t.Error("stats bucket missing 'accepted' key")
			}
		}

		reporter.Record("Tags", "GetTagStats", "HTTP", !t.Failed(), "")
	})

	// -----------------------------------------------------------------------
	// 9.6 Get Tag Aggregates (Countries)
	// -----------------------------------------------------------------------

	t.Run("HTTP_GetTagAggregatesCountries", func(t *testing.T) {
		resp, err := doRequest("GET", "/v3/"+domain+"/tags/newsletter/stats/aggregates/countries", nil)
		if err != nil {
			reporter.Record("Tags", "GetTagAggregatesCountries", "HTTP", false, err.Error())
			t.Fatalf("GET tag aggregates countries: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["tag"] != "newsletter" {
			t.Errorf("expected tag 'newsletter', got %v", result["tag"])
		}
		if _, ok := result["countries"]; !ok {
			t.Error("expected 'countries' key in response")
		}

		reporter.Record("Tags", "GetTagAggregatesCountries", "HTTP", !t.Failed(), "")
	})

	// -----------------------------------------------------------------------
	// 9.7 Get Tag Aggregates (Providers)
	// -----------------------------------------------------------------------

	t.Run("HTTP_GetTagAggregatesProviders", func(t *testing.T) {
		resp, err := doRequest("GET", "/v3/"+domain+"/tags/newsletter/stats/aggregates/providers", nil)
		if err != nil {
			reporter.Record("Tags", "GetTagAggregatesProviders", "HTTP", false, err.Error())
			t.Fatalf("GET tag aggregates providers: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["tag"] != "newsletter" {
			t.Errorf("expected tag 'newsletter', got %v", result["tag"])
		}
		if _, ok := result["providers"]; !ok {
			t.Error("expected 'providers' key in response")
		}

		reporter.Record("Tags", "GetTagAggregatesProviders", "HTTP", !t.Failed(), "")
	})

	// -----------------------------------------------------------------------
	// 9.8 Get Tag Aggregates (Devices)
	// -----------------------------------------------------------------------

	t.Run("HTTP_GetTagAggregatesDevices", func(t *testing.T) {
		resp, err := doRequest("GET", "/v3/"+domain+"/tags/newsletter/stats/aggregates/devices", nil)
		if err != nil {
			reporter.Record("Tags", "GetTagAggregatesDevices", "HTTP", false, err.Error())
			t.Fatalf("GET tag aggregates devices: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["tag"] != "newsletter" {
			t.Errorf("expected tag 'newsletter', got %v", result["tag"])
		}
		if _, ok := result["devices"]; !ok {
			t.Error("expected 'devices' key in response")
		}

		reporter.Record("Tags", "GetTagAggregatesDevices", "HTTP", !t.Failed(), "")
	})

	// -----------------------------------------------------------------------
	// 9.9 Get Tag Limits
	// -----------------------------------------------------------------------

	t.Run("SDK_GetTagLimits", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		limits, err := mg.GetTagLimits(ctx, domain)
		if err != nil {
			reporter.Record("Tags", "GetTagLimits", "SDK", false, err.Error())
			t.Fatalf("GetTagLimits: %v", err)
		}

		if limits.Limit != 5000 {
			t.Errorf("expected limit 5000, got %d", limits.Limit)
		}
		// After deleting 2 tags (updates + promo), we should have 1 remaining (newsletter)
		if limits.Count != 1 {
			t.Errorf("expected count 1, got %d", limits.Count)
		}

		reporter.Record("Tags", "GetTagLimits", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_GetTagLimits", func(t *testing.T) {
		resp, err := doRequest("GET", "/v3/domains/"+domain+"/limits/tag", nil)
		if err != nil {
			reporter.Record("Tags", "GetTagLimits", "HTTP", false, err.Error())
			t.Fatalf("GET tag limits: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["id"] != domain {
			t.Errorf("expected id %q, got %v", domain, result["id"])
		}

		limit, ok := result["limit"].(float64)
		if !ok || limit != 5000 {
			t.Errorf("expected limit 5000, got %v", result["limit"])
		}

		count, ok := result["count"].(float64)
		if !ok {
			t.Error("expected 'count' key in response")
		} else if int(count) != 1 {
			t.Errorf("expected count 1, got %v", result["count"])
		}

		reporter.Record("Tags", "GetTagLimits", "HTTP", !t.Failed(), "")
	})
}

// Ensure imports are used.
var _ = strings.Contains
var _ = fmt.Sprintf
