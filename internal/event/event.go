package event

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bethmaloney/mailgun-mock-api/internal/database"
	"github.com/bethmaloney/mailgun-mock-api/internal/mock"
	"github.com/bethmaloney/mailgun-mock-api/internal/pagination"
	"github.com/bethmaloney/mailgun-mock-api/internal/response"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// GORM Model
// ---------------------------------------------------------------------------

// Event represents a Mailgun event stored in the database. Events are generated
// when messages are sent and track the lifecycle of each message (accepted,
// delivered, failed, etc.).
type Event struct {
	database.BaseModel
	DomainName      string  `gorm:"index"`
	EventType       string  `gorm:"index"`           // "accepted", "delivered", "failed", "rejected", "opened", "clicked", "unsubscribed", "complained"
	Timestamp       float64 `gorm:"index"`            // Unix epoch with microsecond precision
	LogLevel        string                            // "info", "warn", "error"
	MessageID       string  `gorm:"index"`            // Links to stored message
	StorageKey      string                            // Storage key for message retrieval
	Recipient       string  `gorm:"index"`
	RecipientDomain string
	Tags            string                            // JSON array
	UserVariables   string                            // JSON object
	Payload         string                            // Full event JSON payload (varies by type)
	Severity        string                            // "temporary" or "permanent" (failed events only)
	Reason          string                            // e.g., "bounce", "suppress-bounce" (failed events only)
	From            string                            // Sender address (for filtering)
	Subject         string                            // Message subject (for filtering)
}

// ---------------------------------------------------------------------------
// Domain lookup helper (avoids importing domain package)
// ---------------------------------------------------------------------------

type domainLookup struct {
	Name string
}

func (domainLookup) TableName() string { return "domains" }

// ---------------------------------------------------------------------------
// Suppression lookup helpers (avoids importing suppression package)
// ---------------------------------------------------------------------------

// bounceLookup is a minimal struct for querying the bounces table
type bounceLookup struct {
	database.BaseModel
	DomainName string
	Address    string
	Code       string
	Error      string
}

func (bounceLookup) TableName() string { return "bounces" }

// complaintLookup is a minimal struct for querying the complaints table
type complaintLookup struct {
	database.BaseModel
	DomainName string
	Address    string
	Count      int
}

func (complaintLookup) TableName() string { return "complaints" }

// unsubscribeLookup is a minimal struct for querying the unsubscribes table
type unsubscribeLookup struct {
	database.BaseModel
	DomainName string
	Address    string
	Tags       string
}

func (unsubscribeLookup) TableName() string { return "unsubscribes" }

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// Handlers holds the database connection and mock configuration for event endpoints.
type Handlers struct {
	db     *gorm.DB
	config *mock.MockConfig
}

// NewHandlers creates a new Handlers instance.
func NewHandlers(db *gorm.DB, config *mock.MockConfig) *Handlers {
	return &Handlers{db: db, config: config}
}

// ---------------------------------------------------------------------------
// ListEvents — GET /v3/{domain_name}/events
// ---------------------------------------------------------------------------

