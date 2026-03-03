package subaccount_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/bethmaloney/mailgun-mock-api/internal/mock"
	"github.com/bethmaloney/mailgun-mock-api/internal/subaccount"
	"github.com/go-chi/chi/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// Response structs for JSON decoding
// ---------------------------------------------------------------------------

type featureValue struct {
	Enabled bool `json:"enabled"`
}

type featuresObj struct {
	EmailPreview    featureValue `json:"email_preview"`
	InboxPlacement  featureValue `json:"inbox_placement"`
	Sending         featureValue `json:"sending"`
	Validations     featureValue `json:"validations"`
	ValidationsBulk featureValue `json:"validations_bulk"`
}

type subaccountJSON struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Status    string       `json:"status"`
	CreatedAt string       `json:"created_at"`
	UpdatedAt string       `json:"updated_at"`
	Features  *featuresObj `json:"features,omitempty"`
}

type createSubaccountResponse struct {
	Subaccount subaccountJSON `json:"subaccount"`
}

type getSubaccountResponse struct {
	Subaccount subaccountJSON `json:"subaccount"`
}

type listSubaccountsResponse struct {
	Subaccounts []subaccountJSON `json:"subaccounts"`
	Total       int              `json:"total"`
}

type sendingLimitResponse struct {
	Limit   int    `json:"limit"`
	Current int    `json:"current"`
	Period  string `json:"period"`
}

type successResponse struct {
	Success bool `json:"success"`
}

type updateFeaturesResponse struct {
	Features featuresObj `json:"features"`
}

// ---------------------------------------------------------------------------
// Test Helpers
// ---------------------------------------------------------------------------

// setupTestDB creates an in-memory SQLite database for testing.
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}
	if err := db.AutoMigrate(
		&subaccount.Subaccount{},
		&subaccount.SendingLimit{},
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
		Authentication: mock.AuthenticationConfig{
			AuthMode:  "accept_any",
			SigningKey: "key-mock-signing-key-000000000000",
		},
	}
}

func setupRouter(db *gorm.DB, cfg *mock.MockConfig) http.Handler {
	sh := subaccount.NewHandlers(db)
	r := chi.NewRouter()

	r.Route("/v5/accounts/subaccounts", func(r chi.Router) {
		r.Post("/", sh.CreateSubaccount)
		r.Get("/", sh.ListSubaccounts)
		r.Delete("/", sh.DeleteSubaccount) // uses X-Mailgun-On-Behalf-Of header
		r.Get("/{subaccount_id}", sh.GetSubaccount)
		r.Post("/{subaccount_id}/disable", sh.DisableSubaccount)
		r.Post("/{subaccount_id}/enable", sh.EnableSubaccount)
		r.Get("/{subaccount_id}/limit/custom/monthly", sh.GetSendingLimit)
		r.Put("/{subaccount_id}/limit/custom/monthly", sh.SetSendingLimit)
		r.Delete("/{subaccount_id}/limit/custom/monthly", sh.RemoveSendingLimit)
		r.Put("/{subaccount_id}/features", sh.UpdateFeatures)
	})

	return r
}

func setup(t *testing.T) http.Handler {
	t.Helper()
	db := setupTestDB(t)
	cfg := defaultConfig()
	return setupRouter(db, cfg)
}

type fieldPair struct {
	key   string
	value string
}

func doFormURLEncoded(t *testing.T, router http.Handler, method, urlStr string, fields []fieldPair) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	form := url.Values{}
	for _, f := range fields {
		form.Add(f.key, f.value)
	}
	req := httptest.NewRequest(method, urlStr, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	router.ServeHTTP(rec, req)
	return rec
}

func doRequest(t *testing.T, router http.Handler, method, urlStr string, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	var req *http.Request
	if fields != nil {
		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)
		for key, val := range fields {
			writer.WriteField(key, val)
		}
		writer.Close()
		req = httptest.NewRequest(method, urlStr, &buf)
		req.Header.Set("Content-Type", writer.FormDataContentType())
	} else {
		req = httptest.NewRequest(method, urlStr, nil)
	}
	router.ServeHTTP(rec, req)
	return rec
}

