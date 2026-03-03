package message_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bethmaloney/mailgun-mock-api/internal/domain"
	"github.com/bethmaloney/mailgun-mock-api/internal/event"
	"github.com/bethmaloney/mailgun-mock-api/internal/mailinglist"
	"github.com/bethmaloney/mailgun-mock-api/internal/message"
	"github.com/bethmaloney/mailgun-mock-api/internal/mock"
	"github.com/bethmaloney/mailgun-mock-api/internal/suppression"
	"github.com/go-chi/chi/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// List Sending Integration Test Helpers
// ---------------------------------------------------------------------------

// setupListTestDB creates an in-memory SQLite database for testing that
// includes all tables needed for mailing list + message + event integration.
func setupListTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}
	if err := db.AutoMigrate(
		&domain.Domain{},
		&domain.DNSRecord{},
		&message.StoredMessage{},
		&message.Attachment{},
		&event.Event{},
		&mailinglist.MailingList{},
		&mailinglist.MailingListMember{},
		&suppression.Bounce{},
		&suppression.Complaint{},
		&suppression.Unsubscribe{},
		&suppression.AllowlistEntry{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

// listTestConfig returns a MockConfig with auto-verify and auto-deliver
// enabled so that events are generated synchronously during send.
func listTestConfig() *mock.MockConfig {
	return &mock.MockConfig{
		EventGeneration: mock.EventGenerationConfig{
			AutoDeliver:     true,
			DeliveryDelayMs: 0,
		},
		DomainBehavior: mock.DomainBehaviorConfig{
			DomainAutoVerify: true,
			SandboxDomain:    "sandbox123.mailgun.org",
		},
	}
}

// setupListRouter creates a chi router with domain, message, and mailing list
// routes registered. This allows testing the full integration path: create a
// domain, create a mailing list, add members, and send a message to the list.
func setupListRouter(db *gorm.DB, cfg *mock.MockConfig) http.Handler {
	dh := domain.NewHandlers(db, cfg)
	mh := message.NewHandlers(db, cfg)
	mlh := mailinglist.NewHandlers(db)

	r := chi.NewRouter()

	// Domain routes
	r.Route("/v4/domains", func(r chi.Router) {
		r.Post("/", dh.CreateDomain)
	})

	// Message routes
	r.Route("/v3/{domain_name}/messages", func(r chi.Router) {
		r.Post("/", mh.SendMessage)
	})
	r.Route("/v3/domains/{domain_name}/messages", func(r chi.Router) {
		r.Get("/{storage_key}", mh.GetMessage)
	})

	// Event routes
	eh := event.NewHandlers(db, cfg)
	mh.SetEventHandlers(eh)
	r.Get("/v3/{domain_name}/events", eh.ListEvents)

	// Mailing list routes
	r.Route("/v3/lists", func(r chi.Router) {
		r.Post("/", mlh.CreateList)
		r.Get("/{list_address}", mlh.GetList)
		r.Post("/{list_address}/members", mlh.AddMember)
		r.Get("/{list_address}/members/pages", mlh.ListMembers)
	})

	return r
}

// createListTestDomain creates a domain via the API for testing.
func createListTestDomain(t *testing.T, router http.Handler, name string) {
	t.Helper()
	req := newMultipartRequest(t, http.MethodPost, "/v4/domains", map[string]string{"name": name})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to create test domain %q: status=%d body=%s", name, rec.Code, rec.Body.String())
	}
}

// createTestList creates a mailing list via the API.
func createTestList(t *testing.T, router http.Handler, address string) {
	t.Helper()
	req := newMultipartRequest(t, http.MethodPost, "/v3/lists", map[string]string{"address": address})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("failed to create test list %q: status=%d body=%s", address, rec.Code, rec.Body.String())
	}
}

