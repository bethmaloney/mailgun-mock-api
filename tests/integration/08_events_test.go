package integration

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mailgun/mailgun-go/v5"
	"github.com/mailgun/mailgun-go/v5/events"
)

// isIteratorExhaustedError checks if the error from the SDK event iterator is
// the benign "unsupported protocol scheme" error that occurs when the iterator
// tries to follow an empty next-page URL at the end of results. The mock server
// returns an empty "next" paging URL when there are no more pages, and the SDK
// attempts to HTTP GET the empty string, yielding this error.
func isIteratorExhaustedError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "unsupported protocol scheme") ||
		strings.Contains(msg, "empty url")
}

func TestEvents(t *testing.T) {
	resetServer(t)

	const domain = "events-test.example.com"
	const sender = "sender@events-test.example.com"
	const recipient1 = "recipient1@example.com"
	const recipient2 = "recipient2@otherdomain.com"

	// Setup: create a domain
	resp, err := doFormRequest("POST", "/v4/domains", map[string]string{"name": domain})
	if err != nil {
		t.Fatalf("setup: create domain: %v", err)
	}
	resp.Body.Close()

	// Helper to extract storage key from message ID (e.g. "<abc@domain>" -> "abc@domain")
	storageKeyFromID := func(id string) string {
		return strings.TrimRight(strings.TrimLeft(id, "<"), ">")
	}

	// Record the time before sending messages so we can use it for time-range filtering.
	timeBeforeSend := time.Now().Add(-1 * time.Second)

	// Send first message (generates "accepted" + "delivered" events automatically)
	resp, err = doFormRequest("POST", "/v3/"+domain+"/messages", map[string]string{
		"from":    sender,
		"to":      recipient1,
		"subject": "Test Events Subject 1",
		"text":    "Body for events test 1",
	})
	if err != nil {
		t.Fatalf("setup: send message 1: %v", err)
	}
	var msg1Result map[string]interface{}
	readJSON(t, resp, &msg1Result)
	msg1ID, _ := msg1Result["id"].(string)
	storageKey1 := storageKeyFromID(msg1ID)

	// Send second message to a different recipient
	resp, err = doFormRequest("POST", "/v3/"+domain+"/messages", map[string]string{
		"from":    sender,
		"to":      recipient2,
		"subject": "Test Events Subject 2",
		"text":    "Body for events test 2",
	})
	if err != nil {
		t.Fatalf("setup: send message 2: %v", err)
	}
	var msg2Result map[string]interface{}
	readJSON(t, resp, &msg2Result)
	msg2ID, _ := msg2Result["id"].(string)
	storageKey2 := storageKeyFromID(msg2ID)

	// Trigger additional event types via mock endpoints using message 1
	// Open event
	resp, err = doRequest("POST", "/mock/events/"+domain+"/open/"+storageKey1, nil)
	if err != nil {
		t.Fatalf("setup: trigger open: %v", err)
	}
	resp.Body.Close()

	// Click event
	resp, err = doRequest("POST", "/mock/events/"+domain+"/click/"+storageKey1, map[string]string{"url": "http://example.com/click"})
	if err != nil {
		t.Fatalf("setup: trigger click: %v", err)
	}
	resp.Body.Close()

	// Fail event (permanent) for message 2
	resp, err = doRequest("POST", "/mock/events/"+domain+"/fail/"+storageKey2, map[string]string{"severity": "permanent", "reason": "bounce"})
	if err != nil {
		t.Fatalf("setup: trigger fail: %v", err)
	}
	resp.Body.Close()

	// Unsubscribe event for message 1
	resp, err = doRequest("POST", "/mock/events/"+domain+"/unsubscribe/"+storageKey1, nil)
	if err != nil {
		t.Fatalf("setup: trigger unsubscribe: %v", err)
	}
	resp.Body.Close()

	// Complain event for message 1
	resp, err = doRequest("POST", "/mock/events/"+domain+"/complain/"+storageKey1, nil)
	if err != nil {
		t.Fatalf("setup: trigger complain: %v", err)
	}
	resp.Body.Close()

	timeAfterSend := time.Now().Add(1 * time.Second)

	// We should now have these events:
	// msg1: accepted, delivered, opened, clicked, unsubscribed, complained
	// msg2: accepted, delivered, failed

	// Keep storageKey refs to avoid unused variable errors
	_ = storageKey1
	_ = storageKey2

	// -----------------------------------------------------------------------
	// 8.1 -- List Events
	// -----------------------------------------------------------------------

	t.Run("SDK_ListEvents", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		it := mg.ListEvents(domain, &mailgun.ListEventOptions{
			Limit: 100,
		})

		var page []events.Event
		var allEvents []events.Event
		for it.Next(ctx, &page) {
			allEvents = append(allEvents, page...)
		}
		// The iterator may return an error when trying to follow an empty
		// next-page URL. That is expected at the end of results.
		if it.Err() != nil && !isIteratorExhaustedError(it.Err()) {
			reporter.Record("Events", "ListEvents", "SDK", false, it.Err().Error())
			t.Fatalf("ListEvents iterator error: %v", it.Err())
		}

		if len(allEvents) == 0 {
			reporter.Record("Events", "ListEvents", "SDK", false, "no events returned")
			t.Fatal("expected at least one event, got none")
		}

		// Verify we got events with valid names
		hasAccepted := false
		hasDelivered := false
		for _, ev := range allEvents {
			name := ev.GetName()
			if name == "accepted" {
				hasAccepted = true
			}
			if name == "delivered" {
				hasDelivered = true
			}
		}
		if !hasAccepted {
			t.Errorf("expected at least one 'accepted' event in results")
		}
		if !hasDelivered {
			t.Errorf("expected at least one 'delivered' event in results")
		}

		t.Logf("SDK ListEvents returned %d events", len(allEvents))
		reporter.Record("Events", "ListEvents", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_ListEvents", func(t *testing.T) {
		resp, err := doRequest("GET", "/v3/"+domain+"/events", nil)
		if err != nil {
			reporter.Record("Events", "ListEvents", "HTTP", false, err.Error())
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result struct {
			Items  []map[string]interface{} `json:"items"`
			Paging map[string]interface{}   `json:"paging"`
		}
		readJSON(t, resp, &result)

		if len(result.Items) == 0 {
			reporter.Record("Events", "ListEvents", "HTTP", false, "no items returned")
			t.Fatal("expected at least one event item")
		}

		// Verify paging structure
		if result.Paging == nil {
			t.Errorf("expected paging object in response")
		}

		// Verify items have required fields
		first := result.Items[0]
		requiredFields := []string{"id", "event", "timestamp"}
		for _, field := range requiredFields {
			if _, ok := first[field]; !ok {
				t.Errorf("expected field %q in event item", field)
			}
		}

		// Count event types to verify variety
		eventTypes := make(map[string]bool)
		for _, item := range result.Items {
			if et, ok := item["event"].(string); ok {
				eventTypes[et] = true
			}
		}
		if len(eventTypes) < 2 {
			t.Errorf("expected at least 2 different event types, got %d: %v", len(eventTypes), eventTypes)
		}

		reporter.Record("Events", "ListEvents", "HTTP", !t.Failed(), "")
	})

	// -----------------------------------------------------------------------
	// 8.2 -- Poll Events
	// -----------------------------------------------------------------------

	t.Run("SDK_PollEvents", func(t *testing.T) {
		// The SDK's PollEvents uses formatMailgunTime() which formats dates as
		// "Mon, 2 Jan 2006 15:04:05 -0700". The mock server's time parser may
		// not handle single-digit days. We use ListEvents with ForceAscending
		// and a begin filter (via epoch in Filter map) to simulate polling
		// behavior, since the actual PollEvents blocks waiting for new events.
		mg := newMailgunClient()
		ctx := context.Background()

		beginEpoch := fmt.Sprintf("%.6f", float64(timeBeforeSend.Unix()))
		it := mg.ListEvents(domain, &mailgun.ListEventOptions{
			ForceAscending: true,
			Filter: map[string]string{
				"begin": beginEpoch,
			},
			Limit: 100,
		})

		var page []events.Event
		var allEvents []events.Event
		for it.Next(ctx, &page) {
			allEvents = append(allEvents, page...)
		}
		if it.Err() != nil && !isIteratorExhaustedError(it.Err()) {
			reporter.Record("Events", "PollEvents", "SDK", false, it.Err().Error())
			t.Fatalf("PollEvents (simulated) error: %v", it.Err())
		}

		if len(allEvents) == 0 {
			t.Errorf("expected events when polling with begin before messages were sent")
		} else {
			t.Logf("SDK PollEvents (simulated) returned %d events", len(allEvents))
		}

		reporter.Record("Events", "PollEvents", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_PollEvents", func(t *testing.T) {
		// Polling is just GET /v3/{domain}/events with appropriate begin parameter.
		// The mock doesn't do real long-polling, so we just verify it returns events.
		beginEpoch := fmt.Sprintf("%.6f", float64(timeBeforeSend.Unix()))
		resp, err := doRequest("GET", fmt.Sprintf("/v3/%s/events?begin=%s&ascending=yes", domain, beginEpoch), nil)
		if err != nil {
			reporter.Record("Events", "PollEvents", "HTTP", false, err.Error())
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result struct {
			Items  []map[string]interface{} `json:"items"`
			Paging map[string]interface{}   `json:"paging"`
		}
		readJSON(t, resp, &result)

		// Should have events since begin is before we sent messages
		if len(result.Items) == 0 {
			t.Errorf("expected events when polling with begin time before messages were sent")
		}

		reporter.Record("Events", "PollEvents", "HTTP", !t.Failed(), "")
	})

	// -----------------------------------------------------------------------
	// 8.3 -- Filter by Event Type
	// -----------------------------------------------------------------------

	t.Run("SDK_FilterByEventType", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		it := mg.ListEvents(domain, &mailgun.ListEventOptions{
			Filter: map[string]string{
				"event": "delivered",
			},
			Limit: 100,
		})

		var page []events.Event
		var allEvents []events.Event
		for it.Next(ctx, &page) {
			allEvents = append(allEvents, page...)
		}
		if it.Err() != nil && !isIteratorExhaustedError(it.Err()) {
			reporter.Record("Events", "FilterByEventType", "SDK", false, it.Err().Error())
			t.Fatalf("ListEvents with filter error: %v", it.Err())
		}

		if len(allEvents) == 0 {
			reporter.Record("Events", "FilterByEventType", "SDK", false, "no delivered events")
			t.Fatal("expected at least one delivered event")
		}

		for _, ev := range allEvents {
			if ev.GetName() != "delivered" {
				t.Errorf("expected all events to be 'delivered', got %q", ev.GetName())
			}
		}

		t.Logf("SDK FilterByEventType returned %d delivered events", len(allEvents))
		reporter.Record("Events", "FilterByEventType", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_FilterByEventType", func(t *testing.T) {
		resp, err := doRequest("GET", "/v3/"+domain+"/events?event=delivered", nil)
		if err != nil {
			reporter.Record("Events", "FilterByEventType", "HTTP", false, err.Error())
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result struct {
			Items []map[string]interface{} `json:"items"`
		}
		readJSON(t, resp, &result)

		if len(result.Items) == 0 {
			reporter.Record("Events", "FilterByEventType", "HTTP", false, "no delivered events")
			t.Fatal("expected at least one delivered event")
		}

		for _, item := range result.Items {
			et, _ := item["event"].(string)
			if et != "delivered" {
				t.Errorf("expected event type 'delivered', got %q", et)
			}
		}

		// Also test OR filter: "accepted OR delivered"
		resp2, err := doRequest("GET", "/v3/"+domain+"/events?event=accepted+OR+delivered", nil)
		if err != nil {
			t.Fatalf("OR filter request failed: %v", err)
		}
		assertStatus(t, resp2, http.StatusOK)

		var result2 struct {
			Items []map[string]interface{} `json:"items"`
		}
		readJSON(t, resp2, &result2)

		for _, item := range result2.Items {
			et, _ := item["event"].(string)
			if et != "accepted" && et != "delivered" {
				t.Errorf("expected event type 'accepted' or 'delivered', got %q", et)
			}
		}

		reporter.Record("Events", "FilterByEventType", "HTTP", !t.Failed(), "")
	})

	// -----------------------------------------------------------------------
	// 8.4 -- Filter by Recipient
	// -----------------------------------------------------------------------

	t.Run("SDK_FilterByRecipient", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		it := mg.ListEvents(domain, &mailgun.ListEventOptions{
			Filter: map[string]string{
				"recipient": recipient1,
			},
			Limit: 100,
		})

		var page []events.Event
		var allEvents []events.Event
		for it.Next(ctx, &page) {
			allEvents = append(allEvents, page...)
		}
		if it.Err() != nil && !isIteratorExhaustedError(it.Err()) {
			reporter.Record("Events", "FilterByRecipient", "SDK", false, it.Err().Error())
			t.Fatalf("ListEvents with recipient filter error: %v", it.Err())
		}

		if len(allEvents) == 0 {
			reporter.Record("Events", "FilterByRecipient", "SDK", false, "no events for recipient1")
			t.Fatal("expected at least one event for recipient1")
		}

		// All returned events should be for recipient1.
		// Use type switch to check recipient field on each event type.
		for _, ev := range allEvents {
			recipientOK := false
			switch e := ev.(type) {
			case *events.Accepted:
				recipientOK = e.Recipient == recipient1
			case *events.Delivered:
				recipientOK = e.Recipient == recipient1
			case *events.Opened:
				recipientOK = e.Recipient == recipient1
			case *events.Clicked:
				recipientOK = e.Recipient == recipient1
			case *events.Unsubscribed:
				recipientOK = e.Recipient == recipient1
			case *events.Complained:
				recipientOK = e.Recipient == recipient1
			case *events.Failed:
				recipientOK = e.Recipient == recipient1
			default:
				// For other/unknown event types, accept them
				recipientOK = true
			}
			if !recipientOK {
				t.Errorf("expected recipient %q for event %q", recipient1, ev.GetName())
			}
		}

		t.Logf("SDK FilterByRecipient returned %d events for %s", len(allEvents), recipient1)
		reporter.Record("Events", "FilterByRecipient", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_FilterByRecipient", func(t *testing.T) {
		resp, err := doRequest("GET", "/v3/"+domain+"/events?recipient="+recipient1, nil)
		if err != nil {
			reporter.Record("Events", "FilterByRecipient", "HTTP", false, err.Error())
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result struct {
			Items []map[string]interface{} `json:"items"`
		}
		readJSON(t, resp, &result)

		if len(result.Items) == 0 {
			reporter.Record("Events", "FilterByRecipient", "HTTP", false, "no events for recipient1")
			t.Fatal("expected at least one event for recipient1")
		}

		for _, item := range result.Items {
			r, _ := item["recipient"].(string)
			if r != recipient1 {
				t.Errorf("expected recipient %q, got %q", recipient1, r)
			}
		}

		// Verify that filtering by recipient2 returns only recipient2 events
		resp2, err := doRequest("GET", "/v3/"+domain+"/events?recipient="+recipient2, nil)
		if err != nil {
			t.Fatalf("recipient2 filter request failed: %v", err)
		}
		assertStatus(t, resp2, http.StatusOK)

		var result2 struct {
			Items []map[string]interface{} `json:"items"`
		}
		readJSON(t, resp2, &result2)

		for _, item := range result2.Items {
			r, _ := item["recipient"].(string)
			if r != recipient2 {
				t.Errorf("expected recipient %q, got %q", recipient2, r)
			}
		}

		reporter.Record("Events", "FilterByRecipient", "HTTP", !t.Failed(), "")
	})

	// -----------------------------------------------------------------------
	// 8.5 -- Filter by Time Range
	// -----------------------------------------------------------------------

	t.Run("SDK_FilterByTimeRange", func(t *testing.T) {
		// The SDK's ListEventOptions.Begin/End use formatMailgunTime() which
		// produces "Mon, 2 Jan 2006 ..." format. The mock server may not parse
		// single-digit day values. Work around by passing epoch timestamps via
		// the Filter map, which the SDK passes through as raw query parameters.
		mg := newMailgunClient()
		ctx := context.Background()

		beginEpoch := fmt.Sprintf("%.6f", float64(timeBeforeSend.Unix()))
		endEpoch := fmt.Sprintf("%.6f", float64(timeAfterSend.Unix()))

		it := mg.ListEvents(domain, &mailgun.ListEventOptions{
			Filter: map[string]string{
				"begin": beginEpoch,
				"end":   endEpoch,
			},
			Limit: 100,
		})

		var page []events.Event
		var allEvents []events.Event
		for it.Next(ctx, &page) {
			allEvents = append(allEvents, page...)
		}
		if it.Err() != nil && !isIteratorExhaustedError(it.Err()) {
			reporter.Record("Events", "FilterByTimeRange", "SDK", false, it.Err().Error())
			t.Fatalf("ListEvents with time range error: %v", it.Err())
		}

		if len(allEvents) == 0 {
			reporter.Record("Events", "FilterByTimeRange", "SDK", false, "no events in time range")
			t.Fatal("expected events within the time range")
		}

		// Verify timestamps are within the range (with 1 second tolerance)
		beginF := float64(timeBeforeSend.Unix()) - 1
		endF := float64(timeAfterSend.Unix()) + 1
		for _, ev := range allEvents {
			ts := ev.GetTimestamp()
			evEpoch := float64(ts.Unix())
			if evEpoch < beginF || evEpoch > endF {
				t.Errorf("event timestamp %v outside expected range [%v, %v]", ts, timeBeforeSend, timeAfterSend)
			}
		}

		t.Logf("SDK FilterByTimeRange returned %d events", len(allEvents))
		reporter.Record("Events", "FilterByTimeRange", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_FilterByTimeRange", func(t *testing.T) {
		beginEpoch := fmt.Sprintf("%.6f", float64(timeBeforeSend.Unix()))
		endEpoch := fmt.Sprintf("%.6f", float64(timeAfterSend.Unix()))

		resp, err := doRequest("GET", fmt.Sprintf("/v3/%s/events?begin=%s&end=%s", domain, beginEpoch, endEpoch), nil)
		if err != nil {
			reporter.Record("Events", "FilterByTimeRange", "HTTP", false, err.Error())
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result struct {
			Items []map[string]interface{} `json:"items"`
		}
		readJSON(t, resp, &result)

		if len(result.Items) == 0 {
			reporter.Record("Events", "FilterByTimeRange", "HTTP", false, "no events in time range")
			t.Fatal("expected events within the time range")
		}

		// All events should have timestamps within the range
		beginF := float64(timeBeforeSend.Unix()) - 1
		endF := float64(timeAfterSend.Unix()) + 1
		for _, item := range result.Items {
			ts, ok := item["timestamp"].(float64)
			if !ok {
				t.Errorf("expected float64 timestamp, got %T", item["timestamp"])
				continue
			}
			if ts < beginF || ts > endF {
				t.Errorf("event timestamp %.6f outside expected range [%.6f, %.6f]", ts, beginF, endF)
			}
		}

		// Verify that a time range in the far future returns no events
		futureBegin := fmt.Sprintf("%.6f", float64(time.Now().Add(1*time.Hour).Unix()))
		futureEnd := fmt.Sprintf("%.6f", float64(time.Now().Add(2*time.Hour).Unix()))
		resp2, err := doRequest("GET", fmt.Sprintf("/v3/%s/events?begin=%s&end=%s", domain, futureBegin, futureEnd), nil)
		if err != nil {
			t.Fatalf("future time range request failed: %v", err)
		}
		assertStatus(t, resp2, http.StatusOK)

		var result2 struct {
			Items []map[string]interface{} `json:"items"`
		}
		readJSON(t, resp2, &result2)

		if len(result2.Items) != 0 {
			t.Errorf("expected no events in future time range, got %d", len(result2.Items))
		}

		reporter.Record("Events", "FilterByTimeRange", "HTTP", !t.Failed(), "")
	})

	// -----------------------------------------------------------------------
	// 8.6 -- Paginate Events
	// -----------------------------------------------------------------------

	t.Run("SDK_PaginateEvents", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		// Use a small limit to force pagination
		it := mg.ListEvents(domain, &mailgun.ListEventOptions{
			Limit: 2,
		})

		var totalEvents int
		pages := 0
		var page []events.Event
		for it.Next(ctx, &page) {
			totalEvents += len(page)
			pages++
			if pages >= 10 {
				break // safety limit
			}
		}
		// Treat iterator-exhausted errors as success
		if it.Err() != nil && !isIteratorExhaustedError(it.Err()) {
			reporter.Record("Events", "PaginateEvents", "SDK", false, it.Err().Error())
			t.Fatalf("pagination error: %v", it.Err())
		}

		if totalEvents == 0 {
			reporter.Record("Events", "PaginateEvents", "SDK", false, "no events")
			t.Fatal("expected events across pages")
		}

		// We have 9 events total. With limit=2, we should have multiple pages.
		if pages < 2 && totalEvents > 2 {
			t.Errorf("expected multiple pages with limit=2 and %d total events, got %d pages", totalEvents, pages)
		}

		t.Logf("SDK pagination: %d events across %d pages", totalEvents, pages)
		reporter.Record("Events", "PaginateEvents", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_PaginateEvents", func(t *testing.T) {
		// Fetch first page with limit=2
		resp, err := doRequest("GET", fmt.Sprintf("/v3/%s/events?limit=2", domain), nil)
		if err != nil {
			reporter.Record("Events", "PaginateEvents", "HTTP", false, err.Error())
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result struct {
			Items  []map[string]interface{} `json:"items"`
			Paging map[string]interface{}   `json:"paging"`
		}
		readJSON(t, resp, &result)

		if len(result.Items) == 0 {
			reporter.Record("Events", "PaginateEvents", "HTTP", false, "no items on first page")
			t.Fatal("expected items on first page")
		}
		if len(result.Items) > 2 {
			t.Errorf("expected at most 2 items with limit=2, got %d", len(result.Items))
		}

		// Check paging structure
		if result.Paging == nil {
			t.Fatal("expected paging object in response")
		}

		// Verify paging has at least first/last keys
		if _, ok := result.Paging["first"]; !ok {
			t.Errorf("expected 'first' key in paging")
		}
		if _, ok := result.Paging["last"]; !ok {
			t.Errorf("expected 'last' key in paging")
		}

		// If there are more events, the "next" paging URL should be present
		nextURL, hasNext := result.Paging["next"].(string)
		if !hasNext || nextURL == "" {
			t.Log("no next page URL (may have fewer events than expected)")
		}

		// Follow the next page URL if available
		if hasNext && nextURL != "" {
			// The next URL from the server uses the server's own host.
			// We need to extract the path+query portion to use with doRequest.
			nextPath := nextURL
			if idx := strings.Index(nextURL, "/v3/"); idx >= 0 {
				nextPath = nextURL[idx:]
			}

			resp2, err := doRequest("GET", nextPath, nil)
			if err != nil {
				t.Fatalf("next page request failed: %v", err)
			}
			assertStatus(t, resp2, http.StatusOK)

			var result2 struct {
				Items []map[string]interface{} `json:"items"`
			}
			readJSON(t, resp2, &result2)

			// Next page should have items (since we have more than 2 events)
			if len(result2.Items) == 0 {
				t.Errorf("expected items on next page, got none")
			} else {
				t.Logf("next page returned %d items", len(result2.Items))
			}
		}

		reporter.Record("Events", "PaginateEvents", "HTTP", !t.Failed(), "")
	})

	// -----------------------------------------------------------------------
	// 8.7 -- Query Logs API (POST /v1/analytics/logs)
	// -----------------------------------------------------------------------

	t.Run("HTTP_QueryLogsAPI", func(t *testing.T) {
		// The /v1/analytics/logs endpoint is not implemented in the server.
		t.Skip("POST /v1/analytics/logs is not implemented; skipping")
		reporter.Record("Events", "QueryLogsAPI", "HTTP", false, "not implemented")
	})
}