func doRequestWithHeaders(t *testing.T, router http.Handler, method, urlStr string, fields map[string]string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	var req *http.Request
	if fields != nil {
		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)
		for key, val := range fields {
			writer.WriteField(key, val)
		}
		writer.Close()
		req = httptest.NewRequest(method, urlStr, &buf)
		req.Header.Set("Content-Type", writer.FormDataContentType())
	} else {
		req = httptest.NewRequest(method, urlStr, nil)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
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

// createSubaccountHelper creates a subaccount via multipart form and returns the response.
func createSubaccountHelper(t *testing.T, router http.Handler, name string) createSubaccountResponse {
	t.Helper()
	rec := doRequest(t, router, http.MethodPost, "/v5/accounts/subaccounts", map[string]string{
		"name": name,
	})
	assertStatus(t, rec, http.StatusOK)

	var resp createSubaccountResponse
	decodeJSON(t, rec, &resp)
	return resp
}

// is24CharHex checks if a string is exactly 24 hex characters.
func is24CharHex(s string) bool {
	matched, _ := regexp.MatchString(`^[0-9a-f]{24}$`, s)
	return matched
}

// isRFC1123Time checks if a string parses as RFC1123 format.
func isRFC1123Time(s string) bool {
	_, err := time.Parse(time.RFC1123, s)
	return err == nil
}

// =========================================================================
// Create Subaccount Tests
// =========================================================================

// ---------------------------------------------------------------------------
// 1. TestCreateSubaccount_Basic -- create with form-data, verify 200,
//    24-char hex ID, status="open", features have sending=true and rest false,
//    timestamps in RFC1123 format
// ---------------------------------------------------------------------------

func TestCreateSubaccount_Basic(t *testing.T) {
	router := setup(t)

	rec := doRequest(t, router, http.MethodPost, "/v5/accounts/subaccounts", map[string]string{
		"name": "Test Subaccount",
	})
	assertStatus(t, rec, http.StatusOK)

	var resp createSubaccountResponse
	decodeJSON(t, rec, &resp)

	sa := resp.Subaccount

	// Verify ID is a 24-character hex string
	if !is24CharHex(sa.ID) {
		t.Errorf("expected 24-char hex ID, got %q (len=%d)", sa.ID, len(sa.ID))
	}

	// Verify name
	if sa.Name != "Test Subaccount" {
		t.Errorf("expected name %q, got %q", "Test Subaccount", sa.Name)
	}

	// Verify status is "open"
	if sa.Status != "open" {
		t.Errorf("expected status %q, got %q", "open", sa.Status)
	}

	// Verify timestamps are in RFC1123 format
	if !isRFC1123Time(sa.CreatedAt) {
		t.Errorf("expected created_at in RFC1123 format, got %q", sa.CreatedAt)
	}
	if !isRFC1123Time(sa.UpdatedAt) {
		t.Errorf("expected updated_at in RFC1123 format, got %q", sa.UpdatedAt)
	}

	// Verify features: sending=true, all others=false
	if sa.Features == nil {
		t.Fatal("expected features to be present in create response")
	}
	if !sa.Features.Sending.Enabled {
		t.Error("expected features.sending.enabled=true")
	}
	if sa.Features.EmailPreview.Enabled {
		t.Error("expected features.email_preview.enabled=false")
	}
	if sa.Features.InboxPlacement.Enabled {
		t.Error("expected features.inbox_placement.enabled=false")
	}
	if sa.Features.Validations.Enabled {
		t.Error("expected features.validations.enabled=false")
	}
	if sa.Features.ValidationsBulk.Enabled {
		t.Error("expected features.validations_bulk.enabled=false")
	}
}

// ---------------------------------------------------------------------------
// 2. TestCreateSubaccount_URLEncoded -- create via URL-encoded form body
// ---------------------------------------------------------------------------

func TestCreateSubaccount_URLEncoded(t *testing.T) {
	router := setup(t)

	rec := doFormURLEncoded(t, router, http.MethodPost, "/v5/accounts/subaccounts", []fieldPair{
		{key: "name", value: "URL Encoded Subaccount"},
	})
	assertStatus(t, rec, http.StatusOK)

	var resp createSubaccountResponse
	decodeJSON(t, rec, &resp)

	if resp.Subaccount.Name != "URL Encoded Subaccount" {
		t.Errorf("expected name %q, got %q", "URL Encoded Subaccount", resp.Subaccount.Name)
	}
	if resp.Subaccount.Status != "open" {
		t.Errorf("expected status %q, got %q", "open", resp.Subaccount.Status)
	}
	if !is24CharHex(resp.Subaccount.ID) {
		t.Errorf("expected 24-char hex ID, got %q", resp.Subaccount.ID)
	}
}

// ---------------------------------------------------------------------------
// 3. TestCreateSubaccount_QueryParam -- create with name as query parameter
// ---------------------------------------------------------------------------

func TestCreateSubaccount_QueryParam(t *testing.T) {
	router := setup(t)

	rec := doRequest(t, router, http.MethodPost, "/v5/accounts/subaccounts?name=QueryParamSubaccount", nil)
	assertStatus(t, rec, http.StatusOK)

	var resp createSubaccountResponse
	decodeJSON(t, rec, &resp)

	if resp.Subaccount.Name != "QueryParamSubaccount" {
		t.Errorf("expected name %q, got %q", "QueryParamSubaccount", resp.Subaccount.Name)
	}
	if resp.Subaccount.Status != "open" {
		t.Errorf("expected status %q, got %q", "open", resp.Subaccount.Status)
	}
}

// ---------------------------------------------------------------------------
// 4. TestCreateSubaccount_MissingName -- verify 400 error
// ---------------------------------------------------------------------------

func TestCreateSubaccount_MissingName(t *testing.T) {
	router := setup(t)

	// No name field at all
	rec := doRequest(t, router, http.MethodPost, "/v5/accounts/subaccounts", map[string]string{})
	assertStatus(t, rec, http.StatusBadRequest)
	assertMessage(t, rec, "Bad request")
}

// =========================================================================
// List Subaccounts Tests
// =========================================================================

// ---------------------------------------------------------------------------
// 5. TestListSubaccounts_Empty -- empty list returns { "subaccounts": [], "total": 0 }
// ---------------------------------------------------------------------------

func TestListSubaccounts_Empty(t *testing.T) {
	router := setup(t)

	rec := doRequest(t, router, http.MethodGet, "/v5/accounts/subaccounts", nil)
	assertStatus(t, rec, http.StatusOK)

	var resp listSubaccountsResponse
	decodeJSON(t, rec, &resp)

	if resp.Total != 0 {
		t.Errorf("expected total 0, got %d", resp.Total)
	}
	if resp.Subaccounts == nil {
		t.Fatal("expected subaccounts to be a non-nil empty array")
	}
	if len(resp.Subaccounts) != 0 {
		t.Errorf("expected 0 subaccounts, got %d", len(resp.Subaccounts))
	}
}

// ---------------------------------------------------------------------------
// 6. TestListSubaccounts_Multiple -- create several, verify all returned with
//    correct total
// ---------------------------------------------------------------------------

func TestListSubaccounts_Multiple(t *testing.T) {
	router := setup(t)

	// Create three subaccounts
	createSubaccountHelper(t, router, "Alpha")
	createSubaccountHelper(t, router, "Beta")
	createSubaccountHelper(t, router, "Gamma")

	rec := doRequest(t, router, http.MethodGet, "/v5/accounts/subaccounts?limit=100", nil)
	assertStatus(t, rec, http.StatusOK)

	var resp listSubaccountsResponse
	decodeJSON(t, rec, &resp)

	if resp.Total != 3 {
		t.Errorf("expected total 3, got %d", resp.Total)
	}
	if len(resp.Subaccounts) != 3 {
		t.Errorf("expected 3 subaccounts, got %d", len(resp.Subaccounts))
	}

	// Verify all names are present
	names := map[string]bool{}
	for _, sa := range resp.Subaccounts {
		names[sa.Name] = true
	}
	for _, expected := range []string{"Alpha", "Beta", "Gamma"} {
		if !names[expected] {
			t.Errorf("expected subaccount %q in list", expected)
		}
	}
}

// ---------------------------------------------------------------------------
// 7. TestListSubaccounts_Pagination -- test skip/limit params
// ---------------------------------------------------------------------------

func TestListSubaccounts_Pagination(t *testing.T) {
	router := setup(t)

	// Create five subaccounts
	for i := 0; i < 5; i++ {
		createSubaccountHelper(t, router, fmt.Sprintf("Sub%d", i))
	}

	t.Run("limit=2 returns 2 items", func(t *testing.T) {
		rec := doRequest(t, router, http.MethodGet, "/v5/accounts/subaccounts?limit=2", nil)
		assertStatus(t, rec, http.StatusOK)

		var resp listSubaccountsResponse
		decodeJSON(t, rec, &resp)

		if len(resp.Subaccounts) != 2 {
			t.Errorf("expected 2 subaccounts with limit=2, got %d", len(resp.Subaccounts))
		}
		// Total should still reflect all records
		if resp.Total != 5 {
			t.Errorf("expected total 5, got %d", resp.Total)
		}
	})

	t.Run("skip=3,limit=10 returns remaining items", func(t *testing.T) {
		rec := doRequest(t, router, http.MethodGet, "/v5/accounts/subaccounts?skip=3&limit=10", nil)
		assertStatus(t, rec, http.StatusOK)

		var resp listSubaccountsResponse
		decodeJSON(t, rec, &resp)

		if len(resp.Subaccounts) != 2 {
			t.Errorf("expected 2 subaccounts with skip=3, got %d", len(resp.Subaccounts))
		}
		if resp.Total != 5 {
			t.Errorf("expected total 5, got %d", resp.Total)
		}
	})

	t.Run("skip beyond total returns empty", func(t *testing.T) {
		rec := doRequest(t, router, http.MethodGet, "/v5/accounts/subaccounts?skip=100", nil)
		assertStatus(t, rec, http.StatusOK)

		var resp listSubaccountsResponse
		decodeJSON(t, rec, &resp)

		if len(resp.Subaccounts) != 0 {
			t.Errorf("expected 0 subaccounts with skip=100, got %d", len(resp.Subaccounts))
		}
		if resp.Total != 5 {
			t.Errorf("expected total 5, got %d", resp.Total)
		}
	})
}

// ---------------------------------------------------------------------------
// 8. TestListSubaccounts_SortAsc -- sort=asc orders by name ascending
// ---------------------------------------------------------------------------

func TestListSubaccounts_SortAsc(t *testing.T) {
	router := setup(t)

	createSubaccountHelper(t, router, "Charlie")
	createSubaccountHelper(t, router, "Alice")
	createSubaccountHelper(t, router, "Bob")

	rec := doRequest(t, router, http.MethodGet, "/v5/accounts/subaccounts?sort=asc&limit=100", nil)
	assertStatus(t, rec, http.StatusOK)

	var resp listSubaccountsResponse
	decodeJSON(t, rec, &resp)

	if len(resp.Subaccounts) != 3 {
		t.Fatalf("expected 3 subaccounts, got %d", len(resp.Subaccounts))
	}
	if resp.Subaccounts[0].Name != "Alice" {
		t.Errorf("expected first subaccount to be Alice, got %q", resp.Subaccounts[0].Name)
	}
	if resp.Subaccounts[1].Name != "Bob" {
		t.Errorf("expected second subaccount to be Bob, got %q", resp.Subaccounts[1].Name)
	}
	if resp.Subaccounts[2].Name != "Charlie" {
		t.Errorf("expected third subaccount to be Charlie, got %q", resp.Subaccounts[2].Name)
	}
}

// ---------------------------------------------------------------------------
// 9. TestListSubaccounts_SortDesc -- sort=desc orders by name descending
// ---------------------------------------------------------------------------

func TestListSubaccounts_SortDesc(t *testing.T) {
	router := setup(t)

	createSubaccountHelper(t, router, "Charlie")
	createSubaccountHelper(t, router, "Alice")
	createSubaccountHelper(t, router, "Bob")

	rec := doRequest(t, router, http.MethodGet, "/v5/accounts/subaccounts?sort=desc&limit=100", nil)
	assertStatus(t, rec, http.StatusOK)

	var resp listSubaccountsResponse
	decodeJSON(t, rec, &resp)

	if len(resp.Subaccounts) != 3 {
		t.Fatalf("expected 3 subaccounts, got %d", len(resp.Subaccounts))
	}
	if resp.Subaccounts[0].Name != "Charlie" {
		t.Errorf("expected first subaccount to be Charlie, got %q", resp.Subaccounts[0].Name)
	}
	if resp.Subaccounts[1].Name != "Bob" {
		t.Errorf("expected second subaccount to be Bob, got %q", resp.Subaccounts[1].Name)
	}
	if resp.Subaccounts[2].Name != "Alice" {
		t.Errorf("expected third subaccount to be Alice, got %q", resp.Subaccounts[2].Name)
	}
}

// ---------------------------------------------------------------------------
// 10. TestListSubaccounts_Filter -- filter by partial name match
// ---------------------------------------------------------------------------

func TestListSubaccounts_Filter(t *testing.T) {
	router := setup(t)

	createSubaccountHelper(t, router, "Production App")
	createSubaccountHelper(t, router, "Staging App")
	createSubaccountHelper(t, router, "Development Environment")

	rec := doRequest(t, router, http.MethodGet, "/v5/accounts/subaccounts?filter=App&limit=100", nil)
	assertStatus(t, rec, http.StatusOK)

	var resp listSubaccountsResponse
	decodeJSON(t, rec, &resp)

	if resp.Total != 2 {
		t.Errorf("expected total 2 matching 'App', got %d", resp.Total)
	}
	if len(resp.Subaccounts) != 2 {
		t.Errorf("expected 2 subaccounts matching 'App', got %d", len(resp.Subaccounts))
	}

	// Verify none of the returned items is "Development Environment"
	for _, sa := range resp.Subaccounts {
		if sa.Name == "Development Environment" {
			t.Error("did not expect 'Development Environment' to match filter 'App'")
		}
	}
}

// ---------------------------------------------------------------------------
// 11. TestListSubaccounts_EnabledFilter -- enabled=true returns only open,
//     enabled=false returns only disabled
// ---------------------------------------------------------------------------

func TestListSubaccounts_EnabledFilter(t *testing.T) {
	router := setup(t)

	// Create two subaccounts, then disable one
	resp1 := createSubaccountHelper(t, router, "Active Sub")
	resp2 := createSubaccountHelper(t, router, "Disabled Sub")

	// Disable the second subaccount
	rec := doRequest(t, router, http.MethodPost, fmt.Sprintf("/v5/accounts/subaccounts/%s/disable", resp2.Subaccount.ID), nil)
	assertStatus(t, rec, http.StatusOK)

	t.Run("enabled=true returns only open", func(t *testing.T) {
		rec := doRequest(t, router, http.MethodGet, "/v5/accounts/subaccounts?enabled=true&limit=100", nil)
		assertStatus(t, rec, http.StatusOK)

		var resp listSubaccountsResponse
		decodeJSON(t, rec, &resp)

		if resp.Total != 1 {
			t.Errorf("expected total 1 open subaccount, got %d", resp.Total)
		}
		if len(resp.Subaccounts) != 1 {
			t.Fatalf("expected 1 subaccount, got %d", len(resp.Subaccounts))
		}
		if resp.Subaccounts[0].Name != "Active Sub" {
			t.Errorf("expected active sub, got %q", resp.Subaccounts[0].Name)
		}
	})

	t.Run("enabled=false returns only disabled", func(t *testing.T) {
		rec := doRequest(t, router, http.MethodGet, "/v5/accounts/subaccounts?enabled=false&limit=100", nil)
		assertStatus(t, rec, http.StatusOK)

		var resp listSubaccountsResponse
		decodeJSON(t, rec, &resp)

		if resp.Total != 1 {
			t.Errorf("expected total 1 disabled subaccount, got %d", resp.Total)
		}
		if len(resp.Subaccounts) != 1 {
			t.Fatalf("expected 1 subaccount, got %d", len(resp.Subaccounts))
		}
		if resp.Subaccounts[0].Name != "Disabled Sub" {
			t.Errorf("expected disabled sub, got %q", resp.Subaccounts[0].Name)
		}
	})

	// Ensure both IDs are used so the compiler doesn't complain
	_ = resp1.Subaccount.ID
}

// ---------------------------------------------------------------------------
// 12. TestListSubaccounts_NoFeaturesInList -- verify list responses don't
//     include features object
// ---------------------------------------------------------------------------

func TestListSubaccounts_NoFeaturesInList(t *testing.T) {
	router := setup(t)

	createSubaccountHelper(t, router, "Features Test Sub")

	rec := doRequest(t, router, http.MethodGet, "/v5/accounts/subaccounts?limit=100", nil)
	assertStatus(t, rec, http.StatusOK)

	var resp listSubaccountsResponse
	decodeJSON(t, rec, &resp)

	if len(resp.Subaccounts) != 1 {
		t.Fatalf("expected 1 subaccount, got %d", len(resp.Subaccounts))
	}

	// Features should be nil/omitted in list responses
	if resp.Subaccounts[0].Features != nil {
		t.Error("expected features to be omitted in list response")
	}

	// Also verify using raw JSON to be thorough
	var rawResp map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &rawResp); err != nil {
		t.Fatalf("failed to decode raw response: %v", err)
	}
	var rawSubaccounts []map[string]json.RawMessage
	if err := json.Unmarshal(rawResp["subaccounts"], &rawSubaccounts); err != nil {
		t.Fatalf("failed to decode subaccounts array: %v", err)
	}
	if len(rawSubaccounts) > 0 {
		if _, hasFeatures := rawSubaccounts[0]["features"]; hasFeatures {
			t.Error("expected 'features' key to be absent from list response items")
		}
	}
}

