package suppression_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bethmaloney/mailgun-mock-api/internal/domain"
	"github.com/bethmaloney/mailgun-mock-api/internal/mock"
	"github.com/bethmaloney/mailgun-mock-api/internal/suppression"
	"github.com/go-chi/chi/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

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
		&domain.Domain{}, &domain.DNSRecord{},
		&suppression.Bounce{}, &suppression.Complaint{},
		&suppression.Unsubscribe{}, &suppression.AllowlistEntry{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

func defaultConfig() *mock.MockConfig {
	return &mock.MockConfig{
		DomainBehavior: mock.DomainBehaviorConfig{
			DomainAutoVerify: true,
			SandboxDomain:    "sandbox123.mailgun.org",
		},
		EventGeneration: mock.EventGenerationConfig{
			AutoDeliver:               true,
			DeliveryDelayMs:           0,
			DefaultDeliveryStatusCode: 250,
			AutoFailRate:              0.0,
		},
	}
}

func setupRouter(db *gorm.DB, cfg *mock.MockConfig) http.Handler {
	dh := domain.NewHandlers(db, cfg)
	sh := suppression.NewHandlers(db)
	r := chi.NewRouter()

	// Domain routes for setup
	r.Route("/v4/domains", func(r chi.Router) {
		r.Post("/", dh.CreateDomain)
	})

	// Suppression routes (mounted under domain_name)
	r.Route("/v3/{domain_name}", func(r chi.Router) {
		// Bounces
		r.Get("/bounces", sh.ListBounces)
		r.Get("/bounces/{address}", sh.GetBounce)
		r.Post("/bounces", sh.CreateBounces)
		r.Post("/bounces/import", sh.ImportBounces)
		r.Delete("/bounces/{address}", sh.DeleteBounce)
		r.Delete("/bounces", sh.ClearBounces)

		// Complaints
		r.Get("/complaints", sh.ListComplaints)
		r.Get("/complaints/{address}", sh.GetComplaint)
		r.Post("/complaints", sh.CreateComplaints)
		r.Post("/complaints/import", sh.ImportComplaints)
		r.Delete("/complaints/{address}", sh.DeleteComplaint)
		r.Delete("/complaints", sh.ClearComplaints)

		// Unsubscribes
		r.Get("/unsubscribes", sh.ListUnsubscribes)
		r.Get("/unsubscribes/{address}", sh.GetUnsubscribe)
		r.Post("/unsubscribes", sh.CreateUnsubscribes)
		r.Post("/unsubscribes/import", sh.ImportUnsubscribes)
		r.Delete("/unsubscribes/{address}", sh.DeleteUnsubscribe)
		r.Delete("/unsubscribes", sh.ClearUnsubscribes)

		// Allowlist (whitelists)
		r.Get("/whitelists", sh.ListAllowlist)
		r.Get("/whitelists/{value}", sh.GetAllowlistEntry)
		r.Post("/whitelists", sh.CreateAllowlistEntry)
		r.Post("/whitelists/import", sh.ImportAllowlist)
		r.Delete("/whitelists/{value}", sh.DeleteAllowlistEntry)
		r.Delete("/whitelists", sh.ClearAllowlist)
	})

	return r
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

func newJSONRequest(t *testing.T, method, url string, body interface{}) *http.Request {
	t.Helper()
	jsonBytes, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("failed to marshal JSON: %v", err)
	}
	req := httptest.NewRequest(method, url, bytes.NewReader(jsonBytes))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder, dest interface{}) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dest); err != nil {
		t.Fatalf("failed to decode response (body=%q): %v", rec.Body.String(), err)
	}
}

func createDomain(t *testing.T, router http.Handler, name string) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := newMultipartRequest(t, "POST", "/v4/domains", map[string]string{"name": name})
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to create domain %q: status %d, body: %s", name, rec.Code, rec.Body.String())
	}
}

