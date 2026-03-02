package apikey

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/bethmaloney/mailgun-mock-api/internal/database"
	"github.com/bethmaloney/mailgun-mock-api/internal/request"
	"github.com/bethmaloney/mailgun-mock-api/internal/response"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// APIKey represents an API key for authenticating with the Mailgun API.
type APIKey struct {
	database.BaseModel
	Description    string     `gorm:"" json:"description"`
	Kind           string     `gorm:"" json:"kind"`
	Role           string     `gorm:"" json:"role"`
	Secret         string     `gorm:"" json:"-"`
	ExpiresAt      *time.Time `gorm:"" json:"expires_at"`
	IsDisabled     bool       `gorm:"" json:"is_disabled"`
	DisabledReason *string    `gorm:"" json:"disabled_reason"`
	DomainName     *string    `gorm:"" json:"domain_name"`
	Requestor      *string    `gorm:"" json:"requestor"`
	UserName       *string    `gorm:"" json:"user_name"`
}

// ---------------------------------------------------------------------------
// DTOs
// ---------------------------------------------------------------------------

// keyResponseDTO is used for list responses (no secret).
type keyResponseDTO struct {
	ID             string  `json:"id"`
	Description    string  `json:"description"`
	Kind           string  `json:"kind"`
	Role           string  `json:"role"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
	ExpiresAt      *string `json:"expires_at"`
	IsDisabled     bool    `json:"is_disabled"`
	DisabledReason *string `json:"disabled_reason"`
	DomainName     *string `json:"domain_name"`
	Requestor      *string `json:"requestor"`
	UserName       *string `json:"user_name"`
}

// keyWithSecretDTO extends keyResponseDTO and includes the secret.
type keyWithSecretDTO struct {
	keyResponseDTO
	Secret string `json:"secret"`
}

// toResponseDTO converts an APIKey model to a keyResponseDTO.
func toResponseDTO(key APIKey) keyResponseDTO {
	dto := keyResponseDTO{
		ID:             key.ID,
		Description:    key.Description,
		Kind:           key.Kind,
		Role:           key.Role,
		CreatedAt:      key.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:      key.UpdatedAt.UTC().Format(time.RFC3339),
		IsDisabled:     key.IsDisabled,
		DisabledReason: key.DisabledReason,
		DomainName:     key.DomainName,
		Requestor:      key.Requestor,
		UserName:       key.UserName,
	}
	if key.ExpiresAt != nil {
		formatted := key.ExpiresAt.UTC().Format(time.RFC3339)
		dto.ExpiresAt = &formatted
	}
	return dto
}

// toWithSecretDTO converts an APIKey model to a keyWithSecretDTO (includes secret).
func toWithSecretDTO(key APIKey) keyWithSecretDTO {
	return keyWithSecretDTO{
		keyResponseDTO: toResponseDTO(key),
		Secret:         key.Secret,
	}
}

// ---------------------------------------------------------------------------
// Input struct
// ---------------------------------------------------------------------------

type createKeyInput struct {
	Role        string `json:"role" form:"role"`
	Description string `json:"description" form:"description"`
	Kind        string `json:"kind" form:"kind"`
	DomainName  string `json:"domain_name" form:"domain_name"`
	Expiration  int    `json:"expiration" form:"expiration"`
	UserName    string `json:"user_name" form:"user_name"`
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// Handlers holds the database connection needed for API key endpoints.
type Handlers struct {
	db        *gorm.DB
	publicKey string
}

// NewHandlers creates a new Handlers instance.
// It cleans up any existing APIKey data to ensure test isolation
// when using a shared in-memory database.
func NewHandlers(db *gorm.DB) *Handlers {
	db.Unscoped().Where("1 = 1").Delete(&APIKey{})
	return &Handlers{
		db:        db,
		publicKey: generateSecret("pubkey-"),
	}
}

// validRoles is the set of allowed role values.
var validRoles = map[string]bool{
	"admin":     true,
	"basic":     true,
	"sending":   true,
	"developer": true,
}

// generateSecret generates a random secret with the given prefix.
// It uses crypto/rand to generate 24 random bytes and hex-encodes them
// to produce 48 hex characters after the prefix.
func generateSecret(prefix string) string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand.Read failed: %v", err))
	}
	return prefix + hex.EncodeToString(b)
}

// ListKeys handles GET /v1/keys.
func (h *Handlers) ListKeys(w http.ResponseWriter, r *http.Request) {
	query := h.db.Model(&APIKey{})

	if kind := r.URL.Query().Get("kind"); kind != "" {
		query = query.Where("kind = ?", kind)
	}
	if domainName := r.URL.Query().Get("domain_name"); domainName != "" {
		query = query.Where("domain_name = ?", domainName)
	}

	var keys []APIKey
	if err := query.Find(&keys).Error; err != nil {
		response.RespondError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list keys: %v", err))
		return
	}

	items := make([]keyResponseDTO, 0, len(keys))
	for _, key := range keys {
		items = append(items, toResponseDTO(key))
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"total_count": len(items),
		"items":       items,
	})
}

// CreateKey handles POST /v1/keys.
func (h *Handlers) CreateKey(w http.ResponseWriter, r *http.Request) {
	var input createKeyInput
	if err := request.Parse(r, &input); err != nil {
		response.RespondError(w, http.StatusBadRequest, fmt.Sprintf("Failed to parse request: %v", err))
		return
	}

	// Validate role.
	if input.Role == "" {
		response.RespondError(w, http.StatusBadRequest, "role is required")
		return
	}
	if !validRoles[input.Role] {
		response.RespondError(w, http.StatusBadRequest, fmt.Sprintf("invalid role %q: must be one of admin, basic, sending, developer", input.Role))
		return
	}

	// Default kind to "user" if not specified.
	if input.Kind == "" {
		input.Kind = "user"
	}

	// Generate secret.
	secret := generateSecret("key-")

	// Build the API key model.
	key := APIKey{
		Description: input.Description,
		Kind:        input.Kind,
		Role:        input.Role,
		Secret:      secret,
	}

	// Set optional pointer fields.
	if input.DomainName != "" {
		key.DomainName = &input.DomainName
	}
	if input.UserName != "" {
		key.UserName = &input.UserName
	}

	// Compute expiration if > 0.
	if input.Expiration > 0 {
		expiresAt := time.Now().UTC().Add(time.Duration(input.Expiration) * time.Second)
		key.ExpiresAt = &expiresAt
	}

	if err := h.db.Create(&key).Error; err != nil {
		response.RespondError(w, http.StatusInternalServerError, fmt.Sprintf("failed to create key: %v", err))
		return
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "great success",
		"key":     toWithSecretDTO(key),
	})
}

// DeleteKey handles DELETE /v1/keys/{id}.
func (h *Handlers) DeleteKey(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var key APIKey
	if err := h.db.First(&key, "id = ?", id).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Key not found")
		return
	}

	if err := h.db.Unscoped().Delete(&key).Error; err != nil {
		response.RespondError(w, http.StatusInternalServerError, fmt.Sprintf("failed to delete key: %v", err))
		return
	}

	response.RespondJSON(w, http.StatusOK, map[string]string{
		"message": "key deleted",
	})
}

// RegenerateKey handles POST /v1/keys/{id}/regenerate.
func (h *Handlers) RegenerateKey(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var key APIKey
	if err := h.db.First(&key, "id = ?", id).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Key not found")
		return
	}

	// Generate a new secret.
	key.Secret = generateSecret("key-")

	if err := h.db.Save(&key).Error; err != nil {
		response.RespondError(w, http.StatusInternalServerError, fmt.Sprintf("failed to regenerate key: %v", err))
		return
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "key regenerated",
		"key":     toWithSecretDTO(key),
	})
}

// GetPublicKey handles GET /v1/keys/public.
func (h *Handlers) GetPublicKey(w http.ResponseWriter, r *http.Request) {
	response.RespondJSON(w, http.StatusOK, map[string]string{
		"key":     h.publicKey,
		"message": "public key",
	})
}