// =========================================================================
// Get Subaccount Tests
// =========================================================================

// ---------------------------------------------------------------------------
// 13. TestGetSubaccount_Basic -- get existing, verify full object with features
// ---------------------------------------------------------------------------

func TestGetSubaccount_Basic(t *testing.T) {
	router := setup(t)

	created := createSubaccountHelper(t, router, "Get Me")

	rec := doRequest(t, router, http.MethodGet, "/v5/accounts/subaccounts/"+created.Subaccount.ID, nil)
	assertStatus(t, rec, http.StatusOK)

	var resp getSubaccountResponse
	decodeJSON(t, rec, &resp)

	if resp.Subaccount.ID != created.Subaccount.ID {
		t.Errorf("expected id %q, got %q", created.Subaccount.ID, resp.Subaccount.ID)
	}
	if resp.Subaccount.Name != "Get Me" {
		t.Errorf("expected name %q, got %q", "Get Me", resp.Subaccount.Name)
	}
	if resp.Subaccount.Status != "open" {
		t.Errorf("expected status %q, got %q", "open", resp.Subaccount.Status)
	}

	// Verify features are included in get response
	if resp.Subaccount.Features == nil {
		t.Fatal("expected features to be present in get response")
	}
	if !resp.Subaccount.Features.Sending.Enabled {
		t.Error("expected features.sending.enabled=true")
	}
	if resp.Subaccount.Features.EmailPreview.Enabled {
		t.Error("expected features.email_preview.enabled=false")
	}

	// Verify timestamps
	if !isRFC1123Time(resp.Subaccount.CreatedAt) {
		t.Errorf("expected created_at in RFC1123 format, got %q", resp.Subaccount.CreatedAt)
	}
	if !isRFC1123Time(resp.Subaccount.UpdatedAt) {
		t.Errorf("expected updated_at in RFC1123 format, got %q", resp.Subaccount.UpdatedAt)
	}
}

