package webhook_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bethmaloney/mailgun-mock-api/internal/domain"
	"github.com/bethmaloney/mailgun-mock-api/internal/event"
	"github.com/bethmaloney/mailgun-mock-api/internal/message"
	"github.com/bethmaloney/mailgun-mock-api/internal/mock"
	"github.com/bethmaloney/mailgun-mock-api/internal/webhook"
	"github.com/go-chi/chi/v5"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// Test Helpers (unique names to avoid conflicts with handlers_test.go)
// ---------------------------------------------------------------------------

// setupAccountTestDB creates an in-memory SQLite database with all models
// needed for account webhook and delivery pipeline tests.
func setupAccountTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}
	if err := db.AutoMigrate(
		&domain.Domain{}, &domain.DNSRecord{},
		&webhook.DomainWebhook{},
		&webhook.AccountWebhook{},
		&webhook.WebhookDelivery{},
		&message.StoredMessage{},
		&event.Event{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

// accountDefaultConfig returns a MockConfig suitable for account webhook tests.
func accountDefaultConfig() *mock.MockConfig {
	return &mock.MockConfig{
		DomainBehavior: mock.DomainBehaviorConfig{
			DomainAutoVerify: true,
			SandboxDomain:    "sandbox123.mailgun.org",
		},
		EventGeneration: mock.EventGenerationConfig{
			AutoDeliver:               true,
			DeliveryDelayMs:           0,
			DefaultDeliveryStatusCode: 250,
			AutoFailRate:              0.0,
		},
		Authentication: mock.AuthenticationConfig{
			AuthMode:   "accept_any",
			SigningKey:  "key-mock-signing-key-000000000000",
		},
	}
}

// setupAccountRouter creates a chi router with v1 account webhook routes,
// mock webhook inspection routes, domain routes, v3 domain webhook routes,
// and message sending routes.
func setupAccountRouter(db *gorm.DB, cfg *mock.MockConfig) http.Handler {
	domain.ResetForTests(db)
	event.ResetForTests(db)
	dh := domain.NewHandlers(db, cfg)
	wh := webhook.NewHandlers(db, cfg)
	eh := event.NewHandlers(db, cfg)
	mh := message.NewHandlers(db, cfg)
	mh.SetEventHandlers(eh)

	r := chi.NewRouter()

	// Domain creation
	r.Route("/v4/domains", func(r chi.Router) {
		r.Post("/", dh.CreateDomain)
	})

	// v3 domain webhook routes (for trigger test setup)
	r.Route("/v3/domains/{domain_name}/webhooks", func(r chi.Router) {
		r.Get("/", wh.ListWebhooks)
		r.Post("/", wh.CreateWebhook)
	})

	// Message sending (for delivery pipeline tests)
	r.Post("/v3/{domain_name}/messages", mh.SendMessage)

	// v1 account webhook routes
	r.Route("/v1/webhooks", func(r chi.Router) {
		r.Get("/", wh.ListAccountWebhooks)
		r.Post("/", wh.CreateAccountWebhook)
		r.Delete("/", wh.BulkDeleteAccountWebhooks)
		r.Get("/{webhook_id}", wh.GetAccountWebhook)
		r.Put("/{webhook_id}", wh.UpdateAccountWebhook)
		r.Delete("/{webhook_id}", wh.DeleteAccountWebhook)
	})

	// Mock webhook inspection routes
	r.Get("/mock/webhooks/deliveries", wh.ListDeliveries)
	r.Post("/mock/webhooks/trigger", wh.TriggerWebhook)

	return r
}

// setupAccount creates a fresh database, router, and test domain for account
// webhook tests. Returns the router and database.
func setupAccount(t *testing.T) (http.Handler, *gorm.DB) {
	t.Helper()
	db := setupAccountTestDB(t)
	cfg := accountDefaultConfig()
	router := setupAccountRouter(db, cfg)

	// Create the test domain
	rec := doRequest(t, router, "POST", "/v4/domains", map[string]string{
		"name": testDomain,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to create test domain: %s", rec.Body.String())
	}

	return router, db
}

// doAccountMultipart sends a multipart/form-data request with potentially
// repeated keys. This is a local helper identical in spirit to doRequestMultiValue
// but named uniquely to be self-contained.
func doAccountMultipart(t *testing.T, router http.Handler, method, urlStr string, fields []fieldPair) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for _, f := range fields {
		if err := writer.WriteField(f.key, f.value); err != nil {
			t.Fatalf("failed to write field %q: %v", f.key, err)
		}
	}
	writer.Close()
	req := httptest.NewRequest(method, urlStr, &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	router.ServeHTTP(rec, req)
	return rec
}

// doAccountJSON sends a request with a JSON body.
func doAccountJSON(t *testing.T, router http.Handler, method, urlStr string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("failed to encode JSON body: %v", err)
		}
	}
	req := httptest.NewRequest(method, urlStr, &buf)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	return rec
}

// createAccountWebhook is a helper that creates an account webhook and returns
// the webhook_id from the response. It fails the test if creation is unsuccessful.
func createAccountWebhook(t *testing.T, router http.Handler, url string, eventTypes []string, description string) string {
	t.Helper()
	fields := []fieldPair{
		{key: "url", value: url},
	}
	for _, et := range eventTypes {
		fields = append(fields, fieldPair{key: "event_types", value: et})
	}
	if description != "" {
		fields = append(fields, fieldPair{key: "description", value: description})
	}

	rec := doAccountMultipart(t, router, "POST", "/v1/webhooks", fields)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)
	webhookID, ok := body["webhook_id"].(string)
	if !ok || webhookID == "" {
		t.Fatalf("expected non-empty 'webhook_id' in create response, got: %v", body)
	}
	return webhookID
}

