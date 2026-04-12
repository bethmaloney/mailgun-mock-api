package mock_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
// Response types (mirroring expected JSON shapes)
// ---------------------------------------------------------------------------

type messagesListResponse struct {
	Items      []messageListItem `json:"items"`
	Paging     pagingResponse    `json:"paging"`
	TotalCount int               `json:"total_count"`
}

type messageListItem struct {
	ID             string   `json:"id"`
	StorageKey     string   `json:"storage_key"`
	Domain         string   `json:"domain"`
	From           string   `json:"from"`
	To             []string `json:"to"`
	Subject        string   `json:"subject"`
	Tags           []string `json:"tags"`
	Timestamp      float64  `json:"timestamp"`
	Status         string   `json:"status"`
	HasAttachments bool     `json:"has_attachments"`
}

type pagingResponse struct {
	Next     string `json:"next"`
	Previous string `json:"previous"`
}

type messageDetailResponse struct {
	ID              string                 `json:"id"`
	MessageID       string                 `json:"message_id"`
	StorageKey      string                 `json:"storage_key"`
	Domain          string                 `json:"domain"`
	From            string                 `json:"from"`
	To              []string               `json:"to"`
	Subject         string                 `json:"subject"`
	TextBody        string                 `json:"text_body"`
	HTMLBody        string                 `json:"html_body"`
	Tags            []string               `json:"tags"`
	CustomHeaders   map[string]interface{} `json:"custom_headers"`
	CustomVariables map[string]interface{} `json:"custom_variables"`
	Options         map[string]interface{} `json:"options"`
	Timestamp       float64                `json:"timestamp"`
	Attachments     []attachmentItem       `json:"attachments"`
	Events          []eventItem            `json:"events"`
}

type attachmentItem struct {
	Filename    string `json:"filename"`
	Size        int    `json:"size"`
	ContentType string `json:"content_type"`
}

type eventItem struct {
	ID        string  `json:"id"`
	EventType string  `json:"event_type"`
	Timestamp float64 `json:"timestamp"`
}

type successResponse struct {
	Message string `json:"message"`
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func setupMessagesDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}

	err = db.AutoMigrate(
		&message.StoredMessage{},
		&message.Attachment{},
		&event.Event{},
		&domain.Domain{},
		&domain.DNSRecord{},
		&webhook.DomainWebhook{},
		&webhook.AccountWebhook{},
		&webhook.WebhookDelivery{},
	)
	if err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	return db
}

func setupMessagesRouter(h *mock.Handlers) http.Handler {
	r := chi.NewRouter()
	r.Route("/mock", func(r chi.Router) {
		r.Get("/messages", h.ListMessages)
		r.Get("/messages/{message_id}", h.GetMessageDetail)
		r.Delete("/messages/{message_id}", h.DeleteSingleMessage)
		r.Post("/messages/clear", h.ClearAllMessages)
	})
	return r
}

func doRequest(t *testing.T, router http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func decodeMessagesList(t *testing.T, rec *httptest.ResponseRecorder) messagesListResponse {
	t.Helper()
	var resp messagesListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode messages list response: %v\nbody: %s", err, rec.Body.String())
	}
	return resp
}

func decodeMessageDetail(t *testing.T, rec *httptest.ResponseRecorder) messageDetailResponse {
	t.Helper()
	var resp messageDetailResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode message detail response: %v\nbody: %s", err, rec.Body.String())
	}
	return resp
}

func decodeSuccess(t *testing.T, rec *httptest.ResponseRecorder) successResponse {
	t.Helper()
	var resp successResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode success response: %v\nbody: %s", err, rec.Body.String())
	}
	return resp
}

// createTestMessage creates a StoredMessage in the database and returns it.
func createTestMessage(t *testing.T, db *gorm.DB, domainName, storageKey, from, to, subject, tags string) message.StoredMessage {
	t.Helper()
	messageID := fmt.Sprintf("<%s>", storageKey)
	msg := message.StoredMessage{
		DomainName: domainName,
		MessageID:  messageID,
		StorageKey: storageKey,
		From:       from,
		To:         to,
		Subject:    subject,
		Tags:       tags,
	}
	if err := db.Create(&msg).Error; err != nil {
		t.Fatalf("failed to create test message: %v", err)
	}
	return msg
}

// createTestEvent creates an Event in the database.
func createTestEvent(t *testing.T, db *gorm.DB, domainName, messageID, storageKey, eventType, recipient string, timestamp float64) event.Event {
	t.Helper()
	ev := event.Event{
		DomainName: domainName,
		EventType:  eventType,
		Timestamp:  timestamp,
		LogLevel:   "info",
		MessageID:  messageID,
		StorageKey: storageKey,
		Recipient:  recipient,
	}
	if err := db.Create(&ev).Error; err != nil {
		t.Fatalf("failed to create test event: %v", err)
	}
	return ev
}

// createTestAttachment creates an Attachment record in the database.
func createTestAttachment(t *testing.T, db *gorm.DB, storedMessageID, filename, contentType string, size int) message.Attachment {
	t.Helper()
	att := message.Attachment{
		StoredMessageID: storedMessageID,
		FileName:        filename,
		ContentType:     contentType,
		Size:            size,
		Content:         make([]byte, size),
	}
	if err := db.Create(&att).Error; err != nil {
		t.Fatalf("failed to create test attachment: %v", err)
	}
	return att
}

// =========================================================================
// ListMessages tests
// =========================================================================