// ---------------------------------------------------------------------------
// 14. TestGetSubaccount_NotFound -- 404 for non-existent ID
// ---------------------------------------------------------------------------

func TestGetSubaccount_NotFound(t *testing.T) {
	router := setup(t)

	rec := doRequest(t, router, http.MethodGet, "/v5/accounts/subaccounts/aabbccddee112233aabbccdd", nil)
	assertStatus(t, rec, http.StatusNotFound)
	assertMessage(t, rec, "Not Found")
}

// =========================================================================
// Disable/Enable Tests
// =========================================================================

// ---------------------------------------------------------------------------
// 15. TestDisableSubaccount_Basic -- disable sets status to "disabled"
// ---------------------------------------------------------------------------

func TestDisableSubaccount_Basic(t *testing.T) {
	router := setup(t)

	created := createSubaccountHelper(t, router, "To Disable")

	rec := doRequest(t, router, http.MethodPost, fmt.Sprintf("/v5/accounts/subaccounts/%s/disable", created.Subaccount.ID), nil)
	assertStatus(t, rec, http.StatusOK)

	var resp getSubaccountResponse
	decodeJSON(t, rec, &resp)

	if resp.Subaccount.Status != "disabled" {
		t.Errorf("expected status %q, got %q", "disabled", resp.Subaccount.Status)
	}
	if resp.Subaccount.ID != created.Subaccount.ID {
		t.Errorf("expected id %q, got %q", created.Subaccount.ID, resp.Subaccount.ID)
	}
}