func newCSVImportRequest(t *testing.T, url string, csvContent string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", "import.csv")
	if err != nil {
		t.Fatalf("failed to create form file: %v", err)
	}
	_, err = part.Write([]byte(csvContent))
	if err != nil {
		t.Fatalf("failed to write CSV content: %v", err)
	}
	writer.Close()
	req := httptest.NewRequest("POST", url, &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

// ---------------------------------------------------------------------------
// Response Structs for Assertions
// ---------------------------------------------------------------------------

type messageResponse struct {
	Message string `json:"message"`
}

type pagingURLs struct {
	First    string `json:"first"`
	Last     string `json:"last"`
	Next     string `json:"next"`
	Previous string `json:"previous"`
}

type bounceItem struct {
	Address   string `json:"address"`
	Code      string `json:"code"`
	Error     string `json:"error"`
	CreatedAt string `json:"created_at"`
}

type bounceListResponse struct {
	Items  []bounceItem `json:"items"`
	Paging pagingURLs   `json:"paging"`
}

type complaintItem struct {
	Address   string `json:"address"`
	Count     int    `json:"count"`
	CreatedAt string `json:"created_at"`
}

type complaintListResponse struct {
	Items  []complaintItem `json:"items"`
	Paging pagingURLs      `json:"paging"`
}

type unsubscribeItem struct {
	ID        string   `json:"id"`
	Address   string   `json:"address"`
	Tags      []string `json:"tags"`
	CreatedAt string   `json:"created_at"`
}

type unsubscribeListResponse struct {
	Items  []unsubscribeItem `json:"items"`
	Paging pagingURLs        `json:"paging"`
}

type allowlistItem struct {
	Type      string `json:"type"`
	Value     string `json:"value"`
	Reason    string `json:"reason"`
	CreatedAt string `json:"createdAt"`
}

type allowlistListResponse struct {
	Items  []allowlistItem `json:"items"`
	Paging pagingURLs      `json:"paging"`
}

type deleteResponse struct {
	Message string `json:"message"`
	Address string `json:"address"`
}

type deleteAllowlistResponse struct {
	Message string `json:"message"`
	Value   string `json:"value"`
}

// ---------------------------------------------------------------------------
// Convenience: create a fresh router + domain for each test
// ---------------------------------------------------------------------------

const testDomain = "test.example.com"

func setup(t *testing.T) http.Handler {
	t.Helper()
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createDomain(t, router, testDomain)
	return router
}

// ===========================================================================
// Bounces Tests
// ===========================================================================

func TestCreateBounce_FormData(t *testing.T) {
	router := setup(t)

	rec := httptest.NewRecorder()
	req := newMultipartRequest(t, "POST", fmt.Sprintf("/v3/%s/bounces", testDomain), map[string]string{
		"address": "bounce@example.com",
		"code":    "550",
		"error":   "No such mailbox",
	})
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp messageResponse
	decodeJSON(t, rec, &resp)

	if !strings.Contains(resp.Message, "bounces table") {
		t.Errorf("expected message to reference bounces table, got %q", resp.Message)
	}
	if !strings.Contains(resp.Message, "1") {
		t.Errorf("expected message to contain count '1', got %q", resp.Message)
	}
}

func TestCreateBounce_FormData_DefaultCode(t *testing.T) {
	router := setup(t)

	// Create a bounce without specifying code
	rec := httptest.NewRecorder()
	req := newMultipartRequest(t, "POST", fmt.Sprintf("/v3/%s/bounces", testDomain), map[string]string{
		"address": "nocode@example.com",
	})
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	// Retrieve the bounce and verify code defaults to "550"
	getRec := httptest.NewRecorder()
	getReq := httptest.NewRequest("GET", fmt.Sprintf("/v3/%s/bounces/nocode@example.com", testDomain), nil)
	router.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on GET, got %d (body: %s)", getRec.Code, getRec.Body.String())
	}

	var bounce bounceItem
	decodeJSON(t, getRec, &bounce)

	if bounce.Code != "550" {
		t.Errorf("expected default code '550', got %q", bounce.Code)
	}
}

func TestCreateBounce_FormData_MissingAddress(t *testing.T) {
	router := setup(t)

	rec := httptest.NewRecorder()
	req := newMultipartRequest(t, "POST", fmt.Sprintf("/v3/%s/bounces", testDomain), map[string]string{
		"code":  "550",
		"error": "No such mailbox",
	})
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing address, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestCreateBounce_JSONBatch(t *testing.T) {
	router := setup(t)

	bounces := []map[string]string{
		{"address": "a@example.com", "code": "550", "error": "Bad mailbox"},
		{"address": "b@example.com", "code": "552", "error": "Mailbox full"},
		{"address": "c@example.com", "code": "550", "error": "No such user"},
		{"address": "d@example.com", "code": "421", "error": "Try later"},
	}

	rec := httptest.NewRecorder()
	req := newJSONRequest(t, "POST", fmt.Sprintf("/v3/%s/bounces", testDomain), bounces)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp messageResponse
	decodeJSON(t, rec, &resp)

	if !strings.Contains(resp.Message, "4") {
		t.Errorf("expected message to contain count '4', got %q", resp.Message)
	}
	if !strings.Contains(resp.Message, "bounces table") {
		t.Errorf("expected message to reference bounces table, got %q", resp.Message)
	}
}

func TestCreateBounce_JSONBatch_TooLarge(t *testing.T) {
	router := setup(t)

	// Create a batch of 1001 bounces
	bounces := make([]map[string]string, 1001)
	for i := range bounces {
		bounces[i] = map[string]string{
			"address": fmt.Sprintf("user%d@example.com", i),
			"code":    "550",
		}
	}

	rec := httptest.NewRecorder()
	req := newJSONRequest(t, "POST", fmt.Sprintf("/v3/%s/bounces", testDomain), bounces)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for batch > 1000, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp messageResponse
	decodeJSON(t, rec, &resp)

	if !strings.Contains(resp.Message, "1000") {
		t.Errorf("expected error about batch size limit, got %q", resp.Message)
	}
}

func TestGetBounce(t *testing.T) {
	router := setup(t)

	// Create a bounce first
	createRec := httptest.NewRecorder()
	createReq := newMultipartRequest(t, "POST", fmt.Sprintf("/v3/%s/bounces", testDomain), map[string]string{
		"address": "getme@example.com",
		"code":    "550",
		"error":   "No such mailbox",
	})
	router.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("failed to create bounce: status %d, body: %s", createRec.Code, createRec.Body.String())
	}

	// Retrieve it
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", fmt.Sprintf("/v3/%s/bounces/getme@example.com", testDomain), nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var bounce bounceItem
	decodeJSON(t, rec, &bounce)

	if bounce.Address != "getme@example.com" {
		t.Errorf("expected address 'getme@example.com', got %q", bounce.Address)
	}
	if bounce.Code != "550" {
		t.Errorf("expected code '550', got %q", bounce.Code)
	}
	if bounce.Error != "No such mailbox" {
		t.Errorf("expected error 'No such mailbox', got %q", bounce.Error)
	}
	if bounce.CreatedAt == "" {
		t.Error("expected created_at to be non-empty")
	}
	// Verify RFC 2822 format: should contain a day abbreviation
	if !strings.Contains(bounce.CreatedAt, "UTC") && !strings.Contains(bounce.CreatedAt, "GMT") {
		t.Errorf("expected created_at in RFC 2822 format with timezone, got %q", bounce.CreatedAt)
	}
}

func TestGetBounce_NotFound(t *testing.T) {
	router := setup(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", fmt.Sprintf("/v3/%s/bounces/nobody@example.com", testDomain), nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp messageResponse
	decodeJSON(t, rec, &resp)

	if resp.Message != "Address not found in bounces table" {
		t.Errorf("expected 'Address not found in bounces table', got %q", resp.Message)
	}
}

func TestListBounces(t *testing.T) {
	router := setup(t)

	// Create some bounces
	for _, addr := range []string{"alice@example.com", "bob@example.com", "charlie@example.com"} {
		rec := httptest.NewRecorder()
		req := newMultipartRequest(t, "POST", fmt.Sprintf("/v3/%s/bounces", testDomain), map[string]string{
			"address": addr,
			"code":    "550",
		})
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("failed to create bounce for %s: status %d", addr, rec.Code)
		}
	}

	// List them
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", fmt.Sprintf("/v3/%s/bounces", testDomain), nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp bounceListResponse
	decodeJSON(t, rec, &resp)

	if len(resp.Items) != 3 {
		t.Errorf("expected 3 items, got %d", len(resp.Items))
	}

	// Verify paging object is present
	if resp.Paging.First == "" {
		t.Error("expected paging.first to be non-empty")
	}
	if resp.Paging.Last == "" {
		t.Error("expected paging.last to be non-empty")
	}
}

func TestListBounces_Pagination(t *testing.T) {
	router := setup(t)

	// Create 5 bounces
	for i := 0; i < 5; i++ {
		rec := httptest.NewRecorder()
		req := newMultipartRequest(t, "POST", fmt.Sprintf("/v3/%s/bounces", testDomain), map[string]string{
			"address": fmt.Sprintf("user%d@example.com", i),
			"code":    "550",
		})
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("failed to create bounce %d: status %d", i, rec.Code)
		}
	}

	// List with limit=2
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", fmt.Sprintf("/v3/%s/bounces?limit=2", testDomain), nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp bounceListResponse
	decodeJSON(t, rec, &resp)

	if len(resp.Items) != 2 {
		t.Errorf("expected 2 items with limit=2, got %d", len(resp.Items))
	}
}

