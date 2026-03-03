package message

import (
	"crypto/rand"
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

// domainLookup is a minimal struct used to query the domains table
// without importing the domain package (avoiding circular imports).
type domainLookup struct {
	Name string
}

func (domainLookup) TableName() string { return "domains" }

// StoredMessage represents a message stored in the database after being sent
// via the Mailgun API.
type StoredMessage struct {
	database.BaseModel
	DomainName         string `gorm:"index"`
	MessageID          string `gorm:"uniqueIndex"`  // RFC-2392 format: <timestamp.random@domain>
	StorageKey         string `gorm:"uniqueIndex"`   // Inner part without angle brackets
	From               string
	To                 string  // comma-separated recipients
	CC                 string
	BCC                string
	Subject            string
	TextBody           string
	HTMLBody           string
	AMPHTMLBody        string
	Template           string
	TemplateVersion    string
	TemplateVariables  string  // JSON string
	Tags               string  // JSON array string
	CustomHeaders      string  // JSON object string (h: prefixed fields)
	CustomVariables    string  // JSON object string (v: prefixed fields)
	RecipientVariables string  // JSON string
	Options            string  // JSON object string (o: prefixed fields)
	TestMode           bool
}

// Handlers holds the database connection and mock configuration for message endpoints.
type Handlers struct {
	db     *gorm.DB
	config *mock.MockConfig
}

// NewHandlers creates a new Handlers instance.
func NewHandlers(db *gorm.DB, config *mock.MockConfig) *Handlers {
	return &Handlers{db: db, config: config}
}

// generateMessageID creates a message ID in the format <timestamp.hexrandom@domain>.
func generateMessageID(domainName string) (string, string) {
	timestamp := time.Now().Unix()
	randomBytes := make([]byte, 8)
	rand.Read(randomBytes)
	randomHex := hex.EncodeToString(randomBytes)
	storageKey := fmt.Sprintf("%d.%s@%s", timestamp, randomHex, domainName)
	messageID := fmt.Sprintf("<%s>", storageKey)
	return messageID, storageKey
}

// SendMessage handles POST /v3/{domain_name}/messages.
func (h *Handlers) SendMessage(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")

	// Verify domain exists
	var dl domainLookup
	if err := h.db.Where("name = ?", domainName).First(&dl).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Domain not found")
		return
	}

	// Parse multipart form
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		if err2 := r.ParseForm(); err2 != nil {
			response.RespondError(w, http.StatusBadRequest, "Failed to parse request")
			return
		}
	}

	// Validate required fields
	from := r.FormValue("from")
	if from == "" {
		response.RespondError(w, http.StatusBadRequest, "from parameter is missing")
		return
	}

	to := r.FormValue("to")
	if to == "" {
		response.RespondError(w, http.StatusBadRequest, "to parameter is missing")
		return
	}

	subject := r.FormValue("subject")
	template := r.FormValue("template")

	// Subject is required unless a template is provided (the template may supply it)
	if subject == "" && template == "" {
		response.RespondError(w, http.StatusBadRequest, "subject parameter is missing")
		return
	}

	text := r.FormValue("text")
	html := r.FormValue("html")
	ampHTML := r.FormValue("amp-html")

	if text == "" && html == "" && ampHTML == "" && template == "" {
		response.RespondError(w, http.StatusBadRequest, "Need at least one of 'text', 'html', 'amp-html' or 'template' parameters specified")
		return
	}

	// Parse optional fields
	cc := r.FormValue("cc")
	bcc := r.FormValue("bcc")
	recipientVariables := r.FormValue("recipient-variables")
	templateVersion := r.FormValue("t:version")
	templateVariables := r.FormValue("t:variables")

	// Parse tags (o:tag can be repeated)
	tags := r.Form["o:tag"]
	tagsJSON := "[]"
	if len(tags) > 0 {
		b, err := json.Marshal(tags)
		if err == nil {
			tagsJSON = string(b)
		}
	}

	// Parse custom headers (h:* prefixed fields)
	customHeaders := make(map[string]string)
	for key, values := range r.Form {
		if strings.HasPrefix(key, "h:") && len(values) > 0 {
			headerName := strings.TrimPrefix(key, "h:")
			customHeaders[headerName] = values[0]
		}
	}
	customHeadersJSON, _ := json.Marshal(customHeaders)

	// Parse custom variables (v:* prefixed fields)
	customVariables := make(map[string]string)
	for key, values := range r.Form {
		if strings.HasPrefix(key, "v:") && len(values) > 0 {
			varName := strings.TrimPrefix(key, "v:")
			customVariables[varName] = values[0]
		}
	}
	customVariablesJSON, _ := json.Marshal(customVariables)

	// Parse sending options (o:* prefixed fields, except o:tag)
	options := make(map[string]string)
	for key, values := range r.Form {
		if strings.HasPrefix(key, "o:") && key != "o:tag" && len(values) > 0 {
			optName := strings.TrimPrefix(key, "o:")
			options[optName] = values[0]
		}
	}
	optionsJSON, _ := json.Marshal(options)

	// Check test mode
	testMode := r.FormValue("o:testmode") == "yes"

	// Generate message ID and storage key
	messageID, storageKey := generateMessageID(domainName)

	// Create stored message
	msg := StoredMessage{
		DomainName:         domainName,
		MessageID:          messageID,
		StorageKey:         storageKey,
		From:               from,
		To:                 to,
		CC:                 cc,
		BCC:                bcc,
		Subject:            subject,
		TextBody:           text,
		HTMLBody:           html,
		AMPHTMLBody:        ampHTML,
		Template:           template,
		TemplateVersion:    templateVersion,
		TemplateVariables:  templateVariables,
		Tags:               tagsJSON,
		CustomHeaders:      string(customHeadersJSON),
		CustomVariables:    string(customVariablesJSON),
		RecipientVariables: recipientVariables,
		Options:            string(optionsJSON),
		TestMode:           testMode,
	}

	if err := h.db.Create(&msg).Error; err != nil {
		response.RespondError(w, http.StatusInternalServerError, "Failed to store message")
		return
	}

	response.RespondJSON(w, http.StatusOK, map[string]string{
		"id":      messageID,
		"message": "Queued. Thank you.",
	})
}