// ---------------------------------------------------------------------------
// 16. TestDisableSubaccount_AlreadyDisabled -- returns 400
// ---------------------------------------------------------------------------

func TestDisableSubaccount_AlreadyDisabled(t *testing.T) {
	router := setup(t)

	created := createSubaccountHelper(t, router, "Already Disabled")

	// Disable once
	rec := doRequest(t, router, http.MethodPost, fmt.Sprintf("/v5/accounts/subaccounts/%s/disable", created.Subaccount.ID), nil)
	assertStatus(t, rec, http.StatusOK)

	// Disable again -- should get 400
	rec = doRequest(t, router, http.MethodPost, fmt.Sprintf("/v5/accounts/subaccounts/%s/disable", created.Subaccount.ID), nil)
	assertStatus(t, rec, http.StatusBadRequest)
	assertMessage(t, rec, "subaccount is already disabled")
}

// ---------------------------------------------------------------------------
// 17. TestDisableSubaccount_NotFound -- returns 404
// ---------------------------------------------------------------------------

func TestDisableSubaccount_NotFound(t *testing.T) {
	router := setup(t)

	rec := doRequest(t, router, http.MethodPost, "/v5/accounts/subaccounts/aabbccddee112233aabbccdd/disable", nil)
	assertStatus(t, rec, http.StatusNotFound)
	assertMessage(t, rec, "Not Found")
}

// ---------------------------------------------------------------------------
// 18. TestEnableSubaccount_Basic -- enable sets status back to "open"
// ---------------------------------------------------------------------------

func TestEnableSubaccount_Basic(t *testing.T) {
	router := setup(t)

	created := createSubaccountHelper(t, router, "To Enable")

	// Disable first
	rec := doRequest(t, router, http.MethodPost, fmt.Sprintf("/v5/accounts/subaccounts/%s/disable", created.Subaccount.ID), nil)
	assertStatus(t, rec, http.StatusOK)

	// Enable
	rec = doRequest(t, router, http.MethodPost, fmt.Sprintf("/v5/accounts/subaccounts/%s/enable", created.Subaccount.ID), nil)
	assertStatus(t, rec, http.StatusOK)

	var resp getSubaccountResponse
	decodeJSON(t, rec, &resp)

	if resp.Subaccount.Status != "open" {
		t.Errorf("expected status %q, got %q", "open", resp.Subaccount.Status)
	}
}

// ---------------------------------------------------------------------------
// 19. TestEnableSubaccount_NotFound -- returns 404
// ---------------------------------------------------------------------------

func TestEnableSubaccount_NotFound(t *testing.T) {
	router := setup(t)

	rec := doRequest(t, router, http.MethodPost, "/v5/accounts/subaccounts/aabbccddee112233aabbccdd/enable", nil)
	assertStatus(t, rec, http.StatusNotFound)
	assertMessage(t, rec, "Not Found")
}

// =========================================================================
// Delete Tests
// =========================================================================

// ---------------------------------------------------------------------------
// 20. TestDeleteSubaccount_Basic -- delete via X-Mailgun-On-Behalf-Of header
// ---------------------------------------------------------------------------

func TestDeleteSubaccount_Basic(t *testing.T) {
	router := setup(t)

	created := createSubaccountHelper(t, router, "To Delete")

	rec := doRequestWithHeaders(t, router, http.MethodDelete, "/v5/accounts/subaccounts", nil, map[string]string{
		"X-Mailgun-On-Behalf-Of": created.Subaccount.ID,
	})
	assertStatus(t, rec, http.StatusOK)
	assertMessage(t, rec, "Subaccount successfully deleted")
}

// ---------------------------------------------------------------------------
// 21. TestDeleteSubaccount_MissingHeader -- returns 400 when header missing
// ---------------------------------------------------------------------------

func TestDeleteSubaccount_MissingHeader(t *testing.T) {
	router := setup(t)

	// No X-Mailgun-On-Behalf-Of header
	rec := doRequest(t, router, http.MethodDelete, "/v5/accounts/subaccounts", nil)
	assertStatus(t, rec, http.StatusBadRequest)
	assertMessage(t, rec, "Bad request")
}

// ---------------------------------------------------------------------------
// 22. TestDeleteSubaccount_InvalidID -- returns 400 when ID doesn't exist
// ---------------------------------------------------------------------------

func TestDeleteSubaccount_InvalidID(t *testing.T) {
	router := setup(t)

	rec := doRequestWithHeaders(t, router, http.MethodDelete, "/v5/accounts/subaccounts", nil, map[string]string{
		"X-Mailgun-On-Behalf-Of": "aabbccddee112233aabbccdd",
	})
	assertStatus(t, rec, http.StatusBadRequest)
	assertMessage(t, rec, "Bad request")
}