func TestListBounces_TermFilter(t *testing.T) {
	router := setup(t)

	// Create bounces with different address prefixes
	for _, addr := range []string{"alice@example.com", "alex@example.com", "bob@example.com"} {
		rec := httptest.NewRecorder()
		req := newMultipartRequest(t, "POST", fmt.Sprintf("/v3/%s/bounces", testDomain), map[string]string{
			"address": addr,
			"code":    "550",
		})
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("failed to create bounce for %s: status %d", addr, rec.Code)
		}
	}

	// Filter by prefix "al"
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", fmt.Sprintf("/v3/%s/bounces?term=al", testDomain), nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp bounceListResponse
	decodeJSON(t, rec, &resp)

	if len(resp.Items) != 2 {
		t.Errorf("expected 2 items matching prefix 'al', got %d", len(resp.Items))
	}

	for _, item := range resp.Items {
		if !strings.HasPrefix(item.Address, "al") {
			t.Errorf("expected address to start with 'al', got %q", item.Address)
		}
	}
}

func TestDeleteBounce(t *testing.T) {
	router := setup(t)

	// Create a bounce
	createRec := httptest.NewRecorder()
	createReq := newMultipartRequest(t, "POST", fmt.Sprintf("/v3/%s/bounces", testDomain), map[string]string{
		"address": "delete-me@example.com",
		"code":    "550",
	})
	router.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("failed to create bounce: status %d", createRec.Code)
	}

	// Delete it
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", fmt.Sprintf("/v3/%s/bounces/delete-me@example.com", testDomain), nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp deleteResponse
	decodeJSON(t, rec, &resp)

	if resp.Message != "Bounced addresses for this domain have been removed" {
		t.Errorf("expected delete message, got %q", resp.Message)
	}
	if resp.Address != "delete-me@example.com" {
		t.Errorf("expected address 'delete-me@example.com', got %q", resp.Address)
	}

	// Verify it's gone
	getRec := httptest.NewRecorder()
	getReq := httptest.NewRequest("GET", fmt.Sprintf("/v3/%s/bounces/delete-me@example.com", testDomain), nil)
	router.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusNotFound {
		t.Errorf("expected 404 after deletion, got %d", getRec.Code)
	}
}

func TestDeleteBounce_NotFound(t *testing.T) {
	router := setup(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", fmt.Sprintf("/v3/%s/bounces/nonexistent@example.com", testDomain), nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestClearBounces(t *testing.T) {
	router := setup(t)

	// Create some bounces
	for _, addr := range []string{"a@example.com", "b@example.com"} {
		rec := httptest.NewRecorder()
		req := newMultipartRequest(t, "POST", fmt.Sprintf("/v3/%s/bounces", testDomain), map[string]string{
			"address": addr,
			"code":    "550",
		})
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("failed to create bounce for %s: status %d", addr, rec.Code)
		}
	}

	// Clear all
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", fmt.Sprintf("/v3/%s/bounces", testDomain), nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp messageResponse
	decodeJSON(t, rec, &resp)

	if resp.Message != "Bounced addresses for this domain have been removed" {
		t.Errorf("expected clear message, got %q", resp.Message)
	}

	// Verify list is now empty
	listRec := httptest.NewRecorder()
	listReq := httptest.NewRequest("GET", fmt.Sprintf("/v3/%s/bounces", testDomain), nil)
	router.ServeHTTP(listRec, listReq)

	var listResp bounceListResponse
	decodeJSON(t, listRec, &listResp)

	if len(listResp.Items) != 0 {
		t.Errorf("expected 0 items after clear, got %d", len(listResp.Items))
	}
}

