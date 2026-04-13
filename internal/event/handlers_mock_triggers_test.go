package event_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bethmaloney/mailgun-mock-api/internal/domain"
	"github.com/bethmaloney/mailgun-mock-api/internal/event"
	"github.com/bethmaloney/mailgun-mock-api/internal/message"
	"github.com/bethmaloney/mailgun-mock-api/internal/mock"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// Test Helpers (specific to mock trigger tests)
// ---------------------------------------------------------------------------

// setupRouterWithMockTriggers creates a chi router with domain, message, event,
// and mock event trigger routes registered. This extends setupRouter with the
// additional /mock/events routes.
func setupRouterWithMockTriggers(db *gorm.DB, cfg *mock.MockConfig) http.Handler {
	domain.ResetForTests(db)
	event.ResetForTests(db)
	dh := domain.NewHandlers(db, cfg)
	mh := message.NewHandlers(db, cfg)
	eh := event.NewHandlers(db, cfg)
	r := chi.NewRouter()
	// Domain routes for creating test domains
	r.Route("/v4/domains", func(r chi.Router) {
		r.Post("/", dh.CreateDomain)
	})
	// Message routes for sending test messages
	r.Route("/v3/{domain_name}/messages", func(r chi.Router) {
		r.Post("/", mh.SendMessage)
	})
	// Event routes
	r.Get("/v3/{domain_name}/events", eh.ListEvents)
	// Mock event trigger routes
	r.Route("/mock/events/{domain}", func(r chi.Router) {
		r.Post("/deliver/{message_id}", eh.TriggerDeliver)
		r.Post("/fail/{message_id}", eh.TriggerFail)
		r.Post("/open/{message_id}", eh.TriggerOpen)
		r.Post("/click/{message_id}", eh.TriggerClick)
		r.Post("/unsubscribe/{message_id}", eh.TriggerUnsubscribe)
		r.Post("/complain/{message_id}", eh.TriggerComplain)
	})
	return r
}

// triggerResponse is the expected JSON response from a mock event trigger endpoint.
type triggerResponse struct {
	Message string `json:"message"`
	EventID string `json:"event_id"`
}

// triggerEvent sends a POST request to the mock event trigger endpoint and
// returns the response recorder.
func triggerEvent(t *testing.T, router http.Handler, domainName, action, messageID string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	url := fmt.Sprintf("/mock/events/%s/%s/%s", domainName, action, messageID)
	var req *http.Request
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("failed to marshal trigger body: %v", err)
		}
		req = httptest.NewRequest(http.MethodPost, url, bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(http.MethodPost, url, nil)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// decodeTriggerResponse decodes a trigger response and returns it.
func decodeTriggerResponse(t *testing.T, rec *httptest.ResponseRecorder) triggerResponse {
	t.Helper()
	var resp triggerResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode trigger response (body=%q): %v", rec.Body.String(), err)
	}
	return resp
}

// findEventByType searches for an event of the given type in the events response items.
func findEventByType(items []eventItem, eventType string) *eventItem {
	for i := range items {
		if items[i].Event == eventType {
			return &items[i]
		}
	}
	return nil
}

// noAutoDeliverConfigForTriggers returns a config with auto-deliver disabled
// so that trigger tests do not have auto-generated delivered events confusing
// assertions.
func noAutoDeliverConfigForTriggers() *mock.MockConfig {
	cfg := defaultConfig()
	cfg.EventGeneration.AutoDeliver = false
	return cfg
}

// setupDomainAndMessage is a convenience helper that creates a test domain,
// sends a message, and returns the message ID and storage key. It uses the
// no-auto-deliver config to keep the event list clean for trigger tests.
func setupDomainAndMessage(t *testing.T, db *gorm.DB) (http.Handler, string, string, string) {
	t.Helper()
	cfg := noAutoDeliverConfigForTriggers()
	router := setupRouterWithMockTriggers(db, cfg)
	domainName := "triggers.example.com"
	createTestDomain(t, router, domainName)
	messageID, storageKey := sendTestMessage(t, router, domainName, defaultMessageFields(
		"sender@triggers.example.com", "recipient@example.com", "Trigger Test",
	))
	return router, domainName, messageID, storageKey
}

// ---------------------------------------------------------------------------
// Deliver Trigger Tests
// ---------------------------------------------------------------------------