// ---------------------------------------------------------------------------
// Test 1: Empty database returns empty items array, total_count=0
// ---------------------------------------------------------------------------

func TestListMessages_EmptyDatabase(t *testing.T) {
	db := setupMessagesDB(t)
	h := mock.NewHandlers(db, nil)
	router := setupMessagesRouter(h)

	rec := doRequest(t, router, http.MethodGet, "/mock/messages")

	t.Run("returns 200 status", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}
	})

	resp := decodeMessagesList(t, rec)

	t.Run("items is empty array", func(t *testing.T) {
		if resp.Items == nil {
			t.Fatal("expected items to be an empty array, got nil")
		}
		if len(resp.Items) != 0 {
			t.Errorf("expected 0 items, got %d", len(resp.Items))
		}
	})

	t.Run("total_count is zero", func(t *testing.T) {
		if resp.TotalCount != 0 {
			t.Errorf("expected total_count=0, got %d", resp.TotalCount)
		}
	})
}

// ---------------------------------------------------------------------------
// Test 2: Lists messages ordered by creation time (newest first)
// ---------------------------------------------------------------------------

func TestListMessages_OrderedByCreationTimeDesc(t *testing.T) {
	db := setupMessagesDB(t)
	h := mock.NewHandlers(db, nil)
	router := setupMessagesRouter(h)

	now := time.Now()

	// Create 3 messages with staggered creation times
	msg1 := createTestMessage(t, db, "example.com", "oldest@example.com", "a@example.com", "b@example.com", "Oldest", "[]")
	db.Model(&message.StoredMessage{}).Where("id = ?", msg1.ID).Update("created_at", now.Add(-2*time.Hour))

	msg2 := createTestMessage(t, db, "example.com", "middle@example.com", "a@example.com", "b@example.com", "Middle", "[]")
	db.Model(&message.StoredMessage{}).Where("id = ?", msg2.ID).Update("created_at", now.Add(-1*time.Hour))

	msg3 := createTestMessage(t, db, "example.com", "newest@example.com", "a@example.com", "b@example.com", "Newest", "[]")
	db.Model(&message.StoredMessage{}).Where("id = ?", msg3.ID).Update("created_at", now)

	rec := doRequest(t, router, http.MethodGet, "/mock/messages")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	resp := decodeMessagesList(t, rec)

	t.Run("returns all 3 messages", func(t *testing.T) {
		if len(resp.Items) != 3 {
			t.Fatalf("expected 3 items, got %d", len(resp.Items))
		}
	})

	t.Run("newest message is first", func(t *testing.T) {
		if resp.Items[0].StorageKey != "newest@example.com" {
			t.Errorf("expected first item storage_key=%q, got %q", "newest@example.com", resp.Items[0].StorageKey)
		}
	})

	t.Run("oldest message is last", func(t *testing.T) {
		if resp.Items[2].StorageKey != "oldest@example.com" {
			t.Errorf("expected last item storage_key=%q, got %q", "oldest@example.com", resp.Items[2].StorageKey)
		}
	})
}

// ---------------------------------------------------------------------------
// Test 3: Filters by domain parameter
// ---------------------------------------------------------------------------

func TestListMessages_FilterByDomain(t *testing.T) {
	db := setupMessagesDB(t)
	h := mock.NewHandlers(db, nil)
	router := setupMessagesRouter(h)

	createTestMessage(t, db, "example.com", "msg1@example.com", "a@example.com", "b@example.com", "Example msg", "[]")
	createTestMessage(t, db, "example.com", "msg2@example.com", "a@example.com", "b@example.com", "Example msg 2", "[]")
	createTestMessage(t, db, "other.com", "msg3@other.com", "a@other.com", "b@other.com", "Other msg", "[]")

	rec := doRequest(t, router, http.MethodGet, "/mock/messages?domain=example.com")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	resp := decodeMessagesList(t, rec)

	t.Run("returns only messages from the specified domain", func(t *testing.T) {
		if len(resp.Items) != 2 {
			t.Errorf("expected 2 items for domain=example.com, got %d", len(resp.Items))
		}
		for _, item := range resp.Items {
			if item.Domain != "example.com" {
				t.Errorf("expected domain=%q, got %q", "example.com", item.Domain)
			}
		}
	})

	t.Run("total_count reflects filtered count", func(t *testing.T) {
		if resp.TotalCount != 2 {
			t.Errorf("expected total_count=2, got %d", resp.TotalCount)
		}
	})
}

// ---------------------------------------------------------------------------
// Test 4: Filters by from parameter (substring match, case-insensitive)
// ---------------------------------------------------------------------------

func TestListMessages_FilterByFrom(t *testing.T) {
	db := setupMessagesDB(t)
	h := mock.NewHandlers(db, nil)
	router := setupMessagesRouter(h)

	createTestMessage(t, db, "example.com", "msg1@example.com", "Alice <alice@example.com>", "b@example.com", "From Alice", "[]")
	createTestMessage(t, db, "example.com", "msg2@example.com", "Bob <bob@example.com>", "b@example.com", "From Bob", "[]")
	createTestMessage(t, db, "example.com", "msg3@example.com", "ALICE <alice@other.com>", "b@example.com", "From ALICE uppercase", "[]")

	rec := doRequest(t, router, http.MethodGet, "/mock/messages?from=alice")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	resp := decodeMessagesList(t, rec)

	t.Run("returns messages matching from substring (case-insensitive)", func(t *testing.T) {
		if len(resp.Items) != 2 {
			t.Errorf("expected 2 items matching from=alice, got %d", len(resp.Items))
		}
	})
}