func TestImportBounces_CSV(t *testing.T) {
	router := setup(t)

	csv := "address,code,error\nbounce1@example.com,550,Bad mailbox\nbounce2@example.com,552,Mailbox full\n"

	rec := httptest.NewRecorder()
	req := newCSVImportRequest(t, fmt.Sprintf("/v3/%s/bounces/import", testDomain), csv)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp messageResponse
	decodeJSON(t, rec, &resp)

	if !strings.Contains(resp.Message, "file uploaded successfully") {
		t.Errorf("expected upload success message, got %q", resp.Message)
	}
}

func TestCreateBounce_Upsert(t *testing.T) {
	router := setup(t)

	// Create a bounce
	rec1 := httptest.NewRecorder()
	req1 := newMultipartRequest(t, "POST", fmt.Sprintf("/v3/%s/bounces", testDomain), map[string]string{
		"address": "upsert@example.com",
		"code":    "550",
		"error":   "Original error",
	})
	router.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first create failed: status %d, body: %s", rec1.Code, rec1.Body.String())
	}

	// Create the same address again with different error
	rec2 := httptest.NewRecorder()
	req2 := newMultipartRequest(t, "POST", fmt.Sprintf("/v3/%s/bounces", testDomain), map[string]string{
		"address": "upsert@example.com",
		"code":    "552",
		"error":   "Updated error",
	})
	router.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("second create (upsert) failed: status %d, body: %s", rec2.Code, rec2.Body.String())
	}

	// Verify there is only one bounce for this address and it has the updated values
	getRec := httptest.NewRecorder()
	getReq := httptest.NewRequest("GET", fmt.Sprintf("/v3/%s/bounces/upsert@example.com", testDomain), nil)
	router.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", getRec.Code, getRec.Body.String())
	}

	var bounce bounceItem
	decodeJSON(t, getRec, &bounce)

	if bounce.Code != "552" {
		t.Errorf("expected updated code '552', got %q", bounce.Code)
	}
	if bounce.Error != "Updated error" {
		t.Errorf("expected updated error 'Updated error', got %q", bounce.Error)
	}

	// Verify list only has one entry
	listRec := httptest.NewRecorder()
	listReq := httptest.NewRequest("GET", fmt.Sprintf("/v3/%s/bounces", testDomain), nil)
	router.ServeHTTP(listRec, listReq)

	var listResp bounceListResponse
	decodeJSON(t, listRec, &listResp)

	count := 0
	for _, item := range listResp.Items {
		if item.Address == "upsert@example.com" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 entry for upsert@example.com, got %d", count)
	}
}

// ===========================================================================
// Complaints Tests
// ===========================================================================

func TestCreateComplaint_FormData(t *testing.T) {
	router := setup(t)

	rec := httptest.NewRecorder()
	req := newMultipartRequest(t, "POST", fmt.Sprintf("/v3/%s/complaints", testDomain), map[string]string{
		"address": "spam@example.com",
	})
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp messageResponse
	decodeJSON(t, rec, &resp)

	if !strings.Contains(resp.Message, "complaints table") {
		t.Errorf("expected message to reference complaints table, got %q", resp.Message)
	}
	if !strings.Contains(resp.Message, "1") {
		t.Errorf("expected message to contain count '1', got %q", resp.Message)
	}
}

func TestCreateComplaint_JSONBatch(t *testing.T) {
	router := setup(t)

	complaints := []map[string]string{
		{"address": "spam1@example.com"},
		{"address": "spam2@example.com"},
		{"address": "spam3@example.com"},
	}

	rec := httptest.NewRecorder()
	req := newJSONRequest(t, "POST", fmt.Sprintf("/v3/%s/complaints", testDomain), complaints)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp messageResponse
	decodeJSON(t, rec, &resp)

	if !strings.Contains(resp.Message, "3") {
		t.Errorf("expected message to contain count '3', got %q", resp.Message)
	}
	if !strings.Contains(resp.Message, "complaints table") {
		t.Errorf("expected message to reference complaints table, got %q", resp.Message)
	}
}

func TestGetComplaint(t *testing.T) {
	router := setup(t)

	// Create a complaint
	createRec := httptest.NewRecorder()
	createReq := newMultipartRequest(t, "POST", fmt.Sprintf("/v3/%s/complaints", testDomain), map[string]string{
		"address": "getme@example.com",
	})
	router.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("failed to create complaint: status %d", createRec.Code)
	}

	// Get it
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", fmt.Sprintf("/v3/%s/complaints/getme@example.com", testDomain), nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var complaint complaintItem
	decodeJSON(t, rec, &complaint)

	if complaint.Address != "getme@example.com" {
		t.Errorf("expected address 'getme@example.com', got %q", complaint.Address)
	}
	if complaint.CreatedAt == "" {
		t.Error("expected created_at to be non-empty")
	}
	// Count should be present and default to 1
	if complaint.Count < 1 {
		t.Errorf("expected count >= 1, got %d", complaint.Count)
	}
}

