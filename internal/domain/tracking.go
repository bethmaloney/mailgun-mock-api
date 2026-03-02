package domain

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/bethmaloney/mailgun-mock-api/internal/request"
	"github.com/bethmaloney/mailgun-mock-api/internal/response"
	"github.com/go-chi/chi/v5"
)

// ---------------------------------------------------------------------------
// Tracking Response DTOs
// ---------------------------------------------------------------------------

type openTrackingDTO struct {
	Active        bool `json:"active"`
	PlaceAtTheTop bool `json:"place_at_the_top"`
}

type clickTrackingDTO struct {
	Active clickActiveValue `json:"active"`
}

type unsubscribeTrackingDTO struct {
	Active     bool   `json:"active"`
	HTMLFooter string `json:"html_footer"`
	TextFooter string `json:"text_footer"`
}

type trackingDTO struct {
	Open        openTrackingDTO        `json:"open"`
	Click       clickTrackingDTO       `json:"click"`
	Unsubscribe unsubscribeTrackingDTO `json:"unsubscribe"`
}

type trackingResponseDTO struct {
	Tracking trackingDTO `json:"tracking"`
}

// clickActiveValue handles JSON serialization of the click tracking active field,
// which can be a boolean (true/false) or the string "htmlonly".
type clickActiveValue string

func (c clickActiveValue) MarshalJSON() ([]byte, error) {
	switch string(c) {
	case "true":
		return []byte("true"), nil
	case "false":
		return []byte("false"), nil
	default:
		return json.Marshal(string(c))
	}
}

// ---------------------------------------------------------------------------
// Tracking Input structs
// ---------------------------------------------------------------------------

type updateOpenTrackingInput struct {
	Active        *bool `json:"active" form:"active"`
	PlaceAtTheTop *bool `json:"place_at_the_top" form:"place_at_the_top"`
}

type updateUnsubscribeTrackingInput struct {
	Active     *bool   `json:"active" form:"active"`
	HTMLFooter *string `json:"html_footer" form:"html_footer"`
	TextFooter *string `json:"text_footer" form:"text_footer"`
}

// ---------------------------------------------------------------------------
// Helper: build tracking DTO from domain
// ---------------------------------------------------------------------------

func buildTrackingDTO(d *Domain) trackingDTO {
	return trackingDTO{
		Open: openTrackingDTO{
			Active:        d.TrackingOpenActive,
			PlaceAtTheTop: d.TrackingOpenPlaceAtTop,
		},
		Click: clickTrackingDTO{
			Active: clickActiveValue(d.TrackingClickActive),
		},
		Unsubscribe: unsubscribeTrackingDTO{
			Active:     d.TrackingUnsubscribeActive,
			HTMLFooter: d.TrackingUnsubscribeHTMLFooter,
			TextFooter: d.TrackingUnsubscribeTextFooter,
		},
	}
}

// ---------------------------------------------------------------------------
// GetTracking handles GET /v3/domains/{name}/tracking.
// ---------------------------------------------------------------------------

func (h *Handlers) GetTracking(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	var d Domain
	if err := h.db.Where("name = ?", name).First(&d).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Domain not found")
		return
	}

	response.RespondJSON(w, http.StatusOK, trackingResponseDTO{
		Tracking: buildTrackingDTO(&d),
	})
}

// ---------------------------------------------------------------------------
// UpdateOpenTracking handles PUT /v3/domains/{name}/tracking/open.
// ---------------------------------------------------------------------------

func (h *Handlers) UpdateOpenTracking(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	var d Domain
	if err := h.db.Where("name = ?", name).First(&d).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Domain not found")
		return
	}

	var input updateOpenTrackingInput
	if err := request.Parse(r, &input); err != nil {
		response.RespondError(w, http.StatusBadRequest, "Failed to parse request")
		return
	}

	if input.Active != nil {
		d.TrackingOpenActive = *input.Active
	}
	if input.PlaceAtTheTop != nil {
		d.TrackingOpenPlaceAtTop = *input.PlaceAtTheTop
	}

	if err := h.db.Save(&d).Error; err != nil {
		response.RespondError(w, http.StatusInternalServerError, "Failed to update tracking settings")
		return
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Domain tracking settings have been updated",
		"open": openTrackingDTO{
			Active:        d.TrackingOpenActive,
			PlaceAtTheTop: d.TrackingOpenPlaceAtTop,
		},
	})
}

// ---------------------------------------------------------------------------
// UpdateClickTracking handles PUT /v3/domains/{name}/tracking/click.
// ---------------------------------------------------------------------------

func (h *Handlers) UpdateClickTracking(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	var d Domain
	if err := h.db.Where("name = ?", name).First(&d).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Domain not found")
		return
	}

	// Parse active as a raw string to support "htmlonly" in addition to "true"/"false".
	activeVal := request.ParseFormValue(r, "active")
	if activeVal != "" {
		activeVal = strings.ToLower(activeVal)
		d.TrackingClickActive = activeVal
	}

	if err := h.db.Save(&d).Error; err != nil {
		response.RespondError(w, http.StatusInternalServerError, "Failed to update tracking settings")
		return
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Domain tracking settings have been updated",
		"click": clickTrackingDTO{
			Active: clickActiveValue(d.TrackingClickActive),
		},
	})
}

// ---------------------------------------------------------------------------
// UpdateUnsubscribeTracking handles PUT /v3/domains/{name}/tracking/unsubscribe.
// ---------------------------------------------------------------------------

func (h *Handlers) UpdateUnsubscribeTracking(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	var d Domain
	if err := h.db.Where("name = ?", name).First(&d).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Domain not found")
		return
	}

	var input updateUnsubscribeTrackingInput
	if err := request.Parse(r, &input); err != nil {
		response.RespondError(w, http.StatusBadRequest, "Failed to parse request")
		return
	}

	if input.Active != nil {
		d.TrackingUnsubscribeActive = *input.Active
	}
	if input.HTMLFooter != nil {
		d.TrackingUnsubscribeHTMLFooter = *input.HTMLFooter
	}
	if input.TextFooter != nil {
		d.TrackingUnsubscribeTextFooter = *input.TextFooter
	}

	if err := h.db.Save(&d).Error; err != nil {
		response.RespondError(w, http.StatusInternalServerError, "Failed to update tracking settings")
		return
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Domain tracking settings have been updated",
		"unsubscribe": unsubscribeTrackingDTO{
			Active:     d.TrackingUnsubscribeActive,
			HTMLFooter: d.TrackingUnsubscribeHTMLFooter,
			TextFooter: d.TrackingUnsubscribeTextFooter,
		},
	})
}
