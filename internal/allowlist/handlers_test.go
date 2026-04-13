package allowlist_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bethmaloney/mailgun-mock-api/internal/allowlist"
	"github.com/go-chi/chi/v5"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// Test Helpers
// ---------------------------------------------------------------------------

// setupTestDB creates an in-memory SQLite database for testing with the
// IPAllowlistEntry table migrated.
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}
	if err := db.AutoMigrate(&allowlist.IPAllowlistEntry{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

// setupRouter creates a chi router with IP allowlist routes registered.
func setupRouter(db *gorm.DB) http.Handler {
	allowlist.ResetForTests(db)
	h := allowlist.NewHandlers(db)
	r := chi.NewRouter()
	r.Route("/v2/ip_whitelist", func(r chi.Router) {
		r.Get("/", h.ListEntries)
		r.Post("/", h.AddEntry)
		r.Put("/", h.UpdateEntry)
		r.Delete("/", h.DeleteEntry)
	})
	return r
}

// newMultipartRequest creates an HTTP request with multipart/form-data body.
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

// decodeJSON unmarshals the response body into the provided destination.
func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder, dest interface{}) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dest); err != nil {
		t.Fatalf("failed to decode response (body=%q): %v", rec.Body.String(), err)
	}
}

// addEntry is a convenience helper that adds an IP allowlist entry and returns
// the recorder for inspection.
func addEntry(t *testing.T, router http.Handler, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := newMultipartRequest(t, http.MethodPost, "/v2/ip_whitelist", fields)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// listEntries is a convenience helper that lists IP allowlist entries.
func listEntries(t *testing.T, router http.Handler) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v2/ip_whitelist", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// updateEntry is a convenience helper that updates an IP allowlist entry's description.
func updateEntry(t *testing.T, router http.Handler, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := newMultipartRequest(t, http.MethodPut, "/v2/ip_whitelist", fields)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// deleteEntry is a convenience helper that deletes an IP allowlist entry by address.
func deleteEntry(t *testing.T, router http.Handler, address string) *httptest.ResponseRecorder {
	t.Helper()
	url := "/v2/ip_whitelist?address=" + address
	req := httptest.NewRequest(http.MethodDelete, url, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// ---------------------------------------------------------------------------
// Response Structs for Assertions
// ---------------------------------------------------------------------------

type allowlistEntry struct {
	IPAddress   string `json:"ip_address"`
	Description string `json:"description"`
}

type allowlistResponse struct {
	Addresses []allowlistEntry `json:"addresses"`
}

type errorResponse struct {
	Message string `json:"message"`
}

// ---------------------------------------------------------------------------
// Helper to find entry by IP in response
// ---------------------------------------------------------------------------

func findEntry(entries []allowlistEntry, ip string) (allowlistEntry, bool) {
	for _, e := range entries {
		if e.IPAddress == ip {
			return e, true
		}
	}
	return allowlistEntry{}, false
}

// ---------------------------------------------------------------------------
// POST /v2/ip_whitelist -- Add Allowlist Entry
// ---------------------------------------------------------------------------

func TestAddEntry_HappyPath(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	rec := addEntry(t, router, map[string]string{
		"address": "10.11.11.111",
	})

	t.Run("returns 200", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp allowlistResponse
	decodeJSON(t, rec, &resp)

	t.Run("response contains the added entry", func(t *testing.T) {
		if len(resp.Addresses) != 1 {
			t.Fatalf("expected 1 address, got %d", len(resp.Addresses))
		}
		if resp.Addresses[0].IPAddress != "10.11.11.111" {
			t.Errorf("expected ip_address %q, got %q", "10.11.11.111", resp.Addresses[0].IPAddress)
		}
	})
}

func TestAddEntry_WithCIDR(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	rec := addEntry(t, router, map[string]string{
		"address": "192.168.1.0/24",
	})

	t.Run("returns 200", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp allowlistResponse
	decodeJSON(t, rec, &resp)

	t.Run("response contains the CIDR entry", func(t *testing.T) {
		found := false
		for _, addr := range resp.Addresses {
			if addr.IPAddress == "192.168.1.0/24" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected to find 192.168.1.0/24 in addresses, got %+v", resp.Addresses)
		}
	})
}

func TestAddEntry_WithDescription(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	rec := addEntry(t, router, map[string]string{
		"address":     "10.0.0.1",
		"description": "OnPrem Server",
	})

	t.Run("returns 200", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp allowlistResponse
	decodeJSON(t, rec, &resp)

	t.Run("description is stored correctly", func(t *testing.T) {
		entry, found := findEntry(resp.Addresses, "10.0.0.1")
		if !found {
			t.Fatalf("expected to find 10.0.0.1 in addresses, got %+v", resp.Addresses)
		}
		if entry.Description != "OnPrem Server" {
			t.Errorf("expected description %q, got %q", "OnPrem Server", entry.Description)
		}
	})
}

func TestAddEntry_WithoutDescription(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	rec := addEntry(t, router, map[string]string{
		"address": "10.0.0.2",
	})

	t.Run("returns 200", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp allowlistResponse
	decodeJSON(t, rec, &resp)

	t.Run("description defaults to empty string", func(t *testing.T) {
		entry, found := findEntry(resp.Addresses, "10.0.0.2")
		if !found {
			t.Fatalf("expected to find 10.0.0.2 in addresses, got %+v", resp.Addresses)
		}
		if entry.Description != "" {
			t.Errorf("expected empty description, got %q", entry.Description)
		}
	})
}

func TestAddEntry_InvalidIP(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	rec := addEntry(t, router, map[string]string{
		"address": "not-an-ip",
	})

	t.Run("returns 400", func(t *testing.T) {
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("body contains error message", func(t *testing.T) {
		var resp errorResponse
		decodeJSON(t, rec, &resp)
		if resp.Message != "Invalid IP Address or CIDR" {
			t.Errorf("expected %q, got %q", "Invalid IP Address or CIDR", resp.Message)
		}
	})
}

func TestAddEntry_MissingAddress(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	rec := addEntry(t, router, map[string]string{})

	t.Run("returns 400", func(t *testing.T) {
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})
}

func TestAddEntry_EmptyAddress(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	rec := addEntry(t, router, map[string]string{
		"address": "",
	})

	t.Run("returns 400", func(t *testing.T) {
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})
}

func TestAddEntry_DuplicateIP(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	// Add the IP once
	rec1 := addEntry(t, router, map[string]string{
		"address":     "10.20.30.40",
		"description": "First add",
	})
	if rec1.Code != http.StatusOK {
		t.Fatalf("first add failed: status=%d body=%s", rec1.Code, rec1.Body.String())
	}

	// Add the same IP again
	rec2 := addEntry(t, router, map[string]string{
		"address":     "10.20.30.40",
		"description": "Second add",
	})

	t.Run("duplicate returns error or is idempotent", func(t *testing.T) {
		// Either the server returns an error (4xx) or it is idempotent (200).
		// In either case, the allowlist should not have duplicate entries.
		if rec2.Code == http.StatusOK {
			var resp allowlistResponse
			decodeJSON(t, rec2, &resp)
			count := 0
			for _, addr := range resp.Addresses {
				if addr.IPAddress == "10.20.30.40" {
					count++
				}
			}
			if count > 1 {
				t.Errorf("expected at most 1 entry for 10.20.30.40, found %d", count)
			}
		} else if rec2.Code < 400 || rec2.Code >= 500 {
			t.Errorf("expected 4xx error or 200 for duplicate IP, got %d (body: %s)", rec2.Code, rec2.Body.String())
		}
	})
}

// ---------------------------------------------------------------------------
// GET /v2/ip_whitelist -- List Allowlist Entries
// ---------------------------------------------------------------------------

func TestListEntries_EmptyList(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	rec := listEntries(t, router)

	t.Run("returns 200", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp allowlistResponse
	decodeJSON(t, rec, &resp)

	t.Run("addresses is empty array", func(t *testing.T) {
		if len(resp.Addresses) != 0 {
			t.Errorf("expected 0 addresses, got %d", len(resp.Addresses))
		}
	})

	t.Run("addresses is empty array not null", func(t *testing.T) {
		raw := rec.Body.String()
		if strings.Contains(raw, `"addresses":null`) || strings.Contains(raw, `"addresses": null`) {
			t.Errorf("expected addresses to be empty array, not null (body: %s)", raw)
		}
	})
}

func TestListEntries_WithEntries(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	// Add several entries
	ips := []struct {
		address     string
		description string
	}{
		{"10.0.0.1", "Server A"},
		{"10.0.0.2", "Server B"},
		{"192.168.1.0/24", "Internal network"},
	}
	for _, ip := range ips {
		rec := addEntry(t, router, map[string]string{
			"address":     ip.address,
			"description": ip.description,
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("failed to add entry %q: status=%d body=%s", ip.address, rec.Code, rec.Body.String())
		}
	}

	rec := listEntries(t, router)

	t.Run("returns 200", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp allowlistResponse
	decodeJSON(t, rec, &resp)

	t.Run("returns all entries", func(t *testing.T) {
		if len(resp.Addresses) != 3 {
			t.Errorf("expected 3 addresses, got %d", len(resp.Addresses))
		}
	})

	t.Run("entries have correct data", func(t *testing.T) {
		for _, ip := range ips {
			entry, found := findEntry(resp.Addresses, ip.address)
			if !found {
				t.Errorf("expected to find %q in addresses", ip.address)
				continue
			}
			if entry.Description != ip.description {
				t.Errorf("for %q: expected description %q, got %q", ip.address, ip.description, entry.Description)
			}
		}
	})
}

func TestListEntries_ResponseShape(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	rec := addEntry(t, router, map[string]string{
		"address":     "10.11.11.111",
		"description": "OnPrem Server",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to add entry: status=%d body=%s", rec.Code, rec.Body.String())
	}

	listRec := listEntries(t, router)

	var resp allowlistResponse
	decodeJSON(t, listRec, &resp)

	t.Run("each entry has ip_address field", func(t *testing.T) {
		for i, entry := range resp.Addresses {
			if entry.IPAddress == "" {
				t.Errorf("entry[%d] has empty ip_address", i)
			}
		}
	})

	t.Run("response contains addresses key", func(t *testing.T) {
		raw := listRec.Body.String()
		if !strings.Contains(raw, `"addresses"`) {
			t.Errorf("expected response to contain 'addresses' key, got: %s", raw)
		}
	})

	t.Run("each entry has description field", func(t *testing.T) {
		// Verify the raw JSON contains the description field (even if empty)
		raw := listRec.Body.String()
		if !strings.Contains(raw, `"description"`) {
			t.Errorf("expected response to contain 'description' key, got: %s", raw)
		}
	})
}

// ---------------------------------------------------------------------------
// PUT /v2/ip_whitelist -- Update Allowlist Entry Description
// ---------------------------------------------------------------------------

func TestUpdateEntry_HappyPath(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	// Add an entry first
	addRec := addEntry(t, router, map[string]string{
		"address":     "10.0.0.1",
		"description": "Original description",
	})
	if addRec.Code != http.StatusOK {
		t.Fatalf("failed to add entry: status=%d body=%s", addRec.Code, addRec.Body.String())
	}

	// Update the description
	rec := updateEntry(t, router, map[string]string{
		"address":     "10.0.0.1",
		"description": "Updated description",
	})

	t.Run("returns 200", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp allowlistResponse
	decodeJSON(t, rec, &resp)

	t.Run("response reflects updated description", func(t *testing.T) {
		entry, found := findEntry(resp.Addresses, "10.0.0.1")
		if !found {
			t.Fatalf("expected to find 10.0.0.1 in response, got %+v", resp.Addresses)
		}
		if entry.Description != "Updated description" {
			t.Errorf("expected description %q, got %q", "Updated description", entry.Description)
		}
	})

	// Verify the update persists via a subsequent GET
	t.Run("update persists in list", func(t *testing.T) {
		listRec := listEntries(t, router)
		var listResp allowlistResponse
		decodeJSON(t, listRec, &listResp)
		entry, found := findEntry(listResp.Addresses, "10.0.0.1")
		if !found {
			t.Fatalf("expected to find 10.0.0.1 in list, got %+v", listResp.Addresses)
		}
		if entry.Description != "Updated description" {
			t.Errorf("expected description %q after update, got %q", "Updated description", entry.Description)
		}
	})
}

func TestUpdateEntry_IPNotFound(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	rec := updateEntry(t, router, map[string]string{
		"address":     "10.99.99.99",
		"description": "Does not exist",
	})

	t.Run("returns 400", func(t *testing.T) {
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("body contains error message", func(t *testing.T) {
		var resp errorResponse
		decodeJSON(t, rec, &resp)
		if resp.Message != "IP not found" {
			t.Errorf("expected %q, got %q", "IP not found", resp.Message)
		}
	})
}

func TestUpdateEntry_ToEmptyDescription(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	// Add an entry with a description
	addRec := addEntry(t, router, map[string]string{
		"address":     "10.0.0.5",
		"description": "Will be cleared",
	})
	if addRec.Code != http.StatusOK {
		t.Fatalf("failed to add entry: status=%d body=%s", addRec.Code, addRec.Body.String())
	}

	// Update to empty description
	rec := updateEntry(t, router, map[string]string{
		"address":     "10.0.0.5",
		"description": "",
	})

	t.Run("returns 200", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp allowlistResponse
	decodeJSON(t, rec, &resp)

	t.Run("description is set to empty string", func(t *testing.T) {
		entry, found := findEntry(resp.Addresses, "10.0.0.5")
		if !found {
			t.Fatalf("expected to find 10.0.0.5 in response, got %+v", resp.Addresses)
		}
		if entry.Description != "" {
			t.Errorf("expected empty description, got %q", entry.Description)
		}
	})
}

// ---------------------------------------------------------------------------
// DELETE /v2/ip_whitelist -- Delete Allowlist Entry
// ---------------------------------------------------------------------------

func TestDeleteEntry_HappyPath(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	// Add an entry first
	addRec := addEntry(t, router, map[string]string{
		"address":     "10.0.0.1",
		"description": "To be deleted",
	})
	if addRec.Code != http.StatusOK {
		t.Fatalf("failed to add entry: status=%d body=%s", addRec.Code, addRec.Body.String())
	}

	// Delete it
	rec := deleteEntry(t, router, "10.0.0.1")

	t.Run("returns 200", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp allowlistResponse
	decodeJSON(t, rec, &resp)

	t.Run("response does not include deleted entry", func(t *testing.T) {
		_, found := findEntry(resp.Addresses, "10.0.0.1")
		if found {
			t.Errorf("expected 10.0.0.1 to be removed from response, but it was found")
		}
	})

	// Verify via GET
	t.Run("entry is no longer listed", func(t *testing.T) {
		listRec := listEntries(t, router)
		var listResp allowlistResponse
		decodeJSON(t, listRec, &listResp)
		_, found := findEntry(listResp.Addresses, "10.0.0.1")
		if found {
			t.Errorf("expected 10.0.0.1 to be gone from list, but it was found")
		}
	})
}

func TestDeleteEntry_IPNotFound(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	rec := deleteEntry(t, router, "10.99.99.99")

	t.Run("returns 400", func(t *testing.T) {
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})
}

func TestDeleteEntry_LeavesOtherEntries(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	// Add two entries
	addRec1 := addEntry(t, router, map[string]string{
		"address":     "10.0.0.1",
		"description": "Server A",
	})
	if addRec1.Code != http.StatusOK {
		t.Fatalf("failed to add first entry: status=%d body=%s", addRec1.Code, addRec1.Body.String())
	}

	addRec2 := addEntry(t, router, map[string]string{
		"address":     "10.0.0.2",
		"description": "Server B",
	})
	if addRec2.Code != http.StatusOK {
		t.Fatalf("failed to add second entry: status=%d body=%s", addRec2.Code, addRec2.Body.String())
	}

	// Delete only the first
	rec := deleteEntry(t, router, "10.0.0.1")

	t.Run("returns 200", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp allowlistResponse
	decodeJSON(t, rec, &resp)

	t.Run("deleted entry is gone", func(t *testing.T) {
		_, found := findEntry(resp.Addresses, "10.0.0.1")
		if found {
			t.Errorf("expected 10.0.0.1 to be removed, but it was found")
		}
	})

	t.Run("other entry remains", func(t *testing.T) {
		entry, found := findEntry(resp.Addresses, "10.0.0.2")
		if !found {
			t.Errorf("expected 10.0.0.2 to remain, but it was not found")
		}
		if found && entry.Description != "Server B" {
			t.Errorf("expected description %q, got %q", "Server B", entry.Description)
		}
	})

	t.Run("list confirms one entry remains", func(t *testing.T) {
		listRec := listEntries(t, router)
		var listResp allowlistResponse
		decodeJSON(t, listRec, &listResp)
		if len(listResp.Addresses) != 1 {
			t.Errorf("expected 1 address remaining, got %d", len(listResp.Addresses))
		}
	})
}

// ---------------------------------------------------------------------------
// Full Lifecycle Test
// ---------------------------------------------------------------------------

func TestAllowlistLifecycle(t *testing.T) {
	db := setupTestDB(t)
	router := setupRouter(db)

	// Step 1: Create an entry
	t.Run("add entry", func(t *testing.T) {
		rec := addEntry(t, router, map[string]string{
			"address":     "10.11.11.111",
			"description": "OnPrem Server",
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("add failed: status=%d body=%s", rec.Code, rec.Body.String())
		}
		var resp allowlistResponse
		decodeJSON(t, rec, &resp)
		if len(resp.Addresses) != 1 {
			t.Fatalf("expected 1 address, got %d", len(resp.Addresses))
		}
		if resp.Addresses[0].IPAddress != "10.11.11.111" {
			t.Errorf("expected ip_address %q, got %q", "10.11.11.111", resp.Addresses[0].IPAddress)
		}
		if resp.Addresses[0].Description != "OnPrem Server" {
			t.Errorf("expected description %q, got %q", "OnPrem Server", resp.Addresses[0].Description)
		}
	})

	// Step 2: List and verify the entry
	t.Run("list shows added entry", func(t *testing.T) {
		rec := listEntries(t, router)
		if rec.Code != http.StatusOK {
			t.Fatalf("list failed: status=%d body=%s", rec.Code, rec.Body.String())
		}
		var resp allowlistResponse
		decodeJSON(t, rec, &resp)
		if len(resp.Addresses) != 1 {
			t.Fatalf("expected 1 address, got %d", len(resp.Addresses))
		}
		entry, found := findEntry(resp.Addresses, "10.11.11.111")
		if !found {
			t.Fatal("expected to find 10.11.11.111 in list")
		}
		if entry.Description != "OnPrem Server" {
			t.Errorf("expected description %q, got %q", "OnPrem Server", entry.Description)
		}
	})

	// Step 3: Update the description
	t.Run("update description", func(t *testing.T) {
		rec := updateEntry(t, router, map[string]string{
			"address":     "10.11.11.111",
			"description": "Updated OnPrem Server",
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("update failed: status=%d body=%s", rec.Code, rec.Body.String())
		}
		var resp allowlistResponse
		decodeJSON(t, rec, &resp)
		entry, found := findEntry(resp.Addresses, "10.11.11.111")
		if !found {
			t.Fatal("expected to find 10.11.11.111 after update")
		}
		if entry.Description != "Updated OnPrem Server" {
			t.Errorf("expected description %q, got %q", "Updated OnPrem Server", entry.Description)
		}
	})

	// Step 4: List again and verify the update persisted
	t.Run("list shows updated description", func(t *testing.T) {
		rec := listEntries(t, router)
		if rec.Code != http.StatusOK {
			t.Fatalf("list failed: status=%d body=%s", rec.Code, rec.Body.String())
		}
		var resp allowlistResponse
		decodeJSON(t, rec, &resp)
		entry, found := findEntry(resp.Addresses, "10.11.11.111")
		if !found {
			t.Fatal("expected to find 10.11.11.111 in list")
		}
		if entry.Description != "Updated OnPrem Server" {
			t.Errorf("expected description %q, got %q", "Updated OnPrem Server", entry.Description)
		}
	})

	// Step 5: Delete the entry
	t.Run("delete entry", func(t *testing.T) {
		rec := deleteEntry(t, router, "10.11.11.111")
		if rec.Code != http.StatusOK {
			t.Fatalf("delete failed: status=%d body=%s", rec.Code, rec.Body.String())
		}
		var resp allowlistResponse
		decodeJSON(t, rec, &resp)
		if len(resp.Addresses) != 0 {
			t.Errorf("expected 0 addresses after deletion, got %d", len(resp.Addresses))
		}
	})

	// Step 6: List is empty
	t.Run("list is empty after deletion", func(t *testing.T) {
		rec := listEntries(t, router)
		if rec.Code != http.StatusOK {
			t.Fatalf("list failed: status=%d body=%s", rec.Code, rec.Body.String())
		}
		var resp allowlistResponse
		decodeJSON(t, rec, &resp)
		if len(resp.Addresses) != 0 {
			t.Errorf("expected 0 addresses, got %d", len(resp.Addresses))
		}
	})
}
