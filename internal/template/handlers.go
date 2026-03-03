package template

import (
	"encoding/json"
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
// GORM Models
// ---------------------------------------------------------------------------

// Template represents a stored message template.
type Template struct {
	database.BaseModel
	DomainName  string `gorm:"index;uniqueIndex:idx_template_domain_name"`
	Name        string `gorm:"uniqueIndex:idx_template_domain_name"`
	Description string
	CreatedBy   string
}

// TemplateVersion represents a version of a template.
type TemplateVersion struct {
	database.BaseModel
	TemplateID string `gorm:"index;uniqueIndex:idx_version_template_tag"`
	Tag        string `gorm:"uniqueIndex:idx_version_template_tag"`
	Template   string // template content
	Engine     string `gorm:"default:handlebars"`
	Mjml       string
	Comment    string
	Active     bool
	Headers    string // JSON-encoded headers
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const rfc2822 = "Mon, 02 Jan 2006 15:04:05 MST"

func formatTime(t time.Time) string {
	return t.UTC().Format(rfc2822)
}

func baseURL(r *http.Request, path string) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s%s", scheme, r.Host, path)
}

// templateMap builds the JSON-serialisable map for a template (without version).
func templateMap(t *Template) map[string]interface{} {
	return map[string]interface{}{
		"name":        t.Name,
		"description": t.Description,
		"createdAt":   formatTime(t.CreatedAt),
		"createdBy":   t.CreatedBy,
		"id":          t.ID,
	}
}

// versionMap builds the full JSON-serialisable map for a version (with content).
func versionMap(v *TemplateVersion) map[string]interface{} {
	m := map[string]interface{}{
		"tag":       v.Tag,
		"template":  v.Template,
		"engine":    v.Engine,
		"mjml":      v.Mjml,
		"createdAt": formatTime(v.CreatedAt),
		"comment":   v.Comment,
		"active":    v.Active,
		"id":        v.ID,
	}
	if v.Headers != "" {
		var h map[string]interface{}
		if err := json.Unmarshal([]byte(v.Headers), &h); err == nil {
			m["headers"] = h
		}
	}
	return m
}

// versionMetaMap builds a version map WITHOUT the template content field
// (used in list-versions responses).
func versionMetaMap(v *TemplateVersion) map[string]interface{} {
	return map[string]interface{}{
		"tag":       v.Tag,
		"engine":    v.Engine,
		"mjml":      v.Mjml,
		"createdAt": formatTime(v.CreatedAt),
		"comment":   v.Comment,
		"active":    v.Active,
		"id":        v.ID,
	}
}

// findTemplate looks up a template by domain and name.
func (h *Handlers) findTemplate(domainName, name string) (*Template, error) {
	var tmpl Template
	err := h.db.Where("domain_name = ? AND name = ?", domainName, name).First(&tmpl).Error
	if err != nil {
		return nil, err
	}
	return &tmpl, nil
}

// findVersion looks up a version by template ID and tag.
func (h *Handlers) findVersion(templateID, tag string) (*TemplateVersion, error) {
	var ver TemplateVersion
	err := h.db.Where("template_id = ? AND tag = ?", templateID, tag).First(&ver).Error
	if err != nil {
		return nil, err
	}
	return &ver, nil
}

// activeVersion returns the active version for a template, or nil.
func (h *Handlers) activeVersion(templateID string) *TemplateVersion {
	var ver TemplateVersion
	err := h.db.Where("template_id = ? AND active = ?", templateID, true).First(&ver).Error
	if err != nil {
		return nil
	}
	return &ver
}

// deactivateAll sets active=false for all versions of a template.
func (h *Handlers) deactivateAll(templateID string) {
	h.db.Model(&TemplateVersion{}).Where("template_id = ? AND active = ?", templateID, true).Update("active", false)
}

// versionCount returns the number of versions for a template.
func (h *Handlers) versionCount(templateID string) int64 {
	var count int64
	h.db.Model(&TemplateVersion{}).Where("template_id = ?", templateID).Count(&count)
	return count
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// Handlers provides HTTP handlers for template endpoints.
type Handlers struct {
	db *gorm.DB
}

// NewHandlers creates a new Handlers instance.
func NewHandlers(db *gorm.DB) *Handlers {
	return &Handlers{db: db}
}

// ---------------------------------------------------------------------------
// Template CRUD
// ---------------------------------------------------------------------------

// CreateTemplate handles POST /v3/{domain_name}/templates.
func (h *Handlers) CreateTemplate(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")
	r.ParseMultipartForm(32 << 20)

	name := strings.ToLower(r.FormValue("name"))
	if name == "" {
		response.RespondError(w, http.StatusBadRequest, "name is required")
		return
	}

	description := r.FormValue("description")
	createdBy := r.FormValue("createdBy")

	// Check max templates per domain.
	var count int64
	h.db.Model(&Template{}).Where("domain_name = ?", domainName).Count(&count)
	if count >= 100 {
		response.RespondError(w, http.StatusBadRequest, "maximum number of templates reached")
		return
	}

	// Check for duplicate.
	var existing Template
	if err := h.db.Where("domain_name = ? AND name = ?", domainName, name).First(&existing).Error; err == nil {
		response.RespondError(w, http.StatusBadRequest, fmt.Sprintf("template with name '%s' already exists", name))
		return
	}

	tmpl := Template{
		DomainName:  domainName,
		Name:        name,
		Description: description,
		CreatedBy:   createdBy,
	}
	if err := h.db.Create(&tmpl).Error; err != nil {
		response.RespondError(w, http.StatusInternalServerError, "failed to create template")
		return
	}

	result := templateMap(&tmpl)

	// If template content is provided, create initial version.
	templateContent := r.FormValue("template")
	if templateContent != "" {
		tag := strings.ToLower(r.FormValue("tag"))
		if tag == "" {
			tag = "initial"
		}
		engine := r.FormValue("engine")
		if engine == "" {
			engine = "handlebars"
		}
		comment := r.FormValue("comment")
		headers := r.FormValue("headers")

		ver := TemplateVersion{
			TemplateID: tmpl.ID,
			Tag:        tag,
			Template:   templateContent,
			Engine:     engine,
			Comment:    comment,
			Active:     true,
			Headers:    headers,
		}
		if err := h.db.Create(&ver).Error; err != nil {
			response.RespondError(w, http.StatusInternalServerError, "failed to create initial version")
			return
		}
		result["version"] = versionMap(&ver)
	} else {
		result["version"] = nil
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"message":  "template has been stored",
		"template": result,
	})
}

// ListTemplates handles GET /v3/{domain_name}/templates.
func (h *Handlers) ListTemplates(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")
	cp := pagination.ParseCursorParams(r)
	activeFlag := r.URL.Query().Get("active") == "yes"

	query := h.db.Where("domain_name = ?", domainName)

	// Apply cursor pagination.
	if cp.Pivot != "" {
		cursorData, err := pagination.DecodeCursor(cp.Pivot)
		if err == nil {
			if name := cursorData["name"]; name != "" {
				switch cp.Page {
				case "next":
					query = query.Where("name > ?", name)
				case "prev":
					query = query.Where("name < ?", name)
				}
			}
		}
	}

	if cp.Page == "last" {
		query = query.Order("name DESC")
	} else {
		query = query.Order("name ASC")
	}

	var templates []Template
	query.Limit(cp.Limit + 1).Find(&templates)

	hasMore := len(templates) > cp.Limit
	if hasMore {
		templates = templates[:cp.Limit]
	}

	if cp.Page == "prev" || cp.Page == "last" {
		for i, j := 0, len(templates)-1; i < j; i, j = i+1, j-1 {
			templates[i], templates[j] = templates[j], templates[i]
		}
	}

	items := make([]interface{}, 0, len(templates))
	var lastName string
	for _, t := range templates {
		item := templateMap(&t)
		if activeFlag {
			if av := h.activeVersion(t.ID); av != nil {
				item["version"] = versionMap(av)
			} else {
				item["version"] = nil
			}
		}
		items = append(items, item)
		lastName = t.Name
	}

	var cursor string
	if lastName != "" {
		cursor = pagination.EncodeCursor(map[string]string{"name": lastName})
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

// GetTemplate handles GET /v3/{domain_name}/templates/{name}.
func (h *Handlers) GetTemplate(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")
	name := chi.URLParam(r, "name")
	activeFlag := r.URL.Query().Get("active") == "yes"

	tmpl, err := h.findTemplate(domainName, name)
	if err != nil {
		response.RespondError(w, http.StatusNotFound, "template not found")
		return
	}

	result := templateMap(tmpl)
	if activeFlag {
		if av := h.activeVersion(tmpl.ID); av != nil {
			result["version"] = versionMap(av)
		} else {
			result["version"] = nil
		}
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"template": result,
	})
}

// UpdateTemplate handles PUT /v3/{domain_name}/templates/{name}.
func (h *Handlers) UpdateTemplate(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")
	name := chi.URLParam(r, "name")
	r.ParseMultipartForm(32 << 20)

	tmpl, err := h.findTemplate(domainName, name)
	if err != nil {
		response.RespondError(w, http.StatusNotFound, "template not found")
		return
	}

	description := r.FormValue("description")
	tmpl.Description = description
	h.db.Save(tmpl)

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "template has been updated",
		"template": map[string]interface{}{
			"name": tmpl.Name,
		},
	})
}

