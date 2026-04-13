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
	"github.com/bethmaloney/mailgun-mock-api/internal/suppression"
	"github.com/go-chi/chi/v5"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// Suppression Integration Test Helpers
// ---------------------------------------------------------------------------

// setupTestDBWithSuppressions creates an in-memory SQLite database for testing
// with all required tables migrated, including suppression tables.
func setupTestDBWithSuppressions(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}
	if err := db.AutoMigrate(
		&domain.Domain{}, &domain.DNSRecord{},
		&message.StoredMessage{}, &message.Attachment{},
		&event.Event{},
		&suppression.Bounce{}, &suppression.Complaint{},
		&suppression.Unsubscribe{}, &suppression.AllowlistEntry{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

// setupSuppressionRouter creates a chi router with domain, message, event,
// suppression, and mock trigger routes registered. This is the full router
// needed for suppression integration testing.
func setupSuppressionRouter(db *gorm.DB, cfg *mock.MockConfig) http.Handler {
	domain.ResetForTests(db)
	event.ResetForTests(db)
	suppression.ResetForTests(db)
	dh := domain.NewHandlers(db, cfg)
	mh := message.NewHandlers(db, cfg)
	eh := event.NewHandlers(db, cfg)
	sh := suppression.NewHandlers(db)

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

	// Suppression routes
	r.Route("/v3/{domain_name}", func(r chi.Router) {
		// Bounces
		r.Get("/bounces", sh.ListBounces)
		r.Get("/bounces/{address}", sh.GetBounce)
		r.Post("/bounces", sh.CreateBounces)
		r.Delete("/bounces/{address}", sh.DeleteBounce)
		r.Delete("/bounces", sh.ClearBounces)

		// Complaints
		r.Get("/complaints", sh.ListComplaints)
		r.Get("/complaints/{address}", sh.GetComplaint)
		r.Post("/complaints", sh.CreateComplaints)
		r.Delete("/complaints/{address}", sh.DeleteComplaint)
		r.Delete("/complaints", sh.ClearComplaints)

		// Unsubscribes
		r.Get("/unsubscribes", sh.ListUnsubscribes)
		r.Get("/unsubscribes/{address}", sh.GetUnsubscribe)
		r.Post("/unsubscribes", sh.CreateUnsubscribes)
		r.Delete("/unsubscribes/{address}", sh.DeleteUnsubscribe)
		r.Delete("/unsubscribes", sh.ClearUnsubscribes)

		// Allowlist (whitelists)
		r.Get("/whitelists", sh.ListAllowlist)
		r.Get("/whitelists/{value}", sh.GetAllowlistEntry)
		r.Post("/whitelists", sh.CreateAllowlistEntry)
		r.Delete("/whitelists/{value}", sh.DeleteAllowlistEntry)
		r.Delete("/whitelists", sh.ClearAllowlist)
	})

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

// addBounce adds an address to the bounce suppression list for a domain.
func addBounce(t *testing.T, router http.Handler, domainName, address, code, errMsg string) {
	t.Helper()
	fields := map[string]string{
		"address": address,
	}
	if code != "" {
		fields["code"] = code
	}
	if errMsg != "" {
		fields["error"] = errMsg
	}
	req := newMultipartRequest(t, http.MethodPost, fmt.Sprintf("/v3/%s/bounces", domainName), fields)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to add bounce for %q: %d %s", address, rec.Code, rec.Body.String())
	}
}

// addComplaint adds an address to the complaint suppression list for a domain.
func addComplaint(t *testing.T, router http.Handler, domainName, address string) {
	t.Helper()
	req := newMultipartRequest(t, http.MethodPost, fmt.Sprintf("/v3/%s/complaints", domainName), map[string]string{
		"address": address,
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to add complaint for %q: %d %s", address, rec.Code, rec.Body.String())
	}
}

// addUnsubscribe adds an address to the unsubscribe suppression list with the
// given tag. Pass "*" for wildcard (all tags) or a specific tag name.
func addUnsubscribe(t *testing.T, router http.Handler, domainName, address, tag string) {
	t.Helper()
	req := newMultipartRequest(t, http.MethodPost, fmt.Sprintf("/v3/%s/unsubscribes", domainName), map[string]string{
		"address": address,
		"tag":     tag,
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to add unsubscribe for %q: %d %s", address, rec.Code, rec.Body.String())
	}
}

// addAllowlistAddress adds an email address to the allowlist for a domain.
func addAllowlistAddress(t *testing.T, router http.Handler, domainName, address string) {
	t.Helper()
	req := newMultipartRequest(t, http.MethodPost, fmt.Sprintf("/v3/%s/whitelists", domainName), map[string]string{
		"address": address,
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to add allowlist address %q: %d %s", address, rec.Code, rec.Body.String())
	}
}

// addAllowlistDomain adds a domain to the allowlist for a sending domain.
func addAllowlistDomain(t *testing.T, router http.Handler, domainName, allowDomain string) {
	t.Helper()
	req := newMultipartRequest(t, http.MethodPost, fmt.Sprintf("/v3/%s/whitelists", domainName), map[string]string{
		"domain": allowDomain,
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to add allowlist domain %q: %d %s", allowDomain, rec.Code, rec.Body.String())
	}
}

// sendMessageHelper sends a message and returns the parsed response including
// the message ID and storage key.
func sendMessageHelper(t *testing.T, router http.Handler, domainName string, fields map[string]string) (messageID, storageKey string) {
	t.Helper()
	return sendTestMessage(t, router, domainName, fields)
}

// sendMessageWithTags sends a message with one or more tags, returning the
// message ID and storage key.
func sendMessageWithTags(t *testing.T, router http.Handler, domainName string, from, to, subject string, tags []string) (messageID, storageKey string) {
	t.Helper()
	fields := []fieldPair{
		{Key: "from", Value: from},
		{Key: "to", Value: to},
		{Key: "subject", Value: subject},
		{Key: "text", Value: "Hello, this is a test message."},
	}
	for _, tag := range tags {
		fields = append(fields, fieldPair{Key: "o:tag", Value: tag})
	}
	return sendTestMessageWithRepeatedFields(t, router, domainName, fields)
}

// getAllEvents fetches all events for a domain and returns the raw items.
func getAllEvents(t *testing.T, router http.Handler, domainName string) []map[string]interface{} {
	t.Helper()
	rec := getEvents(t, router, domainName, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to get events: %d %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Items []map[string]interface{} `json:"items"`
	}
	decodeJSON(t, rec, &resp)
	return resp.Items
}

// getEventsForRecipient fetches events for a domain filtered by recipient.
func getEventsForRecipient(t *testing.T, router http.Handler, domainName, recipient string) []map[string]interface{} {
	t.Helper()
	rec := getEvents(t, router, domainName, map[string]string{"recipient": recipient})
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to get events: %d %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Items []map[string]interface{} `json:"items"`
	}
	decodeJSON(t, rec, &resp)
	return resp.Items
}

// findRawEventByType finds the first event item matching the given type.
func findRawEventByType(items []map[string]interface{}, eventType string) map[string]interface{} {
	for _, item := range items {
		if item["event"] == eventType {
			return item
		}
	}
	return nil
}

// findRawEventsByType returns all event items matching the given type.
func findRawEventsByType(items []map[string]interface{}, eventType string) []map[string]interface{} {
	var result []map[string]interface{}
	for _, item := range items {
		if item["event"] == eventType {
			result = append(result, item)
		}
	}
	return result
}

// findRawEventByTypeAndRecipient finds the first event matching both type and recipient.
func findRawEventByTypeAndRecipient(items []map[string]interface{}, eventType, recipient string) map[string]interface{} {
	for _, item := range items {
		if item["event"] == eventType && item["recipient"] == recipient {
			return item
		}
	}
	return nil
}

// getBounces fetches the bounce list for a domain and returns the items.
func getBounces(t *testing.T, router http.Handler, domainName string) []map[string]interface{} {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/v3/%s/bounces", domainName), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to list bounces: %d %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Items []map[string]interface{} `json:"items"`
	}
	decodeJSON(t, rec, &resp)
	return resp.Items
}

// getComplaints fetches the complaint list for a domain and returns the items.
func getComplaints(t *testing.T, router http.Handler, domainName string) []map[string]interface{} {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/v3/%s/complaints", domainName), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to list complaints: %d %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Items []map[string]interface{} `json:"items"`
	}
	decodeJSON(t, rec, &resp)
	return resp.Items
}

// getBounceForAddress checks if a specific address exists in the bounce list.
func getBounceForAddress(t *testing.T, router http.Handler, domainName, address string) (map[string]interface{}, bool) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/v3/%s/bounces/%s", domainName, address), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code == http.StatusNotFound {
		return nil, false
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status getting bounce for %q: %d %s", address, rec.Code, rec.Body.String())
	}
	var result map[string]interface{}
	decodeJSON(t, rec, &result)
	return result, true
}

// getComplaintForAddress checks if a specific address exists in the complaint list.
func getComplaintForAddress(t *testing.T, router http.Handler, domainName, address string) (map[string]interface{}, bool) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/v3/%s/complaints/%s", domainName, address), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code == http.StatusNotFound {
		return nil, false
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status getting complaint for %q: %d %s", address, rec.Code, rec.Body.String())
	}
	var result map[string]interface{}
	decodeJSON(t, rec, &result)
	return result, true
}

