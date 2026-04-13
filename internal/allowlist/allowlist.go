package allowlist

import (
	"fmt"
	"net"
	"net/http"

	"github.com/bethmaloney/mailgun-mock-api/internal/database"
	"github.com/bethmaloney/mailgun-mock-api/internal/request"
	"github.com/bethmaloney/mailgun-mock-api/internal/response"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// GORM Model
// ---------------------------------------------------------------------------

// IPAllowlistEntry represents an IP address or CIDR block in the allowlist.
type IPAllowlistEntry struct {
	database.BaseModel
	IPAddress   string `gorm:"uniqueIndex" json:"ip_address"`
	Description string `json:"description"`
}

// ---------------------------------------------------------------------------
// DTOs
// ---------------------------------------------------------------------------

type entryDTO struct {
	IPAddress   string `json:"ip_address"`
	Description string `json:"description"`
}

// ---------------------------------------------------------------------------
// Input structs
// ---------------------------------------------------------------------------

type addEntryInput struct {
	Address     string `json:"address" form:"address"`
	Description string `json:"description" form:"description"`
}

type updateEntryInput struct {
	Address     string `json:"address" form:"address"`
	Description string `json:"description" form:"description"`
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// Handlers holds the database connection needed for IP allowlist endpoints.
type Handlers struct {
	db *gorm.DB
}

// NewHandlers creates a new Handlers instance.
func NewHandlers(db *gorm.DB) *Handlers {
	return &Handlers{db: db}
}

// ResetForTests deletes all IP allowlist data. Call this in tests that need a
// clean database state (e.g. when using a shared in-memory SQLite DB).
func ResetForTests(db *gorm.DB) {
	db.Unscoped().Where("1 = 1").Delete(&IPAllowlistEntry{})
}

// isValidIPOrCIDR checks whether the given address is a valid IP or CIDR notation.
func isValidIPOrCIDR(address string) bool {
	// Check if it's a valid IP
	if ip := net.ParseIP(address); ip != nil {
		return true
	}
	// Check if it's a valid CIDR
	if _, _, err := net.ParseCIDR(address); err == nil {
		return true
	}
	return false
}

// respondWithAllEntries queries all entries and writes them as JSON.
func (h *Handlers) respondWithAllEntries(w http.ResponseWriter) {
	var entries []IPAllowlistEntry
	if err := h.db.Order("created_at ASC").Find(&entries).Error; err != nil {
		response.RespondError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list entries: %v", err))
		return
	}

	items := make([]entryDTO, 0, len(entries))
	for _, e := range entries {
		items = append(items, entryDTO{
			IPAddress:   e.IPAddress,
			Description: e.Description,
		})
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"addresses": items,
	})
}

// ListEntries handles GET /v2/ip_whitelist.
func (h *Handlers) ListEntries(w http.ResponseWriter, r *http.Request) {
	h.respondWithAllEntries(w)
}

// AddEntry handles POST /v2/ip_whitelist.
func (h *Handlers) AddEntry(w http.ResponseWriter, r *http.Request) {
	var input addEntryInput
	if err := request.Parse(r, &input); err != nil {
		response.RespondError(w, http.StatusBadRequest, fmt.Sprintf("Failed to parse request: %v", err))
		return
	}

	if input.Address == "" {
		response.RespondError(w, http.StatusBadRequest, "Invalid IP Address or CIDR")
		return
	}

	if !isValidIPOrCIDR(input.Address) {
		response.RespondError(w, http.StatusBadRequest, "Invalid IP Address or CIDR")
		return
	}

	// Check for duplicate
	var existing IPAllowlistEntry
	if err := h.db.Where("ip_address = ?", input.Address).First(&existing).Error; err == nil {
		// Entry already exists — return current list (idempotent)
		h.respondWithAllEntries(w)
		return
	}

	entry := IPAllowlistEntry{
		IPAddress:   input.Address,
		Description: input.Description,
	}
	if err := h.db.Create(&entry).Error; err != nil {
		response.RespondError(w, http.StatusInternalServerError, fmt.Sprintf("failed to create entry: %v", err))
		return
	}

	h.respondWithAllEntries(w)
}

// UpdateEntry handles PUT /v2/ip_whitelist.
func (h *Handlers) UpdateEntry(w http.ResponseWriter, r *http.Request) {
	var input updateEntryInput
	if err := request.Parse(r, &input); err != nil {
		response.RespondError(w, http.StatusBadRequest, fmt.Sprintf("Failed to parse request: %v", err))
		return
	}

	var entry IPAllowlistEntry
	if err := h.db.Where("ip_address = ?", input.Address).First(&entry).Error; err != nil {
		response.RespondError(w, http.StatusBadRequest, "IP not found")
		return
	}

	entry.Description = input.Description
	if err := h.db.Save(&entry).Error; err != nil {
		response.RespondError(w, http.StatusInternalServerError, fmt.Sprintf("failed to update entry: %v", err))
		return
	}

	h.respondWithAllEntries(w)
}

// DeleteEntry handles DELETE /v2/ip_whitelist.
func (h *Handlers) DeleteEntry(w http.ResponseWriter, r *http.Request) {
	address := r.URL.Query().Get("address")

	var entry IPAllowlistEntry
	if err := h.db.Where("ip_address = ?", address).First(&entry).Error; err != nil {
		response.RespondError(w, http.StatusBadRequest, "IP not found")
		return
	}

	if err := h.db.Unscoped().Delete(&entry).Error; err != nil {
		response.RespondError(w, http.StatusInternalServerError, fmt.Sprintf("failed to delete entry: %v", err))
		return
	}

	h.respondWithAllEntries(w)
}