// ---------------------------------------------------------------------------
// v1 Account Webhooks Tests
// ---------------------------------------------------------------------------

func TestListAccountWebhooks_Empty(t *testing.T) {
	router, _ := setupAccount(t)

	rec := doRequest(t, router, "GET", "/v1/webhooks", nil)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	webhooksRaw, ok := body["webhooks"]
	if !ok {
		t.Fatalf("expected 'webhooks' key in response, got: %v", body)
	}

	webhooks, ok := webhooksRaw.([]interface{})
	if !ok {
		t.Fatalf("expected 'webhooks' to be an array, got %T: %v", webhooksRaw, webhooksRaw)
	}

	if len(webhooks) != 0 {
		t.Errorf("expected empty webhooks array, got %d items: %v", len(webhooks), webhooks)
	}
}

func TestCreateAccountWebhook(t *testing.T) {
	router, _ := setupAccount(t)

	rec := doAccountMultipart(t, router, "POST", "/v1/webhooks", []fieldPair{
		{key: "url", value: "https://example.com/account-hook"},
		{key: "event_types", value: "delivered"},
		{key: "event_types", value: "opened"},
		{key: "description", value: "My test webhook"},
	})
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	webhookID, ok := body["webhook_id"].(string)
	if !ok || webhookID == "" {
		t.Fatalf("expected non-empty 'webhook_id' in response, got: %v", body)
	}

	// The webhook_id should start with "wh_"
	if !strings.HasPrefix(webhookID, "wh_") {
		t.Errorf("expected webhook_id to start with 'wh_', got %q", webhookID)
	}
}

func TestCreateAccountWebhook_MissingURL(t *testing.T) {
	router, _ := setupAccount(t)

	rec := doAccountMultipart(t, router, "POST", "/v1/webhooks", []fieldPair{
		{key: "event_types", value: "delivered"},
	})
	assertStatus(t, rec, http.StatusBadRequest)
}

func TestCreateAccountWebhook_MissingEventTypes(t *testing.T) {
	router, _ := setupAccount(t)

	rec := doAccountMultipart(t, router, "POST", "/v1/webhooks", []fieldPair{
		{key: "url", value: "https://example.com/hook"},
	})
	assertStatus(t, rec, http.StatusBadRequest)
}

func TestCreateAccountWebhook_InvalidEventType(t *testing.T) {
	router, _ := setupAccount(t)

	rec := doAccountMultipart(t, router, "POST", "/v1/webhooks", []fieldPair{
		{key: "url", value: "https://example.com/hook"},
		{key: "event_types", value: "nonexistent_event"},
	})
	assertStatus(t, rec, http.StatusBadRequest)
}