// ---------------------------------------------------------------------------
// Test 1: Bounce Suppression Prevents Delivery
// ---------------------------------------------------------------------------

func TestSuppressionIntegration_BounceSuppression(t *testing.T) {
	db := setupTestDBWithSuppressions(t)
	cfg := defaultConfig()
	router := setupSuppressionRouter(db, cfg)

	domainName := "supptest-bounce.example.com"
	createTestDomain(t, router, domainName)

	recipientAddr := "bounced@example.com"

	// Add recipient to bounce suppression list
	addBounce(t, router, domainName, recipientAddr, "550", "Mailbox not found")

	// Send message to the bounced address
	sendMessageHelper(t, router, domainName, defaultMessageFields(
		"sender@supptest-bounce.example.com", recipientAddr, "Bounce Test",
	))

	// Fetch events
	items := getAllEvents(t, router, domainName)

	// An accepted event should still be generated
	accepted := findRawEventByType(items, "accepted")
	if accepted == nil {
		t.Error("expected an accepted event for suppressed recipient, found none")
	}

	// A delivered event should NOT exist
	delivered := findRawEventByType(items, "delivered")
	if delivered != nil {
		t.Error("expected NO delivered event for bounce-suppressed recipient, but found one")
	}

	// A failed event with suppress-bounce reason should exist
	failed := findRawEventByType(items, "failed")
	if failed == nil {
		t.Fatal("expected a failed event for bounce-suppressed recipient, found none")
	}

	severity, _ := failed["severity"].(string)
	if severity != "permanent" {
		t.Errorf("expected severity %q, got %q", "permanent", severity)
	}

	reason, _ := failed["reason"].(string)
	if reason != "suppress-bounce" {
		t.Errorf("expected reason %q, got %q", "suppress-bounce", reason)
	}

	logLevel, _ := failed["log-level"].(string)
	if logLevel != "error" {
		t.Errorf("expected log-level %q, got %q", "error", logLevel)
	}
}

