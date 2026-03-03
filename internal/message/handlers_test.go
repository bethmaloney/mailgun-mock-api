package message_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/bethmaloney/mailgun-mock-api/internal/domain"
	"github.com/bethmaloney/mailgun-mock-api/internal/message"
	"github.com/bethmaloney/mailgun-mock-api/internal/mock"
	"github.com/go-chi/chi/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// Test Helpers
// ---------------------------------------------------------------------------

// setupTestDB creates an in-memory SQLite database for testing with the
// Domain, DNSRecord, and StoredMessage tables migrated.
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}
	if err := db.AutoMigrate(&domain.Domain{}, &domain.DNSRecord{}, &message.StoredMessage{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

// defaultConfig returns a MockConfig with auto-verify enabled (the default).
func defaultConfig() *mock.MockConfig {
	return &mock.MockConfig{
		DomainBehavior: mock.DomainBehaviorConfig{
			DomainAutoVerify: true,
			SandboxDomain:    "sandbox123.mailgun.org",
		},
	}
}

// setupRouter creates a chi router with domain and message routes registered.
func setupRouter(db *gorm.DB, cfg *mock.MockConfig) http.Handler {
	dh := domain.NewHandlers(db, cfg)
	mh := message.NewHandlers(db, cfg)
	r := chi.NewRouter()
	r.Route("/v4/domains", func(r chi.Router) {
		r.Post("/", dh.CreateDomain)
	})
	r.Route("/v3/{domain_name}/messages", func(r chi.Router) {
		r.Post("/", mh.SendMessage)
	})
	r.Route("/v3/domains/{domain_name}/messages", func(r chi.Router) {
		r.Get("/{storage_key}", mh.GetMessage)
		r.Delete("/{storage_key}", mh.DeleteMessage)
	})
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

// decodeJSON unmarshals the response body into the provided destination.
func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder, dest interface{}) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dest); err != nil {
		t.Fatalf("failed to decode response (body=%q): %v", rec.Body.String(), err)
	}
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

// sendMessage sends a message via the API and returns the recorder.
func sendMessage(t *testing.T, router http.Handler, domainName string, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	url := fmt.Sprintf("/v3/%s/messages", domainName)
	req := newMultipartRequest(t, http.MethodPost, url, fields)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// sendMessageWithRepeatedFields sends a message with repeated field keys (e.g., multiple o:tag).
func sendMessageWithRepeatedFields(t *testing.T, router http.Handler, domainName string, fields []fieldPair) *httptest.ResponseRecorder {
	t.Helper()
	url := fmt.Sprintf("/v3/%s/messages", domainName)
	req := newMultipartRequestWithRepeatedFields(t, http.MethodPost, url, fields)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// getMessage retrieves a stored message via the API.
func getMessage(t *testing.T, router http.Handler, domainName, storageKey string) *httptest.ResponseRecorder {
	t.Helper()
	url := fmt.Sprintf("/v3/domains/%s/messages/%s", domainName, storageKey)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// deleteMessage deletes a stored message via the API.
func deleteMessage(t *testing.T, router http.Handler, domainName, storageKey string) *httptest.ResponseRecorder {
	t.Helper()
	url := fmt.Sprintf("/v3/domains/%s/messages/%s", domainName, storageKey)
	req := httptest.NewRequest(http.MethodDelete, url, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// extractStorageKey extracts the storage key from a send response message ID.
// The storage key is typically derived from or related to the message ID.
// In the mock, we expect the send response to include a storage key or the
// message ID can be used to retrieve the message. We parse the ID to derive it.
func extractStorageKey(t *testing.T, sendResp sendResponse) string {
	t.Helper()
	// The message ID is in angle brackets like <timestamp.random@domain>.
	// Strip the angle brackets and use the inner part as the storage key.
	id := sendResp.ID
	id = strings.TrimPrefix(id, "<")
	id = strings.TrimSuffix(id, ">")
	return id
}

// ---------------------------------------------------------------------------
// Response Structs for Assertions
// ---------------------------------------------------------------------------

type sendResponse struct {
	ID      string `json:"id"`
	Message string `json:"message"`
}

type errorResponse struct {
	Message string `json:"message"`
}

type messageDetailResponse struct {
	From           string     `json:"From"`
	To             string     `json:"To"`
	Subject        string     `json:"Subject"`
	Sender         string     `json:"sender"`
	Recipients     string     `json:"recipients"`
	BodyHTML       string     `json:"body-html"`
	BodyPlain      string     `json:"body-plain"`
	MessageHeaders [][]string `json:"message-headers"`
}

type deleteMessageResponse struct {
	Message string `json:"message"`
}

// ---------------------------------------------------------------------------
// Scenario 1: Basic send -- POST with from/to/subject/text -> 200
// ---------------------------------------------------------------------------

func TestSendMessage_BasicText(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	rec := sendMessage(t, router, "example.com", map[string]string{
		"from":    "sender@example.com",
		"to":      "recipient@example.com",
		"subject": "Hello",
		"text":    "Hello, World!",
	})

	t.Run("returns 200", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp sendResponse
	decodeJSON(t, rec, &resp)

	t.Run("returns message ID", func(t *testing.T) {
		if resp.ID == "" {
			t.Error("expected non-empty message ID")
		}
		// ID should be in angle brackets like <...@domain>
		if !strings.HasPrefix(resp.ID, "<") || !strings.HasSuffix(resp.ID, ">") {
			t.Errorf("expected ID in angle brackets, got %q", resp.ID)
		}
	})

	t.Run("returns queued message", func(t *testing.T) {
		if resp.Message != "Queued. Thank you." {
			t.Errorf("expected %q, got %q", "Queued. Thank you.", resp.Message)
		}
	})
}

// ---------------------------------------------------------------------------
// Scenario 2: HTML send -- POST with html body -> stored with html content
// ---------------------------------------------------------------------------

func TestSendMessage_HTMLBody(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	htmlContent := "<html><body><h1>Hello</h1></body></html>"
	rec := sendMessage(t, router, "example.com", map[string]string{
		"from":    "sender@example.com",
		"to":      "recipient@example.com",
		"subject": "HTML Test",
		"html":    htmlContent,
	})

	t.Run("returns 200", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp sendResponse
	decodeJSON(t, rec, &resp)

	t.Run("returns message ID", func(t *testing.T) {
		if resp.ID == "" {
			t.Error("expected non-empty message ID")
		}
	})

	// Retrieve the message and verify HTML is stored
	t.Run("stored message contains HTML body", func(t *testing.T) {
		storageKey := extractStorageKey(t, resp)
		getRec := getMessage(t, router, "example.com", storageKey)
		if getRec.Code != http.StatusOK {
			t.Fatalf("expected 200 on GET, got %d (body: %s)", getRec.Code, getRec.Body.String())
		}
		var detail messageDetailResponse
		decodeJSON(t, getRec, &detail)
		if detail.BodyHTML != htmlContent {
			t.Errorf("expected body-html=%q, got %q", htmlContent, detail.BodyHTML)
		}
	})
}

// ---------------------------------------------------------------------------
// Scenario 3: Missing "from" -> 400
// ---------------------------------------------------------------------------

func TestSendMessage_MissingFrom(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	rec := sendMessage(t, router, "example.com", map[string]string{
		"to":      "recipient@example.com",
		"subject": "Hello",
		"text":    "Hello, World!",
	})

	t.Run("returns 400", func(t *testing.T) {
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("body contains error message", func(t *testing.T) {
		var resp errorResponse
		decodeJSON(t, rec, &resp)
		if resp.Message == "" {
			t.Error("expected non-empty error message")
		}
		// The error should mention the "from" parameter
		if !strings.Contains(strings.ToLower(resp.Message), "from") {
			t.Errorf("expected error message to mention 'from', got %q", resp.Message)
		}
	})
}

// ---------------------------------------------------------------------------
// Scenario 4: Missing "to" -> 400
// ---------------------------------------------------------------------------

func TestSendMessage_MissingTo(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	rec := sendMessage(t, router, "example.com", map[string]string{
		"from":    "sender@example.com",
		"subject": "Hello",
		"text":    "Hello, World!",
	})

	t.Run("returns 400", func(t *testing.T) {
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("body contains error message", func(t *testing.T) {
		var resp errorResponse
		decodeJSON(t, rec, &resp)
		if resp.Message == "" {
			t.Error("expected non-empty error message")
		}
		// The error should mention the "to" parameter
		if !strings.Contains(strings.ToLower(resp.Message), "to") {
			t.Errorf("expected error message to mention 'to', got %q", resp.Message)
		}
	})
}

// ---------------------------------------------------------------------------
// Scenario 5: Missing body params (no text, html, amp-html, or template) -> 400
// ---------------------------------------------------------------------------

func TestSendMessage_MissingBodyParams(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	rec := sendMessage(t, router, "example.com", map[string]string{
		"from":    "sender@example.com",
		"to":      "recipient@example.com",
		"subject": "Hello",
	})

	t.Run("returns 400", func(t *testing.T) {
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("body contains error message about body params", func(t *testing.T) {
		var resp errorResponse
		decodeJSON(t, rec, &resp)
		if resp.Message == "" {
			t.Error("expected non-empty error message")
		}
	})
}

// ---------------------------------------------------------------------------
// Scenario 6: Tags -- POST with o:tag values -> stored correctly
// ---------------------------------------------------------------------------

func TestSendMessage_Tags(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	rec := sendMessageWithRepeatedFields(t, router, "example.com", []fieldPair{
		{Key: "from", Value: "sender@example.com"},
		{Key: "to", Value: "recipient@example.com"},
		{Key: "subject", Value: "Tagged Message"},
		{Key: "text", Value: "Hello with tags"},
		{Key: "o:tag", Value: "newsletter"},
		{Key: "o:tag", Value: "important"},
	})

	t.Run("returns 200", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp sendResponse
	decodeJSON(t, rec, &resp)

	t.Run("returns message ID", func(t *testing.T) {
		if resp.ID == "" {
			t.Error("expected non-empty message ID")
		}
	})

	// Retrieve and verify tags are stored
	t.Run("stored message contains tags", func(t *testing.T) {
		storageKey := extractStorageKey(t, resp)
		getRec := getMessage(t, router, "example.com", storageKey)
		if getRec.Code != http.StatusOK {
			t.Fatalf("expected 200 on GET, got %d (body: %s)", getRec.Code, getRec.Body.String())
		}
		// Verify the response body contains the tag information
		body := getRec.Body.String()
		if !strings.Contains(body, "newsletter") {
			t.Errorf("expected stored message to contain tag 'newsletter', body: %s", body)
		}
		if !strings.Contains(body, "important") {
			t.Errorf("expected stored message to contain tag 'important', body: %s", body)
		}
	})
}

// ---------------------------------------------------------------------------
// Scenario 7: Custom headers and variables -- h:Reply-To and v:user-id
// ---------------------------------------------------------------------------

func TestSendMessage_CustomHeadersAndVariables(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	rec := sendMessage(t, router, "example.com", map[string]string{
		"from":       "sender@example.com",
		"to":         "recipient@example.com",
		"subject":    "Custom Headers Test",
		"text":       "Hello with custom headers",
		"h:Reply-To": "reply@example.com",
		"v:user-id":  "12345",
	})

	t.Run("returns 200", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp sendResponse
	decodeJSON(t, rec, &resp)

	t.Run("returns message ID", func(t *testing.T) {
		if resp.ID == "" {
			t.Error("expected non-empty message ID")
		}
	})

	// Retrieve and verify custom headers/variables are stored
	t.Run("stored message contains custom headers and variables", func(t *testing.T) {
		storageKey := extractStorageKey(t, resp)
		getRec := getMessage(t, router, "example.com", storageKey)
		if getRec.Code != http.StatusOK {
			t.Fatalf("expected 200 on GET, got %d (body: %s)", getRec.Code, getRec.Body.String())
		}
		body := getRec.Body.String()
		if !strings.Contains(body, "reply@example.com") {
			t.Errorf("expected stored message to contain Reply-To header value, body: %s", body)
		}
		if !strings.Contains(body, "12345") {
			t.Errorf("expected stored message to contain v:user-id value, body: %s", body)
		}
	})
}

// ---------------------------------------------------------------------------
// Scenario 8: Test mode -- POST with o:testmode=yes -> 200
// ---------------------------------------------------------------------------

func TestSendMessage_TestMode(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	rec := sendMessage(t, router, "example.com", map[string]string{
		"from":       "sender@example.com",
		"to":         "recipient@example.com",
		"subject":    "Test Mode Message",
		"text":       "This is a test mode message",
		"o:testmode": "yes",
	})

	t.Run("returns 200", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp sendResponse
	decodeJSON(t, rec, &resp)

	t.Run("returns message ID", func(t *testing.T) {
		if resp.ID == "" {
			t.Error("expected non-empty message ID")
		}
	})

	t.Run("returns queued message", func(t *testing.T) {
		if resp.Message != "Queued. Thank you." {
			t.Errorf("expected %q, got %q", "Queued. Thank you.", resp.Message)
		}
	})
}

// ---------------------------------------------------------------------------
// Scenario 9: Invalid domain -- POST to non-existent domain -> 404
// ---------------------------------------------------------------------------

func TestSendMessage_InvalidDomain(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	// Do NOT create any domain

	rec := sendMessage(t, router, "nonexistent.com", map[string]string{
		"from":    "sender@nonexistent.com",
		"to":      "recipient@example.com",
		"subject": "Hello",
		"text":    "Hello, World!",
	})

	t.Run("returns 404", func(t *testing.T) {
		if rec.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})
}

// ---------------------------------------------------------------------------
// Scenario 10: Retrieve message -- Send, extract storage key, GET -> verify fields
// ---------------------------------------------------------------------------

func TestRetrieveMessage(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	// Send a message first
	sendRec := sendMessage(t, router, "example.com", map[string]string{
		"from":    "sender@example.com",
		"to":      "recipient@example.com",
		"subject": "Retrieve Test",
		"text":    "Plain text body",
		"html":    "<p>HTML body</p>",
	})

	if sendRec.Code != http.StatusOK {
		t.Fatalf("failed to send message: status=%d body=%s", sendRec.Code, sendRec.Body.String())
	}

	var sendResp sendResponse
	decodeJSON(t, sendRec, &sendResp)

	storageKey := extractStorageKey(t, sendResp)

	// Retrieve the message
	getRec := getMessage(t, router, "example.com", storageKey)

	t.Run("returns 200", func(t *testing.T) {
		if getRec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", getRec.Code, getRec.Body.String())
		}
	})

	var detail messageDetailResponse
	decodeJSON(t, getRec, &detail)

	t.Run("From field matches", func(t *testing.T) {
		if detail.From != "sender@example.com" {
			t.Errorf("expected From=%q, got %q", "sender@example.com", detail.From)
		}
	})

	t.Run("To field matches", func(t *testing.T) {
		if !strings.Contains(detail.To, "recipient@example.com") {
			t.Errorf("expected To to contain %q, got %q", "recipient@example.com", detail.To)
		}
	})

	t.Run("Subject field matches", func(t *testing.T) {
		if detail.Subject != "Retrieve Test" {
			t.Errorf("expected Subject=%q, got %q", "Retrieve Test", detail.Subject)
		}
	})

	t.Run("body-plain matches", func(t *testing.T) {
		if detail.BodyPlain != "Plain text body" {
			t.Errorf("expected body-plain=%q, got %q", "Plain text body", detail.BodyPlain)
		}
	})

	t.Run("body-html matches", func(t *testing.T) {
		if detail.BodyHTML != "<p>HTML body</p>" {
			t.Errorf("expected body-html=%q, got %q", "<p>HTML body</p>", detail.BodyHTML)
		}
	})

	t.Run("sender field is populated", func(t *testing.T) {
		if detail.Sender == "" {
			t.Error("expected non-empty sender field")
		}
	})

	t.Run("recipients field is populated", func(t *testing.T) {
		if detail.Recipients == "" {
			t.Error("expected non-empty recipients field")
		}
		if !strings.Contains(detail.Recipients, "recipient@example.com") {
			t.Errorf("expected recipients to contain %q, got %q", "recipient@example.com", detail.Recipients)
		}
	})

	t.Run("message-headers is an array of pairs", func(t *testing.T) {
		if detail.MessageHeaders == nil {
			t.Error("expected non-nil message-headers")
		}
		if len(detail.MessageHeaders) == 0 {
			t.Error("expected at least one message header pair")
		}
		// Each header should be a pair [name, value]
		for _, header := range detail.MessageHeaders {
			if len(header) != 2 {
				t.Errorf("expected header pair of length 2, got %d: %v", len(header), header)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Scenario 11: Delete message -- Send, DELETE -> 200, then GET -> 404
// ---------------------------------------------------------------------------

func TestDeleteMessage(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	// Send a message
	sendRec := sendMessage(t, router, "example.com", map[string]string{
		"from":    "sender@example.com",
		"to":      "recipient@example.com",
		"subject": "Delete Test",
		"text":    "To be deleted",
	})

	if sendRec.Code != http.StatusOK {
		t.Fatalf("failed to send message: status=%d body=%s", sendRec.Code, sendRec.Body.String())
	}

	var sendResp sendResponse
	decodeJSON(t, sendRec, &sendResp)

	storageKey := extractStorageKey(t, sendResp)

	// Delete the message
	delRec := deleteMessage(t, router, "example.com", storageKey)

	t.Run("delete returns 200", func(t *testing.T) {
		if delRec.Code != http.StatusOK {
			t.Errorf("expected 200 on DELETE, got %d (body: %s)", delRec.Code, delRec.Body.String())
		}
	})

	t.Run("delete returns correct message", func(t *testing.T) {
		var resp deleteMessageResponse
		decodeJSON(t, delRec, &resp)
		if resp.Message != "Message has been deleted" {
			t.Errorf("expected %q, got %q", "Message has been deleted", resp.Message)
		}
	})

	// Verify the message is gone
	t.Run("GET after DELETE returns 404", func(t *testing.T) {
		getRec := getMessage(t, router, "example.com", storageKey)
		if getRec.Code != http.StatusNotFound {
			t.Errorf("expected 404 after delete, got %d (body: %s)", getRec.Code, getRec.Body.String())
		}
	})
}

// ---------------------------------------------------------------------------
// Scenario 12: Multiple recipients -- POST with comma-separated "to"
// ---------------------------------------------------------------------------

func TestSendMessage_MultipleRecipients(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	rec := sendMessage(t, router, "example.com", map[string]string{
		"from":    "sender@example.com",
		"to":      "alice@example.com, bob@example.com, charlie@example.com",
		"subject": "Multiple Recipients",
		"text":    "Hello to all!",
	})

	t.Run("returns 200", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp sendResponse
	decodeJSON(t, rec, &resp)

	// Retrieve and verify all recipients are stored
	t.Run("stored message contains all recipients", func(t *testing.T) {
		storageKey := extractStorageKey(t, resp)
		getRec := getMessage(t, router, "example.com", storageKey)
		if getRec.Code != http.StatusOK {
			t.Fatalf("expected 200 on GET, got %d (body: %s)", getRec.Code, getRec.Body.String())
		}
		var detail messageDetailResponse
		decodeJSON(t, getRec, &detail)

		// The To or recipients field should contain all addresses
		body := getRec.Body.String()
		for _, addr := range []string{"alice@example.com", "bob@example.com", "charlie@example.com"} {
			if !strings.Contains(body, addr) {
				t.Errorf("expected stored message to contain recipient %q, body: %s", addr, body)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Scenario 13: CC and BCC -- POST with cc and bcc -> stored
// ---------------------------------------------------------------------------

func TestSendMessage_CCAndBCC(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	rec := sendMessage(t, router, "example.com", map[string]string{
		"from":    "sender@example.com",
		"to":      "primary@example.com",
		"cc":      "cc-user@example.com",
		"bcc":     "bcc-user@example.com",
		"subject": "CC and BCC Test",
		"text":    "Hello with CC and BCC",
	})

	t.Run("returns 200", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp sendResponse
	decodeJSON(t, rec, &resp)

	t.Run("returns message ID", func(t *testing.T) {
		if resp.ID == "" {
			t.Error("expected non-empty message ID")
		}
	})

	// Retrieve and verify CC/BCC are stored
	t.Run("stored message contains cc", func(t *testing.T) {
		storageKey := extractStorageKey(t, resp)
		getRec := getMessage(t, router, "example.com", storageKey)
		if getRec.Code != http.StatusOK {
			t.Fatalf("expected 200 on GET, got %d (body: %s)", getRec.Code, getRec.Body.String())
		}
		body := getRec.Body.String()
		if !strings.Contains(body, "cc-user@example.com") {
			t.Errorf("expected stored message to contain CC recipient, body: %s", body)
		}
	})
}

// ---------------------------------------------------------------------------
// Scenario 14: Recipient variables -- POST with recipient-variables JSON
// ---------------------------------------------------------------------------

func TestSendMessage_RecipientVariables(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	recipientVars := `{"alice@example.com":{"name":"Alice","id":1},"bob@example.com":{"name":"Bob","id":2}}`
	rec := sendMessage(t, router, "example.com", map[string]string{
		"from":                "sender@example.com",
		"to":                  "alice@example.com, bob@example.com",
		"subject":             "Batch Send",
		"text":                "Hello %recipient.name%!",
		"recipient-variables": recipientVars,
	})

	t.Run("returns 200", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp sendResponse
	decodeJSON(t, rec, &resp)

	t.Run("returns message ID", func(t *testing.T) {
		if resp.ID == "" {
			t.Error("expected non-empty message ID")
		}
	})

	// Retrieve and verify recipient-variables are stored
	t.Run("stored message contains recipient-variables", func(t *testing.T) {
		storageKey := extractStorageKey(t, resp)
		getRec := getMessage(t, router, "example.com", storageKey)
		if getRec.Code != http.StatusOK {
			t.Fatalf("expected 200 on GET, got %d (body: %s)", getRec.Code, getRec.Body.String())
		}
		body := getRec.Body.String()
		// The stored message should contain the recipient variables
		if !strings.Contains(body, "Alice") && !strings.Contains(body, "recipient-variables") {
			t.Errorf("expected stored message to contain recipient variable data, body: %s", body)
		}
	})
}

// ---------------------------------------------------------------------------
// Scenario 15: Message ID format -- Verify ID matches <timestamp.random@domain>
// ---------------------------------------------------------------------------

func TestSendMessage_MessageIDFormat(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	rec := sendMessage(t, router, "example.com", map[string]string{
		"from":    "sender@example.com",
		"to":      "recipient@example.com",
		"subject": "ID Format Test",
		"text":    "Testing message ID format",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp sendResponse
	decodeJSON(t, rec, &resp)

	t.Run("ID is in angle brackets", func(t *testing.T) {
		if !strings.HasPrefix(resp.ID, "<") {
			t.Errorf("expected ID to start with '<', got %q", resp.ID)
		}
		if !strings.HasSuffix(resp.ID, ">") {
			t.Errorf("expected ID to end with '>', got %q", resp.ID)
		}
	})

	t.Run("ID contains @ symbol", func(t *testing.T) {
		inner := strings.TrimPrefix(resp.ID, "<")
		inner = strings.TrimSuffix(inner, ">")
		if !strings.Contains(inner, "@") {
			t.Errorf("expected ID to contain '@', got %q", resp.ID)
		}
	})

	t.Run("ID ends with @domain>", func(t *testing.T) {
		inner := strings.TrimPrefix(resp.ID, "<")
		inner = strings.TrimSuffix(inner, ">")
		if !strings.HasSuffix(inner, "@example.com") {
			t.Errorf("expected ID to end with '@example.com', got %q", resp.ID)
		}
	})

	t.Run("ID matches timestamp.random@domain pattern", func(t *testing.T) {
		// Pattern: <timestamp.random@domain>
		// The timestamp and random parts should be separated by a dot,
		// followed by @domain
		pattern := `^<\d+\.\w+@example\.com>$`
		matched, err := regexp.MatchString(pattern, resp.ID)
		if err != nil {
			t.Fatalf("failed to compile regex: %v", err)
		}
		if !matched {
			t.Errorf("expected ID to match pattern %q, got %q", pattern, resp.ID)
		}
	})
}

// ---------------------------------------------------------------------------
// Scenario 16: Tracking options -- o:tracking, o:tracking-clicks, o:tracking-opens
// ---------------------------------------------------------------------------

func TestSendMessage_TrackingOptions(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	rec := sendMessage(t, router, "example.com", map[string]string{
		"from":              "sender@example.com",
		"to":                "recipient@example.com",
		"subject":           "Tracking Test",
		"text":              "Hello with tracking options",
		"o:tracking":        "yes",
		"o:tracking-clicks": "htmlonly",
		"o:tracking-opens":  "no",
	})

	t.Run("returns 200", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp sendResponse
	decodeJSON(t, rec, &resp)

	t.Run("returns message ID", func(t *testing.T) {
		if resp.ID == "" {
			t.Error("expected non-empty message ID")
		}
	})

	// Retrieve and verify tracking options are stored
	t.Run("stored message contains tracking options", func(t *testing.T) {
		storageKey := extractStorageKey(t, resp)
		getRec := getMessage(t, router, "example.com", storageKey)
		if getRec.Code != http.StatusOK {
			t.Fatalf("expected 200 on GET, got %d (body: %s)", getRec.Code, getRec.Body.String())
		}
		body := getRec.Body.String()
		// The stored message should contain tracking-related information
		if !strings.Contains(body, "tracking") && !strings.Contains(body, "Tracking") {
			t.Errorf("expected stored message to contain tracking information, body: %s", body)
		}
	})
}

// ---------------------------------------------------------------------------
// Scenario 17: Delivery time -- POST with o:deliverytime -> stored
// ---------------------------------------------------------------------------

func TestSendMessage_DeliveryTime(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	rec := sendMessage(t, router, "example.com", map[string]string{
		"from":           "sender@example.com",
		"to":             "recipient@example.com",
		"subject":        "Scheduled Message",
		"text":           "This is a scheduled message",
		"o:deliverytime": "Thu, 13 Oct 2026 18:02:00 +0000",
	})

	t.Run("returns 200", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp sendResponse
	decodeJSON(t, rec, &resp)

	t.Run("returns message ID", func(t *testing.T) {
		if resp.ID == "" {
			t.Error("expected non-empty message ID")
		}
	})

	t.Run("returns queued message", func(t *testing.T) {
		if resp.Message != "Queued. Thank you." {
			t.Errorf("expected %q, got %q", "Queued. Thank you.", resp.Message)
		}
	})
}

// ---------------------------------------------------------------------------
// Scenario 18: Retrieve non-existent message -> 404
// ---------------------------------------------------------------------------

func TestGetMessage_NotFound(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	getRec := getMessage(t, router, "example.com", "nonexistent-storage-key")

	t.Run("returns 404", func(t *testing.T) {
		if getRec.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d (body: %s)", getRec.Code, getRec.Body.String())
		}
	})
}

// ---------------------------------------------------------------------------
// Scenario 19: Delete non-existent message -> 404
// ---------------------------------------------------------------------------

func TestDeleteMessage_NotFound(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	delRec := deleteMessage(t, router, "example.com", "nonexistent-storage-key")

	t.Run("returns 404", func(t *testing.T) {
		if delRec.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d (body: %s)", delRec.Code, delRec.Body.String())
		}
	})
}

// ---------------------------------------------------------------------------
// Scenario 20: Messages are domain-scoped -- Send on domain A, cannot retrieve from domain B
// ---------------------------------------------------------------------------

func TestMessage_DomainScoped(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createTestDomain(t, router, "alpha.com")
	createTestDomain(t, router, "beta.com")

	// Send a message on alpha.com
	sendRec := sendMessage(t, router, "alpha.com", map[string]string{
		"from":    "sender@alpha.com",
		"to":      "recipient@example.com",
		"subject": "Domain Scoped Test",
		"text":    "This message belongs to alpha.com",
	})

	if sendRec.Code != http.StatusOK {
		t.Fatalf("failed to send message on alpha.com: status=%d body=%s", sendRec.Code, sendRec.Body.String())
	}

	var sendResp sendResponse
	decodeJSON(t, sendRec, &sendResp)

	storageKey := extractStorageKey(t, sendResp)

	t.Run("can retrieve from alpha.com", func(t *testing.T) {
		getRec := getMessage(t, router, "alpha.com", storageKey)
		if getRec.Code != http.StatusOK {
			t.Errorf("expected 200 when retrieving from alpha.com, got %d (body: %s)", getRec.Code, getRec.Body.String())
		}
	})

	t.Run("cannot retrieve from beta.com", func(t *testing.T) {
		getRec := getMessage(t, router, "beta.com", storageKey)
		if getRec.Code != http.StatusNotFound {
			t.Errorf("expected 404 when retrieving from beta.com, got %d (body: %s)", getRec.Code, getRec.Body.String())
		}
	})
}

// ---------------------------------------------------------------------------
// Full Lifecycle Test -- Send, Retrieve, Delete, Verify Deleted
// ---------------------------------------------------------------------------

func TestMessageLifecycle(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	// Step 1: Send a message
	var storageKey string
	t.Run("send message", func(t *testing.T) {
		rec := sendMessage(t, router, "example.com", map[string]string{
			"from":    "sender@example.com",
			"to":      "recipient@example.com",
			"subject": "Lifecycle Test",
			"text":    "Plain text for lifecycle",
			"html":    "<p>HTML for lifecycle</p>",
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("send failed: status=%d body=%s", rec.Code, rec.Body.String())
		}
		var resp sendResponse
		decodeJSON(t, rec, &resp)
		if resp.ID == "" {
			t.Fatal("expected non-empty message ID")
		}
		if resp.Message != "Queued. Thank you." {
			t.Errorf("expected %q, got %q", "Queued. Thank you.", resp.Message)
		}
		storageKey = extractStorageKey(t, resp)
	})

	// Step 2: Retrieve the message and verify
	t.Run("retrieve message", func(t *testing.T) {
		if storageKey == "" {
			t.Skip("no storage key from send step")
		}
		rec := getMessage(t, router, "example.com", storageKey)
		if rec.Code != http.StatusOK {
			t.Fatalf("retrieve failed: status=%d body=%s", rec.Code, rec.Body.String())
		}
		var detail messageDetailResponse
		decodeJSON(t, rec, &detail)
		if detail.From != "sender@example.com" {
			t.Errorf("expected From=%q, got %q", "sender@example.com", detail.From)
		}
		if detail.Subject != "Lifecycle Test" {
			t.Errorf("expected Subject=%q, got %q", "Lifecycle Test", detail.Subject)
		}
		if detail.BodyPlain != "Plain text for lifecycle" {
			t.Errorf("expected body-plain=%q, got %q", "Plain text for lifecycle", detail.BodyPlain)
		}
		if detail.BodyHTML != "<p>HTML for lifecycle</p>" {
			t.Errorf("expected body-html=%q, got %q", "<p>HTML for lifecycle</p>", detail.BodyHTML)
		}
	})

	// Step 3: Delete the message
	t.Run("delete message", func(t *testing.T) {
		if storageKey == "" {
			t.Skip("no storage key from send step")
		}
		rec := deleteMessage(t, router, "example.com", storageKey)
		if rec.Code != http.StatusOK {
			t.Fatalf("delete failed: status=%d body=%s", rec.Code, rec.Body.String())
		}
		var resp deleteMessageResponse
		decodeJSON(t, rec, &resp)
		if resp.Message != "Message has been deleted" {
			t.Errorf("expected %q, got %q", "Message has been deleted", resp.Message)
		}
	})

	// Step 4: Verify the message is gone
	t.Run("retrieve after delete returns 404", func(t *testing.T) {
		if storageKey == "" {
			t.Skip("no storage key from send step")
		}
		rec := getMessage(t, router, "example.com", storageKey)
		if rec.Code != http.StatusNotFound {
			t.Errorf("expected 404 after delete, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})
}

// ---------------------------------------------------------------------------
// AMP HTML body -- POST with amp-html body -> accepted
// ---------------------------------------------------------------------------

func TestSendMessage_AMPHTMLBody(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	rec := sendMessage(t, router, "example.com", map[string]string{
		"from":     "sender@example.com",
		"to":       "recipient@example.com",
		"subject":  "AMP Test",
		"amp-html": "<!doctype html><html amp4email>...</html>",
	})

	t.Run("returns 200", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp sendResponse
	decodeJSON(t, rec, &resp)

	t.Run("returns message ID", func(t *testing.T) {
		if resp.ID == "" {
			t.Error("expected non-empty message ID")
		}
	})
}

// ---------------------------------------------------------------------------
// Template as body param -- POST with template -> accepted
// ---------------------------------------------------------------------------

func TestSendMessage_TemplateAsBody(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	rec := sendMessage(t, router, "example.com", map[string]string{
		"from":     "sender@example.com",
		"to":       "recipient@example.com",
		"subject":  "Template Test",
		"template": "welcome-email",
	})

	t.Run("returns 200", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp sendResponse
	decodeJSON(t, rec, &resp)

	t.Run("returns message ID", func(t *testing.T) {
		if resp.ID == "" {
			t.Error("expected non-empty message ID")
		}
	})
}