func TestGetComplaint_NotFound(t *testing.T) {
	router := setup(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", fmt.Sprintf("/v3/%s/complaints/nobody@example.com", testDomain), nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp messageResponse
	decodeJSON(t, rec, &resp)

	if resp.Message != "No spam complaints found for this address" {
		t.Errorf("expected 'No spam complaints found for this address', got %q", resp.Message)
	}
}

func TestListComplaints(t *testing.T) {
	router := setup(t)

	// Create some complaints
	for _, addr := range []string{"spam1@example.com", "spam2@example.com"} {
		rec := httptest.NewRecorder()
		req := newMultipartRequest(t, "POST", fmt.Sprintf("/v3/%s/complaints", testDomain), map[string]string{
			"address": addr,
		})
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("failed to create complaint for %s: status %d", addr, rec.Code)
		}
	}

	// List them
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", fmt.Sprintf("/v3/%s/complaints", testDomain), nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp complaintListResponse
	decodeJSON(t, rec, &resp)

	if len(resp.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(resp.Items))
	}

	if resp.Paging.First == "" {
		t.Error("expected paging.first to be non-empty")
	}
}

func TestDeleteComplaint(t *testing.T) {
	router := setup(t)

	// Create a complaint
	createRec := httptest.NewRecorder()
	createReq := newMultipartRequest(t, "POST", fmt.Sprintf("/v3/%s/complaints", testDomain), map[string]string{
		"address": "delete-me@example.com",
	})
	router.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("failed to create complaint: status %d", createRec.Code)
	}

	// Delete it
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", fmt.Sprintf("/v3/%s/complaints/delete-me@example.com", testDomain), nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp deleteResponse
	decodeJSON(t, rec, &resp)

	if resp.Address != "delete-me@example.com" {
		t.Errorf("expected address 'delete-me@example.com', got %q", resp.Address)
	}
}

func TestDeleteComplaint_NotFound(t *testing.T) {
	router := setup(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", fmt.Sprintf("/v3/%s/complaints/nonexistent@example.com", testDomain), nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestClearComplaints(t *testing.T) {
	router := setup(t)

	// Create a complaint
	createRec := httptest.NewRecorder()
	createReq := newMultipartRequest(t, "POST", fmt.Sprintf("/v3/%s/complaints", testDomain), map[string]string{
		"address": "spam@example.com",
	})
	router.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("failed to create complaint: status %d", createRec.Code)
	}

	// Clear all
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", fmt.Sprintf("/v3/%s/complaints", testDomain), nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp messageResponse
	decodeJSON(t, rec, &resp)

	if resp.Message != "Complaint addresses for this domain have been removed" {
		t.Errorf("expected clear message, got %q", resp.Message)
	}

	// Verify list is empty
	listRec := httptest.NewRecorder()
	listReq := httptest.NewRequest("GET", fmt.Sprintf("/v3/%s/complaints", testDomain), nil)
	router.ServeHTTP(listRec, listReq)

	var listResp complaintListResponse
	decodeJSON(t, listRec, &listResp)

	if len(listResp.Items) != 0 {
		t.Errorf("expected 0 items after clear, got %d", len(listResp.Items))
	}
}

func TestImportComplaints_CSV(t *testing.T) {
	router := setup(t)

	csv := "address\nspam1@example.com\nspam2@example.com\n"

	rec := httptest.NewRecorder()
	req := newCSVImportRequest(t, fmt.Sprintf("/v3/%s/complaints/import", testDomain), csv)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp messageResponse
	decodeJSON(t, rec, &resp)

	if !strings.Contains(resp.Message, "file uploaded successfully") {
		t.Errorf("expected upload success message, got %q", resp.Message)
	}
}

// ===========================================================================
// Unsubscribes Tests
// ===========================================================================

func TestCreateUnsubscribe_FormData(t *testing.T) {
	router := setup(t)

	rec := httptest.NewRecorder()
	req := newMultipartRequest(t, "POST", fmt.Sprintf("/v3/%s/unsubscribes", testDomain), map[string]string{
		"address": "unsub@example.com",
		"tag":     "newsletter",
	})
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp messageResponse
	decodeJSON(t, rec, &resp)

	if !strings.Contains(resp.Message, "unsubscribes table") || !strings.Contains(resp.Message, "unsubscribe") {
		// The message may say "unsubscribes table" or "unsubscribe table"
		if !strings.Contains(strings.ToLower(resp.Message), "unsubscribe") {
			t.Errorf("expected message to reference unsubscribes, got %q", resp.Message)
		}
	}

	// Verify the tag was stored
	getRec := httptest.NewRecorder()
	getReq := httptest.NewRequest("GET", fmt.Sprintf("/v3/%s/unsubscribes/unsub@example.com", testDomain), nil)
	router.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on GET, got %d (body: %s)", getRec.Code, getRec.Body.String())
	}

	var unsub unsubscribeItem
	decodeJSON(t, getRec, &unsub)

	found := false
	for _, tag := range unsub.Tags {
		if tag == "newsletter" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected tags to contain 'newsletter', got %v", unsub.Tags)
	}
}

func TestCreateUnsubscribe_FormData_DefaultTag(t *testing.T) {
	router := setup(t)

	// Create without specifying tag
	rec := httptest.NewRecorder()
	req := newMultipartRequest(t, "POST", fmt.Sprintf("/v3/%s/unsubscribes", testDomain), map[string]string{
		"address": "notag@example.com",
	})
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	// Retrieve and verify default tag is "*"
	getRec := httptest.NewRecorder()
	getReq := httptest.NewRequest("GET", fmt.Sprintf("/v3/%s/unsubscribes/notag@example.com", testDomain), nil)
	router.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on GET, got %d (body: %s)", getRec.Code, getRec.Body.String())
	}

	var unsub unsubscribeItem
	decodeJSON(t, getRec, &unsub)

	if len(unsub.Tags) == 0 {
		t.Fatal("expected tags to be non-empty, got empty array")
	}

	found := false
	for _, tag := range unsub.Tags {
		if tag == "*" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected default tag '*' in tags, got %v", unsub.Tags)
	}
}