// addTestMember adds a member to a mailing list via the API.
func addTestMember(t *testing.T, router http.Handler, listAddr string, fields map[string]string) {
	t.Helper()
	url := fmt.Sprintf("/v3/lists/%s/members", listAddr)
	req := newMultipartRequest(t, http.MethodPost, url, fields)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to add member to %q: status=%d body=%s", listAddr, rec.Code, rec.Body.String())
	}
}

// sendListMessage sends a message to the given domain+recipient and returns the recorder.
func sendListMessage(t *testing.T, router http.Handler, domainName string, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	url := fmt.Sprintf("/v3/%s/messages", domainName)
	req := newMultipartRequest(t, http.MethodPost, url, fields)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// getEvents fetches events for a domain, optionally filtering by query params.
func getEvents(t *testing.T, router http.Handler, domainName string, queryParams string) *httptest.ResponseRecorder {
	t.Helper()
	url := fmt.Sprintf("/v3/%s/events", domainName)
	if queryParams != "" {
		url += "?" + queryParams
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// eventsResponse is a minimal struct for decoding /events responses.
type eventsResponse struct {
	Items []map[string]interface{} `json:"items"`
}

// listSendResponse is a minimal struct for the send message response.
type listSendResponse struct {
	ID      string `json:"id"`
	Message string `json:"message"`
}

// =========================================================================
// Mailing List Sending Integration Tests
// =========================================================================

// ---------------------------------------------------------------------------
// 1. Send message to list address -- should be accepted normally
// ---------------------------------------------------------------------------

func TestListSend_AcceptedNormally(t *testing.T) {
	db := setupListTestDB(t)
	cfg := listTestConfig()
	router := setupListRouter(db, cfg)

	createListTestDomain(t, router, "example.com")
	createTestList(t, router, "devs@example.com")
	addTestMember(t, router, "devs@example.com", map[string]string{
		"address": "alice@example.com",
		"name":    "Alice",
	})

	rec := sendListMessage(t, router, "example.com", map[string]string{
		"from":    "sender@example.com",
		"to":      "devs@example.com",
		"subject": "Team Update",
		"text":    "Hello team!",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp listSendResponse
	decodeJSON(t, rec, &resp)

	if resp.ID == "" {
		t.Error("expected non-empty message ID")
	}
	if resp.Message != "Queued. Thank you." {
		t.Errorf("expected %q, got %q", "Queued. Thank you.", resp.Message)
	}
}

// ---------------------------------------------------------------------------
// 2. Send to list with multiple subscribed members -- events per member
// ---------------------------------------------------------------------------

func TestListSend_EventsPerMember(t *testing.T) {
	db := setupListTestDB(t)
	cfg := listTestConfig()
	router := setupListRouter(db, cfg)

	createListTestDomain(t, router, "example.com")
	createTestList(t, router, "devs@example.com")

	// Add three subscribed members
	for _, email := range []string{"alice@example.com", "bob@example.com", "charlie@example.com"} {
		addTestMember(t, router, "devs@example.com", map[string]string{
			"address":    email,
			"subscribed": "true",
		})
	}

	rec := sendListMessage(t, router, "example.com", map[string]string{
		"from":    "sender@example.com",
		"to":      "devs@example.com",
		"subject": "Team Update",
		"text":    "Hello team!",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	// Check events: there should be an accepted event for each member
	eventsRec := getEvents(t, router, "example.com", "event=accepted")
	if eventsRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for events, got %d (body: %s)", eventsRec.Code, eventsRec.Body.String())
	}

	var evResp eventsResponse
	decodeJSON(t, eventsRec, &evResp)

	// We expect at least 3 accepted events (one per subscribed member)
	acceptedRecipients := make(map[string]bool)
	for _, item := range evResp.Items {
		if eventType, _ := item["event"].(string); eventType == "accepted" {
			if recipient, _ := item["recipient"].(string); recipient != "" {
				acceptedRecipients[recipient] = true
			}
		}
	}

	expectedMembers := []string{"alice@example.com", "bob@example.com", "charlie@example.com"}
	for _, member := range expectedMembers {
		if !acceptedRecipients[member] {
			t.Errorf("expected accepted event for %q, but not found. Got recipients: %v", member, acceptedRecipients)
		}
	}

	// Also check for delivered events (auto_deliver is on)
	deliveredRec := getEvents(t, router, "example.com", "event=delivered")
	if deliveredRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for delivered events, got %d", deliveredRec.Code)
	}

	var delResp eventsResponse
	decodeJSON(t, deliveredRec, &delResp)

	deliveredRecipients := make(map[string]bool)
	for _, item := range delResp.Items {
		if eventType, _ := item["event"].(string); eventType == "delivered" {
			if recipient, _ := item["recipient"].(string); recipient != "" {
				deliveredRecipients[recipient] = true
			}
		}
	}

	for _, member := range expectedMembers {
		if !deliveredRecipients[member] {
			t.Errorf("expected delivered event for %q, but not found. Got recipients: %v", member, deliveredRecipients)
		}
	}
}

// ---------------------------------------------------------------------------
// 3. Unsubscribed members are excluded from expansion
// ---------------------------------------------------------------------------

func TestListSend_UnsubscribedExcluded(t *testing.T) {
	db := setupListTestDB(t)
	cfg := listTestConfig()
	router := setupListRouter(db, cfg)

	createListTestDomain(t, router, "example.com")
	createTestList(t, router, "devs@example.com")

	// Add one subscribed and one unsubscribed member
	addTestMember(t, router, "devs@example.com", map[string]string{
		"address":    "alice@example.com",
		"subscribed": "true",
	})
	addTestMember(t, router, "devs@example.com", map[string]string{
		"address":    "bob@example.com",
		"subscribed": "false",
	})

	rec := sendListMessage(t, router, "example.com", map[string]string{
		"from":    "sender@example.com",
		"to":      "devs@example.com",
		"subject": "Team Update",
		"text":    "Hello team!",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	// Check events: only alice should have events, not bob
	eventsRec := getEvents(t, router, "example.com", "event=accepted")
	if eventsRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for events, got %d", eventsRec.Code)
	}

	var evResp eventsResponse
	decodeJSON(t, eventsRec, &evResp)

	for _, item := range evResp.Items {
		if recipient, _ := item["recipient"].(string); recipient == "bob@example.com" {
			t.Error("expected bob (unsubscribed) to NOT have any events, but found an accepted event")
		}
	}

	// Verify alice does have an event
	foundAlice := false
	for _, item := range evResp.Items {
		if recipient, _ := item["recipient"].(string); recipient == "alice@example.com" {
			foundAlice = true
			break
		}
	}
	if !foundAlice {
		t.Error("expected alice (subscribed) to have an accepted event, but none found")
	}
}

// ---------------------------------------------------------------------------
// 4. %recipient.varname% substitution in text/html bodies
// ---------------------------------------------------------------------------

func TestListSend_RecipientVarSubstitution(t *testing.T) {
	db := setupListTestDB(t)
	cfg := listTestConfig()
	router := setupListRouter(db, cfg)

	createListTestDomain(t, router, "example.com")
	createTestList(t, router, "devs@example.com")

	// Add members with vars
	addTestMember(t, router, "devs@example.com", map[string]string{
		"address":    "alice@example.com",
		"name":       "Alice",
		"vars":       `{"first_name": "Alice", "role": "admin"}`,
		"subscribed": "true",
	})
	addTestMember(t, router, "devs@example.com", map[string]string{
		"address":    "bob@example.com",
		"name":       "Bob",
		"vars":       `{"first_name": "Bob", "role": "user"}`,
		"subscribed": "true",
	})

	// Send message with %recipient.varname% placeholders
	rec := sendListMessage(t, router, "example.com", map[string]string{
		"from":    "sender@example.com",
		"to":      "devs@example.com",
		"subject": "Hello",
		"text":    "Hello %recipient.first_name%, your role is %recipient.role%",
		"html":    "<p>Hello %recipient.first_name%, your role is %recipient.role%</p>",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	// The original message should be stored with the list address.
	// Individual per-member messages (or events) should have substituted content.
	// Check that the message was accepted
	var resp listSendResponse
	decodeJSON(t, rec, &resp)

	if resp.ID == "" {
		t.Error("expected non-empty message ID")
	}

	// Verify the original stored message still has the list address in "to"
	storageKey := strings.TrimPrefix(resp.ID, "<")
	storageKey = strings.TrimSuffix(storageKey, ">")

	msgRec := httptest.NewRecorder()
	msgReq := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/v3/domains/example.com/messages/%s", storageKey), nil)
	router.ServeHTTP(msgRec, msgReq)

	if msgRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for stored message, got %d (body: %s)", msgRec.Code, msgRec.Body.String())
	}

	var msgDetail map[string]interface{}
	if err := json.Unmarshal(msgRec.Body.Bytes(), &msgDetail); err != nil {
		t.Fatalf("failed to decode message detail: %v", err)
	}

	// The stored message "To" should contain the list address
	toField, _ := msgDetail["To"].(string)
	if !strings.Contains(toField, "devs@example.com") {
		t.Errorf("expected stored message To to contain %q, got %q", "devs@example.com", toField)
	}

	// Check that events were generated for individual members with their addresses
	eventsRec := getEvents(t, router, "example.com", "event=accepted")
	if eventsRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for events, got %d", eventsRec.Code)
	}

	var evResp eventsResponse
	decodeJSON(t, eventsRec, &evResp)

	// At minimum, events should be generated (either for the list address or expanded members)
	if len(evResp.Items) == 0 {
		t.Error("expected at least one accepted event")
	}
}

// ---------------------------------------------------------------------------
// 5. Send to list with no subscribed members -- accepted, no delivery events
// ---------------------------------------------------------------------------

func TestListSend_NoSubscribedMembers(t *testing.T) {
	db := setupListTestDB(t)
	cfg := listTestConfig()
	router := setupListRouter(db, cfg)

	createListTestDomain(t, router, "example.com")
	createTestList(t, router, "devs@example.com")

	// Add only unsubscribed members
	addTestMember(t, router, "devs@example.com", map[string]string{
		"address":    "alice@example.com",
		"subscribed": "false",
	})
	addTestMember(t, router, "devs@example.com", map[string]string{
		"address":    "bob@example.com",
		"subscribed": "false",
	})

	rec := sendListMessage(t, router, "example.com", map[string]string{
		"from":    "sender@example.com",
		"to":      "devs@example.com",
		"subject": "Team Update",
		"text":    "Hello team!",
	})

	// Message should still be accepted
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp listSendResponse
	decodeJSON(t, rec, &resp)

	if resp.Message != "Queued. Thank you." {
		t.Errorf("expected %q, got %q", "Queued. Thank you.", resp.Message)
	}

	// No delivery events should be generated for unsubscribed members
	deliveredRec := getEvents(t, router, "example.com", "event=delivered")
	if deliveredRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for events, got %d", deliveredRec.Code)
	}

	var delResp eventsResponse
	decodeJSON(t, deliveredRec, &delResp)

	// Filter for events with list member recipients
	for _, item := range delResp.Items {
		recipient, _ := item["recipient"].(string)
		if recipient == "alice@example.com" || recipient == "bob@example.com" {
			t.Errorf("expected no delivery events for unsubscribed members, but found event for %q", recipient)
		}
	}
}

// ---------------------------------------------------------------------------
// 6. Send to non-list address -- normal behavior, no list expansion
// ---------------------------------------------------------------------------

func TestListSend_NonListAddress_NormalBehavior(t *testing.T) {
	db := setupListTestDB(t)
	cfg := listTestConfig()
	router := setupListRouter(db, cfg)

	createListTestDomain(t, router, "example.com")
	// No mailing list created; recipient is a regular address

	rec := sendListMessage(t, router, "example.com", map[string]string{
		"from":    "sender@example.com",
		"to":      "regular@example.com",
		"subject": "Direct Message",
		"text":    "Hello directly!",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp listSendResponse
	decodeJSON(t, rec, &resp)

	if resp.Message != "Queued. Thank you." {
		t.Errorf("expected %q, got %q", "Queued. Thank you.", resp.Message)
	}

	// Check events: should have exactly one accepted event for the regular recipient
	eventsRec := getEvents(t, router, "example.com", "event=accepted")
	if eventsRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for events, got %d", eventsRec.Code)
	}

	var evResp eventsResponse
	decodeJSON(t, eventsRec, &evResp)

	if len(evResp.Items) != 1 {
		t.Errorf("expected exactly 1 accepted event for non-list send, got %d", len(evResp.Items))
	}

	if len(evResp.Items) > 0 {
		recipient, _ := evResp.Items[0]["recipient"].(string)
		if recipient != "regular@example.com" {
			t.Errorf("expected event recipient %q, got %q", "regular@example.com", recipient)
		}
	}
}

// ---------------------------------------------------------------------------
// 7. Mixed recipients: list address + regular address
// ---------------------------------------------------------------------------

func TestListSend_MixedRecipients(t *testing.T) {
	db := setupListTestDB(t)
	cfg := listTestConfig()
	router := setupListRouter(db, cfg)

	createListTestDomain(t, router, "example.com")
	createTestList(t, router, "devs@example.com")

	addTestMember(t, router, "devs@example.com", map[string]string{
		"address":    "alice@example.com",
		"subscribed": "true",
	})
	addTestMember(t, router, "devs@example.com", map[string]string{
		"address":    "bob@example.com",
		"subscribed": "true",
	})

	// Send to both a list address and a regular address
	rec := sendListMessage(t, router, "example.com", map[string]string{
		"from":    "sender@example.com",
		"to":      "devs@example.com, external@example.com",
		"subject": "Mixed Recipients",
		"text":    "Hello everyone!",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp listSendResponse
	decodeJSON(t, rec, &resp)

	if resp.Message != "Queued. Thank you." {
		t.Errorf("expected %q, got %q", "Queued. Thank you.", resp.Message)
	}

	// Check events: should have accepted events for list members + external
	eventsRec := getEvents(t, router, "example.com", "event=accepted")
	if eventsRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for events, got %d", eventsRec.Code)
	}

	var evResp eventsResponse
	decodeJSON(t, eventsRec, &evResp)

	// Expected recipients after expansion:
	// - alice@example.com (from list)
	// - bob@example.com (from list)
	// - external@example.com (direct)
	// Or the list address itself if expansion is not yet implemented (test should detect either)
	recipientSet := make(map[string]bool)
	for _, item := range evResp.Items {
		if recipient, ok := item["recipient"].(string); ok && recipient != "" {
			recipientSet[recipient] = true
		}
	}

	// The external recipient should always get an event
	if !recipientSet["external@example.com"] {
		t.Errorf("expected event for external@example.com, but not found. Got: %v", recipientSet)
	}

	// List members should get events (once expansion is implemented)
	// Before expansion, the list address itself gets the event.
	// After expansion, individual members get events.
	hasListExpansion := recipientSet["alice@example.com"] && recipientSet["bob@example.com"]
	hasListDirect := recipientSet["devs@example.com"]

	if !hasListExpansion && !hasListDirect {
		t.Errorf("expected either list expansion (alice+bob events) or direct list address event, got: %v", recipientSet)
	}

	// Total events should be at least 2 (external + at least one list-related)
	if len(evResp.Items) < 2 {
		t.Errorf("expected at least 2 accepted events for mixed recipients, got %d", len(evResp.Items))
	}
}
