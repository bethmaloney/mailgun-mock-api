package message_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bethmaloney/mailgun-mock-api/internal/domain"
	"github.com/bethmaloney/mailgun-mock-api/internal/message"
	"github.com/bethmaloney/mailgun-mock-api/internal/mock"
	"github.com/go-chi/chi/v5"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// Attachment Test Helpers
// ---------------------------------------------------------------------------

// setupAttachmentTestDB creates an in-memory SQLite database with the
// Attachment model migrated in addition to the standard tables.
func setupAttachmentTestDB(t *testing.T) *gorm.DB {
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
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

// setupAttachmentRouter creates a chi router with domain, message, and
// attachment routes registered.
func setupAttachmentRouter(db *gorm.DB, cfg *mock.MockConfig) http.Handler {
	domain.ResetForTests(db)
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
		r.Post("/{storage_key}", mh.ResendMessage)
		r.Get("/{storage_key}/attachments/{attachment_id}", mh.GetAttachment)
	})
	return r
}

// attachmentInfo represents a single attachment entry in a message detail response.
type attachmentInfo struct {
	Size        int    `json:"size"`
	URL         string `json:"url"`
	FileName    string `json:"filename"`
	ContentType string `json:"content-type"`
}

// messageDetailWithAttachments extends the message detail response to include
// the attachments array.
type messageDetailWithAttachments struct {
	From        string           `json:"From"`
	To          string           `json:"To"`
	Subject     string           `json:"Subject"`
	BodyHTML    string           `json:"body-html"`
	BodyPlain   string           `json:"body-plain"`
	Attachments []attachmentInfo `json:"attachments"`
}

// newMultipartRequestWithFiles creates an HTTP request with multipart/form-data
// containing multiple file uploads and optional form fields. Each file is
// described by a fileUpload struct.
type fileUpload struct {
	FieldName string // e.g., "attachment" or "inline"
	FileName  string
	Content   []byte
}