// ListEvents handles GET /v3/{domain_name}/events.
// It returns events filtered by the query parameters.
func (h *Handlers) ListEvents(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")

	// Verify domain exists
	var dl domainLookup
	if err := h.db.Where("name = ?", domainName).First(&dl).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Domain not found")
		return
	}

	q := r.URL.Query()

	// Parse limit
	limit := 100
	if v := q.Get("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > 300 {
		limit = 300
	}

	// Parse begin/end times
	var beginTS, endTS float64
	var hasBegin, hasEnd bool

	if v := q.Get("begin"); v != "" {
		ts, err := parseTimestamp(v)
		if err != nil {
			response.RespondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid begin format: %v", err))
			return
		}
		beginTS = ts
		hasBegin = true
	}

	if v := q.Get("end"); v != "" {
		ts, err := parseTimestamp(v)
		if err != nil {
			response.RespondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid end format: %v", err))
			return
		}
		endTS = ts
		hasEnd = true
	}

	// Parse order direction (needed before cursor logic)
	ascending := q.Get("ascending") == "yes"

	// Build query
	query := h.db.Where("domain_name = ?", domainName)

	// Handle cursor-based pagination: decode page/pivot params
	pageParam := q.Get("page")
	pivotParam := q.Get("pivot")
	if pageParam != "" && pivotParam != "" {
		cursorData, err := pagination.DecodeCursor(pivotParam)
		if err == nil {
			cursorTS := cursorData["timestamp"]
			cursorID := cursorData["id"]
			if cursorTS != "" && cursorID != "" {
				tsVal, parseErr := strconv.ParseFloat(cursorTS, 64)
				if parseErr == nil {
					if ascending {
						query = query.Where("(timestamp > ? OR (timestamp = ? AND id > ?))", tsVal, tsVal, cursorID)
					} else {
						query = query.Where("(timestamp < ? OR (timestamp = ? AND id < ?))", tsVal, tsVal, cursorID)
					}
				}
			}
		}
	}

	// Event type filter (supports OR)
	if eventFilter := q.Get("event"); eventFilter != "" {
		types := parseEventTypeFilter(eventFilter)
		if len(types) == 1 {
			query = query.Where("event_type = ?", types[0])
		} else if len(types) > 1 {
			query = query.Where("event_type IN ?", types)
		}
	}

	// Recipient filter
	if recipient := q.Get("recipient"); recipient != "" {
		query = query.Where("recipient = ?", recipient)
	}

	// To filter (maps to recipient in the mock; Mailgun uses "to" for the MIME To header)
	if toFilter := q.Get("to"); toFilter != "" {
		query = query.Where("recipient LIKE ?", fmt.Sprintf("%%%s%%", toFilter))
	}

	// Message-ID filter
	if messageID := q.Get("message-id"); messageID != "" {
		query = query.Where("message_id = ?", messageID)
	}

	// Severity filter
	if severity := q.Get("severity"); severity != "" {
		query = query.Where("severity = ?", severity)
	}

	// Tags filter (JSON LIKE)
	if tags := q.Get("tags"); tags != "" {
		query = query.Where("tags LIKE ?", fmt.Sprintf("%%\"%s\"%%", tags))
	}

	// From filter
	if from := q.Get("from"); from != "" {
		query = query.Where("\"from\" LIKE ?", fmt.Sprintf("%%%s%%", from))
	}

	// Subject filter
	if subject := q.Get("subject"); subject != "" {
		query = query.Where("subject LIKE ?", fmt.Sprintf("%%%s%%", subject))
	}

	// Time range
	if hasBegin {
		query = query.Where("timestamp >= ?", beginTS)
	}
	if hasEnd {
		query = query.Where("timestamp <= ?", endTS)
	}

	// Order
	if ascending {
		query = query.Order("timestamp ASC, id ASC")
	} else {
		query = query.Order("timestamp DESC, id DESC")
	}

	// Fetch limit+1 to determine if there are more results
	var events []Event
	query.Limit(limit + 1).Find(&events)

	hasMore := len(events) > limit
	if hasMore {
		events = events[:limit]
	}

	// Build response items
	items := make([]map[string]interface{}, 0, len(events))
	for _, ev := range events {
		items = append(items, buildEventResponseItem(ev, domainName))
	}

	// Build paging URLs
	baseURL := fmt.Sprintf("http://%s/v3/%s/events", r.Host, domainName)
	if r.Host == "" {
		baseURL = fmt.Sprintf("http://localhost/v3/%s/events", domainName)
	}

	var cursor string
	if len(events) > 0 {
		lastEvent := events[len(events)-1]
		cursor = pagination.EncodeCursor(map[string]string{
			"timestamp": fmt.Sprintf("%.6f", lastEvent.Timestamp),
			"id":        lastEvent.ID,
		})
	}

	paging := pagination.GeneratePagingURLs(baseURL, limit, cursor, hasMore)

	resp := map[string]interface{}{
		"items":  items,
		"paging": paging,
	}

	response.RespondJSON(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// Event Generation
// ---------------------------------------------------------------------------

// GenerateAcceptedEvent creates an "accepted" event for a recipient when a
// message is sent. It is called by the message sending handler.
func (h *Handlers) GenerateAcceptedEvent(domainName, messageID, storageKey, from, recipient, subject, tags, userVariables string) error {
	recipientDomain := extractDomain(recipient)
	ts := float64(time.Now().UnixMicro()) / 1e6

	ev := Event{
		DomainName:      domainName,
		EventType:       "accepted",
		Timestamp:       ts,
		LogLevel:        "info",
		MessageID:       messageID,
		StorageKey:      storageKey,
		Recipient:       recipient,
		RecipientDomain: recipientDomain,
		Tags:            tags,
		UserVariables:   userVariables,
		From:            from,
		Subject:         subject,
	}

	// Build payload
	payload := buildEventPayload(ev, domainName)
	payloadJSON, _ := json.Marshal(payload)
	ev.Payload = string(payloadJSON)

	return h.db.Create(&ev).Error
}

// GenerateDeliveryEvent creates a "delivered" event for a recipient. It is
// called after an accepted event when auto-delivery is enabled.
func (h *Handlers) GenerateDeliveryEvent(domainName, messageID, storageKey, from, recipient, subject, tags, userVariables string) error {
	recipientDomain := extractDomain(recipient)
	ts := float64(time.Now().UnixMicro()) / 1e6

	ev := Event{
		DomainName:      domainName,
		EventType:       "delivered",
		Timestamp:       ts,
		LogLevel:        "info",
		MessageID:       messageID,
		StorageKey:      storageKey,
		Recipient:       recipient,
		RecipientDomain: recipientDomain,
		Tags:            tags,
		UserVariables:   userVariables,
		From:            from,
		Subject:         subject,
	}

	// Build payload with delivery-status
	payload := buildEventPayload(ev, domainName)
	payload["delivery-status"] = map[string]interface{}{
		"code":                 250,
		"attempt-no":           1,
		"message":              "OK",
		"description":          "",
		"mx-host":              "mock-mx.example.com",
		"session-seconds":      0.1,
		"tls":                  true,
		"certificate-verified": true,
		"utf8":                 true,
	}
	payloadJSON, _ := json.Marshal(payload)
	ev.Payload = string(payloadJSON)

	return h.db.Create(&ev).Error
}

// ---------------------------------------------------------------------------
// Suppression Checking
// ---------------------------------------------------------------------------

// CheckSuppression checks if a recipient is on any suppression list for the domain.
// Returns the suppression reason (e.g., "suppress-bounce") or "" if not suppressed.
// messageTags should be a JSON array string of the message's tags.
func (h *Handlers) CheckSuppression(domainName, recipient, messageTags string) string {
	// 1. Check bounce list
	var bounce bounceLookup
	if err := h.db.Where("domain_name = ? AND address = ?", domainName, recipient).First(&bounce).Error; err == nil {
		return "suppress-bounce"
	}

	// 2. Check complaint list
	var complaint complaintLookup
	if err := h.db.Where("domain_name = ? AND address = ?", domainName, recipient).First(&complaint).Error; err == nil {
		return "suppress-complaint"
	}

	// 3. Check unsubscribe list
	var unsub unsubscribeLookup
	if err := h.db.Where("domain_name = ? AND address = ?", domainName, recipient).First(&unsub).Error; err == nil {
		// Parse the unsubscribe tags
		var unsubTags []string
		if unsub.Tags != "" {
			_ = json.Unmarshal([]byte(unsub.Tags), &unsubTags)
		}

		// If tags contain "*", always suppress
		for _, tag := range unsubTags {
			if tag == "*" {
				return "suppress-unsubscribe"
			}
		}

		// Parse the message tags
		var msgTags []string
		if messageTags != "" && messageTags != "[]" {
			_ = json.Unmarshal([]byte(messageTags), &msgTags)
		}

		// Check if any unsubscribe tag matches any message tag
		for _, unsubTag := range unsubTags {
			for _, msgTag := range msgTags {
				if unsubTag == msgTag {
					return "suppress-unsubscribe"
				}
			}
		}
	}

	return ""
}

// GenerateSuppressionFailedEvent creates a "failed" event for a recipient
// that is on a suppression list. The reason should be one of
// "suppress-bounce", "suppress-complaint", or "suppress-unsubscribe".
func (h *Handlers) GenerateSuppressionFailedEvent(domainName, messageID, storageKey, from, recipient, subject, tags, userVariables, reason string) error {
	recipientDomain := extractDomain(recipient)
	ts := float64(time.Now().UnixMicro()) / 1e6

	ev := Event{
		DomainName:      domainName,
		EventType:       "failed",
		Timestamp:       ts,
		LogLevel:        "error",
		MessageID:       messageID,
		StorageKey:      storageKey,
		Recipient:       recipient,
		RecipientDomain: recipientDomain,
		Tags:            tags,
		UserVariables:   userVariables,
		Severity:        "permanent",
		Reason:          reason,
		From:            from,
		Subject:         subject,
	}

	// Build payload
	payload := buildEventPayload(ev, domainName)

	// Determine description based on reason
	var description string
	switch reason {
	case "suppress-bounce":
		description = "Not delivering to previously bounced address"
	case "suppress-complaint":
		description = "Not delivering to a user who marked your messages as spam"
	case "suppress-unsubscribe":
		description = "Not delivering to unsubscribed address"
	default:
		description = "Suppressed"
	}

	// Add delivery-status block
	payload["delivery-status"] = map[string]interface{}{
		"code":          550,
		"attempt-no":    1,
		"description":   description,
		"message":       description,
		"enhanced-code": "5.1.1",
		"bounce-type":   "hard",
	}

	payloadJSON, _ := json.Marshal(payload)
	ev.Payload = string(payloadJSON)

	return h.db.Create(&ev).Error
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// parseTimestamp parses a time string as either a Unix epoch float or RFC 2822/RFC 1123Z date.
func parseTimestamp(s string) (float64, error) {
	// Try Unix epoch float first
	if ts, err := strconv.ParseFloat(s, 64); err == nil {
		return ts, nil
	}

	// Try RFC 1123Z (RFC 2822 compatible)
	if t, err := time.Parse(time.RFC1123Z, s); err == nil {
		return float64(t.UnixMicro()) / 1e6, nil
	}

	// Try RFC 1123
	if t, err := time.Parse(time.RFC1123, s); err == nil {
		return float64(t.UnixMicro()) / 1e6, nil
	}

	return 0, fmt.Errorf("unrecognized time format: %q", s)
}

// parseEventTypeFilter splits an event type filter that may contain OR.
func parseEventTypeFilter(filter string) []string {
	// Check for OR (case insensitive)
	parts := strings.Split(filter, " OR ")
	if len(parts) == 1 {
		// Try lowercase "or"
		parts = strings.Split(filter, " or ")
	}
	var types []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			types = append(types, p)
		}
	}
	return types
}

