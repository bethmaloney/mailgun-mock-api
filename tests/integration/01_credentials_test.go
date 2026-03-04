package integration

import "testing"

func TestCredentials(t *testing.T) {
	resetServer(t)

	// --- API Keys (v1) ---

	t.Run("SDK_ListAPIKeys", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_ListAPIKeys", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v1/keys")
	})

	t.Run("SDK_CreateAPIKey", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_CreateAPIKey", func(t *testing.T) {
		t.Skip("TODO: implement — POST /v1/keys")
	})

	t.Run("SDK_DeleteAPIKey", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_DeleteAPIKey", func(t *testing.T) {
		t.Skip("TODO: implement — DELETE /v1/keys/{key_id}")
	})

	t.Run("SDK_RegeneratePublicKey", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_RegeneratePublicKey", func(t *testing.T) {
		t.Skip("TODO: implement — POST /v1/keys/public")
	})

	// --- SMTP Credentials (v3) ---

	t.Run("SDK_ListCredentials", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_ListCredentials", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/domains/{domain}/credentials")
	})

	t.Run("SDK_CreateCredential", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_CreateCredential", func(t *testing.T) {
		t.Skip("TODO: implement — POST /v3/domains/{domain}/credentials")
	})

	t.Run("SDK_ChangeCredentialPassword", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_ChangeCredentialPassword", func(t *testing.T) {
		t.Skip("TODO: implement — PUT /v3/domains/{domain}/credentials/{login}")
	})

	t.Run("SDK_DeleteCredential", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_DeleteCredential", func(t *testing.T) {
		t.Skip("TODO: implement — DELETE /v3/domains/{domain}/credentials/{login}")
	})
}