func TestTriggerDeliver_HappyPath(t *testing.T) {
	db := setupTestDB(t)
	router, domainName, _, storageKey := setupDomainAndMessage(t, db)

	rec := triggerEvent(t, router, domainName, "deliver", storageKey, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeTriggerResponse(t, rec)
	if resp.Message != "Event created" {
		t.Errorf("expected message %q, got %q", "Event created", resp.Message)
	}
	if resp.EventID == "" {
		t.Error("expected non-empty event_id in response")
	}
}

func TestTriggerDeliver_AppearsInEventsList(t *testing.T) {
	db := setupTestDB(t)
	router, domainName, _, storageKey := setupDomainAndMessage(t, db)

	triggerRec := triggerEvent(t, router, domainName, "deliver", storageKey, nil)
	if triggerRec.Code != http.StatusOK {
		t.Fatalf("trigger failed: status=%d body=%s", triggerRec.Code, triggerRec.Body.String())
	}

	eventsRec := getEvents(t, router, domainName, map[string]string{"event": "delivered"})
	if eventsRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", eventsRec.Code, eventsRec.Body.String())
	}

	var resp eventsResponse
	decodeJSON(t, eventsRec, &resp)

	found := findEventByType(resp.Items, "delivered")
	if found == nil {
		t.Fatal("expected a delivered event in events list, found none")
	}
}

