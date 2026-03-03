package subaccount

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/bethmaloney/mailgun-mock-api/internal/database"
	"github.com/bethmaloney/mailgun-mock-api/internal/response"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// Models

// Subaccount represents a Mailgun subaccount.
type Subaccount struct {
	database.BaseModel
	SubaccountID           string `gorm:"uniqueIndex;size:24"`
	Name                   string
	Status                 string
	FeatureSending         bool
	FeatureEmailPreview    bool
	FeatureInboxPlacement  bool
	FeatureValidations     bool
	FeatureValidationsBulk bool
}

// SendingLimit represents a monthly sending limit for a subaccount.
type SendingLimit struct {
	database.BaseModel
	SubaccountID string `gorm:"uniqueIndex"`
	Limit        int
	Current      int
	Period       string
}

// Handlers holds the database connection for subaccount operations.
type Handlers struct {
	db *gorm.DB
}

// NewHandlers creates a new Handlers instance.
func NewHandlers(db *gorm.DB) *Handlers {
	return &Handlers{db: db}
}

// generateSubaccountID generates a 24-character hex string.
func generateSubaccountID() string {
	b := make([]byte, 12) // 12 bytes = 24 hex chars
	rand.Read(b)
	return hex.EncodeToString(b)
}

// toListJSON returns the subaccount as a JSON map for list responses (no features).
func (sa *Subaccount) toListJSON() map[string]interface{} {
	return map[string]interface{}{
		"id":         sa.SubaccountID,
		"name":       sa.Name,
		"status":     sa.Status,
		"created_at": sa.CreatedAt.UTC().Format(time.RFC1123),
		"updated_at": sa.UpdatedAt.UTC().Format(time.RFC1123),
	}
}

// toDetailJSON returns the subaccount as a JSON map with features (for create/get).
func (sa *Subaccount) toDetailJSON() map[string]interface{} {
	m := sa.toListJSON()
	m["features"] = sa.featuresJSON()
	return m
}

// featuresJSON returns only the features map.
func (sa *Subaccount) featuresJSON() map[string]interface{} {
	return map[string]interface{}{
		"sending":          map[string]interface{}{"enabled": sa.FeatureSending},
		"email_preview":    map[string]interface{}{"enabled": sa.FeatureEmailPreview},
		"inbox_placement":  map[string]interface{}{"enabled": sa.FeatureInboxPlacement},
		"validations":      map[string]interface{}{"enabled": sa.FeatureValidations},
		"validations_bulk": map[string]interface{}{"enabled": sa.FeatureValidationsBulk},
	}
}

// CreateSubaccount handles POST /v5/accounts/subaccounts.
func (h *Handlers) CreateSubaccount(w http.ResponseWriter, r *http.Request) {
	// Parse name from form data or query parameter
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		r.ParseForm()
	}
	name := r.FormValue("name")
	if name == "" {
		name = r.URL.Query().Get("name")
	}

	if name == "" {
		response.RespondError(w, http.StatusBadRequest, "Bad request")
		return
	}

	sa := Subaccount{
		SubaccountID:           generateSubaccountID(),
		Name:                   name,
		Status:                 "open",
		FeatureSending:         true,
		FeatureEmailPreview:    false,
		FeatureInboxPlacement:  false,
		FeatureValidations:     false,
		FeatureValidationsBulk: false,
	}

	if err := h.db.Create(&sa).Error; err != nil {
		response.RespondError(w, http.StatusInternalServerError, "Failed to create subaccount")
		return
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"subaccount": sa.toDetailJSON(),
	})
}