// extractDomain returns the domain part of an email address.
func extractDomain(email string) string {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return ""
}

// buildEventPayload builds the JSON payload map for an event.
func buildEventPayload(ev Event, domainName string) map[string]interface{} {
	// Parse tags
	var tags []string
	if ev.Tags != "" && ev.Tags != "[]" {
		_ = json.Unmarshal([]byte(ev.Tags), &tags)
	}
	if tags == nil {
		tags = []string{}
	}

	// Parse user variables
	var userVars map[string]interface{}
	if ev.UserVariables != "" && ev.UserVariables != "{}" {
		_ = json.Unmarshal([]byte(ev.UserVariables), &userVars)
	}
	if userVars == nil {
		userVars = map[string]interface{}{}
	}

	payload := map[string]interface{}{
		"id":               ev.ID,
		"event":            ev.EventType,
		"timestamp":        ev.Timestamp,
		"log-level":        ev.LogLevel,
		"recipient":        ev.Recipient,
		"recipient-domain": ev.RecipientDomain,
		"envelope": map[string]interface{}{
			"sender":     ev.From,
			"transport":  "http",
			"targets":    ev.Recipient,
			"sending-ip": "127.0.0.1",
		},
		"message": map[string]interface{}{
			"headers": map[string]interface{}{
				"to":         ev.Recipient,
				"message-id": ev.MessageID,
				"from":       ev.From,
				"subject":    ev.Subject,
			},
			"attachments": []interface{}{},
			"recipients":  []string{ev.Recipient},
			"size":        0,
		},
		"flags": map[string]interface{}{
			"is-authenticated": true,
			"is-test-mode":     false,
			"is-system-test":   false,
		},
		"tags":           tags,
		"user-variables": userVars,
		"storage": map[string]interface{}{
			"key": ev.StorageKey,
			"url": fmt.Sprintf("http://localhost/v3/domains/%s/messages/%s", domainName, ev.StorageKey),
		},
	}

	// Add method field for accepted and delivered events (API-sent messages use "http")
	if ev.EventType == "accepted" || ev.EventType == "delivered" {
		payload["method"] = "http"
	}

	if ev.Severity != "" {
		payload["severity"] = ev.Severity
	}
	if ev.Reason != "" {
		payload["reason"] = ev.Reason
	}

	return payload
}

// buildEventResponseItem creates the JSON response representation of an event.
func buildEventResponseItem(ev Event, domainName string) map[string]interface{} {
	// Try to use the stored payload first
	if ev.Payload != "" {
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(ev.Payload), &payload); err == nil {
			// Override the ID with the actual event ID (payload was generated before ID was set)
			payload["id"] = ev.ID
			return payload
		}
	}

	// Fall back to reconstructing from fields
	return buildEventPayload(ev, domainName)
}
