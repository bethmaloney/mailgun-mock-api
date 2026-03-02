package credential

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/bethmaloney/mailgun-mock-api/internal/database"
	"github.com/bethmaloney/mailgun-mock-api/internal/domain"
	"github.com/bethmaloney/mailgun-mock-api/internal/pagination"
	"github.com/bethmaloney/mailgun-mock-api/internal/request"
	"github.com/bethmaloney/mailgun-mock-api/internal/response"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// GORM Model
// ---------------------------------------------------------------------------

// SMTPCredential represents an SMTP credential associated with a domain.
type SMTPCredential struct {
	database.BaseModel
	DomainName string `gorm:"uniqueIndex:idx_domain_login" json:"-"`
	Login      string `gorm:"uniqueIndex:idx_domain_login" json:"login"`
	Password   string `json:"-"`
}

// ---------------------------------------------------------------------------
// DTOs
// ---------------------------------------------------------------------------

type credentialResponseDTO struct {
	CreatedAt string      `json:"created_at"`
	Login     string      `json:"login"`
	Mailbox   string      `json:"mailbox"`
	SizeBytes interface{} `json:"size_bytes"`
}

type createCredentialInput struct {
	Login    string `json:"login" form:"login"`
	Password string `json:"password" form:"password"`
}

type updateCredentialInput struct {
	Password string `json:"password" form:"password"`
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// Handlers holds the database connection needed for credential endpoints.
type Handlers struct {
	db *gorm.DB
}

// NewHandlers creates a new Handlers instance.
// It cleans up any existing SMTP credential data to ensure test isolation
// when using a shared in-memory database.
func NewHandlers(db *gorm.DB) *Handlers {
	db.Unscoped().Where("1 = 1").Delete(&SMTPCredential{})
	return &Handlers{db: db}
}

// getDomainName extracts the domain name from the URL, checking both
// "domain_name" (used in tests) and "name" (used in the server router).
func getDomainName(r *http.Request) string {
	if name := chi.URLParam(r, "domain_name"); name != "" {
		return name
	}
	return chi.URLParam(r, "name")
}

// ListCredentials handles GET /v3/domains/{domain_name}/credentials.
func (h *Handlers) ListCredentials(w http.ResponseWriter, r *http.Request) {
	domainName := getDomainName(r)

	// Verify domain exists.
	var d domain.Domain
	if err := h.db.Where("name = ?", domainName).First(&d).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Domain not found")
		return
	}

	// Parse skip/limit pagination.
	params := pagination.ParseSkipLimitParams(r, 100, 1000)

	// Get total count.
	var totalCount int64
	h.db.Model(&SMTPCredential{}).Where("domain_name = ?", domainName).Count(&totalCount)

	// Query credentials with skip/limit.
	var creds []SMTPCredential
	h.db.Where("domain_name = ?", domainName).Offset(params.Skip).Limit(params.Limit).Find(&creds)

	// Build response DTOs.
	items := make([]credentialResponseDTO, 0, len(creds))
	for _, cred := range creds {
		items = append(items, credentialResponseDTO{
			CreatedAt: cred.CreatedAt.UTC().Format(time.RFC1123Z),
			Login:     cred.Login,
			Mailbox:   cred.Login,
			SizeBytes: nil,
		})
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"total_count": totalCount,
		"items":       items,
	})
}

