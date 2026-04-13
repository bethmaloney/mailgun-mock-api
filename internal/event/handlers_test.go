package event_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"strings"
	"testing"
	"time"

	"github.com/bethmaloney/mailgun-mock-api/internal/domain"
	"github.com/bethmaloney/mailgun-mock-api/internal/event"
	"github.com/bethmaloney/mailgun-mock-api/internal/message"
	"github.com/bethmaloney/mailgun-mock-api/internal/mock"
	"github.com/go-chi/chi/v5"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// Test Helpers
// ---------------------------------------------------------------------------

// setupTestDB creates an in-memory SQLite database for testing with the
// Domain, DNSRecord, StoredMessage, and Event tables migrated.
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}
	if err := db.AutoMigrate(&domain.Domain{}, &domain.DNSRecord{}, &message.StoredMessage{}, &event.Event{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

// defaultConfig returns a MockConfig with auto-verify and auto-deliver enabled.
func defaultConfig() *mock.MockConfig {
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
	}
}

// noAutoDeliverConfig returns a MockConfig with auto-deliver disabled.
func noAutoDeliverConfig() *mock.MockConfig {
	cfg := defaultConfig()
	cfg.EventGeneration.AutoDeliver = false
	return cfg
}

// setupRouter creates a chi router with domain, message, and event routes registered.
func setupRouter(db *gorm.DB, cfg *mock.MockConfig) http.Handler {
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
	return r
}

// newMultipartRequest creates an HTTP request with multipart/form-data body.
func newMultipartRequest(t *testing.T, method, url string, fields map[string]string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for key, val := range fields {
		if err := writer.WriteField(key, val); err != nil {
			t.Fatalf("failed to write field %q: %v", key, err)
		}
	}
	writer.Close()
	req := httptest.NewRequest(method, url, &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

// fieldPair represents a key-value pair for multipart form fields,
// allowing repeated keys (e.g., multiple o:tag values).
type fieldPair struct {
	Key   string
	Value string
}

// newMultipartRequestWithRepeatedFields allows repeated keys (e.g., multiple o:tag values).
func newMultipartRequestWithRepeatedFields(t *testing.T, method, url string, fields []fieldPair) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for _, f := range fields {
		if err := writer.WriteField(f.Key, f.Value); err != nil {
			t.Fatalf("failed to write field %q: %v", f.Key, err)
		}
	}
	writer.Close()
	req := httptest.NewRequest(method, url, &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

// createTestDomain creates a domain via the API, which is required before
// sending messages (since messages are domain-scoped).
func createTestDomain(t *testing.T, router http.Handler, name string) {
	t.Helper()
	req := newMultipartRequest(t, http.MethodPost, "/v4/domains", map[string]string{"name": name})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to create test domain %q: status=%d body=%s", name, rec.Code, rec.Body.String())
	}
}

// sendTestMessage sends a message via the API and returns the message ID and storage key.
func sendTestMessage(t *testing.T, router http.Handler, domainName string, fields map[string]string) (messageID, storageKey string) {
	t.Helper()
	url := fmt.Sprintf("/v3/%s/messages", domainName)
	req := newMultipartRequest(t, http.MethodPost, url, fields)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to send test message: status=%d body=%s", rec.Code, rec.Body.String())
	}

	var resp sendResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode send response: %v", err)
	}

	messageID = resp.ID
	storageKey = strings.TrimPrefix(messageID, "<")
	storageKey = strings.TrimSuffix(storageKey, ">")
	return messageID, storageKey
}

// sendTestMessageWithRepeatedFields sends a message with repeated field keys.
func sendTestMessageWithRepeatedFields(t *testing.T, router http.Handler, domainName string, fields []fieldPair) (messageID, storageKey string) {
	t.Helper()
	url := fmt.Sprintf("/v3/%s/messages", domainName)
	req := newMultipartRequestWithRepeatedFields(t, http.MethodPost, url, fields)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to send test message: status=%d body=%s", rec.Code, rec.Body.String())
	}

	var resp sendResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode send response: %v", err)
	}

	messageID = resp.ID
	storageKey = strings.TrimPrefix(messageID, "<")
	storageKey = strings.TrimSuffix(storageKey, ">")
	return messageID, storageKey
}

// getEvents makes a GET request to the events endpoint and returns the response.
func getEvents(t *testing.T, router http.Handler, domainName string, queryParams map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	u := fmt.Sprintf("/v3/%s/events", domainName)
	if len(queryParams) > 0 {
		v := neturl.Values{}
		for key, val := range queryParams {
			v.Set(key, val)
		}
		u += "?" + v.Encode()
	}
	req := httptest.NewRequest(http.MethodGet, u, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// decodeJSON unmarshals the response body into the provided destination.
func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder, dest interface{}) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dest); err != nil {
		t.Fatalf("failed to decode response (body=%q): %v", rec.Body.String(), err)
	}
}

// defaultMessageFields returns the minimum fields required to send a message.
func defaultMessageFields(from, to, subject string) map[string]string {
	return map[string]string{
		"from":    from,
		"to":      to,
		"subject": subject,
		"text":    "Hello, this is a test message.",
	}
}

// ---------------------------------------------------------------------------
// Response Structs for Assertions
// ---------------------------------------------------------------------------

type sendResponse struct {
	ID      string `json:"id"`
	Message string `json:"message"`
}