// ---------------------------------------------------------------------------
// Test 5: Filters by to parameter (substring match)
// ---------------------------------------------------------------------------

func TestListMessages_FilterByTo(t *testing.T) {
	db := setupMessagesDB(t)
	h := mock.NewHandlers(db, nil)
	router := setupMessagesRouter(h)

	createTestMessage(t, db, "example.com", "msg1@example.com", "a@example.com", "recipient@test.com", "To test.com", "[]")
	createTestMessage(t, db, "example.com", "msg2@example.com", "a@example.com", "user@other.com", "To other.com", "[]")
	createTestMessage(t, db, "example.com", "msg3@example.com", "a@example.com", "recipient@test.com, user@other.com", "To both", "[]")

	rec := doRequest(t, router, http.MethodGet, "/mock/messages?to=test.com")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	resp := decodeMessagesList(t, rec)

	t.Run("returns messages where to field contains substring", func(t *testing.T) {
		if len(resp.Items) != 2 {
			t.Errorf("expected 2 items matching to=test.com, got %d", len(resp.Items))
		}
	})
}

// ---------------------------------------------------------------------------
// Test 6: Filters by subject parameter (substring match)
// ---------------------------------------------------------------------------

func TestListMessages_FilterBySubject(t *testing.T) {
	db := setupMessagesDB(t)
	h := mock.NewHandlers(db, nil)
	router := setupMessagesRouter(h)

	createTestMessage(t, db, "example.com", "msg1@example.com", "a@example.com", "b@example.com", "Welcome to our platform", "[]")
	createTestMessage(t, db, "example.com", "msg2@example.com", "a@example.com", "b@example.com", "Password reset", "[]")
	createTestMessage(t, db, "example.com", "msg3@example.com", "a@example.com", "b@example.com", "Welcome back", "[]")

	rec := doRequest(t, router, http.MethodGet, "/mock/messages?subject=Welcome")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	resp := decodeMessagesList(t, rec)

	t.Run("returns messages matching subject substring", func(t *testing.T) {
		if len(resp.Items) != 2 {
			t.Errorf("expected 2 items matching subject=Welcome, got %d", len(resp.Items))
		}
	})
}

// ---------------------------------------------------------------------------
// Test 7: Filters by tag parameter
// ---------------------------------------------------------------------------

func TestListMessages_FilterByTag(t *testing.T) {
	db := setupMessagesDB(t)
	h := mock.NewHandlers(db, nil)
	router := setupMessagesRouter(h)

	createTestMessage(t, db, "example.com", "msg1@example.com", "a@example.com", "b@example.com", "Tagged welcome", `["welcome","onboarding"]`)
	createTestMessage(t, db, "example.com", "msg2@example.com", "a@example.com", "b@example.com", "Tagged promo", `["promo"]`)
	createTestMessage(t, db, "example.com", "msg3@example.com", "a@example.com", "b@example.com", "Tagged welcome only", `["welcome"]`)

	rec := doRequest(t, router, http.MethodGet, "/mock/messages?tag=welcome")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	resp := decodeMessagesList(t, rec)

	t.Run("returns messages with matching tag", func(t *testing.T) {
		if len(resp.Items) != 2 {
			t.Errorf("expected 2 items matching tag=welcome, got %d", len(resp.Items))
		}
	})
}

// ---------------------------------------------------------------------------
// Test 8: Filters by time range (start and end)
// ---------------------------------------------------------------------------

func TestListMessages_FilterByTimeRange(t *testing.T) {
	db := setupMessagesDB(t)
	h := mock.NewHandlers(db, nil)
	router := setupMessagesRouter(h)

	now := time.Now()

	msg1 := createTestMessage(t, db, "example.com", "old@example.com", "a@example.com", "b@example.com", "Old message", "[]")
	db.Model(&message.StoredMessage{}).Where("id = ?", msg1.ID).Update("created_at", now.Add(-3*time.Hour))

	msg2 := createTestMessage(t, db, "example.com", "recent@example.com", "a@example.com", "b@example.com", "Recent message", "[]")
	db.Model(&message.StoredMessage{}).Where("id = ?", msg2.ID).Update("created_at", now.Add(-30*time.Minute))

	msg3 := createTestMessage(t, db, "example.com", "newest@example.com", "a@example.com", "b@example.com", "Newest message", "[]")
	db.Model(&message.StoredMessage{}).Where("id = ?", msg3.ID).Update("created_at", now)

	// Query messages from 2 hours ago to now
	startTS := now.Add(-2 * time.Hour).Unix()
	endTS := now.Unix()
	url := fmt.Sprintf("/mock/messages?start=%d&end=%d", startTS, endTS)

	rec := doRequest(t, router, http.MethodGet, url)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	resp := decodeMessagesList(t, rec)

	t.Run("returns only messages in time range", func(t *testing.T) {
		if len(resp.Items) != 2 {
			t.Errorf("expected 2 items in time range, got %d", len(resp.Items))
		}
	})
}

// ---------------------------------------------------------------------------
// Test 9: Respects limit parameter (default 50, max 300)
// ---------------------------------------------------------------------------

