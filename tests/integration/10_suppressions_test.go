package integration

import "testing"

func TestSuppressions(t *testing.T) {
	resetServer(t)

	// --- Bounces ---

	t.Run("SDK_AddBounce", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_AddBounce", func(t *testing.T) {
		t.Skip("TODO: implement — POST /v3/{domain}/bounces")
	})

	t.Run("SDK_ListBounces", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_ListBounces", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/{domain}/bounces")
	})

	t.Run("SDK_GetBounce", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_GetBounce", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/{domain}/bounces/{address}")
	})

	t.Run("SDK_DeleteBounce", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_DeleteBounce", func(t *testing.T) {
		t.Skip("TODO: implement — DELETE /v3/{domain}/bounces/{address}")
	})

	t.Run("SDK_DeleteBounceList", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_DeleteAllBounces", func(t *testing.T) {
		t.Skip("TODO: implement — DELETE /v3/{domain}/bounces")
	})

	t.Run("SDK_BulkImportBounces", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_BulkImportBounces", func(t *testing.T) {
		t.Skip("TODO: implement — POST /v3/{domain}/bounces (JSON array)")
	})

	// --- Complaints ---

	t.Run("SDK_CreateComplaint", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_CreateComplaint", func(t *testing.T) {
		t.Skip("TODO: implement — POST /v3/{domain}/complaints")
	})

	t.Run("SDK_ListComplaints", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_ListComplaints", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/{domain}/complaints")
	})

	t.Run("SDK_GetComplaint", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_GetComplaint", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/{domain}/complaints/{address}")
	})

	t.Run("SDK_DeleteComplaint", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_DeleteComplaint", func(t *testing.T) {
		t.Skip("TODO: implement — DELETE /v3/{domain}/complaints/{address}")
	})

	t.Run("SDK_BulkImportComplaints", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_BulkImportComplaints", func(t *testing.T) {
		t.Skip("TODO: implement — POST /v3/{domain}/complaints (JSON array)")
	})

	// --- Unsubscribes ---

	t.Run("SDK_CreateUnsubscribe", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_CreateUnsubscribe", func(t *testing.T) {
		t.Skip("TODO: implement — POST /v3/{domain}/unsubscribes")
	})

	t.Run("SDK_ListUnsubscribes", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_ListUnsubscribes", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/{domain}/unsubscribes")
	})

	t.Run("SDK_GetUnsubscribe", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_GetUnsubscribe", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/{domain}/unsubscribes/{address}")
	})

	t.Run("SDK_DeleteUnsubscribe", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_DeleteUnsubscribe", func(t *testing.T) {
		t.Skip("TODO: implement — DELETE /v3/{domain}/unsubscribes/{address}")
	})

	t.Run("SDK_DeleteUnsubscribeWithTag", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_DeleteUnsubscribeWithTag", func(t *testing.T) {
		t.Skip("TODO: implement — DELETE /v3/{domain}/unsubscribes/{address}?tag={tag}")
	})

	t.Run("SDK_BulkImportUnsubscribes", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_BulkImportUnsubscribes", func(t *testing.T) {
		t.Skip("TODO: implement — POST /v3/{domain}/unsubscribes (JSON array)")
	})

	// --- Allowlist (Whitelists) ---

	t.Run("HTTP_AddToAllowlist", func(t *testing.T) {
		t.Skip("TODO: implement — POST /v3/{domain}/whitelists")
	})

	t.Run("HTTP_ListAllowlist", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/{domain}/whitelists")
	})

	t.Run("HTTP_GetAllowlistEntry", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/{domain}/whitelists/{value}")
	})

	t.Run("HTTP_DeleteFromAllowlist", func(t *testing.T) {
		t.Skip("TODO: implement — DELETE /v3/{domain}/whitelists/{value}")
	})
}
