package integration

import "testing"

func TestMock(t *testing.T) {
	resetServer(t)

	// --- Dashboard & Messages ---

	t.Run("HTTP_DashboardSummary", func(t *testing.T) {
		t.Skip("TODO: implement — GET /mock/dashboard")
	})

	t.Run("HTTP_ListCapturedMessages", func(t *testing.T) {
		t.Skip("TODO: implement — GET /mock/messages")
	})

	t.Run("HTTP_GetCapturedMessage", func(t *testing.T) {
		t.Skip("TODO: implement — GET /mock/messages/{id}")
	})

	t.Run("HTTP_DeleteCapturedMessage", func(t *testing.T) {
		t.Skip("TODO: implement — DELETE /mock/messages/{id}")
	})

	// --- Event Triggers ---

	t.Run("HTTP_TriggerDeliver", func(t *testing.T) {
		t.Skip("TODO: implement — POST /mock/events/{domain}/deliver/{message_id}")
	})

	t.Run("HTTP_TriggerFail", func(t *testing.T) {
		t.Skip("TODO: implement — POST /mock/events/{domain}/fail/{message_id}")
	})

	t.Run("HTTP_TriggerOpen", func(t *testing.T) {
		t.Skip("TODO: implement — POST /mock/events/{domain}/open/{message_id}")
	})

	t.Run("HTTP_TriggerClick", func(t *testing.T) {
		t.Skip("TODO: implement — POST /mock/events/{domain}/click/{message_id}")
	})

	t.Run("HTTP_TriggerUnsubscribe", func(t *testing.T) {
		t.Skip("TODO: implement — POST /mock/events/{domain}/unsubscribe/{message_id}")
	})

	t.Run("HTTP_TriggerComplain", func(t *testing.T) {
		t.Skip("TODO: implement — POST /mock/events/{domain}/complain/{message_id}")
	})

	// --- Webhook Delivery Log ---

	t.Run("HTTP_WebhookDeliveryLog", func(t *testing.T) {
		t.Skip("TODO: implement — GET /mock/webhooks/deliveries")
	})

	// --- Inbound Simulation ---

	t.Run("HTTP_SimulateInbound", func(t *testing.T) {
		t.Skip("TODO: implement — POST /mock/inbound/{domain}")
	})

	// --- Config ---

	t.Run("HTTP_GetMockConfig", func(t *testing.T) {
		t.Skip("TODO: implement — GET /mock/config")
	})

	t.Run("HTTP_UpdateMockConfig", func(t *testing.T) {
		t.Skip("TODO: implement — PUT /mock/config")
	})

	// --- Reset ---

	t.Run("HTTP_ResetAllData", func(t *testing.T) {
		t.Skip("TODO: implement — POST /mock/reset")
	})

	// --- WebSocket ---

	t.Run("HTTP_WebSocketLiveUpdates", func(t *testing.T) {
		t.Skip("TODO: implement — WS /mock/ws")
	})
}