// ---------------------------------------------------------------------------
// Test 2: Complaint Suppression Prevents Delivery
// ---------------------------------------------------------------------------

func TestSuppressionIntegration_ComplaintSuppression(t *testing.T) {
	db := setupTestDBWithSuppressions(t)
	cfg := defaultConfig()
	router := setupSuppressionRouter(db, cfg)

	domainName := "supptest-complaint.example.com"
	createTestDomain(t, router, domainName)

	recipientAddr := "complainer@example.com"

	// Add recipient to complaint suppression list
	addComplaint(t, router, domainName, recipientAddr)

	// Send message to the complained address
	sendMessageHelper(t, router, domainName, defaultMessageFields(
		"sender@supptest-complaint.example.com", recipientAddr, "Complaint Test",
	))

	// Fetch events
	items := getAllEvents(t, router, domainName)

	// An accepted event should still be generated
	accepted := findRawEventByType(items, "accepted")
	if accepted == nil {
		t.Error("expected an accepted event for suppressed recipient, found none")
	}

	// A delivered event should NOT exist
	delivered := findRawEventByType(items, "delivered")
	if delivered != nil {
		t.Error("expected NO delivered event for complaint-suppressed recipient, but found one")
	}

	// A failed event with suppress-complaint reason should exist
	failed := findRawEventByType(items, "failed")
	if failed == nil {
		t.Fatal("expected a failed event for complaint-suppressed recipient, found none")
	}

	severity, _ := failed["severity"].(string)
	if severity != "permanent" {
		t.Errorf("expected severity %q, got %q", "permanent", severity)
	}

	reason, _ := failed["reason"].(string)
	if reason != "suppress-complaint" {
		t.Errorf("expected reason %q, got %q", "suppress-complaint", reason)
	}

	logLevel, _ := failed["log-level"].(string)
	if logLevel != "error" {
		t.Errorf("expected log-level %q, got %q", "error", logLevel)
	}
}

// ---------------------------------------------------------------------------
// Test 3: Unsubscribe Wildcard Suppression
// ---------------------------------------------------------------------------

func TestSuppressionIntegration_UnsubscribeWildcard(t *testing.T) {
	db := setupTestDBWithSuppressions(t)
	cfg := defaultConfig()
	router := setupSuppressionRouter(db, cfg)

	domainName := "supptest-unsub-wild.example.com"
	createTestDomain(t, router, domainName)

	recipientAddr := "unsubbed@example.com"

	// Add recipient to unsubscribe list with wildcard tag
	addUnsubscribe(t, router, domainName, recipientAddr, "*")

	// Send message (with or without tags, wildcard should always suppress)
	sendMessageHelper(t, router, domainName, defaultMessageFields(
		"sender@supptest-unsub-wild.example.com", recipientAddr, "Unsub Wildcard Test",
	))

	// Fetch events
	items := getAllEvents(t, router, domainName)

	// An accepted event should still be generated
	accepted := findRawEventByType(items, "accepted")
	if accepted == nil {
		t.Error("expected an accepted event for suppressed recipient, found none")
	}

	// A delivered event should NOT exist
	delivered := findRawEventByType(items, "delivered")
	if delivered != nil {
		t.Error("expected NO delivered event for unsubscribe-suppressed recipient, but found one")
	}

	// A failed event with suppress-unsubscribe reason should exist
	failed := findRawEventByType(items, "failed")
	if failed == nil {
		t.Fatal("expected a failed event for unsubscribe-suppressed recipient, found none")
	}

	reason, _ := failed["reason"].(string)
	if reason != "suppress-unsubscribe" {
		t.Errorf("expected reason %q, got %q", "suppress-unsubscribe", reason)
	}

	severity, _ := failed["severity"].(string)
	if severity != "permanent" {
		t.Errorf("expected severity %q, got %q", "permanent", severity)
	}

	logLevel, _ := failed["log-level"].(string)
	if logLevel != "error" {
		t.Errorf("expected log-level %q, got %q", "error", logLevel)
	}
}