func newMultipartRequestWithFiles(t *testing.T, method, url string, files []fileUpload, fields map[string]string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Write file parts
	for _, f := range files {
		filePart, err := writer.CreateFormFile(f.FieldName, f.FileName)
		if err != nil {
			t.Fatalf("failed to create form file %q (%q): %v", f.FieldName, f.FileName, err)
		}
		if _, err := io.Copy(filePart, bytes.NewReader(f.Content)); err != nil {
			t.Fatalf("failed to write file content for %q: %v", f.FileName, err)
		}
	}

	// Write extra form fields
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

// sendMessageWithAttachments sends a POST /v3/{domain}/messages with file
// uploads and form fields, returning the response recorder.
func sendMessageWithAttachments(t *testing.T, router http.Handler, domainName string, files []fileUpload, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	url := fmt.Sprintf("/v3/%s/messages", domainName)
	req := newMultipartRequestWithFiles(t, http.MethodPost, url, files, fields)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// getAttachment retrieves an attachment via GET /v3/domains/{domain}/messages/{storage_key}/attachments/{attachment_id}.
func getAttachment(t *testing.T, router http.Handler, domainName, storageKey, attachmentID string) *httptest.ResponseRecorder {
	t.Helper()
	url := fmt.Sprintf("/v3/domains/%s/messages/%s/attachments/%s", domainName, storageKey, attachmentID)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// ---------------------------------------------------------------------------
// Test 1: Send message with single attachment
// ---------------------------------------------------------------------------

func TestAttachment_SendWithSingleAttachment(t *testing.T) {
	db := setupAttachmentTestDB(t)
	cfg := defaultConfig()
	router := setupAttachmentRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	pdfContent := []byte("%PDF-1.4 fake pdf content here")

	rec := sendMessageWithAttachments(t, router, "example.com",
		[]fileUpload{
			{FieldName: "attachment", FileName: "document.pdf", Content: pdfContent},
		},
		map[string]string{
			"from":    "sender@example.com",
			"to":      "recipient@example.com",
			"subject": "Single Attachment Test",
			"text":    "Please see attached document.",
		},
	)

	t.Run("send returns 200", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp sendResponse
	decodeJSON(t, rec, &resp)
	storageKey := extractStorageKey(t, resp)

	t.Run("message detail contains one attachment", func(t *testing.T) {
		getRec := getMessage(t, router, "example.com", storageKey)
		if getRec.Code != http.StatusOK {
			t.Fatalf("expected 200 on GET, got %d (body: %s)", getRec.Code, getRec.Body.String())
		}

		var detail messageDetailWithAttachments
		decodeJSON(t, getRec, &detail)

		if len(detail.Attachments) != 1 {
			t.Fatalf("expected 1 attachment, got %d", len(detail.Attachments))
		}

		att := detail.Attachments[0]

		if att.FileName != "document.pdf" {
			t.Errorf("expected filename %q, got %q", "document.pdf", att.FileName)
		}
		if att.ContentType != "application/pdf" {
			t.Errorf("expected content-type %q, got %q", "application/pdf", att.ContentType)
		}
		if att.Size != len(pdfContent) {
			t.Errorf("expected size %d, got %d", len(pdfContent), att.Size)
		}
		if att.URL == "" {
			t.Error("expected non-empty attachment URL")
		}

		expectedURLSuffix := fmt.Sprintf("/v3/domains/example.com/messages/%s/attachments/", storageKey)
		if len(att.URL) == 0 || !contains(att.URL, expectedURLSuffix) {
			t.Errorf("expected URL to contain %q, got %q", expectedURLSuffix, att.URL)
		}
	})
}

// ---------------------------------------------------------------------------
// Test 2: Send message with multiple attachments
// ---------------------------------------------------------------------------

func TestAttachment_SendWithMultipleAttachments(t *testing.T) {
	db := setupAttachmentTestDB(t)
	cfg := defaultConfig()
	router := setupAttachmentRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	pdfContent := []byte("%PDF-1.4 fake pdf content here")
	txtContent := []byte("This is a plain text file for testing.")

	rec := sendMessageWithAttachments(t, router, "example.com",
		[]fileUpload{
			{FieldName: "attachment", FileName: "document.pdf", Content: pdfContent},
			{FieldName: "attachment", FileName: "notes.txt", Content: txtContent},
		},
		map[string]string{
			"from":    "sender@example.com",
			"to":      "recipient@example.com",
			"subject": "Multiple Attachments Test",
			"text":    "Please see attached documents.",
		},
	)

	t.Run("send returns 200", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp sendResponse
	decodeJSON(t, rec, &resp)
	storageKey := extractStorageKey(t, resp)

	t.Run("message detail contains two attachments", func(t *testing.T) {
		getRec := getMessage(t, router, "example.com", storageKey)
		if getRec.Code != http.StatusOK {
			t.Fatalf("expected 200 on GET, got %d (body: %s)", getRec.Code, getRec.Body.String())
		}

		var detail messageDetailWithAttachments
		decodeJSON(t, getRec, &detail)

		if len(detail.Attachments) != 2 {
			t.Fatalf("expected 2 attachments, got %d", len(detail.Attachments))
		}

		// Verify both filenames are present (order may vary)
		filenames := map[string]bool{}
		for _, att := range detail.Attachments {
			filenames[att.FileName] = true
		}

		if !filenames["document.pdf"] {
			t.Error("expected attachment with filename 'document.pdf'")
		}
		if !filenames["notes.txt"] {
			t.Error("expected attachment with filename 'notes.txt'")
		}

		// Verify sizes
		for _, att := range detail.Attachments {
			switch att.FileName {
			case "document.pdf":
				if att.Size != len(pdfContent) {
					t.Errorf("expected document.pdf size %d, got %d", len(pdfContent), att.Size)
				}
			case "notes.txt":
				if att.Size != len(txtContent) {
					t.Errorf("expected notes.txt size %d, got %d", len(txtContent), att.Size)
				}
			}
		}

		// Verify each attachment has a unique URL
		urls := map[string]bool{}
		for _, att := range detail.Attachments {
			if att.URL == "" {
				t.Error("expected non-empty URL for attachment")
			}
			if urls[att.URL] {
				t.Errorf("duplicate attachment URL: %s", att.URL)
			}
			urls[att.URL] = true
		}
	})
}

// ---------------------------------------------------------------------------
// Test 3: Send message with inline attachment
// ---------------------------------------------------------------------------

func TestAttachment_SendWithInlineAttachment(t *testing.T) {
	db := setupAttachmentTestDB(t)
	cfg := defaultConfig()
	router := setupAttachmentRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	imageContent := []byte("\x89PNG\r\n\x1a\n fake png data")

	rec := sendMessageWithAttachments(t, router, "example.com",
		[]fileUpload{
			{FieldName: "inline", FileName: "logo.png", Content: imageContent},
		},
		map[string]string{
			"from":    "sender@example.com",
			"to":      "recipient@example.com",
			"subject": "Inline Attachment Test",
			"html":    "<html><body><img src=\"cid:logo.png\"></body></html>",
		},
	)

	t.Run("send returns 200", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp sendResponse
	decodeJSON(t, rec, &resp)
	storageKey := extractStorageKey(t, resp)

	t.Run("message detail contains inline attachment", func(t *testing.T) {
		getRec := getMessage(t, router, "example.com", storageKey)
		if getRec.Code != http.StatusOK {
			t.Fatalf("expected 200 on GET, got %d (body: %s)", getRec.Code, getRec.Body.String())
		}

		var detail messageDetailWithAttachments
		decodeJSON(t, getRec, &detail)

		if len(detail.Attachments) != 1 {
			t.Fatalf("expected 1 attachment, got %d", len(detail.Attachments))
		}

		att := detail.Attachments[0]

		if att.FileName != "logo.png" {
			t.Errorf("expected filename %q, got %q", "logo.png", att.FileName)
		}
		if att.Size != len(imageContent) {
			t.Errorf("expected size %d, got %d", len(imageContent), att.Size)
		}
		if att.URL == "" {
			t.Error("expected non-empty attachment URL")
		}
	})
}

// ---------------------------------------------------------------------------
// Test 4: Retrieve attachment content (raw bytes)
// ---------------------------------------------------------------------------

func TestAttachment_RetrieveContent(t *testing.T) {
	db := setupAttachmentTestDB(t)
	cfg := defaultConfig()
	router := setupAttachmentRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	pdfContent := []byte("%PDF-1.4 this is test PDF content for retrieval")

	rec := sendMessageWithAttachments(t, router, "example.com",
		[]fileUpload{
			{FieldName: "attachment", FileName: "report.pdf", Content: pdfContent},
		},
		map[string]string{
			"from":    "sender@example.com",
			"to":      "recipient@example.com",
			"subject": "Retrieve Attachment Test",
			"text":    "See attached report.",
		},
	)

	if rec.Code != http.StatusOK {
		t.Fatalf("failed to send message: status=%d body=%s", rec.Code, rec.Body.String())
	}

	var resp sendResponse
	decodeJSON(t, rec, &resp)
	storageKey := extractStorageKey(t, resp)

	// Get the message detail to find the attachment URL
	getRec := getMessage(t, router, "example.com", storageKey)
	if getRec.Code != http.StatusOK {
		t.Fatalf("failed to get message: status=%d body=%s", getRec.Code, getRec.Body.String())
	}

	var detail messageDetailWithAttachments
	decodeJSON(t, getRec, &detail)

	if len(detail.Attachments) != 1 {
		t.Fatalf("expected 1 attachment in detail, got %d", len(detail.Attachments))
	}

	// Extract the attachment ID from the URL. The URL format is:
	// /v3/domains/{domain}/messages/{storage_key}/attachments/{attachment_id}
	attURL := detail.Attachments[0].URL
	// Use the URL directly or parse the attachment_id from it
	// We use "0" as the first attachment index-based ID
	attRec := getAttachment(t, router, "example.com", storageKey, extractAttachmentID(t, attURL))

	t.Run("returns 200", func(t *testing.T) {
		if attRec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", attRec.Code, attRec.Body.String())
		}
	})

	t.Run("returns correct Content-Type header", func(t *testing.T) {
		ct := attRec.Header().Get("Content-Type")
		if ct != "application/pdf" {
			t.Errorf("expected Content-Type %q, got %q", "application/pdf", ct)
		}
	})

	t.Run("returns the raw file bytes", func(t *testing.T) {
		body := attRec.Body.Bytes()
		if !bytes.Equal(body, pdfContent) {
			t.Errorf("expected attachment body to match original content (len %d), got len %d", len(pdfContent), len(body))
		}
	})
}

// ---------------------------------------------------------------------------
// Test 5: Attachment not found -> 404
// ---------------------------------------------------------------------------

func TestAttachment_NotFound(t *testing.T) {
	db := setupAttachmentTestDB(t)
	cfg := defaultConfig()
	router := setupAttachmentRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	// Send a message (without attachments) so the storage key is valid
	rec := sendMessage(t, router, "example.com", map[string]string{
		"from":    "sender@example.com",
		"to":      "recipient@example.com",
		"subject": "No Attachments",
		"text":    "Plain message.",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("failed to send message: status=%d body=%s", rec.Code, rec.Body.String())
	}

	var resp sendResponse
	decodeJSON(t, rec, &resp)
	storageKey := extractStorageKey(t, resp)

	t.Run("invalid attachment ID returns 404", func(t *testing.T) {
		attRec := getAttachment(t, router, "example.com", storageKey, "nonexistent-id")
		if attRec.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d (body: %s)", attRec.Code, attRec.Body.String())
		}
	})

	t.Run("invalid storage key returns 404", func(t *testing.T) {
		attRec := getAttachment(t, router, "example.com", "bogus-storage-key", "0")
		if attRec.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d (body: %s)", attRec.Code, attRec.Body.String())
		}
	})
}

// ---------------------------------------------------------------------------
// Test 6: Message without attachments -> empty attachments array
// ---------------------------------------------------------------------------

func TestAttachment_MessageWithoutAttachments(t *testing.T) {
	db := setupAttachmentTestDB(t)
	cfg := defaultConfig()
	router := setupAttachmentRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	rec := sendMessage(t, router, "example.com", map[string]string{
		"from":    "sender@example.com",
		"to":      "recipient@example.com",
		"subject": "Plain Message",
		"text":    "No attachments here.",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("failed to send message: status=%d body=%s", rec.Code, rec.Body.String())
	}

	var resp sendResponse
	decodeJSON(t, rec, &resp)
	storageKey := extractStorageKey(t, resp)

	t.Run("attachments array is empty", func(t *testing.T) {
		getRec := getMessage(t, router, "example.com", storageKey)
		if getRec.Code != http.StatusOK {
			t.Fatalf("expected 200 on GET, got %d (body: %s)", getRec.Code, getRec.Body.String())
		}

		// Parse the raw JSON to check the attachments field
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(getRec.Body.Bytes(), &raw); err != nil {
			t.Fatalf("failed to parse response JSON: %v", err)
		}

		attJSON, ok := raw["attachments"]
		if !ok {
			t.Fatal("expected 'attachments' key in response")
		}

		var attachments []attachmentInfo
		if err := json.Unmarshal(attJSON, &attachments); err != nil {
			t.Fatalf("failed to parse attachments: %v", err)
		}

		if len(attachments) != 0 {
			t.Errorf("expected 0 attachments, got %d", len(attachments))
		}
	})
}

// ---------------------------------------------------------------------------
// Test 7: Resend preserves attachments
// ---------------------------------------------------------------------------

func TestAttachment_ResendPreservesAttachments(t *testing.T) {
	db := setupAttachmentTestDB(t)
	cfg := defaultConfig()
	router := setupAttachmentRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	csvContent := []byte("name,email\nAlice,alice@example.com\nBob,bob@example.com")

	// Send original message with attachment
	sendRec := sendMessageWithAttachments(t, router, "example.com",
		[]fileUpload{
			{FieldName: "attachment", FileName: "contacts.csv", Content: csvContent},
		},
		map[string]string{
			"from":    "sender@example.com",
			"to":      "original@example.com",
			"subject": "Resend Attachment Test",
			"text":    "See the attached contacts list.",
		},
	)

	if sendRec.Code != http.StatusOK {
		t.Fatalf("failed to send original message: status=%d body=%s", sendRec.Code, sendRec.Body.String())
	}

	var origResp sendResponse
	decodeJSON(t, sendRec, &origResp)
	origStorageKey := extractStorageKey(t, origResp)

	// Resend the message to a new recipient
	resendRec := resendMessage(t, router, "example.com", origStorageKey, map[string]string{
		"to": "new-recipient@example.com",
	})

	t.Run("resend returns 200", func(t *testing.T) {
		if resendRec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", resendRec.Code, resendRec.Body.String())
		}
	})

	var resendResp sendResponse
	decodeJSON(t, resendRec, &resendResp)
	resendStorageKey := extractStorageKey(t, resendResp)

	t.Run("resent message has attachments", func(t *testing.T) {
		getRec := getMessage(t, router, "example.com", resendStorageKey)
		if getRec.Code != http.StatusOK {
			t.Fatalf("expected 200 on GET, got %d (body: %s)", getRec.Code, getRec.Body.String())
		}

		var detail messageDetailWithAttachments
		decodeJSON(t, getRec, &detail)

		if len(detail.Attachments) != 1 {
			t.Fatalf("expected 1 attachment in resent message, got %d", len(detail.Attachments))
		}

		att := detail.Attachments[0]
		if att.FileName != "contacts.csv" {
			t.Errorf("expected filename %q, got %q", "contacts.csv", att.FileName)
		}
		if att.Size != len(csvContent) {
			t.Errorf("expected size %d, got %d", len(csvContent), att.Size)
		}
	})

	t.Run("resent attachment content is retrievable", func(t *testing.T) {
		// Get resent message detail to find attachment URL
		getRec := getMessage(t, router, "example.com", resendStorageKey)
		if getRec.Code != http.StatusOK {
			t.Fatalf("expected 200 on GET, got %d", getRec.Code)
		}

		var detail messageDetailWithAttachments
		decodeJSON(t, getRec, &detail)

		if len(detail.Attachments) < 1 {
			t.Fatal("no attachments found in resent message")
		}

		attID := extractAttachmentID(t, detail.Attachments[0].URL)
		attRec := getAttachment(t, router, "example.com", resendStorageKey, attID)

		if attRec.Code != http.StatusOK {
			t.Fatalf("expected 200 on GET attachment, got %d (body: %s)", attRec.Code, attRec.Body.String())
		}

		if !bytes.Equal(attRec.Body.Bytes(), csvContent) {
			t.Errorf("expected resent attachment content to match original (len %d), got len %d", len(csvContent), len(attRec.Body.Bytes()))
		}
	})
}

// ---------------------------------------------------------------------------
// Test 8: Delete message removes attachments
// ---------------------------------------------------------------------------

func TestAttachment_DeleteMessageRemovesAttachments(t *testing.T) {
	db := setupAttachmentTestDB(t)
	cfg := defaultConfig()
	router := setupAttachmentRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	docContent := []byte("Important document content for deletion test")

	// Send message with attachment
	sendRec := sendMessageWithAttachments(t, router, "example.com",
		[]fileUpload{
			{FieldName: "attachment", FileName: "important.txt", Content: docContent},
		},
		map[string]string{
			"from":    "sender@example.com",
			"to":      "recipient@example.com",
			"subject": "Delete Attachment Test",
			"text":    "This message will be deleted.",
		},
	)

	if sendRec.Code != http.StatusOK {
		t.Fatalf("failed to send message: status=%d body=%s", sendRec.Code, sendRec.Body.String())
	}

	var resp sendResponse
	decodeJSON(t, sendRec, &resp)
	storageKey := extractStorageKey(t, resp)

	// Get the attachment ID before deletion
	getRec := getMessage(t, router, "example.com", storageKey)
	if getRec.Code != http.StatusOK {
		t.Fatalf("failed to get message: status=%d body=%s", getRec.Code, getRec.Body.String())
	}

	var detail messageDetailWithAttachments
	decodeJSON(t, getRec, &detail)

	if len(detail.Attachments) < 1 {
		t.Fatal("expected at least 1 attachment before deletion")
	}

	attID := extractAttachmentID(t, detail.Attachments[0].URL)

	// Verify attachment is accessible before deletion
	attRec := getAttachment(t, router, "example.com", storageKey, attID)
	if attRec.Code != http.StatusOK {
		t.Fatalf("expected attachment to be accessible before deletion, got %d", attRec.Code)
	}

	// Delete the message
	delRec := deleteMessage(t, router, "example.com", storageKey)

	t.Run("delete returns 200", func(t *testing.T) {
		if delRec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", delRec.Code, delRec.Body.String())
		}
	})

	t.Run("attachment returns 404 after message deletion", func(t *testing.T) {
		attRecAfter := getAttachment(t, router, "example.com", storageKey, attID)
		if attRecAfter.Code != http.StatusNotFound {
			t.Errorf("expected 404 for attachment after message deletion, got %d (body: %s)", attRecAfter.Code, attRecAfter.Body.String())
		}
	})

	t.Run("message returns 404 after deletion", func(t *testing.T) {
		msgRec := getMessage(t, router, "example.com", storageKey)
		if msgRec.Code != http.StatusNotFound {
			t.Errorf("expected 404 for message after deletion, got %d", msgRec.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// Utility helpers
// ---------------------------------------------------------------------------

// contains checks if s contains substr (used to avoid importing strings
// just for this, since the test package already imports what it needs).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// extractAttachmentID extracts the attachment ID from a full attachment URL.
// Expected URL format: .../attachments/{attachment_id}
func extractAttachmentID(t *testing.T, url string) string {
	t.Helper()
	// Find the last path segment after "/attachments/"
	const prefix = "/attachments/"
	idx := -1
	for i := len(url) - 1; i >= 0; i-- {
		if i+len(prefix) <= len(url) && url[i:i+len(prefix)] == prefix {
			idx = i + len(prefix)
			break
		}
	}
	if idx == -1 || idx >= len(url) {
		t.Fatalf("could not extract attachment ID from URL %q", url)
	}
	return url[idx:]
}