func TestListMessages_LimitParameter(t *testing.T) {
	db := setupMessagesDB(t)
	h := mock.NewHandlers(db, nil)
	router := setupMessagesRouter(h)

	// Create 5 messages
	for i := 0; i < 5; i++ {
		createTestMessage(t, db, "example.com",
			fmt.Sprintf("msg%d@example.com", i),
			"a@example.com", "b@example.com",
			fmt.Sprintf("Message %d", i), "[]")
	}

	t.Run("respects explicit limit", func(t *testing.T) {
		rec := doRequest(t, router, http.MethodGet, "/mock/messages?limit=2")
		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}
		resp := decodeMessagesList(t, rec)
		if len(resp.Items) != 2 {
			t.Errorf("expected 2 items with limit=2, got %d", len(resp.Items))
		}
	})

	t.Run("default limit returns all when under 50", func(t *testing.T) {
		rec := doRequest(t, router, http.MethodGet, "/mock/messages")
		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}
		resp := decodeMessagesList(t, rec)
		if len(resp.Items) != 5 {
			t.Errorf("expected 5 items with default limit (under 50), got %d", len(resp.Items))
		}
	})

	t.Run("limit capped at 300", func(t *testing.T) {
		rec := doRequest(t, router, http.MethodGet, "/mock/messages?limit=999")
		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}
		resp := decodeMessagesList(t, rec)
		// With only 5 messages, should return all 5 (capped limit doesn't matter here)
		if len(resp.Items) != 5 {
			t.Errorf("expected 5 items (all available), got %d", len(resp.Items))
		}
		// total_count should still reflect the real count
		if resp.TotalCount != 5 {
			t.Errorf("expected total_count=5, got %d", resp.TotalCount)
		}
	})
}

// ---------------------------------------------------------------------------
// Test 10: Pagination with page cursor token
// ---------------------------------------------------------------------------

func TestListMessages_Pagination(t *testing.T) {
	db := setupMessagesDB(t)
	h := mock.NewHandlers(db, nil)
	router := setupMessagesRouter(h)

	// Create 5 messages
	for i := 0; i < 5; i++ {
		msg := createTestMessage(t, db, "example.com",
			fmt.Sprintf("page-msg%d@example.com", i),
			"a@example.com", "b@example.com",
			fmt.Sprintf("Page Message %d", i), "[]")
		// Stagger creation times
		db.Model(&message.StoredMessage{}).Where("id = ?", msg.ID).
			Update("created_at", time.Now().Add(time.Duration(i)*time.Minute))
	}

	// First page with limit=2
	rec := doRequest(t, router, http.MethodGet, "/mock/messages?limit=2")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	resp := decodeMessagesList(t, rec)

	t.Run("first page has 2 items", func(t *testing.T) {
		if len(resp.Items) != 2 {
			t.Errorf("expected 2 items on first page, got %d", len(resp.Items))
		}
	})

	t.Run("first page has next paging link", func(t *testing.T) {
		if resp.Paging.Next == "" {
			t.Error("expected paging.next to be non-empty on first page")
		}
	})

	t.Run("total_count reflects full count regardless of page", func(t *testing.T) {
		if resp.TotalCount != 5 {
			t.Errorf("expected total_count=5, got %d", resp.TotalCount)
		}
	})
}

// ---------------------------------------------------------------------------
// Test 11: Response includes correct status field from most recent event
// ---------------------------------------------------------------------------

func TestListMessages_StatusFromMostRecentEvent(t *testing.T) {
	db := setupMessagesDB(t)
	h := mock.NewHandlers(db, nil)
	router := setupMessagesRouter(h)

	now := float64(time.Now().UnixMicro()) / 1e6

	msg := createTestMessage(t, db, "example.com", "status-msg@example.com", "a@example.com", "b@example.com", "Status test", "[]")

	// Create events for this message: accepted then delivered
	createTestEvent(t, db, "example.com", msg.MessageID, msg.StorageKey, "accepted", "b@example.com", now-1)
	createTestEvent(t, db, "example.com", msg.MessageID, msg.StorageKey, "delivered", "b@example.com", now)

	rec := doRequest(t, router, http.MethodGet, "/mock/messages")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	resp := decodeMessagesList(t, rec)

	t.Run("status reflects most recent event type", func(t *testing.T) {
		if len(resp.Items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(resp.Items))
		}
		if resp.Items[0].Status != "delivered" {
			t.Errorf("expected status=%q, got %q", "delivered", resp.Items[0].Status)
		}
	})
}

// ---------------------------------------------------------------------------
// Test 12: Response includes has_attachments field
// ---------------------------------------------------------------------------

func TestListMessages_HasAttachmentsField(t *testing.T) {
	db := setupMessagesDB(t)
	h := mock.NewHandlers(db, nil)
	router := setupMessagesRouter(h)

	// Message with attachment
	msgWithAtt := createTestMessage(t, db, "example.com", "att-msg@example.com", "a@example.com", "b@example.com", "Has attachment", "[]")
	createTestAttachment(t, db, msgWithAtt.ID, "report.pdf", "application/pdf", 1024)

	// Message without attachment
	createTestMessage(t, db, "example.com", "no-att-msg@example.com", "a@example.com", "b@example.com", "No attachment", "[]")

	rec := doRequest(t, router, http.MethodGet, "/mock/messages")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	resp := decodeMessagesList(t, rec)

	t.Run("has_attachments is true for messages with attachments", func(t *testing.T) {
		found := false
		for _, item := range resp.Items {
			if item.StorageKey == "att-msg@example.com" {
				found = true
				if !item.HasAttachments {
					t.Error("expected has_attachments=true for message with attachment")
				}
			}
		}
		if !found {
			t.Error("did not find message with storage_key att-msg@example.com")
		}
	})

	t.Run("has_attachments is false for messages without attachments", func(t *testing.T) {
		found := false
		for _, item := range resp.Items {
			if item.StorageKey == "no-att-msg@example.com" {
				found = true
				if item.HasAttachments {
					t.Error("expected has_attachments=false for message without attachment")
				}
			}
		}
		if !found {
			t.Error("did not find message with storage_key no-att-msg@example.com")
		}
	})
}