// ListSubaccounts handles GET /v5/accounts/subaccounts.
func (h *Handlers) ListSubaccounts(w http.ResponseWriter, r *http.Request) {
	query := h.db.Model(&Subaccount{})

	// Apply filter (LIKE partial name match)
	if filter := r.URL.Query().Get("filter"); filter != "" {
		query = query.Where("name LIKE ?", "%"+filter+"%")
	}

	// Apply enabled filter
	if enabled := r.URL.Query().Get("enabled"); enabled != "" {
		if enabled == "true" {
			query = query.Where("status = ?", "open")
		} else if enabled == "false" {
			query = query.Where("status = ?", "disabled")
		}
	}

	// Count total after filtering but before skip/limit
	var total int64
	query.Count(&total)

	// Apply sort
	sort := r.URL.Query().Get("sort")
	switch sort {
	case "asc":
		query = query.Order("name ASC")
	case "desc":
		query = query.Order("name DESC")
	default:
		query = query.Order("created_at ASC")
	}

	// Apply limit (default 10, max 1000)
	limit := 10
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
			if limit > 1000 {
				limit = 1000
			}
		}
	}
	query = query.Limit(limit)

	// Apply skip (offset)
	if skipStr := r.URL.Query().Get("skip"); skipStr != "" {
		if parsed, err := strconv.Atoi(skipStr); err == nil && parsed > 0 {
			query = query.Offset(parsed)
		}
	}

	var subaccounts []Subaccount
	query.Find(&subaccounts)

	// Build list response (no features)
	items := make([]map[string]interface{}, 0, len(subaccounts))
	for _, sa := range subaccounts {
		items = append(items, sa.toListJSON())
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"subaccounts": items,
		"total":       total,
	})
}

// GetSubaccount handles GET /v5/accounts/subaccounts/{subaccount_id}.
func (h *Handlers) GetSubaccount(w http.ResponseWriter, r *http.Request) {
	subaccountID := chi.URLParam(r, "subaccount_id")

	var sa Subaccount
	if err := h.db.Where("subaccount_id = ?", subaccountID).First(&sa).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Not Found")
		return
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"subaccount": sa.toDetailJSON(),
	})
}

// DisableSubaccount handles POST /v5/accounts/subaccounts/{subaccount_id}/disable.
func (h *Handlers) DisableSubaccount(w http.ResponseWriter, r *http.Request) {
	subaccountID := chi.URLParam(r, "subaccount_id")

	var sa Subaccount
	if err := h.db.Where("subaccount_id = ?", subaccountID).First(&sa).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Not Found")
		return
	}

	if sa.Status == "disabled" {
		response.RespondError(w, http.StatusBadRequest, "subaccount is already disabled")
		return
	}

	sa.Status = "disabled"
	h.db.Save(&sa)

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"subaccount": sa.toListJSON(),
	})
}

// EnableSubaccount handles POST /v5/accounts/subaccounts/{subaccount_id}/enable.
func (h *Handlers) EnableSubaccount(w http.ResponseWriter, r *http.Request) {
	subaccountID := chi.URLParam(r, "subaccount_id")

	var sa Subaccount
	if err := h.db.Where("subaccount_id = ?", subaccountID).First(&sa).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Not Found")
		return
	}

	sa.Status = "open"
	h.db.Save(&sa)

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"subaccount": sa.toListJSON(),
	})
}

// DeleteSubaccount handles DELETE /v5/accounts/subaccounts.
// Uses X-Mailgun-On-Behalf-Of header to identify which subaccount to delete.
func (h *Handlers) DeleteSubaccount(w http.ResponseWriter, r *http.Request) {
	subaccountID := r.Header.Get("X-Mailgun-On-Behalf-Of")
	if subaccountID == "" {
		response.RespondError(w, http.StatusBadRequest, "Bad request")
		return
	}

	var sa Subaccount
	if err := h.db.Where("subaccount_id = ?", subaccountID).First(&sa).Error; err != nil {
		response.RespondError(w, http.StatusBadRequest, "Bad request")
		return
	}

	h.db.Unscoped().Delete(&sa)

	response.RespondSuccess(w, "Subaccount successfully deleted")
}

