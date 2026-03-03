package message

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/bethmaloney/mailgun-mock-api/internal/database"
	"github.com/bethmaloney/mailgun-mock-api/internal/event"
	"github.com/bethmaloney/mailgun-mock-api/internal/mock"
	"github.com/bethmaloney/mailgun-mock-api/internal/response"
	"github.com/bethmaloney/mailgun-mock-api/internal/tag"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// domainLookup is a minimal struct used to query the domains table
// without importing the domain package (avoiding circular imports).
type domainLookup struct {
	Name string
}

func (domainLookup) TableName() string { return "domains" }

// templateLookup is a minimal struct used to query the templates table
// without importing the template package (avoiding circular imports).
type templateLookup struct {
	ID         string
	DomainName string
	Name       string
}

func (templateLookup) TableName() string { return "templates" }

// templateVersionLookup is a minimal struct used to query the template_versions table
// without importing the template package (avoiding circular imports).
type templateVersionLookup struct {
	ID         string
	TemplateID string
	Tag        string
	Template   string // the Handlebars content
	Engine     string
	Active     bool
	Headers    string // JSON-encoded headers
}

func (templateVersionLookup) TableName() string { return "template_versions" }

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
	MIMEBody           string  // Raw MIME content for messages sent via messages.mime
	Attachments        string  // JSON string describing attachment metadata
}

// Handlers holds the database connection and mock configuration for message endpoints.
type Handlers struct {
	db            *gorm.DB
	config        *mock.MockConfig
	eventHandlers *event.Handlers
}

// NewHandlers creates a new Handlers instance. Event handlers are automatically
// created using the same db and config so that event generation works without
// requiring an explicit SetEventHandlers call.
func NewHandlers(db *gorm.DB, config *mock.MockConfig) *Handlers {
	return &Handlers{
		db:            db,
		config:        config,
		eventHandlers: event.NewHandlers(db, config),
	}
}

// SetEventHandlers sets the event handlers used for event generation when
// messages are sent. This allows sharing event handlers with the server.
func (h *Handlers) SetEventHandlers(eh *event.Handlers) {
	h.eventHandlers = eh
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

	// Template resolution: if a template name is provided, look it up, find
	// the correct version, render variables, and apply template headers.
	if template != "" {
		result, errMsg := h.resolveTemplate(domainName, template, templateVersion, templateVariables, customVariables)
		if errMsg != "" {
			response.RespondError(w, http.StatusBadRequest, errMsg)
			return
		}
		if result != nil {
			html = result.renderedHTML
			if subject == "" && result.subject != "" {
				subject = result.subject
			}
			if result.replyTo != "" {
				if _, exists := customHeaders["Reply-To"]; !exists {
					customHeaders["Reply-To"] = result.replyTo
				}
			}
			templateVersion = result.versionTag

			// Handle t:text=yes — generate plain text from rendered HTML
			if r.FormValue("t:text") == "yes" && text == "" {
				text = stripHTMLTags(html)
			}

			// Re-marshal customHeaders since we may have added Reply-To
			customHeadersJSON, _ = json.Marshal(customHeaders)
		}
	}

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

	// Save file attachments
	if r.MultipartForm != nil {
		for _, fieldName := range []string{"attachment", "inline"} {
			files := r.MultipartForm.File[fieldName]
			isInline := fieldName == "inline"
			for _, fileHeader := range files {
				file, err := fileHeader.Open()
				if err != nil {
					continue
				}
				content, err := io.ReadAll(file)
				file.Close()
				if err != nil {
					continue
				}

				contentType := fileHeader.Header.Get("Content-Type")
				if contentType == "" || contentType == "application/octet-stream" {
					if ext := filepath.Ext(fileHeader.Filename); ext != "" {
						if mimeType := mime.TypeByExtension(ext); mimeType != "" {
							contentType = mimeType
						}
					}
					if contentType == "" || contentType == "application/octet-stream" {
						contentType = "application/octet-stream"
					}
				}

				att := Attachment{
					StoredMessageID: msg.ID,
					FileName:        fileHeader.Filename,
					ContentType:     contentType,
					Size:            len(content),
					Content:         content,
					Inline:          isInline,
				}
				h.db.Create(&att)
			}
		}
	}

	// Auto-create tags
	if len(tags) > 0 {
		tag.EnsureTags(h.db, domainName, tags)
	}

	// Generate events for each recipient
	if h.eventHandlers != nil {
		recipients := parseRecipients(to)
		for _, recipient := range recipients {
			_ = h.eventHandlers.GenerateAcceptedEvent(
				domainName, messageID, storageKey, from, recipient, subject,
				tagsJSON, string(customVariablesJSON),
			)

			// Check if recipient is on any suppression list
			suppressionReason := h.eventHandlers.CheckSuppression(domainName, recipient, tagsJSON)
			if suppressionReason != "" {
				// Generate a suppression failed event instead of delivery
				_ = h.eventHandlers.GenerateSuppressionFailedEvent(
					domainName, messageID, storageKey, from, recipient, subject,
					tagsJSON, string(customVariablesJSON), suppressionReason,
				)
			} else if h.config != nil && h.config.EventGeneration.AutoDeliver && h.config.EventGeneration.DeliveryDelayMs == 0 {
				_ = h.eventHandlers.GenerateDeliveryEvent(
					domainName, messageID, storageKey, from, recipient, subject,
					tagsJSON, string(customVariablesJSON),
				)
			}
		}
	}

	response.RespondJSON(w, http.StatusOK, map[string]string{
		"id":      messageID,
		"message": "Queued. Thank you.",
	})
}

