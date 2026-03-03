package suppression

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bethmaloney/mailgun-mock-api/internal/database"
	"github.com/bethmaloney/mailgun-mock-api/internal/pagination"
	"github.com/bethmaloney/mailgun-mock-api/internal/response"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ---------------------------------------------------------------------------
// Models
// ---------------------------------------------------------------------------

// Bounce represents a bounced email address.
type Bounce struct {
	database.BaseModel
	DomainName string `gorm:"index;uniqueIndex:idx_bounce_domain_address"`
	Address    string `gorm:"uniqueIndex:idx_bounce_domain_address"`
	Code       string
	Error      string
}

// Complaint represents a spam complaint.
type Complaint struct {
	database.BaseModel
	DomainName string `gorm:"index;uniqueIndex:idx_complaint_domain_address"`
	Address    string `gorm:"uniqueIndex:idx_complaint_domain_address"`
	Count      int    `gorm:"default:1"`
}

// Unsubscribe represents an unsubscribed address.
type Unsubscribe struct {
	database.BaseModel
	DomainName string `gorm:"index;uniqueIndex:idx_unsub_domain_address"`
	Address    string `gorm:"uniqueIndex:idx_unsub_domain_address"`
	Tags       string // JSON array stored as string, e.g. '["*"]'
}

// AllowlistEntry represents an allowlisted address or domain.
type AllowlistEntry struct {
	database.BaseModel
	DomainName string `gorm:"index;uniqueIndex:idx_allowlist_domain_value"`
	Type       string // "address" or "domain"
	Value      string `gorm:"uniqueIndex:idx_allowlist_domain_value"`
	Reason     string
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// Handlers provides HTTP handlers for suppression endpoints.
type Handlers struct {
	db *gorm.DB
}

// NewHandlers creates a new Handlers instance. It resets suppression-related
// data in the database to ensure a clean state for the mock server.
func NewHandlers(db *gorm.DB) *Handlers {
	db.Unscoped().Where("1 = 1").Delete(&Bounce{})
	db.Unscoped().Where("1 = 1").Delete(&Complaint{})
	db.Unscoped().Where("1 = 1").Delete(&Unsubscribe{})
	db.Unscoped().Where("1 = 1").Delete(&AllowlistEntry{})
	return &Handlers{db: db}
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

func isJSONContent(r *http.Request) bool {
	ct := r.Header.Get("Content-Type")
	return strings.HasPrefix(ct, "application/json")
}

// ---------------------------------------------------------------------------
// Bounce Handlers
// ---------------------------------------------------------------------------

// ListBounces handles GET /v3/{domain_name}/bounces
func (h *Handlers) ListBounces(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")
	cp := pagination.ParseCursorParams(r)
	term := r.URL.Query().Get("term")

	query := h.db.Where("domain_name = ?", domainName)
	if term != "" {
		query = query.Where("address LIKE ?", term+"%")
	}

	// Apply cursor pagination
	if cp.Pivot != "" {
		cursorData, err := pagination.DecodeCursor(cp.Pivot)
		if err == nil {
			if addr := cursorData["address"]; addr != "" {
				switch cp.Page {
				case "next":
					query = query.Where("address > ?", addr)
				case "prev":
					query = query.Where("address < ?", addr)
				}
			}
		}
	}

	// Handle page=last: order descending, limit, then reverse
	if cp.Page == "last" {
		query = query.Order("address DESC")
	} else {
		query = query.Order("address ASC")
	}

	var bounces []Bounce
	// Fetch limit+1 to check if there are more results.
	query.Limit(cp.Limit + 1).Find(&bounces)

	hasMore := len(bounces) > cp.Limit
	if hasMore {
		bounces = bounces[:cp.Limit]
	}

	// For page=prev or page=last, reverse results to restore ascending order
	if cp.Page == "prev" || cp.Page == "last" {
		for i, j := 0, len(bounces)-1; i < j; i, j = i+1, j-1 {
			bounces[i], bounces[j] = bounces[j], bounces[i]
		}
	}

	items := make([]map[string]interface{}, 0, len(bounces))
	var lastAddr string
	for _, b := range bounces {
		items = append(items, map[string]interface{}{
			"address":    b.Address,
			"code":       b.Code,
			"error":      b.Error,
			"created_at": formatTime(b.CreatedAt),
		})
		lastAddr = b.Address
	}

	var cursor string
	if lastAddr != "" {
		cursor = pagination.EncodeCursor(map[string]string{"address": lastAddr})
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

// GetBounce handles GET /v3/{domain_name}/bounces/{address}
func (h *Handlers) GetBounce(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")
	address := chi.URLParam(r, "address")

	var bounce Bounce
	if err := h.db.Where("domain_name = ? AND address = ?", domainName, address).First(&bounce).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Address not found in bounces table")
		return
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"address":    bounce.Address,
		"code":       bounce.Code,
		"error":      bounce.Error,
		"created_at": formatTime(bounce.CreatedAt),
	})
}

// CreateBounces handles POST /v3/{domain_name}/bounces
func (h *Handlers) CreateBounces(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")

	if isJSONContent(r) {
		h.createBouncesJSON(w, r, domainName)
		return
	}
	h.createBounceForm(w, r, domainName)
}

func (h *Handlers) createBounceForm(w http.ResponseWriter, r *http.Request, domainName string) {
	r.ParseMultipartForm(32 << 20)
	address := r.FormValue("address")
	if address == "" {
		response.RespondError(w, http.StatusBadRequest, "address is required")
		return
	}

	code := r.FormValue("code")
	if code == "" {
		code = "550"
	}
	errMsg := r.FormValue("error")

	h.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "domain_name"}, {Name: "address"}},
		DoUpdates: clause.AssignmentColumns([]string{"code", "error", "updated_at"}),
	}).Create(&Bounce{
		DomainName: domainName,
		Address:    address,
		Code:       code,
		Error:      errMsg,
	})

	response.RespondSuccess(w, "1 addresses have been added to the bounces table")
}