// ---------------------------------------------------------------------------
// Test 4: Unsubscribe with Matching Tag
// ---------------------------------------------------------------------------

func TestSuppressionIntegration_UnsubscribeMatchingTag(t *testing.T) {
	db := setupTestDBWithSuppressions(t)
	cfg := defaultConfig()
	router := setupSuppressionRouter(db, cfg)

	domainName := "supptest-unsub-match.example.com"
	createTestDomain(t, router, domainName)

	recipientAddr := "taggeduser@example.com"

	// Add recipient to unsubscribe list with specific tag "newsletter"
	addUnsubscribe(t, router, domainName, recipientAddr, "newsletter")

	// Send message with matching tag o:tag=newsletter
	sendMessageWithTags(t, router, domainName,
		"sender@supptest-unsub-match.example.com", recipientAddr,
		"Newsletter Issue #42", []string{"newsletter"},
	)

	// Fetch events
	items := getAllEvents(t, router, domainName)

	// A delivered event should NOT exist (suppressed by matching tag)
	delivered := findRawEventByType(items, "delivered")
	if delivered != nil {
		t.Error("expected NO delivered event for unsubscribe-suppressed recipient with matching tag, but found one")
	}

	// A failed event with suppress-unsubscribe reason should exist
	failed := findRawEventByType(items, "failed")
	if failed == nil {
		t.Fatal("expected a failed event for unsubscribe-suppressed recipient with matching tag, found none")
	}

	reason, _ := failed["reason"].(string)
	if reason != "suppress-unsubscribe" {
		t.Errorf("expected reason %q, got %q", "suppress-unsubscribe", reason)
	}
}

// ---------------------------------------------------------------------------
// Test 5: Unsubscribe with Non-Matching Tag (No Suppression)
// ---------------------------------------------------------------------------

func TestSuppressionIntegration_UnsubscribeNonMatchingTag(t *testing.T) {
	db := setupTestDBWithSuppressions(t)
	cfg := defaultConfig()
	router := setupSuppressionRouter(db, cfg)

	domainName := "supptest-unsub-nomatch.example.com"
	createTestDomain(t, router, domainName)

	recipientAddr := "taggeduser2@example.com"

	// Add recipient to unsubscribe list with specific tag "newsletter"
	addUnsubscribe(t, router, domainName, recipientAddr, "newsletter")

	// Send message with a DIFFERENT tag o:tag=alerts
	sendMessageWithTags(t, router, domainName,
		"sender@supptest-unsub-nomatch.example.com", recipientAddr,
		"Alert: Server Down", []string{"alerts"},
	)

	// Fetch events
	items := getAllEvents(t, router, domainName)

	// A delivered event SHOULD exist (non-matching tag means no suppression)
	delivered := findRawEventByType(items, "delivered")
	if delivered == nil {
		t.Error("expected a delivered event when unsubscribe tag does not match message tag, found none")
	}

	// A suppression-failed event should NOT exist
	failed := findRawEventByType(items, "failed")
	if failed != nil {
		reason, _ := failed["reason"].(string)
		if reason == "suppress-unsubscribe" {
			t.Error("did NOT expect a suppress-unsubscribe failed event when tags do not match")
		}
	}
}

// ---------------------------------------------------------------------------
// Test 6: Multiple Recipients - Mixed Suppression
// ---------------------------------------------------------------------------

