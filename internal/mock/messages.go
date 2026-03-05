package mock

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/bethmaloney/mailgun-mock-api/internal/database"
	"github.com/bethmaloney/mailgun-mock-api/internal/response"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// Lookup structs for messages handler (query other packages' tables without
// importing those packages).
// ---------------------------------------------------------------------------

type storedMessageLookup struct {
	database.BaseModel
	DomainName      string
	MessageID       string
	StorageKey      string
	From            string
	To              string
	Subject         string
	TextBody        string
	HTMLBody        string
	Tags            string
	CustomHeaders   string
	CustomVariables string
	Options         string
}

func (storedMessageLookup) TableName() string { return "stored_messages" }

type eventDetailLookup struct {
	database.BaseModel
	EventType string
	Timestamp float64
	MessageID string
}

func (eventDetailLookup) TableName() string { return "events" }

type attachmentLookup struct {
	database.BaseModel
	StoredMessageID string
	FileName        string
	ContentType     string
	Size            int
}

func (attachmentLookup) TableName() string { return "attachments" }

// ---------------------------------------------------------------------------
// Response types
// ---------------------------------------------------------------------------

type messageListItemResponse struct {
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

type messagesPagingResponse struct {
	Next     string `json:"next"`
	Previous string `json:"previous"`
}

type messagesListResponseBody struct {
	Items      []messageListItemResponse `json:"items"`
	Paging     messagesPagingResponse    `json:"paging"`
	TotalCount int                       `json:"total_count"`
}

type messageDetailResponseBody struct {
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
	Attachments     []attachmentResponse   `json:"attachments"`
	Events          []eventResponse        `json:"events"`
}

type attachmentResponse struct {
	Filename    string `json:"filename"`
	Size        int    `json:"size"`
	ContentType string `json:"content_type"`
}

type eventResponse struct {
	ID        string  `json:"id"`
	EventType string  `json:"event_type"`
	Timestamp float64 `json:"timestamp"`
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func splitTo(to string) []string {
	parts := strings.Split(to, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func parseTags(tagsJSON string) []string {
	var tags []string
	if tagsJSON != "" {
		_ = json.Unmarshal([]byte(tagsJSON), &tags)
	}
	if tags == nil {
		tags = []string{}
	}
	return tags
}

func parseJSONObject(s string) map[string]interface{} {
	var result map[string]interface{}
	if s != "" {
		_ = json.Unmarshal([]byte(s), &result)
	}
	if result == nil {
		result = map[string]interface{}{}
	}
	return result
}

func getLatestEventType(db *gorm.DB, messageID string) string {
	var ev eventDetailLookup
	err := db.Where("message_id = ?", messageID).Order("timestamp DESC").First(&ev).Error
	if err != nil {
		return "accepted"
	}
	return ev.EventType
}

func hasAttachments(db *gorm.DB, storedMessageID string) bool {
	var count int64
	db.Model(&attachmentLookup{}).Where("stored_message_id = ?", storedMessageID).Count(&count)
	return count > 0
}

// encodeCursor encodes an offset into a base64 cursor token.
func encodeCursor(offset int) string {
	return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%d", offset)))
}

// decodeCursor decodes a base64 cursor token into an offset.
func decodeCursor(cursor string) int {
	if cursor == "" {
		return 0
	}
	data, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return 0
	}
	offset, err := strconv.Atoi(string(data))
	if err != nil {
		return 0
	}
	return offset
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// ListMessages handles GET /mock/messages -- paginated message list for the Web UI.
func (h *Handlers) ListMessages(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	// Parse limit (default 50, max 300)
	limit := 50
	if l := q.Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > 300 {
		limit = 300
	}

	// Parse page cursor
	offset := decodeCursor(q.Get("page"))

	// Build query
	query := h.db.Model(&storedMessageLookup{})

	// Apply filters
	if domain := q.Get("domain"); domain != "" {
		query = query.Where("domain_name = ?", domain)
	}
	if from := q.Get("from"); from != "" {
		query = query.Where("LOWER(\"from\") LIKE LOWER(?)", "%"+from+"%")
	}
	if to := q.Get("to"); to != "" {
		query = query.Where("LOWER(\"to\") LIKE LOWER(?)", "%"+to+"%")
	}
	if subject := q.Get("subject"); subject != "" {
		query = query.Where("LOWER(subject) LIKE LOWER(?)", "%"+subject+"%")
	}
	if tag := q.Get("tag"); tag != "" {
		query = query.Where("tags LIKE ?", "%"+tag+"%")
	}
	if start := q.Get("start"); start != "" {
		if ts, err := strconv.ParseInt(start, 10, 64); err == nil {
			startTime := time.Unix(ts, 0)
			query = query.Where("created_at >= ?", startTime)
		}
	}
	if end := q.Get("end"); end != "" {
		if ts, err := strconv.ParseInt(end, 10, 64); err == nil {
			// Add 1 second to make the range inclusive of the entire end second
			endTime := time.Unix(ts+1, 0)
			query = query.Where("created_at < ?", endTime)
		}
	}

	// Get total count
	var totalCount int64
	query.Count(&totalCount)

	// Fetch messages with pagination
	var messages []storedMessageLookup
	query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&messages)

	// Build response items
	items := make([]messageListItemResponse, 0, len(messages))
	for _, msg := range messages {
		items = append(items, messageListItemResponse{
			ID:             msg.ID,
			StorageKey:     msg.StorageKey,
			Domain:         msg.DomainName,
			From:           msg.From,
			To:             splitTo(msg.To),
			Subject:        msg.Subject,
			Tags:           parseTags(msg.Tags),
			Timestamp:      float64(msg.CreatedAt.Unix()),
			Status:         getLatestEventType(h.db, msg.MessageID),
			HasAttachments: hasAttachments(h.db, msg.ID),
		})
	}

	// Build paging
	paging := messagesPagingResponse{}
	nextOffset := offset + limit
	if nextOffset < int(totalCount) {
		paging.Next = fmt.Sprintf("/mock/messages?page=%s", encodeCursor(nextOffset))
	}
	if offset > 0 {
		prevOffset := offset - limit
		if prevOffset < 0 {
			prevOffset = 0
		}
		paging.Previous = fmt.Sprintf("/mock/messages?page=%s", encodeCursor(prevOffset))
	}

	resp := messagesListResponseBody{
		Items:      items,
		Paging:     paging,
		TotalCount: int(totalCount),
	}

	response.RespondJSON(w, http.StatusOK, resp)
}

// GetMessageDetail handles GET /mock/messages/{message_id} -- full message detail for the Web UI.
func (h *Handlers) GetMessageDetail(w http.ResponseWriter, r *http.Request) {
	rawKey := chi.URLParam(r, "message_id")
	storageKey, _ := url.PathUnescape(rawKey)

	var msg storedMessageLookup
	if err := h.db.Where("storage_key = ?", storageKey).First(&msg).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Message not found")
		return
	}

	// Query events for this message
	var events []eventDetailLookup
	h.db.Where("message_id = ?", msg.MessageID).Order("timestamp ASC").Find(&events)

	eventItems := make([]eventResponse, 0, len(events))
	for _, ev := range events {
		eventItems = append(eventItems, eventResponse{
			ID:        ev.ID,
			EventType: ev.EventType,
			Timestamp: ev.Timestamp,
		})
	}

	// Query attachments for this message
	var attachments []attachmentLookup
	h.db.Where("stored_message_id = ?", msg.ID).Find(&attachments)

	attachmentItems := make([]attachmentResponse, 0, len(attachments))
	for _, att := range attachments {
		attachmentItems = append(attachmentItems, attachmentResponse{
			Filename:    att.FileName,
			Size:        att.Size,
			ContentType: att.ContentType,
		})
	}

	resp := messageDetailResponseBody{
		ID:              msg.ID,
		MessageID:       msg.MessageID,
		StorageKey:      msg.StorageKey,
		Domain:          msg.DomainName,
		From:            msg.From,
		To:              splitTo(msg.To),
		Subject:         msg.Subject,
		TextBody:        msg.TextBody,
		HTMLBody:        msg.HTMLBody,
		Tags:            parseTags(msg.Tags),
		CustomHeaders:   parseJSONObject(msg.CustomHeaders),
		CustomVariables: parseJSONObject(msg.CustomVariables),
		Options:         parseJSONObject(msg.Options),
		Timestamp:       float64(msg.CreatedAt.Unix()),
		Attachments:     attachmentItems,
		Events:          eventItems,
	}

	response.RespondJSON(w, http.StatusOK, resp)
}

// DeleteSingleMessage handles DELETE /mock/messages/{message_id} -- delete a single message.
func (h *Handlers) DeleteSingleMessage(w http.ResponseWriter, r *http.Request) {
	rawKey := chi.URLParam(r, "message_id")
	storageKey, _ := url.PathUnescape(rawKey)

	var msg storedMessageLookup
	if err := h.db.Where("storage_key = ?", storageKey).First(&msg).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Message not found")
		return
	}

	// Delete associated attachments
	h.db.Unscoped().Where("stored_message_id = ?", msg.ID).Delete(&attachmentLookup{})

	// Delete associated events
	h.db.Unscoped().Where("message_id = ?", msg.MessageID).Delete(&eventDetailLookup{})

	// Hard delete the message
	h.db.Unscoped().Delete(&msg)

	response.RespondJSON(w, http.StatusOK, map[string]string{"message": "Message deleted"})
}

// ClearAllMessages handles POST /mock/messages/clear -- clear all messages.
func (h *Handlers) ClearAllMessages(w http.ResponseWriter, r *http.Request) {
	// Hard delete all messages
	h.db.Unscoped().Where("1 = 1").Delete(&storedMessageLookup{})

	// Hard delete all events
	h.db.Unscoped().Where("1 = 1").Delete(&eventDetailLookup{})

	// Hard delete all attachments
	h.db.Unscoped().Where("1 = 1").Delete(&attachmentLookup{})

	response.RespondJSON(w, http.StatusOK, map[string]string{"message": "All messages have been cleared"})
}
