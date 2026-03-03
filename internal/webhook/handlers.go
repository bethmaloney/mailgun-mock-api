package webhook

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/bethmaloney/mailgun-mock-api/internal/database"
	"github.com/bethmaloney/mailgun-mock-api/internal/mock"
	"github.com/bethmaloney/mailgun-mock-api/internal/response"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// DomainWebhook represents a webhook URL registered for a specific event type on a domain.
type DomainWebhook struct {
	database.BaseModel
	DomainName string `gorm:"index;uniqueIndex:idx_webhook_domain_event_url"`
	EventType  string `gorm:"uniqueIndex:idx_webhook_domain_event_url"` // accepted, delivered, etc.
	URL        string `gorm:"uniqueIndex:idx_webhook_domain_event_url"`
}

// Handlers provides HTTP handlers for webhook endpoints.
type Handlers struct {
	db     *gorm.DB
	config *mock.MockConfig
}

// NewHandlers creates a new Handlers instance.
func NewHandlers(db *gorm.DB, config *mock.MockConfig) *Handlers {
	return &Handlers{db: db, config: config}
}

// validEventTypes is the set of valid Mailgun webhook event types.
var validEventTypes = map[string]bool{
	"accepted": true, "delivered": true, "opened": true, "clicked": true,
	"unsubscribed": true, "complained": true, "temporary_fail": true, "permanent_fail": true,
}

// allEventTypes is the canonical ordered list of event types.
var allEventTypes = []string{
	"accepted", "delivered", "opened", "clicked",
	"unsubscribed", "complained", "temporary_fail", "permanent_fail",
}

// domainLookup is a lightweight struct for checking domain existence without importing domain package.
type domainLookup struct {
	Name string
}

func (domainLookup) TableName() string { return "domains" }

// getDomainName extracts the domain name from the URL params, trying "domain_name", "domain", then "name".
func getDomainName(r *http.Request) string {
	if v := chi.URLParam(r, "domain_name"); v != "" {
		return v
	}
	if v := chi.URLParam(r, "domain"); v != "" {
		return v
	}
	if v := chi.URLParam(r, "name"); v != "" {
		return v
	}
	return ""
}

// checkDomainExists verifies the domain exists in the database. Returns false and writes an error response if not.
func (h *Handlers) checkDomainExists(w http.ResponseWriter, domainName string) bool {
	var dl domainLookup
	if err := h.db.Where("name = ?", domainName).First(&dl).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Domain not found")
		return false
	}
	return true
}

// buildWebhooksMap builds the full webhooks map for a domain, with all event types present.
func (h *Handlers) buildWebhooksMap(domainName string) map[string]interface{} {
	var webhooks []DomainWebhook
	h.db.Where("domain_name = ?", domainName).Order("event_type, url").Find(&webhooks)

	// Group URLs by event type
	urlsByType := map[string][]string{}
	for _, wh := range webhooks {
		urlsByType[wh.EventType] = append(urlsByType[wh.EventType], wh.URL)
	}

	// Build the response map — nil for unconfigured types
	result := map[string]interface{}{}
	for _, et := range allEventTypes {
		if urls, ok := urlsByType[et]; ok && len(urls) > 0 {
			result[et] = map[string]interface{}{"urls": urls}
		} else {
			result[et] = nil
		}
	}
	return result
}

// ListWebhooks handles GET /v3/domains/{domain_name}/webhooks
func (h *Handlers) ListWebhooks(w http.ResponseWriter, r *http.Request) {
	domainName := getDomainName(r)
	if !h.checkDomainExists(w, domainName) {
		return
	}

	webhooksMap := h.buildWebhooksMap(domainName)
	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"webhooks": webhooksMap,
	})
}

// GetWebhook handles GET /v3/domains/{domain_name}/webhooks/{webhook_name}
func (h *Handlers) GetWebhook(w http.ResponseWriter, r *http.Request) {
	domainName := getDomainName(r)
	if !h.checkDomainExists(w, domainName) {
		return
	}

	webhookName := chi.URLParam(r, "webhook_name")

	var webhooks []DomainWebhook
	h.db.Where("domain_name = ? AND event_type = ?", domainName, webhookName).Order("url").Find(&webhooks)

	if len(webhooks) == 0 {
		response.RespondError(w, http.StatusNotFound, "Webhook not found")
		return
	}

	urls := make([]string, len(webhooks))
	for i, wh := range webhooks {
		urls[i] = wh.URL
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"webhook": map[string]interface{}{
			"urls": urls,
		},
	})
}