func (h *Handlers) createBouncesJSON(w http.ResponseWriter, r *http.Request, domainName string) {
	var items []struct {
		Address string `json:"address"`
		Code    string `json:"code"`
		Error   string `json:"error"`
	}
	if err := json.NewDecoder(r.Body).Decode(&items); err != nil {
		response.RespondError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if len(items) > 1000 {
		response.RespondError(w, http.StatusBadRequest, "Batch size should be less than 1000")
		return
	}

	for _, item := range items {
		code := item.Code
		if code == "" {
			code = "550"
		}
		h.db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "domain_name"}, {Name: "address"}},
			DoUpdates: clause.AssignmentColumns([]string{"code", "error", "updated_at"}),
		}).Create(&Bounce{
			DomainName: domainName,
			Address:    item.Address,
			Code:       code,
			Error:      item.Error,
		})
	}

	response.RespondSuccess(w, fmt.Sprintf("%d addresses have been added to the bounces table", len(items)))
}

// ImportBounces handles POST /v3/{domain_name}/bounces/import
func (h *Handlers) ImportBounces(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")

	file, _, err := r.FormFile("file")
	if err != nil {
		response.RespondError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	reader := csv.NewReader(file)
	header, err := reader.Read()
	if err != nil {
		response.RespondError(w, http.StatusBadRequest, "invalid CSV")
		return
	}

	colIndex := csvColumnIndex(header)

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		address := csvField(record, colIndex, "address")
		if address == "" {
			continue
		}

		code := csvField(record, colIndex, "code")
		if code == "" {
			code = "550"
		}

		h.db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "domain_name"}, {Name: "address"}},
			DoUpdates: clause.AssignmentColumns([]string{"code", "error", "updated_at"}),
		}).Create(&Bounce{
			DomainName: domainName,
			Address:    address,
			Code:       code,
			Error:      csvField(record, colIndex, "error"),
		})
	}

	response.RespondJSON(w, http.StatusAccepted, map[string]string{
		"message": "file uploaded successfully for processing. standby...",
	})
}