// ---------------------------------------------------------------------------
// 23. TestDeleteSubaccount_VerifyGone -- get after delete returns 404
// ---------------------------------------------------------------------------

func TestDeleteSubaccount_VerifyGone(t *testing.T) {
	router := setup(t)

	created := createSubaccountHelper(t, router, "Delete Then Get")

	// Delete
	rec := doRequestWithHeaders(t, router, http.MethodDelete, "/v5/accounts/subaccounts", nil, map[string]string{
		"X-Mailgun-On-Behalf-Of": created.Subaccount.ID,
	})
	assertStatus(t, rec, http.StatusOK)

	// Try to get -- should be 404
	rec = doRequest(t, router, http.MethodGet, "/v5/accounts/subaccounts/"+created.Subaccount.ID, nil)
	assertStatus(t, rec, http.StatusNotFound)
	assertMessage(t, rec, "Not Found")
}

// =========================================================================
// Sending Limits Tests
// =========================================================================

// ---------------------------------------------------------------------------
// 24. TestSetSendingLimit_Basic -- set limit, verify success
// ---------------------------------------------------------------------------

func TestSetSendingLimit_Basic(t *testing.T) {
	router := setup(t)

	created := createSubaccountHelper(t, router, "Limit Sub")

	rec := doRequest(t, router, http.MethodPut, fmt.Sprintf("/v5/accounts/subaccounts/%s/limit/custom/monthly?limit=10000", created.Subaccount.ID), nil)
	assertStatus(t, rec, http.StatusOK)

	var resp successResponse
	decodeJSON(t, rec, &resp)

	if !resp.Success {
		t.Error("expected success=true")
	}
}

// ---------------------------------------------------------------------------
// 25. TestGetSendingLimit_Basic -- get limit after setting
// ---------------------------------------------------------------------------

func TestGetSendingLimit_Basic(t *testing.T) {
	router := setup(t)

	created := createSubaccountHelper(t, router, "Limit Get Sub")

	// Set limit first
	rec := doRequest(t, router, http.MethodPut, fmt.Sprintf("/v5/accounts/subaccounts/%s/limit/custom/monthly?limit=5000", created.Subaccount.ID), nil)
	assertStatus(t, rec, http.StatusOK)

	// Get limit
	rec = doRequest(t, router, http.MethodGet, fmt.Sprintf("/v5/accounts/subaccounts/%s/limit/custom/monthly", created.Subaccount.ID), nil)
	assertStatus(t, rec, http.StatusOK)

	var resp sendingLimitResponse
	decodeJSON(t, rec, &resp)

	if resp.Limit != 5000 {
		t.Errorf("expected limit 5000, got %d", resp.Limit)
	}
	if resp.Current != 0 {
		t.Errorf("expected current 0, got %d", resp.Current)
	}
	if resp.Period != "1m" {
		t.Errorf("expected period %q, got %q", "1m", resp.Period)
	}
}

// ---------------------------------------------------------------------------
// 26. TestGetSendingLimit_NotSet -- 404 when no limit set
// ---------------------------------------------------------------------------

func TestGetSendingLimit_NotSet(t *testing.T) {
	router := setup(t)

	created := createSubaccountHelper(t, router, "No Limit Sub")

	rec := doRequest(t, router, http.MethodGet, fmt.Sprintf("/v5/accounts/subaccounts/%s/limit/custom/monthly", created.Subaccount.ID), nil)
	assertStatus(t, rec, http.StatusNotFound)
	assertMessage(t, rec, "No threshold for account")
}

// ---------------------------------------------------------------------------
// 27. TestRemoveSendingLimit_Basic -- remove limit, verify success
// ---------------------------------------------------------------------------

func TestRemoveSendingLimit_Basic(t *testing.T) {
	router := setup(t)

	created := createSubaccountHelper(t, router, "Remove Limit Sub")

	// Set limit first
	rec := doRequest(t, router, http.MethodPut, fmt.Sprintf("/v5/accounts/subaccounts/%s/limit/custom/monthly?limit=10000", created.Subaccount.ID), nil)
	assertStatus(t, rec, http.StatusOK)

	// Remove limit
	rec = doRequest(t, router, http.MethodDelete, fmt.Sprintf("/v5/accounts/subaccounts/%s/limit/custom/monthly", created.Subaccount.ID), nil)
	assertStatus(t, rec, http.StatusOK)

	var resp successResponse
	decodeJSON(t, rec, &resp)

	if !resp.Success {
		t.Error("expected success=true")
	}

	// Verify limit is gone
	rec = doRequest(t, router, http.MethodGet, fmt.Sprintf("/v5/accounts/subaccounts/%s/limit/custom/monthly", created.Subaccount.ID), nil)
	assertStatus(t, rec, http.StatusNotFound)
	assertMessage(t, rec, "No threshold for account")
}

// ---------------------------------------------------------------------------
// 28. TestRemoveSendingLimit_NotSet -- 400 when no limit to remove
// ---------------------------------------------------------------------------

func TestRemoveSendingLimit_NotSet(t *testing.T) {
	router := setup(t)

	created := createSubaccountHelper(t, router, "No Remove Limit Sub")

	rec := doRequest(t, router, http.MethodDelete, fmt.Sprintf("/v5/accounts/subaccounts/%s/limit/custom/monthly", created.Subaccount.ID), nil)
	assertStatus(t, rec, http.StatusBadRequest)
	assertMessage(t, rec, "Could not delete threshold for account")
}

// ---------------------------------------------------------------------------
// 29. TestSendingLimit_SubaccountNotFound -- 400 for non-existent subaccount
// ---------------------------------------------------------------------------