func TestCreateUnsubscribe_JSONBatch(t *testing.T) {
	router := setup(t)

	// JSON batch uses "tags" (plural array)
	unsubs := []map[string]interface{}{
		{"address": "batch1@example.com", "tags": []string{"newsletter", "promo"}},
		{"address": "batch2@example.com", "tags": []string{"alerts"}},
	}

	rec := httptest.NewRecorder()
	req := newJSONRequest(t, "POST", fmt.Sprintf("/v3/%s/unsubscribes", testDomain), unsubs)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp messageResponse
	decodeJSON(t, rec, &resp)

	if !strings.Contains(resp.Message, "2") {
		t.Errorf("expected message to contain count '2', got %q", resp.Message)
	}
}

func TestGetUnsubscribe(t *testing.T) {
	router := setup(t)

	// Create an unsubscribe
	createRec := httptest.NewRecorder()
	createReq := newMultipartRequest(t, "POST", fmt.Sprintf("/v3/%s/unsubscribes", testDomain), map[string]string{
		"address": "getme@example.com",
		"tag":     "news",
	})
	router.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("failed to create unsubscribe: status %d", createRec.Code)
	}

	// Get it
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", fmt.Sprintf("/v3/%s/unsubscribes/getme@example.com", testDomain), nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var unsub unsubscribeItem
	decodeJSON(t, rec, &unsub)

	if unsub.Address != "getme@example.com" {
		t.Errorf("expected address 'getme@example.com', got %q", unsub.Address)
	}
	if unsub.ID == "" {
		t.Error("expected id field to be non-empty")
	}
	if unsub.Tags == nil || len(unsub.Tags) == 0 {
		t.Error("expected tags to be non-empty")
	}
	if unsub.CreatedAt == "" {
		t.Error("expected created_at to be non-empty")
	}
}

func TestGetUnsubscribe_NotFound(t *testing.T) {
	router := setup(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", fmt.Sprintf("/v3/%s/unsubscribes/nobody@example.com", testDomain), nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp messageResponse
	decodeJSON(t, rec, &resp)

	if resp.Message != "Address not found in unsubscribers table" {
		t.Errorf("expected 'Address not found in unsubscribers table', got %q", resp.Message)
	}
}

func TestListUnsubscribes(t *testing.T) {
	router := setup(t)

	// Create unsubscribes
	for _, addr := range []string{"unsub1@example.com", "unsub2@example.com", "unsub3@example.com"} {
		rec := httptest.NewRecorder()
		req := newMultipartRequest(t, "POST", fmt.Sprintf("/v3/%s/unsubscribes", testDomain), map[string]string{
			"address": addr,
		})
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("failed to create unsubscribe for %s: status %d", addr, rec.Code)
		}
	}

	// List
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", fmt.Sprintf("/v3/%s/unsubscribes", testDomain), nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp unsubscribeListResponse
	decodeJSON(t, rec, &resp)

	if len(resp.Items) != 3 {
		t.Errorf("expected 3 items, got %d", len(resp.Items))
	}

	if resp.Paging.First == "" {
		t.Error("expected paging.first to be non-empty")
	}
}

func TestDeleteUnsubscribe(t *testing.T) {
	router := setup(t)

	// Create an unsubscribe
	createRec := httptest.NewRecorder()
	createReq := newMultipartRequest(t, "POST", fmt.Sprintf("/v3/%s/unsubscribes", testDomain), map[string]string{
		"address": "delete-me@example.com",
	})
	router.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("failed to create unsubscribe: status %d", createRec.Code)
	}

	// Delete it
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", fmt.Sprintf("/v3/%s/unsubscribes/delete-me@example.com", testDomain), nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp deleteResponse
	decodeJSON(t, rec, &resp)

	if resp.Address != "delete-me@example.com" {
		t.Errorf("expected address 'delete-me@example.com', got %q", resp.Address)
	}

	// Verify it's gone
	getRec := httptest.NewRecorder()
	getReq := httptest.NewRequest("GET", fmt.Sprintf("/v3/%s/unsubscribes/delete-me@example.com", testDomain), nil)
	router.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusNotFound {
		t.Errorf("expected 404 after deletion, got %d", getRec.Code)
	}
}

func TestClearUnsubscribes(t *testing.T) {
	router := setup(t)

	// Create unsubscribes
	for _, addr := range []string{"a@example.com", "b@example.com"} {
		rec := httptest.NewRecorder()
		req := newMultipartRequest(t, "POST", fmt.Sprintf("/v3/%s/unsubscribes", testDomain), map[string]string{
			"address": addr,
		})
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("failed to create unsubscribe for %s: status %d", addr, rec.Code)
		}
	}

	// Clear all
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", fmt.Sprintf("/v3/%s/unsubscribes", testDomain), nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp messageResponse
	decodeJSON(t, rec, &resp)

	if resp.Message != "Unsubscribe addresses for this domain have been removed" {
		t.Errorf("expected clear message, got %q", resp.Message)
	}

	// Verify list is empty
	listRec := httptest.NewRecorder()
	listReq := httptest.NewRequest("GET", fmt.Sprintf("/v3/%s/unsubscribes", testDomain), nil)
	router.ServeHTTP(listRec, listReq)

	var listResp unsubscribeListResponse
	decodeJSON(t, listRec, &listResp)

	if len(listResp.Items) != 0 {
		t.Errorf("expected 0 items after clear, got %d", len(listResp.Items))
	}
}