// DeleteBounce handles DELETE /v3/{domain_name}/bounces/{address}
func (h *Handlers) DeleteBounce(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")
	address := chi.URLParam(r, "address")

	result := h.db.Where("domain_name = ? AND address = ?", domainName, address).Delete(&Bounce{})
	if result.RowsAffected == 0 {
		response.RespondError(w, http.StatusNotFound, "Address not found in bounces table")
		return
	}

	response.RespondJSON(w, http.StatusOK, map[string]string{
		"message": "Bounced addresses for this domain have been removed",
		"address": address,
	})
}

// ClearBounces handles DELETE /v3/{domain_name}/bounces
func (h *Handlers) ClearBounces(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")
	h.db.Where("domain_name = ?", domainName).Delete(&Bounce{})
	response.RespondSuccess(w, "Bounced addresses for this domain have been removed")
}

// ---------------------------------------------------------------------------
// Complaint Handlers
// ---------------------------------------------------------------------------

// ListComplaints handles GET /v3/{domain_name}/complaints
func (h *Handlers) ListComplaints(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")
	cp := pagination.ParseCursorParams(r)
	term := r.URL.Query().Get("term")

	query := h.db.Where("domain_name = ?", domainName)
	if term != "" {
		query = query.Where("address LIKE ?", term+"%")
	}

	// Apply cursor pagination
	if cp.Pivot != "" {
		cursorData, err := pagination.DecodeCursor(cp.Pivot)
		if err == nil {
			if addr := cursorData["address"]; addr != "" {
				switch cp.Page {
				case "next":
					query = query.Where("address > ?", addr)
				case "prev":
					query = query.Where("address < ?", addr)
				}
			}
		}
	}

	// Handle page=last: order descending, limit, then reverse
	if cp.Page == "last" {
		query = query.Order("address DESC")
	} else {
		query = query.Order("address ASC")
	}

	var complaints []Complaint
	query.Limit(cp.Limit + 1).Find(&complaints)

	hasMore := len(complaints) > cp.Limit
	if hasMore {
		complaints = complaints[:cp.Limit]
	}

	// For page=prev or page=last, reverse results to restore ascending order
	if cp.Page == "prev" || cp.Page == "last" {
		for i, j := 0, len(complaints)-1; i < j; i, j = i+1, j-1 {
			complaints[i], complaints[j] = complaints[j], complaints[i]
		}
	}

	items := make([]map[string]interface{}, 0, len(complaints))
	var lastAddr string
	for _, c := range complaints {
		items = append(items, map[string]interface{}{
			"address":    c.Address,
			"count":      c.Count,
			"created_at": formatTime(c.CreatedAt),
		})
		lastAddr = c.Address
	}

	var cursor string
	if lastAddr != "" {
		cursor = pagination.EncodeCursor(map[string]string{"address": lastAddr})
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

// GetComplaint handles GET /v3/{domain_name}/complaints/{address}
func (h *Handlers) GetComplaint(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")
	address := chi.URLParam(r, "address")

	var complaint Complaint
	if err := h.db.Where("domain_name = ? AND address = ?", domainName, address).First(&complaint).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "No spam complaints found for this address")
		return
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"address":    complaint.Address,
		"count":      complaint.Count,
		"created_at": formatTime(complaint.CreatedAt),
	})
}

// CreateComplaints handles POST /v3/{domain_name}/complaints
func (h *Handlers) CreateComplaints(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")

	if isJSONContent(r) {
		h.createComplaintsJSON(w, r, domainName)
		return
	}
	h.createComplaintForm(w, r, domainName)
}

func (h *Handlers) createComplaintForm(w http.ResponseWriter, r *http.Request, domainName string) {
	r.ParseMultipartForm(32 << 20)
	address := r.FormValue("address")
	if address == "" {
		response.RespondError(w, http.StatusBadRequest, "address is required")
		return
	}

	h.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "domain_name"}, {Name: "address"}},
		DoUpdates: clause.AssignmentColumns([]string{"count", "updated_at"}),
	}).Create(&Complaint{
		DomainName: domainName,
		Address:    address,
		Count:      1,
	})

	response.RespondSuccess(w, "1 addresses have been added to the complaints table")
}

