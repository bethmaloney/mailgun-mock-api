package message

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/bethmaloney/mailgun-mock-api/internal/response"
	"github.com/go-chi/chi/v5"
)

// SendMIMEMessage handles POST /v3/{domain_name}/messages.mime.
func (h *Handlers) SendMIMEMessage(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")

	// Verify domain exists
	var dl domainLookup
	if err := h.db.Where("name = ?", domainName).First(&dl).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Domain not found")
		return
	}

	// Parse multipart form
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		response.RespondError(w, http.StatusBadRequest, "Failed to parse request")
		return
	}

	// Validate required fields
	to := r.FormValue("to")
	if to == "" {
		response.RespondError(w, http.StatusBadRequest, "to parameter is missing")
		return
	}

	// Get the uploaded MIME message file
	file, _, err := r.FormFile("message")
	if err != nil {
		response.RespondError(w, http.StatusBadRequest, "message parameter is missing")
		return
	}
	defer file.Close()

	mimeContent, err := io.ReadAll(file)
	if err != nil {
		response.RespondError(w, http.StatusInternalServerError, "Failed to read message file")
		return
	}

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

	// Parse recipient variables
	recipientVariables := r.FormValue("recipient-variables")

	// Check test mode
	testMode := r.FormValue("o:testmode") == "yes"

	// Generate message ID and storage key
	messageID, storageKey := generateMessageID(domainName)

	// Create stored message
	msg := StoredMessage{
		DomainName:         domainName,
		MessageID:          messageID,
		StorageKey:         storageKey,
		To:                 to,
		MIMEBody:           string(mimeContent),
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

// ResendMessage handles POST /v3/domains/{domain_name}/messages/{storage_key}.
func (h *Handlers) ResendMessage(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")
	storageKey := chi.URLParam(r, "storage_key")

	// Look up the original message
	var original StoredMessage
	if err := h.db.Where("storage_key = ? AND domain_name = ?", storageKey, domainName).First(&original).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Message not found")
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
	to := r.FormValue("to")
	if to == "" {
		response.RespondError(w, http.StatusBadRequest, "to parameter is missing")
		return
	}

	// Generate a new message ID and storage key
	messageID, newStorageKey := generateMessageID(domainName)

	// Create a new message copying content from the original
	msg := StoredMessage{
		DomainName:         domainName,
		MessageID:          messageID,
		StorageKey:         newStorageKey,
		From:               original.From,
		To:                 to,
		CC:                 original.CC,
		BCC:                original.BCC,
		Subject:            original.Subject,
		TextBody:           original.TextBody,
		HTMLBody:            original.HTMLBody,
		AMPHTMLBody:        original.AMPHTMLBody,
		Template:           original.Template,
		TemplateVersion:    original.TemplateVersion,
		TemplateVariables:  original.TemplateVariables,
		Tags:               original.Tags,
		CustomHeaders:      original.CustomHeaders,
		CustomVariables:    original.CustomVariables,
		RecipientVariables: original.RecipientVariables,
		Options:            original.Options,
		TestMode:           original.TestMode,
		MIMEBody:           original.MIMEBody,
		Attachments:        original.Attachments,
	}

	if err := h.db.Create(&msg).Error; err != nil {
		response.RespondError(w, http.StatusInternalServerError, "Failed to store message")
		return
	}

	// Copy attachments from original message
	var origAttachments []Attachment
	h.db.Where("stored_message_id = ?", original.ID).Find(&origAttachments)
	for _, origAtt := range origAttachments {
		newAtt := Attachment{
			StoredMessageID: msg.ID,
			FileName:        origAtt.FileName,
			ContentType:     origAtt.ContentType,
			Size:            origAtt.Size,
			Content:         origAtt.Content,
			Inline:          origAtt.Inline,
		}
		h.db.Create(&newAtt)
	}

	response.RespondJSON(w, http.StatusOK, map[string]string{
		"id":      messageID,
		"message": "Queued. Thank you.",
	})
}

// GetSendingQueues handles GET /v3/domains/{domain_name}/sending_queues.
func (h *Handlers) GetSendingQueues(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")

	// Verify domain exists
	var dl domainLookup
	if err := h.db.Where("name = ?", domainName).First(&dl).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Domain not found")
		return
	}

	type disabledInfo struct {
		Until  string `json:"until"`
		Reason string `json:"reason"`
	}

	type queueStatus struct {
		IsDisabled bool         `json:"is_disabled"`
		Disabled   disabledInfo `json:"disabled"`
	}

	resp := map[string]queueStatus{
		"regular": {
			IsDisabled: false,
			Disabled:   disabledInfo{Until: "", Reason: ""},
		},
		"scheduled": {
			IsDisabled: false,
			Disabled:   disabledInfo{Until: "", Reason: ""},
		},
	}

	response.RespondJSON(w, http.StatusOK, resp)
}

// DeleteEnvelopes handles DELETE /v3/{domain_name}/envelopes.
func (h *Handlers) DeleteEnvelopes(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")

	// Verify domain exists
	var dl domainLookup
	if err := h.db.Where("name = ?", domainName).First(&dl).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Domain not found")
		return
	}

	response.RespondJSON(w, http.StatusOK, map[string]string{
		"message": "done",
	})
}
