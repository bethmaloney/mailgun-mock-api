package integration

import "testing"

func TestWebhooks(t *testing.T) {
	resetServer(t)

	// --- Domain Webhooks (v3) ---

	t.Run("SDK_CreateWebhook", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_CreateWebhook", func(t *testing.T) {
		t.Skip("TODO: implement — POST /v3/domains/{domain}/webhooks")
	})

	t.Run("SDK_ListWebhooks", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_ListWebhooks", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/domains/{domain}/webhooks")
	})

	t.Run("SDK_GetWebhook", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_GetWebhook", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/domains/{domain}/webhooks/{name}")
	})

	t.Run("SDK_UpdateWebhook", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_UpdateWebhook", func(t *testing.T) {
		t.Skip("TODO: implement — PUT /v3/domains/{domain}/webhooks/{name}")
	})

	t.Run("SDK_DeleteWebhook", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_DeleteWebhook", func(t *testing.T) {
		t.Skip("TODO: implement — DELETE /v3/domains/{domain}/webhooks/{name}")
	})

	// --- Account Webhooks (v1) ---

	t.Run("HTTP_CreateAccountWebhook", func(t *testing.T) {
		t.Skip("TODO: implement — POST /v1/webhooks")
	})

	t.Run("HTTP_ListAccountWebhooks", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v1/webhooks")
	})

	t.Run("HTTP_GetAccountWebhook", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v1/webhooks/{id}")
	})

	t.Run("HTTP_UpdateAccountWebhook", func(t *testing.T) {
		t.Skip("TODO: implement — PUT /v1/webhooks/{id}")
	})

	t.Run("HTTP_DeleteAccountWebhook", func(t *testing.T) {
		t.Skip("TODO: implement — DELETE /v1/webhooks/{id}")
	})

	// --- Webhook Signing Key (v5) ---

	t.Run("HTTP_GetSigningKey", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v5/accounts/http_signing_key")
	})

	t.Run("HTTP_RotateSigningKey", func(t *testing.T) {
		t.Skip("TODO: implement — POST /v5/accounts/http_signing_key")
	})
}