func (h *Handlers) createComplaintsJSON(w http.ResponseWriter, r *http.Request, domainName string) {
	var items []struct {
		Address string `json:"address"`
	}
	if err := json.NewDecoder(r.Body).Decode(&items); err != nil {
		response.RespondError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if len(items) > 1000 {
		response.RespondError(w, http.StatusBadRequest, "Batch size should be less than 1000")
		return
	}

	for _, item := range items {
		h.db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "domain_name"}, {Name: "address"}},
			DoUpdates: clause.AssignmentColumns([]string{"count", "updated_at"}),
		}).Create(&Complaint{
			DomainName: domainName,
			Address:    item.Address,
			Count:      1,
		})
	}

	response.RespondSuccess(w, fmt.Sprintf("%d addresses have been added to the complaints table", len(items)))
}

// ImportComplaints handles POST /v3/{domain_name}/complaints/import
func (h *Handlers) ImportComplaints(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")

	file, _, err := r.FormFile("file")
	if err != nil {
		response.RespondError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	reader := csv.NewReader(file)
	header, err := reader.Read()
	if err != nil {
		response.RespondError(w, http.StatusBadRequest, "invalid CSV")
		return
	}

	colIndex := csvColumnIndex(header)

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		address := csvField(record, colIndex, "address")
		if address == "" {
			continue
		}

		h.db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "domain_name"}, {Name: "address"}},
			DoUpdates: clause.AssignmentColumns([]string{"count", "updated_at"}),
		}).Create(&Complaint{
			DomainName: domainName,
			Address:    address,
			Count:      1,
		})
	}

	response.RespondJSON(w, http.StatusAccepted, map[string]string{
		"message": "file uploaded successfully for processing. standby...",
	})
}

// DeleteComplaint handles DELETE /v3/{domain_name}/complaints/{address}
func (h *Handlers) DeleteComplaint(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")
	address := chi.URLParam(r, "address")

	result := h.db.Where("domain_name = ? AND address = ?", domainName, address).Delete(&Complaint{})
	if result.RowsAffected == 0 {
		response.RespondError(w, http.StatusNotFound, "No spam complaints found for this address")
		return
	}

	response.RespondJSON(w, http.StatusOK, map[string]string{
		"message": "Complaint addresses for this domain have been removed",
		"address": address,
	})
}

// ClearComplaints handles DELETE /v3/{domain_name}/complaints
func (h *Handlers) ClearComplaints(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")
	h.db.Where("domain_name = ?", domainName).Delete(&Complaint{})
	response.RespondSuccess(w, "Complaint addresses for this domain have been removed")
}

// ---------------------------------------------------------------------------
// Unsubscribe Handlers
// ---------------------------------------------------------------------------