func TestImportUnsubscribes_CSV(t *testing.T) {
	router := setup(t)

	csv := "address,tag\nunsub1@example.com,newsletter\nunsub2@example.com,promo\n"

	rec := httptest.NewRecorder()
	req := newCSVImportRequest(t, fmt.Sprintf("/v3/%s/unsubscribes/import", testDomain), csv)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp messageResponse
	decodeJSON(t, rec, &resp)

	if !strings.Contains(resp.Message, "file uploaded successfully") {
		t.Errorf("expected upload success message, got %q", resp.Message)
	}
}

// ===========================================================================
// Allowlist (Whitelists) Tests
// ===========================================================================

func TestCreateAllowlistEntry_Address(t *testing.T) {
	router := setup(t)

	rec := httptest.NewRecorder()
	req := newMultipartRequest(t, "POST", fmt.Sprintf("/v3/%s/whitelists", testDomain), map[string]string{
		"address": "allowed@example.com",
		"reason":  "VIP customer",
	})
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp messageResponse
	decodeJSON(t, rec, &resp)

	if !strings.Contains(resp.Message, "1") {
		t.Errorf("expected message to contain count '1', got %q", resp.Message)
	}

	// Verify it was stored as type "address"
	getRec := httptest.NewRecorder()
	getReq := httptest.NewRequest("GET", fmt.Sprintf("/v3/%s/whitelists/allowed@example.com", testDomain), nil)
	router.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on GET, got %d (body: %s)", getRec.Code, getRec.Body.String())
	}

	var entry allowlistItem
	decodeJSON(t, getRec, &entry)

	if entry.Type != "address" {
		t.Errorf("expected type 'address', got %q", entry.Type)
	}
	if entry.Value != "allowed@example.com" {
		t.Errorf("expected value 'allowed@example.com', got %q", entry.Value)
	}
}

func TestCreateAllowlistEntry_Domain(t *testing.T) {
	router := setup(t)

	rec := httptest.NewRecorder()
	req := newMultipartRequest(t, "POST", fmt.Sprintf("/v3/%s/whitelists", testDomain), map[string]string{
		"domain": "partner.com",
		"reason": "Partner domain",
	})
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	// Verify it was stored as type "domain"
	getRec := httptest.NewRecorder()
	getReq := httptest.NewRequest("GET", fmt.Sprintf("/v3/%s/whitelists/partner.com", testDomain), nil)
	router.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on GET, got %d (body: %s)", getRec.Code, getRec.Body.String())
	}

	var entry allowlistItem
	decodeJSON(t, getRec, &entry)

	if entry.Type != "domain" {
		t.Errorf("expected type 'domain', got %q", entry.Type)
	}
	if entry.Value != "partner.com" {
		t.Errorf("expected value 'partner.com', got %q", entry.Value)
	}
}

func TestCreateAllowlistEntry_BothAddressAndDomain(t *testing.T) {
	router := setup(t)

	// When both address and domain are provided, address takes priority
	rec := httptest.NewRecorder()
	req := newMultipartRequest(t, "POST", fmt.Sprintf("/v3/%s/whitelists", testDomain), map[string]string{
		"address": "priority@example.com",
		"domain":  "example.com",
	})
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	// Retrieve and verify address was used (type should be "address")
	getRec := httptest.NewRecorder()
	getReq := httptest.NewRequest("GET", fmt.Sprintf("/v3/%s/whitelists/priority@example.com", testDomain), nil)
	router.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on GET, got %d (body: %s)", getRec.Code, getRec.Body.String())
	}

	var entry allowlistItem
	decodeJSON(t, getRec, &entry)

	if entry.Type != "address" {
		t.Errorf("expected type 'address' (address takes priority), got %q", entry.Type)
	}
	if entry.Value != "priority@example.com" {
		t.Errorf("expected value 'priority@example.com', got %q", entry.Value)
	}
}

func TestCreateAllowlistEntry_NeitherAddressNorDomain(t *testing.T) {
	router := setup(t)

	rec := httptest.NewRecorder()
	req := newMultipartRequest(t, "POST", fmt.Sprintf("/v3/%s/whitelists", testDomain), map[string]string{
		"reason": "No address or domain provided",
	})
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 when neither address nor domain provided, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestGetAllowlistEntry(t *testing.T) {
	router := setup(t)

	// Create an allowlist entry
	createRec := httptest.NewRecorder()
	createReq := newMultipartRequest(t, "POST", fmt.Sprintf("/v3/%s/whitelists", testDomain), map[string]string{
		"domain": "example.com",
		"reason": "Partner domain",
	})
	router.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("failed to create allowlist entry: status %d, body: %s", createRec.Code, createRec.Body.String())
	}

	// Get it
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", fmt.Sprintf("/v3/%s/whitelists/example.com", testDomain), nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var entry allowlistItem
	decodeJSON(t, rec, &entry)

	if entry.Value != "example.com" {
		t.Errorf("expected value 'example.com', got %q", entry.Value)
	}
	if entry.Reason != "Partner domain" {
		t.Errorf("expected reason 'Partner domain', got %q", entry.Reason)
	}

	// Verify camelCase "createdAt" field is present
	if entry.CreatedAt == "" {
		t.Error("expected createdAt to be non-empty")
	}

	// Also verify the raw JSON uses camelCase, not snake_case
	raw := rec.Body.String()
	if !strings.Contains(raw, `"createdAt"`) {
		t.Errorf("expected JSON to use camelCase 'createdAt', got: %s", raw)
	}
	if strings.Contains(raw, `"created_at"`) {
		t.Errorf("expected JSON NOT to use snake_case 'created_at', but it does: %s", raw)
	}
}