func TestSendingLimit_SubaccountNotFound(t *testing.T) {
	router := setup(t)

	nonExistentID := "aabbccddee112233aabbccdd"

	t.Run("set limit on non-existent", func(t *testing.T) {
		rec := doRequest(t, router, http.MethodPut, fmt.Sprintf("/v5/accounts/subaccounts/%s/limit/custom/monthly?limit=1000", nonExistentID), nil)
		assertStatus(t, rec, http.StatusBadRequest)
	})

	t.Run("get limit on non-existent", func(t *testing.T) {
		rec := doRequest(t, router, http.MethodGet, fmt.Sprintf("/v5/accounts/subaccounts/%s/limit/custom/monthly", nonExistentID), nil)
		// Could be 400 or 404 depending on implementation; spec says 400 for set, 404 for get when no limit
		// Since the subaccount doesn't exist at all, a 404 or 400 is acceptable
		if rec.Code != http.StatusBadRequest && rec.Code != http.StatusNotFound {
			t.Errorf("expected status 400 or 404, got %d; body=%s", rec.Code, rec.Body.String())
		}
	})
}

// =========================================================================
// Feature Updates Tests
// =========================================================================

// ---------------------------------------------------------------------------
// 30. TestUpdateFeatures_Basic -- update single feature, verify merge
// ---------------------------------------------------------------------------

func TestUpdateFeatures_Basic(t *testing.T) {
	router := setup(t)

	created := createSubaccountHelper(t, router, "Features Sub")

	// Enable email_preview (default is false)
	rec := doFormURLEncoded(t, router, http.MethodPut,
		fmt.Sprintf("/v5/accounts/subaccounts/%s/features", created.Subaccount.ID),
		[]fieldPair{
			{key: "email_preview", value: `{"enabled":true}`},
		},
	)
	assertStatus(t, rec, http.StatusOK)

	var resp updateFeaturesResponse
	decodeJSON(t, rec, &resp)

	// email_preview should now be true
	if !resp.Features.EmailPreview.Enabled {
		t.Error("expected email_preview.enabled=true after update")
	}

	// sending should still be true (unchanged)
	if !resp.Features.Sending.Enabled {
		t.Error("expected sending.enabled=true (unchanged)")
	}

	// Others should still be false
	if resp.Features.InboxPlacement.Enabled {
		t.Error("expected inbox_placement.enabled=false (unchanged)")
	}
	if resp.Features.Validations.Enabled {
		t.Error("expected validations.enabled=false (unchanged)")
	}
	if resp.Features.ValidationsBulk.Enabled {
		t.Error("expected validations_bulk.enabled=false (unchanged)")
	}
}

// ---------------------------------------------------------------------------
// 31. TestUpdateFeatures_Multiple -- update multiple features at once
// ---------------------------------------------------------------------------

func TestUpdateFeatures_Multiple(t *testing.T) {
	router := setup(t)

	created := createSubaccountHelper(t, router, "Multi Features Sub")

	rec := doFormURLEncoded(t, router, http.MethodPut,
		fmt.Sprintf("/v5/accounts/subaccounts/%s/features", created.Subaccount.ID),
		[]fieldPair{
			{key: "email_preview", value: `{"enabled":true}`},
			{key: "inbox_placement", value: `{"enabled":true}`},
			{key: "sending", value: `{"enabled":false}`},
		},
	)
	assertStatus(t, rec, http.StatusOK)

	var resp updateFeaturesResponse
	decodeJSON(t, rec, &resp)

	if !resp.Features.EmailPreview.Enabled {
		t.Error("expected email_preview.enabled=true")
	}
	if !resp.Features.InboxPlacement.Enabled {
		t.Error("expected inbox_placement.enabled=true")
	}
	if resp.Features.Sending.Enabled {
		t.Error("expected sending.enabled=false")
	}
	// Unchanged features remain at defaults
	if resp.Features.Validations.Enabled {
		t.Error("expected validations.enabled=false (unchanged)")
	}
	if resp.Features.ValidationsBulk.Enabled {
		t.Error("expected validations_bulk.enabled=false (unchanged)")
	}
}

// ---------------------------------------------------------------------------
// 32. TestUpdateFeatures_NoValidKeys -- 400 when no valid feature keys
// ---------------------------------------------------------------------------

func TestUpdateFeatures_NoValidKeys(t *testing.T) {
	router := setup(t)

	created := createSubaccountHelper(t, router, "Invalid Features Sub")

	// Send with invalid feature key
	rec := doFormURLEncoded(t, router, http.MethodPut,
		fmt.Sprintf("/v5/accounts/subaccounts/%s/features", created.Subaccount.ID),
		[]fieldPair{
			{key: "nonexistent_feature", value: `{"enabled":true}`},
		},
	)
	assertStatus(t, rec, http.StatusBadRequest)
	assertMessage(t, rec, "No valid updates provided")
}

// ---------------------------------------------------------------------------
// 33. TestUpdateFeatures_SubaccountNotFound -- 404 for non-existent subaccount
// ---------------------------------------------------------------------------

func TestUpdateFeatures_SubaccountNotFound(t *testing.T) {
	router := setup(t)

	rec := doFormURLEncoded(t, router, http.MethodPut,
		"/v5/accounts/subaccounts/aabbccddee112233aabbccdd/features",
		[]fieldPair{
			{key: "email_preview", value: `{"enabled":true}`},
		},
	)
	assertStatus(t, rec, http.StatusNotFound)
	assertMessage(t, rec, "Not Found")
}

// =========================================================================
// Integration Tests
// =========================================================================

// ---------------------------------------------------------------------------
// 34. TestSubaccount_FullLifecycle -- create -> get -> list -> disable ->
//     enable -> set limit -> update features -> delete
// ---------------------------------------------------------------------------