// CreateWebhook handles POST /v3/domains/{domain_name}/webhooks
func (h *Handlers) CreateWebhook(w http.ResponseWriter, r *http.Request) {
	domainName := getDomainName(r)
	if !h.checkDomainExists(w, domainName) {
		return
	}

	r.ParseMultipartForm(32 << 20)

	eventType := r.FormValue("id")
	if eventType == "" {
		response.RespondError(w, http.StatusBadRequest, "id is required")
		return
	}
	if !validEventTypes[eventType] {
		response.RespondError(w, http.StatusBadRequest, "Invalid event type")
		return
	}

	urls := r.Form["url"]
	if len(urls) == 0 {
		response.RespondError(w, http.StatusBadRequest, "At least one url is required")
		return
	}
	if len(urls) > 3 {
		response.RespondError(w, http.StatusBadRequest, "Maximum 3 URLs allowed")
		return
	}

	for _, u := range urls {
		wh := DomainWebhook{
			DomainName: domainName,
			EventType:  eventType,
			URL:        u,
		}
		if err := h.db.Create(&wh).Error; err != nil {
			response.RespondError(w, http.StatusInternalServerError, "Failed to create webhook")
			return
		}
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Webhook has been created",
		"webhook": map[string]interface{}{
			"urls": urls,
		},
	})
}

// UpdateWebhook handles PUT /v3/domains/{domain_name}/webhooks/{webhook_name}
func (h *Handlers) UpdateWebhook(w http.ResponseWriter, r *http.Request) {
	domainName := getDomainName(r)
	if !h.checkDomainExists(w, domainName) {
		return
	}

	webhookName := chi.URLParam(r, "webhook_name")

	r.ParseMultipartForm(32 << 20)

	urls := r.Form["url"]
	if len(urls) == 0 {
		response.RespondError(w, http.StatusBadRequest, "At least one url is required")
		return
	}
	if len(urls) > 3 {
		response.RespondError(w, http.StatusBadRequest, "Maximum 3 URLs allowed")
		return
	}

	// Delete all existing webhooks for this domain + event type
	h.db.Unscoped().Where("domain_name = ? AND event_type = ?", domainName, webhookName).Delete(&DomainWebhook{})

	// Create new rows
	for _, u := range urls {
		wh := DomainWebhook{
			DomainName: domainName,
			EventType:  webhookName,
			URL:        u,
		}
		if err := h.db.Create(&wh).Error; err != nil {
			response.RespondError(w, http.StatusInternalServerError, "Failed to update webhook")
			return
		}
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Webhook has been updated",
		"webhook": map[string]interface{}{
			"urls": urls,
		},
	})
}

// DeleteWebhook handles DELETE /v3/domains/{domain_name}/webhooks/{webhook_name}
func (h *Handlers) DeleteWebhook(w http.ResponseWriter, r *http.Request) {
	domainName := getDomainName(r)
	if !h.checkDomainExists(w, domainName) {
		return
	}

	webhookName := chi.URLParam(r, "webhook_name")

	// Check if any webhooks exist for this event type
	var count int64
	h.db.Model(&DomainWebhook{}).Where("domain_name = ? AND event_type = ?", domainName, webhookName).Count(&count)
	if count == 0 {
		response.RespondError(w, http.StatusNotFound, "Webhook not found")
		return
	}

	// Delete all webhooks for this domain + event type
	h.db.Unscoped().Where("domain_name = ? AND event_type = ?", domainName, webhookName).Delete(&DomainWebhook{})

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Webhook has been deleted",
		"webhook": map[string]interface{}{
			"urls": []string{},
		},
	})
}

// V4CreateWebhook handles POST /v4/domains/{domain}/webhooks
func (h *Handlers) V4CreateWebhook(w http.ResponseWriter, r *http.Request) {
	domainName := getDomainName(r)
	if !h.checkDomainExists(w, domainName) {
		return
	}

	r.ParseForm()

	webhookURL := r.FormValue("url")
	if webhookURL == "" {
		response.RespondError(w, http.StatusBadRequest, "url is required")
		return
	}

	eventTypes := r.Form["event_types"]
	if len(eventTypes) == 0 {
		response.RespondError(w, http.StatusBadRequest, "At least one event_types is required")
		return
	}

	for _, et := range eventTypes {
		if !validEventTypes[et] {
			response.RespondError(w, http.StatusBadRequest, "Invalid event type: "+et)
			return
		}
	}

	// Create a DomainWebhook row for each event type (skip if already exists due to unique index)
	for _, et := range eventTypes {
		wh := DomainWebhook{
			DomainName: domainName,
			EventType:  et,
			URL:        webhookURL,
		}
		// Use FirstOrCreate to handle the unique constraint gracefully
		h.db.Where("domain_name = ? AND event_type = ? AND url = ?", domainName, et, webhookURL).FirstOrCreate(&wh)
	}

	webhooksMap := h.buildWebhooksMap(domainName)
	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"webhooks": webhooksMap,
	})
}