func TestGetAllowlistEntry_NotFound(t *testing.T) {
	router := setup(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", fmt.Sprintf("/v3/%s/whitelists/nonexistent.com", testDomain), nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp messageResponse
	decodeJSON(t, rec, &resp)

	if resp.Message != "Address/Domain not found in allowlist table" {
		t.Errorf("expected 'Address/Domain not found in allowlist table', got %q", resp.Message)
	}
}

func TestListAllowlist(t *testing.T) {
	router := setup(t)

	// Create some allowlist entries
	entries := []map[string]string{
		{"address": "vip@example.com", "reason": "VIP"},
		{"domain": "partner.com", "reason": "Partner"},
	}
	for _, fields := range entries {
		rec := httptest.NewRecorder()
		req := newMultipartRequest(t, "POST", fmt.Sprintf("/v3/%s/whitelists", testDomain), fields)
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("failed to create allowlist entry: status %d, body: %s", rec.Code, rec.Body.String())
		}
	}

	// List them
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", fmt.Sprintf("/v3/%s/whitelists", testDomain), nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp allowlistListResponse
	decodeJSON(t, rec, &resp)

	if len(resp.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(resp.Items))
	}

	if resp.Paging.First == "" {
		t.Error("expected paging.first to be non-empty")
	}
}

func TestDeleteAllowlistEntry(t *testing.T) {
	router := setup(t)

	// Create an allowlist entry
	createRec := httptest.NewRecorder()
	createReq := newMultipartRequest(t, "POST", fmt.Sprintf("/v3/%s/whitelists", testDomain), map[string]string{
		"address": "delete-me@example.com",
	})
	router.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("failed to create allowlist entry: status %d", createRec.Code)
	}

	// Delete it
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", fmt.Sprintf("/v3/%s/whitelists/delete-me@example.com", testDomain), nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp deleteAllowlistResponse
	decodeJSON(t, rec, &resp)

	// Allowlist uses "value" key instead of "address"
	if resp.Value != "delete-me@example.com" {
		t.Errorf("expected value 'delete-me@example.com', got %q", resp.Value)
	}
	if resp.Message != "Allowlist address/domain has been removed" {
		t.Errorf("expected 'Allowlist address/domain has been removed', got %q", resp.Message)
	}

	// Verify the raw JSON uses "value" not "address"
	raw := rec.Body.String()
	if !strings.Contains(raw, `"value"`) {
		t.Errorf("expected JSON to contain 'value' key, got: %s", raw)
	}
}

func TestDeleteAllowlistEntry_NotFound(t *testing.T) {
	router := setup(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", fmt.Sprintf("/v3/%s/whitelists/nonexistent@example.com", testDomain), nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestClearAllowlist(t *testing.T) {
	router := setup(t)

	// Create an allowlist entry
	createRec := httptest.NewRecorder()
	createReq := newMultipartRequest(t, "POST", fmt.Sprintf("/v3/%s/whitelists", testDomain), map[string]string{
		"address": "vip@example.com",
	})
	router.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("failed to create allowlist entry: status %d", createRec.Code)
	}

	// Clear all
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", fmt.Sprintf("/v3/%s/whitelists", testDomain), nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp messageResponse
	decodeJSON(t, rec, &resp)

	if resp.Message != "Allowlist addresses/domains for this domain have been removed" {
		t.Errorf("expected clear message, got %q", resp.Message)
	}

	// Verify list is empty
	listRec := httptest.NewRecorder()
	listReq := httptest.NewRequest("GET", fmt.Sprintf("/v3/%s/whitelists", testDomain), nil)
	router.ServeHTTP(listRec, listReq)

	var listResp allowlistListResponse
	decodeJSON(t, listRec, &listResp)

	if len(listResp.Items) != 0 {
		t.Errorf("expected 0 items after clear, got %d", len(listResp.Items))
	}
}

func TestImportAllowlist_CSV(t *testing.T) {
	router := setup(t)

	csv := "address,domain,reason\nallowed@example.com,,VIP\n,partner.com,Partner\n"

	rec := httptest.NewRecorder()
	req := newCSVImportRequest(t, fmt.Sprintf("/v3/%s/whitelists/import", testDomain), csv)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp messageResponse
	decodeJSON(t, rec, &resp)

	if !strings.Contains(resp.Message, "file uploaded successfully") {
		t.Errorf("expected upload success message, got %q", resp.Message)
	}
}

// ===========================================================================
// Cross-cutting Tests
// ===========================================================================

func TestListBounces_EmptyDomain(t *testing.T) {
	router := setup(t)

	// List bounces on a domain that has no bounces
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", fmt.Sprintf("/v3/%s/bounces", testDomain), nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp bounceListResponse
	decodeJSON(t, rec, &resp)

	// Items should be an empty array, not null
	if resp.Items == nil {
		t.Error("expected items to be an empty array, got nil")
	}
	if len(resp.Items) != 0 {
		t.Errorf("expected 0 items, got %d", len(resp.Items))
	}

	// Verify the raw JSON has items as [] not null
	raw := rec.Body.String()
	if strings.Contains(raw, `"items":null`) || strings.Contains(raw, `"items": null`) {
		t.Errorf("expected items to be [] not null (body: %s)", raw)
	}

	// Paging should still be present
	if resp.Paging.First == "" {
		t.Error("expected paging.first to be non-empty even for empty list")
	}
	if resp.Paging.Last == "" {
		t.Error("expected paging.last to be non-empty even for empty list")
	}
}
