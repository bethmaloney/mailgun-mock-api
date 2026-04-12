package domain_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bethmaloney/mailgun-mock-api/internal/domain"
	appMiddleware "github.com/bethmaloney/mailgun-mock-api/internal/middleware"
	"github.com/bethmaloney/mailgun-mock-api/internal/mock"
	"github.com/bethmaloney/mailgun-mock-api/internal/subaccount"
	"github.com/go-chi/chi/v5"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// Helpers for subaccount scoping tests
// ---------------------------------------------------------------------------

// setupScopingDB creates an in-memory SQLite database with the Domain,
// DNSRecord, and Subaccount tables migrated.
func setupScopingDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}
	if err := db.AutoMigrate(&domain.Domain{}, &domain.DNSRecord{}, &subaccount.Subaccount{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

// scopingConfig returns a MockConfig with auto-verify enabled and accept_any
// auth mode (so we don't need to set Basic Auth on every request).
func scopingConfig() *mock.MockConfig {
	return &mock.MockConfig{
		DomainBehavior: mock.DomainBehaviorConfig{
			DomainAutoVerify: true,
			SandboxDomain:    "sandbox123.mailgun.org",
		},
		Authentication: mock.AuthenticationConfig{
			AuthMode: "accept_any",
		},
	}
}

// setupScopingRouter creates a chi router with SubaccountScoping middleware
// and domain routes. This mirrors how the production server wires things up
// but includes the SubaccountScoping middleware before domain handlers.
func setupScopingRouter(db *gorm.DB, cfg *mock.MockConfig) http.Handler {
	h := domain.NewHandlers(db, cfg)
	r := chi.NewRouter()
	r.Route("/v4/domains", func(r chi.Router) {
		r.Use(appMiddleware.SubaccountScoping(db))
		r.Post("/", h.CreateDomain)
		r.Get("/", h.ListDomains)
		r.Get("/{name}", h.GetDomain)
	})
	return r
}

// createScopingSubaccount inserts a subaccount record directly into the DB.
func createScopingSubaccount(t *testing.T, db *gorm.DB, subaccountID, name, status string) {
	t.Helper()
	sa := subaccount.Subaccount{
		SubaccountID: subaccountID,
		Name:         name,
		Status:       status,
	}
	if err := db.Create(&sa).Error; err != nil {
		t.Fatalf("failed to create subaccount %q: %v", subaccountID, err)
	}
}

// createScopedDomain creates a domain via the API with an optional
// X-Mailgun-On-Behalf-Of header. Returns the response recorder.
func createScopedDomain(t *testing.T, router http.Handler, domainName string, onBehalfOf string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	writer.WriteField("name", domainName)
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/v4/domains", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if onBehalfOf != "" {
		req.Header.Set("X-Mailgun-On-Behalf-Of", onBehalfOf)
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// listScopedDomains makes a GET /v4/domains request with optional
// X-Mailgun-On-Behalf-Of header and query parameters.
func listScopedDomains(t *testing.T, router http.Handler, onBehalfOf string, queryParams string) *httptest.ResponseRecorder {
	t.Helper()
	url := "/v4/domains"
	if queryParams != "" {
		url += "?" + queryParams
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	if onBehalfOf != "" {
		req.Header.Set("X-Mailgun-On-Behalf-Of", onBehalfOf)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// scopedListResponse represents the JSON response from the list domains endpoint.
type scopedListResponse struct {
	TotalCount int              `json:"total_count"`
	Items      []scopedDomainJSON `json:"items"`
}

// scopedDomainJSON represents a domain object in the JSON response, including
// optional subaccount_id for scoping verification.
type scopedDomainJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// decodeScopedList decodes the response body into a scopedListResponse.
func decodeScopedList(t *testing.T, rec *httptest.ResponseRecorder) scopedListResponse {
	t.Helper()
	var resp scopedListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode list response (body=%q): %v", rec.Body.String(), err)
	}
	return resp
}

// getDomainSubaccountID queries the database directly to get the subaccount_id
// of a domain by name. Returns nil if the domain has no subaccount_id.
func getDomainSubaccountID(t *testing.T, db *gorm.DB, domainName string) *string {
	t.Helper()
	var d domain.Domain
	if err := db.Where("name = ?", domainName).First(&d).Error; err != nil {
		t.Fatalf("failed to find domain %q in DB: %v", domainName, err)
	}
	return d.SubaccountID
}

// domainNames extracts the domain names from a list response.
func domainNames(items []scopedDomainJSON) []string {
	names := make([]string, len(items))
	for i, item := range items {
		names[i] = item.Name
	}
	return names
}

// containsName checks if a name is in a string slice.
func containsName(names []string, target string) bool {
	for _, n := range names {
		if n == target {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Domain Creation with Subaccount Context
// ---------------------------------------------------------------------------

func TestCreateDomain_WithSubaccountHeader_SetsSubaccountID(t *testing.T) {
	db := setupScopingDB(t)
	cfg := scopingConfig()

	createScopingSubaccount(t, db, "sa_create_test_001234", "Creator Subaccount", "open")
	router := setupScopingRouter(db, cfg)

	rec := createScopedDomain(t, router, "scoped-create.example.com", "sa_create_test_001234")

	t.Run("returns 200 status", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("domain in DB has subaccount_id set", func(t *testing.T) {
		saID := getDomainSubaccountID(t, db, "scoped-create.example.com")
		if saID == nil {
			t.Fatal("expected subaccount_id to be set, got nil")
		}
		if *saID != "sa_create_test_001234" {
			t.Errorf("expected subaccount_id %q, got %q", "sa_create_test_001234", *saID)
		}
	})
}

func TestCreateDomain_WithoutSubaccountHeader_NoSubaccountID(t *testing.T) {
	db := setupScopingDB(t)
	cfg := scopingConfig()
	router := setupScopingRouter(db, cfg)

	rec := createScopedDomain(t, router, "unscoped-create.example.com", "")

	t.Run("returns 200 status", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("domain in DB has no subaccount_id", func(t *testing.T) {
		saID := getDomainSubaccountID(t, db, "unscoped-create.example.com")
		if saID != nil {
			t.Errorf("expected subaccount_id to be nil, got %q", *saID)
		}
	})
}

// ---------------------------------------------------------------------------
// Domain Listing with Subaccount Filtering
// ---------------------------------------------------------------------------

func TestListDomains_WithSubaccountHeader_FiltersToSubaccount(t *testing.T) {
	db := setupScopingDB(t)
	cfg := scopingConfig()

	// Create two subaccounts.
	createScopingSubaccount(t, db, "sa_list_aaa_012345678", "Subaccount A", "open")
	createScopingSubaccount(t, db, "sa_list_bbb_012345678", "Subaccount B", "open")

	router := setupScopingRouter(db, cfg)

	// Create domains for subaccount A.
	rec := createScopedDomain(t, router, "domain-a1.example.com", "sa_list_aaa_012345678")
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to create domain-a1: status %d, body: %s", rec.Code, rec.Body.String())
	}
	rec = createScopedDomain(t, router, "domain-a2.example.com", "sa_list_aaa_012345678")
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to create domain-a2: status %d, body: %s", rec.Code, rec.Body.String())
	}

	// Create domains for subaccount B.
	rec = createScopedDomain(t, router, "domain-b1.example.com", "sa_list_bbb_012345678")
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to create domain-b1: status %d, body: %s", rec.Code, rec.Body.String())
	}

	// Create domains with no subaccount.
	rec = createScopedDomain(t, router, "domain-none.example.com", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to create domain-none: status %d, body: %s", rec.Code, rec.Body.String())
	}

	t.Run("listing with subaccount A header returns only A domains", func(t *testing.T) {
		listRec := listScopedDomains(t, router, "sa_list_aaa_012345678", "")
		if listRec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d (body: %s)", listRec.Code, listRec.Body.String())
		}

		resp := decodeScopedList(t, listRec)
		names := domainNames(resp.Items)

		if resp.TotalCount != 2 {
			t.Errorf("expected total_count=2 for subaccount A, got %d", resp.TotalCount)
		}
		if len(resp.Items) != 2 {
			t.Errorf("expected 2 items for subaccount A, got %d", len(resp.Items))
		}
		if !containsName(names, "domain-a1.example.com") {
			t.Error("expected domain-a1.example.com in results")
		}
		if !containsName(names, "domain-a2.example.com") {
			t.Error("expected domain-a2.example.com in results")
		}
		if containsName(names, "domain-b1.example.com") {
			t.Error("domain-b1.example.com should not appear in subaccount A results")
		}
		if containsName(names, "domain-none.example.com") {
			t.Error("domain-none.example.com should not appear in subaccount A results")
		}
	})

	t.Run("listing with subaccount B header returns only B domains", func(t *testing.T) {
		listRec := listScopedDomains(t, router, "sa_list_bbb_012345678", "")
		if listRec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d (body: %s)", listRec.Code, listRec.Body.String())
		}

		resp := decodeScopedList(t, listRec)
		names := domainNames(resp.Items)

		if resp.TotalCount != 1 {
			t.Errorf("expected total_count=1 for subaccount B, got %d", resp.TotalCount)
		}
		if len(resp.Items) != 1 {
			t.Errorf("expected 1 item for subaccount B, got %d", len(resp.Items))
		}
		if !containsName(names, "domain-b1.example.com") {
			t.Error("expected domain-b1.example.com in results")
		}
	})

	t.Run("listing without header returns only unscoped domains", func(t *testing.T) {
		listRec := listScopedDomains(t, router, "", "")
		if listRec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d (body: %s)", listRec.Code, listRec.Body.String())
		}

		resp := decodeScopedList(t, listRec)
		names := domainNames(resp.Items)

		if resp.TotalCount != 1 {
			t.Errorf("expected total_count=1 for unscoped listing, got %d", resp.TotalCount)
		}
		if len(resp.Items) != 1 {
			t.Errorf("expected 1 item for unscoped listing, got %d", len(resp.Items))
		}
		if !containsName(names, "domain-none.example.com") {
			t.Error("expected domain-none.example.com in unscoped results")
		}
		if containsName(names, "domain-a1.example.com") {
			t.Error("domain-a1.example.com should not appear in unscoped results")
		}
	})

	t.Run("listing with include_subaccounts=true returns all domains", func(t *testing.T) {
		listRec := listScopedDomains(t, router, "", "include_subaccounts=true")
		if listRec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d (body: %s)", listRec.Code, listRec.Body.String())
		}

		resp := decodeScopedList(t, listRec)
		names := domainNames(resp.Items)

		if resp.TotalCount != 4 {
			t.Errorf("expected total_count=4 with include_subaccounts=true, got %d", resp.TotalCount)
		}
		if len(resp.Items) != 4 {
			t.Errorf("expected 4 items with include_subaccounts=true, got %d", len(resp.Items))
		}
		if !containsName(names, "domain-a1.example.com") {
			t.Error("expected domain-a1.example.com in include_subaccounts results")
		}
		if !containsName(names, "domain-a2.example.com") {
			t.Error("expected domain-a2.example.com in include_subaccounts results")
		}
		if !containsName(names, "domain-b1.example.com") {
			t.Error("expected domain-b1.example.com in include_subaccounts results")
		}
		if !containsName(names, "domain-none.example.com") {
			t.Error("expected domain-none.example.com in include_subaccounts results")
		}
	})
}