func TestTriggerDeliver_DomainNotFound(t *testing.T) {
	db := setupTestDB(t)
	cfg := noAutoDeliverConfigForTriggers()
	router := setupRouterWithMockTriggers(db, cfg)

	rec := triggerEvent(t, router, "nonexistent.example.com", "deliver", "fake-storage-key", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestTriggerDeliver_MessageNotFound(t *testing.T) {
	db := setupTestDB(t)
	cfg := noAutoDeliverConfigForTriggers()
	router := setupRouterWithMockTriggers(db, cfg)

	domainName := "triggers.example.com"
	createTestDomain(t, router, domainName)

	rec := triggerEvent(t, router, domainName, "deliver", "nonexistent-message-id", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestTriggerDeliver_EventFieldsVerification(t *testing.T) {
	db := setupTestDB(t)
	router, domainName, _, storageKey := setupDomainAndMessage(t, db)

	triggerRec := triggerEvent(t, router, domainName, "deliver", storageKey, nil)
	if triggerRec.Code != http.StatusOK {
		t.Fatalf("trigger failed: status=%d body=%s", triggerRec.Code, triggerRec.Body.String())
	}

	eventsRec := getEvents(t, router, domainName, map[string]string{"event": "delivered"})
	if eventsRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", eventsRec.Code, eventsRec.Body.String())
	}

	var resp eventsResponse
	decodeJSON(t, eventsRec, &resp)

	ev := findEventByType(resp.Items, "delivered")
	if ev == nil {
		t.Fatal("expected a delivered event, found none")
	}

	// Check log-level
	if ev.LogLevel != "info" {
		t.Errorf("expected log-level %q, got %q", "info", ev.LogLevel)
	}

	// Check recipient
	if ev.Recipient != "recipient@example.com" {
		t.Errorf("expected recipient %q, got %q", "recipient@example.com", ev.Recipient)
	}

	// Check that the raw response contains delivery-status with code 250
	var rawResp map[string]json.RawMessage
	if err := json.Unmarshal(eventsRec.Body.Bytes(), &rawResp); err != nil {
		t.Fatalf("failed to decode raw events response: %v", err)
	}
	var rawItems []map[string]interface{}
	if err := json.Unmarshal(rawResp["items"], &rawItems); err != nil {
		t.Fatalf("failed to decode raw items: %v", err)
	}

	var deliveredItem map[string]interface{}
	for _, item := range rawItems {
		if item["event"] == "delivered" {
			deliveredItem = item
			break
		}
	}
	if deliveredItem == nil {
		t.Fatal("delivered event not found in raw items")
	}

	deliveryStatus, ok := deliveredItem["delivery-status"].(map[string]interface{})
	if !ok {
		t.Fatal("expected delivery-status map in delivered event payload")
	}
	code, ok := deliveryStatus["code"].(float64)
	if !ok || code != 250 {
		t.Errorf("expected delivery-status code 250, got %v", deliveryStatus["code"])
	}

	// Check message headers
	msgField, ok := deliveredItem["message"].(map[string]interface{})
	if !ok {
		t.Fatal("expected message field in delivered event")
	}
	headers, ok := msgField["headers"].(map[string]interface{})
	if !ok {
		t.Fatal("expected headers in message field")
	}
	if headers["subject"] != "Trigger Test" {
		t.Errorf("expected subject %q in headers, got %v", "Trigger Test", headers["subject"])
	}
}

// ---------------------------------------------------------------------------
// Fail Trigger Tests
// ---------------------------------------------------------------------------

func TestTriggerFail_HappyPath(t *testing.T) {
	db := setupTestDB(t)
	router, domainName, _, storageKey := setupDomainAndMessage(t, db)

	rec := triggerEvent(t, router, domainName, "fail", storageKey, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeTriggerResponse(t, rec)
	if resp.Message != "Event created" {
		t.Errorf("expected message %q, got %q", "Event created", resp.Message)
	}
	if resp.EventID == "" {
		t.Error("expected non-empty event_id in response")
	}
}

func TestTriggerFail_AppearsInEventsList(t *testing.T) {
	db := setupTestDB(t)
	router, domainName, _, storageKey := setupDomainAndMessage(t, db)

	triggerRec := triggerEvent(t, router, domainName, "fail", storageKey, nil)
	if triggerRec.Code != http.StatusOK {
		t.Fatalf("trigger failed: status=%d body=%s", triggerRec.Code, triggerRec.Body.String())
	}

	eventsRec := getEvents(t, router, domainName, map[string]string{"event": "failed"})
	if eventsRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", eventsRec.Code, eventsRec.Body.String())
	}

	var resp eventsResponse
	decodeJSON(t, eventsRec, &resp)

	found := findEventByType(resp.Items, "failed")
	if found == nil {
		t.Fatal("expected a failed event in events list, found none")
	}
}

func TestTriggerFail_DomainNotFound(t *testing.T) {
	db := setupTestDB(t)
	cfg := noAutoDeliverConfigForTriggers()
	router := setupRouterWithMockTriggers(db, cfg)

	rec := triggerEvent(t, router, "nonexistent.example.com", "fail", "fake-storage-key", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestTriggerFail_MessageNotFound(t *testing.T) {
	db := setupTestDB(t)
	cfg := noAutoDeliverConfigForTriggers()
	router := setupRouterWithMockTriggers(db, cfg)

	domainName := "triggers.example.com"
	createTestDomain(t, router, domainName)

	rec := triggerEvent(t, router, domainName, "fail", "nonexistent-message-id", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestTriggerFail_DefaultSeverityAndReason(t *testing.T) {
	db := setupTestDB(t)
	router, domainName, _, storageKey := setupDomainAndMessage(t, db)

	triggerRec := triggerEvent(t, router, domainName, "fail", storageKey, nil)
	if triggerRec.Code != http.StatusOK {
		t.Fatalf("trigger failed: status=%d body=%s", triggerRec.Code, triggerRec.Body.String())
	}

	eventsRec := getEvents(t, router, domainName, map[string]string{"event": "failed"})
	if eventsRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", eventsRec.Code, eventsRec.Body.String())
	}

	var rawResp map[string]json.RawMessage
	if err := json.Unmarshal(eventsRec.Body.Bytes(), &rawResp); err != nil {
		t.Fatalf("failed to decode raw response: %v", err)
	}
	var items []map[string]interface{}
	if err := json.Unmarshal(rawResp["items"], &items); err != nil {
		t.Fatalf("failed to decode items: %v", err)
	}

	var failedItem map[string]interface{}
	for _, item := range items {
		if item["event"] == "failed" {
			failedItem = item
			break
		}
	}
	if failedItem == nil {
		t.Fatal("failed event not found")
	}

	// Default severity should be "permanent"
	severity, _ := failedItem["severity"].(string)
	if severity != "permanent" {
		t.Errorf("expected default severity %q, got %q", "permanent", severity)
	}

	// Default reason should be "bounce"
	reason, _ := failedItem["reason"].(string)
	if reason != "bounce" {
		t.Errorf("expected default reason %q, got %q", "bounce", reason)
	}

	// Log level should be "error" for permanent failure
	logLevel, _ := failedItem["log-level"].(string)
	if logLevel != "error" {
		t.Errorf("expected log-level %q for permanent failure, got %q", "error", logLevel)
	}

	// Delivery status code should be 550 for permanent failure
	deliveryStatus, ok := failedItem["delivery-status"].(map[string]interface{})
	if !ok {
		t.Fatal("expected delivery-status in failed event")
	}
	code, _ := deliveryStatus["code"].(float64)
	if code != 550 {
		t.Errorf("expected delivery-status code 550, got %v", code)
	}
}

func TestTriggerFail_CustomSeverityAndReason(t *testing.T) {
	db := setupTestDB(t)
	router, domainName, _, storageKey := setupDomainAndMessage(t, db)

	body := map[string]string{
		"severity": "temporary",
		"reason":   "greylisted",
	}
	triggerRec := triggerEvent(t, router, domainName, "fail", storageKey, body)
	if triggerRec.Code != http.StatusOK {
		t.Fatalf("trigger failed: status=%d body=%s", triggerRec.Code, triggerRec.Body.String())
	}

	eventsRec := getEvents(t, router, domainName, map[string]string{"event": "failed"})
	if eventsRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", eventsRec.Code, eventsRec.Body.String())
	}

	var rawResp map[string]json.RawMessage
	if err := json.Unmarshal(eventsRec.Body.Bytes(), &rawResp); err != nil {
		t.Fatalf("failed to decode raw response: %v", err)
	}
	var items []map[string]interface{}
	if err := json.Unmarshal(rawResp["items"], &items); err != nil {
		t.Fatalf("failed to decode items: %v", err)
	}

	var failedItem map[string]interface{}
	for _, item := range items {
		if item["event"] == "failed" {
			failedItem = item
			break
		}
	}
	if failedItem == nil {
		t.Fatal("failed event not found")
	}

	// Severity should be "temporary"
	severity, _ := failedItem["severity"].(string)
	if severity != "temporary" {
		t.Errorf("expected severity %q, got %q", "temporary", severity)
	}

	// Reason should be "greylisted"
	reason, _ := failedItem["reason"].(string)
	if reason != "greylisted" {
		t.Errorf("expected reason %q, got %q", "greylisted", reason)
	}

	// Log level should be "warn" for temporary failure
	logLevel, _ := failedItem["log-level"].(string)
	if logLevel != "warn" {
		t.Errorf("expected log-level %q for temporary failure, got %q", "warn", logLevel)
	}

	// Delivery status code should be 421 for temporary failure
	deliveryStatus, ok := failedItem["delivery-status"].(map[string]interface{})
	if !ok {
		t.Fatal("expected delivery-status in failed event")
	}
	code, _ := deliveryStatus["code"].(float64)
	if code != 421 {
		t.Errorf("expected delivery-status code 421 for temporary failure, got %v", code)
	}
}

// ---------------------------------------------------------------------------
// Open Trigger Tests
// ---------------------------------------------------------------------------

func TestTriggerOpen_HappyPath(t *testing.T) {
	db := setupTestDB(t)
	router, domainName, _, storageKey := setupDomainAndMessage(t, db)

	rec := triggerEvent(t, router, domainName, "open", storageKey, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeTriggerResponse(t, rec)
	if resp.Message != "Event created" {
		t.Errorf("expected message %q, got %q", "Event created", resp.Message)
	}
	if resp.EventID == "" {
		t.Error("expected non-empty event_id in response")
	}
}

func TestTriggerOpen_AppearsInEventsList(t *testing.T) {
	db := setupTestDB(t)
	router, domainName, _, storageKey := setupDomainAndMessage(t, db)

	triggerRec := triggerEvent(t, router, domainName, "open", storageKey, nil)
	if triggerRec.Code != http.StatusOK {
		t.Fatalf("trigger failed: status=%d body=%s", triggerRec.Code, triggerRec.Body.String())
	}

	eventsRec := getEvents(t, router, domainName, map[string]string{"event": "opened"})
	if eventsRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", eventsRec.Code, eventsRec.Body.String())
	}

	var resp eventsResponse
	decodeJSON(t, eventsRec, &resp)

	found := findEventByType(resp.Items, "opened")
	if found == nil {
		t.Fatal("expected an opened event in events list, found none")
	}
}

func TestTriggerOpen_DomainNotFound(t *testing.T) {
	db := setupTestDB(t)
	cfg := noAutoDeliverConfigForTriggers()
	router := setupRouterWithMockTriggers(db, cfg)

	rec := triggerEvent(t, router, "nonexistent.example.com", "open", "fake-storage-key", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestTriggerOpen_MessageNotFound(t *testing.T) {
	db := setupTestDB(t)
	cfg := noAutoDeliverConfigForTriggers()
	router := setupRouterWithMockTriggers(db, cfg)

	domainName := "triggers.example.com"
	createTestDomain(t, router, domainName)

	rec := triggerEvent(t, router, domainName, "open", "nonexistent-message-id", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestTriggerOpen_EventFieldsVerification(t *testing.T) {
	db := setupTestDB(t)
	router, domainName, _, storageKey := setupDomainAndMessage(t, db)

	triggerRec := triggerEvent(t, router, domainName, "open", storageKey, nil)
	if triggerRec.Code != http.StatusOK {
		t.Fatalf("trigger failed: status=%d body=%s", triggerRec.Code, triggerRec.Body.String())
	}

	eventsRec := getEvents(t, router, domainName, map[string]string{"event": "opened"})
	if eventsRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", eventsRec.Code, eventsRec.Body.String())
	}

	var rawResp map[string]json.RawMessage
	if err := json.Unmarshal(eventsRec.Body.Bytes(), &rawResp); err != nil {
		t.Fatalf("failed to decode raw response: %v", err)
	}
	var items []map[string]interface{}
	if err := json.Unmarshal(rawResp["items"], &items); err != nil {
		t.Fatalf("failed to decode items: %v", err)
	}

	var openedItem map[string]interface{}
	for _, item := range items {
		if item["event"] == "opened" {
			openedItem = item
			break
		}
	}
	if openedItem == nil {
		t.Fatal("opened event not found")
	}

	// Check log-level
	logLevel, _ := openedItem["log-level"].(string)
	if logLevel != "info" {
		t.Errorf("expected log-level %q, got %q", "info", logLevel)
	}

	// Check client-info
	clientInfo, ok := openedItem["client-info"].(map[string]interface{})
	if !ok {
		t.Fatal("expected client-info in opened event payload")
	}
	if clientInfo["client-type"] == nil {
		t.Error("expected client-type in client-info")
	}

	// Check geolocation
	geo, ok := openedItem["geolocation"].(map[string]interface{})
	if !ok {
		t.Fatal("expected geolocation in opened event payload")
	}
	if geo["country"] == nil {
		t.Error("expected country in geolocation")
	}

	// Check ip
	ip, _ := openedItem["ip"].(string)
	if ip == "" {
		t.Error("expected non-empty ip in opened event payload")
	}
}

// ---------------------------------------------------------------------------
// Click Trigger Tests
// ---------------------------------------------------------------------------

func TestTriggerClick_HappyPath(t *testing.T) {
	db := setupTestDB(t)
	router, domainName, _, storageKey := setupDomainAndMessage(t, db)

	rec := triggerEvent(t, router, domainName, "click", storageKey, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeTriggerResponse(t, rec)
	if resp.Message != "Event created" {
		t.Errorf("expected message %q, got %q", "Event created", resp.Message)
	}
	if resp.EventID == "" {
		t.Error("expected non-empty event_id in response")
	}
}

func TestTriggerClick_AppearsInEventsList(t *testing.T) {
	db := setupTestDB(t)
	router, domainName, _, storageKey := setupDomainAndMessage(t, db)

	triggerRec := triggerEvent(t, router, domainName, "click", storageKey, nil)
	if triggerRec.Code != http.StatusOK {
		t.Fatalf("trigger failed: status=%d body=%s", triggerRec.Code, triggerRec.Body.String())
	}

	eventsRec := getEvents(t, router, domainName, map[string]string{"event": "clicked"})
	if eventsRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", eventsRec.Code, eventsRec.Body.String())
	}

	var resp eventsResponse
	decodeJSON(t, eventsRec, &resp)

	found := findEventByType(resp.Items, "clicked")
	if found == nil {
		t.Fatal("expected a clicked event in events list, found none")
	}
}

func TestTriggerClick_DomainNotFound(t *testing.T) {
	db := setupTestDB(t)
	cfg := noAutoDeliverConfigForTriggers()
	router := setupRouterWithMockTriggers(db, cfg)

	rec := triggerEvent(t, router, "nonexistent.example.com", "click", "fake-storage-key", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestTriggerClick_MessageNotFound(t *testing.T) {
	db := setupTestDB(t)
	cfg := noAutoDeliverConfigForTriggers()
	router := setupRouterWithMockTriggers(db, cfg)

	domainName := "triggers.example.com"
	createTestDomain(t, router, domainName)

	rec := triggerEvent(t, router, domainName, "click", "nonexistent-message-id", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestTriggerClick_DefaultURL(t *testing.T) {
	db := setupTestDB(t)
	router, domainName, _, storageKey := setupDomainAndMessage(t, db)

	triggerRec := triggerEvent(t, router, domainName, "click", storageKey, nil)
	if triggerRec.Code != http.StatusOK {
		t.Fatalf("trigger failed: status=%d body=%s", triggerRec.Code, triggerRec.Body.String())
	}

	eventsRec := getEvents(t, router, domainName, map[string]string{"event": "clicked"})
	if eventsRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", eventsRec.Code, eventsRec.Body.String())
	}

	var rawResp map[string]json.RawMessage
	if err := json.Unmarshal(eventsRec.Body.Bytes(), &rawResp); err != nil {
		t.Fatalf("failed to decode raw response: %v", err)
	}
	var items []map[string]interface{}
	if err := json.Unmarshal(rawResp["items"], &items); err != nil {
		t.Fatalf("failed to decode items: %v", err)
	}

	var clickedItem map[string]interface{}
	for _, item := range items {
		if item["event"] == "clicked" {
			clickedItem = item
			break
		}
	}
	if clickedItem == nil {
		t.Fatal("clicked event not found")
	}

	// Default URL should be "http://example.com"
	url, _ := clickedItem["url"].(string)
	if url != "http://example.com" {
		t.Errorf("expected default url %q, got %q", "http://example.com", url)
	}
}

func TestTriggerClick_CustomURL(t *testing.T) {
	db := setupTestDB(t)
	router, domainName, _, storageKey := setupDomainAndMessage(t, db)

	body := map[string]string{
		"url": "https://custom.com/link",
	}
	triggerRec := triggerEvent(t, router, domainName, "click", storageKey, body)
	if triggerRec.Code != http.StatusOK {
		t.Fatalf("trigger failed: status=%d body=%s", triggerRec.Code, triggerRec.Body.String())
	}

	eventsRec := getEvents(t, router, domainName, map[string]string{"event": "clicked"})
	if eventsRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", eventsRec.Code, eventsRec.Body.String())
	}

	var rawResp map[string]json.RawMessage
	if err := json.Unmarshal(eventsRec.Body.Bytes(), &rawResp); err != nil {
		t.Fatalf("failed to decode raw response: %v", err)
	}
	var items []map[string]interface{}
	if err := json.Unmarshal(rawResp["items"], &items); err != nil {
		t.Fatalf("failed to decode items: %v", err)
	}

	var clickedItem map[string]interface{}
	for _, item := range items {
		if item["event"] == "clicked" {
			clickedItem = item
			break
		}
	}
	if clickedItem == nil {
		t.Fatal("clicked event not found")
	}

	// URL should match the custom value
	url, _ := clickedItem["url"].(string)
	if url != "https://custom.com/link" {
		t.Errorf("expected url %q, got %q", "https://custom.com/link", url)
	}
}

func TestTriggerClick_EventFieldsVerification(t *testing.T) {
	db := setupTestDB(t)
	router, domainName, _, storageKey := setupDomainAndMessage(t, db)

	triggerRec := triggerEvent(t, router, domainName, "click", storageKey, nil)
	if triggerRec.Code != http.StatusOK {
		t.Fatalf("trigger failed: status=%d body=%s", triggerRec.Code, triggerRec.Body.String())
	}

	eventsRec := getEvents(t, router, domainName, map[string]string{"event": "clicked"})
	if eventsRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", eventsRec.Code, eventsRec.Body.String())
	}

	var rawResp map[string]json.RawMessage
	if err := json.Unmarshal(eventsRec.Body.Bytes(), &rawResp); err != nil {
		t.Fatalf("failed to decode raw response: %v", err)
	}
	var items []map[string]interface{}
	if err := json.Unmarshal(rawResp["items"], &items); err != nil {
		t.Fatalf("failed to decode items: %v", err)
	}

	var clickedItem map[string]interface{}
	for _, item := range items {
		if item["event"] == "clicked" {
			clickedItem = item
			break
		}
	}
	if clickedItem == nil {
		t.Fatal("clicked event not found")
	}

	// Check log-level
	logLevel, _ := clickedItem["log-level"].(string)
	if logLevel != "info" {
		t.Errorf("expected log-level %q, got %q", "info", logLevel)
	}

	// Check client-info
	_, ok := clickedItem["client-info"].(map[string]interface{})
	if !ok {
		t.Error("expected client-info in clicked event payload")
	}

	// Check geolocation
	_, ok = clickedItem["geolocation"].(map[string]interface{})
	if !ok {
		t.Error("expected geolocation in clicked event payload")
	}

	// Check ip
	ip, _ := clickedItem["ip"].(string)
	if ip == "" {
		t.Error("expected non-empty ip in clicked event payload")
	}
}

// ---------------------------------------------------------------------------
// Unsubscribe Trigger Tests
// ---------------------------------------------------------------------------

func TestTriggerUnsubscribe_HappyPath(t *testing.T) {
	db := setupTestDB(t)
	router, domainName, _, storageKey := setupDomainAndMessage(t, db)

	rec := triggerEvent(t, router, domainName, "unsubscribe", storageKey, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeTriggerResponse(t, rec)
	if resp.Message != "Event created" {
		t.Errorf("expected message %q, got %q", "Event created", resp.Message)
	}
	if resp.EventID == "" {
		t.Error("expected non-empty event_id in response")
	}
}

func TestTriggerUnsubscribe_AppearsInEventsList(t *testing.T) {
	db := setupTestDB(t)
	router, domainName, _, storageKey := setupDomainAndMessage(t, db)

	triggerRec := triggerEvent(t, router, domainName, "unsubscribe", storageKey, nil)
	if triggerRec.Code != http.StatusOK {
		t.Fatalf("trigger failed: status=%d body=%s", triggerRec.Code, triggerRec.Body.String())
	}

	eventsRec := getEvents(t, router, domainName, map[string]string{"event": "unsubscribed"})
	if eventsRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", eventsRec.Code, eventsRec.Body.String())
	}

	var resp eventsResponse
	decodeJSON(t, eventsRec, &resp)

	found := findEventByType(resp.Items, "unsubscribed")
	if found == nil {
		t.Fatal("expected an unsubscribed event in events list, found none")
	}
}

func TestTriggerUnsubscribe_DomainNotFound(t *testing.T) {
	db := setupTestDB(t)
	cfg := noAutoDeliverConfigForTriggers()
	router := setupRouterWithMockTriggers(db, cfg)

	rec := triggerEvent(t, router, "nonexistent.example.com", "unsubscribe", "fake-storage-key", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestTriggerUnsubscribe_MessageNotFound(t *testing.T) {
	db := setupTestDB(t)
	cfg := noAutoDeliverConfigForTriggers()
	router := setupRouterWithMockTriggers(db, cfg)

	domainName := "triggers.example.com"
	createTestDomain(t, router, domainName)

	rec := triggerEvent(t, router, domainName, "unsubscribe", "nonexistent-message-id", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestTriggerUnsubscribe_EventFieldsVerification(t *testing.T) {
	db := setupTestDB(t)
	router, domainName, _, storageKey := setupDomainAndMessage(t, db)

	triggerRec := triggerEvent(t, router, domainName, "unsubscribe", storageKey, nil)
	if triggerRec.Code != http.StatusOK {
		t.Fatalf("trigger failed: status=%d body=%s", triggerRec.Code, triggerRec.Body.String())
	}

	eventsRec := getEvents(t, router, domainName, map[string]string{"event": "unsubscribed"})
	if eventsRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", eventsRec.Code, eventsRec.Body.String())
	}

	var rawResp map[string]json.RawMessage
	if err := json.Unmarshal(eventsRec.Body.Bytes(), &rawResp); err != nil {
		t.Fatalf("failed to decode raw response: %v", err)
	}
	var items []map[string]interface{}
	if err := json.Unmarshal(rawResp["items"], &items); err != nil {
		t.Fatalf("failed to decode items: %v", err)
	}

	var unsubItem map[string]interface{}
	for _, item := range items {
		if item["event"] == "unsubscribed" {
			unsubItem = item
			break
		}
	}
	if unsubItem == nil {
		t.Fatal("unsubscribed event not found")
	}

	// Check log-level
	logLevel, _ := unsubItem["log-level"].(string)
	if logLevel != "warn" {
		t.Errorf("expected log-level %q, got %q", "warn", logLevel)
	}

	// Check client-info
	_, ok := unsubItem["client-info"].(map[string]interface{})
	if !ok {
		t.Error("expected client-info in unsubscribed event payload")
	}

	// Check geolocation
	_, ok = unsubItem["geolocation"].(map[string]interface{})
	if !ok {
		t.Error("expected geolocation in unsubscribed event payload")
	}

	// Check ip
	ip, _ := unsubItem["ip"].(string)
	if ip == "" {
		t.Error("expected non-empty ip in unsubscribed event payload")
	}
}

// ---------------------------------------------------------------------------
// Complain Trigger Tests
// ---------------------------------------------------------------------------

func TestTriggerComplain_HappyPath(t *testing.T) {
	db := setupTestDB(t)
	router, domainName, _, storageKey := setupDomainAndMessage(t, db)

	rec := triggerEvent(t, router, domainName, "complain", storageKey, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeTriggerResponse(t, rec)
	if resp.Message != "Event created" {
		t.Errorf("expected message %q, got %q", "Event created", resp.Message)
	}
	if resp.EventID == "" {
		t.Error("expected non-empty event_id in response")
	}
}

func TestTriggerComplain_AppearsInEventsList(t *testing.T) {
	db := setupTestDB(t)
	router, domainName, _, storageKey := setupDomainAndMessage(t, db)

	triggerRec := triggerEvent(t, router, domainName, "complain", storageKey, nil)
	if triggerRec.Code != http.StatusOK {
		t.Fatalf("trigger failed: status=%d body=%s", triggerRec.Code, triggerRec.Body.String())
	}

	eventsRec := getEvents(t, router, domainName, map[string]string{"event": "complained"})
	if eventsRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", eventsRec.Code, eventsRec.Body.String())
	}

	var resp eventsResponse
	decodeJSON(t, eventsRec, &resp)

	found := findEventByType(resp.Items, "complained")
	if found == nil {
		t.Fatal("expected a complained event in events list, found none")
	}
}

func TestTriggerComplain_DomainNotFound(t *testing.T) {
	db := setupTestDB(t)
	cfg := noAutoDeliverConfigForTriggers()
	router := setupRouterWithMockTriggers(db, cfg)

	rec := triggerEvent(t, router, "nonexistent.example.com", "complain", "fake-storage-key", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestTriggerComplain_MessageNotFound(t *testing.T) {
	db := setupTestDB(t)
	cfg := noAutoDeliverConfigForTriggers()
	router := setupRouterWithMockTriggers(db, cfg)

	domainName := "triggers.example.com"
	createTestDomain(t, router, domainName)

	rec := triggerEvent(t, router, domainName, "complain", "nonexistent-message-id", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestTriggerComplain_EventFieldsVerification(t *testing.T) {
	db := setupTestDB(t)
	router, domainName, _, storageKey := setupDomainAndMessage(t, db)

	triggerRec := triggerEvent(t, router, domainName, "complain", storageKey, nil)
	if triggerRec.Code != http.StatusOK {
		t.Fatalf("trigger failed: status=%d body=%s", triggerRec.Code, triggerRec.Body.String())
	}

	eventsRec := getEvents(t, router, domainName, map[string]string{"event": "complained"})
	if eventsRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", eventsRec.Code, eventsRec.Body.String())
	}

	var rawResp map[string]json.RawMessage
	if err := json.Unmarshal(eventsRec.Body.Bytes(), &rawResp); err != nil {
		t.Fatalf("failed to decode raw response: %v", err)
	}
	var items []map[string]interface{}
	if err := json.Unmarshal(rawResp["items"], &items); err != nil {
		t.Fatalf("failed to decode items: %v", err)
	}

	var complainedItem map[string]interface{}
	for _, item := range items {
		if item["event"] == "complained" {
			complainedItem = item
			break
		}
	}
	if complainedItem == nil {
		t.Fatal("complained event not found")
	}

	// Check log-level
	logLevel, _ := complainedItem["log-level"].(string)
	if logLevel != "warn" {
		t.Errorf("expected log-level %q, got %q", "warn", logLevel)
	}

	// Check message with headers
	msgField, ok := complainedItem["message"].(map[string]interface{})
	if !ok {
		t.Fatal("expected message field in complained event")
	}
	headers, ok := msgField["headers"].(map[string]interface{})
	if !ok {
		t.Fatal("expected headers in message field")
	}
	if headers["from"] == nil {
		t.Error("expected from in message headers")
	}
	if headers["to"] == nil {
		t.Error("expected to in message headers")
	}
	if headers["subject"] == nil {
		t.Error("expected subject in message headers")
	}
}

// ---------------------------------------------------------------------------
// Cross-cutting Tests
// ---------------------------------------------------------------------------

func TestMultipleTriggersOnSameMessage(t *testing.T) {
	db := setupTestDB(t)
	router, domainName, _, storageKey := setupDomainAndMessage(t, db)

	// Trigger deliver
	rec1 := triggerEvent(t, router, domainName, "deliver", storageKey, nil)
	if rec1.Code != http.StatusOK {
		t.Fatalf("deliver trigger failed: status=%d body=%s", rec1.Code, rec1.Body.String())
	}

	// Trigger open
	rec2 := triggerEvent(t, router, domainName, "open", storageKey, nil)
	if rec2.Code != http.StatusOK {
		t.Fatalf("open trigger failed: status=%d body=%s", rec2.Code, rec2.Body.String())
	}

	// Both events should appear
	eventsRec := getEvents(t, router, domainName, nil)
	if eventsRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", eventsRec.Code, eventsRec.Body.String())
	}

	var resp eventsResponse
	decodeJSON(t, eventsRec, &resp)

	foundDelivered := false
	foundOpened := false
	for _, item := range resp.Items {
		if item.Event == "delivered" {
			foundDelivered = true
		}
		if item.Event == "opened" {
			foundOpened = true
		}
	}

	if !foundDelivered {
		t.Error("expected delivered event after triggering deliver, not found")
	}
	if !foundOpened {
		t.Error("expected opened event after triggering open, not found")
	}
}

func TestAllSixTriggersOnSameMessage(t *testing.T) {
	db := setupTestDB(t)
	router, domainName, _, storageKey := setupDomainAndMessage(t, db)

	actions := []struct {
		action    string
		eventType string
	}{
		{"deliver", "delivered"},
		{"fail", "failed"},
		{"open", "opened"},
		{"click", "clicked"},
		{"unsubscribe", "unsubscribed"},
		{"complain", "complained"},
	}

	for _, a := range actions {
		rec := triggerEvent(t, router, domainName, a.action, storageKey, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("trigger %s failed: status=%d body=%s", a.action, rec.Code, rec.Body.String())
		}
		resp := decodeTriggerResponse(t, rec)
		if resp.EventID == "" {
			t.Errorf("trigger %s returned empty event_id", a.action)
		}
	}

	// All event types should appear in the events list
	eventsRec := getEvents(t, router, domainName, nil)
	if eventsRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", eventsRec.Code, eventsRec.Body.String())
	}

	var resp eventsResponse
	decodeJSON(t, eventsRec, &resp)

	foundTypes := make(map[string]bool)
	for _, item := range resp.Items {
		foundTypes[item.Event] = true
	}

	for _, a := range actions {
		if !foundTypes[a.eventType] {
			t.Errorf("expected %s event in events list after triggering %s, not found", a.eventType, a.action)
		}
	}
}

func TestTriggerEventIDsAreUnique(t *testing.T) {
	db := setupTestDB(t)
	router, domainName, _, storageKey := setupDomainAndMessage(t, db)

	// Trigger the same event type twice
	rec1 := triggerEvent(t, router, domainName, "deliver", storageKey, nil)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first trigger failed: status=%d body=%s", rec1.Code, rec1.Body.String())
	}
	resp1 := decodeTriggerResponse(t, rec1)

	rec2 := triggerEvent(t, router, domainName, "deliver", storageKey, nil)
	if rec2.Code != http.StatusOK {
		t.Fatalf("second trigger failed: status=%d body=%s", rec2.Code, rec2.Body.String())
	}
	resp2 := decodeTriggerResponse(t, rec2)

	if resp1.EventID == resp2.EventID {
		t.Errorf("expected unique event IDs, both returned %q", resp1.EventID)
	}
}