// V4UpdateWebhook handles PUT /v4/domains/{domain}/webhooks
func (h *Handlers) V4UpdateWebhook(w http.ResponseWriter, r *http.Request) {
	domainName := getDomainName(r)
	if !h.checkDomainExists(w, domainName) {
		return
	}

	r.ParseForm()

	webhookURL := r.FormValue("url")
	if webhookURL == "" {
		response.RespondError(w, http.StatusBadRequest, "url is required")
		return
	}

	eventTypes := r.Form["event_types"]
	if len(eventTypes) == 0 {
		response.RespondError(w, http.StatusBadRequest, "At least one event_types is required")
		return
	}

	for _, et := range eventTypes {
		if !validEventTypes[et] {
			response.RespondError(w, http.StatusBadRequest, "Invalid event type: "+et)
			return
		}
	}

	// Delete ALL existing DomainWebhook rows for this URL on this domain
	h.db.Unscoped().Where("domain_name = ? AND url = ?", domainName, webhookURL).Delete(&DomainWebhook{})

	// Create new rows for each specified event type
	for _, et := range eventTypes {
		wh := DomainWebhook{
			DomainName: domainName,
			EventType:  et,
			URL:        webhookURL,
		}
		h.db.Create(&wh)
	}

	webhooksMap := h.buildWebhooksMap(domainName)
	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"webhooks": webhooksMap,
	})
}

// V4DeleteWebhook handles DELETE /v4/domains/{domain}/webhooks
func (h *Handlers) V4DeleteWebhook(w http.ResponseWriter, r *http.Request) {
	domainName := getDomainName(r)
	if !h.checkDomainExists(w, domainName) {
		return
	}

	// Parse comma-separated URLs from query param
	urlParam := r.URL.Query().Get("url")
	if urlParam == "" {
		response.RespondError(w, http.StatusBadRequest, "url query parameter is required")
		return
	}

	urls := strings.Split(urlParam, ",")
	for _, u := range urls {
		h.db.Unscoped().Where("domain_name = ? AND url = ?", domainName, u).Delete(&DomainWebhook{})
	}

	webhooksMap := h.buildWebhooksMap(domainName)
	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"webhooks": webhooksMap,
	})
}

// GetSigningKey handles GET /v5/accounts/http_signing_key
func (h *Handlers) GetSigningKey(w http.ResponseWriter, r *http.Request) {
	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"message":          "success",
		"http_signing_key": h.config.Authentication.SigningKey,
	})
}

// RegenerateSigningKey handles POST /v5/accounts/http_signing_key
func (h *Handlers) RegenerateSigningKey(w http.ResponseWriter, r *http.Request) {
	// Generate 24 random bytes (48 hex chars)
	randomBytes := make([]byte, 24)
	if _, err := rand.Read(randomBytes); err != nil {
		response.RespondError(w, http.StatusInternalServerError, "Failed to generate signing key")
		return
	}
	newKey := "key-" + hex.EncodeToString(randomBytes)

	h.config.Authentication.SigningKey = newKey

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"message":          "success",
		"http_signing_key": newKey,
	})
}

// ---------------------------------------------------------------------------
// Account-Level Webhooks (v1)
// ---------------------------------------------------------------------------

// AccountWebhook represents an account-level webhook configuration.
type AccountWebhook struct {
	database.BaseModel
	WebhookID   string `gorm:"uniqueIndex"`
	URL         string
	EventTypes  string // JSON array stored as string
	Description string
}

// WebhookDelivery records a webhook delivery attempt.
type WebhookDelivery struct {
	database.BaseModel
	WebhookURL     string
	EventType      string
	EventID        string
	DomainName     string
	StatusCode     int
	ResponseTimeMs int
	Attempt        int
	Payload        string // JSON payload
}