// DeleteTemplate handles DELETE /v3/{domain_name}/templates/{name}.
func (h *Handlers) DeleteTemplate(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")
	name := chi.URLParam(r, "name")

	tmpl, err := h.findTemplate(domainName, name)
	if err != nil {
		response.RespondError(w, http.StatusNotFound, "template not found")
		return
	}

	// Delete all versions first.
	h.db.Where("template_id = ?", tmpl.ID).Delete(&TemplateVersion{})
	// Delete the template.
	h.db.Delete(tmpl)

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "template has been deleted",
		"template": map[string]interface{}{
			"name": tmpl.Name,
		},
	})
}

// DeleteAllTemplates handles DELETE /v3/{domain_name}/templates.
func (h *Handlers) DeleteAllTemplates(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")

	// Find all templates for this domain to delete their versions.
	var templates []Template
	h.db.Where("domain_name = ?", domainName).Find(&templates)

	for _, t := range templates {
		h.db.Where("template_id = ?", t.ID).Delete(&TemplateVersion{})
	}

	// Delete all templates for this domain.
	h.db.Where("domain_name = ?", domainName).Delete(&Template{})

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "templates have been deleted",
	})
}

// ---------------------------------------------------------------------------
// Version Management
// ---------------------------------------------------------------------------

// CreateVersion handles POST /v3/{domain_name}/templates/{name}/versions.
func (h *Handlers) CreateVersion(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")
	name := chi.URLParam(r, "name")
	r.ParseMultipartForm(32 << 20)

	tmpl, err := h.findTemplate(domainName, name)
	if err != nil {
		response.RespondError(w, http.StatusNotFound, "template not found")
		return
	}

	templateContent := r.FormValue("template")
	if templateContent == "" {
		response.RespondError(w, http.StatusBadRequest, "template is required")
		return
	}

	tag := strings.ToLower(r.FormValue("tag"))
	if tag == "" {
		response.RespondError(w, http.StatusBadRequest, "tag is required")
		return
	}

	// Check max versions.
	if h.versionCount(tmpl.ID) >= 40 {
		response.RespondError(w, http.StatusBadRequest, "maximum number of versions reached")
		return
	}

	// Check for duplicate tag.
	var existingVer TemplateVersion
	if err := h.db.Where("template_id = ? AND tag = ?", tmpl.ID, tag).First(&existingVer).Error; err == nil {
		response.RespondError(w, http.StatusBadRequest, fmt.Sprintf("version with tag '%s' already exists", tag))
		return
	}

	engine := r.FormValue("engine")
	if engine == "" {
		engine = "handlebars"
	}
	comment := r.FormValue("comment")
	headers := r.FormValue("headers")
	activeStr := r.FormValue("active")

	// First version is auto-active. Otherwise, active only if explicitly set.
	isFirstVersion := h.versionCount(tmpl.ID) == 0
	makeActive := isFirstVersion || activeStr == "yes"

	if makeActive {
		h.deactivateAll(tmpl.ID)
	}

	ver := TemplateVersion{
		TemplateID: tmpl.ID,
		Tag:        tag,
		Template:   templateContent,
		Engine:     engine,
		Comment:    comment,
		Active:     makeActive,
		Headers:    headers,
	}
	if err := h.db.Create(&ver).Error; err != nil {
		response.RespondError(w, http.StatusInternalServerError, "failed to create version")
		return
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "new version of the template has been stored",
		"template": map[string]interface{}{
			"name":    tmpl.Name,
			"version": versionMap(&ver),
		},
	})
}