func TestGetAccountWebhook(t *testing.T) {
	router, _ := setupAccount(t)

	// Create a webhook first.
	webhookID := createAccountWebhook(t, router,
		"https://example.com/get-test",
		[]string{"delivered", "opened"},
		"Get test webhook",
	)

	// GET the webhook by ID.
	rec := doRequest(t, router, "GET", fmt.Sprintf("/v1/webhooks/%s", webhookID), nil)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	// Verify fields.
	gotID, _ := body["webhook_id"].(string)
	if gotID != webhookID {
		t.Errorf("expected webhook_id %q, got %q", webhookID, gotID)
	}

	gotURL, _ := body["url"].(string)
	if gotURL != "https://example.com/get-test" {
		t.Errorf("expected url %q, got %q", "https://example.com/get-test", gotURL)
	}

	gotDesc, _ := body["description"].(string)
	if gotDesc != "Get test webhook" {
		t.Errorf("expected description %q, got %q", "Get test webhook", gotDesc)
	}

	// Verify event_types is an array containing "delivered" and "opened".
	eventTypesRaw, ok := body["event_types"].([]interface{})
	if !ok {
		t.Fatalf("expected 'event_types' to be an array, got %T: %v", body["event_types"], body["event_types"])
	}
	etSet := map[string]bool{}
	for _, et := range eventTypesRaw {
		etStr, _ := et.(string)
		etSet[etStr] = true
	}
	if !etSet["delivered"] {
		t.Errorf("expected 'delivered' in event_types, got: %v", eventTypesRaw)
	}
	if !etSet["opened"] {
		t.Errorf("expected 'opened' in event_types, got: %v", eventTypesRaw)
	}

	// Verify created_at is present and non-empty.
	createdAt, _ := body["created_at"].(string)
	if createdAt == "" {
		t.Errorf("expected non-empty 'created_at' in response")
	}
}

func TestGetAccountWebhook_NotFound(t *testing.T) {
	router, _ := setupAccount(t)

	rec := doRequest(t, router, "GET", "/v1/webhooks/wh_nonexistent", nil)
	assertStatus(t, rec, http.StatusNotFound)
}

func TestUpdateAccountWebhook(t *testing.T) {
	router, _ := setupAccount(t)

	// Create a webhook.
	webhookID := createAccountWebhook(t, router,
		"https://example.com/before-update",
		[]string{"delivered"},
		"Before update",
	)

	// Update the webhook with new fields.
	rec := doAccountMultipart(t, router, "PUT", fmt.Sprintf("/v1/webhooks/%s", webhookID), []fieldPair{
		{key: "url", value: "https://example.com/after-update"},
		{key: "event_types", value: "clicked"},
		{key: "event_types", value: "complained"},
		{key: "description", value: "After update"},
	})
	assertStatus(t, rec, http.StatusNoContent)

	// Verify the update by GETting the webhook.
	getRec := doRequest(t, router, "GET", fmt.Sprintf("/v1/webhooks/%s", webhookID), nil)
	assertStatus(t, getRec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, getRec, &body)

	gotURL, _ := body["url"].(string)
	if gotURL != "https://example.com/after-update" {
		t.Errorf("expected updated url %q, got %q", "https://example.com/after-update", gotURL)
	}

	gotDesc, _ := body["description"].(string)
	if gotDesc != "After update" {
		t.Errorf("expected updated description %q, got %q", "After update", gotDesc)
	}

	eventTypesRaw, ok := body["event_types"].([]interface{})
	if !ok {
		t.Fatalf("expected 'event_types' array after update, got %T", body["event_types"])
	}
	etSet := map[string]bool{}
	for _, et := range eventTypesRaw {
		etStr, _ := et.(string)
		etSet[etStr] = true
	}
	if etSet["delivered"] {
		t.Errorf("old event type 'delivered' should no longer be present after update")
	}
	if !etSet["clicked"] {
		t.Errorf("expected 'clicked' in updated event_types")
	}
	if !etSet["complained"] {
		t.Errorf("expected 'complained' in updated event_types")
	}
}

func TestUpdateAccountWebhook_NotFound(t *testing.T) {
	router, _ := setupAccount(t)

	rec := doAccountMultipart(t, router, "PUT", "/v1/webhooks/wh_nonexistent", []fieldPair{
		{key: "url", value: "https://example.com/nope"},
		{key: "event_types", value: "delivered"},
	})
	assertStatus(t, rec, http.StatusNotFound)
}