// generateRandomHex generates n random bytes and returns their hex encoding.
func generateRandomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// accountWebhookToMap converts an AccountWebhook to its JSON response representation.
func accountWebhookToMap(aw AccountWebhook) map[string]interface{} {
	var eventTypes []string
	if aw.EventTypes != "" {
		json.Unmarshal([]byte(aw.EventTypes), &eventTypes)
	}
	if eventTypes == nil {
		eventTypes = []string{}
	}
	return map[string]interface{}{
		"webhook_id":  aw.WebhookID,
		"url":         aw.URL,
		"event_types": eventTypes,
		"description": aw.Description,
		"created_at":  aw.CreatedAt.Format(time.RFC3339),
	}
}

// ListAccountWebhooks handles GET /v1/webhooks
func (h *Handlers) ListAccountWebhooks(w http.ResponseWriter, r *http.Request) {
	var webhooks []AccountWebhook
	query := h.db

	if idsParam := r.URL.Query().Get("webhook_ids"); idsParam != "" {
		ids := strings.Split(idsParam, ",")
		query = query.Where("webhook_id IN ?", ids)
	}

	query.Find(&webhooks)

	result := make([]map[string]interface{}, 0, len(webhooks))
	for _, wh := range webhooks {
		result = append(result, accountWebhookToMap(wh))
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"webhooks": result,
	})
}

// GetAccountWebhook handles GET /v1/webhooks/{webhook_id}
func (h *Handlers) GetAccountWebhook(w http.ResponseWriter, r *http.Request) {
	webhookID := chi.URLParam(r, "webhook_id")

	var aw AccountWebhook
	if err := h.db.Where("webhook_id = ?", webhookID).First(&aw).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Webhook not found")
		return
	}

	response.RespondJSON(w, http.StatusOK, accountWebhookToMap(aw))
}

// CreateAccountWebhook handles POST /v1/webhooks
func (h *Handlers) CreateAccountWebhook(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(32 << 20)

	url := r.FormValue("url")
	if url == "" {
		response.RespondError(w, http.StatusBadRequest, "url is required")
		return
	}

	eventTypes := r.Form["event_types"]
	if len(eventTypes) == 0 {
		response.RespondError(w, http.StatusBadRequest, "At least one event_types is required")
		return
	}

	for _, et := range eventTypes {
		if !validEventTypes[et] {
			response.RespondError(w, http.StatusBadRequest, "Invalid event type: "+et)
			return
		}
	}

	description := r.FormValue("description")

	eventTypesJSON, _ := json.Marshal(eventTypes)
	webhookID := "wh_" + generateRandomHex(12)

	aw := AccountWebhook{
		WebhookID:   webhookID,
		URL:         url,
		EventTypes:  string(eventTypesJSON),
		Description: description,
	}

	if err := h.db.Create(&aw).Error; err != nil {
		response.RespondError(w, http.StatusInternalServerError, "Failed to create webhook")
		return
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"webhook_id": webhookID,
	})
}

// UpdateAccountWebhook handles PUT /v1/webhooks/{webhook_id}
func (h *Handlers) UpdateAccountWebhook(w http.ResponseWriter, r *http.Request) {
	webhookID := chi.URLParam(r, "webhook_id")

	var aw AccountWebhook
	if err := h.db.Where("webhook_id = ?", webhookID).First(&aw).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Webhook not found")
		return
	}

	r.ParseMultipartForm(32 << 20)

	if url := r.FormValue("url"); url != "" {
		aw.URL = url
	}

	if eventTypes := r.Form["event_types"]; len(eventTypes) > 0 {
		for _, et := range eventTypes {
			if !validEventTypes[et] {
				response.RespondError(w, http.StatusBadRequest, "Invalid event type: "+et)
				return
			}
		}
		eventTypesJSON, _ := json.Marshal(eventTypes)
		aw.EventTypes = string(eventTypesJSON)
	}

	if _, ok := r.Form["description"]; ok {
		aw.Description = r.FormValue("description")
	}

	h.db.Save(&aw)
	w.WriteHeader(http.StatusNoContent)
}

// DeleteAccountWebhook handles DELETE /v1/webhooks/{webhook_id}
func (h *Handlers) DeleteAccountWebhook(w http.ResponseWriter, r *http.Request) {
	webhookID := chi.URLParam(r, "webhook_id")

	var aw AccountWebhook
	if err := h.db.Where("webhook_id = ?", webhookID).First(&aw).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Webhook not found")
		return
	}

	h.db.Unscoped().Delete(&aw)
	w.WriteHeader(http.StatusNoContent)
}