// ---------------------------------------------------------------------------
// Test 13: Response to field is a JSON array (split from comma-separated string)
// ---------------------------------------------------------------------------

func TestListMessages_ToFieldIsArray(t *testing.T) {
	db := setupMessagesDB(t)
	h := mock.NewHandlers(db, nil)
	router := setupMessagesRouter(h)

	createTestMessage(t, db, "example.com", "multi-to@example.com", "a@example.com", "alice@test.com, bob@test.com", "Multiple recipients", "[]")

	rec := doRequest(t, router, http.MethodGet, "/mock/messages")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	resp := decodeMessagesList(t, rec)

	t.Run("to field is an array of recipients", func(t *testing.T) {
		if len(resp.Items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(resp.Items))
		}
		to := resp.Items[0].To
		if len(to) != 2 {
			t.Fatalf("expected 2 recipients in to array, got %d: %v", len(to), to)
		}
		if to[0] != "alice@test.com" {
			t.Errorf("expected to[0]=%q, got %q", "alice@test.com", to[0])
		}
		if to[1] != "bob@test.com" {
			t.Errorf("expected to[1]=%q, got %q", "bob@test.com", to[1])
		}
	})
}

// ---------------------------------------------------------------------------
// Test 14: Response tags field is a JSON array (parsed from JSON string)
// ---------------------------------------------------------------------------

func TestListMessages_TagsFieldIsArray(t *testing.T) {
	db := setupMessagesDB(t)
	h := mock.NewHandlers(db, nil)
	router := setupMessagesRouter(h)

	createTestMessage(t, db, "example.com", "tagged-msg@example.com", "a@example.com", "b@example.com", "Tagged", `["welcome","onboarding"]`)

	rec := doRequest(t, router, http.MethodGet, "/mock/messages")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	resp := decodeMessagesList(t, rec)

	t.Run("tags field is a parsed JSON array", func(t *testing.T) {
		if len(resp.Items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(resp.Items))
		}
		tags := resp.Items[0].Tags
		if len(tags) != 2 {
			t.Fatalf("expected 2 tags, got %d: %v", len(tags), tags)
		}
		if tags[0] != "welcome" {
			t.Errorf("expected tags[0]=%q, got %q", "welcome", tags[0])
		}
		if tags[1] != "onboarding" {
			t.Errorf("expected tags[1]=%q, got %q", "onboarding", tags[1])
		}
	})
}

// =========================================================================
// GetMessageDetail tests
// =========================================================================

// ---------------------------------------------------------------------------
// Test 1: Returns 404 for non-existent message
// ---------------------------------------------------------------------------

func TestGetMessageDetail_NotFound(t *testing.T) {
	db := setupMessagesDB(t)
	h := mock.NewHandlers(db, nil)
	router := setupMessagesRouter(h)

	rec := doRequest(t, router, http.MethodGet, "/mock/messages/nonexistent-key")

	t.Run("returns 404 status", func(t *testing.T) {
		if rec.Code != http.StatusNotFound {
			t.Errorf("expected status 404, got %d; body: %s", rec.Code, rec.Body.String())
		}
	})
}

// ---------------------------------------------------------------------------
// Test 2: Returns full message detail for existing message
// ---------------------------------------------------------------------------

func TestGetMessageDetail_ReturnsFullDetail(t *testing.T) {
	db := setupMessagesDB(t)
	h := mock.NewHandlers(db, nil)
	router := setupMessagesRouter(h)

	msg := message.StoredMessage{
		DomainName:      "example.com",
		MessageID:       "<detail-test@example.com>",
		StorageKey:      "detail-test@example.com",
		From:            "sender@example.com",
		To:              "recipient@test.com",
		Subject:         "Detailed test email",
		TextBody:        "Hello plain text",
		HTMLBody:        "<h1>Hello HTML</h1>",
		Tags:            `["welcome","onboarding"]`,
		CustomHeaders:   `{"X-Custom":"value1"}`,
		CustomVariables: `{"campaign":"spring2024"}`,
		Options:         `{"tracking":"yes"}`,
	}
	if err := db.Create(&msg).Error; err != nil {
		t.Fatalf("failed to create message: %v", err)
	}

	rec := doRequest(t, router, http.MethodGet, "/mock/messages/detail-test@example.com")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	resp := decodeMessageDetail(t, rec)

	t.Run("returns correct id", func(t *testing.T) {
		if resp.ID == "" {
			t.Error("expected non-empty id")
		}
	})

	t.Run("returns correct message_id", func(t *testing.T) {
		if resp.MessageID != "<detail-test@example.com>" {
			t.Errorf("expected message_id=%q, got %q", "<detail-test@example.com>", resp.MessageID)
		}
	})

	t.Run("returns correct storage_key", func(t *testing.T) {
		if resp.StorageKey != "detail-test@example.com" {
			t.Errorf("expected storage_key=%q, got %q", "detail-test@example.com", resp.StorageKey)
		}
	})

	t.Run("returns correct domain", func(t *testing.T) {
		if resp.Domain != "example.com" {
			t.Errorf("expected domain=%q, got %q", "example.com", resp.Domain)
		}
	})

	t.Run("returns correct from", func(t *testing.T) {
		if resp.From != "sender@example.com" {
			t.Errorf("expected from=%q, got %q", "sender@example.com", resp.From)
		}
	})

	t.Run("returns to as array", func(t *testing.T) {
		if len(resp.To) != 1 || resp.To[0] != "recipient@test.com" {
			t.Errorf("expected to=[%q], got %v", "recipient@test.com", resp.To)
		}
	})

	t.Run("returns correct subject", func(t *testing.T) {
		if resp.Subject != "Detailed test email" {
			t.Errorf("expected subject=%q, got %q", "Detailed test email", resp.Subject)
		}
	})

	t.Run("returns text_body", func(t *testing.T) {
		if resp.TextBody != "Hello plain text" {
			t.Errorf("expected text_body=%q, got %q", "Hello plain text", resp.TextBody)
		}
	})

	t.Run("returns html_body", func(t *testing.T) {
		if resp.HTMLBody != "<h1>Hello HTML</h1>" {
			t.Errorf("expected html_body=%q, got %q", "<h1>Hello HTML</h1>", resp.HTMLBody)
		}
	})
}

