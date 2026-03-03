package tag_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bethmaloney/mailgun-mock-api/internal/domain"
	"github.com/bethmaloney/mailgun-mock-api/internal/event"
	"github.com/bethmaloney/mailgun-mock-api/internal/message"
	"github.com/bethmaloney/mailgun-mock-api/internal/mock"
	"github.com/bethmaloney/mailgun-mock-api/internal/tag"
	"github.com/go-chi/chi/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const testDomain = "test.example.com"

// ---------------------------------------------------------------------------
// Test Helpers
// ---------------------------------------------------------------------------

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}
	if err := db.AutoMigrate(
		&domain.Domain{}, &domain.DNSRecord{},
		&tag.Tag{},
		&message.StoredMessage{}, &event.Event{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

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

func setupRouter(db *gorm.DB, cfg *mock.MockConfig) http.Handler {
	dh := domain.NewHandlers(db, cfg)
	tgh := tag.NewHandlers(db)
	eh := event.NewHandlers(db, cfg)
	mh := message.NewHandlers(db, cfg)
	r := chi.NewRouter()

	// Domain creation
	r.Route("/v4/domains", func(r chi.Router) {
		r.Post("/", dh.CreateDomain)
	})

	// Tag routes
	r.Route("/v3/{domain_name}/tags", func(r chi.Router) {
		r.Get("/", tgh.ListTags)
		r.Get("/{tag}", tgh.GetTag)
		r.Put("/{tag}", tgh.UpdateTag)
		r.Delete("/{tag}", tgh.DeleteTag)
	})

	// Tag limits route (note: different path pattern!)
	r.Get("/v3/domains/{domain_name}/limits/tag", tgh.GetTagLimits)

	// Message sending (for tag auto-creation tests)
	r.Route("/v3/{domain_name}/messages", func(r chi.Router) {
		r.Post("/", mh.SendMessage)
	})

	// Suppress event handler linting: eh is used to ensure event generation
	// works during message sends (the message handler creates one internally).
	_ = eh

	return r
}

func setup(t *testing.T) http.Handler {
	t.Helper()
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	// Create the test domain
	rec := doRequest(t, router, "POST", "/v4/domains", map[string]string{
		"name": testDomain,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to create test domain: %s", rec.Body.String())
	}

	return router
}

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

// doRequest supports repeated form fields (e.g., multiple "o:tag" values).
// Pass `fields` as nil for bodyless requests.
func doRequest(t *testing.T, router http.Handler, method, url string, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	var req *http.Request
	if fields != nil {
		req = newMultipartRequest(t, method, url, fields)
	} else {
		req = httptest.NewRequest(method, url, nil)
	}
	router.ServeHTTP(rec, req)
	return rec
}

type fieldPair struct {
	key   string
	value string
}

// doRequestMultiValue supports repeated form field keys (e.g., multiple o:tag values).
func doRequestMultiValue(t *testing.T, router http.Handler, method, url string, fields []fieldPair) *httptest.ResponseRecorder {
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
	req := httptest.NewRequest(method, url, &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	router.ServeHTTP(rec, req)
	return rec
}

func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder, dest interface{}) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dest); err != nil {
		t.Fatalf("failed to decode response (body=%q): %v", rec.Body.String(), err)
	}
}

func assertStatus(t *testing.T, rec *httptest.ResponseRecorder, expected int) {
	t.Helper()
	if rec.Code != expected {
		t.Errorf("expected status %d, got %d; body=%s", expected, rec.Code, rec.Body.String())
	}
}

func assertMessage(t *testing.T, rec *httptest.ResponseRecorder, expected string) {
	t.Helper()
	var body map[string]interface{}
	decodeJSON(t, rec, &body)
	msg, ok := body["message"].(string)
	if !ok {
		t.Fatalf("expected string 'message' field in response, got: %v", body)
	}
	if msg != expected {
		t.Errorf("expected message %q, got %q", expected, msg)
	}
}

// sendMessage sends a message via the API with the given o:tag value and
// returns the response recorder.
func sendMessage(t *testing.T, router http.Handler, tagValue string) *httptest.ResponseRecorder {
	t.Helper()
	return doRequest(t, router, "POST", fmt.Sprintf("/v3/%s/messages", testDomain), map[string]string{
		"from":    "sender@example.com",
		"to":      "recipient@example.com",
		"subject": "Test message",
		"text":    "Hello, world!",
		"o:tag":   tagValue,
	})
}

// sendMessageWithMultipleTags sends a message with multiple o:tag values.
func sendMessageWithMultipleTags(t *testing.T, router http.Handler, tags []string) *httptest.ResponseRecorder {
	t.Helper()
	fields := []fieldPair{
		{key: "from", value: "sender@example.com"},
		{key: "to", value: "recipient@example.com"},
		{key: "subject", value: "Test message"},
		{key: "text", value: "Hello, world!"},
	}
	for _, tg := range tags {
		fields = append(fields, fieldPair{key: "o:tag", value: tg})
	}
	return doRequestMultiValue(t, router, "POST", fmt.Sprintf("/v3/%s/messages", testDomain), fields)
}

// =========================================================================
// Tag CRUD Tests
// =========================================================================

// ---------------------------------------------------------------------------
// 1. GET /v3/{domain_name}/tags — List Tags (Empty)
// ---------------------------------------------------------------------------

func TestListTags_Empty(t *testing.T) {
	router := setup(t)

	rec := doRequest(t, router, "GET", fmt.Sprintf("/v3/%s/tags", testDomain), nil)
	assertStatus(t, rec, http.StatusOK)

	var body struct {
		Items  []interface{}          `json:"items"`
		Paging map[string]interface{} `json:"paging"`
	}
	decodeJSON(t, rec, &body)

	if len(body.Items) != 0 {
		t.Errorf("expected 0 items, got %d", len(body.Items))
	}

	// Paging should still be present
	if body.Paging == nil {
		t.Error("expected paging object in response")
	}
}

// ---------------------------------------------------------------------------
// 2. GET /v3/{domain_name}/tags/{tag} — Not Found
// ---------------------------------------------------------------------------

func TestGetTag_NotFound(t *testing.T) {
	router := setup(t)

	rec := doRequest(t, router, "GET", fmt.Sprintf("/v3/%s/tags/nonexistent", testDomain), nil)
	assertStatus(t, rec, http.StatusNotFound)
	assertMessage(t, rec, "tag not found")
}

// ---------------------------------------------------------------------------
// 3. PUT /v3/{domain_name}/tags/{tag} — Not Found
// ---------------------------------------------------------------------------

func TestUpdateTag_NotFound(t *testing.T) {
	router := setup(t)

	rec := doRequest(t, router, "PUT", fmt.Sprintf("/v3/%s/tags/nonexistent", testDomain), map[string]string{
		"description": "should fail",
	})
	assertStatus(t, rec, http.StatusNotFound)
	assertMessage(t, rec, "tag not found")
}

// ---------------------------------------------------------------------------
// 4. DELETE /v3/{domain_name}/tags/{tag} — Not Found
// ---------------------------------------------------------------------------

func TestDeleteTag_NotFound(t *testing.T) {
	router := setup(t)

	rec := doRequest(t, router, "DELETE", fmt.Sprintf("/v3/%s/tags/nonexistent", testDomain), nil)
	assertStatus(t, rec, http.StatusNotFound)
	assertMessage(t, rec, "tag not found")
}

// ---------------------------------------------------------------------------
// 5. Tag Auto-creation on Message Send
// ---------------------------------------------------------------------------

func TestTagAutoCreation_OnMessageSend(t *testing.T) {
	router := setup(t)

	// Send a message with a tag
	beforeSend := time.Now().UTC().Add(-1 * time.Second)
	rec := sendMessage(t, router, "newsletter")
	assertStatus(t, rec, http.StatusOK)

	// GET the tag — it should exist
	rec = doRequest(t, router, "GET", fmt.Sprintf("/v3/%s/tags/newsletter", testDomain), nil)
	assertStatus(t, rec, http.StatusOK)

	var tagResp map[string]interface{}
	decodeJSON(t, rec, &tagResp)

	if tagResp["tag"] != "newsletter" {
		t.Errorf("expected tag name %q, got %q", "newsletter", tagResp["tag"])
	}

	// Verify first-seen is set and reasonable
	firstSeen, ok := tagResp["first-seen"].(string)
	if !ok || firstSeen == "" {
		t.Fatalf("expected non-empty 'first-seen' field, got: %v", tagResp["first-seen"])
	}

	firstSeenTime, err := time.Parse(time.RFC3339, firstSeen)
	if err != nil {
		// Try with fractional seconds
		firstSeenTime, err = time.Parse(time.RFC3339Nano, firstSeen)
		if err != nil {
			t.Fatalf("failed to parse first-seen timestamp %q: %v", firstSeen, err)
		}
	}

	if firstSeenTime.Before(beforeSend) {
		t.Errorf("first-seen %v is before the message was sent %v", firstSeenTime, beforeSend)
	}
}

// ---------------------------------------------------------------------------
// 6. Tag Auto-creation with Multiple Tags
// ---------------------------------------------------------------------------

func TestTagAutoCreation_MultipleTags(t *testing.T) {
	router := setup(t)

	// Send a message with multiple tags
	rec := sendMessageWithMultipleTags(t, router, []string{"newsletter", "promo", "weekly"})
	assertStatus(t, rec, http.StatusOK)

	// Verify each tag was created
	for _, tagName := range []string{"newsletter", "promo", "weekly"} {
		rec = doRequest(t, router, "GET", fmt.Sprintf("/v3/%s/tags/%s", testDomain, tagName), nil)
		assertStatus(t, rec, http.StatusOK)

		var tagResp map[string]interface{}
		decodeJSON(t, rec, &tagResp)

		if tagResp["tag"] != tagName {
			t.Errorf("expected tag name %q, got %q", tagName, tagResp["tag"])
		}
	}
}

// ---------------------------------------------------------------------------
// 7. Tag Auto-creation Updates last-seen
// ---------------------------------------------------------------------------

func TestTagAutoCreation_UpdatesLastSeen(t *testing.T) {
	router := setup(t)

	// Send first message with the tag
	rec := sendMessage(t, router, "updates")
	assertStatus(t, rec, http.StatusOK)

	// Get the tag and record first-seen
	rec = doRequest(t, router, "GET", fmt.Sprintf("/v3/%s/tags/updates", testDomain), nil)
	assertStatus(t, rec, http.StatusOK)

	var tagResp1 map[string]interface{}
	decodeJSON(t, rec, &tagResp1)
	firstSeen1 := tagResp1["first-seen"]

	// Small delay to ensure time difference
	time.Sleep(10 * time.Millisecond)

	// Send second message with the same tag
	rec = sendMessage(t, router, "updates")
	assertStatus(t, rec, http.StatusOK)

	// Get the tag again
	rec = doRequest(t, router, "GET", fmt.Sprintf("/v3/%s/tags/updates", testDomain), nil)
	assertStatus(t, rec, http.StatusOK)

	var tagResp2 map[string]interface{}
	decodeJSON(t, rec, &tagResp2)

	// first-seen should remain unchanged
	if tagResp2["first-seen"] != firstSeen1 {
		t.Errorf("first-seen changed: %v -> %v", firstSeen1, tagResp2["first-seen"])
	}

	// last-seen should be updated (not nil/null and different from first seen, or at least present)
	lastSeen, ok := tagResp2["last-seen"].(string)
	if !ok || lastSeen == "" {
		t.Fatalf("expected non-empty 'last-seen' after second message, got: %v", tagResp2["last-seen"])
	}

	// Parse first-seen and last-seen to verify last-seen >= first-seen
	firstSeenStr, _ := firstSeen1.(string)
	fs, err1 := time.Parse(time.RFC3339Nano, firstSeenStr)
	if err1 != nil {
		fs, err1 = time.Parse(time.RFC3339, firstSeenStr)
	}
	ls, err2 := time.Parse(time.RFC3339Nano, lastSeen)
	if err2 != nil {
		ls, err2 = time.Parse(time.RFC3339, lastSeen)
	}

	if err1 == nil && err2 == nil {
		if ls.Before(fs) {
			t.Errorf("last-seen %v is before first-seen %v", ls, fs)
		}
	}
}

// ---------------------------------------------------------------------------
// 8. List Tags in Alphabetical Order
// ---------------------------------------------------------------------------

func TestListTags_Alphabetical(t *testing.T) {
	router := setup(t)

	// Create tags in non-alphabetical order
	for _, tagName := range []string{"zebra", "apple", "mango", "banana"} {
		rec := sendMessage(t, router, tagName)
		assertStatus(t, rec, http.StatusOK)
	}

	// List tags
	rec := doRequest(t, router, "GET", fmt.Sprintf("/v3/%s/tags", testDomain), nil)
	assertStatus(t, rec, http.StatusOK)

	var body struct {
		Items []map[string]interface{} `json:"items"`
	}
	decodeJSON(t, rec, &body)

	if len(body.Items) != 4 {
		t.Fatalf("expected 4 items, got %d", len(body.Items))
	}

	expectedOrder := []string{"apple", "banana", "mango", "zebra"}
	for i, expected := range expectedOrder {
		got, _ := body.Items[i]["tag"].(string)
		if got != expected {
			t.Errorf("item[%d]: expected tag %q, got %q", i, expected, got)
		}
	}
}

// ---------------------------------------------------------------------------
// 9. List Tags with Prefix Filter
// ---------------------------------------------------------------------------

func TestListTags_PrefixFilter(t *testing.T) {
	router := setup(t)

	// Create tags
	for _, tagName := range []string{"newsletter", "notify", "billing"} {
		rec := sendMessage(t, router, tagName)
		assertStatus(t, rec, http.StatusOK)
	}

	// Filter with prefix "ne" — only "newsletter" should match
	rec := doRequest(t, router, "GET", fmt.Sprintf("/v3/%s/tags?prefix=ne", testDomain), nil)
	assertStatus(t, rec, http.StatusOK)

	var body struct {
		Items []map[string]interface{} `json:"items"`
	}
	decodeJSON(t, rec, &body)

	if len(body.Items) != 1 {
		t.Fatalf("expected 1 item with prefix 'ne', got %d", len(body.Items))
	}

	got, _ := body.Items[0]["tag"].(string)
	if got != "newsletter" {
		t.Errorf("expected tag %q, got %q", "newsletter", got)
	}
}

// ---------------------------------------------------------------------------
// 10. List Tags with Pagination
// ---------------------------------------------------------------------------

func TestListTags_Pagination(t *testing.T) {
	router := setup(t)

	// Create enough tags to require pagination
	tagNames := []string{"alpha", "bravo", "charlie", "delta", "echo"}
	for _, tagName := range tagNames {
		rec := sendMessage(t, router, tagName)
		assertStatus(t, rec, http.StatusOK)
	}

	// Request with limit=2
	rec := doRequest(t, router, "GET", fmt.Sprintf("/v3/%s/tags?limit=2", testDomain), nil)
	assertStatus(t, rec, http.StatusOK)

	var body struct {
		Items  []map[string]interface{} `json:"items"`
		Paging map[string]interface{}   `json:"paging"`
	}
	decodeJSON(t, rec, &body)

	if len(body.Items) != 2 {
		t.Fatalf("expected 2 items on first page, got %d", len(body.Items))
	}

	// Items should be alphabetically ordered: alpha, bravo
	if got, _ := body.Items[0]["tag"].(string); got != "alpha" {
		t.Errorf("first item: expected %q, got %q", "alpha", got)
	}
	if got, _ := body.Items[1]["tag"].(string); got != "bravo" {
		t.Errorf("second item: expected %q, got %q", "bravo", got)
	}

	// Paging should have first and last URLs
	if body.Paging == nil {
		t.Fatal("expected paging object in response")
	}
	if first, ok := body.Paging["first"].(string); !ok || first == "" {
		t.Error("expected non-empty 'first' paging URL")
	}
	if last, ok := body.Paging["last"].(string); !ok || last == "" {
		t.Error("expected non-empty 'last' paging URL")
	}

	// Next URL should be present since there are more items
	nextURL, ok := body.Paging["next"].(string)
	if !ok || nextURL == "" {
		t.Fatal("expected non-empty 'next' paging URL since there are more items")
	}

	// Follow the next URL to get the second page
	rec = httptest.NewRecorder()
	req := httptest.NewRequest("GET", nextURL, nil)
	router.ServeHTTP(rec, req)
	assertStatus(t, rec, http.StatusOK)

	var body2 struct {
		Items  []map[string]interface{} `json:"items"`
		Paging map[string]interface{}   `json:"paging"`
	}
	decodeJSON(t, rec, &body2)

	if len(body2.Items) != 2 {
		t.Fatalf("expected 2 items on second page, got %d", len(body2.Items))
	}

	// Second page should have: charlie, delta
	if got, _ := body2.Items[0]["tag"].(string); got != "charlie" {
		t.Errorf("third item: expected %q, got %q", "charlie", got)
	}
	if got, _ := body2.Items[1]["tag"].(string); got != "delta" {
		t.Errorf("fourth item: expected %q, got %q", "delta", got)
	}
}

// ---------------------------------------------------------------------------
// 11. GET /v3/{domain_name}/tags/{tag} — Success
// ---------------------------------------------------------------------------

func TestGetTag_Success(t *testing.T) {
	router := setup(t)

	// Create a tag via message send
	rec := sendMessage(t, router, "welcome")
	assertStatus(t, rec, http.StatusOK)

	// Get the tag
	rec = doRequest(t, router, "GET", fmt.Sprintf("/v3/%s/tags/welcome", testDomain), nil)
	assertStatus(t, rec, http.StatusOK)

	var tagResp map[string]interface{}
	decodeJSON(t, rec, &tagResp)

	// Verify tag name
	if tagResp["tag"] != "welcome" {
		t.Errorf("expected tag %q, got %q", "welcome", tagResp["tag"])
	}

	// Verify description exists (defaults to empty string)
	if _, ok := tagResp["description"]; !ok {
		t.Error("expected 'description' field in response")
	}

	// Verify first-seen is present and is a valid timestamp
	firstSeen, ok := tagResp["first-seen"].(string)
	if !ok || firstSeen == "" {
		t.Fatalf("expected non-empty 'first-seen' field, got: %v", tagResp["first-seen"])
	}
	_, err := time.Parse(time.RFC3339Nano, firstSeen)
	if err != nil {
		_, err = time.Parse(time.RFC3339, firstSeen)
		if err != nil {
			t.Errorf("first-seen %q is not valid ISO 8601: %v", firstSeen, err)
		}
	}

	// Verify last-seen field is present (may be null for a single send)
	if _, ok := tagResp["last-seen"]; !ok {
		t.Error("expected 'last-seen' field in response")
	}
}

// ---------------------------------------------------------------------------
// 12. PUT /v3/{domain_name}/tags/{tag} — Update Description
// ---------------------------------------------------------------------------

func TestUpdateTag_Success(t *testing.T) {
	router := setup(t)

	// Create a tag via message send
	rec := sendMessage(t, router, "monthly")
	assertStatus(t, rec, http.StatusOK)

	// Update the tag description
	rec = doRequest(t, router, "PUT", fmt.Sprintf("/v3/%s/tags/monthly", testDomain), map[string]string{
		"description": "Monthly newsletter tag",
	})
	assertStatus(t, rec, http.StatusOK)
	assertMessage(t, rec, "Tag updated")

	// Verify the description was updated
	rec = doRequest(t, router, "GET", fmt.Sprintf("/v3/%s/tags/monthly", testDomain), nil)
	assertStatus(t, rec, http.StatusOK)

	var tagResp map[string]interface{}
	decodeJSON(t, rec, &tagResp)

	desc, _ := tagResp["description"].(string)
	if desc != "Monthly newsletter tag" {
		t.Errorf("expected description %q, got %q", "Monthly newsletter tag", desc)
	}
}

// ---------------------------------------------------------------------------
// 13. DELETE /v3/{domain_name}/tags/{tag} — Success
// ---------------------------------------------------------------------------

func TestDeleteTag_Success(t *testing.T) {
	router := setup(t)

	// Create a tag via message send
	rec := sendMessage(t, router, "temporary")
	assertStatus(t, rec, http.StatusOK)

	// Verify the tag exists
	rec = doRequest(t, router, "GET", fmt.Sprintf("/v3/%s/tags/temporary", testDomain), nil)
	assertStatus(t, rec, http.StatusOK)

	// Delete the tag
	rec = doRequest(t, router, "DELETE", fmt.Sprintf("/v3/%s/tags/temporary", testDomain), nil)
	assertStatus(t, rec, http.StatusOK)
	assertMessage(t, rec, "Tag has been removed")

	// Verify the tag is gone
	rec = doRequest(t, router, "GET", fmt.Sprintf("/v3/%s/tags/temporary", testDomain), nil)
	assertStatus(t, rec, http.StatusNotFound)

	// Verify the tag is not in the list
	rec = doRequest(t, router, "GET", fmt.Sprintf("/v3/%s/tags", testDomain), nil)
	assertStatus(t, rec, http.StatusOK)

	var body struct {
		Items []map[string]interface{} `json:"items"`
	}
	decodeJSON(t, rec, &body)

	for _, item := range body.Items {
		if item["tag"] == "temporary" {
			t.Error("deleted tag 'temporary' still appears in list")
		}
	}
}

// ---------------------------------------------------------------------------
// 14. GET /v3/domains/{domain_name}/limits/tag — Tag Limits
// ---------------------------------------------------------------------------

func TestGetTagLimits(t *testing.T) {
	router := setup(t)

	// Verify limits response with no tags
	rec := doRequest(t, router, "GET", fmt.Sprintf("/v3/domains/%s/limits/tag", testDomain), nil)
	assertStatus(t, rec, http.StatusOK)

	var limitsResp map[string]interface{}
	decodeJSON(t, rec, &limitsResp)

	// Verify the id field matches the domain name
	if id, _ := limitsResp["id"].(string); id != testDomain {
		t.Errorf("expected id %q, got %q", testDomain, id)
	}

	// Verify the limit field is present and is a number
	limitVal, ok := limitsResp["limit"].(float64)
	if !ok {
		t.Fatalf("expected 'limit' field as number, got: %v", limitsResp["limit"])
	}
	if limitVal <= 0 {
		t.Errorf("expected positive limit, got %v", limitVal)
	}

	// Verify count is 0 initially
	countVal, ok := limitsResp["count"].(float64)
	if !ok {
		t.Fatalf("expected 'count' field as number, got: %v", limitsResp["count"])
	}
	if countVal != 0 {
		t.Errorf("expected count 0, got %v", countVal)
	}

	// Create some tags and verify the count updates
	for _, tagName := range []string{"tag1", "tag2", "tag3"} {
		rec := sendMessage(t, router, tagName)
		assertStatus(t, rec, http.StatusOK)
	}

	rec = doRequest(t, router, "GET", fmt.Sprintf("/v3/domains/%s/limits/tag", testDomain), nil)
	assertStatus(t, rec, http.StatusOK)

	decodeJSON(t, rec, &limitsResp)

	countVal, ok = limitsResp["count"].(float64)
	if !ok {
		t.Fatalf("expected 'count' field as number, got: %v", limitsResp["count"])
	}
	if countVal != 3 {
		t.Errorf("expected count 3, got %v", countVal)
	}
}

// ---------------------------------------------------------------------------
// 15. Tag Auto-creation Case Insensitive
// ---------------------------------------------------------------------------

func TestTagAutoCreation_CaseInsensitive(t *testing.T) {
	router := setup(t)

	// Send messages with different casings of the same tag
	rec := sendMessage(t, router, "Newsletter")
	assertStatus(t, rec, http.StatusOK)

	rec = sendMessage(t, router, "newsletter")
	assertStatus(t, rec, http.StatusOK)

	rec = sendMessage(t, router, "NEWSLETTER")
	assertStatus(t, rec, http.StatusOK)

	// List tags — should have only one tag
	rec = doRequest(t, router, "GET", fmt.Sprintf("/v3/%s/tags", testDomain), nil)
	assertStatus(t, rec, http.StatusOK)

	var body struct {
		Items []map[string]interface{} `json:"items"`
	}
	decodeJSON(t, rec, &body)

	if len(body.Items) != 1 {
		t.Fatalf("expected 1 tag (case-insensitive dedup), got %d", len(body.Items))
	}

	// The tag name should be stored in lowercase
	got, _ := body.Items[0]["tag"].(string)
	if got != "newsletter" {
		t.Errorf("expected tag stored as %q, got %q", "newsletter", got)
	}
}

// ---------------------------------------------------------------------------
// Additional edge-case coverage
// ---------------------------------------------------------------------------

// TestListTags_PrefixFilter_NoMatch verifies that filtering with a prefix that
// matches no tags returns an empty items array.
func TestListTags_PrefixFilter_NoMatch(t *testing.T) {
	router := setup(t)

	// Create a tag
	rec := sendMessage(t, router, "newsletter")
	assertStatus(t, rec, http.StatusOK)

	// Filter with a prefix that does not match
	rec = doRequest(t, router, "GET", fmt.Sprintf("/v3/%s/tags?prefix=zzz", testDomain), nil)
	assertStatus(t, rec, http.StatusOK)

	var body struct {
		Items []map[string]interface{} `json:"items"`
	}
	decodeJSON(t, rec, &body)

	if len(body.Items) != 0 {
		t.Errorf("expected 0 items for non-matching prefix, got %d", len(body.Items))
	}
}

// TestListTags_PrefixFilter_MultipleMatches verifies that a prefix filter
// correctly returns all matching tags in alphabetical order.
func TestListTags_PrefixFilter_MultipleMatches(t *testing.T) {
	router := setup(t)

	// Create tags with different prefixes
	for _, tagName := range []string{"newsletter", "notify", "billing", "news-flash"} {
		rec := sendMessage(t, router, tagName)
		assertStatus(t, rec, http.StatusOK)
	}

	// Filter with prefix "n" — should match "newsletter", "news-flash", "notify"
	rec := doRequest(t, router, "GET", fmt.Sprintf("/v3/%s/tags?prefix=n", testDomain), nil)
	assertStatus(t, rec, http.StatusOK)

	var body struct {
		Items []map[string]interface{} `json:"items"`
	}
	decodeJSON(t, rec, &body)

	if len(body.Items) != 3 {
		t.Fatalf("expected 3 items with prefix 'n', got %d", len(body.Items))
	}

	// Verify alphabetical order: news-flash, newsletter, notify
	expectedOrder := []string{"news-flash", "newsletter", "notify"}
	for i, expected := range expectedOrder {
		got, _ := body.Items[i]["tag"].(string)
		if got != expected {
			t.Errorf("item[%d]: expected tag %q, got %q", i, expected, got)
		}
	}
}

// TestUpdateTag_DescriptionPersists verifies that updating a tag's description
// does not affect its first-seen or last-seen timestamps.
func TestUpdateTag_DescriptionPersists(t *testing.T) {
	router := setup(t)

	// Create a tag
	rec := sendMessage(t, router, "stable")
	assertStatus(t, rec, http.StatusOK)

	// Get original timestamps
	rec = doRequest(t, router, "GET", fmt.Sprintf("/v3/%s/tags/stable", testDomain), nil)
	assertStatus(t, rec, http.StatusOK)

	var before map[string]interface{}
	decodeJSON(t, rec, &before)

	// Update description
	rec = doRequest(t, router, "PUT", fmt.Sprintf("/v3/%s/tags/stable", testDomain), map[string]string{
		"description": "A stable tag",
	})
	assertStatus(t, rec, http.StatusOK)

	// Verify timestamps are unchanged
	rec = doRequest(t, router, "GET", fmt.Sprintf("/v3/%s/tags/stable", testDomain), nil)
	assertStatus(t, rec, http.StatusOK)

	var after map[string]interface{}
	decodeJSON(t, rec, &after)

	if before["first-seen"] != after["first-seen"] {
		t.Errorf("first-seen changed after description update: %v -> %v", before["first-seen"], after["first-seen"])
	}
}

// TestDeleteTag_ThenReCreate verifies that a tag can be re-created after deletion
// by sending a new message with the same tag name.
func TestDeleteTag_ThenReCreate(t *testing.T) {
	router := setup(t)

	// Create a tag
	rec := sendMessage(t, router, "recyclable")
	assertStatus(t, rec, http.StatusOK)

	// Delete the tag
	rec = doRequest(t, router, "DELETE", fmt.Sprintf("/v3/%s/tags/recyclable", testDomain), nil)
	assertStatus(t, rec, http.StatusOK)

	// Verify it is gone
	rec = doRequest(t, router, "GET", fmt.Sprintf("/v3/%s/tags/recyclable", testDomain), nil)
	assertStatus(t, rec, http.StatusNotFound)

	// Re-create via a new message send
	rec = sendMessage(t, router, "recyclable")
	assertStatus(t, rec, http.StatusOK)

	// Verify it exists again with a fresh first-seen
	rec = doRequest(t, router, "GET", fmt.Sprintf("/v3/%s/tags/recyclable", testDomain), nil)
	assertStatus(t, rec, http.StatusOK)

	var tagResp map[string]interface{}
	decodeJSON(t, rec, &tagResp)
	if tagResp["tag"] != "recyclable" {
		t.Errorf("expected tag %q, got %q", "recyclable", tagResp["tag"])
	}
}

// TestGetTagLimits_LimitValue verifies the tag limit is the expected default (5000).
func TestGetTagLimits_LimitValue(t *testing.T) {
	router := setup(t)

	rec := doRequest(t, router, "GET", fmt.Sprintf("/v3/domains/%s/limits/tag", testDomain), nil)
	assertStatus(t, rec, http.StatusOK)

	var limitsResp map[string]interface{}
	decodeJSON(t, rec, &limitsResp)

	limitVal, ok := limitsResp["limit"].(float64)
	if !ok {
		t.Fatalf("expected 'limit' field as number, got: %v", limitsResp["limit"])
	}
	if limitVal != 5000 {
		t.Errorf("expected tag limit of 5000, got %v", limitVal)
	}
}

// TestTagResponseUsesHyphenatedKeys verifies that the JSON response uses
// hyphenated keys (first-seen, last-seen) as per the Mailgun API spec.
func TestTagResponseUsesHyphenatedKeys(t *testing.T) {
	router := setup(t)

	rec := sendMessage(t, router, "hyphen-check")
	assertStatus(t, rec, http.StatusOK)

	rec = doRequest(t, router, "GET", fmt.Sprintf("/v3/%s/tags/hyphen-check", testDomain), nil)
	assertStatus(t, rec, http.StatusOK)

	// Check the raw JSON for hyphenated keys
	raw := rec.Body.String()

	if !strings.Contains(raw, `"first-seen"`) {
		t.Errorf("response should contain 'first-seen' key, got: %s", raw)
	}
	if !strings.Contains(raw, `"last-seen"`) {
		t.Errorf("response should contain 'last-seen' key, got: %s", raw)
	}

	// Should NOT contain camelCase or snake_case variants
	if strings.Contains(raw, `"firstSeen"`) || strings.Contains(raw, `"first_seen"`) {
		t.Errorf("response should use hyphenated keys, not camelCase/snake_case: %s", raw)
	}
	if strings.Contains(raw, `"lastSeen"`) || strings.Contains(raw, `"last_seen"`) {
		t.Errorf("response should use hyphenated keys, not camelCase/snake_case: %s", raw)
	}
}