func TestSuppressionIntegration_MultipleRecipientsMixed(t *testing.T) {
	db := setupTestDBWithSuppressions(t)
	cfg := defaultConfig()
	router := setupSuppressionRouter(db, cfg)

	domainName := "supptest-multi.example.com"
	createTestDomain(t, router, domainName)

	suppressedAddr := "suppressed@example.com"
	normalAddr := "normal@example.com"

	// Add only the first recipient to the bounce suppression list
	addBounce(t, router, domainName, suppressedAddr, "550", "Mailbox not found")

	// Send message to both recipients
	sendMessageHelper(t, router, domainName, map[string]string{
		"from":    "sender@supptest-multi.example.com",
		"to":      suppressedAddr + ", " + normalAddr,
		"subject": "Multi Recipient Test",
		"text":    "Hello, this is a test message.",
	})

	// Fetch events
	items := getAllEvents(t, router, domainName)

	// Both recipients should have accepted events
	acceptedEvents := findRawEventsByType(items, "accepted")
	if len(acceptedEvents) < 2 {
		t.Errorf("expected at least 2 accepted events (one per recipient), got %d", len(acceptedEvents))
	}

	// The suppressed recipient should have a failed event
	failedForSuppressed := findRawEventByTypeAndRecipient(items, "failed", suppressedAddr)
	if failedForSuppressed == nil {
		t.Error("expected a failed event for the bounce-suppressed recipient, found none")
	} else {
		reason, _ := failedForSuppressed["reason"].(string)
		if reason != "suppress-bounce" {
			t.Errorf("expected reason %q for suppressed recipient, got %q", "suppress-bounce", reason)
		}
	}

	// The suppressed recipient should NOT have a delivered event
	deliveredForSuppressed := findRawEventByTypeAndRecipient(items, "delivered", suppressedAddr)
	if deliveredForSuppressed != nil {
		t.Error("expected NO delivered event for the bounce-suppressed recipient, but found one")
	}

	// The normal recipient should have a delivered event
	deliveredForNormal := findRawEventByTypeAndRecipient(items, "delivered", normalAddr)
	if deliveredForNormal == nil {
		t.Error("expected a delivered event for the non-suppressed recipient, found none")
	}

	// The normal recipient should NOT have a suppression-failed event
	failedForNormal := findRawEventByTypeAndRecipient(items, "failed", normalAddr)
	if failedForNormal != nil {
		t.Error("expected NO failed event for the non-suppressed recipient, but found one")
	}
}

// ---------------------------------------------------------------------------
// Test 7: No Suppression - Normal Delivery
// ---------------------------------------------------------------------------

func TestSuppressionIntegration_NoSuppressionNormalDelivery(t *testing.T) {
	db := setupTestDBWithSuppressions(t)
	cfg := defaultConfig()
	router := setupSuppressionRouter(db, cfg)

	domainName := "supptest-normal.example.com"
	createTestDomain(t, router, domainName)

	recipientAddr := "happyuser@example.com"

	// Send message with NO suppressions in place
	sendMessageHelper(t, router, domainName, defaultMessageFields(
		"sender@supptest-normal.example.com", recipientAddr, "Normal Delivery Test",
	))

	// Fetch events
	items := getAllEvents(t, router, domainName)

	// An accepted event should exist
	accepted := findRawEventByType(items, "accepted")
	if accepted == nil {
		t.Error("expected an accepted event, found none")
	}

	// A delivered event should exist (auto-deliver is on)
	delivered := findRawEventByType(items, "delivered")
	if delivered == nil {
		t.Error("expected a delivered event for non-suppressed recipient, found none")
	}

	// No failed event should exist
	failed := findRawEventByType(items, "failed")
	if failed != nil {
		t.Error("expected NO failed event for non-suppressed recipient, but found one")
	}
}

// ---------------------------------------------------------------------------
// Test 8: Failed Event Payload Shape for Suppression
// ---------------------------------------------------------------------------