func TestDeleteAccountWebhook(t *testing.T) {
	router, _ := setupAccount(t)

	// Create a webhook.
	webhookID := createAccountWebhook(t, router,
		"https://example.com/to-delete",
		[]string{"delivered"},
		"",
	)

	// Delete it.
	rec := doRequest(t, router, "DELETE", fmt.Sprintf("/v1/webhooks/%s", webhookID), nil)
	assertStatus(t, rec, http.StatusNoContent)

	// Verify it is gone.
	getRec := doRequest(t, router, "GET", fmt.Sprintf("/v1/webhooks/%s", webhookID), nil)
	assertStatus(t, getRec, http.StatusNotFound)
}

func TestDeleteAccountWebhook_NotFound(t *testing.T) {
	router, _ := setupAccount(t)

	rec := doRequest(t, router, "DELETE", "/v1/webhooks/wh_nonexistent", nil)
	assertStatus(t, rec, http.StatusNotFound)
}

func TestBulkDeleteAccountWebhooks_ByIDs(t *testing.T) {
	router, _ := setupAccount(t)

	// Create two webhooks.
	id1 := createAccountWebhook(t, router,
		"https://example.com/bulk-1",
		[]string{"delivered"},
		"",
	)
	id2 := createAccountWebhook(t, router,
		"https://example.com/bulk-2",
		[]string{"opened"},
		"",
	)

	// Bulk delete by webhook_ids query param.
	deleteURL := fmt.Sprintf("/v1/webhooks?webhook_ids=%s,%s", id1, id2)
	rec := doRequest(t, router, "DELETE", deleteURL, nil)
	assertStatus(t, rec, http.StatusNoContent)

	// Verify both are gone.
	getRec1 := doRequest(t, router, "GET", fmt.Sprintf("/v1/webhooks/%s", id1), nil)
	assertStatus(t, getRec1, http.StatusNotFound)

	getRec2 := doRequest(t, router, "GET", fmt.Sprintf("/v1/webhooks/%s", id2), nil)
	assertStatus(t, getRec2, http.StatusNotFound)
}

func TestBulkDeleteAccountWebhooks_All(t *testing.T) {
	router, _ := setupAccount(t)

	// Create two webhooks.
	createAccountWebhook(t, router,
		"https://example.com/all-1",
		[]string{"delivered"},
		"",
	)
	createAccountWebhook(t, router,
		"https://example.com/all-2",
		[]string{"opened"},
		"",
	)

	// Bulk delete with all=true.
	rec := doRequest(t, router, "DELETE", "/v1/webhooks?all=true", nil)
	assertStatus(t, rec, http.StatusNoContent)

	// Verify the list is now empty.
	listRec := doRequest(t, router, "GET", "/v1/webhooks", nil)
	assertStatus(t, listRec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, listRec, &body)

	webhooks, ok := body["webhooks"].([]interface{})
	if !ok {
		t.Fatalf("expected 'webhooks' array, got %T: %v", body["webhooks"], body["webhooks"])
	}
	if len(webhooks) != 0 {
		t.Errorf("expected empty webhooks array after bulk delete all, got %d items", len(webhooks))
	}
}

func TestBulkDeleteAccountWebhooks_IDsTakePrecedenceOverAll(t *testing.T) {
	router, _ := setupAccount(t)

	// Create three webhooks.
	id1 := createAccountWebhook(t, router,
		"https://example.com/prec-1",
		[]string{"delivered"},
		"",
	)
	id2 := createAccountWebhook(t, router,
		"https://example.com/prec-2",
		[]string{"opened"},
		"",
	)
	id3 := createAccountWebhook(t, router,
		"https://example.com/prec-3",
		[]string{"clicked"},
		"",
	)

	// Bulk delete with both webhook_ids and all=true. webhook_ids should win.
	deleteURL := fmt.Sprintf("/v1/webhooks?webhook_ids=%s,%s&all=true", id1, id2)
	rec := doRequest(t, router, "DELETE", deleteURL, nil)
	assertStatus(t, rec, http.StatusNoContent)

	// id1 and id2 should be gone.
	assertStatus(t, doRequest(t, router, "GET", fmt.Sprintf("/v1/webhooks/%s", id1), nil), http.StatusNotFound)
	assertStatus(t, doRequest(t, router, "GET", fmt.Sprintf("/v1/webhooks/%s", id2), nil), http.StatusNotFound)

	// id3 should still exist (all=true was ignored because webhook_ids was present).
	getRec3 := doRequest(t, router, "GET", fmt.Sprintf("/v1/webhooks/%s", id3), nil)
	assertStatus(t, getRec3, http.StatusOK)
}

