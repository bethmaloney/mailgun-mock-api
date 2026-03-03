package mailinglist_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bethmaloney/mailgun-mock-api/internal/mailinglist"
	"github.com/go-chi/chi/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// Response structs for JSON decoding
// ---------------------------------------------------------------------------

type messageResponse struct {
	Message string `json:"message"`
}

type mailingListItem struct {
	Address         string `json:"address"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	AccessLevel     string `json:"access_level"`
	ReplyPreference string `json:"reply_preference"`
	CreatedAt       string `json:"created_at"`
	MembersCount    int    `json:"members_count"`
}

type listResponse struct {
	Message string          `json:"message"`
	List    mailingListItem `json:"list"`
}

type pagingURLs struct {
	First    string `json:"first"`
	Last     string `json:"last"`
	Next     string `json:"next"`
	Previous string `json:"previous"`
}

type listListResponse struct {
	Items  []mailingListItem `json:"items"`
	Paging pagingURLs        `json:"paging"`
}

type legacyListResponse struct {
	TotalCount int               `json:"total_count"`
	Items      []mailingListItem `json:"items"`
}

type memberItem struct {
	Address    string                 `json:"address"`
	Name       string                 `json:"name"`
	Subscribed bool                   `json:"subscribed"`
	Vars       map[string]interface{} `json:"vars"`
}

type memberResponse struct {
	Message string     `json:"message"`
	Member  memberItem `json:"member"`
}

type memberListResponse struct {
	Items  []memberItem `json:"items"`
	Paging pagingURLs   `json:"paging"`
}

type legacyMemberListResponse struct {
	TotalCount int          `json:"total_count"`
	Items      []memberItem `json:"items"`
}

type bulkAddResponse struct {
	List    mailingListItem `json:"list"`
	Message string          `json:"message"`
	TaskID  string          `json:"task-id"`
}

type deleteListResponse struct {
	Address string `json:"address"`
	Message string `json:"message"`
}

type deleteMemberResponse struct {
	Member  memberItem `json:"member"`
	Message string     `json:"message"`
}

// ---------------------------------------------------------------------------
// Test Helpers
// ---------------------------------------------------------------------------

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}
	if err := db.AutoMigrate(
		&mailinglist.MailingList{}, &mailinglist.MailingListMember{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

func setupRouter(db *gorm.DB) http.Handler {
	h := mailinglist.NewHandlers(db)
	r := chi.NewRouter()

	r.Route("/v3/lists", func(r chi.Router) {
		r.Post("/", h.CreateList)
		r.Get("/", h.ListListsLegacy)  // offset-based
		r.Get("/pages", h.ListLists)    // cursor-based
		r.Get("/{list_address}", h.GetList)
		r.Put("/{list_address}", h.UpdateList)
		r.Delete("/{list_address}", h.DeleteList)

		// Members
		r.Post("/{list_address}/members", h.AddMember)
		r.Get("/{list_address}/members", h.ListMembersLegacy)    // offset-based
		r.Get("/{list_address}/members/pages", h.ListMembers)    // cursor-based
		r.Get("/{list_address}/members/{member_address}", h.GetMember)
		r.Put("/{list_address}/members/{member_address}", h.UpdateMember)
		r.Delete("/{list_address}/members/{member_address}", h.DeleteMember)

		// Bulk
		r.Post("/{list_address}/members.json", h.BulkAddMembers)
		r.Post("/{list_address}/members.csv", h.CSVImportMembers)
	})

	return r
}

func setup(t *testing.T) http.Handler {
	t.Helper()
	db := setupTestDB(t)
	return setupRouter(db)
}

func newMultipartRequest(t *testing.T, method, url string, fields map[string]string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for key, val := range fields {
		if err := writer.WriteField(key, val); err != nil {
			t.Fatalf("failed to write field %q: %v", key, err)
		}
	}
	writer.Close()
	req := httptest.NewRequest(method, url, &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func doRequest(t *testing.T, router http.Handler, method, url string, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	var req *http.Request
	if fields != nil {
		req = newMultipartRequest(t, method, url, fields)
	} else {
		req = httptest.NewRequest(method, url, nil)
	}
	router.ServeHTTP(rec, req)
	return rec
}

func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder, dest interface{}) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dest); err != nil {
		t.Fatalf("failed to decode response (body=%q): %v", rec.Body.String(), err)
	}
}

func assertStatus(t *testing.T, rec *httptest.ResponseRecorder, expected int) {
	t.Helper()
	if rec.Code != expected {
		t.Errorf("expected status %d, got %d; body=%s", expected, rec.Code, rec.Body.String())
	}
}

func assertMessage(t *testing.T, rec *httptest.ResponseRecorder, expected string) {
	t.Helper()
	var body map[string]interface{}
	decodeJSON(t, rec, &body)
	msg, ok := body["message"].(string)
	if !ok {
		t.Fatalf("expected string 'message' field in response, got: %v", body)
	}
	if msg != expected {
		t.Errorf("expected message %q, got %q", expected, msg)
	}
}

// createList creates a mailing list via the API and returns the response recorder.
func createList(t *testing.T, router http.Handler, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	return doRequest(t, router, "POST", "/v3/lists", fields)
}

// createListOK creates a mailing list via the API and asserts a 201 response.
func createListOK(t *testing.T, router http.Handler, fields map[string]string) {
	t.Helper()
	rec := createList(t, router, fields)
	assertStatus(t, rec, http.StatusCreated)
}

// addMember adds a member to a mailing list via the API and returns the recorder.
func addMember(t *testing.T, router http.Handler, listAddr string, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	return doRequest(t, router, "POST", fmt.Sprintf("/v3/lists/%s/members", listAddr), fields)
}

// addMemberOK adds a member and asserts a 200 response.
func addMemberOK(t *testing.T, router http.Handler, listAddr string, fields map[string]string) {
	t.Helper()
	rec := addMember(t, router, listAddr, fields)
	assertStatus(t, rec, http.StatusOK)
}

// =========================================================================
// Mailing List CRUD Tests
// =========================================================================

// ---------------------------------------------------------------------------
// 1. Create a list -> 201 with correct response shape
// ---------------------------------------------------------------------------

func TestCreateList_Basic(t *testing.T) {
	router := setup(t)

	rec := createList(t, router, map[string]string{
		"address": "developers@example.com",
	})
	assertStatus(t, rec, http.StatusCreated)

	var resp listResponse
	decodeJSON(t, rec, &resp)

	if resp.Message != "Mailing list has been created" {
		t.Errorf("expected message %q, got %q", "Mailing list has been created", resp.Message)
	}
	if resp.List.Address != "developers@example.com" {
		t.Errorf("expected address %q, got %q", "developers@example.com", resp.List.Address)
	}
	// Defaults
	if resp.List.AccessLevel != "readonly" {
		t.Errorf("expected default access_level %q, got %q", "readonly", resp.List.AccessLevel)
	}
	if resp.List.ReplyPreference != "list" {
		t.Errorf("expected default reply_preference %q, got %q", "list", resp.List.ReplyPreference)
	}
	if resp.List.MembersCount != 0 {
		t.Errorf("expected members_count 0, got %d", resp.List.MembersCount)
	}
	if resp.List.CreatedAt == "" {
		t.Error("expected non-empty created_at")
	}
}

// ---------------------------------------------------------------------------
// 2. Create with all fields (name, description, access_level, reply_preference)
// ---------------------------------------------------------------------------

func TestCreateList_AllFields(t *testing.T) {
	router := setup(t)

	rec := createList(t, router, map[string]string{
		"address":          "team@example.com",
		"name":             "Team List",
		"description":      "A list for the team",
		"access_level":     "members",
		"reply_preference": "sender",
	})
	assertStatus(t, rec, http.StatusCreated)

	var resp listResponse
	decodeJSON(t, rec, &resp)

	if resp.List.Address != "team@example.com" {
		t.Errorf("expected address %q, got %q", "team@example.com", resp.List.Address)
	}
	if resp.List.Name != "Team List" {
		t.Errorf("expected name %q, got %q", "Team List", resp.List.Name)
	}
	if resp.List.Description != "A list for the team" {
		t.Errorf("expected description %q, got %q", "A list for the team", resp.List.Description)
	}
	if resp.List.AccessLevel != "members" {
		t.Errorf("expected access_level %q, got %q", "members", resp.List.AccessLevel)
	}
	if resp.List.ReplyPreference != "sender" {
		t.Errorf("expected reply_preference %q, got %q", "sender", resp.List.ReplyPreference)
	}
}

// ---------------------------------------------------------------------------
// 3. Create with invalid access_level -> 400
// ---------------------------------------------------------------------------

func TestCreateList_InvalidAccessLevel(t *testing.T) {
	router := setup(t)

	rec := createList(t, router, map[string]string{
		"address":      "bad@example.com",
		"access_level": "fake",
	})
	assertStatus(t, rec, http.StatusBadRequest)
	assertMessage(t, rec, "Invalid access level 'fake'. It can be any of: 'readonly', 'members', 'everyone'.")
}

// ---------------------------------------------------------------------------
// 4. Create with invalid reply_preference -> 400
// ---------------------------------------------------------------------------

func TestCreateList_InvalidReplyPreference(t *testing.T) {
	router := setup(t)

	rec := createList(t, router, map[string]string{
		"address":          "bad@example.com",
		"reply_preference": "wrong",
	})
	assertStatus(t, rec, http.StatusBadRequest)
	assertMessage(t, rec, "Invalid reply preference 'wrong'. It can be any of: 'sender', 'list'")
}

// ---------------------------------------------------------------------------
// 5. Get list -> 200 with correct response
// ---------------------------------------------------------------------------

func TestGetList_Success(t *testing.T) {
	router := setup(t)

	createListOK(t, router, map[string]string{
		"address":     "developers@example.com",
		"name":        "Developers",
		"description": "Dev list",
	})

	rec := doRequest(t, router, "GET", "/v3/lists/developers@example.com", nil)
	assertStatus(t, rec, http.StatusOK)

	var resp listResponse
	decodeJSON(t, rec, &resp)

	if resp.List.Address != "developers@example.com" {
		t.Errorf("expected address %q, got %q", "developers@example.com", resp.List.Address)
	}
	if resp.List.Name != "Developers" {
		t.Errorf("expected name %q, got %q", "Developers", resp.List.Name)
	}
	if resp.List.Description != "Dev list" {
		t.Errorf("expected description %q, got %q", "Dev list", resp.List.Description)
	}
}

// ---------------------------------------------------------------------------
// 6. Get non-existent list -> 404
// ---------------------------------------------------------------------------

func TestGetList_NotFound(t *testing.T) {
	router := setup(t)

	rec := doRequest(t, router, "GET", "/v3/lists/developers@example.com", nil)
	assertStatus(t, rec, http.StatusNotFound)
	assertMessage(t, rec, "Mailing list developers@example.com not found")
}

// ---------------------------------------------------------------------------
// 7. Update list name/description -> 200 with updated values
// ---------------------------------------------------------------------------

func TestUpdateList_Success(t *testing.T) {
	router := setup(t)

	createListOK(t, router, map[string]string{
		"address":     "developers@example.com",
		"name":        "Old Name",
		"description": "Old description",
	})

	rec := doRequest(t, router, "PUT", "/v3/lists/developers@example.com", map[string]string{
		"name":        "New Name",
		"description": "New description",
	})
	assertStatus(t, rec, http.StatusOK)

	var resp listResponse
	decodeJSON(t, rec, &resp)

	if resp.Message != "Mailing list has been updated" {
		t.Errorf("expected message %q, got %q", "Mailing list has been updated", resp.Message)
	}
	if resp.List.Name != "New Name" {
		t.Errorf("expected name %q, got %q", "New Name", resp.List.Name)
	}
	if resp.List.Description != "New description" {
		t.Errorf("expected description %q, got %q", "New description", resp.List.Description)
	}
	// Address should not have changed
	if resp.List.Address != "developers@example.com" {
		t.Errorf("expected address %q, got %q", "developers@example.com", resp.List.Address)
	}
}

// ---------------------------------------------------------------------------
// 8. Update non-existent list -> 404
// ---------------------------------------------------------------------------

func TestUpdateList_NotFound(t *testing.T) {
	router := setup(t)

	rec := doRequest(t, router, "PUT", "/v3/lists/developers@example.com", map[string]string{
		"name": "New Name",
	})
	assertStatus(t, rec, http.StatusNotFound)
	assertMessage(t, rec, "Mailing list developers@example.com not found")
}

// ---------------------------------------------------------------------------
// 9. Delete list -> 200 with correct response
// ---------------------------------------------------------------------------

func TestDeleteList_Success(t *testing.T) {
	router := setup(t)

	createListOK(t, router, map[string]string{
		"address": "developers@example.com",
	})

	rec := doRequest(t, router, "DELETE", "/v3/lists/developers@example.com", nil)
	assertStatus(t, rec, http.StatusOK)

	var resp deleteListResponse
	decodeJSON(t, rec, &resp)

	if resp.Address != "developers@example.com" {
		t.Errorf("expected address %q, got %q", "developers@example.com", resp.Address)
	}
	if resp.Message != "Mailing list has been removed" {
		t.Errorf("expected message %q, got %q", "Mailing list has been removed", resp.Message)
	}

	// Verify the list is actually gone
	rec = doRequest(t, router, "GET", "/v3/lists/developers@example.com", nil)
	assertStatus(t, rec, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// 10. Delete non-existent list -> 404
// ---------------------------------------------------------------------------

func TestDeleteList_NotFound(t *testing.T) {
	router := setup(t)

	rec := doRequest(t, router, "DELETE", "/v3/lists/developers@example.com", nil)
	assertStatus(t, rec, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// 11. Delete list cascades to members
// ---------------------------------------------------------------------------

func TestDeleteList_CascadesToMembers(t *testing.T) {
	router := setup(t)

	createListOK(t, router, map[string]string{
		"address": "team@example.com",
	})

	// Add a member
	addMemberOK(t, router, "team@example.com", map[string]string{
		"address": "alice@example.com",
		"name":    "Alice",
	})

	// Verify member exists
	rec := doRequest(t, router, "GET", "/v3/lists/team@example.com/members/alice@example.com", nil)
	assertStatus(t, rec, http.StatusOK)

	// Delete the list
	rec = doRequest(t, router, "DELETE", "/v3/lists/team@example.com", nil)
	assertStatus(t, rec, http.StatusOK)

	// Re-create the same list
	createListOK(t, router, map[string]string{
		"address": "team@example.com",
	})

	// The member should not exist in the re-created list
	rec = doRequest(t, router, "GET", "/v3/lists/team@example.com/members/alice@example.com", nil)
	assertStatus(t, rec, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// 12. Reply preference forced to "sender" for readonly lists
// ---------------------------------------------------------------------------

func TestCreateList_ReadonlyForcesReplyPreferenceSender(t *testing.T) {
	router := setup(t)

	rec := createList(t, router, map[string]string{
		"address":          "readonly-list@example.com",
		"access_level":     "readonly",
		"reply_preference": "list",
	})
	assertStatus(t, rec, http.StatusCreated)

	var resp listResponse
	decodeJSON(t, rec, &resp)

	if resp.List.ReplyPreference != "sender" {
		t.Errorf("expected reply_preference forced to %q for readonly lists, got %q", "sender", resp.List.ReplyPreference)
	}
}

// =========================================================================
// Mailing List Pagination Tests
// =========================================================================

// ---------------------------------------------------------------------------
// 13. Cursor-based: GET /v3/lists/pages returns items and paging
// ---------------------------------------------------------------------------

func TestListLists_CursorBased(t *testing.T) {
	router := setup(t)

	// Create several lists
	for _, addr := range []string{"alpha@example.com", "bravo@example.com", "charlie@example.com"} {
		createListOK(t, router, map[string]string{
			"address": addr,
		})
	}

	rec := doRequest(t, router, "GET", "/v3/lists/pages?limit=2", nil)
	assertStatus(t, rec, http.StatusOK)

	var resp listListResponse
	decodeJSON(t, rec, &resp)

	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 items on first page, got %d", len(resp.Items))
	}

	// Paging URLs should be present
	if resp.Paging.First == "" {
		t.Error("expected non-empty 'first' paging URL")
	}
	if resp.Paging.Last == "" {
		t.Error("expected non-empty 'last' paging URL")
	}
	if resp.Paging.Next == "" {
		t.Error("expected non-empty 'next' paging URL since there are more items")
	}
}

// ---------------------------------------------------------------------------
// 13b. Cursor-based: follow next page
// ---------------------------------------------------------------------------

func TestListLists_CursorBased_FollowNextPage(t *testing.T) {
	router := setup(t)

	for _, addr := range []string{"alpha@example.com", "bravo@example.com", "charlie@example.com"} {
		createListOK(t, router, map[string]string{
			"address": addr,
		})
	}

	// First page
	rec := doRequest(t, router, "GET", "/v3/lists/pages?limit=2", nil)
	assertStatus(t, rec, http.StatusOK)

	var page1 listListResponse
	decodeJSON(t, rec, &page1)

	if page1.Paging.Next == "" {
		t.Fatal("expected non-empty 'next' paging URL")
	}

	// Follow next page
	rec = httptest.NewRecorder()
	req := httptest.NewRequest("GET", page1.Paging.Next, nil)
	router.ServeHTTP(rec, req)
	assertStatus(t, rec, http.StatusOK)

	var page2 listListResponse
	decodeJSON(t, rec, &page2)

	if len(page2.Items) != 1 {
		t.Fatalf("expected 1 item on second page, got %d", len(page2.Items))
	}
}

// ---------------------------------------------------------------------------
// 13c. Cursor-based: empty result
// ---------------------------------------------------------------------------

func TestListLists_CursorBased_Empty(t *testing.T) {
	router := setup(t)

	rec := doRequest(t, router, "GET", "/v3/lists/pages", nil)
	assertStatus(t, rec, http.StatusOK)

	var resp listListResponse
	decodeJSON(t, rec, &resp)

	if len(resp.Items) != 0 {
		t.Errorf("expected 0 items, got %d", len(resp.Items))
	}
}

// ---------------------------------------------------------------------------
// 14. Offset-based: GET /v3/lists with skip/limit returns items and total_count
// ---------------------------------------------------------------------------

func TestListLists_OffsetBased(t *testing.T) {
	router := setup(t)

	for _, addr := range []string{"alpha@example.com", "bravo@example.com", "charlie@example.com"} {
		createListOK(t, router, map[string]string{
			"address": addr,
		})
	}

	// First page: skip=0, limit=2
	rec := doRequest(t, router, "GET", "/v3/lists?limit=2&skip=0", nil)
	assertStatus(t, rec, http.StatusOK)

	var resp legacyListResponse
	decodeJSON(t, rec, &resp)

	if resp.TotalCount != 3 {
		t.Errorf("expected total_count 3, got %d", resp.TotalCount)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp.Items))
	}

	// Second page: skip=2, limit=2
	rec = doRequest(t, router, "GET", "/v3/lists?limit=2&skip=2", nil)
	assertStatus(t, rec, http.StatusOK)

	var resp2 legacyListResponse
	decodeJSON(t, rec, &resp2)

	if resp2.TotalCount != 3 {
		t.Errorf("expected total_count 3, got %d", resp2.TotalCount)
	}
	if len(resp2.Items) != 1 {
		t.Fatalf("expected 1 item on second page, got %d", len(resp2.Items))
	}
}

// ---------------------------------------------------------------------------
// 14b. Offset-based: address filter
// ---------------------------------------------------------------------------

func TestListLists_OffsetBased_AddressFilter(t *testing.T) {
	router := setup(t)

	for _, addr := range []string{"alpha@example.com", "bravo@example.com", "charlie@example.com"} {
		createListOK(t, router, map[string]string{
			"address": addr,
		})
	}

	rec := doRequest(t, router, "GET", "/v3/lists?address=bravo@example.com", nil)
	assertStatus(t, rec, http.StatusOK)

	var resp legacyListResponse
	decodeJSON(t, rec, &resp)

	if len(resp.Items) != 1 {
		t.Fatalf("expected 1 item matching address filter, got %d", len(resp.Items))
	}
	if resp.Items[0].Address != "bravo@example.com" {
		t.Errorf("expected address %q, got %q", "bravo@example.com", resp.Items[0].Address)
	}
}

// =========================================================================
// Member CRUD Tests
// =========================================================================

// ---------------------------------------------------------------------------
// 15. Add member -> 200 with response containing member object
// ---------------------------------------------------------------------------

func TestAddMember_Basic(t *testing.T) {
	router := setup(t)

	createListOK(t, router, map[string]string{
		"address": "team@example.com",
	})

	rec := addMember(t, router, "team@example.com", map[string]string{
		"address": "alice@example.com",
		"name":    "Alice",
	})
	assertStatus(t, rec, http.StatusOK)

	var resp memberResponse
	decodeJSON(t, rec, &resp)

	if resp.Message != "Mailing list member has been created" {
		t.Errorf("expected message %q, got %q", "Mailing list member has been created", resp.Message)
	}
	if resp.Member.Address != "alice@example.com" {
		t.Errorf("expected address %q, got %q", "alice@example.com", resp.Member.Address)
	}
	if resp.Member.Name != "Alice" {
		t.Errorf("expected name %q, got %q", "Alice", resp.Member.Name)
	}
	// Default subscribed should be true
	if !resp.Member.Subscribed {
		t.Error("expected subscribed to default to true")
	}
}

// ---------------------------------------------------------------------------
// 16. Add member with vars (JSON string) -> vars returned as object
// ---------------------------------------------------------------------------

func TestAddMember_WithVars(t *testing.T) {
	router := setup(t)

	createListOK(t, router, map[string]string{
		"address": "team@example.com",
	})

	rec := addMember(t, router, "team@example.com", map[string]string{
		"address": "bob@example.com",
		"name":    "Bob",
		"vars":    `{"age": 30, "role": "engineer"}`,
	})
	assertStatus(t, rec, http.StatusOK)

	var resp memberResponse
	decodeJSON(t, rec, &resp)

	if resp.Member.Vars == nil {
		t.Fatal("expected vars to be non-nil")
	}
	if role, ok := resp.Member.Vars["role"].(string); !ok || role != "engineer" {
		t.Errorf("expected vars.role %q, got %v", "engineer", resp.Member.Vars["role"])
	}
	if age, ok := resp.Member.Vars["age"].(float64); !ok || age != 30 {
		t.Errorf("expected vars.age 30, got %v", resp.Member.Vars["age"])
	}
}

// ---------------------------------------------------------------------------
// 17. Add duplicate member without upsert -> 400
// ---------------------------------------------------------------------------

func TestAddMember_Duplicate_NoUpsert(t *testing.T) {
	router := setup(t)

	createListOK(t, router, map[string]string{
		"address": "team@example.com",
	})

	addMemberOK(t, router, "team@example.com", map[string]string{
		"address": "alice@example.com",
	})

	rec := addMember(t, router, "team@example.com", map[string]string{
		"address": "alice@example.com",
	})
	assertStatus(t, rec, http.StatusBadRequest)
	assertMessage(t, rec, "Address already exists 'alice@example.com'")
}

// ---------------------------------------------------------------------------
// 18. Add duplicate member with upsert=true -> 200 (updated)
// ---------------------------------------------------------------------------

func TestAddMember_Duplicate_WithUpsert(t *testing.T) {
	router := setup(t)

	createListOK(t, router, map[string]string{
		"address": "team@example.com",
	})

	addMemberOK(t, router, "team@example.com", map[string]string{
		"address": "alice@example.com",
		"name":    "Alice Original",
	})

	rec := addMember(t, router, "team@example.com", map[string]string{
		"address": "alice@example.com",
		"name":    "Alice Updated",
		"upsert":  "true",
	})
	assertStatus(t, rec, http.StatusOK)

	var resp memberResponse
	decodeJSON(t, rec, &resp)

	if resp.Member.Name != "Alice Updated" {
		t.Errorf("expected name %q after upsert, got %q", "Alice Updated", resp.Member.Name)
	}
}

// ---------------------------------------------------------------------------
// 19. Get member -> 200
// ---------------------------------------------------------------------------

func TestGetMember_Success(t *testing.T) {
	router := setup(t)

	createListOK(t, router, map[string]string{
		"address": "team@example.com",
	})

	addMemberOK(t, router, "team@example.com", map[string]string{
		"address": "alice@example.com",
		"name":    "Alice",
	})

	rec := doRequest(t, router, "GET", "/v3/lists/team@example.com/members/alice@example.com", nil)
	assertStatus(t, rec, http.StatusOK)

	var resp memberResponse
	decodeJSON(t, rec, &resp)

	if resp.Member.Address != "alice@example.com" {
		t.Errorf("expected address %q, got %q", "alice@example.com", resp.Member.Address)
	}
	if resp.Member.Name != "Alice" {
		t.Errorf("expected name %q, got %q", "Alice", resp.Member.Name)
	}
}

// ---------------------------------------------------------------------------
// 20. Get non-existent member -> 404
// ---------------------------------------------------------------------------

func TestGetMember_NotFound(t *testing.T) {
	router := setup(t)

	createListOK(t, router, map[string]string{
		"address": "team@example.com",
	})

	rec := doRequest(t, router, "GET", "/v3/lists/team@example.com/members/dev@example.com", nil)
	assertStatus(t, rec, http.StatusNotFound)
	assertMessage(t, rec, "Member dev@example.com of mailing list team@example.com not found")
}

// ---------------------------------------------------------------------------
// 21. Update member -> 200 with updated fields
// ---------------------------------------------------------------------------

func TestUpdateMember_Success(t *testing.T) {
	router := setup(t)

	createListOK(t, router, map[string]string{
		"address": "team@example.com",
	})

	addMemberOK(t, router, "team@example.com", map[string]string{
		"address": "alice@example.com",
		"name":    "Alice",
	})

	rec := doRequest(t, router, "PUT", "/v3/lists/team@example.com/members/alice@example.com", map[string]string{
		"name":       "Alice Updated",
		"subscribed": "false",
	})
	assertStatus(t, rec, http.StatusOK)

	var resp memberResponse
	decodeJSON(t, rec, &resp)

	if resp.Message != "Mailing list member has been updated" {
		t.Errorf("expected message %q, got %q", "Mailing list member has been updated", resp.Message)
	}
	if resp.Member.Name != "Alice Updated" {
		t.Errorf("expected name %q, got %q", "Alice Updated", resp.Member.Name)
	}
	if resp.Member.Subscribed {
		t.Error("expected subscribed to be false after update")
	}
}

// ---------------------------------------------------------------------------
// 21b. Update member vars
// ---------------------------------------------------------------------------

func TestUpdateMember_Vars(t *testing.T) {
	router := setup(t)

	createListOK(t, router, map[string]string{
		"address": "team@example.com",
	})

	addMemberOK(t, router, "team@example.com", map[string]string{
		"address": "alice@example.com",
		"vars":    `{"color": "blue"}`,
	})

	rec := doRequest(t, router, "PUT", "/v3/lists/team@example.com/members/alice@example.com", map[string]string{
		"vars": `{"color": "red", "size": "large"}`,
	})
	assertStatus(t, rec, http.StatusOK)

	var resp memberResponse
	decodeJSON(t, rec, &resp)

	if color, ok := resp.Member.Vars["color"].(string); !ok || color != "red" {
		t.Errorf("expected vars.color %q, got %v", "red", resp.Member.Vars["color"])
	}
	if size, ok := resp.Member.Vars["size"].(string); !ok || size != "large" {
		t.Errorf("expected vars.size %q, got %v", "large", resp.Member.Vars["size"])
	}
}

// ---------------------------------------------------------------------------
// 22. Delete member -> 200, verify members_count decremented
// ---------------------------------------------------------------------------

func TestDeleteMember_Success(t *testing.T) {
	router := setup(t)

	createListOK(t, router, map[string]string{
		"address": "team@example.com",
	})

	addMemberOK(t, router, "team@example.com", map[string]string{
		"address": "alice@example.com",
		"name":    "Alice",
	})

	rec := doRequest(t, router, "DELETE", "/v3/lists/team@example.com/members/alice@example.com", nil)
	assertStatus(t, rec, http.StatusOK)

	var resp deleteMemberResponse
	decodeJSON(t, rec, &resp)

	if resp.Member.Address != "alice@example.com" {
		t.Errorf("expected member address %q, got %q", "alice@example.com", resp.Member.Address)
	}
	if resp.Message != "Mailing list member has been deleted" {
		t.Errorf("expected message %q, got %q", "Mailing list member has been deleted", resp.Message)
	}

	// Verify member is gone
	rec = doRequest(t, router, "GET", "/v3/lists/team@example.com/members/alice@example.com", nil)
	assertStatus(t, rec, http.StatusNotFound)

	// Verify members_count decremented back to 0
	rec = doRequest(t, router, "GET", "/v3/lists/team@example.com", nil)
	assertStatus(t, rec, http.StatusOK)

	var listResp listResponse
	decodeJSON(t, rec, &listResp)

	if listResp.List.MembersCount != 0 {
		t.Errorf("expected members_count 0 after deleting member, got %d", listResp.List.MembersCount)
	}
}

// ---------------------------------------------------------------------------
// 23. subscribed accepts "yes"/"no" and true/false
// ---------------------------------------------------------------------------

func TestAddMember_SubscribedVariants(t *testing.T) {
	router := setup(t)

	createListOK(t, router, map[string]string{
		"address": "team@example.com",
	})

	tests := []struct {
		email      string
		subscribed string
		expected   bool
	}{
		{"yes-user@example.com", "yes", true},
		{"no-user@example.com", "no", false},
		{"true-user@example.com", "true", true},
		{"false-user@example.com", "false", false},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("subscribed=%s", tc.subscribed), func(t *testing.T) {
			rec := addMember(t, router, "team@example.com", map[string]string{
				"address":    tc.email,
				"subscribed": tc.subscribed,
			})
			assertStatus(t, rec, http.StatusOK)

			var resp memberResponse
			decodeJSON(t, rec, &resp)

			if resp.Member.Subscribed != tc.expected {
				t.Errorf("subscribed=%q: expected %v, got %v", tc.subscribed, tc.expected, resp.Member.Subscribed)
			}
		})
	}
}

// =========================================================================
// Member Pagination Tests
// =========================================================================

// ---------------------------------------------------------------------------
// 24. Cursor-based: GET .../members/pages with subscribed filter
// ---------------------------------------------------------------------------

func TestListMembers_CursorBased(t *testing.T) {
	router := setup(t)

	createListOK(t, router, map[string]string{
		"address": "team@example.com",
	})

	// Add several members with different subscription statuses
	addMemberOK(t, router, "team@example.com", map[string]string{
		"address":    "alice@example.com",
		"name":       "Alice",
		"subscribed": "true",
	})
	addMemberOK(t, router, "team@example.com", map[string]string{
		"address":    "bob@example.com",
		"name":       "Bob",
		"subscribed": "false",
	})
	addMemberOK(t, router, "team@example.com", map[string]string{
		"address":    "charlie@example.com",
		"name":       "Charlie",
		"subscribed": "true",
	})

	// Get all members
	rec := doRequest(t, router, "GET", "/v3/lists/team@example.com/members/pages", nil)
	assertStatus(t, rec, http.StatusOK)

	var resp memberListResponse
	decodeJSON(t, rec, &resp)

	if len(resp.Items) != 3 {
		t.Fatalf("expected 3 members, got %d", len(resp.Items))
	}

	// Filter by subscribed=true
	rec = doRequest(t, router, "GET", "/v3/lists/team@example.com/members/pages?subscribed=true", nil)
	assertStatus(t, rec, http.StatusOK)

	var filteredResp memberListResponse
	decodeJSON(t, rec, &filteredResp)

	if len(filteredResp.Items) != 2 {
		t.Fatalf("expected 2 subscribed members, got %d", len(filteredResp.Items))
	}

	for _, m := range filteredResp.Items {
		if !m.Subscribed {
			t.Errorf("expected all filtered members to be subscribed, but %q is not", m.Address)
		}
	}
}

// ---------------------------------------------------------------------------
// 24b. Cursor-based: pagination with limit
// ---------------------------------------------------------------------------

func TestListMembers_CursorBased_Pagination(t *testing.T) {
	router := setup(t)

	createListOK(t, router, map[string]string{
		"address": "team@example.com",
	})

	for _, email := range []string{"a@example.com", "b@example.com", "c@example.com"} {
		addMemberOK(t, router, "team@example.com", map[string]string{
			"address": email,
		})
	}

	rec := doRequest(t, router, "GET", "/v3/lists/team@example.com/members/pages?limit=2", nil)
	assertStatus(t, rec, http.StatusOK)

	var resp memberListResponse
	decodeJSON(t, rec, &resp)

	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 items on first page, got %d", len(resp.Items))
	}

	// Paging should have URLs
	if resp.Paging.First == "" {
		t.Error("expected non-empty 'first' paging URL")
	}
	if resp.Paging.Last == "" {
		t.Error("expected non-empty 'last' paging URL")
	}
	if resp.Paging.Next == "" {
		t.Error("expected non-empty 'next' paging URL since there are more members")
	}
}

// ---------------------------------------------------------------------------
// 25. Offset-based: GET .../members with skip/limit
// ---------------------------------------------------------------------------

func TestListMembers_OffsetBased(t *testing.T) {
	router := setup(t)

	createListOK(t, router, map[string]string{
		"address": "team@example.com",
	})

	for _, email := range []string{"alice@example.com", "bob@example.com", "charlie@example.com"} {
		addMemberOK(t, router, "team@example.com", map[string]string{
			"address": email,
		})
	}

	// First page
	rec := doRequest(t, router, "GET", "/v3/lists/team@example.com/members?limit=2&skip=0", nil)
	assertStatus(t, rec, http.StatusOK)

	var resp legacyMemberListResponse
	decodeJSON(t, rec, &resp)

	if resp.TotalCount != 3 {
		t.Errorf("expected total_count 3, got %d", resp.TotalCount)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp.Items))
	}

	// Second page
	rec = doRequest(t, router, "GET", "/v3/lists/team@example.com/members?limit=2&skip=2", nil)
	assertStatus(t, rec, http.StatusOK)

	var resp2 legacyMemberListResponse
	decodeJSON(t, rec, &resp2)

	if resp2.TotalCount != 3 {
		t.Errorf("expected total_count 3, got %d", resp2.TotalCount)
	}
	if len(resp2.Items) != 1 {
		t.Fatalf("expected 1 item on second page, got %d", len(resp2.Items))
	}
}

// ---------------------------------------------------------------------------
// 25b. Offset-based: subscribed filter
// ---------------------------------------------------------------------------

func TestListMembers_OffsetBased_SubscribedFilter(t *testing.T) {
	router := setup(t)

	createListOK(t, router, map[string]string{
		"address": "team@example.com",
	})

	addMemberOK(t, router, "team@example.com", map[string]string{
		"address":    "alice@example.com",
		"subscribed": "true",
	})
	addMemberOK(t, router, "team@example.com", map[string]string{
		"address":    "bob@example.com",
		"subscribed": "false",
	})

	rec := doRequest(t, router, "GET", "/v3/lists/team@example.com/members?subscribed=false", nil)
	assertStatus(t, rec, http.StatusOK)

	var resp legacyMemberListResponse
	decodeJSON(t, rec, &resp)

	if resp.TotalCount != 1 {
		t.Errorf("expected total_count 1 for unsubscribed, got %d", resp.TotalCount)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("expected 1 unsubscribed member, got %d", len(resp.Items))
	}
	if resp.Items[0].Address != "bob@example.com" {
		t.Errorf("expected bob@example.com, got %q", resp.Items[0].Address)
	}
}

// ---------------------------------------------------------------------------
// 25c. Offset-based: address filter for members
// ---------------------------------------------------------------------------

func TestListMembers_OffsetBased_AddressFilter(t *testing.T) {
	router := setup(t)

	createListOK(t, router, map[string]string{
		"address": "team@example.com",
	})

	addMemberOK(t, router, "team@example.com", map[string]string{
		"address": "alice@example.com",
	})
	addMemberOK(t, router, "team@example.com", map[string]string{
		"address": "bob@example.com",
	})

	rec := doRequest(t, router, "GET", "/v3/lists/team@example.com/members?address=alice@example.com", nil)
	assertStatus(t, rec, http.StatusOK)

	var resp legacyMemberListResponse
	decodeJSON(t, rec, &resp)

	if len(resp.Items) != 1 {
		t.Fatalf("expected 1 member matching address filter, got %d", len(resp.Items))
	}
	if resp.Items[0].Address != "alice@example.com" {
		t.Errorf("expected address %q, got %q", "alice@example.com", resp.Items[0].Address)
	}
}

// =========================================================================
// Bulk Add Tests
// =========================================================================

// ---------------------------------------------------------------------------
// 26. Add members via JSON array of objects -> 200 with task-id
// ---------------------------------------------------------------------------

func TestBulkAddMembers_Objects(t *testing.T) {
	router := setup(t)

	createListOK(t, router, map[string]string{
		"address": "team@example.com",
	})

	members := `[{"address": "alice@example.com", "name": "Alice"}, {"address": "bob@example.com", "name": "Bob"}]`
	rec := doRequest(t, router, "POST", "/v3/lists/team@example.com/members.json", map[string]string{
		"members": members,
	})
	assertStatus(t, rec, http.StatusOK)

	var resp bulkAddResponse
	decodeJSON(t, rec, &resp)

	if resp.Message != "Mailing list has been updated" {
		t.Errorf("expected message %q, got %q", "Mailing list has been updated", resp.Message)
	}
	if resp.TaskID == "" {
		t.Error("expected non-empty task-id")
	}
	if resp.List.Address != "team@example.com" {
		t.Errorf("expected list address %q, got %q", "team@example.com", resp.List.Address)
	}

	// Verify members were added
	rec = doRequest(t, router, "GET", "/v3/lists/team@example.com/members/alice@example.com", nil)
	assertStatus(t, rec, http.StatusOK)

	rec = doRequest(t, router, "GET", "/v3/lists/team@example.com/members/bob@example.com", nil)
	assertStatus(t, rec, http.StatusOK)
}

// ---------------------------------------------------------------------------
// 27. Add members via JSON array of email strings -> 200
// ---------------------------------------------------------------------------

func TestBulkAddMembers_Strings(t *testing.T) {
	router := setup(t)

	createListOK(t, router, map[string]string{
		"address": "team@example.com",
	})

	members := `["carol@example.com", "dave@example.com"]`
	rec := doRequest(t, router, "POST", "/v3/lists/team@example.com/members.json", map[string]string{
		"members": members,
	})
	assertStatus(t, rec, http.StatusOK)

	var resp bulkAddResponse
	decodeJSON(t, rec, &resp)

	if resp.Message != "Mailing list has been updated" {
		t.Errorf("expected message %q, got %q", "Mailing list has been updated", resp.Message)
	}
	if resp.TaskID == "" {
		t.Error("expected non-empty task-id")
	}

	// Verify members were added
	rec = doRequest(t, router, "GET", "/v3/lists/team@example.com/members/carol@example.com", nil)
	assertStatus(t, rec, http.StatusOK)

	rec = doRequest(t, router, "GET", "/v3/lists/team@example.com/members/dave@example.com", nil)
	assertStatus(t, rec, http.StatusOK)
}

// ---------------------------------------------------------------------------
// 28. Bulk add with upsert -> updates existing
// ---------------------------------------------------------------------------

func TestBulkAddMembers_Upsert(t *testing.T) {
	router := setup(t)

	createListOK(t, router, map[string]string{
		"address": "team@example.com",
	})

	// Add alice initially
	addMemberOK(t, router, "team@example.com", map[string]string{
		"address": "alice@example.com",
		"name":    "Alice Original",
	})

	// Bulk add with upsert including alice and a new member
	members := `[{"address": "alice@example.com", "name": "Alice Bulk Updated"}, {"address": "eve@example.com", "name": "Eve"}]`
	rec := doRequest(t, router, "POST", "/v3/lists/team@example.com/members.json", map[string]string{
		"members": members,
		"upsert":  "true",
	})
	assertStatus(t, rec, http.StatusOK)

	// Verify alice was updated
	rec = doRequest(t, router, "GET", "/v3/lists/team@example.com/members/alice@example.com", nil)
	assertStatus(t, rec, http.StatusOK)

	var resp memberResponse
	decodeJSON(t, rec, &resp)

	if resp.Member.Name != "Alice Bulk Updated" {
		t.Errorf("expected name %q after bulk upsert, got %q", "Alice Bulk Updated", resp.Member.Name)
	}

	// Verify eve was added
	rec = doRequest(t, router, "GET", "/v3/lists/team@example.com/members/eve@example.com", nil)
	assertStatus(t, rec, http.StatusOK)
}

// ---------------------------------------------------------------------------
// 29. Bulk add to non-existent list -> 404
// ---------------------------------------------------------------------------

func TestBulkAddMembers_ListNotFound(t *testing.T) {
	router := setup(t)

	members := `[{"address": "alice@example.com"}]`
	rec := doRequest(t, router, "POST", "/v3/lists/nonexistent@example.com/members.json", map[string]string{
		"members": members,
	})
	assertStatus(t, rec, http.StatusNotFound)
}

// =========================================================================
// members_count Tracking Tests
// =========================================================================

// ---------------------------------------------------------------------------
// 30. Verify members_count increments on add and decrements on delete
// ---------------------------------------------------------------------------

func TestMembersCount_IncrementAndDecrement(t *testing.T) {
	router := setup(t)

	createListOK(t, router, map[string]string{
		"address": "team@example.com",
	})

	// Initially 0
	rec := doRequest(t, router, "GET", "/v3/lists/team@example.com", nil)
	assertStatus(t, rec, http.StatusOK)

	var resp listResponse
	decodeJSON(t, rec, &resp)

	if resp.List.MembersCount != 0 {
		t.Errorf("expected initial members_count 0, got %d", resp.List.MembersCount)
	}

	// Add first member -> count = 1
	addMemberOK(t, router, "team@example.com", map[string]string{
		"address": "alice@example.com",
	})

	rec = doRequest(t, router, "GET", "/v3/lists/team@example.com", nil)
	assertStatus(t, rec, http.StatusOK)
	decodeJSON(t, rec, &resp)

	if resp.List.MembersCount != 1 {
		t.Errorf("expected members_count 1, got %d", resp.List.MembersCount)
	}

	// Add second member -> count = 2
	addMemberOK(t, router, "team@example.com", map[string]string{
		"address": "bob@example.com",
	})

	rec = doRequest(t, router, "GET", "/v3/lists/team@example.com", nil)
	assertStatus(t, rec, http.StatusOK)
	decodeJSON(t, rec, &resp)

	if resp.List.MembersCount != 2 {
		t.Errorf("expected members_count 2, got %d", resp.List.MembersCount)
	}

	// Delete one member -> count = 1
	rec = doRequest(t, router, "DELETE", "/v3/lists/team@example.com/members/alice@example.com", nil)
	assertStatus(t, rec, http.StatusOK)

	rec = doRequest(t, router, "GET", "/v3/lists/team@example.com", nil)
	assertStatus(t, rec, http.StatusOK)
	decodeJSON(t, rec, &resp)

	if resp.List.MembersCount != 1 {
		t.Errorf("expected members_count 1 after delete, got %d", resp.List.MembersCount)
	}
}

// ---------------------------------------------------------------------------
// 30b. members_count in create response after adding via bulk
// ---------------------------------------------------------------------------

func TestMembersCount_AfterBulkAdd(t *testing.T) {
	router := setup(t)

	createListOK(t, router, map[string]string{
		"address": "team@example.com",
	})

	members := `[{"address": "alice@example.com"}, {"address": "bob@example.com"}, {"address": "charlie@example.com"}]`
	rec := doRequest(t, router, "POST", "/v3/lists/team@example.com/members.json", map[string]string{
		"members": members,
	})
	assertStatus(t, rec, http.StatusOK)

	var bulkResp bulkAddResponse
	decodeJSON(t, rec, &bulkResp)

	if bulkResp.List.MembersCount != 3 {
		t.Errorf("expected members_count 3 in bulk add response, got %d", bulkResp.List.MembersCount)
	}

	// Verify via GET as well
	rec = doRequest(t, router, "GET", "/v3/lists/team@example.com", nil)
	assertStatus(t, rec, http.StatusOK)

	var listResp listResponse
	decodeJSON(t, rec, &listResp)

	if listResp.List.MembersCount != 3 {
		t.Errorf("expected members_count 3 via GET, got %d", listResp.List.MembersCount)
	}
}

// =========================================================================
// Additional Edge Case Tests
// =========================================================================

// ---------------------------------------------------------------------------
// Update list with partial fields (only name, not description)
// ---------------------------------------------------------------------------

func TestUpdateList_PartialFields(t *testing.T) {
	router := setup(t)

	createListOK(t, router, map[string]string{
		"address":     "team@example.com",
		"name":        "Original Name",
		"description": "Original Description",
	})

	// Update only the name
	rec := doRequest(t, router, "PUT", "/v3/lists/team@example.com", map[string]string{
		"name": "Updated Name",
	})
	assertStatus(t, rec, http.StatusOK)

	var resp listResponse
	decodeJSON(t, rec, &resp)

	if resp.List.Name != "Updated Name" {
		t.Errorf("expected name %q, got %q", "Updated Name", resp.List.Name)
	}
	// Description should remain unchanged
	if resp.List.Description != "Original Description" {
		t.Errorf("expected description %q (unchanged), got %q", "Original Description", resp.List.Description)
	}
}

// ---------------------------------------------------------------------------
// Update list address
// ---------------------------------------------------------------------------

func TestUpdateList_ChangeAddress(t *testing.T) {
	router := setup(t)

	createListOK(t, router, map[string]string{
		"address": "old@example.com",
		"name":    "My List",
	})

	rec := doRequest(t, router, "PUT", "/v3/lists/old@example.com", map[string]string{
		"address": "new@example.com",
	})
	assertStatus(t, rec, http.StatusOK)

	var resp listResponse
	decodeJSON(t, rec, &resp)

	if resp.List.Address != "new@example.com" {
		t.Errorf("expected address %q, got %q", "new@example.com", resp.List.Address)
	}

	// Old address should no longer work
	rec = doRequest(t, router, "GET", "/v3/lists/old@example.com", nil)
	assertStatus(t, rec, http.StatusNotFound)

	// New address should work
	rec = doRequest(t, router, "GET", "/v3/lists/new@example.com", nil)
	assertStatus(t, rec, http.StatusOK)
}

// ---------------------------------------------------------------------------
// Create list with access_level "everyone"
// ---------------------------------------------------------------------------

func TestCreateList_AccessLevelEveryone(t *testing.T) {
	router := setup(t)

	rec := createList(t, router, map[string]string{
		"address":      "public@example.com",
		"access_level": "everyone",
	})
	assertStatus(t, rec, http.StatusCreated)

	var resp listResponse
	decodeJSON(t, rec, &resp)

	if resp.List.AccessLevel != "everyone" {
		t.Errorf("expected access_level %q, got %q", "everyone", resp.List.AccessLevel)
	}
}

// ---------------------------------------------------------------------------
// Create list with access_level "members"
// ---------------------------------------------------------------------------

func TestCreateList_AccessLevelMembers(t *testing.T) {
	router := setup(t)

	rec := createList(t, router, map[string]string{
		"address":      "private@example.com",
		"access_level": "members",
	})
	assertStatus(t, rec, http.StatusCreated)

	var resp listResponse
	decodeJSON(t, rec, &resp)

	if resp.List.AccessLevel != "members" {
		t.Errorf("expected access_level %q, got %q", "members", resp.List.AccessLevel)
	}
}

// ---------------------------------------------------------------------------
// Add member to non-existent list
// ---------------------------------------------------------------------------

func TestAddMember_ListNotFound(t *testing.T) {
	router := setup(t)

	rec := addMember(t, router, "nonexistent@example.com", map[string]string{
		"address": "alice@example.com",
	})
	assertStatus(t, rec, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// Delete member from non-existent member
// ---------------------------------------------------------------------------

func TestDeleteMember_NotFound(t *testing.T) {
	router := setup(t)

	createListOK(t, router, map[string]string{
		"address": "team@example.com",
	})

	rec := doRequest(t, router, "DELETE", "/v3/lists/team@example.com/members/nonexistent@example.com", nil)
	assertStatus(t, rec, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// Update non-existent member
// ---------------------------------------------------------------------------

func TestUpdateMember_NotFound(t *testing.T) {
	router := setup(t)

	createListOK(t, router, map[string]string{
		"address": "team@example.com",
	})

	rec := doRequest(t, router, "PUT", "/v3/lists/team@example.com/members/nonexistent@example.com", map[string]string{
		"name": "Ghost",
	})
	assertStatus(t, rec, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// Cursor-based list pagination with address filter
// ---------------------------------------------------------------------------

func TestListLists_CursorBased_AddressFilter(t *testing.T) {
	router := setup(t)

	for _, addr := range []string{"alpha@example.com", "bravo@example.com", "charlie@example.com"} {
		createListOK(t, router, map[string]string{
			"address": addr,
		})
	}

	// Use address as pivot point for cursor navigation
	rec := doRequest(t, router, "GET", "/v3/lists/pages?address=bravo@example.com&page=next&limit=10", nil)
	assertStatus(t, rec, http.StatusOK)

	var resp listListResponse
	decodeJSON(t, rec, &resp)

	// Should return items after bravo
	for _, item := range resp.Items {
		if item.Address == "bravo@example.com" {
			t.Error("pivot address should not appear in next page results")
		}
	}
}

// ---------------------------------------------------------------------------
// Default limit for cursor-based pagination
// ---------------------------------------------------------------------------

func TestListLists_CursorBased_DefaultLimit(t *testing.T) {
	router := setup(t)

	// Create a few lists
	for i := 0; i < 5; i++ {
		createListOK(t, router, map[string]string{
			"address": fmt.Sprintf("list%d@example.com", i),
		})
	}

	// No limit specified — should return all (up to default 100)
	rec := doRequest(t, router, "GET", "/v3/lists/pages", nil)
	assertStatus(t, rec, http.StatusOK)

	var resp listListResponse
	decodeJSON(t, rec, &resp)

	if len(resp.Items) != 5 {
		t.Errorf("expected 5 items with default limit, got %d", len(resp.Items))
	}
}

// ---------------------------------------------------------------------------
// Offset-based pagination with default skip
// ---------------------------------------------------------------------------

func TestListLists_OffsetBased_DefaultSkip(t *testing.T) {
	router := setup(t)

	for i := 0; i < 3; i++ {
		createListOK(t, router, map[string]string{
			"address": fmt.Sprintf("list%d@example.com", i),
		})
	}

	// No skip specified — should default to 0
	rec := doRequest(t, router, "GET", "/v3/lists?limit=10", nil)
	assertStatus(t, rec, http.StatusOK)

	var resp legacyListResponse
	decodeJSON(t, rec, &resp)

	if resp.TotalCount != 3 {
		t.Errorf("expected total_count 3, got %d", resp.TotalCount)
	}
	if len(resp.Items) != 3 {
		t.Errorf("expected 3 items, got %d", len(resp.Items))
	}
}

// ---------------------------------------------------------------------------
// Cursor-based member list: empty list
// ---------------------------------------------------------------------------

func TestListMembers_CursorBased_Empty(t *testing.T) {
	router := setup(t)

	createListOK(t, router, map[string]string{
		"address": "team@example.com",
	})

	rec := doRequest(t, router, "GET", "/v3/lists/team@example.com/members/pages", nil)
	assertStatus(t, rec, http.StatusOK)

	var resp memberListResponse
	decodeJSON(t, rec, &resp)

	if len(resp.Items) != 0 {
		t.Errorf("expected 0 members, got %d", len(resp.Items))
	}
}

// ---------------------------------------------------------------------------
// Offset-based member list: empty list
// ---------------------------------------------------------------------------

func TestListMembers_OffsetBased_Empty(t *testing.T) {
	router := setup(t)

	createListOK(t, router, map[string]string{
		"address": "team@example.com",
	})

	rec := doRequest(t, router, "GET", "/v3/lists/team@example.com/members", nil)
	assertStatus(t, rec, http.StatusOK)

	var resp legacyMemberListResponse
	decodeJSON(t, rec, &resp)

	if resp.TotalCount != 0 {
		t.Errorf("expected total_count 0, got %d", resp.TotalCount)
	}
	if len(resp.Items) != 0 {
		t.Errorf("expected 0 items, got %d", len(resp.Items))
	}
}

// ---------------------------------------------------------------------------
// Bulk add with mixed objects including vars and subscribed
// ---------------------------------------------------------------------------

func TestBulkAddMembers_ObjectsWithVarsAndSubscribed(t *testing.T) {
	router := setup(t)

	createListOK(t, router, map[string]string{
		"address": "team@example.com",
	})

	members := `[
		{"address": "alice@example.com", "name": "Alice", "vars": {"role": "admin"}, "subscribed": true},
		{"address": "bob@example.com", "name": "Bob", "vars": {"role": "user"}, "subscribed": false}
	]`
	rec := doRequest(t, router, "POST", "/v3/lists/team@example.com/members.json", map[string]string{
		"members": members,
	})
	assertStatus(t, rec, http.StatusOK)

	// Verify alice
	rec = doRequest(t, router, "GET", "/v3/lists/team@example.com/members/alice@example.com", nil)
	assertStatus(t, rec, http.StatusOK)

	var aliceResp memberResponse
	decodeJSON(t, rec, &aliceResp)

	if !aliceResp.Member.Subscribed {
		t.Error("expected alice to be subscribed")
	}
	if role, ok := aliceResp.Member.Vars["role"].(string); !ok || role != "admin" {
		t.Errorf("expected alice vars.role %q, got %v", "admin", aliceResp.Member.Vars["role"])
	}

	// Verify bob
	rec = doRequest(t, router, "GET", "/v3/lists/team@example.com/members/bob@example.com", nil)
	assertStatus(t, rec, http.StatusOK)

	var bobResp memberResponse
	decodeJSON(t, rec, &bobResp)

	if bobResp.Member.Subscribed {
		t.Error("expected bob to be unsubscribed")
	}
	if role, ok := bobResp.Member.Vars["role"].(string); !ok || role != "user" {
		t.Errorf("expected bob vars.role %q, got %v", "user", bobResp.Member.Vars["role"])
	}
}

// ---------------------------------------------------------------------------
// Created_at in RFC 2822 format
// ---------------------------------------------------------------------------

func TestCreateList_CreatedAtFormat(t *testing.T) {
	router := setup(t)

	rec := createList(t, router, map[string]string{
		"address": "formatcheck@example.com",
	})
	assertStatus(t, rec, http.StatusCreated)

	var resp listResponse
	decodeJSON(t, rec, &resp)

	if resp.List.CreatedAt == "" {
		t.Fatal("expected non-empty created_at")
	}

	// RFC 2822 example: "Mon, 02 Jan 2006 15:04:05 -0700"
	// We cannot parse with a fixed Go layout for RFC 2822 perfectly, but we can
	// check that it's non-empty and looks like a date string. The implementation
	// should use time.RFC1123Z or similar.
	// Just verify it exists and is a reasonable length (at least 20 chars).
	if len(resp.List.CreatedAt) < 20 {
		t.Errorf("created_at %q seems too short for RFC 2822 format", resp.List.CreatedAt)
	}
}

// ---------------------------------------------------------------------------
// Member default subscribed is true when not specified
// ---------------------------------------------------------------------------

func TestAddMember_DefaultSubscribed(t *testing.T) {
	router := setup(t)

	createListOK(t, router, map[string]string{
		"address": "team@example.com",
	})

	rec := addMember(t, router, "team@example.com", map[string]string{
		"address": "default-sub@example.com",
	})
	assertStatus(t, rec, http.StatusOK)

	var resp memberResponse
	decodeJSON(t, rec, &resp)

	if !resp.Member.Subscribed {
		t.Error("expected subscribed to default to true when not specified")
	}
}

// ---------------------------------------------------------------------------
// Member upsert defaults to false
// ---------------------------------------------------------------------------

func TestAddMember_UpsertDefaultsFalse(t *testing.T) {
	router := setup(t)

	createListOK(t, router, map[string]string{
		"address": "team@example.com",
	})

	addMemberOK(t, router, "team@example.com", map[string]string{
		"address": "alice@example.com",
	})

	// Adding again without upsert should fail
	rec := addMember(t, router, "team@example.com", map[string]string{
		"address": "alice@example.com",
	})
	assertStatus(t, rec, http.StatusBadRequest)
}

// ---------------------------------------------------------------------------
// Cursor-based members: filter by subscribed=false
// ---------------------------------------------------------------------------

func TestListMembers_CursorBased_SubscribedFalseFilter(t *testing.T) {
	router := setup(t)

	createListOK(t, router, map[string]string{
		"address": "team@example.com",
	})

	addMemberOK(t, router, "team@example.com", map[string]string{
		"address":    "sub@example.com",
		"subscribed": "true",
	})
	addMemberOK(t, router, "team@example.com", map[string]string{
		"address":    "unsub@example.com",
		"subscribed": "false",
	})

	rec := doRequest(t, router, "GET", "/v3/lists/team@example.com/members/pages?subscribed=false", nil)
	assertStatus(t, rec, http.StatusOK)

	var resp memberListResponse
	decodeJSON(t, rec, &resp)

	if len(resp.Items) != 1 {
		t.Fatalf("expected 1 unsubscribed member, got %d", len(resp.Items))
	}
	if resp.Items[0].Address != "unsub@example.com" {
		t.Errorf("expected unsub@example.com, got %q", resp.Items[0].Address)
	}
	if resp.Items[0].Subscribed {
		t.Error("expected member to be unsubscribed")
	}
}

// =========================================================================
// CSV Import Tests
// =========================================================================

// newCSVImportRequest creates a multipart/form-data request with a CSV file
// uploaded in the "members" file field, plus optional extra form fields.
func newCSVImportRequest(t *testing.T, url string, csvContent string, fields map[string]string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add file field
	part, err := writer.CreateFormFile("members", "members.csv")
	if err != nil {
		t.Fatalf("failed to create form file: %v", err)
	}
	if _, err := io.Copy(part, bytes.NewReader([]byte(csvContent))); err != nil {
		t.Fatalf("failed to write CSV content: %v", err)
	}

	// Add other fields
	for key, val := range fields {
		if err := writer.WriteField(key, val); err != nil {
			t.Fatalf("failed to write field %q: %v", key, err)
		}
	}
	writer.Close()

	req := httptest.NewRequest("POST", url, &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

// doCSVImport creates the CSV import request and sends it through the router.
func doCSVImport(t *testing.T, router http.Handler, listAddr, csvContent string, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	url := fmt.Sprintf("/v3/lists/%s/members.csv", listAddr)
	req := newCSVImportRequest(t, url, csvContent, fields)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// ---------------------------------------------------------------------------
// 31. Basic CSV import with address and name columns
// ---------------------------------------------------------------------------

func TestCSVImport_BasicAddressAndName(t *testing.T) {
	router := setup(t)

	createListOK(t, router, map[string]string{
		"address": "devs@example.com",
	})

	csv := "address,name\nalice@example.com,Alice\nbob@example.com,Bob\n"

	rec := doCSVImport(t, router, "devs@example.com", csv, nil)
	assertStatus(t, rec, http.StatusOK)

	var resp bulkAddResponse
	decodeJSON(t, rec, &resp)

	if resp.Message != "Mailing list has been updated" {
		t.Errorf("expected message %q, got %q", "Mailing list has been updated", resp.Message)
	}
	if resp.TaskID == "" {
		t.Error("expected non-empty task-id")
	}
	if resp.List.Address != "devs@example.com" {
		t.Errorf("expected list address %q, got %q", "devs@example.com", resp.List.Address)
	}

	// Verify alice was added
	rec = doRequest(t, router, "GET", "/v3/lists/devs@example.com/members/alice@example.com", nil)
	assertStatus(t, rec, http.StatusOK)

	var aliceResp memberResponse
	decodeJSON(t, rec, &aliceResp)

	if aliceResp.Member.Address != "alice@example.com" {
		t.Errorf("expected address %q, got %q", "alice@example.com", aliceResp.Member.Address)
	}
	if aliceResp.Member.Name != "Alice" {
		t.Errorf("expected name %q, got %q", "Alice", aliceResp.Member.Name)
	}

	// Verify bob was added
	rec = doRequest(t, router, "GET", "/v3/lists/devs@example.com/members/bob@example.com", nil)
	assertStatus(t, rec, http.StatusOK)

	var bobResp memberResponse
	decodeJSON(t, rec, &bobResp)

	if bobResp.Member.Name != "Bob" {
		t.Errorf("expected name %q, got %q", "Bob", bobResp.Member.Name)
	}
}

// ---------------------------------------------------------------------------
// 32. CSV import with all columns (address, name, subscribed, vars)
// ---------------------------------------------------------------------------

func TestCSVImport_AllColumns(t *testing.T) {
	router := setup(t)

	createListOK(t, router, map[string]string{
		"address": "devs@example.com",
	})

	csv := "address,name,subscribed,vars\n" +
		"alice@example.com,Alice,true,\"{\"\"role\"\":\"\"admin\"\"}\"\n" +
		"bob@example.com,Bob,false,\"{\"\"role\"\":\"\"user\"\"}\"\n"

	rec := doCSVImport(t, router, "devs@example.com", csv, nil)
	assertStatus(t, rec, http.StatusOK)

	var resp bulkAddResponse
	decodeJSON(t, rec, &resp)

	if resp.Message != "Mailing list has been updated" {
		t.Errorf("expected message %q, got %q", "Mailing list has been updated", resp.Message)
	}

	// Verify alice
	rec = doRequest(t, router, "GET", "/v3/lists/devs@example.com/members/alice@example.com", nil)
	assertStatus(t, rec, http.StatusOK)

	var aliceResp memberResponse
	decodeJSON(t, rec, &aliceResp)

	if !aliceResp.Member.Subscribed {
		t.Error("expected alice to be subscribed")
	}
	if role, ok := aliceResp.Member.Vars["role"].(string); !ok || role != "admin" {
		t.Errorf("expected alice vars.role %q, got %v", "admin", aliceResp.Member.Vars["role"])
	}

	// Verify bob
	rec = doRequest(t, router, "GET", "/v3/lists/devs@example.com/members/bob@example.com", nil)
	assertStatus(t, rec, http.StatusOK)

	var bobResp memberResponse
	decodeJSON(t, rec, &bobResp)

	if bobResp.Member.Subscribed {
		t.Error("expected bob to be unsubscribed")
	}
	if role, ok := bobResp.Member.Vars["role"].(string); !ok || role != "user" {
		t.Errorf("expected bob vars.role %q, got %v", "user", bobResp.Member.Vars["role"])
	}
}

// ---------------------------------------------------------------------------
// 33. CSV import with only address column
// ---------------------------------------------------------------------------

func TestCSVImport_OnlyAddressColumn(t *testing.T) {
	router := setup(t)

	createListOK(t, router, map[string]string{
		"address": "devs@example.com",
	})

	csv := "address\nalice@example.com\nbob@example.com\n"

	rec := doCSVImport(t, router, "devs@example.com", csv, nil)
	assertStatus(t, rec, http.StatusOK)

	var resp bulkAddResponse
	decodeJSON(t, rec, &resp)

	if resp.Message != "Mailing list has been updated" {
		t.Errorf("expected message %q, got %q", "Mailing list has been updated", resp.Message)
	}

	// Verify both members were added with defaults
	rec = doRequest(t, router, "GET", "/v3/lists/devs@example.com/members/alice@example.com", nil)
	assertStatus(t, rec, http.StatusOK)

	var aliceResp memberResponse
	decodeJSON(t, rec, &aliceResp)

	if aliceResp.Member.Address != "alice@example.com" {
		t.Errorf("expected address %q, got %q", "alice@example.com", aliceResp.Member.Address)
	}
	// Default subscribed should be true
	if !aliceResp.Member.Subscribed {
		t.Error("expected default subscribed to be true")
	}
	// Name should be empty
	if aliceResp.Member.Name != "" {
		t.Errorf("expected empty name, got %q", aliceResp.Member.Name)
	}

	rec = doRequest(t, router, "GET", "/v3/lists/devs@example.com/members/bob@example.com", nil)
	assertStatus(t, rec, http.StatusOK)
}

// ---------------------------------------------------------------------------
// 34. CSV import with upsert=true updates existing members
// ---------------------------------------------------------------------------

func TestCSVImport_UpsertTrue(t *testing.T) {
	router := setup(t)

	createListOK(t, router, map[string]string{
		"address": "devs@example.com",
	})

	// Add alice with original name
	addMemberOK(t, router, "devs@example.com", map[string]string{
		"address": "alice@example.com",
		"name":    "Alice Original",
	})

	// CSV import with upsert=true should update alice and add bob
	csv := "address,name\nalice@example.com,Alice Updated\nbob@example.com,Bob New\n"

	rec := doCSVImport(t, router, "devs@example.com", csv, map[string]string{
		"upsert": "true",
	})
	assertStatus(t, rec, http.StatusOK)

	// Verify alice was updated
	rec = doRequest(t, router, "GET", "/v3/lists/devs@example.com/members/alice@example.com", nil)
	assertStatus(t, rec, http.StatusOK)

	var aliceResp memberResponse
	decodeJSON(t, rec, &aliceResp)

	if aliceResp.Member.Name != "Alice Updated" {
		t.Errorf("expected name %q after upsert, got %q", "Alice Updated", aliceResp.Member.Name)
	}

	// Verify bob was added
	rec = doRequest(t, router, "GET", "/v3/lists/devs@example.com/members/bob@example.com", nil)
	assertStatus(t, rec, http.StatusOK)

	var bobResp memberResponse
	decodeJSON(t, rec, &bobResp)

	if bobResp.Member.Name != "Bob New" {
		t.Errorf("expected name %q, got %q", "Bob New", bobResp.Member.Name)
	}
}

// ---------------------------------------------------------------------------
// 35. CSV import with upsert=false skips existing members silently
// ---------------------------------------------------------------------------

func TestCSVImport_UpsertFalse_SkipsExisting(t *testing.T) {
	router := setup(t)

	createListOK(t, router, map[string]string{
		"address": "devs@example.com",
	})

	// Add alice with original name
	addMemberOK(t, router, "devs@example.com", map[string]string{
		"address": "alice@example.com",
		"name":    "Alice Original",
	})

	// CSV import without upsert (default false) should skip alice, add bob
	csv := "address,name\nalice@example.com,Alice Should Not Change\nbob@example.com,Bob New\n"

	rec := doCSVImport(t, router, "devs@example.com", csv, nil)
	assertStatus(t, rec, http.StatusOK)

	// Verify alice was NOT updated (bulk operations skip silently)
	rec = doRequest(t, router, "GET", "/v3/lists/devs@example.com/members/alice@example.com", nil)
	assertStatus(t, rec, http.StatusOK)

	var aliceResp memberResponse
	decodeJSON(t, rec, &aliceResp)

	if aliceResp.Member.Name != "Alice Original" {
		t.Errorf("expected name %q (unchanged), got %q", "Alice Original", aliceResp.Member.Name)
	}

	// Verify bob was added
	rec = doRequest(t, router, "GET", "/v3/lists/devs@example.com/members/bob@example.com", nil)
	assertStatus(t, rec, http.StatusOK)
}

// ---------------------------------------------------------------------------
// 36. CSV import to non-existent list returns 404
// ---------------------------------------------------------------------------

func TestCSVImport_ListNotFound(t *testing.T) {
	router := setup(t)

	csv := "address,name\nalice@example.com,Alice\n"

	rec := doCSVImport(t, router, "nonexistent@example.com", csv, nil)
	assertStatus(t, rec, http.StatusNotFound)

	var resp messageResponse
	decodeJSON(t, rec, &resp)

	if resp.Message == "" {
		t.Error("expected non-empty error message for not found list")
	}
}

// ---------------------------------------------------------------------------
// 37. CSV import with no file returns 400
// ---------------------------------------------------------------------------

func TestCSVImport_NoFile(t *testing.T) {
	router := setup(t)

	createListOK(t, router, map[string]string{
		"address": "devs@example.com",
	})

	// Send a regular multipart request without any file field
	rec := doRequest(t, router, "POST", "/v3/lists/devs@example.com/members.csv", map[string]string{
		"upsert": "true",
	})
	assertStatus(t, rec, http.StatusBadRequest)

	var resp messageResponse
	decodeJSON(t, rec, &resp)

	if resp.Message != "file is required" {
		t.Errorf("expected message %q, got %q", "file is required", resp.Message)
	}
}

// ---------------------------------------------------------------------------
// 38. CSV import correctly updates members_count
// ---------------------------------------------------------------------------

func TestCSVImport_UpdatesMembersCount(t *testing.T) {
	router := setup(t)

	createListOK(t, router, map[string]string{
		"address": "devs@example.com",
	})

	csv := "address,name\nalice@example.com,Alice\nbob@example.com,Bob\ncharlie@example.com,Charlie\n"

	rec := doCSVImport(t, router, "devs@example.com", csv, nil)
	assertStatus(t, rec, http.StatusOK)

	var resp bulkAddResponse
	decodeJSON(t, rec, &resp)

	// The response should include the updated members_count
	if resp.List.MembersCount != 3 {
		t.Errorf("expected members_count 3 in response, got %d", resp.List.MembersCount)
	}

	// Also verify via GET
	rec = doRequest(t, router, "GET", "/v3/lists/devs@example.com", nil)
	assertStatus(t, rec, http.StatusOK)

	var listResp listResponse
	decodeJSON(t, rec, &listResp)

	if listResp.List.MembersCount != 3 {
		t.Errorf("expected members_count 3 via GET, got %d", listResp.List.MembersCount)
	}
}

// ---------------------------------------------------------------------------
// 38b. CSV import with existing members: count only increments for new members
// ---------------------------------------------------------------------------

func TestCSVImport_MembersCountOnlyCountsNew(t *testing.T) {
	router := setup(t)

	createListOK(t, router, map[string]string{
		"address": "devs@example.com",
	})

	// Add alice first
	addMemberOK(t, router, "devs@example.com", map[string]string{
		"address": "alice@example.com",
	})

	// CSV import includes alice (existing) and bob (new), no upsert
	csv := "address,name\nalice@example.com,Alice\nbob@example.com,Bob\n"

	rec := doCSVImport(t, router, "devs@example.com", csv, nil)
	assertStatus(t, rec, http.StatusOK)

	var resp bulkAddResponse
	decodeJSON(t, rec, &resp)

	// Should be 2 total: 1 existing + 1 newly added
	if resp.List.MembersCount != 2 {
		t.Errorf("expected members_count 2, got %d", resp.List.MembersCount)
	}
}

// ---------------------------------------------------------------------------
// 39. CSV import with subscribed form field sets default subscription status
// ---------------------------------------------------------------------------

func TestCSVImport_SubscribedFormFieldDefault(t *testing.T) {
	router := setup(t)

	createListOK(t, router, map[string]string{
		"address": "devs@example.com",
	})

	// CSV has no subscribed column; the form field "subscribed" should set the default
	csv := "address,name\nalice@example.com,Alice\nbob@example.com,Bob\n"

	rec := doCSVImport(t, router, "devs@example.com", csv, map[string]string{
		"subscribed": "false",
	})
	assertStatus(t, rec, http.StatusOK)

	// Verify both members are unsubscribed (inheriting the form field default)
	rec = doRequest(t, router, "GET", "/v3/lists/devs@example.com/members/alice@example.com", nil)
	assertStatus(t, rec, http.StatusOK)

	var aliceResp memberResponse
	decodeJSON(t, rec, &aliceResp)

	if aliceResp.Member.Subscribed {
		t.Error("expected alice to be unsubscribed (from form field default)")
	}

	rec = doRequest(t, router, "GET", "/v3/lists/devs@example.com/members/bob@example.com", nil)
	assertStatus(t, rec, http.StatusOK)

	var bobResp memberResponse
	decodeJSON(t, rec, &bobResp)

	if bobResp.Member.Subscribed {
		t.Error("expected bob to be unsubscribed (from form field default)")
	}
}

// ---------------------------------------------------------------------------
// 39b. CSV row subscribed column overrides form field default
// ---------------------------------------------------------------------------

func TestCSVImport_RowSubscribedOverridesDefault(t *testing.T) {
	router := setup(t)

	createListOK(t, router, map[string]string{
		"address": "devs@example.com",
	})

	// Form field says subscribed=false, but CSV row says subscribed=true for alice
	csv := "address,name,subscribed\nalice@example.com,Alice,true\nbob@example.com,Bob,\n"

	rec := doCSVImport(t, router, "devs@example.com", csv, map[string]string{
		"subscribed": "false",
	})
	assertStatus(t, rec, http.StatusOK)

	// alice should be subscribed (row overrides form default)
	rec = doRequest(t, router, "GET", "/v3/lists/devs@example.com/members/alice@example.com", nil)
	assertStatus(t, rec, http.StatusOK)

	var aliceResp memberResponse
	decodeJSON(t, rec, &aliceResp)

	if !aliceResp.Member.Subscribed {
		t.Error("expected alice to be subscribed (row subscribed=true overrides form default)")
	}

	// bob should be unsubscribed (empty row value, uses form default)
	rec = doRequest(t, router, "GET", "/v3/lists/devs@example.com/members/bob@example.com", nil)
	assertStatus(t, rec, http.StatusOK)

	var bobResp memberResponse
	decodeJSON(t, rec, &bobResp)

	if bobResp.Member.Subscribed {
		t.Error("expected bob to be unsubscribed (empty row value, form default=false)")
	}
}
