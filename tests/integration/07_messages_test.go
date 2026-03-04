package integration

import "testing"

func TestMessages(t *testing.T) {
	resetServer(t)

	// --- Sending ---

	t.Run("SDK_SendPlainText", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_SendPlainText", func(t *testing.T) {
		t.Skip("TODO: implement — POST /v3/{domain}/messages")
	})

	t.Run("SDK_SendHTML", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_SendHTML", func(t *testing.T) {
		t.Skip("TODO: implement — POST /v3/{domain}/messages with html field")
	})

	t.Run("SDK_SendWithAttachments", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_SendWithAttachments", func(t *testing.T) {
		t.Skip("TODO: implement — POST /v3/{domain}/messages with attachment")
	})

	t.Run("SDK_SendWithInlineImages", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_SendWithInlineImages", func(t *testing.T) {
		t.Skip("TODO: implement — POST /v3/{domain}/messages with inline")
	})

	t.Run("SDK_SendWithTags", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_SendWithTags", func(t *testing.T) {
		t.Skip("TODO: implement — POST /v3/{domain}/messages with o:tag")
	})

	t.Run("SDK_SendWithTemplate", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_SendWithTemplate", func(t *testing.T) {
		t.Skip("TODO: implement — POST /v3/{domain}/messages with template")
	})

	t.Run("SDK_SendWithCustomVariables", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_SendWithCustomVariables", func(t *testing.T) {
		t.Skip("TODO: implement — POST /v3/{domain}/messages with v: fields")
	})

	t.Run("SDK_SendWithRecipientVariables", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_SendWithRecipientVariables", func(t *testing.T) {
		t.Skip("TODO: implement — POST /v3/{domain}/messages with recipient-variables")
	})

	t.Run("SDK_SendScheduled", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_SendScheduled", func(t *testing.T) {
		t.Skip("TODO: implement — POST /v3/{domain}/messages with o:deliverytime")
	})

	t.Run("SDK_SendMIME", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_SendMIME", func(t *testing.T) {
		t.Skip("TODO: implement — POST /v3/{domain}/messages.mime")
	})

	t.Run("SDK_SendTestMode", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_SendTestMode", func(t *testing.T) {
		t.Skip("TODO: implement — POST /v3/{domain}/messages with o:testmode=yes")
	})

	t.Run("SDK_SendWithTrackingOverrides", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_SendWithTrackingOverrides", func(t *testing.T) {
		t.Skip("TODO: implement — POST /v3/{domain}/messages with o:tracking* fields")
	})

	t.Run("SDK_SendWithRequireTLS", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_SendWithRequireTLS", func(t *testing.T) {
		t.Skip("TODO: implement — POST /v3/{domain}/messages with o:require-tls")
	})

	// --- Stored Messages ---

	t.Run("SDK_GetStoredMessage", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_GetStoredMessage", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/domains/{domain}/messages/{key}")
	})

	t.Run("SDK_GetStoredMessageRaw", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_GetStoredMessageRaw", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/domains/{domain}/messages/{key} (raw MIME)")
	})

	t.Run("SDK_ResendStoredMessage", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_ResendStoredMessage", func(t *testing.T) {
		t.Skip("TODO: implement — POST /v3/domains/{domain}/messages/{key}")
	})

	// --- Sending Queues ---

	t.Run("HTTP_GetQueueStatus", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/{domain}/sending_queues")
	})

	t.Run("HTTP_ClearQueue", func(t *testing.T) {
		t.Skip("TODO: implement — DELETE /v3/{domain}/envelopes")
	})
}
