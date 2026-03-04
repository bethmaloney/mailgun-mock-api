package integration

import "testing"

func TestEvents(t *testing.T) {
	resetServer(t)

	t.Run("SDK_ListEvents", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_ListEvents", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/{domain}/events")
	})

	t.Run("SDK_PollEvents", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_PollEvents", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/{domain}/events (polling)")
	})

	t.Run("SDK_FilterByEventType", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_FilterByEventType", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/{domain}/events?event=delivered")
	})

	t.Run("SDK_FilterByRecipient", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_FilterByRecipient", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/{domain}/events?recipient=...")
	})

	t.Run("SDK_FilterByTimeRange", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_FilterByTimeRange", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/{domain}/events?begin=...&end=...")
	})

	t.Run("SDK_PaginateEvents", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_PaginateEvents", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/{domain}/events with pagination")
	})

	t.Run("HTTP_QueryLogsAPI", func(t *testing.T) {
		t.Skip("TODO: implement — POST /v1/analytics/logs")
	})
}