// ListVersions handles GET /v3/{domain_name}/templates/{name}/versions.
func (h *Handlers) ListVersions(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")
	name := chi.URLParam(r, "name")

	tmpl, err := h.findTemplate(domainName, name)
	if err != nil {
		response.RespondError(w, http.StatusNotFound, "template not found")
		return
	}

	cp := pagination.ParseCursorParams(r)

	query := h.db.Where("template_id = ?", tmpl.ID)

	if cp.Pivot != "" {
		cursorData, err := pagination.DecodeCursor(cp.Pivot)
		if err == nil {
			if tag := cursorData["tag"]; tag != "" {
				switch cp.Page {
				case "next":
					query = query.Where("tag > ?", tag)
				case "prev":
					query = query.Where("tag < ?", tag)
				}
			}
		}
	}

	if cp.Page == "last" {
		query = query.Order("tag DESC")
	} else {
		query = query.Order("tag ASC")
	}

	var versions []TemplateVersion
	query.Limit(cp.Limit + 1).Find(&versions)

	hasMore := len(versions) > cp.Limit
	if hasMore {
		versions = versions[:cp.Limit]
	}

	if cp.Page == "prev" || cp.Page == "last" {
		for i, j := 0, len(versions)-1; i < j; i, j = i+1, j-1 {
			versions[i], versions[j] = versions[j], versions[i]
		}
	}

	versionItems := make([]interface{}, 0, len(versions))
	var lastTag string
	for _, v := range versions {
		versionItems = append(versionItems, versionMetaMap(&v))
		lastTag = v.Tag
	}

	var cursor string
	if lastTag != "" {
		cursor = pagination.EncodeCursor(map[string]string{"tag": lastTag})
	}

	paging := pagination.GeneratePagingURLs(
		baseURL(r, r.URL.Path),
		cp.Limit,
		cursor,
		hasMore,
	)

	result := templateMap(tmpl)
	result["versions"] = versionItems

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"template": result,
		"paging":   paging,
	})
}