func TestSuppressionIntegration_FailedEventPayloadShape(t *testing.T) {
	db := setupTestDBWithSuppressions(t)
	cfg := defaultConfig()
	router := setupSuppressionRouter(db, cfg)

	domainName := "supptest-payload.example.com"
	createTestDomain(t, router, domainName)

	recipientAddr := "bounced-payload@example.com"

	// Add recipient to bounce list
	addBounce(t, router, domainName, recipientAddr, "550", "Mailbox not found")

	// Send message
	sendMessageHelper(t, router, domainName, defaultMessageFields(
		"sender@supptest-payload.example.com", recipientAddr, "Payload Shape Test",
	))

	// Fetch events
	items := getAllEvents(t, router, domainName)

	// Find the failed event
	failed := findRawEventByType(items, "failed")
	if failed == nil {
		t.Fatal("expected a failed event for bounce-suppressed recipient, found none")
	}

	// Verify event type
	eventType, _ := failed["event"].(string)
	if eventType != "failed" {
		t.Errorf("expected event %q, got %q", "failed", eventType)
	}

	// Verify severity
	severity, _ := failed["severity"].(string)
	if severity != "permanent" {
		t.Errorf("expected severity %q, got %q", "permanent", severity)
	}

	// Verify reason
	reason, _ := failed["reason"].(string)
	if reason != "suppress-bounce" {
		t.Errorf("expected reason %q, got %q", "suppress-bounce", reason)
	}

	// Verify log-level
	logLevel, _ := failed["log-level"].(string)
	if logLevel != "error" {
		t.Errorf("expected log-level %q, got %q", "error", logLevel)
	}

	// Verify delivery-status exists and has expected fields
	deliveryStatus, ok := failed["delivery-status"].(map[string]interface{})
	if !ok {
		t.Fatal("expected delivery-status map in failed event payload")
	}

	// The delivery-status should have a code (e.g., 550)
	code, ok := deliveryStatus["code"].(float64)
	if !ok {
		t.Error("expected delivery-status code to be present")
	} else if code != 550 {
		t.Errorf("expected delivery-status code 550, got %v", code)
	}

	// The delivery-status should have a description matching the suppression reason
	description, _ := deliveryStatus["description"].(string)
	if description == "" {
		t.Error("expected non-empty delivery-status description")
	}

	// Verify envelope exists
	envelope, ok := failed["envelope"].(map[string]interface{})
	if !ok {
		t.Error("expected envelope map in failed event payload")
	} else {
		sender, _ := envelope["sender"].(string)
		if sender == "" {
			t.Error("expected non-empty sender in envelope")
		}
	}

	// Verify message headers exist
	msgField, ok := failed["message"].(map[string]interface{})
	if !ok {
		t.Error("expected message map in failed event payload")
	} else {
		headers, ok := msgField["headers"].(map[string]interface{})
		if !ok {
			t.Error("expected headers in message field")
		} else {
			if headers["message-id"] == nil {
				t.Error("expected message-id in headers")
			}
			if headers["from"] == nil {
				t.Error("expected from in headers")
			}
			if headers["to"] == nil {
				t.Error("expected to in headers")
			}
			if headers["subject"] == nil {
				t.Error("expected subject in headers")
			}
		}
	}

	// Verify tags exist (even if empty)
	if _, ok := failed["tags"]; !ok {
		t.Error("expected tags field in failed event payload")
	}

	// Verify user-variables exist (even if empty)
	if _, ok := failed["user-variables"]; !ok {
		t.Error("expected user-variables field in failed event payload")
	}

	// Verify storage exists
	storage, ok := failed["storage"].(map[string]interface{})
	if !ok {
		t.Error("expected storage map in failed event payload")
	} else {
		if storage["key"] == nil {
			t.Error("expected key in storage")
		}
		if storage["url"] == nil {
			t.Error("expected url in storage")
		}
	}

	// Verify recipient
	recipient, _ := failed["recipient"].(string)
	if recipient != recipientAddr {
		t.Errorf("expected recipient %q, got %q", recipientAddr, recipient)
	}

	// Verify recipient-domain
	recipientDomain, _ := failed["recipient-domain"].(string)
	if recipientDomain != "example.com" {
		t.Errorf("expected recipient-domain %q, got %q", "example.com", recipientDomain)
	}

	// Verify timestamp is present and reasonable
	timestamp, ok := failed["timestamp"].(float64)
	if !ok || timestamp == 0 {
		t.Error("expected a non-zero timestamp")
	}
}

// ---------------------------------------------------------------------------
// Test 9: Auto-Create Bounce on Fail Trigger (Permanent)
// ---------------------------------------------------------------------------

func TestAutoCreateBounceOnFail(t *testing.T) {
	db := setupTestDBWithSuppressions(t)
	cfg := defaultConfig()
	cfg.EventGeneration.AutoDeliver = false
	router := setupSuppressionRouter(db, cfg)

	domainName := "supptest-autobounce.example.com"
	createTestDomain(t, router, domainName)

	recipientAddr := "victim@example.com"

	// Send a message first
	_, storageKey := sendMessageHelper(t, router, domainName, defaultMessageFields(
		"sender@supptest-autobounce.example.com", recipientAddr, "Auto Bounce Test",
	))

	// Verify the recipient is NOT in the bounce list initially
	_, found := getBounceForAddress(t, router, domainName, recipientAddr)
	if found {
		t.Fatal("expected recipient NOT to be in bounce list before fail trigger")
	}

	// Trigger a permanent fail event
	triggerURL := fmt.Sprintf("/mock/events/%s/fail/%s", domainName, storageKey)
	body := map[string]string{"severity": "permanent", "reason": "bounce"}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, triggerURL, bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("fail trigger failed: %d %s", rec.Code, rec.Body.String())
	}

	// Verify the recipient is now in the bounce list
	_, found = getBounceForAddress(t, router, domainName, recipientAddr)
	if !found {
		t.Error("expected recipient to be auto-added to bounce list after permanent fail trigger")
	}
}

// ---------------------------------------------------------------------------
// Test 10: Auto-Create Bounce on Fail Trigger (Temporary) - Should NOT create
// ---------------------------------------------------------------------------

