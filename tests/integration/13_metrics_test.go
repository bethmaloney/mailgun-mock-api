package integration

import "testing"

func TestMetrics(t *testing.T) {
	resetServer(t)

	t.Run("SDK_GetDomainStats", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_GetDomainStats", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/{domain}/stats/total")
	})

	t.Run("SDK_GetAccountStats", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_GetAccountStats", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/stats/total")
	})

	t.Run("SDK_ListMetrics", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_QueryMetricsAPI", func(t *testing.T) {
		t.Skip("TODO: implement — POST /v1/analytics/metrics")
	})

	t.Run("HTTP_QueryUsageMetrics", func(t *testing.T) {
		t.Skip("TODO: implement — POST /v1/analytics/usage/metrics")
	})

	t.Run("HTTP_GetDomainAggregatesProviders", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/{domain}/aggregates/providers")
	})

	t.Run("HTTP_GetDomainAggregatesCountries", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/{domain}/aggregates/countries")
	})

	t.Run("HTTP_GetDomainAggregatesDevices", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/{domain}/aggregates/devices")
	})
}
