package event

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/bethmaloney/mailgun-mock-api/internal/database"
	"github.com/bethmaloney/mailgun-mock-api/internal/response"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm/clause"
)

// ---------------------------------------------------------------------------
// Message lookup helper (avoids importing message package)
// ---------------------------------------------------------------------------

type messageLookup struct {
	StorageKey      string
	DomainName      string
	MessageID       string
	From            string
	To              string
	Subject         string
	Tags            string
	CustomVariables string
}

func (messageLookup) TableName() string { return "stored_messages" }

// allowlistLookup is a minimal struct for querying the allowlist_entries table
type allowlistLookup struct {
	database.BaseModel
	DomainName string
	Type       string
	Value      string
}

func (allowlistLookup) TableName() string { return "allowlist_entries" }

// isAllowlisted checks if a recipient address or its domain is on the allowlist.
func (h *Handlers) isAllowlisted(domainName, recipientAddress string) bool {
	// Check if the exact address is on the allowlist
	var addrEntry allowlistLookup
	if err := h.db.Where("domain_name = ? AND type = ? AND value = ?", domainName, "address", recipientAddress).First(&addrEntry).Error; err == nil {
		return true
	}

	// Check if the recipient's domain is on the allowlist
	recipientDomain := extractDomain(recipientAddress)
	if recipientDomain != "" {
		var domEntry allowlistLookup
		if err := h.db.Where("domain_name = ? AND type = ? AND value = ?", domainName, "domain", recipientDomain).First(&domEntry).Error; err == nil {
			return true
		}
	}

	return false
}

// ---------------------------------------------------------------------------
// Shared trigger helper
// ---------------------------------------------------------------------------

// triggerLookup extracts URL params, verifies the domain exists, and looks up
// the stored message. It returns the domain name and message lookup on success,
// or writes an error response and returns false.
func (h *Handlers) triggerLookup(w http.ResponseWriter, r *http.Request) (string, messageLookup, bool) {
	domainName := chi.URLParam(r, "domain")
	messageID := chi.URLParam(r, "message_id")

	// Verify domain exists
	var dl domainLookup
	if err := h.db.Where("name = ?", domainName).First(&dl).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Domain not found")
		return "", messageLookup{}, false
	}

	// Look up the stored message by storage key and domain
	var msg messageLookup
	if err := h.db.Where("storage_key = ? AND domain_name = ?", messageID, domainName).First(&msg).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Message not found")
		return "", messageLookup{}, false
	}

	return domainName, msg, true
}

// createAndSaveEvent builds an Event from the common fields, marshals the
// payload, saves to the database, and writes the JSON response.
func (h *Handlers) createAndSaveEvent(w http.ResponseWriter, ev Event, payload map[string]interface{}, domainName string) {
	payloadJSON, _ := json.Marshal(payload)
	ev.Payload = string(payloadJSON)

	if err := h.db.Create(&ev).Error; err != nil {
		response.RespondError(w, http.StatusInternalServerError, "Failed to create event")
		return
	}

	h.broadcastEventNew(domainName, ev.EventType)

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"message":  "Event created",
		"event_id": ev.ID,
	})
}

// firstRecipient returns the first comma-separated recipient from the To field.
func firstRecipient(to string) string {
	parts := strings.SplitN(to, ",", 2)
	if len(parts) > 0 {
		return strings.TrimSpace(parts[0])
	}
	return to
}

// buildBaseEvent creates an Event struct populated with common fields from the
// message lookup.
func buildBaseEvent(domainName string, msg messageLookup, eventType, logLevel string) Event {
	recipient := firstRecipient(msg.To)
	return Event{
		DomainName:      domainName,
		EventType:       eventType,
		Timestamp:       float64(time.Now().UnixMicro()) / 1e6,
		LogLevel:        logLevel,
		MessageID:       msg.MessageID,
		StorageKey:      msg.StorageKey,
		Recipient:       recipient,
		RecipientDomain: extractDomain(recipient),
		Tags:            msg.Tags,
		UserVariables:   msg.CustomVariables,
		From:            msg.From,
		Subject:         msg.Subject,
	}
}

// buildMinimalPayload builds a payload with only the message-id header in the
// message field (used for open, click, unsubscribe).
func buildMinimalPayload(ev Event, domainName string) map[string]interface{} {
	// Start with the standard payload
	payload := buildEventPayload(ev, domainName)
	// Replace the full message with a minimal one (only message-id header)
	payload["message"] = map[string]interface{}{
		"headers": map[string]interface{}{
			"message-id": ev.MessageID,
		},
	}
	return payload
}

// addClientInfoAndGeo adds ip, client-info, and geolocation fields to a payload.
func addClientInfoAndGeo(payload map[string]interface{}) {
	payload["ip"] = "127.0.0.1"
	payload["client-info"] = map[string]interface{}{
		"client-type": "browser",
		"client-os":   "Linux",
		"client-name": "Chrome",
		"device-type": "desktop",
		"user-agent":  "Mozilla/5.0 (mock)",
	}
	payload["geolocation"] = map[string]interface{}{
		"country": "US",
		"region":  "CA",
		"city":    "San Francisco",
	}
}

// ---------------------------------------------------------------------------
// TriggerDeliver — POST /mock/events/{domain}/deliver/{message_id}
// ---------------------------------------------------------------------------

func (h *Handlers) TriggerDeliver(w http.ResponseWriter, r *http.Request) {
	domainName, msg, ok := h.triggerLookup(w, r)
	if !ok {
		return
	}

	ev := buildBaseEvent(domainName, msg, "delivered", "info")

	payload := buildEventPayload(ev, domainName)
	payload["method"] = "http"
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

	h.createAndSaveEvent(w, ev, payload, domainName)
}

