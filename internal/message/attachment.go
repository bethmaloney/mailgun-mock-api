package message

import (
	"net/http"

	"github.com/bethmaloney/mailgun-mock-api/internal/database"
	"github.com/bethmaloney/mailgun-mock-api/internal/response"
	"github.com/go-chi/chi/v5"
)

// Attachment stores the actual file bytes for a message attachment or inline image.
type Attachment struct {
	database.BaseModel
	StoredMessageID string `gorm:"index"`  // FK to StoredMessage
	FileName        string
	ContentType     string
	Size            int
	Content         []byte // actual file bytes
	Inline          bool   // true for inline, false for attachment
}

// GetAttachment handles GET /v3/domains/{domain_name}/messages/{storage_key}/attachments/{attachment_id}.
func (h *Handlers) GetAttachment(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")
	storageKey := chi.URLParam(r, "storage_key")
	attachmentID := chi.URLParam(r, "attachment_id")

	var msg StoredMessage
	if err := h.db.Where("storage_key = ? AND domain_name = ?", storageKey, domainName).First(&msg).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Message not found")
		return
	}

	var att Attachment
	if err := h.db.Where("id = ? AND stored_message_id = ?", attachmentID, msg.ID).First(&att).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Attachment not found")
		return
	}

	w.Header().Set("Content-Type", att.ContentType)
	w.WriteHeader(http.StatusOK)
	w.Write(att.Content)
}