// ---------------------------------------------------------------------------
// Test 3: Includes events timeline for the message
// ---------------------------------------------------------------------------

func TestGetMessageDetail_IncludesEvents(t *testing.T) {
	db := setupMessagesDB(t)
	h := mock.NewHandlers(db, nil)
	router := setupMessagesRouter(h)

	now := float64(time.Now().UnixMicro()) / 1e6

	msg := createTestMessage(t, db, "example.com", "events-detail@example.com", "a@example.com", "b@example.com", "Events detail test", "[]")

	// Create events for this message
	createTestEvent(t, db, "example.com", msg.MessageID, msg.StorageKey, "accepted", "b@example.com", now-2)
	createTestEvent(t, db, "example.com", msg.MessageID, msg.StorageKey, "delivered", "b@example.com", now-1)
	createTestEvent(t, db, "example.com", msg.MessageID, msg.StorageKey, "opened", "b@example.com", now)

	// Create an event for a different message (should not be included)
	createTestEvent(t, db, "example.com", "<other-msg@example.com>", "other-msg@example.com", "accepted", "c@example.com", now)

	rec := doRequest(t, router, http.MethodGet, "/mock/messages/events-detail@example.com")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	resp := decodeMessageDetail(t, rec)

	t.Run("events list contains events for this message only", func(t *testing.T) {
		if len(resp.Events) != 3 {
			t.Errorf("expected 3 events, got %d", len(resp.Events))
		}
	})

	t.Run("events have event_type field", func(t *testing.T) {
		if len(resp.Events) > 0 {
			foundAccepted := false
			foundDelivered := false
			foundOpened := false
			for _, ev := range resp.Events {
				switch ev.EventType {
				case "accepted":
					foundAccepted = true
				case "delivered":
					foundDelivered = true
				case "opened":
					foundOpened = true
				}
			}
			if !foundAccepted {
				t.Error("expected to find accepted event")
			}
			if !foundDelivered {
				t.Error("expected to find delivered event")
			}
			if !foundOpened {
				t.Error("expected to find opened event")
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Test 4: Includes attachment metadata
// ---------------------------------------------------------------------------

func TestGetMessageDetail_IncludesAttachments(t *testing.T) {
	db := setupMessagesDB(t)
	h := mock.NewHandlers(db, nil)
	router := setupMessagesRouter(h)

	msg := createTestMessage(t, db, "example.com", "att-detail@example.com", "a@example.com", "b@example.com", "Attachment detail", "[]")

	createTestAttachment(t, db, msg.ID, "report.pdf", "application/pdf", 2048)
	createTestAttachment(t, db, msg.ID, "image.png", "image/png", 512)

	rec := doRequest(t, router, http.MethodGet, "/mock/messages/att-detail@example.com")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	resp := decodeMessageDetail(t, rec)

	t.Run("attachments list has correct count", func(t *testing.T) {
		if len(resp.Attachments) != 2 {
			t.Errorf("expected 2 attachments, got %d", len(resp.Attachments))
		}
	})

	t.Run("attachments include filename, size, and content_type", func(t *testing.T) {
		if len(resp.Attachments) < 2 {
			t.Skip("not enough attachments to check fields")
		}
		// Find the PDF attachment
		found := false
		for _, att := range resp.Attachments {
			if att.Filename == "report.pdf" {
				found = true
				if att.Size != 2048 {
					t.Errorf("expected size=2048, got %d", att.Size)
				}
				if att.ContentType != "application/pdf" {
					t.Errorf("expected content_type=%q, got %q", "application/pdf", att.ContentType)
				}
			}
		}
		if !found {
			t.Error("did not find attachment with filename report.pdf")
		}
	})
}

// ---------------------------------------------------------------------------
// Test 5: Parses JSON string fields into proper JSON objects/arrays
// ---------------------------------------------------------------------------

func TestGetMessageDetail_ParsesJSONFields(t *testing.T) {
	db := setupMessagesDB(t)
	h := mock.NewHandlers(db, nil)
	router := setupMessagesRouter(h)

	msg := message.StoredMessage{
		DomainName:      "example.com",
		MessageID:       "<json-fields@example.com>",
		StorageKey:      "json-fields@example.com",
		From:            "a@example.com",
		To:              "b@example.com",
		Subject:         "JSON fields test",
		Tags:            `["tag1","tag2"]`,
		CustomHeaders:   `{"X-Custom-Header":"value"}`,
		CustomVariables: `{"campaign_id":"abc123"}`,
		Options:         `{"tracking":"yes","testmode":"no"}`,
	}
	if err := db.Create(&msg).Error; err != nil {
		t.Fatalf("failed to create message: %v", err)
	}

	rec := doRequest(t, router, http.MethodGet, "/mock/messages/json-fields@example.com")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	resp := decodeMessageDetail(t, rec)

	t.Run("tags is a parsed JSON array", func(t *testing.T) {
		if len(resp.Tags) != 2 {
			t.Errorf("expected 2 tags, got %d", len(resp.Tags))
		}
		if len(resp.Tags) >= 2 {
			if resp.Tags[0] != "tag1" || resp.Tags[1] != "tag2" {
				t.Errorf("expected tags=[tag1,tag2], got %v", resp.Tags)
			}
		}
	})

	t.Run("custom_headers is a parsed JSON object", func(t *testing.T) {
		if resp.CustomHeaders == nil {
			t.Fatal("expected custom_headers to be non-nil")
		}
		val, ok := resp.CustomHeaders["X-Custom-Header"]
		if !ok {
			t.Error("expected custom_headers to contain X-Custom-Header")
		}
		if val != "value" {
			t.Errorf("expected custom_headers[X-Custom-Header]=%q, got %v", "value", val)
		}
	})

	t.Run("custom_variables is a parsed JSON object", func(t *testing.T) {
		if resp.CustomVariables == nil {
			t.Fatal("expected custom_variables to be non-nil")
		}
		val, ok := resp.CustomVariables["campaign_id"]
		if !ok {
			t.Error("expected custom_variables to contain campaign_id")
		}
		if val != "abc123" {
			t.Errorf("expected custom_variables[campaign_id]=%q, got %v", "abc123", val)
		}
	})

	t.Run("options is a parsed JSON object", func(t *testing.T) {
		if resp.Options == nil {
			t.Fatal("expected options to be non-nil")
		}
		val, ok := resp.Options["tracking"]
		if !ok {
			t.Error("expected options to contain tracking")
		}
		if val != "yes" {
			t.Errorf("expected options[tracking]=%q, got %v", "yes", val)
		}
	})
}

// =========================================================================
// DeleteSingleMessage tests
// =========================================================================

// ---------------------------------------------------------------------------
// Test 1: Returns 404 for non-existent message
// ---------------------------------------------------------------------------

func TestDeleteSingleMessage_NotFound(t *testing.T) {
	db := setupMessagesDB(t)
	h := mock.NewHandlers(db, nil)
	router := setupMessagesRouter(h)

	rec := doRequest(t, router, http.MethodDelete, "/mock/messages/nonexistent-key")

	t.Run("returns 404 status", func(t *testing.T) {
		if rec.Code != http.StatusNotFound {
			t.Errorf("expected status 404, got %d; body: %s", rec.Code, rec.Body.String())
		}
	})
}

// ---------------------------------------------------------------------------
// Test 2: Successfully deletes an existing message
// ---------------------------------------------------------------------------

func TestDeleteSingleMessage_Success(t *testing.T) {
	db := setupMessagesDB(t)
	h := mock.NewHandlers(db, nil)
	router := setupMessagesRouter(h)

	createTestMessage(t, db, "example.com", "delete-me@example.com", "a@example.com", "b@example.com", "Delete me", "[]")

	rec := doRequest(t, router, http.MethodDelete, "/mock/messages/delete-me@example.com")

	t.Run("returns 200 status", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d; body: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("returns correct success message", func(t *testing.T) {
		resp := decodeSuccess(t, rec)
		if resp.Message != "Message deleted" {
			t.Errorf("expected message=%q, got %q", "Message deleted", resp.Message)
		}
	})
}

// ---------------------------------------------------------------------------
// Test 3: After deletion, message is not returned by list
// ---------------------------------------------------------------------------

func TestDeleteSingleMessage_NotInListAfterDeletion(t *testing.T) {
	db := setupMessagesDB(t)
	h := mock.NewHandlers(db, nil)
	router := setupMessagesRouter(h)

	createTestMessage(t, db, "example.com", "keep-me@example.com", "a@example.com", "b@example.com", "Keep me", "[]")
	createTestMessage(t, db, "example.com", "delete-me@example.com", "a@example.com", "b@example.com", "Delete me", "[]")

	// Delete one message
	delRec := doRequest(t, router, http.MethodDelete, "/mock/messages/delete-me@example.com")
	if delRec.Code != http.StatusOK {
		t.Fatalf("delete failed with status %d", delRec.Code)
	}

	// List messages
	listRec := doRequest(t, router, http.MethodGet, "/mock/messages")
	if listRec.Code != http.StatusOK {
		t.Fatalf("list failed with status %d; body: %s", listRec.Code, listRec.Body.String())
	}

	resp := decodeMessagesList(t, listRec)

	t.Run("deleted message is not in list", func(t *testing.T) {
		for _, item := range resp.Items {
			if item.StorageKey == "delete-me@example.com" {
				t.Error("deleted message should not appear in list")
			}
		}
	})

	t.Run("remaining messages still in list", func(t *testing.T) {
		if len(resp.Items) != 1 {
			t.Errorf("expected 1 remaining item, got %d", len(resp.Items))
		}
		if len(resp.Items) > 0 && resp.Items[0].StorageKey != "keep-me@example.com" {
			t.Errorf("expected remaining item storage_key=%q, got %q", "keep-me@example.com", resp.Items[0].StorageKey)
		}
	})
}

// =========================================================================
// ClearAllMessages tests
// =========================================================================

// ---------------------------------------------------------------------------
// Test 1: Clears all messages from the database
// ---------------------------------------------------------------------------

func TestClearAllMessages_ClearsMessages(t *testing.T) {
	db := setupMessagesDB(t)
	h := mock.NewHandlers(db, nil)
	router := setupMessagesRouter(h)

	// Create several messages
	createTestMessage(t, db, "example.com", "clear1@example.com", "a@example.com", "b@example.com", "Clear 1", "[]")
	createTestMessage(t, db, "example.com", "clear2@example.com", "a@example.com", "b@example.com", "Clear 2", "[]")
	createTestMessage(t, db, "other.com", "clear3@other.com", "a@other.com", "b@other.com", "Clear 3", "[]")

	rec := doRequest(t, router, http.MethodPost, "/mock/messages/clear")

	t.Run("returns 200 status", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d; body: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("returns correct success message", func(t *testing.T) {
		resp := decodeSuccess(t, rec)
		if resp.Message != "All messages have been cleared" {
			t.Errorf("expected message=%q, got %q", "All messages have been cleared", resp.Message)
		}
	})

	// Verify messages are gone
	listRec := doRequest(t, router, http.MethodGet, "/mock/messages")
	if listRec.Code != http.StatusOK {
		t.Fatalf("list failed with status %d; body: %s", listRec.Code, listRec.Body.String())
	}

	listResp := decodeMessagesList(t, listRec)

	t.Run("list returns empty after clearing", func(t *testing.T) {
		if len(listResp.Items) != 0 {
			t.Errorf("expected 0 items after clear, got %d", len(listResp.Items))
		}
		if listResp.TotalCount != 0 {
			t.Errorf("expected total_count=0 after clear, got %d", listResp.TotalCount)
		}
	})
}

// ---------------------------------------------------------------------------
// Test 2: Associated events are also cleared
// ---------------------------------------------------------------------------

func TestClearAllMessages_ClearsEvents(t *testing.T) {
	db := setupMessagesDB(t)
	h := mock.NewHandlers(db, nil)
	router := setupMessagesRouter(h)

	now := float64(time.Now().UnixMicro()) / 1e6

	msg := createTestMessage(t, db, "example.com", "clear-ev@example.com", "a@example.com", "b@example.com", "Clear events test", "[]")
	createTestEvent(t, db, "example.com", msg.MessageID, msg.StorageKey, "accepted", "b@example.com", now)
	createTestEvent(t, db, "example.com", msg.MessageID, msg.StorageKey, "delivered", "b@example.com", now)

	// Verify events exist before clear
	var eventCountBefore int64
	db.Model(&event.Event{}).Count(&eventCountBefore)
	if eventCountBefore == 0 {
		t.Fatal("expected events to exist before clearing")
	}

	rec := doRequest(t, router, http.MethodPost, "/mock/messages/clear")
	if rec.Code != http.StatusOK {
		t.Fatalf("clear failed with status %d; body: %s", rec.Code, rec.Body.String())
	}

	// Verify events are also cleared
	var eventCountAfter int64
	db.Model(&event.Event{}).Count(&eventCountAfter)

	t.Run("events are cleared along with messages", func(t *testing.T) {
		if eventCountAfter != 0 {
			t.Errorf("expected 0 events after clear, got %d", eventCountAfter)
		}
	})
}

// ---------------------------------------------------------------------------
// Test 3: After clearing, list returns empty
// ---------------------------------------------------------------------------

func TestClearAllMessages_ListReturnsEmpty(t *testing.T) {
	db := setupMessagesDB(t)
	h := mock.NewHandlers(db, nil)
	router := setupMessagesRouter(h)

	createTestMessage(t, db, "example.com", "pre-clear@example.com", "a@example.com", "b@example.com", "Pre-clear", "[]")

	// Clear
	clearRec := doRequest(t, router, http.MethodPost, "/mock/messages/clear")
	if clearRec.Code != http.StatusOK {
		t.Fatalf("clear failed with status %d", clearRec.Code)
	}

	// List
	listRec := doRequest(t, router, http.MethodGet, "/mock/messages")
	if listRec.Code != http.StatusOK {
		t.Fatalf("list failed with status %d; body: %s", listRec.Code, listRec.Body.String())
	}

	resp := decodeMessagesList(t, listRec)

	t.Run("items is empty after clear", func(t *testing.T) {
		if resp.Items == nil {
			t.Fatal("expected items to be empty array, got nil")
		}
		if len(resp.Items) != 0 {
			t.Errorf("expected 0 items, got %d", len(resp.Items))
		}
	})

	t.Run("total_count is zero after clear", func(t *testing.T) {
		if resp.TotalCount != 0 {
			t.Errorf("expected total_count=0, got %d", resp.TotalCount)
		}
	})
}