type eventItem struct {
	ID              string                 `json:"id"`
	Event           string                 `json:"event"`
	Timestamp       float64                `json:"timestamp"`
	LogLevel        string                 `json:"log-level"`
	Recipient       string                 `json:"recipient"`
	RecipientDomain string                 `json:"recipient-domain"`
	Message         map[string]interface{} `json:"message"`
	Envelope        map[string]interface{} `json:"envelope"`
	Flags           map[string]interface{} `json:"flags"`
	Tags            []string               `json:"tags"`
	UserVariables   map[string]interface{} `json:"user-variables"`
	Storage         map[string]interface{} `json:"storage"`
	Severity        string                 `json:"severity,omitempty"`
	Reason          string                 `json:"reason,omitempty"`
}

type pagingInfo struct {
	First    string `json:"first"`
	Last     string `json:"last"`
	Next     string `json:"next"`
	Previous string `json:"previous"`
}

type eventsResponse struct {
	Items  []eventItem `json:"items"`
	Paging pagingInfo  `json:"paging"`
}

// ---------------------------------------------------------------------------
// Event Generation Tests
// ---------------------------------------------------------------------------

func TestSendMessageGeneratesAcceptedEvent(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	domainName := "test.example.com"
	createTestDomain(t, router, domainName)

	recipient := "user@example.com"
	sendTestMessage(t, router, domainName, defaultMessageFields(
		"sender@test.example.com", recipient, "Hello",
	))

	rec := getEvents(t, router, domainName, map[string]string{"event": "accepted"})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp eventsResponse
	decodeJSON(t, rec, &resp)

	if len(resp.Items) == 0 {
		t.Fatal("expected at least one accepted event, got none")
	}

	found := false
	for _, item := range resp.Items {
		if item.Event == "accepted" && item.Recipient == recipient {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected accepted event for recipient %q, not found in %d items", recipient, len(resp.Items))
	}
}

func TestMultipleRecipientsGenerateMultipleAcceptedEvents(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	domainName := "multi.example.com"
	createTestDomain(t, router, domainName)

	recipients := "alice@example.com, bob@example.com, carol@example.com"
	sendTestMessage(t, router, domainName, defaultMessageFields(
		"sender@multi.example.com", recipients, "Hello All",
	))

	rec := getEvents(t, router, domainName, map[string]string{"event": "accepted"})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp eventsResponse
	decodeJSON(t, rec, &resp)

	acceptedCount := 0
	for _, item := range resp.Items {
		if item.Event == "accepted" {
			acceptedCount++
		}
	}
	if acceptedCount != 3 {
		t.Errorf("expected 3 accepted events for 3 recipients, got %d", acceptedCount)
	}
}

func TestAutoDeliverGeneratesDeliveredEvents(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	cfg.EventGeneration.AutoDeliver = true
	router := setupRouter(db, cfg)

	domainName := "deliver.example.com"
	createTestDomain(t, router, domainName)

	recipient := "user@example.com"
	sendTestMessage(t, router, domainName, defaultMessageFields(
		"sender@deliver.example.com", recipient, "Auto Deliver Test",
	))

	// Query all events
	rec := getEvents(t, router, domainName, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp eventsResponse
	decodeJSON(t, rec, &resp)

	hasAccepted := false
	hasDelivered := false
	for _, item := range resp.Items {
		if item.Event == "accepted" && item.Recipient == recipient {
			hasAccepted = true
		}
		if item.Event == "delivered" && item.Recipient == recipient {
			hasDelivered = true
		}
	}

	if !hasAccepted {
		t.Error("expected accepted event when auto-deliver is enabled")
	}
	if !hasDelivered {
		t.Error("expected delivered event when auto-deliver is enabled")
	}
}

func TestNoAutoDeliverWhenDisabled(t *testing.T) {
	db := setupTestDB(t)
	cfg := noAutoDeliverConfig()
	router := setupRouter(db, cfg)

	domainName := "nodeliver.example.com"
	createTestDomain(t, router, domainName)

	recipient := "user@example.com"
	sendTestMessage(t, router, domainName, defaultMessageFields(
		"sender@nodeliver.example.com", recipient, "No Deliver Test",
	))

	rec := getEvents(t, router, domainName, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp eventsResponse
	decodeJSON(t, rec, &resp)

	for _, item := range resp.Items {
		if item.Event == "delivered" {
			t.Error("expected no delivered events when auto-deliver is disabled, but found one")
		}
	}

	hasAccepted := false
	for _, item := range resp.Items {
		if item.Event == "accepted" {
			hasAccepted = true
			break
		}
	}
	if !hasAccepted {
		t.Error("expected accepted event even when auto-deliver is disabled")
	}
}

func TestEventLinksToMessage(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	domainName := "link.example.com"
	createTestDomain(t, router, domainName)

	messageID, storageKey := sendTestMessage(t, router, domainName, defaultMessageFields(
		"sender@link.example.com", "user@example.com", "Link Test",
	))

	// Query events from the database directly to verify the link
	var events []event.Event
	if err := db.Where("domain_name = ?", domainName).Find(&events).Error; err != nil {
		t.Fatalf("failed to query events: %v", err)
	}

	if len(events) == 0 {
		t.Fatal("expected events to be generated, got none")
	}

	for _, ev := range events {
		if ev.MessageID != messageID {
			t.Errorf("event MessageID = %q, want %q", ev.MessageID, messageID)
		}
		if ev.StorageKey != storageKey {
			t.Errorf("event StorageKey = %q, want %q", ev.StorageKey, storageKey)
		}
	}
}

func TestEventHasCorrectLogLevel(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	cfg.EventGeneration.AutoDeliver = true
	router := setupRouter(db, cfg)

	domainName := "loglevel.example.com"
	createTestDomain(t, router, domainName)

	sendTestMessage(t, router, domainName, defaultMessageFields(
		"sender@loglevel.example.com", "user@example.com", "Log Level Test",
	))

	var events []event.Event
	if err := db.Where("domain_name = ?", domainName).Find(&events).Error; err != nil {
		t.Fatalf("failed to query events: %v", err)
	}

	for _, ev := range events {
		switch ev.EventType {
		case "accepted":
			if ev.LogLevel != "info" {
				t.Errorf("accepted event log level = %q, want %q", ev.LogLevel, "info")
			}
		case "delivered":
			if ev.LogLevel != "info" {
				t.Errorf("delivered event log level = %q, want %q", ev.LogLevel, "info")
			}
		}
	}
}

func TestEventTimestampsHaveMicrosecondPrecision(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	domainName := "precision.example.com"
	createTestDomain(t, router, domainName)

	sendTestMessage(t, router, domainName, defaultMessageFields(
		"sender@precision.example.com", "user@example.com", "Precision Test",
	))

	var events []event.Event
	if err := db.Where("domain_name = ?", domainName).Find(&events).Error; err != nil {
		t.Fatalf("failed to query events: %v", err)
	}

	if len(events) == 0 {
		t.Fatal("expected events, got none")
	}

	for _, ev := range events {
		// The timestamp should have decimal places indicating microsecond precision
		if ev.Timestamp == 0 {
			t.Error("event timestamp should not be zero")
			continue
		}
		// Check that there are fractional parts (i.e., not an integer)
		if ev.Timestamp == math.Floor(ev.Timestamp) {
			t.Errorf("event timestamp %f has no fractional component; expected microsecond precision", ev.Timestamp)
		}
		// Timestamp should be a reasonable Unix epoch (after year 2020)
		if ev.Timestamp < 1577836800 {
			t.Errorf("event timestamp %f is unreasonably small (before 2020)", ev.Timestamp)
		}
	}
}

func TestEventIDsAreUnique(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	cfg.EventGeneration.AutoDeliver = true
	router := setupRouter(db, cfg)

	domainName := "unique.example.com"
	createTestDomain(t, router, domainName)

	// Send multiple messages to generate several events
	sendTestMessage(t, router, domainName, defaultMessageFields(
		"sender@unique.example.com", "alice@example.com", "Unique Test 1",
	))
	sendTestMessage(t, router, domainName, defaultMessageFields(
		"sender@unique.example.com", "bob@example.com", "Unique Test 2",
	))

	var events []event.Event
	if err := db.Where("domain_name = ?", domainName).Find(&events).Error; err != nil {
		t.Fatalf("failed to query events: %v", err)
	}

	if len(events) < 4 {
		t.Fatalf("expected at least 4 events (2 accepted + 2 delivered), got %d", len(events))
	}

	seen := make(map[string]bool)
	for _, ev := range events {
		if ev.ID == "" {
			t.Error("event ID should not be empty")
			continue
		}
		if seen[ev.ID] {
			t.Errorf("duplicate event ID: %s", ev.ID)
		}
		seen[ev.ID] = true
	}
}

func TestTagsPropagatedToEvents(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	domainName := "tags.example.com"
	createTestDomain(t, router, domainName)

	sendTestMessageWithRepeatedFields(t, router, domainName, []fieldPair{
		{Key: "from", Value: "sender@tags.example.com"},
		{Key: "to", Value: "user@example.com"},
		{Key: "subject", Value: "Tags Test"},
		{Key: "text", Value: "Hello with tags"},
		{Key: "o:tag", Value: "welcome"},
		{Key: "o:tag", Value: "onboarding"},
	})

	rec := getEvents(t, router, domainName, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp eventsResponse
	decodeJSON(t, rec, &resp)

	if len(resp.Items) == 0 {
		t.Fatal("expected events, got none")
	}

	for _, item := range resp.Items {
		if len(item.Tags) == 0 {
			t.Error("expected tags in event, got none")
			continue
		}
		hasWelcome := false
		hasOnboarding := false
		for _, tag := range item.Tags {
			if tag == "welcome" {
				hasWelcome = true
			}
			if tag == "onboarding" {
				hasOnboarding = true
			}
		}
		if !hasWelcome {
			t.Errorf("expected tag 'welcome' in event tags: %v", item.Tags)
		}
		if !hasOnboarding {
			t.Errorf("expected tag 'onboarding' in event tags: %v", item.Tags)
		}
	}
}

func TestCustomVariablesInEvents(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	domainName := "vars.example.com"
	createTestDomain(t, router, domainName)

	sendTestMessage(t, router, domainName, map[string]string{
		"from":       "sender@vars.example.com",
		"to":         "user@example.com",
		"subject":    "Variables Test",
		"text":       "Hello with variables",
		"v:user-id":  "123",
		"v:campaign": "summer2024",
	})

	rec := getEvents(t, router, domainName, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp eventsResponse
	decodeJSON(t, rec, &resp)

	if len(resp.Items) == 0 {
		t.Fatal("expected events, got none")
	}

	for _, item := range resp.Items {
		if item.UserVariables == nil {
			t.Error("expected user-variables in event, got nil")
			continue
		}
		if item.UserVariables["user-id"] != "123" {
			t.Errorf("expected user-variable 'user-id'='123', got %v", item.UserVariables["user-id"])
		}
		if item.UserVariables["campaign"] != "summer2024" {
			t.Errorf("expected user-variable 'campaign'='summer2024', got %v", item.UserVariables["campaign"])
		}
	}
}

// ---------------------------------------------------------------------------
// Event Querying Tests
// ---------------------------------------------------------------------------

func TestListEventsReturnsItemsAndPaging(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	domainName := "list.example.com"
	createTestDomain(t, router, domainName)

	sendTestMessage(t, router, domainName, defaultMessageFields(
		"sender@list.example.com", "user@example.com", "List Test",
	))

	rec := getEvents(t, router, domainName, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp eventsResponse
	decodeJSON(t, rec, &resp)

	// Should have items array (even if empty)
	if resp.Items == nil {
		t.Error("expected 'items' field in response")
	}

	// Should have paging object
	if resp.Paging.First == "" && resp.Paging.Last == "" {
		t.Error("expected paging object with at least 'first' and 'last' URLs")
	}
}

func TestFilterByEventType(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	cfg.EventGeneration.AutoDeliver = true
	router := setupRouter(db, cfg)

	domainName := "filter-type.example.com"
	createTestDomain(t, router, domainName)

	sendTestMessage(t, router, domainName, defaultMessageFields(
		"sender@filter-type.example.com", "user@example.com", "Filter Type Test",
	))

	// Filter for only delivered events
	rec := getEvents(t, router, domainName, map[string]string{"event": "delivered"})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp eventsResponse
	decodeJSON(t, rec, &resp)

	for _, item := range resp.Items {
		if item.Event != "delivered" {
			t.Errorf("expected only 'delivered' events, got %q", item.Event)
		}
	}

	if len(resp.Items) == 0 {
		t.Error("expected at least one delivered event")
	}
}

func TestFilterByEventTypeOR(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	cfg.EventGeneration.AutoDeliver = true
	router := setupRouter(db, cfg)

	domainName := "filter-or.example.com"
	createTestDomain(t, router, domainName)

	sendTestMessage(t, router, domainName, defaultMessageFields(
		"sender@filter-or.example.com", "user@example.com", "Filter OR Test",
	))

	// Filter for accepted OR delivered events
	rec := getEvents(t, router, domainName, map[string]string{"event": "accepted OR delivered"})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp eventsResponse
	decodeJSON(t, rec, &resp)

	for _, item := range resp.Items {
		if item.Event != "accepted" && item.Event != "delivered" {
			t.Errorf("expected only 'accepted' or 'delivered' events, got %q", item.Event)
		}
	}

	if len(resp.Items) < 2 {
		t.Errorf("expected at least 2 events (accepted + delivered), got %d", len(resp.Items))
	}
}

func TestFilterByRecipient(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	domainName := "filter-recip.example.com"
	createTestDomain(t, router, domainName)

	sendTestMessage(t, router, domainName, defaultMessageFields(
		"sender@filter-recip.example.com", "alice@example.com", "For Alice",
	))
	sendTestMessage(t, router, domainName, defaultMessageFields(
		"sender@filter-recip.example.com", "bob@example.com", "For Bob",
	))

	rec := getEvents(t, router, domainName, map[string]string{"recipient": "alice@example.com"})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp eventsResponse
	decodeJSON(t, rec, &resp)

	for _, item := range resp.Items {
		if item.Recipient != "alice@example.com" {
			t.Errorf("expected only events for alice@example.com, got recipient=%q", item.Recipient)
		}
	}

	if len(resp.Items) == 0 {
		t.Error("expected at least one event for alice@example.com")
	}
}

func TestFilterByMessageID(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	domainName := "filter-msgid.example.com"
	createTestDomain(t, router, domainName)

	msgID1, _ := sendTestMessage(t, router, domainName, defaultMessageFields(
		"sender@filter-msgid.example.com", "user@example.com", "Message 1",
	))
	sendTestMessage(t, router, domainName, defaultMessageFields(
		"sender@filter-msgid.example.com", "user@example.com", "Message 2",
	))

	rec := getEvents(t, router, domainName, map[string]string{"message-id": msgID1})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp eventsResponse
	decodeJSON(t, rec, &resp)

	if len(resp.Items) == 0 {
		t.Fatal("expected at least one event for the specified message-id")
	}

	// All returned events should be for the given message ID
	for _, item := range resp.Items {
		msgHeaders, ok := item.Message["headers"].(map[string]interface{})
		if ok {
			if mid, exists := msgHeaders["message-id"]; exists {
				if mid != msgID1 {
					t.Errorf("expected events for message-id %q, got %q", msgID1, mid)
				}
			}
		}
	}
}

func TestFilterByTags(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	domainName := "filter-tags.example.com"
	createTestDomain(t, router, domainName)

	sendTestMessageWithRepeatedFields(t, router, domainName, []fieldPair{
		{Key: "from", Value: "sender@filter-tags.example.com"},
		{Key: "to", Value: "user@example.com"},
		{Key: "subject", Value: "Tagged Message"},
		{Key: "text", Value: "Hello"},
		{Key: "o:tag", Value: "welcome"},
	})
	sendTestMessage(t, router, domainName, defaultMessageFields(
		"sender@filter-tags.example.com", "user@example.com", "Untagged Message",
	))

	rec := getEvents(t, router, domainName, map[string]string{"tags": "welcome"})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp eventsResponse
	decodeJSON(t, rec, &resp)

	if len(resp.Items) == 0 {
		t.Fatal("expected events with tag 'welcome', got none")
	}

	for _, item := range resp.Items {
		hasTag := false
		for _, tag := range item.Tags {
			if tag == "welcome" {
				hasTag = true
				break
			}
		}
		if !hasTag {
			t.Errorf("expected event to have tag 'welcome', got tags: %v", item.Tags)
		}
	}
}

func TestFilterBySeverity(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	domainName := "filter-severity.example.com"
	createTestDomain(t, router, domainName)

	// Insert failed events directly into the database so we can test the filter
	eh := event.NewHandlers(db, cfg)
	_ = eh // handlers are used via the router

	failedEvent := event.Event{
		DomainName: domainName,
		EventType:  "failed",
		Timestamp:  float64(time.Now().UnixMicro()) / 1e6,
		LogLevel:   "error",
		MessageID:  "<test@filter-severity.example.com>",
		StorageKey: "test@filter-severity.example.com",
		Recipient:  "bounce@example.com",
		Severity:   "permanent",
		Reason:     "bounce",
	}
	if err := db.Create(&failedEvent).Error; err != nil {
		t.Fatalf("failed to create test failed event: %v", err)
	}

	tempFailedEvent := event.Event{
		DomainName: domainName,
		EventType:  "failed",
		Timestamp:  float64(time.Now().UnixMicro()) / 1e6,
		LogLevel:   "warn",
		MessageID:  "<test2@filter-severity.example.com>",
		StorageKey: "test2@filter-severity.example.com",
		Recipient:  "retry@example.com",
		Severity:   "temporary",
		Reason:     "generic",
	}
	if err := db.Create(&tempFailedEvent).Error; err != nil {
		t.Fatalf("failed to create test temp failed event: %v", err)
	}

	rec := getEvents(t, router, domainName, map[string]string{"severity": "permanent"})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp eventsResponse
	decodeJSON(t, rec, &resp)

	if len(resp.Items) == 0 {
		t.Fatal("expected at least one event with severity=permanent")
	}

	for _, item := range resp.Items {
		if item.Severity != "permanent" {
			t.Errorf("expected severity 'permanent', got %q", item.Severity)
		}
	}
}

func TestTimeRangeFiltering(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	domainName := "timerange.example.com"
	createTestDomain(t, router, domainName)

	// Insert events with known timestamps
	now := time.Now()
	pastEvent := event.Event{
		DomainName: domainName,
		EventType:  "accepted",
		Timestamp:  float64(now.Add(-2*time.Hour).UnixMicro()) / 1e6,
		LogLevel:   "info",
		MessageID:  "<past@timerange.example.com>",
		StorageKey: "past@timerange.example.com",
		Recipient:  "past@example.com",
	}
	recentEvent := event.Event{
		DomainName: domainName,
		EventType:  "accepted",
		Timestamp:  float64(now.Add(-30*time.Minute).UnixMicro()) / 1e6,
		LogLevel:   "info",
		MessageID:  "<recent@timerange.example.com>",
		StorageKey: "recent@timerange.example.com",
		Recipient:  "recent@example.com",
	}
	futureEvent := event.Event{
		DomainName: domainName,
		EventType:  "accepted",
		Timestamp:  float64(now.Add(2*time.Hour).UnixMicro()) / 1e6,
		LogLevel:   "info",
		MessageID:  "<future@timerange.example.com>",
		StorageKey: "future@timerange.example.com",
		Recipient:  "future@example.com",
	}

	for _, ev := range []event.Event{pastEvent, recentEvent, futureEvent} {
		if err := db.Create(&ev).Error; err != nil {
			t.Fatalf("failed to create test event: %v", err)
		}
	}

	// Query for events in the last hour only
	begin := fmt.Sprintf("%.6f", float64(now.Add(-1*time.Hour).UnixMicro())/1e6)
	end := fmt.Sprintf("%.6f", float64(now.UnixMicro())/1e6)

	rec := getEvents(t, router, domainName, map[string]string{
		"begin": begin,
		"end":   end,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp eventsResponse
	decodeJSON(t, rec, &resp)

	if len(resp.Items) != 1 {
		t.Errorf("expected 1 event in time range, got %d", len(resp.Items))
	}

	if len(resp.Items) > 0 && resp.Items[0].Recipient != "recent@example.com" {
		t.Errorf("expected event for recent@example.com, got %q", resp.Items[0].Recipient)
	}
}

func TestAscendingOrder(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	domainName := "ascending.example.com"
	createTestDomain(t, router, domainName)

	now := time.Now()
	for i := 0; i < 5; i++ {
		ev := event.Event{
			DomainName: domainName,
			EventType:  "accepted",
			Timestamp:  float64(now.Add(time.Duration(i)*time.Second).UnixMicro()) / 1e6,
			LogLevel:   "info",
			MessageID:  fmt.Sprintf("<msg%d@ascending.example.com>", i),
			StorageKey: fmt.Sprintf("msg%d@ascending.example.com", i),
			Recipient:  fmt.Sprintf("user%d@example.com", i),
		}
		if err := db.Create(&ev).Error; err != nil {
			t.Fatalf("failed to create event %d: %v", i, err)
		}
	}

	rec := getEvents(t, router, domainName, map[string]string{"ascending": "yes"})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp eventsResponse
	decodeJSON(t, rec, &resp)

	if len(resp.Items) < 2 {
		t.Fatalf("expected multiple events, got %d", len(resp.Items))
	}

	// Verify ascending order: each timestamp should be >= the previous one
	for i := 1; i < len(resp.Items); i++ {
		if resp.Items[i].Timestamp < resp.Items[i-1].Timestamp {
			t.Errorf("events not in ascending order: items[%d].timestamp=%f < items[%d].timestamp=%f",
				i, resp.Items[i].Timestamp, i-1, resp.Items[i-1].Timestamp)
		}
	}
}

func TestDescendingOrderDefault(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	domainName := "descending.example.com"
	createTestDomain(t, router, domainName)

	now := time.Now()
	for i := 0; i < 5; i++ {
		ev := event.Event{
			DomainName: domainName,
			EventType:  "accepted",
			Timestamp:  float64(now.Add(time.Duration(i)*time.Second).UnixMicro()) / 1e6,
			LogLevel:   "info",
			MessageID:  fmt.Sprintf("<msg%d@descending.example.com>", i),
			StorageKey: fmt.Sprintf("msg%d@descending.example.com", i),
			Recipient:  fmt.Sprintf("user%d@example.com", i),
		}
		if err := db.Create(&ev).Error; err != nil {
			t.Fatalf("failed to create event %d: %v", i, err)
		}
	}

	// Default order (no ascending param) should be newest first (descending)
	rec := getEvents(t, router, domainName, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp eventsResponse
	decodeJSON(t, rec, &resp)

	if len(resp.Items) < 2 {
		t.Fatalf("expected multiple events, got %d", len(resp.Items))
	}

	// Verify descending order: each timestamp should be <= the previous one
	for i := 1; i < len(resp.Items); i++ {
		if resp.Items[i].Timestamp > resp.Items[i-1].Timestamp {
			t.Errorf("events not in descending order: items[%d].timestamp=%f > items[%d].timestamp=%f",
				i, resp.Items[i].Timestamp, i-1, resp.Items[i-1].Timestamp)
		}
	}
}

func TestPaginationLimit(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	domainName := "pagelimit.example.com"
	createTestDomain(t, router, domainName)

	now := time.Now()
	for i := 0; i < 5; i++ {
		ev := event.Event{
			DomainName: domainName,
			EventType:  "accepted",
			Timestamp:  float64(now.Add(time.Duration(i)*time.Second).UnixMicro()) / 1e6,
			LogLevel:   "info",
			MessageID:  fmt.Sprintf("<msg%d@pagelimit.example.com>", i),
			StorageKey: fmt.Sprintf("msg%d@pagelimit.example.com", i),
			Recipient:  fmt.Sprintf("user%d@example.com", i),
		}
		if err := db.Create(&ev).Error; err != nil {
			t.Fatalf("failed to create event %d: %v", i, err)
		}
	}

	rec := getEvents(t, router, domainName, map[string]string{"limit": "2"})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp eventsResponse
	decodeJSON(t, rec, &resp)

	if len(resp.Items) > 2 {
		t.Errorf("expected at most 2 items, got %d", len(resp.Items))
	}

	// paging.next should be present since there are more events
	if resp.Paging.Next == "" {
		t.Error("expected paging.next to be present when more events exist")
	}
}

func TestPaginationMaxLimit(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	domainName := "maxlimit.example.com"
	createTestDomain(t, router, domainName)

	// Insert a few events
	now := time.Now()
	for i := 0; i < 3; i++ {
		ev := event.Event{
			DomainName: domainName,
			EventType:  "accepted",
			Timestamp:  float64(now.Add(time.Duration(i)*time.Second).UnixMicro()) / 1e6,
			LogLevel:   "info",
			MessageID:  fmt.Sprintf("<msg%d@maxlimit.example.com>", i),
			StorageKey: fmt.Sprintf("msg%d@maxlimit.example.com", i),
			Recipient:  fmt.Sprintf("user%d@example.com", i),
		}
		if err := db.Create(&ev).Error; err != nil {
			t.Fatalf("failed to create event %d: %v", i, err)
		}
	}

	// Request limit=500, should be clamped to 300
	rec := getEvents(t, router, domainName, map[string]string{"limit": "500"})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp eventsResponse
	decodeJSON(t, rec, &resp)

	// We only have 3 events, so we can't directly test the clamp.
	// But the request should succeed without error (not reject 500 as invalid).
	if len(resp.Items) > 300 {
		t.Errorf("expected at most 300 items (max limit), got %d", len(resp.Items))
	}
}

func TestEmptyResults(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	domainName := "empty.example.com"
	createTestDomain(t, router, domainName)

	// Don't send any messages - domain should have no events

	rec := getEvents(t, router, domainName, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp eventsResponse
	decodeJSON(t, rec, &resp)

	if len(resp.Items) != 0 {
		t.Errorf("expected 0 items for domain with no events, got %d", len(resp.Items))
	}

	// Paging should still be present
	if resp.Paging.First == "" || resp.Paging.Last == "" {
		t.Error("expected paging.first and paging.last to be present even for empty results")
	}
}

func TestPagingURLsAlwaysPresent(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	domainName := "paging.example.com"
	createTestDomain(t, router, domainName)

	sendTestMessage(t, router, domainName, defaultMessageFields(
		"sender@paging.example.com", "user@example.com", "Paging Test",
	))

	rec := getEvents(t, router, domainName, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp eventsResponse
	decodeJSON(t, rec, &resp)

	if resp.Paging.First == "" {
		t.Error("expected paging.first to be a non-empty URL")
	}
	if resp.Paging.Last == "" {
		t.Error("expected paging.last to be a non-empty URL")
	}
}

func TestDomainIsolation(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	domainA := "domaina.example.com"
	domainB := "domainb.example.com"
	createTestDomain(t, router, domainA)
	createTestDomain(t, router, domainB)

	sendTestMessage(t, router, domainA, defaultMessageFields(
		"sender@domaina.example.com", "user-a@example.com", "Domain A Message",
	))
	sendTestMessage(t, router, domainB, defaultMessageFields(
		"sender@domainb.example.com", "user-b@example.com", "Domain B Message",
	))

	// Query events for domain A
	recA := getEvents(t, router, domainA, nil)
	if recA.Code != http.StatusOK {
		t.Fatalf("expected 200 for domain A, got %d: %s", recA.Code, recA.Body.String())
	}

	var respA eventsResponse
	decodeJSON(t, recA, &respA)

	for _, item := range respA.Items {
		if item.Recipient == "user-b@example.com" {
			t.Errorf("domain A events should not contain domain B's events (found recipient %q)", item.Recipient)
		}
	}

	// Query events for domain B
	recB := getEvents(t, router, domainB, nil)
	if recB.Code != http.StatusOK {
		t.Fatalf("expected 200 for domain B, got %d: %s", recB.Code, recB.Body.String())
	}

	var respB eventsResponse
	decodeJSON(t, recB, &respB)

	for _, item := range respB.Items {
		if item.Recipient == "user-a@example.com" {
			t.Errorf("domain B events should not contain domain A's events (found recipient %q)", item.Recipient)
		}
	}
}

// ---------------------------------------------------------------------------
// Edge Case Tests
// ---------------------------------------------------------------------------

func TestEventsForNonExistentDomain(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	rec := getEvents(t, router, "nonexistent.example.com", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for non-existent domain, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestInvalidBeginEndFormat(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	domainName := "invalid-time.example.com"
	createTestDomain(t, router, domainName)

	rec := getEvents(t, router, domainName, map[string]string{"begin": "not-a-date"})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid begin format, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Additional Event Generation Tests (DB-level verification)
// ---------------------------------------------------------------------------

func TestEventRecipientDomainExtracted(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	domainName := "recipdomain.example.com"
	createTestDomain(t, router, domainName)

	sendTestMessage(t, router, domainName, defaultMessageFields(
		"sender@recipdomain.example.com", "user@target.org", "RecipDomain Test",
	))

	var events []event.Event
	if err := db.Where("domain_name = ?", domainName).Find(&events).Error; err != nil {
		t.Fatalf("failed to query events: %v", err)
	}

	if len(events) == 0 {
		t.Fatal("expected events, got none")
	}

	for _, ev := range events {
		if ev.RecipientDomain != "target.org" {
			t.Errorf("expected recipient_domain = %q, got %q", "target.org", ev.RecipientDomain)
		}
	}
}

func TestEventStorageFieldInResponse(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	domainName := "storage.example.com"
	createTestDomain(t, router, domainName)

	_, storageKey := sendTestMessage(t, router, domainName, defaultMessageFields(
		"sender@storage.example.com", "user@example.com", "Storage Test",
	))

	rec := getEvents(t, router, domainName, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp eventsResponse
	decodeJSON(t, rec, &resp)

	if len(resp.Items) == 0 {
		t.Fatal("expected events, got none")
	}

	for _, item := range resp.Items {
		if item.Storage == nil {
			t.Error("expected 'storage' field in event response")
			continue
		}
		key, ok := item.Storage["key"].(string)
		if !ok || key == "" {
			t.Error("expected 'storage.key' to be a non-empty string")
		}
		if key != storageKey {
			t.Errorf("expected storage.key = %q, got %q", storageKey, key)
		}
		url, ok := item.Storage["url"].(string)
		if !ok || url == "" {
			t.Error("expected 'storage.url' to be a non-empty string")
		}
		if !strings.Contains(url, storageKey) {
			t.Errorf("expected storage.url to contain the storage key %q, got %q", storageKey, url)
		}
	}
}

func TestEventMessageHeadersInResponse(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	domainName := "headers.example.com"
	createTestDomain(t, router, domainName)

	from := "sender@headers.example.com"
	to := "user@example.com"
	subject := "Headers Test"

	sendTestMessage(t, router, domainName, defaultMessageFields(from, to, subject))

	rec := getEvents(t, router, domainName, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp eventsResponse
	decodeJSON(t, rec, &resp)

	if len(resp.Items) == 0 {
		t.Fatal("expected events, got none")
	}

	item := resp.Items[0]
	if item.Message == nil {
		t.Fatal("expected 'message' field in event response")
	}

	headers, ok := item.Message["headers"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'message.headers' to be an object")
	}

	if headers["from"] != from {
		t.Errorf("expected message.headers.from = %q, got %v", from, headers["from"])
	}
	if headers["to"] != to {
		t.Errorf("expected message.headers.to = %q, got %v", to, headers["to"])
	}
	if headers["subject"] != subject {
		t.Errorf("expected message.headers.subject = %q, got %v", subject, headers["subject"])
	}
}

func TestTimeRangeFilteringWithRFC2822(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	domainName := "rfc2822.example.com"
	createTestDomain(t, router, domainName)

	now := time.Now()
	ev := event.Event{
		DomainName: domainName,
		EventType:  "accepted",
		Timestamp:  float64(now.UnixMicro()) / 1e6,
		LogLevel:   "info",
		MessageID:  "<test@rfc2822.example.com>",
		StorageKey: "test@rfc2822.example.com",
		Recipient:  "user@example.com",
	}
	if err := db.Create(&ev).Error; err != nil {
		t.Fatalf("failed to create event: %v", err)
	}

	// Use RFC 2822 format for the begin parameter
	beginTime := now.Add(-1 * time.Hour).Format(time.RFC1123Z)
	endTime := now.Add(1 * time.Hour).Format(time.RFC1123Z)

	rec := getEvents(t, router, domainName, map[string]string{
		"begin": beginTime,
		"end":   endTime,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp eventsResponse
	decodeJSON(t, rec, &resp)

	if len(resp.Items) == 0 {
		t.Error("expected event within RFC 2822 time range, got none")
	}
}

func TestDefaultPaginationLimit(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	domainName := "deflimit.example.com"
	createTestDomain(t, router, domainName)

	// Insert 105 events to exceed the default limit of 100
	now := time.Now()
	for i := 0; i < 105; i++ {
		ev := event.Event{
			DomainName: domainName,
			EventType:  "accepted",
			Timestamp:  float64(now.Add(time.Duration(i)*time.Millisecond).UnixMicro()) / 1e6,
			LogLevel:   "info",
			MessageID:  fmt.Sprintf("<msg%d@deflimit.example.com>", i),
			StorageKey: fmt.Sprintf("msg%d@deflimit.example.com", i),
			Recipient:  fmt.Sprintf("user%d@example.com", i),
		}
		if err := db.Create(&ev).Error; err != nil {
			t.Fatalf("failed to create event %d: %v", i, err)
		}
	}

	// Request with no limit parameter should default to 100
	rec := getEvents(t, router, domainName, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp eventsResponse
	decodeJSON(t, rec, &resp)

	if len(resp.Items) != 100 {
		t.Errorf("expected default limit of 100 items, got %d", len(resp.Items))
	}
}

func TestMultipleRecipientsAutoDeliverGeneratesAllEvents(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	cfg.EventGeneration.AutoDeliver = true
	router := setupRouter(db, cfg)

	domainName := "multideliver.example.com"
	createTestDomain(t, router, domainName)

	recipients := "alice@example.com, bob@example.com"
	sendTestMessage(t, router, domainName, defaultMessageFields(
		"sender@multideliver.example.com", recipients, "Multi Deliver",
	))

	var events []event.Event
	if err := db.Where("domain_name = ?", domainName).Find(&events).Error; err != nil {
		t.Fatalf("failed to query events: %v", err)
	}

	// Should have 2 accepted + 2 delivered = 4 events
	acceptedCount := 0
	deliveredCount := 0
	for _, ev := range events {
		switch ev.EventType {
		case "accepted":
			acceptedCount++
		case "delivered":
			deliveredCount++
		}
	}

	if acceptedCount != 2 {
		t.Errorf("expected 2 accepted events, got %d", acceptedCount)
	}
	if deliveredCount != 2 {
		t.Errorf("expected 2 delivered events, got %d", deliveredCount)
	}
}

func TestPaginationFollowNext(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	domainName := "follownext.example.com"
	createTestDomain(t, router, domainName)

	// Insert 5 events with distinct timestamps
	now := time.Now()
	for i := 0; i < 5; i++ {
		ev := event.Event{
			DomainName: domainName,
			EventType:  "accepted",
			Timestamp:  float64(now.Add(time.Duration(i)*time.Second).UnixMicro()) / 1e6,
			LogLevel:   "info",
			MessageID:  fmt.Sprintf("<msg%d@follownext.example.com>", i),
			StorageKey: fmt.Sprintf("msg%d@follownext.example.com", i),
			Recipient:  fmt.Sprintf("user%d@example.com", i),
		}
		if err := db.Create(&ev).Error; err != nil {
			t.Fatalf("failed to create event %d: %v", i, err)
		}
	}

	// Page 1: limit=2
	rec := getEvents(t, router, domainName, map[string]string{"limit": "2"})
	if rec.Code != http.StatusOK {
		t.Fatalf("page 1: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var page1 eventsResponse
	decodeJSON(t, rec, &page1)

	if len(page1.Items) != 2 {
		t.Fatalf("page 1: expected 2 items, got %d", len(page1.Items))
	}
	if page1.Paging.Next == "" {
		t.Fatal("page 1: expected paging.next to be present")
	}

	// Page 2: follow paging.next
	nextURL := page1.Paging.Next
	req2 := httptest.NewRequest(http.MethodGet, nextURL, nil)
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("page 2: expected 200, got %d: %s", rec2.Code, rec2.Body.String())
	}

	var page2 eventsResponse
	decodeJSON(t, rec2, &page2)

	if len(page2.Items) != 2 {
		t.Fatalf("page 2: expected 2 items, got %d", len(page2.Items))
	}

	// Verify no overlap: page 1 and page 2 should have different event IDs
	page1IDs := make(map[string]bool)
	for _, item := range page1.Items {
		page1IDs[item.ID] = true
	}
	for _, item := range page2.Items {
		if page1IDs[item.ID] {
			t.Errorf("page 2 contains event ID %s that was already on page 1", item.ID)
		}
	}
}