// GetSendingLimit handles GET /v5/accounts/subaccounts/{subaccount_id}/limit/custom/monthly.
func (h *Handlers) GetSendingLimit(w http.ResponseWriter, r *http.Request) {
	subaccountID := chi.URLParam(r, "subaccount_id")

	// Verify subaccount exists
	var sa Subaccount
	if err := h.db.Where("subaccount_id = ?", subaccountID).First(&sa).Error; err != nil {
		response.RespondError(w, http.StatusBadRequest, "Not a subaccount")
		return
	}

	var limit SendingLimit
	if err := h.db.Where("subaccount_id = ?", subaccountID).First(&limit).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "No threshold for account")
		return
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"limit":   limit.Limit,
		"current": limit.Current,
		"period":  limit.Period,
	})
}

// SetSendingLimit handles PUT /v5/accounts/subaccounts/{subaccount_id}/limit/custom/monthly.
func (h *Handlers) SetSendingLimit(w http.ResponseWriter, r *http.Request) {
	subaccountID := chi.URLParam(r, "subaccount_id")

	// Parse limit from query parameter
	limitStr := r.URL.Query().Get("limit")
	limitVal, err := strconv.Atoi(limitStr)
	if err != nil {
		response.RespondError(w, http.StatusBadRequest, "Invalid limit value")
		return
	}

	// Verify subaccount exists
	var sa Subaccount
	if err := h.db.Where("subaccount_id = ?", subaccountID).First(&sa).Error; err != nil {
		response.RespondError(w, http.StatusBadRequest, "Invalid subaccount ID")
		return
	}

	// Upsert: create or update the SendingLimit record
	var sl SendingLimit
	result := h.db.Where("subaccount_id = ?", subaccountID).First(&sl)
	if result.Error != nil {
		// Create new
		sl = SendingLimit{
			SubaccountID: subaccountID,
			Limit:        limitVal,
			Current:      0,
			Period:       "1m",
		}
		h.db.Create(&sl)
	} else {
		// Update existing
		sl.Limit = limitVal
		h.db.Save(&sl)
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

// RemoveSendingLimit handles DELETE /v5/accounts/subaccounts/{subaccount_id}/limit/custom/monthly.
func (h *Handlers) RemoveSendingLimit(w http.ResponseWriter, r *http.Request) {
	subaccountID := chi.URLParam(r, "subaccount_id")

	var sl SendingLimit
	if err := h.db.Where("subaccount_id = ?", subaccountID).First(&sl).Error; err != nil {
		response.RespondError(w, http.StatusBadRequest, "Could not delete threshold for account")
		return
	}

	h.db.Unscoped().Delete(&sl)

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

// UpdateFeatures handles PUT /v5/accounts/subaccounts/{subaccount_id}/features.
func (h *Handlers) UpdateFeatures(w http.ResponseWriter, r *http.Request) {
	subaccountID := chi.URLParam(r, "subaccount_id")

	var sa Subaccount
	if err := h.db.Where("subaccount_id = ?", subaccountID).First(&sa).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Not Found")
		return
	}

	// Parse form body
	if err := r.ParseForm(); err != nil {
		response.RespondError(w, http.StatusBadRequest, "Bad request")
		return
	}

	validKeys := map[string]bool{
		"email_preview":    true,
		"inbox_placement":  true,
		"sending":          true,
		"validations":      true,
		"validations_bulk": true,
	}

	type featureVal struct {
		Enabled bool `json:"enabled"`
	}

	updated := false
	for key := range r.Form {
		if !validKeys[key] {
			continue
		}

		var fv featureVal
		if err := json.Unmarshal([]byte(r.FormValue(key)), &fv); err != nil {
			continue
		}

		switch key {
		case "email_preview":
			sa.FeatureEmailPreview = fv.Enabled
		case "inbox_placement":
			sa.FeatureInboxPlacement = fv.Enabled
		case "sending":
			sa.FeatureSending = fv.Enabled
		case "validations":
			sa.FeatureValidations = fv.Enabled
		case "validations_bulk":
			sa.FeatureValidationsBulk = fv.Enabled
		}
		updated = true
	}

	if !updated {
		response.RespondError(w, http.StatusBadRequest, "No valid updates provided")
		return
	}

	h.db.Save(&sa)

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"features": sa.featuresJSON(),
	})
}
