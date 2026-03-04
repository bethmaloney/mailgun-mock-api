package integration

import "testing"

func TestIPs(t *testing.T) {
	resetServer(t)

	// --- Account IPs ---

	t.Run("SDK_ListIPs", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_ListIPs", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/ips")
	})

	t.Run("SDK_GetIP", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_GetIP", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/ips/{ip}")
	})

	// --- Domain IPs ---

	t.Run("SDK_ListDomainIPs", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_ListDomainIPs", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/domains/{domain}/ips")
	})

	t.Run("SDK_AddDomainIP", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_AddDomainIP", func(t *testing.T) {
		t.Skip("TODO: implement — POST /v3/domains/{domain}/ips")
	})

	t.Run("SDK_DeleteDomainIP", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_DeleteDomainIP", func(t *testing.T) {
		t.Skip("TODO: implement — DELETE /v3/domains/{domain}/ips/{ip}")
	})

	// --- IP Pools ---

	t.Run("HTTP_ListIPPools", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v1/ip_pools")
	})

	t.Run("HTTP_CreateIPPool", func(t *testing.T) {
		t.Skip("TODO: implement — POST /v1/ip_pools")
	})

	t.Run("HTTP_GetIPPool", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v1/ip_pools/{pool_id}")
	})

	t.Run("HTTP_UpdateIPPool", func(t *testing.T) {
		t.Skip("TODO: implement — PATCH /v1/ip_pools/{pool_id}")
	})

	t.Run("HTTP_DeleteIPPool", func(t *testing.T) {
		t.Skip("TODO: implement — DELETE /v1/ip_pools/{pool_id}")
	})
}
