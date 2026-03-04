package integration

import "testing"

func TestDomains(t *testing.T) {
	resetServer(t)

	// --- Domain CRUD (v4) ---

	t.Run("SDK_CreateDomain", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_CreateDomain", func(t *testing.T) {
		t.Skip("TODO: implement — POST /v4/domains")
	})

	t.Run("SDK_ListDomains", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_ListDomains", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v4/domains")
	})

	t.Run("SDK_GetDomain", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_GetDomain", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v4/domains/{name}")
	})

	t.Run("SDK_UpdateDomain", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_UpdateDomain", func(t *testing.T) {
		t.Skip("TODO: implement — PUT /v4/domains/{name}")
	})

	t.Run("SDK_VerifyDomain", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_VerifyDomain", func(t *testing.T) {
		t.Skip("TODO: implement — PUT /v4/domains/{name}/verify")
	})

	t.Run("SDK_DeleteDomain", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_DeleteDomain", func(t *testing.T) {
		t.Skip("TODO: implement — DELETE /v3/domains/{name}")
	})

	// --- Connection Settings ---

	t.Run("SDK_GetDomainConnection", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_GetDomainConnection", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/domains/{domain}/connection")
	})

	t.Run("SDK_UpdateDomainConnection", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_UpdateDomainConnection", func(t *testing.T) {
		t.Skip("TODO: implement — PUT /v3/domains/{domain}/connection")
	})

	// --- Tracking Settings ---

	t.Run("SDK_GetDomainTracking", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_GetDomainTracking", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/domains/{domain}/tracking")
	})

	t.Run("SDK_UpdateOpenTracking", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_UpdateOpenTracking", func(t *testing.T) {
		t.Skip("TODO: implement — PUT /v3/domains/{domain}/tracking/open")
	})

	t.Run("SDK_UpdateClickTracking", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_UpdateClickTracking", func(t *testing.T) {
		t.Skip("TODO: implement — PUT /v3/domains/{domain}/tracking/click")
	})

	t.Run("SDK_UpdateUnsubscribeTracking", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_UpdateUnsubscribeTracking", func(t *testing.T) {
		t.Skip("TODO: implement — PUT /v3/domains/{domain}/tracking/unsubscribe")
	})

	// --- DKIM ---

	t.Run("SDK_UpdateDkimAuthority", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_UpdateDkimAuthority", func(t *testing.T) {
		t.Skip("TODO: implement — PUT /v3/domains/{domain}/dkim_authority")
	})

	t.Run("SDK_UpdateDkimSelector", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_UpdateDkimSelector", func(t *testing.T) {
		t.Skip("TODO: implement — PUT /v3/domains/{domain}/dkim_selector")
	})

	t.Run("SDK_ListDomainKeys", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_ListDomainKeys", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v1/dkim/keys")
	})

	t.Run("SDK_CreateDomainKey", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_CreateDomainKey", func(t *testing.T) {
		t.Skip("TODO: implement — POST /v1/dkim/keys")
	})

	t.Run("SDK_ActivateDomainKey", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_ActivateDomainKey", func(t *testing.T) {
		t.Skip("TODO: implement — PUT /v4/domains/{domain}/keys/{selector}/activate")
	})

	t.Run("SDK_DeactivateDomainKey", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_DeactivateDomainKey", func(t *testing.T) {
		t.Skip("TODO: implement — PUT /v4/domains/{domain}/keys/{selector}/deactivate")
	})

	t.Run("SDK_DeleteDomainKey", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_DeleteDomainKey", func(t *testing.T) {
		t.Skip("TODO: implement — DELETE /v1/dkim/keys/{key_id}")
	})
}