func TestAutoCreateBounceOnFail_Temporary(t *testing.T) {
	db := setupTestDBWithSuppressions(t)
	cfg := defaultConfig()
	cfg.EventGeneration.AutoDeliver = false
	router := setupSuppressionRouter(db, cfg)

	domainName := "supptest-tempbounce.example.com"
	createTestDomain(t, router, domainName)

	recipientAddr := "tempfail@example.com"

	// Send a message first
	_, storageKey := sendMessageHelper(t, router, domainName, defaultMessageFields(
		"sender@supptest-tempbounce.example.com", recipientAddr, "Temp Bounce Test",
	))

	// Trigger a temporary fail event
	triggerURL := fmt.Sprintf("/mock/events/%s/fail/%s", domainName, storageKey)
	body := map[string]string{"severity": "temporary", "reason": "bounce"}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, triggerURL, bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("fail trigger failed: %d %s", rec.Code, rec.Body.String())
	}

	// Verify the recipient is NOT in the bounce list (temporary fails should not auto-create bounces)
	_, found := getBounceForAddress(t, router, domainName, recipientAddr)
	if found {
		t.Error("expected recipient NOT to be auto-added to bounce list after temporary fail trigger")
	}
}

// ---------------------------------------------------------------------------
// Test 11: Auto-Create Complaint on Complain Trigger
// ---------------------------------------------------------------------------

func TestAutoCreateComplaintOnComplain(t *testing.T) {
	db := setupTestDBWithSuppressions(t)
	cfg := defaultConfig()
	cfg.EventGeneration.AutoDeliver = false
	router := setupSuppressionRouter(db, cfg)

	domainName := "supptest-autocomp.example.com"
	createTestDomain(t, router, domainName)

	recipientAddr := "complainer-auto@example.com"

	// Send a message first
	_, storageKey := sendMessageHelper(t, router, domainName, defaultMessageFields(
		"sender@supptest-autocomp.example.com", recipientAddr, "Auto Complaint Test",
	))

	// Verify the recipient is NOT in the complaint list initially
	_, found := getComplaintForAddress(t, router, domainName, recipientAddr)
	if found {
		t.Fatal("expected recipient NOT to be in complaint list before complain trigger")
	}

	// Trigger a complain event
	triggerURL := fmt.Sprintf("/mock/events/%s/complain/%s", domainName, storageKey)
	req := httptest.NewRequest(http.MethodPost, triggerURL, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("complain trigger failed: %d %s", rec.Code, rec.Body.String())
	}

	// Verify the recipient is now in the complaint list
	_, found = getComplaintForAddress(t, router, domainName, recipientAddr)
	if !found {
		t.Error("expected recipient to be auto-added to complaint list after complain trigger")
	}
}

// ---------------------------------------------------------------------------
// Test 12: Allowlist Prevents Auto-Bounce (Address)
// ---------------------------------------------------------------------------

func TestAllowlistPreventsAutoBounce(t *testing.T) {
	db := setupTestDBWithSuppressions(t)
	cfg := defaultConfig()
	cfg.EventGeneration.AutoDeliver = false
	router := setupSuppressionRouter(db, cfg)

	domainName := "supptest-allowbounce.example.com"
	createTestDomain(t, router, domainName)

	recipientAddr := "protected@example.com"

	// Add recipient address to the allowlist
	addAllowlistAddress(t, router, domainName, recipientAddr)

	// Send a message
	_, storageKey := sendMessageHelper(t, router, domainName, defaultMessageFields(
		"sender@supptest-allowbounce.example.com", recipientAddr, "Allowlist Test",
	))

	// Trigger a permanent fail event
	triggerURL := fmt.Sprintf("/mock/events/%s/fail/%s", domainName, storageKey)
	body := map[string]string{"severity": "permanent", "reason": "bounce"}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, triggerURL, bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("fail trigger failed: %d %s", rec.Code, rec.Body.String())
	}

	// Verify the recipient is NOT in the bounce list (protected by allowlist)
	_, found := getBounceForAddress(t, router, domainName, recipientAddr)
	if found {
		t.Error("expected recipient NOT to be auto-added to bounce list because they are on the allowlist")
	}
}

// ---------------------------------------------------------------------------
// Test 13: Allowlist Domain Prevents Auto-Bounce
// ---------------------------------------------------------------------------

func TestAllowlistDomainPreventsAutoBounce(t *testing.T) {
	db := setupTestDBWithSuppressions(t)
	cfg := defaultConfig()
	cfg.EventGeneration.AutoDeliver = false
	router := setupSuppressionRouter(db, cfg)

	domainName := "supptest-allowdomain.example.com"
	createTestDomain(t, router, domainName)

	recipientAddr := "user@protected-domain.com"

	// Add the recipient's domain to the allowlist
	addAllowlistDomain(t, router, domainName, "protected-domain.com")

	// Send a message
	_, storageKey := sendMessageHelper(t, router, domainName, defaultMessageFields(
		"sender@supptest-allowdomain.example.com", recipientAddr, "Allowlist Domain Test",
	))

	// Trigger a permanent fail event
	triggerURL := fmt.Sprintf("/mock/events/%s/fail/%s", domainName, storageKey)
	body := map[string]string{"severity": "permanent", "reason": "bounce"}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, triggerURL, bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("fail trigger failed: %d %s", rec.Code, rec.Body.String())
	}

	// Verify the recipient is NOT in the bounce list (domain is on the allowlist)
	_, found := getBounceForAddress(t, router, domainName, recipientAddr)
	if found {
		t.Error("expected recipient NOT to be auto-added to bounce list because their domain is on the allowlist")
	}
}

