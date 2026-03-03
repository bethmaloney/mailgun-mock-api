package tag

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/bethmaloney/mailgun-mock-api/internal/database"
	"github.com/bethmaloney/mailgun-mock-api/internal/pagination"
	"github.com/bethmaloney/mailgun-mock-api/internal/response"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// GORM Model
// ---------------------------------------------------------------------------

// Tag represents a message tag associated with a domain.
type Tag struct {
	database.BaseModel
	DomainName  string     `gorm:"index;uniqueIndex:idx_tag_domain_name"`
	Tag         string     `gorm:"uniqueIndex:idx_tag_domain_name"`
	Description string
	FirstSeen   *time.Time
	LastSeen    *time.Time
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func baseURL(r *http.Request, path string) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s%s", scheme, r.Host, path)
}

// tagMap builds a JSON-serialisable map for a tag with hyphenated keys.
func tagMap(t *Tag) map[string]interface{} {
	m := map[string]interface{}{
		"tag":         t.Tag,
		"description": t.Description,
	}

	if t.FirstSeen != nil {
		m["first-seen"] = t.FirstSeen.Format(time.RFC3339)
	} else {
		m["first-seen"] = nil
	}

	if t.LastSeen != nil {
		m["last-seen"] = t.LastSeen.Format(time.RFC3339)
	} else {
		m["last-seen"] = nil
	}

	return m
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// Handlers provides HTTP handlers for tag endpoints.
type Handlers struct {
	db *gorm.DB
}

// NewHandlers creates a new Handlers instance.
func NewHandlers(db *gorm.DB) *Handlers {
	return &Handlers{db: db}
}

// ---------------------------------------------------------------------------
// Tag CRUD
// ---------------------------------------------------------------------------

// ListTags handles GET /v3/{domain_name}/tags.
func (h *Handlers) ListTags(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")
	cp := pagination.ParseCursorParams(r)
	prefix := r.URL.Query().Get("prefix")

	query := h.db.Where("domain_name = ?", domainName)

	// Apply prefix filter
	if prefix != "" {
		query = query.Where("tag LIKE ?", prefix+"%")
	}

	// Apply cursor pagination
	if cp.Pivot != "" {
		cursorData, err := pagination.DecodeCursor(cp.Pivot)
		if err == nil {
			if tagName := cursorData["tag"]; tagName != "" {
				switch cp.Page {
				case "next":
					query = query.Where("tag > ?", tagName)
				case "prev":
					query = query.Where("tag < ?", tagName)
				}
			}
		}
	}

	if cp.Page == "last" {
		query = query.Order("tag DESC")
	} else {
		query = query.Order("tag ASC")
	}

	var tags []Tag
	query.Limit(cp.Limit + 1).Find(&tags)

	hasMore := len(tags) > cp.Limit
	if hasMore {
		tags = tags[:cp.Limit]
	}

	if cp.Page == "prev" || cp.Page == "last" {
		for i, j := 0, len(tags)-1; i < j; i, j = i+1, j-1 {
			tags[i], tags[j] = tags[j], tags[i]
		}
	}

	items := make([]interface{}, 0, len(tags))
	var lastTagName string
	for _, t := range tags {
		items = append(items, tagMap(&t))
		lastTagName = t.Tag
	}

	var cursor string
	if lastTagName != "" {
		cursor = pagination.EncodeCursor(map[string]string{"tag": lastTagName})
	}

	paging := pagination.GeneratePagingURLs(
		baseURL(r, r.URL.Path),
		cp.Limit,
		cursor,
		hasMore,
	)

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"items":  items,
		"paging": paging,
	})
}

// GetTag handles GET /v3/{domain_name}/tags/{tag}.
func (h *Handlers) GetTag(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")
	tagName := strings.ToLower(chi.URLParam(r, "tag"))

	var t Tag
	if err := h.db.Where("domain_name = ? AND tag = ?", domainName, tagName).First(&t).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "tag not found")
		return
	}

	response.RespondJSON(w, http.StatusOK, tagMap(&t))
}

// UpdateTag handles PUT /v3/{domain_name}/tags/{tag}.
func (h *Handlers) UpdateTag(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")
	tagName := strings.ToLower(chi.URLParam(r, "tag"))

	r.ParseMultipartForm(32 << 20)

	var t Tag
	if err := h.db.Where("domain_name = ? AND tag = ?", domainName, tagName).First(&t).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "tag not found")
		return
	}

	description := r.FormValue("description")
	h.db.Model(&t).Update("description", description)

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Tag updated",
	})
}

// DeleteTag handles DELETE /v3/{domain_name}/tags/{tag}.
func (h *Handlers) DeleteTag(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")
	tagName := strings.ToLower(chi.URLParam(r, "tag"))

	var t Tag
	if err := h.db.Where("domain_name = ? AND tag = ?", domainName, tagName).First(&t).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "tag not found")
		return
	}

	// Hard delete so the tag can be re-created later
	h.db.Unscoped().Delete(&t)

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Tag has been removed",
	})
}

// GetTagLimits handles GET /v3/domains/{domain_name}/limits/tag.
func (h *Handlers) GetTagLimits(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")

	var count int64
	h.db.Model(&Tag{}).Where("domain_name = ?", domainName).Count(&count)

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"id":    domainName,
		"limit": 5000,
		"count": count,
	})
}

// ---------------------------------------------------------------------------
// Singular Path Handlers (OpenAPI spec — tag from query parameter)
// ---------------------------------------------------------------------------

// GetTagByQuery handles GET /v3/{domain_name}/tag?tag={name}.
// It reads the tag name from the ?tag= query parameter instead of a URL path segment.
func (h *Handlers) GetTagByQuery(w http.ResponseWriter, r *http.Request) {
	tagName := strings.ToLower(r.URL.Query().Get("tag"))
	if tagName == "" {
		response.RespondError(w, http.StatusBadRequest, "tag query parameter is required")
		return
	}
	domainName := chi.URLParam(r, "domain_name")

	var t Tag
	if err := h.db.Where("domain_name = ? AND tag = ?", domainName, tagName).First(&t).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "tag not found")
		return
	}

	response.RespondJSON(w, http.StatusOK, tagMap(&t))
}

// ---------------------------------------------------------------------------
// Tag Auto-creation
// ---------------------------------------------------------------------------

// EnsureTags creates or updates tags for a domain. For each tag name, if the
// tag does not exist it is created with first-seen and last-seen set to now.
// If it already exists, only last-seen is updated.
func EnsureTags(db *gorm.DB, domainName string, tags []string) {
	now := time.Now().UTC()
	for _, tagName := range tags {
		tagName = strings.ToLower(strings.TrimSpace(tagName))
		if tagName == "" {
			continue
		}
		// Try to create first
		newTag := Tag{
			DomainName: domainName,
			Tag:        tagName,
			FirstSeen:  &now,
			LastSeen:   &now,
		}
		err := db.Create(&newTag).Error
		if err != nil {
			// Unique constraint violation — tag already exists, update last_seen
			db.Model(&Tag{}).Where("domain_name = ? AND tag = ?", domainName, tagName).Update("last_seen", now)
		}
	}
}