func TestListAccountWebhooks_FilterByIDs(t *testing.T) {
	router, _ := setupAccount(t)

	// Create three webhooks.
	id1 := createAccountWebhook(t, router,
		"https://example.com/filter-1",
		[]string{"delivered"},
		"",
	)
	_ = createAccountWebhook(t, router,
		"https://example.com/filter-2",
		[]string{"opened"},
		"",
	)
	id3 := createAccountWebhook(t, router,
		"https://example.com/filter-3",
		[]string{"clicked"},
		"",
	)

	// List with webhook_ids filter — only id1 and id3.
	listURL := fmt.Sprintf("/v1/webhooks?webhook_ids=%s,%s", id1, id3)
	rec := doRequest(t, router, "GET", listURL, nil)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	webhooks, ok := body["webhooks"].([]interface{})
	if !ok {
		t.Fatalf("expected 'webhooks' array, got %T: %v", body["webhooks"], body["webhooks"])
	}

	if len(webhooks) != 2 {
		t.Fatalf("expected 2 webhooks with filter, got %d: %v", len(webhooks), webhooks)
	}

	// Verify the returned IDs match.
	returnedIDs := map[string]bool{}
	for _, wRaw := range webhooks {
		w, ok := wRaw.(map[string]interface{})
		if !ok {
			t.Fatalf("expected webhook object, got %T", wRaw)
		}
		wid, _ := w["webhook_id"].(string)
		returnedIDs[wid] = true
	}
	if !returnedIDs[id1] {
		t.Errorf("expected webhook %s in filtered list", id1)
	}
	if !returnedIDs[id3] {
		t.Errorf("expected webhook %s in filtered list", id3)
	}
}

func TestListAccountWebhooks_AfterCreation(t *testing.T) {
	router, _ := setupAccount(t)

	// Create two webhooks.
	id1 := createAccountWebhook(t, router,
		"https://example.com/list-1",
		[]string{"delivered", "opened"},
		"First webhook",
	)
	id2 := createAccountWebhook(t, router,
		"https://example.com/list-2",
		[]string{"clicked"},
		"Second webhook",
	)

	// List all webhooks.
	rec := doRequest(t, router, "GET", "/v1/webhooks", nil)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	webhooks, ok := body["webhooks"].([]interface{})
	if !ok {
		t.Fatalf("expected 'webhooks' array, got %T: %v", body["webhooks"], body["webhooks"])
	}

	if len(webhooks) != 2 {
		t.Fatalf("expected 2 webhooks, got %d: %v", len(webhooks), webhooks)
	}

	// Build a lookup by webhook_id.
	webhooksByID := map[string]map[string]interface{}{}
	for _, wRaw := range webhooks {
		w, ok := wRaw.(map[string]interface{})
		if !ok {
			t.Fatalf("expected webhook object, got %T", wRaw)
		}
		wid, _ := w["webhook_id"].(string)
		webhooksByID[wid] = w
	}

	// Verify first webhook.
	w1, ok := webhooksByID[id1]
	if !ok {
		t.Fatalf("webhook %s not found in list", id1)
	}
	if url1, _ := w1["url"].(string); url1 != "https://example.com/list-1" {
		t.Errorf("expected url %q for webhook %s, got %q", "https://example.com/list-1", id1, url1)
	}
	if desc1, _ := w1["description"].(string); desc1 != "First webhook" {
		t.Errorf("expected description %q for webhook %s, got %q", "First webhook", id1, desc1)
	}
	etRaw1, ok := w1["event_types"].([]interface{})
	if !ok {
		t.Fatalf("expected event_types array for webhook %s", id1)
	}
	if len(etRaw1) != 2 {
		t.Errorf("expected 2 event_types for webhook %s, got %d", id1, len(etRaw1))
	}
	if _, ok := w1["created_at"].(string); !ok {
		t.Errorf("expected 'created_at' string for webhook %s", id1)
	}

	// Verify second webhook.
	w2, ok := webhooksByID[id2]
	if !ok {
		t.Fatalf("webhook %s not found in list", id2)
	}
	if url2, _ := w2["url"].(string); url2 != "https://example.com/list-2" {
		t.Errorf("expected url %q for webhook %s, got %q", "https://example.com/list-2", id2, url2)
	}
	if desc2, _ := w2["description"].(string); desc2 != "Second webhook" {
		t.Errorf("expected description %q for webhook %s, got %q", "Second webhook", id2, desc2)
	}
	etRaw2, ok := w2["event_types"].([]interface{})
	if !ok {
		t.Fatalf("expected event_types array for webhook %s", id2)
	}
	if len(etRaw2) != 1 {
		t.Errorf("expected 1 event_type for webhook %s, got %d", id2, len(etRaw2))
	}
}