// ListUnsubscribes handles GET /v3/{domain_name}/unsubscribes
func (h *Handlers) ListUnsubscribes(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")
	cp := pagination.ParseCursorParams(r)
	term := r.URL.Query().Get("term")

	query := h.db.Where("domain_name = ?", domainName)
	if term != "" {
		query = query.Where("address LIKE ?", term+"%")
	}

	// Apply cursor pagination
	if cp.Pivot != "" {
		cursorData, err := pagination.DecodeCursor(cp.Pivot)
		if err == nil {
			if addr := cursorData["address"]; addr != "" {
				switch cp.Page {
				case "next":
					query = query.Where("address > ?", addr)
				case "prev":
					query = query.Where("address < ?", addr)
				}
			}
		}
	}

	// Handle page=last: order descending, limit, then reverse
	if cp.Page == "last" {
		query = query.Order("address DESC")
	} else {
		query = query.Order("address ASC")
	}

	var unsubs []Unsubscribe
	query.Limit(cp.Limit + 1).Find(&unsubs)

	hasMore := len(unsubs) > cp.Limit
	if hasMore {
		unsubs = unsubs[:cp.Limit]
	}

	// For page=prev or page=last, reverse results to restore ascending order
	if cp.Page == "prev" || cp.Page == "last" {
		for i, j := 0, len(unsubs)-1; i < j; i, j = i+1, j-1 {
			unsubs[i], unsubs[j] = unsubs[j], unsubs[i]
		}
	}

	items := make([]map[string]interface{}, 0, len(unsubs))
	var lastAddr string
	for _, u := range unsubs {
		items = append(items, map[string]interface{}{
			"id":         u.ID,
			"address":    u.Address,
			"tags":       parseTags(u.Tags),
			"created_at": formatTime(u.CreatedAt),
		})
		lastAddr = u.Address
	}

	var cursor string
	if lastAddr != "" {
		cursor = pagination.EncodeCursor(map[string]string{"address": lastAddr})
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

// GetUnsubscribe handles GET /v3/{domain_name}/unsubscribes/{address}
func (h *Handlers) GetUnsubscribe(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")
	address := chi.URLParam(r, "address")

	var unsub Unsubscribe
	if err := h.db.Where("domain_name = ? AND address = ?", domainName, address).First(&unsub).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Address not found in unsubscribers table")
		return
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"id":         unsub.ID,
		"address":    unsub.Address,
		"tags":       parseTags(unsub.Tags),
		"created_at": formatTime(unsub.CreatedAt),
	})
}

// CreateUnsubscribes handles POST /v3/{domain_name}/unsubscribes
func (h *Handlers) CreateUnsubscribes(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")

	if isJSONContent(r) {
		h.createUnsubscribesJSON(w, r, domainName)
		return
	}
	h.createUnsubscribeForm(w, r, domainName)
}

func (h *Handlers) createUnsubscribeForm(w http.ResponseWriter, r *http.Request, domainName string) {
	r.ParseMultipartForm(32 << 20)
	address := r.FormValue("address")
	if address == "" {
		response.RespondError(w, http.StatusBadRequest, "address is required")
		return
	}

	tag := r.FormValue("tag")
	if tag == "" {
		tag = "*"
	}
	tagsJSON := marshalTags([]string{tag})

	h.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "domain_name"}, {Name: "address"}},
		DoUpdates: clause.AssignmentColumns([]string{"tags", "updated_at"}),
	}).Create(&Unsubscribe{
		DomainName: domainName,
		Address:    address,
		Tags:       tagsJSON,
	})

	response.RespondSuccess(w, "1 addresses have been added to the unsubscribes table")
}

func (h *Handlers) createUnsubscribesJSON(w http.ResponseWriter, r *http.Request, domainName string) {
	var items []struct {
		Address string   `json:"address"`
		Tags    []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&items); err != nil {
		response.RespondError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if len(items) > 1000 {
		response.RespondError(w, http.StatusBadRequest, "Batch size should be less than 1000")
		return
	}

	for _, item := range items {
		tags := item.Tags
		if len(tags) == 0 {
			tags = []string{"*"}
		}
		tagsJSON := marshalTags(tags)

		h.db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "domain_name"}, {Name: "address"}},
			DoUpdates: clause.AssignmentColumns([]string{"tags", "updated_at"}),
		}).Create(&Unsubscribe{
			DomainName: domainName,
			Address:    item.Address,
			Tags:       tagsJSON,
		})
	}

	response.RespondSuccess(w, fmt.Sprintf("%d addresses have been added to the unsubscribes table", len(items)))
}

