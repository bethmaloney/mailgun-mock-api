package apikey

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/bethmaloney/mailgun-mock-api/internal/request"
	"github.com/bethmaloney/mailgun-mock-api/internal/response"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// ManagedHandlers holds the database connection for managed API key endpoints.
type ManagedHandlers struct {
	db *gorm.DB
}

// NewManagedHandlers creates a new ManagedHandlers instance.
func NewManagedHandlers(db *gorm.DB) *ManagedHandlers {
	return &ManagedHandlers{db: db}
}

// ResetManagedForTests deletes all managed API key data. Call this in tests
// that need a clean database state (e.g. when using a shared in-memory SQLite DB).
func ResetManagedForTests(db *gorm.DB) {
	db.Unscoped().Where("1 = 1").Delete(&ManagedAPIKey{})
}

// List handles GET requests and returns all managed API keys.
func (h *ManagedHandlers) List(w http.ResponseWriter, r *http.Request) {
	keys := make([]ManagedAPIKey, 0)
	if err := h.db.Order("created_at DESC").Find(&keys).Error; err != nil {
		response.RespondError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list managed keys: %v", err))
		return
	}
	response.RespondJSON(w, http.StatusOK, keys)
}

type createManagedKeyInput struct {
	Name string `json:"name"`
}

// Create handles POST requests and creates a new managed API key.
func (h *ManagedHandlers) Create(w http.ResponseWriter, r *http.Request) {
	var input createManagedKeyInput
	if err := request.Parse(r, &input); err != nil {
		response.RespondError(w, http.StatusBadRequest, fmt.Sprintf("failed to parse request: %v", err))
		return
	}

	if strings.TrimSpace(input.Name) == "" {
		response.RespondError(w, http.StatusBadRequest, "name is required")
		return
	}

	value, prefix, err := generateManagedKeyValue()
	if err != nil {
		response.RespondError(w, http.StatusInternalServerError, fmt.Sprintf("failed to generate key: %v", err))
		return
	}

	key := ManagedAPIKey{
		Name:     input.Name,
		KeyValue: value,
		Prefix:   prefix,
	}

	if err := h.db.Create(&key).Error; err != nil {
		// One retry on UNIQUE constraint violation only.
		if !isUniqueViolation(err) {
			response.RespondError(w, http.StatusInternalServerError, fmt.Sprintf("failed to create managed key: %v", err))
			return
		}
		value, prefix, genErr := generateManagedKeyValue()
		if genErr != nil {
			response.RespondError(w, http.StatusInternalServerError, fmt.Sprintf("failed to generate key: %v", genErr))
			return
		}
		key.KeyValue = value
		key.Prefix = prefix
		if err := h.db.Create(&key).Error; err != nil {
			response.RespondError(w, http.StatusInternalServerError, fmt.Sprintf("failed to create managed key: %v", err))
			return
		}
	}

	response.RespondJSON(w, http.StatusCreated, key)
}

// isUniqueViolation checks whether the error is a UNIQUE constraint violation.
// SQLite and Postgres surface this differently, so we check the error string.
func isUniqueViolation(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") || strings.Contains(msg, "duplicate key value")
}

// Delete handles DELETE requests and hard-deletes a managed API key by ID.
func (h *ManagedHandlers) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	result := h.db.Unscoped().Delete(&ManagedAPIKey{}, "id = ?", id)
	if result.Error != nil {
		response.RespondError(w, http.StatusInternalServerError, fmt.Sprintf("failed to delete managed key: %v", result.Error))
		return
	}
	if result.RowsAffected == 0 {
		response.RespondError(w, http.StatusNotFound, "managed key not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