// ---------------------------------------------------------------------------
// Mock Webhook Inspection Tests
// ---------------------------------------------------------------------------

func TestListDeliveries_Empty(t *testing.T) {
	router, _ := setupAccount(t)

	rec := doRequest(t, router, "GET", "/mock/webhooks/deliveries", nil)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	itemsRaw, ok := body["items"]
	if !ok {
		t.Fatalf("expected 'items' key in response, got: %v", body)
	}

	items, ok := itemsRaw.([]interface{})
	if !ok {
		t.Fatalf("expected 'items' to be an array, got %T: %v", itemsRaw, itemsRaw)
	}

	if len(items) != 0 {
		t.Errorf("expected empty items array, got %d items: %v", len(items), items)
	}
}

func TestTriggerWebhook(t *testing.T) {
	router, _ := setupAccount(t)

	// 1. Set up a domain webhook for "delivered" events so that there is
	//    a URL to deliver to.
	rec := doRequestMultiValue(t, router, "POST",
		fmt.Sprintf("/v3/domains/%s/webhooks", testDomain),
		[]fieldPair{
			{key: "id", value: "delivered"},
			{key: "url", value: "https://example.com/webhook-receiver"},
		},
	)
	assertStatus(t, rec, http.StatusOK)

	// 2. Send a message so a stored message + message ID exist.
	msgRec := doRequestMultiValue(t, router, "POST",
		fmt.Sprintf("/v3/%s/messages", testDomain),
		[]fieldPair{
			{key: "from", value: fmt.Sprintf("sender@%s", testDomain)},
			{key: "to", value: "recipient@example.com"},
			{key: "subject", value: "Trigger test"},
			{key: "text", value: "Hello from trigger test"},
		},
	)
	assertStatus(t, msgRec, http.StatusOK)

	// Extract the message ID from the send response.
	var msgBody map[string]interface{}
	decodeJSON(t, msgRec, &msgBody)
	messageID, _ := msgBody["id"].(string)
	if messageID == "" {
		t.Fatalf("expected non-empty 'id' in message send response, got: %v", msgBody)
	}

	// 3. Trigger webhook delivery via mock endpoint.
	triggerBody := map[string]interface{}{
		"domain":     testDomain,
		"event_type": "delivered",
		"recipient":  "recipient@example.com",
		"message_id": messageID,
	}
	triggerRec := doAccountJSON(t, router, "POST", "/mock/webhooks/trigger", triggerBody)

	// The trigger endpoint should return a success status (200 or 202).
	if triggerRec.Code != http.StatusOK && triggerRec.Code != http.StatusAccepted {
		t.Errorf("expected success status from trigger, got %d; body=%s",
			triggerRec.Code, triggerRec.Body.String())
	}

	// 4. Check the delivery log for the attempt.
	deliveriesRec := doRequest(t, router, "GET", "/mock/webhooks/deliveries", nil)
	assertStatus(t, deliveriesRec, http.StatusOK)

	var deliveriesBody map[string]interface{}
	decodeJSON(t, deliveriesRec, &deliveriesBody)

	items, ok := deliveriesBody["items"].([]interface{})
	if !ok {
		t.Fatalf("expected 'items' array in response, got %T: %v",
			deliveriesBody["items"], deliveriesBody["items"])
	}

	if len(items) == 0 {
		t.Fatal("expected at least one delivery attempt after trigger, got none")
	}

	// Verify the delivery entry has expected fields.
	delivery, ok := items[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected delivery object, got %T", items[0])
	}

	if webhookURL, _ := delivery["url"].(string); webhookURL != "https://example.com/webhook-receiver" {
		t.Errorf("expected url %q, got %q", "https://example.com/webhook-receiver", webhookURL)
	}

	if eventType, _ := delivery["event_type"].(string); eventType != "delivered" {
		t.Errorf("expected event_type %q, got %q", "delivered", eventType)
	}

	if domainName, _ := delivery["domain"].(string); domainName != testDomain {
		t.Errorf("expected domain %q, got %q", testDomain, domainName)
	}
}