// ImportUnsubscribes handles POST /v3/{domain_name}/unsubscribes/import
func (h *Handlers) ImportUnsubscribes(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")

	file, _, err := r.FormFile("file")
	if err != nil {
		response.RespondError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	reader := csv.NewReader(file)
	header, err := reader.Read()
	if err != nil {
		response.RespondError(w, http.StatusBadRequest, "invalid CSV")
		return
	}

	colIndex := csvColumnIndex(header)

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		address := csvField(record, colIndex, "address")
		if address == "" {
			continue
		}

		tag := csvField(record, colIndex, "tag")
		if tag == "" {
			tag = "*"
		}
		tagsJSON := marshalTags([]string{tag})

		h.db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "domain_name"}, {Name: "address"}},
			DoUpdates: clause.AssignmentColumns([]string{"tags", "updated_at"}),
		}).Create(&Unsubscribe{
			DomainName: domainName,
			Address:    address,
			Tags:       tagsJSON,
		})
	}

	response.RespondJSON(w, http.StatusAccepted, map[string]string{
		"message": "file uploaded successfully for processing. standby...",
	})
}

// DeleteUnsubscribe handles DELETE /v3/{domain_name}/unsubscribes/{address}
func (h *Handlers) DeleteUnsubscribe(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")
	address := chi.URLParam(r, "address")

	result := h.db.Where("domain_name = ? AND address = ?", domainName, address).Delete(&Unsubscribe{})
	if result.RowsAffected == 0 {
		response.RespondError(w, http.StatusNotFound, "Address not found in unsubscribers table")
		return
	}

	response.RespondJSON(w, http.StatusOK, map[string]string{
		"message": "Unsubscribe addresses for this domain have been removed",
		"address": address,
	})
}

// ClearUnsubscribes handles DELETE /v3/{domain_name}/unsubscribes
func (h *Handlers) ClearUnsubscribes(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")
	h.db.Where("domain_name = ?", domainName).Delete(&Unsubscribe{})
	response.RespondSuccess(w, "Unsubscribe addresses for this domain have been removed")
}

// ---------------------------------------------------------------------------
// Allowlist Handlers
// ---------------------------------------------------------------------------

// ListAllowlist handles GET /v3/{domain_name}/whitelists
func (h *Handlers) ListAllowlist(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")
	cp := pagination.ParseCursorParams(r)
	term := r.URL.Query().Get("term")

	query := h.db.Where("domain_name = ?", domainName)
	if term != "" {
		query = query.Where("value LIKE ?", term+"%")
	}

	// Apply cursor pagination
	if cp.Pivot != "" {
		cursorData, err := pagination.DecodeCursor(cp.Pivot)
		if err == nil {
			if val := cursorData["value"]; val != "" {
				switch cp.Page {
				case "next":
					query = query.Where("value > ?", val)
				case "prev":
					query = query.Where("value < ?", val)
				}
			}
		}
	}

	// Handle page=last: order descending, limit, then reverse
	if cp.Page == "last" {
		query = query.Order("value DESC")
	} else {
		query = query.Order("value ASC")
	}

	var entries []AllowlistEntry
	query.Limit(cp.Limit + 1).Find(&entries)

	hasMore := len(entries) > cp.Limit
	if hasMore {
		entries = entries[:cp.Limit]
	}

	// For page=prev or page=last, reverse results to restore ascending order
	if cp.Page == "prev" || cp.Page == "last" {
		for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
			entries[i], entries[j] = entries[j], entries[i]
		}
	}

	items := make([]map[string]interface{}, 0, len(entries))
	var lastValue string
	for _, e := range entries {
		items = append(items, map[string]interface{}{
			"type":      e.Type,
			"value":     e.Value,
			"reason":    e.Reason,
			"createdAt": formatTime(e.CreatedAt),
		})
		lastValue = e.Value
	}

	var cursor string
	if lastValue != "" {
		cursor = pagination.EncodeCursor(map[string]string{"value": lastValue})
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

// GetAllowlistEntry handles GET /v3/{domain_name}/whitelists/{value}
func (h *Handlers) GetAllowlistEntry(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")
	value := chi.URLParam(r, "value")

	var entry AllowlistEntry
	if err := h.db.Where("domain_name = ? AND value = ?", domainName, value).First(&entry).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Address/Domain not found in allowlist table")
		return
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"type":      entry.Type,
		"value":     entry.Value,
		"reason":    entry.Reason,
		"createdAt": formatTime(entry.CreatedAt),
	})
}