// CreateCredential handles POST /v3/domains/{domain_name}/credentials.
func (h *Handlers) CreateCredential(w http.ResponseWriter, r *http.Request) {
	domainName := getDomainName(r)

	// Verify domain exists.
	var d domain.Domain
	if err := h.db.Where("name = ?", domainName).First(&d).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Domain not found")
		return
	}

	// Parse request body.
	var input createCredentialInput
	if err := request.Parse(r, &input); err != nil {
		response.RespondError(w, http.StatusBadRequest, fmt.Sprintf("Failed to parse request: %v", err))
		return
	}

	// Validate login.
	if strings.TrimSpace(input.Login) == "" {
		response.RespondError(w, http.StatusBadRequest, "login is required")
		return
	}

	// Validate password length (5-32 chars).
	if len(input.Password) < 5 {
		response.RespondError(w, http.StatusBadRequest, "password must be at least 5 characters")
		return
	}
	if len(input.Password) > 32 {
		response.RespondError(w, http.StatusBadRequest, "password must be no more than 32 characters")
		return
	}

	// If login doesn't contain @, append @domain_name.
	login := input.Login
	if !strings.Contains(login, "@") {
		login = login + "@" + domainName
	}

	// Check for duplicate login within this domain.
	var existing SMTPCredential
	if err := h.db.Where("domain_name = ? AND login = ?", domainName, login).First(&existing).Error; err == nil {
		response.RespondError(w, http.StatusBadRequest, fmt.Sprintf("credential with login %q already exists", login))
		return
	}

	// Store the credential.
	cred := SMTPCredential{
		DomainName: domainName,
		Login:      login,
		Password:   input.Password,
	}
	if err := h.db.Create(&cred).Error; err != nil {
		response.RespondError(w, http.StatusInternalServerError, fmt.Sprintf("failed to create credential: %v", err))
		return
	}

	response.RespondJSON(w, http.StatusOK, map[string]string{
		"message": "Created 1 credentials pair(s)",
	})
}

// UpdateCredential handles PUT /v3/domains/{domain_name}/credentials/{spec}.
func (h *Handlers) UpdateCredential(w http.ResponseWriter, r *http.Request) {
	domainName := getDomainName(r)
	spec := chi.URLParam(r, "spec")

	// Verify domain exists.
	var d domain.Domain
	if err := h.db.Where("name = ?", domainName).First(&d).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Domain not found")
		return
	}

	// Find credential by login (spec@domain_name).
	login := spec + "@" + domainName
	var cred SMTPCredential
	if err := h.db.Where("domain_name = ? AND login = ?", domainName, login).First(&cred).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Credential not found")
		return
	}

	// Parse request body.
	var input updateCredentialInput
	if err := request.Parse(r, &input); err != nil {
		response.RespondError(w, http.StatusBadRequest, fmt.Sprintf("Failed to parse request: %v", err))
		return
	}

	// Validate password length (5-32 chars).
	if len(input.Password) < 5 {
		response.RespondError(w, http.StatusBadRequest, "password must be at least 5 characters")
		return
	}
	if len(input.Password) > 32 {
		response.RespondError(w, http.StatusBadRequest, "password must be no more than 32 characters")
		return
	}

	// Update the password.
	cred.Password = input.Password
	if err := h.db.Save(&cred).Error; err != nil {
		response.RespondError(w, http.StatusInternalServerError, fmt.Sprintf("failed to update credential: %v", err))
		return
	}

	response.RespondJSON(w, http.StatusOK, map[string]string{
		"message": "Password changed",
	})
}

// DeleteCredential handles DELETE /v3/domains/{domain_name}/credentials/{spec}.
func (h *Handlers) DeleteCredential(w http.ResponseWriter, r *http.Request) {
	domainName := getDomainName(r)
	spec := chi.URLParam(r, "spec")

	// Verify domain exists.
	var d domain.Domain
	if err := h.db.Where("name = ?", domainName).First(&d).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Domain not found")
		return
	}

	// Find credential by login (spec@domain_name).
	login := spec + "@" + domainName
	var cred SMTPCredential
	if err := h.db.Where("domain_name = ? AND login = ?", domainName, login).First(&cred).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Credential not found")
		return
	}

	// Hard delete (bypassing soft delete).
	if err := h.db.Unscoped().Delete(&cred).Error; err != nil {
		response.RespondError(w, http.StatusInternalServerError, fmt.Sprintf("failed to delete credential: %v", err))
		return
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Credentials have been deleted",
		"spec":    login,
	})
}

// DeleteAllCredentials handles DELETE /v3/domains/{domain_name}/credentials.
func (h *Handlers) DeleteAllCredentials(w http.ResponseWriter, r *http.Request) {
	domainName := getDomainName(r)

	// Verify domain exists.
	var d domain.Domain
	if err := h.db.Where("name = ?", domainName).First(&d).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Domain not found")
		return
	}

	// Delete all credentials for this domain (hard delete).
	result := h.db.Unscoped().Where("domain_name = ?", domainName).Delete(&SMTPCredential{})
	count := result.RowsAffected

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "All domain credentials have been deleted",
		"count":   count,
	})
}