// GetMessage handles GET /v3/domains/{domain_name}/messages/{storage_key}.
func (h *Handlers) GetMessage(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")
	storageKey := chi.URLParam(r, "storage_key")

	var msg StoredMessage
	if err := h.db.Where("storage_key = ? AND domain_name = ?", storageKey, domainName).First(&msg).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Message not found")
		return
	}

	// Build message headers
	headers := [][]string{
		{"From", msg.From},
		{"To", msg.To},
		{"Subject", msg.Subject},
	}

	if msg.CC != "" {
		headers = append(headers, []string{"Cc", msg.CC})
	}

	// Add custom headers
	var customHeaders map[string]string
	if msg.CustomHeaders != "" {
		_ = json.Unmarshal([]byte(msg.CustomHeaders), &customHeaders) // data written by SendMessage; safe to ignore
		for name, value := range customHeaders {
			headers = append(headers, []string{name, value})
		}
	}

	// Build the response
	resp := map[string]interface{}{
		"From":            msg.From,
		"To":              msg.To,
		"Subject":         msg.Subject,
		"Cc":              msg.CC,
		"Bcc":             msg.BCC,
		"Message-Id":      msg.MessageID,
		"sender":          msg.From,
		"recipients":      msg.To,
		"body-html":       msg.HTMLBody,
		"body-plain":      msg.TextBody,
		"stripped-html":    msg.HTMLBody,
		"stripped-text":    msg.TextBody,
		"stripped-signature": "",
		"message-headers": headers,
	}

	// Add tags
	var tags []string
	if msg.Tags != "" && msg.Tags != "[]" {
		_ = json.Unmarshal([]byte(msg.Tags), &tags) // data written by SendMessage; safe to ignore
		if len(tags) > 0 {
			resp["X-Mailgun-Tag"] = strings.Join(tags, ", ")
		}
	}

	// Add custom variables
	var customVars map[string]string
	if msg.CustomVariables != "" && msg.CustomVariables != "{}" {
		_ = json.Unmarshal([]byte(msg.CustomVariables), &customVars) // data written by SendMessage; safe to ignore
		if len(customVars) > 0 {
			resp["X-Mailgun-Variables"] = msg.CustomVariables
		}
	}

	// Add recipient variables
	if msg.RecipientVariables != "" {
		resp["recipient-variables"] = msg.RecipientVariables
	}

	// Add options
	var options map[string]string
	if msg.Options != "" && msg.Options != "{}" {
		_ = json.Unmarshal([]byte(msg.Options), &options) // data written by SendMessage; safe to ignore
		if len(options) > 0 {
			resp["X-Mailgun-Options"] = msg.Options
		}
	}

	response.RespondJSON(w, http.StatusOK, resp)
}

// DeleteMessage handles DELETE /v3/domains/{domain_name}/messages/{storage_key}.
func (h *Handlers) DeleteMessage(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")
	storageKey := chi.URLParam(r, "storage_key")

	var msg StoredMessage
	if err := h.db.Where("storage_key = ? AND domain_name = ?", storageKey, domainName).First(&msg).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Message not found")
		return
	}

	if err := h.db.Unscoped().Delete(&msg).Error; err != nil {
		response.RespondError(w, http.StatusInternalServerError, "Failed to delete message")
		return
	}

	response.RespondJSON(w, http.StatusOK, map[string]string{
		"message": "Message has been deleted",
	})
}