// GetVersion handles GET /v3/{domain_name}/templates/{name}/versions/{tag}.
func (h *Handlers) GetVersion(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")
	name := chi.URLParam(r, "name")
	tag := chi.URLParam(r, "tag")

	tmpl, err := h.findTemplate(domainName, name)
	if err != nil {
		response.RespondError(w, http.StatusNotFound, "template not found")
		return
	}

	ver, err := h.findVersion(tmpl.ID, tag)
	if err != nil {
		response.RespondError(w, http.StatusNotFound, "version not found")
		return
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"template": map[string]interface{}{
			"name":    tmpl.Name,
			"version": versionMap(ver),
		},
	})
}

// UpdateVersion handles PUT /v3/{domain_name}/templates/{name}/versions/{tag}.
func (h *Handlers) UpdateVersion(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")
	name := chi.URLParam(r, "name")
	tag := chi.URLParam(r, "tag")
	r.ParseMultipartForm(32 << 20)

	tmpl, err := h.findTemplate(domainName, name)
	if err != nil {
		response.RespondError(w, http.StatusNotFound, "template not found")
		return
	}

	ver, err := h.findVersion(tmpl.ID, tag)
	if err != nil {
		response.RespondError(w, http.StatusNotFound, "version not found")
		return
	}

	// Update fields if provided (use form-presence check so callers can clear to empty).
	if _, ok := r.Form["template"]; ok {
		ver.Template = r.FormValue("template")
	}
	if _, ok := r.Form["comment"]; ok {
		ver.Comment = r.FormValue("comment")
	}
	if _, ok := r.Form["headers"]; ok {
		ver.Headers = r.FormValue("headers")
	}
	if r.FormValue("active") == "yes" {
		h.deactivateAll(tmpl.ID)
		ver.Active = true
	}

	h.db.Save(ver)

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "version has been updated",
		"template": map[string]interface{}{
			"name": tmpl.Name,
			"version": map[string]interface{}{
				"tag": ver.Tag,
			},
		},
	})
}