// ---------------------------------------------------------------------------
// TriggerFail — POST /mock/events/{domain}/fail/{message_id}
// ---------------------------------------------------------------------------

func (h *Handlers) TriggerFail(w http.ResponseWriter, r *http.Request) {
	domainName, msg, ok := h.triggerLookup(w, r)
	if !ok {
		return
	}

	// Parse optional body for severity and reason
	type failRequest struct {
		Severity string `json:"severity"`
		Reason   string `json:"reason"`
	}
	var req failRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	if req.Severity == "" {
		req.Severity = "permanent"
	}
	if req.Reason == "" {
		req.Reason = "bounce"
	}

	logLevel := "error"
	if req.Severity == "temporary" {
		logLevel = "warn"
	}

	ev := buildBaseEvent(domainName, msg, "failed", logLevel)
	ev.Severity = req.Severity
	ev.Reason = req.Reason

	payload := buildEventPayload(ev, domainName)

	// Build delivery-status based on severity
	code := 550
	bounceType := "hard"
	statusMessage := "550 5.1.1 Mailbox does not exist"
	enhancedCode := "5.1.1"
	if req.Severity == "temporary" {
		code = 421
		bounceType = "soft"
		statusMessage = "421 Try again later"
		enhancedCode = "4.2.1"
	}
	payload["delivery-status"] = map[string]interface{}{
		"code":          code,
		"attempt-no":    1,
		"message":       statusMessage,
		"description":   "",
		"enhanced-code": enhancedCode,
		"mx-host":       "mock-mx.example.com",
		"bounce-type":   bounceType,
	}

	h.createAndSaveEvent(w, ev, payload, domainName)

	// Auto-create bounce entry for permanent failures (unless allowlisted)
	if req.Severity == "permanent" {
		recipient := firstRecipient(msg.To)
		if !h.isAllowlisted(domainName, recipient) {
			h.db.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "domain_name"}, {Name: "address"}},
				DoUpdates: clause.AssignmentColumns([]string{"code", "error", "updated_at"}),
			}).Create(&bounceLookup{
				DomainName: domainName,
				Address:    recipient,
				Code:       "550",
				Error:      "Bounced",
			})
		}
	}
}

// ---------------------------------------------------------------------------
// TriggerOpen — POST /mock/events/{domain}/open/{message_id}
// ---------------------------------------------------------------------------

func (h *Handlers) TriggerOpen(w http.ResponseWriter, r *http.Request) {
	domainName, msg, ok := h.triggerLookup(w, r)
	if !ok {
		return
	}

	ev := buildBaseEvent(domainName, msg, "opened", "info")

	payload := buildMinimalPayload(ev, domainName)
	addClientInfoAndGeo(payload)

	h.createAndSaveEvent(w, ev, payload, domainName)
}

// ---------------------------------------------------------------------------
// TriggerClick — POST /mock/events/{domain}/click/{message_id}
// ---------------------------------------------------------------------------

func (h *Handlers) TriggerClick(w http.ResponseWriter, r *http.Request) {
	domainName, msg, ok := h.triggerLookup(w, r)
	if !ok {
		return
	}

	// Parse optional body for URL
	type clickRequest struct {
		URL string `json:"url"`
	}
	var req clickRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	if req.URL == "" {
		req.URL = "http://example.com"
	}

	ev := buildBaseEvent(domainName, msg, "clicked", "info")

	payload := buildMinimalPayload(ev, domainName)
	addClientInfoAndGeo(payload)
	payload["url"] = req.URL

	h.createAndSaveEvent(w, ev, payload, domainName)
}

// ---------------------------------------------------------------------------
// TriggerUnsubscribe — POST /mock/events/{domain}/unsubscribe/{message_id}
// ---------------------------------------------------------------------------

func (h *Handlers) TriggerUnsubscribe(w http.ResponseWriter, r *http.Request) {
	domainName, msg, ok := h.triggerLookup(w, r)
	if !ok {
		return
	}

	ev := buildBaseEvent(domainName, msg, "unsubscribed", "warn")

	payload := buildMinimalPayload(ev, domainName)
	addClientInfoAndGeo(payload)

	h.createAndSaveEvent(w, ev, payload, domainName)
}

// ---------------------------------------------------------------------------
// TriggerComplain — POST /mock/events/{domain}/complain/{message_id}
// ---------------------------------------------------------------------------

func (h *Handlers) TriggerComplain(w http.ResponseWriter, r *http.Request) {
	domainName, msg, ok := h.triggerLookup(w, r)
	if !ok {
		return
	}

	ev := buildBaseEvent(domainName, msg, "complained", "warn")

	// Standard payload with full message (including headers)
	payload := buildEventPayload(ev, domainName)

	// Ensure message has full headers (from, to, subject)
	payload["message"] = map[string]interface{}{
		"headers": map[string]interface{}{
			"to":         ev.Recipient,
			"message-id": ev.MessageID,
			"from":       ev.From,
			"subject":    ev.Subject,
		},
		"attachments": []interface{}{},
		"recipients":  []string{ev.Recipient},
		"size":        0,
	}

	h.createAndSaveEvent(w, ev, payload, domainName)

	// Auto-create complaint entry (allowlist does NOT prevent this)
	recipient := firstRecipient(msg.To)
	h.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "domain_name"}, {Name: "address"}},
		DoUpdates: clause.AssignmentColumns([]string{"count", "updated_at"}),
	}).Create(&complaintLookup{
		DomainName: domainName,
		Address:    recipient,
		Count:      1,
	})
}