// CreateAllowlistEntry handles POST /v3/{domain_name}/whitelists
func (h *Handlers) CreateAllowlistEntry(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")

	r.ParseMultipartForm(32 << 20)
	address := r.FormValue("address")
	domain := r.FormValue("domain")
	reason := r.FormValue("reason")

	var entryType, value string
	if address != "" {
		entryType = "address"
		value = address
	} else if domain != "" {
		entryType = "domain"
		value = domain
	} else {
		response.RespondError(w, http.StatusBadRequest, "address or domain is required")
		return
	}

	h.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "domain_name"}, {Name: "value"}},
		DoUpdates: clause.AssignmentColumns([]string{"type", "reason", "updated_at"}),
	}).Create(&AllowlistEntry{
		DomainName: domainName,
		Type:       entryType,
		Value:      value,
		Reason:     reason,
	})

	response.RespondSuccess(w, "1 addresses have been added to the whitelists table")
}

// ImportAllowlist handles POST /v3/{domain_name}/whitelists/import
func (h *Handlers) ImportAllowlist(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")

	file, _, err := r.FormFile("file")
	if err != nil {
		response.RespondError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	reader := csv.NewReader(file)
	header, err := reader.Read()
	if err != nil {
		response.RespondError(w, http.StatusBadRequest, "invalid CSV")
		return
	}

	colIndex := csvColumnIndex(header)

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		address := csvField(record, colIndex, "address")
		domain := csvField(record, colIndex, "domain")
		reason := csvField(record, colIndex, "reason")

		var entryType, value string
		if address != "" {
			entryType = "address"
			value = address
		} else if domain != "" {
			entryType = "domain"
			value = domain
		} else {
			continue
		}

		h.db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "domain_name"}, {Name: "value"}},
			DoUpdates: clause.AssignmentColumns([]string{"type", "reason", "updated_at"}),
		}).Create(&AllowlistEntry{
			DomainName: domainName,
			Type:       entryType,
			Value:      value,
			Reason:     reason,
		})
	}

	response.RespondJSON(w, http.StatusAccepted, map[string]string{
		"message": "file uploaded successfully for processing. standby...",
	})
}

// DeleteAllowlistEntry handles DELETE /v3/{domain_name}/whitelists/{value}
func (h *Handlers) DeleteAllowlistEntry(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")
	value := chi.URLParam(r, "value")

	result := h.db.Where("domain_name = ? AND value = ?", domainName, value).Delete(&AllowlistEntry{})
	if result.RowsAffected == 0 {
		response.RespondError(w, http.StatusNotFound, "Address/Domain not found in allowlist table")
		return
	}

	response.RespondJSON(w, http.StatusOK, map[string]string{
		"message": "Allowlist address/domain has been removed",
		"value":   value,
	})
}

// ClearAllowlist handles DELETE /v3/{domain_name}/whitelists
func (h *Handlers) ClearAllowlist(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")
	h.db.Where("domain_name = ?", domainName).Delete(&AllowlistEntry{})
	response.RespondSuccess(w, "Allowlist addresses/domains for this domain have been removed")
}

// ---------------------------------------------------------------------------
// CSV Helpers
// ---------------------------------------------------------------------------

func csvColumnIndex(header []string) map[string]int {
	idx := make(map[string]int, len(header))
	for i, col := range header {
		idx[strings.TrimSpace(strings.ToLower(col))] = i
	}
	return idx
}

func csvField(record []string, colIndex map[string]int, name string) string {
	if i, ok := colIndex[name]; ok && i < len(record) {
		return strings.TrimSpace(record[i])
	}
	return ""
}

// ---------------------------------------------------------------------------
// Tag Helpers
// ---------------------------------------------------------------------------

func marshalTags(tags []string) string {
	b, _ := json.Marshal(tags)
	return string(b)
}

func parseTags(s string) []string {
	if s == "" {
		return []string{}
	}
	var tags []string
	if err := json.Unmarshal([]byte(s), &tags); err != nil {
		return []string{}
	}
	return tags
}
