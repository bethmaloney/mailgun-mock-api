package tag

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/bethmaloney/mailgun-mock-api/internal/response"
)

// ---------------------------------------------------------------------------
// v1 Analytics Tags API (account-level, not domain-scoped)
// ---------------------------------------------------------------------------

// v1ListRequest is the JSON body for POST /v1/analytics/tags.
type v1ListRequest struct {
	Tag        string `json:"tag"`
	Pagination struct {
		Skip         int    `json:"skip"`
		Limit        int    `json:"limit"`
		IncludeTotal bool   `json:"include_total"`
		Sort         string `json:"sort"`
	} `json:"pagination"`
}

// v1TagItem builds a v1-style tag item with snake_case fields.
func v1TagItem(t *Tag) map[string]interface{} {
	m := map[string]interface{}{
		"tag":               t.Tag,
		"description":       t.Description,
		"account_id":        "",
		"parent_account_id": "",
		"account_name":      "",
		"metrics":           map[string]interface{}{},
	}

	if t.FirstSeen != nil {
		m["first_seen"] = t.FirstSeen.Format(time.RFC3339)
	} else {
		m["first_seen"] = nil
	}

	if t.LastSeen != nil {
		m["last_seen"] = t.LastSeen.Format(time.RFC3339)
	} else {
		m["last_seen"] = nil
	}

	return m
}

// V1ListTags handles POST /v1/analytics/tags.
// Lists/searches tags across all domains (account-level).
func (h *Handlers) V1ListTags(w http.ResponseWriter, r *http.Request) {
	var req v1ListRequest
	if r.Body != nil {
		json.NewDecoder(r.Body).Decode(&req)
	}

	// Defaults
	limit := req.Pagination.Limit
	if limit <= 0 {
		limit = 100
	}
	skip := req.Pagination.Skip
	if skip < 0 {
		skip = 0
	}
	sort := req.Pagination.Sort
	if sort == "" {
		sort = "tag"
	}

	query := h.db.Model(&Tag{})

	// Apply tag filter (prefix match)
	if req.Tag != "" {
		query = query.Where("tag LIKE ?", strings.ToLower(req.Tag)+"%")
	}

	// Count total if requested
	var total int64
	if req.Pagination.IncludeTotal {
		query.Count(&total)
	}

	// Apply ordering, skip, limit
	query = query.Order("tag ASC").Offset(skip).Limit(limit)

	var tags []Tag
	query.Find(&tags)

	items := make([]map[string]interface{}, 0, len(tags))
	for _, t := range tags {
		items = append(items, v1TagItem(&t))
	}

	paginationResp := map[string]interface{}{
		"sort":  sort,
		"skip":  skip,
		"limit": limit,
	}
	if req.Pagination.IncludeTotal {
		paginationResp["total"] = total
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"items":      items,
		"pagination": paginationResp,
	})
}

// v1ModifyRequest is the JSON body for PUT/DELETE /v1/analytics/tags.
type v1ModifyRequest struct {
	Tag         string `json:"tag"`
	Description string `json:"description"`
}

// V1UpdateTag handles PUT /v1/analytics/tags.
// Updates a tag by name across all domains (account-level).
func (h *Handlers) V1UpdateTag(w http.ResponseWriter, r *http.Request) {
	var req v1ModifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	tagName := strings.ToLower(req.Tag)

	// Find all matching tags across all domains
	var tags []Tag
	if err := h.db.Where("tag = ?", tagName).Find(&tags).Error; err != nil || len(tags) == 0 {
		response.RespondError(w, http.StatusNotFound, "tag not found")
		return
	}

	// Update description for all matching tags
	h.db.Model(&Tag{}).Where("tag = ?", tagName).Update("description", req.Description)

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Tag updated",
	})
}

// V1DeleteTag handles DELETE /v1/analytics/tags.
// Deletes a tag by name across all domains (account-level).
func (h *Handlers) V1DeleteTag(w http.ResponseWriter, r *http.Request) {
	var req v1ModifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	tagName := strings.ToLower(req.Tag)

	// Check if the tag exists
	var tags []Tag
	if err := h.db.Where("tag = ?", tagName).Find(&tags).Error; err != nil || len(tags) == 0 {
		response.RespondError(w, http.StatusNotFound, "tag not found")
		return
	}

	// Hard delete all matching tags across all domains
	h.db.Unscoped().Where("tag = ?", tagName).Delete(&Tag{})

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Tag has been removed",
	})
}

// V1GetTagLimits handles GET /v1/analytics/tags/limits.
// Returns account-wide tag limits (across all domains).
func (h *Handlers) V1GetTagLimits(w http.ResponseWriter, r *http.Request) {
	var count int64
	h.db.Model(&Tag{}).Count(&count)

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"limit":         5000,
		"count":         count,
		"limit_reached": count >= 5000,
	})
}
