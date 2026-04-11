package mailinglist

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bethmaloney/mailgun-mock-api/internal/database"
	"github.com/bethmaloney/mailgun-mock-api/internal/pagination"
	"github.com/bethmaloney/mailgun-mock-api/internal/response"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// Models
// ---------------------------------------------------------------------------

// MailingList represents a mailing list.
type MailingList struct {
	database.BaseModel
	Address         string `gorm:"uniqueIndex" json:"address"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	AccessLevel     string `json:"access_level"`
	ReplyPreference string `json:"reply_preference"`
	MembersCount    int    `json:"members_count"`
}

// MailingListMember represents a member of a mailing list.
type MailingListMember struct {
	database.BaseModel
	ListAddress string `gorm:"index;uniqueIndex:idx_member_list_address" json:"-"`
	Address     string `gorm:"uniqueIndex:idx_member_list_address" json:"address"`
	Name        string `json:"name"`
	Subscribed  bool   `gorm:"default:true" json:"subscribed"`
	Vars        string `json:"-"` // JSON string stored in DB
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// Handlers provides HTTP handlers for mailing list endpoints.
type Handlers struct {
	db *gorm.DB
}

// NewHandlers creates a new Handlers instance. It resets mailing-list-related
// data in the database to ensure a clean state for the mock server.
func NewHandlers(db *gorm.DB) *Handlers {
	db.Unscoped().Where("1 = 1").Delete(&MailingList{})
	db.Unscoped().Where("1 = 1").Delete(&MailingListMember{})
	return &Handlers{db: db}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const rfc2822 = "Mon, 02 Jan 2006 15:04:05 -0700"

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

func parseVars(varsStr string) map[string]interface{} {
	if varsStr == "" || varsStr == "{}" {
		return map[string]interface{}{}
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(varsStr), &result); err != nil {
		return map[string]interface{}{}
	}
	return result
}

func decodeParam(s string) string {
	decoded, err := url.PathUnescape(s)
	if err != nil {
		return s
	}
	return decoded
}

func parseBool(s string) bool {
	switch strings.ToLower(s) {
	case "true", "yes", "1", "on":
		return true
	default:
		return false
	}
}

func formatListItem(ml MailingList) map[string]interface{} {
	return map[string]interface{}{
		"address":          ml.Address,
		"name":             ml.Name,
		"description":      ml.Description,
		"access_level":     ml.AccessLevel,
		"reply_preference": ml.ReplyPreference,
		"created_at":       formatTime(ml.CreatedAt),
		"members_count":    ml.MembersCount,
	}
}

func formatMemberItem(m MailingListMember) map[string]interface{} {
	return map[string]interface{}{
		"address":    m.Address,
		"name":       m.Name,
		"subscribed": m.Subscribed,
		"vars":       parseVars(m.Vars),
	}
}

var validAccessLevels = map[string]bool{
	"readonly": true,
	"members":  true,
	"everyone": true,
}

var validReplyPreferences = map[string]bool{
	"sender": true,
	"list":   true,
}

// ---------------------------------------------------------------------------
// Mailing List CRUD
// ---------------------------------------------------------------------------

// CreateList handles POST /v3/lists
func (h *Handlers) CreateList(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(32 << 20)

	address := r.FormValue("address")
	if address == "" {
		response.RespondError(w, http.StatusBadRequest, "address is required")
		return
	}

	name := r.FormValue("name")
	description := r.FormValue("description")

	accessLevel := r.FormValue("access_level")
	if accessLevel == "" {
		accessLevel = "readonly"
	}
	if !validAccessLevels[accessLevel] {
		response.RespondError(w, http.StatusBadRequest,
			fmt.Sprintf("Invalid access level '%s'. It can be any of: 'readonly', 'members', 'everyone'.", accessLevel))
		return
	}

	replyPreference := r.FormValue("reply_preference")
	if replyPreference == "" {
		replyPreference = "list"
	}
	if !validReplyPreferences[replyPreference] {
		response.RespondError(w, http.StatusBadRequest,
			fmt.Sprintf("Invalid reply preference '%s'. It can be any of: 'sender', 'list'", replyPreference))
		return
	}

	// For readonly access_level, force reply_preference to "sender"
	// but only when access_level was explicitly provided in the request
	if _, hasAccessLevel := r.Form["access_level"]; hasAccessLevel && accessLevel == "readonly" {
		replyPreference = "sender"
	}

	ml := MailingList{
		Address:         address,
		Name:            name,
		Description:     description,
		AccessLevel:     accessLevel,
		ReplyPreference: replyPreference,
		MembersCount:    0,
	}

	if err := h.db.Create(&ml).Error; err != nil {
		response.RespondError(w, http.StatusBadRequest, fmt.Sprintf("Failed to create mailing list: %v", err))
		return
	}

	listItem := formatListItem(ml)
	resp := map[string]interface{}{
		"message": "Mailing list has been created",
		"list":    listItem,
	}
	// Include top-level fields so the mailgun-go SDK can parse them directly
	for k, v := range listItem {
		resp[k] = v
	}
	// Note: spec says 201, but the official mailgun-go SDK only accepts
	// 200/202/204 for this endpoint. Returning 200 to preserve SDK compatibility.
	response.RespondJSON(w, http.StatusOK, resp)
}

// GetList handles GET /v3/lists/{list_address}
func (h *Handlers) GetList(w http.ResponseWriter, r *http.Request) {
	listAddress := decodeParam(chi.URLParam(r, "list_address"))

	var ml MailingList
	if err := h.db.Where("address = ?", listAddress).First(&ml).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, fmt.Sprintf("Mailing list %s not found", listAddress))
		return
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"list": formatListItem(ml),
	})
}

// UpdateList handles PUT /v3/lists/{list_address}
func (h *Handlers) UpdateList(w http.ResponseWriter, r *http.Request) {
	listAddress := decodeParam(chi.URLParam(r, "list_address"))

	var ml MailingList
	if err := h.db.Where("address = ?", listAddress).First(&ml).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, fmt.Sprintf("Mailing list %s not found", listAddress))
		return
	}

	r.ParseMultipartForm(32 << 20)

	// Only update fields that are present in the form data
	if _, ok := r.Form["address"]; ok {
		newAddr := r.FormValue("address")
		if newAddr != "" {
			ml.Address = newAddr
		}
	}
	if _, ok := r.Form["name"]; ok {
		ml.Name = r.FormValue("name")
	}
	if _, ok := r.Form["description"]; ok {
		ml.Description = r.FormValue("description")
	}
	if _, ok := r.Form["access_level"]; ok {
		accessLevel := r.FormValue("access_level")
		if !validAccessLevels[accessLevel] {
			response.RespondError(w, http.StatusBadRequest,
				fmt.Sprintf("Invalid access level '%s'. It can be any of: 'readonly', 'members', 'everyone'.", accessLevel))
			return
		}
		ml.AccessLevel = accessLevel
	}
	if _, ok := r.Form["reply_preference"]; ok {
		replyPreference := r.FormValue("reply_preference")
		if !validReplyPreferences[replyPreference] {
			response.RespondError(w, http.StatusBadRequest,
				fmt.Sprintf("Invalid reply preference '%s'. It can be any of: 'sender', 'list'", replyPreference))
			return
		}
		ml.ReplyPreference = replyPreference
	}

	// For readonly access_level, force reply_preference to "sender"
	if ml.AccessLevel == "readonly" {
		ml.ReplyPreference = "sender"
	}

	if err := h.db.Save(&ml).Error; err != nil {
		response.RespondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to update mailing list: %v", err))
		return
	}

	listItem := formatListItem(ml)
	resp := map[string]interface{}{
		"message": "Mailing list has been updated",
		"list":    listItem,
	}
	// Include top-level fields so the mailgun-go SDK can parse them directly
	for k, v := range listItem {
		resp[k] = v
	}
	response.RespondJSON(w, http.StatusOK, resp)
}

// DeleteList handles DELETE /v3/lists/{list_address}
func (h *Handlers) DeleteList(w http.ResponseWriter, r *http.Request) {
	listAddress := decodeParam(chi.URLParam(r, "list_address"))

	var ml MailingList
	if err := h.db.Where("address = ?", listAddress).First(&ml).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, fmt.Sprintf("Mailing list %s not found", listAddress))
		return
	}

	// Delete all members of this list
	h.db.Unscoped().Where("list_address = ?", listAddress).Delete(&MailingListMember{})

	// Delete the list itself
	h.db.Unscoped().Delete(&ml)

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"address": listAddress,
		"message": "Mailing list has been removed",
	})
}

// ---------------------------------------------------------------------------
// Mailing List Pagination
// ---------------------------------------------------------------------------

// ListLists handles GET /v3/lists/pages (cursor-based pagination)
func (h *Handlers) ListLists(w http.ResponseWriter, r *http.Request) {
	cp := pagination.ParseCursorParams(r)

	query := h.db.Model(&MailingList{})

	// Support address query param as a pivot for cursor navigation
	addressParam := r.URL.Query().Get("address")

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
	} else if addressParam != "" {
		// Use address param as a pivot
		switch cp.Page {
		case "next":
			query = query.Where("address > ?", addressParam)
		case "prev":
			query = query.Where("address < ?", addressParam)
		}
	}

	if cp.Page == "last" {
		query = query.Order("address DESC")
	} else {
		query = query.Order("address ASC")
	}

	var items []MailingList
	query.Limit(cp.Limit + 1).Find(&items)

	hasMore := len(items) > cp.Limit
	if hasMore {
		items = items[:cp.Limit]
	}

	// For prev/last, reverse to restore ascending order
	if cp.Page == "prev" || cp.Page == "last" {
		for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
			items[i], items[j] = items[j], items[i]
		}
	}

	result := make([]map[string]interface{}, 0, len(items))
	var lastAddr string
	for _, ml := range items {
		result = append(result, formatListItem(ml))
		lastAddr = ml.Address
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
		"items":  result,
		"paging": paging,
	})
}

// ListListsLegacy handles GET /v3/lists (offset-based pagination)
func (h *Handlers) ListListsLegacy(w http.ResponseWriter, r *http.Request) {
	sl := pagination.ParseSkipLimitParams(r, 100, 100)

	query := h.db.Model(&MailingList{})

	// Support address query param as exact-match filter
	if addr := r.URL.Query().Get("address"); addr != "" {
		query = query.Where("address = ?", addr)
	}

	var totalCount int64
	query.Count(&totalCount)

	var items []MailingList
	query.Order("address ASC").Offset(sl.Skip).Limit(sl.Limit).Find(&items)

	result := make([]map[string]interface{}, 0, len(items))
	for _, ml := range items {
		result = append(result, formatListItem(ml))
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"total_count": totalCount,
		"items":       result,
	})
}

// ---------------------------------------------------------------------------
// Member CRUD
// ---------------------------------------------------------------------------

// AddMember handles POST /v3/lists/{list_address}/members
func (h *Handlers) AddMember(w http.ResponseWriter, r *http.Request) {
	listAddress := decodeParam(chi.URLParam(r, "list_address"))

	var ml MailingList
	if err := h.db.Where("address = ?", listAddress).First(&ml).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, fmt.Sprintf("Mailing list %s not found", listAddress))
		return
	}

	r.ParseMultipartForm(32 << 20)

	address := r.FormValue("address")
	if address == "" {
		response.RespondError(w, http.StatusBadRequest, "address is required")
		return
	}

	name := r.FormValue("name")
	varsStr := r.FormValue("vars")

	// Parse subscribed (default true)
	subscribed := true
	if _, ok := r.Form["subscribed"]; ok {
		subscribed = parseBool(r.FormValue("subscribed"))
	}

	// Parse upsert (default false)
	upsert := false
	if _, ok := r.Form["upsert"]; ok {
		upsert = parseBool(r.FormValue("upsert"))
	}

	// Check if member already exists
	var existing MailingListMember
	memberExists := h.db.Where("list_address = ? AND address = ?", listAddress, address).First(&existing).Error == nil

	if memberExists && !upsert {
		response.RespondError(w, http.StatusBadRequest, fmt.Sprintf("Address already exists '%s'", address))
		return
	}

	if memberExists && upsert {
		// Update existing member
		existing.Name = name
		existing.Subscribed = subscribed
		if varsStr != "" {
			existing.Vars = varsStr
		}
		h.db.Save(&existing)

		response.RespondJSON(w, http.StatusOK, map[string]interface{}{
			"message": "Mailing list member has been created",
			"member":  formatMemberItem(existing),
		})
		return
	}

	// Create new member
	member := MailingListMember{
		ListAddress: listAddress,
		Address:     address,
		Name:        name,
		Subscribed:  subscribed,
		Vars:        varsStr,
	}

	if err := h.db.Create(&member).Error; err != nil {
		response.RespondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to add member: %v", err))
		return
	}

	// Explicitly set subscribed field to handle gorm default:true override
	if !subscribed {
		h.db.Model(&member).Update("subscribed", false)
		member.Subscribed = false
	}

	// Increment members_count
	h.db.Model(&MailingList{}).Where("address = ?", listAddress).
		Update("members_count", gorm.Expr("members_count + 1"))

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Mailing list member has been created",
		"member":  formatMemberItem(member),
	})
}

// GetMember handles GET /v3/lists/{list_address}/members/{member_address}
func (h *Handlers) GetMember(w http.ResponseWriter, r *http.Request) {
	listAddress := decodeParam(chi.URLParam(r, "list_address"))
	memberAddress := decodeParam(chi.URLParam(r, "member_address"))

	var member MailingListMember
	if err := h.db.Where("list_address = ? AND address = ?", listAddress, memberAddress).First(&member).Error; err != nil {
		response.RespondError(w, http.StatusNotFound,
			fmt.Sprintf("Member %s of mailing list %s not found", memberAddress, listAddress))
		return
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"member": formatMemberItem(member),
	})
}

// UpdateMember handles PUT /v3/lists/{list_address}/members/{member_address}
func (h *Handlers) UpdateMember(w http.ResponseWriter, r *http.Request) {
	listAddress := decodeParam(chi.URLParam(r, "list_address"))
	memberAddress := decodeParam(chi.URLParam(r, "member_address"))

	var member MailingListMember
	if err := h.db.Where("list_address = ? AND address = ?", listAddress, memberAddress).First(&member).Error; err != nil {
		response.RespondError(w, http.StatusNotFound,
			fmt.Sprintf("Member %s of mailing list %s not found", memberAddress, listAddress))
		return
	}

	r.ParseMultipartForm(32 << 20)

	// Only update fields that are present
	if _, ok := r.Form["address"]; ok {
		member.Address = r.FormValue("address")
	}
	if _, ok := r.Form["name"]; ok {
		member.Name = r.FormValue("name")
	}
	if _, ok := r.Form["vars"]; ok {
		member.Vars = r.FormValue("vars")
	}
	if _, ok := r.Form["subscribed"]; ok {
		member.Subscribed = parseBool(r.FormValue("subscribed"))
	}

	h.db.Save(&member)

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Mailing list member has been updated",
		"member":  formatMemberItem(member),
	})
}

// DeleteMember handles DELETE /v3/lists/{list_address}/members/{member_address}
func (h *Handlers) DeleteMember(w http.ResponseWriter, r *http.Request) {
	listAddress := decodeParam(chi.URLParam(r, "list_address"))
	memberAddress := decodeParam(chi.URLParam(r, "member_address"))

	var member MailingListMember
	if err := h.db.Where("list_address = ? AND address = ?", listAddress, memberAddress).First(&member).Error; err != nil {
		response.RespondError(w, http.StatusNotFound,
			fmt.Sprintf("Member %s of mailing list %s not found", memberAddress, listAddress))
		return
	}

	h.db.Unscoped().Delete(&member)

	// Decrement members_count
	h.db.Model(&MailingList{}).Where("address = ?", listAddress).
		Update("members_count", gorm.Expr("members_count - 1"))

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"member": map[string]interface{}{
			"address": memberAddress,
		},
		"message": "Mailing list member has been deleted",
	})
}

// ---------------------------------------------------------------------------
// Member Pagination
// ---------------------------------------------------------------------------

// ListMembers handles GET /v3/lists/{list_address}/members/pages (cursor-based)
func (h *Handlers) ListMembers(w http.ResponseWriter, r *http.Request) {
	listAddress := decodeParam(chi.URLParam(r, "list_address"))
	cp := pagination.ParseCursorParams(r)

	query := h.db.Where("list_address = ?", listAddress)

	// Support subscribed filter
	if subStr := r.URL.Query().Get("subscribed"); subStr != "" {
		sub := parseBool(subStr)
		query = query.Where("subscribed = ?", sub)
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

	if cp.Page == "last" {
		query = query.Order("address DESC")
	} else {
		query = query.Order("address ASC")
	}

	var members []MailingListMember
	query.Limit(cp.Limit + 1).Find(&members)

	hasMore := len(members) > cp.Limit
	if hasMore {
		members = members[:cp.Limit]
	}

	// For prev/last, reverse to restore ascending order
	if cp.Page == "prev" || cp.Page == "last" {
		for i, j := 0, len(members)-1; i < j; i, j = i+1, j-1 {
			members[i], members[j] = members[j], members[i]
		}
	}

	items := make([]map[string]interface{}, 0, len(members))
	var lastAddr string
	for _, m := range members {
		items = append(items, formatMemberItem(m))
		lastAddr = m.Address
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

// ListMembersLegacy handles GET /v3/lists/{list_address}/members (offset-based)
func (h *Handlers) ListMembersLegacy(w http.ResponseWriter, r *http.Request) {
	listAddress := decodeParam(chi.URLParam(r, "list_address"))
	sl := pagination.ParseSkipLimitParams(r, 100, 100)

	query := h.db.Where("list_address = ?", listAddress)

	// Support address query param as exact-match filter
	if addr := r.URL.Query().Get("address"); addr != "" {
		query = query.Where("address = ?", addr)
	}

	// Support subscribed filter
	if subStr := r.URL.Query().Get("subscribed"); subStr != "" {
		sub := parseBool(subStr)
		query = query.Where("subscribed = ?", sub)
	}

	var totalCount int64
	query.Model(&MailingListMember{}).Count(&totalCount)

	var members []MailingListMember
	query.Order("address ASC").Offset(sl.Skip).Limit(sl.Limit).Find(&members)

	items := make([]map[string]interface{}, 0, len(members))
	for _, m := range members {
		items = append(items, formatMemberItem(m))
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"total_count": totalCount,
		"items":       items,
	})
}

// ---------------------------------------------------------------------------
// Bulk Add Members
// ---------------------------------------------------------------------------

// BulkAddMembers handles POST /v3/lists/{list_address}/members.json
func (h *Handlers) BulkAddMembers(w http.ResponseWriter, r *http.Request) {
	listAddress := decodeParam(chi.URLParam(r, "list_address"))

	var ml MailingList
	if err := h.db.Where("address = ?", listAddress).First(&ml).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, fmt.Sprintf("Mailing list %s not found", listAddress))
		return
	}

	r.ParseMultipartForm(32 << 20)

	membersJSON := r.FormValue("members")
	upsert := false
	if _, ok := r.Form["upsert"]; ok {
		upsert = parseBool(r.FormValue("upsert"))
	}

	// Try to parse as array of objects first, then as array of strings
	type memberObj struct {
		Address    string                 `json:"address"`
		Name       string                 `json:"name"`
		Vars       map[string]interface{} `json:"vars"`
		Subscribed *bool                  `json:"subscribed"`
	}

	var memberObjects []memberObj

	// First try parsing as array of objects
	if err := json.Unmarshal([]byte(membersJSON), &memberObjects); err != nil {
		// Try as array of strings
		var emails []string
		if err2 := json.Unmarshal([]byte(membersJSON), &emails); err2 != nil {
			response.RespondError(w, http.StatusBadRequest, "Invalid members JSON")
			return
		}
		// Convert email strings to member objects
		for _, email := range emails {
			memberObjects = append(memberObjects, memberObj{Address: email})
		}
	} else {
		// Check if first element looks like a string (parsed as empty object)
		// This handles the case where JSON is an array of strings but
		// json.Unmarshal to []memberObj succeeds with zero-value structs
		if len(memberObjects) > 0 && memberObjects[0].Address == "" {
			var emails []string
			if err := json.Unmarshal([]byte(membersJSON), &emails); err == nil {
				memberObjects = nil
				for _, email := range emails {
					memberObjects = append(memberObjects, memberObj{Address: email})
				}
			}
		}
	}

	addedCount := 0
	for _, mo := range memberObjects {
		if mo.Address == "" {
			continue
		}

		varsStr := ""
		if mo.Vars != nil {
			b, _ := json.Marshal(mo.Vars)
			varsStr = string(b)
		}

		subscribed := true
		if mo.Subscribed != nil {
			subscribed = *mo.Subscribed
		}

		// Check if member already exists
		var existing MailingListMember
		memberExists := h.db.Where("list_address = ? AND address = ?", listAddress, mo.Address).First(&existing).Error == nil

		if memberExists {
			if upsert {
				existing.Name = mo.Name
				existing.Subscribed = subscribed
				if varsStr != "" {
					existing.Vars = varsStr
				}
				h.db.Save(&existing)
			}
			// If not upsert and exists, skip silently for bulk operations
			continue
		}

		member := MailingListMember{
			ListAddress: listAddress,
			Address:     mo.Address,
			Name:        mo.Name,
			Subscribed:  subscribed,
			Vars:        varsStr,
		}
		h.db.Create(&member)
		// Explicitly set subscribed field to handle gorm default:true override
		if !subscribed {
			h.db.Model(&member).Update("subscribed", false)
		}
		addedCount++
	}

	// Update members_count atomically
	h.db.Model(&MailingList{}).Where("address = ?", listAddress).
		Update("members_count", gorm.Expr("members_count + ?", addedCount))

	// Reload list for response
	h.db.Where("address = ?", listAddress).First(&ml)

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"list":    formatListItem(ml),
		"message": "Mailing list has been updated",
		"task-id": "mock-task-id",
	})
}

// ---------------------------------------------------------------------------
// CSV Import Members
// ---------------------------------------------------------------------------

// csvColumnIndex builds a map from lower-cased, trimmed column name to index.
func csvColumnIndex(header []string) map[string]int {
	idx := make(map[string]int, len(header))
	for i, col := range header {
		idx[strings.TrimSpace(strings.ToLower(col))] = i
	}
	return idx
}

// csvField retrieves a trimmed field value from a CSV record by column name.
func csvField(record []string, colIndex map[string]int, name string) string {
	if i, ok := colIndex[name]; ok && i < len(record) {
		return strings.TrimSpace(record[i])
	}
	return ""
}

// CSVImportMembers handles POST /v3/lists/{list_address}/members.csv
func (h *Handlers) CSVImportMembers(w http.ResponseWriter, r *http.Request) {
	listAddress := decodeParam(chi.URLParam(r, "list_address"))

	var ml MailingList
	if err := h.db.Where("address = ?", listAddress).First(&ml).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, fmt.Sprintf("Mailing list %s not found", listAddress))
		return
	}

	// Read the CSV file from the "members" form field
	file, _, err := r.FormFile("members")
	if err != nil {
		response.RespondError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	// Parse upsert form field (default false)
	upsert := false
	if r.FormValue("upsert") != "" {
		upsert = parseBool(r.FormValue("upsert"))
	}

	// Parse default subscribed from form field
	defaultSubscribed := true
	hasDefaultSubscribed := false
	if subVal := r.FormValue("subscribed"); subVal != "" {
		defaultSubscribed = parseBool(subVal)
		hasDefaultSubscribed = true
	}

	reader := csv.NewReader(file)

	header, err := reader.Read()
	if err != nil {
		response.RespondError(w, http.StatusBadRequest, "failed to read CSV header")
		return
	}
	colIndex := csvColumnIndex(header)

	addedCount := 0
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

		name := csvField(record, colIndex, "name")
		varsStr := csvField(record, colIndex, "vars")

		// Determine subscribed value: row > form field > default true
		subscribed := true
		rowSubscribed := csvField(record, colIndex, "subscribed")
		if rowSubscribed != "" {
			subscribed = parseBool(rowSubscribed)
		} else if hasDefaultSubscribed {
			subscribed = defaultSubscribed
		}

		// Check if member already exists
		var existing MailingListMember
		memberExists := h.db.Where("list_address = ? AND address = ?", listAddress, address).First(&existing).Error == nil

		if memberExists {
			if upsert {
				existing.Name = name
				existing.Subscribed = subscribed
				if varsStr != "" {
					existing.Vars = varsStr
				}
				h.db.Save(&existing)
			}
			// If not upsert and exists, skip silently for bulk operations
			continue
		}

		member := MailingListMember{
			ListAddress: listAddress,
			Address:     address,
			Name:        name,
			Subscribed:  subscribed,
			Vars:        varsStr,
		}
		h.db.Create(&member)
		// Explicitly set subscribed field to handle gorm default:true override
		if !subscribed {
			h.db.Model(&member).Update("subscribed", false)
		}
		addedCount++
	}

	// Update members_count atomically
	h.db.Model(&MailingList{}).Where("address = ?", listAddress).
		Update("members_count", gorm.Expr("members_count + ?", addedCount))

	// Reload list for response
	h.db.Where("address = ?", listAddress).First(&ml)

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"list":    formatListItem(ml),
		"message": "Mailing list has been updated",
		"task-id": "mock-task-id",
	})
}