// templateRenderResult holds the output of template resolution and rendering.
type templateRenderResult struct {
	renderedHTML string
	subject      string
	from         string
	replyTo      string
	versionTag   string
}

// resolveTemplate looks up a template by name within a domain, finds the
// appropriate version, renders the content with variables, and extracts
// template headers. It returns (result, "") on success, (nil, "") if the
// templates table doesn't exist (graceful skip), or (nil, errorMessage) on
// a user-facing error (template not found, version not found, etc.).
func (h *Handlers) resolveTemplate(domainName, templateField, versionTag, templateVariablesJSON string, customVariables map[string]string) (*templateRenderResult, string) {
	templateName := strings.ToLower(templateField)

	// Look up template by domain and name
	var tmpl templateLookup
	if err := h.db.Where("domain_name = ? AND name = ?", domainName, templateName).First(&tmpl).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Sprintf("template '%s' not found", templateName)
		}
		// Table may not exist (e.g. test setups without template migration) — skip gracefully
		return nil, ""
	}

	// Find the version: use specified tag or the active version
	var ver templateVersionLookup
	if versionTag != "" {
		if err := h.db.Where("template_id = ? AND tag = ?", tmpl.ID, versionTag).First(&ver).Error; err != nil {
			return nil, fmt.Sprintf("version '%s' not found for template '%s'", versionTag, templateName)
		}
	} else {
		if err := h.db.Where("template_id = ? AND active = ?", tmpl.ID, true).First(&ver).Error; err != nil {
			return nil, fmt.Sprintf("no active version found for template '%s'", templateName)
		}
	}

	// Merge variables: start with v:* custom vars, then overlay t:variables (takes precedence)
	vars := make(map[string]interface{})
	for k, v := range customVariables {
		vars[k] = v
	}
	if templateVariablesJSON != "" {
		var tvars map[string]interface{}
		if err := json.Unmarshal([]byte(templateVariablesJSON), &tvars); err == nil {
			for k, v := range tvars {
				vars[k] = v
			}
		}
	}

	// Render the template content with variables
	renderedHTML, err := renderTemplate(ver.Template, vars)
	if err != nil {
		return nil, fmt.Sprintf("failed to render template '%s': %s", templateName, err.Error())
	}

	result := &templateRenderResult{
		renderedHTML: renderedHTML,
		versionTag:   ver.Tag,
	}

	// Extract template headers
	if ver.Headers != "" {
		var headers map[string]string
		if err := json.Unmarshal([]byte(ver.Headers), &headers); err == nil {
			if subj, ok := headers["Subject"]; ok {
				// Also render the subject with variables
				if renderedSubj, renderErr := renderTemplate(subj, vars); renderErr == nil {
					result.subject = renderedSubj
				} else {
					result.subject = subj
				}
			}
			if fromHeader, ok := headers["From"]; ok {
				result.from = fromHeader
			}
			if replyTo, ok := headers["Reply-To"]; ok {
				result.replyTo = replyTo
			}
		}
	}

	return result, ""
}

// parseRecipients splits a comma-separated list of email addresses and trims whitespace.
func parseRecipients(to string) []string {
	parts := strings.Split(to, ",")
	var recipients []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			recipients = append(recipients, p)
		}
	}
	return recipients
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

	// Add MIME body if present
	if msg.MIMEBody != "" {
		resp["mime-body"] = msg.MIMEBody
	}

	// Add attachments
	var attachments []Attachment
	h.db.Where("stored_message_id = ?", msg.ID).Find(&attachments)

	attList := make([]map[string]interface{}, 0, len(attachments))
	for _, att := range attachments {
		attList = append(attList, map[string]interface{}{
			"size":         att.Size,
			"url":          fmt.Sprintf("/v3/domains/%s/messages/%s/attachments/%s", domainName, storageKey, att.ID),
			"filename":     att.FileName,
			"content-type": att.ContentType,
		})
	}
	resp["attachments"] = attList

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

	// Delete associated attachments
	h.db.Unscoped().Where("stored_message_id = ?", msg.ID).Delete(&Attachment{})

	if err := h.db.Unscoped().Delete(&msg).Error; err != nil {
		response.RespondError(w, http.StatusInternalServerError, "Failed to delete message")
		return
	}

	response.RespondJSON(w, http.StatusOK, map[string]string{
		"message": "Message has been deleted",
	})
}