// ---------------------------------------------------------------------------
// Test 14: Allowlist Does Not Prevent Complaint Auto-Creation
// ---------------------------------------------------------------------------

func TestAllowlistDoesNotPreventComplaint(t *testing.T) {
	db := setupTestDBWithSuppressions(t)
	cfg := defaultConfig()
	cfg.EventGeneration.AutoDeliver = false
	router := setupSuppressionRouter(db, cfg)

	domainName := "supptest-allowcomp.example.com"
	createTestDomain(t, router, domainName)

	recipientAddr := "allowed-complainer@example.com"

	// Add recipient address to the allowlist
	addAllowlistAddress(t, router, domainName, recipientAddr)

	// Send a message
	_, storageKey := sendMessageHelper(t, router, domainName, defaultMessageFields(
		"sender@supptest-allowcomp.example.com", recipientAddr, "Allowlist Complaint Test",
	))

	// Trigger a complain event
	triggerURL := fmt.Sprintf("/mock/events/%s/complain/%s", domainName, storageKey)
	req := httptest.NewRequest(http.MethodPost, triggerURL, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("complain trigger failed: %d %s", rec.Code, rec.Body.String())
	}

	// Verify the recipient IS in the complaint list (allowlist should NOT prevent complaints)
	_, found := getComplaintForAddress(t, router, domainName, recipientAddr)
	if !found {
		t.Error("expected recipient to be auto-added to complaint list even though they are on the allowlist (allowlist only protects against bounces)")
	}
}

// ---------------------------------------------------------------------------
// Test 15: Suppression Priority - Bounce Checked First
// ---------------------------------------------------------------------------

func TestSuppressionPriority_BounceCheckedFirst(t *testing.T) {
	db := setupTestDBWithSuppressions(t)
	cfg := defaultConfig()
	router := setupSuppressionRouter(db, cfg)

	domainName := "supptest-priority.example.com"
	createTestDomain(t, router, domainName)

	recipientAddr := "both-lists@example.com"

	// Add recipient to BOTH bounce and complaint suppression lists
	addBounce(t, router, domainName, recipientAddr, "550", "Mailbox not found")
	addComplaint(t, router, domainName, recipientAddr)

	// Send message
	sendMessageHelper(t, router, domainName, defaultMessageFields(
		"sender@supptest-priority.example.com", recipientAddr, "Priority Test",
	))

	// Fetch events
	items := getAllEvents(t, router, domainName)

	// A failed event should exist with suppress-bounce (bounce takes priority over complaint)
	failed := findRawEventByType(items, "failed")
	if failed == nil {
		t.Fatal("expected a failed event for recipient on both bounce and complaint lists, found none")
	}

	reason, _ := failed["reason"].(string)
	if reason != "suppress-bounce" {
		t.Errorf("expected reason %q (bounce should take priority), got %q", "suppress-bounce", reason)
	}

	// No delivered event should exist
	delivered := findRawEventByType(items, "delivered")
	if delivered != nil {
		t.Error("expected NO delivered event for suppressed recipient, but found one")
	}
}

// ---------------------------------------------------------------------------
// Test: Message Accepted HTTP 200 Even When Suppressed
// ---------------------------------------------------------------------------

func TestSuppressionIntegration_MessageStillAccepted(t *testing.T) {
	db := setupTestDBWithSuppressions(t)
	cfg := defaultConfig()
	router := setupSuppressionRouter(db, cfg)

	domainName := "supptest-accepted.example.com"
	createTestDomain(t, router, domainName)

	recipientAddr := "suppressed-but-accepted@example.com"

	// Add recipient to bounce list
	addBounce(t, router, domainName, recipientAddr, "550", "Mailbox not found")

	// Send message - should return HTTP 200 even though recipient is suppressed
	url := fmt.Sprintf("/v3/%s/messages", domainName)
	req := newMultipartRequest(t, http.MethodPost, url, defaultMessageFields(
		"sender@supptest-accepted.example.com", recipientAddr, "Accepted Test",
	))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected HTTP 200 for message to suppressed recipient, got %d", rec.Code)
	}

	// Verify the response contains a message ID
	var resp map[string]string
	decodeJSON(t, rec, &resp)
	if resp["id"] == "" {
		t.Error("expected non-empty message ID in response")
	}
	if resp["message"] != "Queued. Thank you." {
		t.Errorf("expected message %q, got %q", "Queued. Thank you.", resp["message"])
	}
}