func TestSubaccount_FullLifecycle(t *testing.T) {
	router := setup(t)

	var subaccountID string

	t.Run("create subaccount", func(t *testing.T) {
		rec := doRequest(t, router, http.MethodPost, "/v5/accounts/subaccounts", map[string]string{
			"name": "Lifecycle Test",
		})
		assertStatus(t, rec, http.StatusOK)

		var resp createSubaccountResponse
		decodeJSON(t, rec, &resp)

		subaccountID = resp.Subaccount.ID

		if !is24CharHex(subaccountID) {
			t.Fatalf("expected 24-char hex ID, got %q", subaccountID)
		}
		if resp.Subaccount.Name != "Lifecycle Test" {
			t.Errorf("expected name %q, got %q", "Lifecycle Test", resp.Subaccount.Name)
		}
		if resp.Subaccount.Status != "open" {
			t.Errorf("expected status %q, got %q", "open", resp.Subaccount.Status)
		}
	})

	t.Run("get subaccount", func(t *testing.T) {
		rec := doRequest(t, router, http.MethodGet, "/v5/accounts/subaccounts/"+subaccountID, nil)
		assertStatus(t, rec, http.StatusOK)

		var resp getSubaccountResponse
		decodeJSON(t, rec, &resp)

		if resp.Subaccount.ID != subaccountID {
			t.Errorf("expected id %q, got %q", subaccountID, resp.Subaccount.ID)
		}
		if resp.Subaccount.Features == nil {
			t.Fatal("expected features in get response")
		}
		if !resp.Subaccount.Features.Sending.Enabled {
			t.Error("expected sending.enabled=true")
		}
	})

	t.Run("list subaccounts", func(t *testing.T) {
		rec := doRequest(t, router, http.MethodGet, "/v5/accounts/subaccounts", nil)
		assertStatus(t, rec, http.StatusOK)

		var resp listSubaccountsResponse
		decodeJSON(t, rec, &resp)

		if resp.Total < 1 {
			t.Errorf("expected at least 1 subaccount in list, got total=%d", resp.Total)
		}

		found := false
		for _, sa := range resp.Subaccounts {
			if sa.ID == subaccountID {
				found = true
				if sa.Features != nil {
					t.Error("features should not be present in list response")
				}
			}
		}
		if !found {
			t.Errorf("expected subaccount %q in list", subaccountID)
		}
	})

	t.Run("disable subaccount", func(t *testing.T) {
		rec := doRequest(t, router, http.MethodPost, fmt.Sprintf("/v5/accounts/subaccounts/%s/disable", subaccountID), nil)
		assertStatus(t, rec, http.StatusOK)

		var resp getSubaccountResponse
		decodeJSON(t, rec, &resp)

		if resp.Subaccount.Status != "disabled" {
			t.Errorf("expected status %q, got %q", "disabled", resp.Subaccount.Status)
		}
	})

	t.Run("enable subaccount", func(t *testing.T) {
		rec := doRequest(t, router, http.MethodPost, fmt.Sprintf("/v5/accounts/subaccounts/%s/enable", subaccountID), nil)
		assertStatus(t, rec, http.StatusOK)

		var resp getSubaccountResponse
		decodeJSON(t, rec, &resp)

		if resp.Subaccount.Status != "open" {
			t.Errorf("expected status %q, got %q", "open", resp.Subaccount.Status)
		}
	})

	t.Run("set sending limit", func(t *testing.T) {
		rec := doRequest(t, router, http.MethodPut, fmt.Sprintf("/v5/accounts/subaccounts/%s/limit/custom/monthly?limit=25000", subaccountID), nil)
		assertStatus(t, rec, http.StatusOK)

		var resp successResponse
		decodeJSON(t, rec, &resp)

		if !resp.Success {
			t.Error("expected success=true")
		}
	})

	t.Run("get sending limit", func(t *testing.T) {
		rec := doRequest(t, router, http.MethodGet, fmt.Sprintf("/v5/accounts/subaccounts/%s/limit/custom/monthly", subaccountID), nil)
		assertStatus(t, rec, http.StatusOK)

		var resp sendingLimitResponse
		decodeJSON(t, rec, &resp)

		if resp.Limit != 25000 {
			t.Errorf("expected limit 25000, got %d", resp.Limit)
		}
		if resp.Period != "1m" {
			t.Errorf("expected period %q, got %q", "1m", resp.Period)
		}
	})

	t.Run("update features", func(t *testing.T) {
		rec := doFormURLEncoded(t, router, http.MethodPut,
			fmt.Sprintf("/v5/accounts/subaccounts/%s/features", subaccountID),
			[]fieldPair{
				{key: "email_preview", value: `{"enabled":true}`},
				{key: "validations", value: `{"enabled":true}`},
			},
		)
		assertStatus(t, rec, http.StatusOK)

		var resp updateFeaturesResponse
		decodeJSON(t, rec, &resp)

		if !resp.Features.EmailPreview.Enabled {
			t.Error("expected email_preview.enabled=true")
		}
		if !resp.Features.Validations.Enabled {
			t.Error("expected validations.enabled=true")
		}
		if !resp.Features.Sending.Enabled {
			t.Error("expected sending.enabled=true (unchanged)")
		}
	})

	t.Run("verify features persisted", func(t *testing.T) {
		rec := doRequest(t, router, http.MethodGet, "/v5/accounts/subaccounts/"+subaccountID, nil)
		assertStatus(t, rec, http.StatusOK)

		var resp getSubaccountResponse
		decodeJSON(t, rec, &resp)

		if resp.Subaccount.Features == nil {
			t.Fatal("expected features in get response")
		}
		if !resp.Subaccount.Features.EmailPreview.Enabled {
			t.Error("expected email_preview.enabled=true after update")
		}
		if !resp.Subaccount.Features.Validations.Enabled {
			t.Error("expected validations.enabled=true after update")
		}
	})

	t.Run("delete subaccount", func(t *testing.T) {
		rec := doRequestWithHeaders(t, router, http.MethodDelete, "/v5/accounts/subaccounts", nil, map[string]string{
			"X-Mailgun-On-Behalf-Of": subaccountID,
		})
		assertStatus(t, rec, http.StatusOK)
		assertMessage(t, rec, "Subaccount successfully deleted")
	})

	t.Run("verify subaccount is gone", func(t *testing.T) {
		rec := doRequest(t, router, http.MethodGet, "/v5/accounts/subaccounts/"+subaccountID, nil)
		assertStatus(t, rec, http.StatusNotFound)
		assertMessage(t, rec, "Not Found")
	})
}