// DeleteVersion handles DELETE /v3/{domain_name}/templates/{name}/versions/{tag}.
func (h *Handlers) DeleteVersion(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")
	name := chi.URLParam(r, "name")
	tag := chi.URLParam(r, "tag")

	tmpl, err := h.findTemplate(domainName, name)
	if err != nil {
		response.RespondError(w, http.StatusNotFound, "template not found")
		return
	}

	ver, err := h.findVersion(tmpl.ID, tag)
	if err != nil {
		response.RespondError(w, http.StatusNotFound, "version not found")
		return
	}

	h.db.Delete(ver)

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "version has been deleted",
		"template": map[string]interface{}{
			"name": tmpl.Name,
			"version": map[string]interface{}{
				"tag": ver.Tag,
			},
		},
	})
}

// CopyVersion handles PUT /v3/{domain_name}/templates/{name}/versions/{tag}/copy/{new_tag}.
func (h *Handlers) CopyVersion(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")
	name := chi.URLParam(r, "name")
	tag := chi.URLParam(r, "tag")
	newTag := strings.ToLower(chi.URLParam(r, "new_tag"))

	tmpl, err := h.findTemplate(domainName, name)
	if err != nil {
		response.RespondError(w, http.StatusNotFound, "template not found")
		return
	}

	srcVer, err := h.findVersion(tmpl.ID, tag)
	if err != nil {
		response.RespondError(w, http.StatusNotFound, "version not found")
		return
	}

	comment := r.URL.Query().Get("comment")

	// Check if target tag already exists; if so, overwrite.
	var targetVer TemplateVersion
	if err := h.db.Where("template_id = ? AND tag = ?", tmpl.ID, newTag).First(&targetVer).Error; err == nil {
		// Overwrite existing version.
		targetVer.Template = srcVer.Template
		targetVer.Engine = srcVer.Engine
		targetVer.Mjml = srcVer.Mjml
		targetVer.Headers = srcVer.Headers
		targetVer.Comment = comment
		targetVer.Active = false
		h.db.Save(&targetVer)

		response.RespondJSON(w, http.StatusOK, map[string]interface{}{
			"message":  "version has been copied",
			"version":  versionMap(&targetVer),
			"template": map[string]interface{}{"tag": newTag},
		})
		return
	}

	// Create new version.
	newVer := TemplateVersion{
		TemplateID: tmpl.ID,
		Tag:        newTag,
		Template:   srcVer.Template,
		Engine:     srcVer.Engine,
		Mjml:       srcVer.Mjml,
		Headers:    srcVer.Headers,
		Comment:    comment,
		Active:     false,
	}
	if err := h.db.Create(&newVer).Error; err != nil {
		response.RespondError(w, http.StatusInternalServerError, "failed to copy version")
		return
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"message":  "version has been copied",
		"version":  versionMap(&newVer),
		"template": map[string]interface{}{"tag": newTag},
	})
}
