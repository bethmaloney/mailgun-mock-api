package integration

import "testing"

func TestTags(t *testing.T) {
	resetServer(t)

	t.Run("SDK_ListTags", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_ListTags", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/{domain}/tags")
	})

	t.Run("SDK_GetTag", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_GetTag", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/{domain}/tags/{tag}")
	})

	t.Run("HTTP_UpdateTagDescription", func(t *testing.T) {
		t.Skip("TODO: implement — PUT /v3/{domain}/tags/{tag}")
	})

	t.Run("SDK_DeleteTag", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_DeleteTag", func(t *testing.T) {
		t.Skip("TODO: implement — DELETE /v3/{domain}/tags/{tag}")
	})

	t.Run("HTTP_GetTagStats", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/{domain}/tags/{tag}/stats")
	})

	t.Run("HTTP_GetTagAggregatesCountries", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/{domain}/tags/{tag}/stats/aggregates/countries")
	})

	t.Run("HTTP_GetTagAggregatesProviders", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/{domain}/tags/{tag}/stats/aggregates/providers")
	})

	t.Run("HTTP_GetTagAggregatesDevices", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/{domain}/tags/{tag}/stats/aggregates/devices")
	})

	t.Run("SDK_GetTagLimits", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_GetTagLimits", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/domains/{domain}/limits/tag")
	})
}