// BulkDeleteAccountWebhooks handles DELETE /v1/webhooks (with query params)
func (h *Handlers) BulkDeleteAccountWebhooks(w http.ResponseWriter, r *http.Request) {
	if idsParam := r.URL.Query().Get("webhook_ids"); idsParam != "" {
		ids := strings.Split(idsParam, ",")
		h.db.Unscoped().Where("webhook_id IN ?", ids).Delete(&AccountWebhook{})
	} else if r.URL.Query().Get("all") == "true" {
		h.db.Unscoped().Where("1 = 1").Delete(&AccountWebhook{})
	}

	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Mock Webhook Inspection
// ---------------------------------------------------------------------------

// ListDeliveries handles GET /mock/webhooks/deliveries
func (h *Handlers) ListDeliveries(w http.ResponseWriter, r *http.Request) {
	var deliveries []WebhookDelivery
	h.db.Order("created_at DESC").Find(&deliveries)

	result := make([]map[string]interface{}, 0, len(deliveries))
	for _, d := range deliveries {
		var payload interface{}
		if d.Payload != "" {
			json.Unmarshal([]byte(d.Payload), &payload)
		}

		result = append(result, map[string]interface{}{
			"id":               d.ID,
			"webhook_url":      d.WebhookURL,
			"event_type":       d.EventType,
			"event_id":         d.EventID,
			"domain":           d.DomainName,
			"status_code":      d.StatusCode,
			"response_time_ms": d.ResponseTimeMs,
			"attempt":          d.Attempt,
			"timestamp":        d.CreatedAt.Format(time.RFC3339),
			"payload":          payload,
		})
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"deliveries": result,
	})
}

// messageLookup is a lightweight struct for looking up stored messages.
type messageLookup struct {
	StorageKey string
	DomainName string
	MessageID  string
	From       string
	To         string
	Subject    string
}

func (messageLookup) TableName() string { return "stored_messages" }

// TriggerWebhook handles POST /mock/webhooks/trigger
func (h *Handlers) TriggerWebhook(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Domain    string `json:"domain"`
		EventType string `json:"event_type"`
		Recipient string `json:"recipient"`
		MessageID string `json:"message_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.RespondError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	// Look up the stored message
	var msg messageLookup
	h.db.Where("message_id = ? AND domain_name = ?", body.MessageID, body.Domain).First(&msg)

	// Find matching webhook URLs
	urlSet := map[string]bool{}

	// Domain-level webhooks
	var domainWebhooks []DomainWebhook
	h.db.Where("domain_name = ? AND event_type = ?", body.Domain, body.EventType).Find(&domainWebhooks)
	for _, dw := range domainWebhooks {
		urlSet[dw.URL] = true
	}

	// Account-level webhooks — find those whose event_types JSON contains the event type
	var accountWebhooks []AccountWebhook
	h.db.Find(&accountWebhooks)
	for _, aw := range accountWebhooks {
		var eventTypes []string
		if err := json.Unmarshal([]byte(aw.EventTypes), &eventTypes); err != nil {
			continue
		}
		for _, et := range eventTypes {
			if et == body.EventType {
				urlSet[aw.URL] = true
				break
			}
		}
	}

	// Build signature
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	token := generateRandomHex(25)
	mac := hmac.New(sha256.New, []byte(h.config.Authentication.SigningKey))
	mac.Write([]byte(timestamp + token))
	signature := hex.EncodeToString(mac.Sum(nil))

	// Build the event payload
	eventID := generateRandomHex(16)
	payload := map[string]interface{}{
		"signature": map[string]interface{}{
			"timestamp": timestamp,
			"token":     token,
			"signature": signature,
		},
		"event-data": map[string]interface{}{
			"event":     body.EventType,
			"timestamp": time.Now().Unix(),
			"id":        eventID,
			"recipient": body.Recipient,
			"message": map[string]interface{}{
				"headers": map[string]interface{}{
					"message-id": body.MessageID,
					"from":       msg.From,
					"to":         msg.To,
					"subject":    msg.Subject,
				},
			},
		},
	}

	payloadJSON, _ := json.Marshal(payload)

	// Create delivery records for each unique URL
	for url := range urlSet {
		delivery := WebhookDelivery{
			WebhookURL:     url,
			EventType:      body.EventType,
			EventID:        eventID,
			DomainName:     body.Domain,
			StatusCode:     0,
			ResponseTimeMs: 0,
			Attempt:        1,
			Payload:        string(payloadJSON),
		}
		h.db.Create(&delivery)
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"message":    "Webhook triggered",
		"deliveries": len(urlSet),
	})
}
