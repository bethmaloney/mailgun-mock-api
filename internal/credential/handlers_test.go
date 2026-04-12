package credential_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bethmaloney/mailgun-mock-api/internal/credential"
	"github.com/bethmaloney/mailgun-mock-api/internal/domain"
	"github.com/bethmaloney/mailgun-mock-api/internal/mock"
	"github.com/go-chi/chi/v5"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// Test Helpers
// ---------------------------------------------------------------------------

// setupTestDB creates an in-memory SQLite database for testing with the
// Domain, DNSRecord, and SMTPCredential tables migrated.
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}
	if err := db.AutoMigrate(&domain.Domain{}, &domain.DNSRecord{}, &credential.SMTPCredential{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

// defaultConfig returns a MockConfig with auto-verify enabled (the default).
func defaultConfig() *mock.MockConfig {
	return &mock.MockConfig{
		DomainBehavior: mock.DomainBehaviorConfig{
			DomainAutoVerify: true,
			SandboxDomain:    "sandbox123.mailgun.org",
		},
	}
}

// setupRouter creates a chi router with domain and credential routes registered.
func setupRouter(db *gorm.DB, cfg *mock.MockConfig) http.Handler {
	dh := domain.NewHandlers(db, cfg)
	ch := credential.NewHandlers(db)
	r := chi.NewRouter()
	// Domain routes needed to create domains first
	r.Route("/v4/domains", func(r chi.Router) {
		r.Post("/", dh.CreateDomain)
	})
	r.Route("/v3/domains/{domain_name}", func(r chi.Router) {
		r.Route("/credentials", func(r chi.Router) {
			r.Get("/", ch.ListCredentials)
			r.Post("/", ch.CreateCredential)
			r.Delete("/", ch.DeleteAllCredentials)
			r.Put("/{spec}", ch.UpdateCredential)
			r.Delete("/{spec}", ch.DeleteCredential)
		})
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

// newJSONRequest creates an HTTP request with a JSON body.
func newJSONRequest(t *testing.T, method, url string, body interface{}) *http.Request {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("failed to marshal JSON body: %v", err)
	}
	req := httptest.NewRequest(method, url, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// decodeJSON unmarshals the response body into the provided destination.
func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder, dest interface{}) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dest); err != nil {
		t.Fatalf("failed to decode response (body=%q): %v", rec.Body.String(), err)
	}
}

// createTestDomain creates a domain via the API, which is required before
// creating credentials (since credentials are domain-scoped).
func createTestDomain(t *testing.T, router http.Handler, name string) {
	t.Helper()
	req := newMultipartRequest(t, http.MethodPost, "/v4/domains", map[string]string{"name": name})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to create test domain %q: status=%d body=%s", name, rec.Code, rec.Body.String())
	}
}

// createCredential is a convenience helper that creates a credential and
// returns the recorder for inspection.
func createCredential(t *testing.T, router http.Handler, domainName string, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	url := fmt.Sprintf("/v3/domains/%s/credentials", domainName)
	req := newMultipartRequest(t, http.MethodPost, url, fields)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// listCredentials is a convenience helper that lists credentials for a domain.
func listCredentials(t *testing.T, router http.Handler, domainName string, queryParams string) *httptest.ResponseRecorder {
	t.Helper()
	url := fmt.Sprintf("/v3/domains/%s/credentials", domainName)
	if queryParams != "" {
		url += "?" + queryParams
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// ---------------------------------------------------------------------------
// Response Structs for Assertions
// ---------------------------------------------------------------------------

type credentialJSON struct {
	CreatedAt string      `json:"created_at"`
	Login     string      `json:"login"`
	Mailbox   string      `json:"mailbox"`
	SizeBytes interface{} `json:"size_bytes"`
}

type listCredentialsResponse struct {
	TotalCount int              `json:"total_count"`
	Items      []credentialJSON `json:"items"`
}

type messageResponse struct {
	Message string `json:"message"`
}

type deleteResponse struct {
	Message string `json:"message"`
	Spec    string `json:"spec"`
}

type deleteAllResponse struct {
	Message string `json:"message"`
	Count   int    `json:"count"`
}

type errorResponse struct {
	Message string `json:"message"`
}

// ---------------------------------------------------------------------------
// POST /v3/domains/{domain_name}/credentials -- Create Credential
// ---------------------------------------------------------------------------

func TestCreateCredential_HappyPath(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	rec := createCredential(t, router, "example.com", map[string]string{
		"login":    "alice",
		"password": "secret123",
	})

	t.Run("returns 200", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp messageResponse
	decodeJSON(t, rec, &resp)

	t.Run("returns success message", func(t *testing.T) {
		if resp.Message != "Created 1 credentials pair(s)" {
			t.Errorf("expected %q, got %q", "Created 1 credentials pair(s)", resp.Message)
		}
	})

	// Verify the credential was stored with full email by listing
	t.Run("stored login is full email", func(t *testing.T) {
		listRec := listCredentials(t, router, "example.com", "")
		var listResp listCredentialsResponse
		decodeJSON(t, listRec, &listResp)
		if listResp.TotalCount != 1 {
			t.Fatalf("expected total_count=1, got %d", listResp.TotalCount)
		}
		if listResp.Items[0].Login != "alice@example.com" {
			t.Errorf("expected login %q, got %q", "alice@example.com", listResp.Items[0].Login)
		}
	})
}

func TestCreateCredential_WithFullEmail(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	rec := createCredential(t, router, "example.com", map[string]string{
		"login":    "bob@example.com",
		"password": "secret123",
	})

	t.Run("returns 200", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp messageResponse
	decodeJSON(t, rec, &resp)

	t.Run("returns success message", func(t *testing.T) {
		if resp.Message != "Created 1 credentials pair(s)" {
			t.Errorf("expected %q, got %q", "Created 1 credentials pair(s)", resp.Message)
		}
	})

	// Verify the login is stored as-is (already has @domain)
	t.Run("stored login is full email as provided", func(t *testing.T) {
		listRec := listCredentials(t, router, "example.com", "")
		var listResp listCredentialsResponse
		decodeJSON(t, listRec, &listResp)
		if listResp.TotalCount != 1 {
			t.Fatalf("expected total_count=1, got %d", listResp.TotalCount)
		}
		if listResp.Items[0].Login != "bob@example.com" {
			t.Errorf("expected login %q, got %q", "bob@example.com", listResp.Items[0].Login)
		}
	})
}

func TestCreateCredential_ShortPassword(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	rec := createCredential(t, router, "example.com", map[string]string{
		"login":    "alice",
		"password": "abcd",
	})

	t.Run("returns 400", func(t *testing.T) {
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("body contains error message", func(t *testing.T) {
		var resp errorResponse
		decodeJSON(t, rec, &resp)
		if resp.Message == "" {
			t.Error("expected non-empty error message")
		}
	})
}

func TestCreateCredential_LongPassword(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	longPass := strings.Repeat("a", 33)
	rec := createCredential(t, router, "example.com", map[string]string{
		"login":    "alice",
		"password": longPass,
	})

	t.Run("returns 400", func(t *testing.T) {
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("body contains error message", func(t *testing.T) {
		var resp errorResponse
		decodeJSON(t, rec, &resp)
		if resp.Message == "" {
			t.Error("expected non-empty error message")
		}
	})
}

func TestCreateCredential_PasswordExactlyAtBoundaries(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	t.Run("5-char password succeeds", func(t *testing.T) {
		rec := createCredential(t, router, "example.com", map[string]string{
			"login":    "minpass",
			"password": "abcde",
		})
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("32-char password succeeds", func(t *testing.T) {
		rec := createCredential(t, router, "example.com", map[string]string{
			"login":    "maxpass",
			"password": strings.Repeat("z", 32),
		})
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})
}

func TestCreateCredential_DuplicateLogin(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	// Create the first credential
	rec1 := createCredential(t, router, "example.com", map[string]string{
		"login":    "alice",
		"password": "secret123",
	})
	if rec1.Code != http.StatusOK {
		t.Fatalf("first create failed: status=%d body=%s", rec1.Code, rec1.Body.String())
	}

	// Attempt to create a duplicate
	rec2 := createCredential(t, router, "example.com", map[string]string{
		"login":    "alice",
		"password": "anotherpass",
	})

	t.Run("duplicate returns error status", func(t *testing.T) {
		// Expect a 4xx error (400 or 409 are both acceptable)
		if rec2.Code < 400 || rec2.Code >= 500 {
			t.Errorf("expected 4xx error for duplicate login, got %d (body: %s)", rec2.Code, rec2.Body.String())
		}
	})
}

func TestCreateCredential_MissingLogin(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	rec := createCredential(t, router, "example.com", map[string]string{
		"password": "secret123",
	})

	t.Run("returns 400", func(t *testing.T) {
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("body contains error message", func(t *testing.T) {
		var resp errorResponse
		decodeJSON(t, rec, &resp)
		if resp.Message == "" {
			t.Error("expected non-empty error message")
		}
	})
}

func TestCreateCredential_MissingPassword(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	rec := createCredential(t, router, "example.com", map[string]string{
		"login": "alice",
	})

	t.Run("returns 400", func(t *testing.T) {
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("body contains error message", func(t *testing.T) {
		var resp errorResponse
		decodeJSON(t, rec, &resp)
		if resp.Message == "" {
			t.Error("expected non-empty error message")
		}
	})
}

func TestCreateCredential_NonexistentDomain(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	// Do NOT create the domain

	rec := createCredential(t, router, "no-such-domain.com", map[string]string{
		"login":    "alice",
		"password": "secret123",
	})

	t.Run("returns 404", func(t *testing.T) {
		if rec.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})
}

// ---------------------------------------------------------------------------
// GET /v3/domains/{domain_name}/credentials -- List Credentials
// ---------------------------------------------------------------------------

func TestListCredentials_ReturnsAllWithCorrectFormat(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	// Create multiple credentials
	names := []string{"alice", "bob", "charlie"}
	for _, name := range names {
		rec := createCredential(t, router, "example.com", map[string]string{
			"login":    name,
			"password": "password123",
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("failed to create credential %q: status=%d body=%s", name, rec.Code, rec.Body.String())
		}
	}

	rec := listCredentials(t, router, "example.com", "")

	t.Run("returns 200", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp listCredentialsResponse
	decodeJSON(t, rec, &resp)

	t.Run("total_count matches number of credentials", func(t *testing.T) {
		if resp.TotalCount != 3 {
			t.Errorf("expected total_count=3, got %d", resp.TotalCount)
		}
	})

	t.Run("items length matches total_count", func(t *testing.T) {
		if len(resp.Items) != 3 {
			t.Errorf("expected 3 items, got %d", len(resp.Items))
		}
	})

	t.Run("each item has correct fields", func(t *testing.T) {
		for _, item := range resp.Items {
			if item.Login == "" {
				t.Error("expected non-empty login")
			}
			if !strings.Contains(item.Login, "@example.com") {
				t.Errorf("expected login to contain @example.com, got %q", item.Login)
			}
			if item.Mailbox != item.Login {
				t.Errorf("expected mailbox=%q to equal login=%q", item.Mailbox, item.Login)
			}
			if item.SizeBytes != nil {
				t.Errorf("expected size_bytes to be null, got %v", item.SizeBytes)
			}
			if item.CreatedAt == "" {
				t.Error("expected non-empty created_at")
			}
		}
	})
}

func TestListCredentials_WithSkipAndLimit(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	// Create 5 credentials
	for i := 0; i < 5; i++ {
		login := fmt.Sprintf("user%d", i)
		rec := createCredential(t, router, "example.com", map[string]string{
			"login":    login,
			"password": "password123",
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("failed to create credential %q: status=%d body=%s", login, rec.Code, rec.Body.String())
		}
	}

	t.Run("limit restricts number of items", func(t *testing.T) {
		rec := listCredentials(t, router, "example.com", "limit=2")
		var resp listCredentialsResponse
		decodeJSON(t, rec, &resp)

		if resp.TotalCount != 5 {
			t.Errorf("expected total_count=5, got %d", resp.TotalCount)
		}
		if len(resp.Items) != 2 {
			t.Errorf("expected 2 items, got %d", len(resp.Items))
		}
	})

	t.Run("skip offsets results", func(t *testing.T) {
		rec := listCredentials(t, router, "example.com", "skip=3&limit=10")
		var resp listCredentialsResponse
		decodeJSON(t, rec, &resp)

		if resp.TotalCount != 5 {
			t.Errorf("expected total_count=5, got %d", resp.TotalCount)
		}
		if len(resp.Items) != 2 {
			t.Errorf("expected 2 items (5 - skip 3), got %d", len(resp.Items))
		}
	})

	t.Run("skip beyond total returns empty items", func(t *testing.T) {
		rec := listCredentials(t, router, "example.com", "skip=100")
		var resp listCredentialsResponse
		decodeJSON(t, rec, &resp)

		if resp.TotalCount != 5 {
			t.Errorf("expected total_count=5, got %d", resp.TotalCount)
		}
		if len(resp.Items) != 0 {
			t.Errorf("expected 0 items, got %d", len(resp.Items))
		}
	})

	t.Run("skip=0 and limit=100 are the defaults", func(t *testing.T) {
		rec := listCredentials(t, router, "example.com", "")
		var resp listCredentialsResponse
		decodeJSON(t, rec, &resp)

		if resp.TotalCount != 5 {
			t.Errorf("expected total_count=5, got %d", resp.TotalCount)
		}
		if len(resp.Items) != 5 {
			t.Errorf("expected 5 items with defaults, got %d", len(resp.Items))
		}
	})
}

func TestListCredentials_EmptyList(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	rec := listCredentials(t, router, "example.com", "")

	t.Run("returns 200", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp listCredentialsResponse
	decodeJSON(t, rec, &resp)

	t.Run("total_count is 0", func(t *testing.T) {
		if resp.TotalCount != 0 {
			t.Errorf("expected total_count=0, got %d", resp.TotalCount)
		}
	})

	t.Run("items is empty array not null", func(t *testing.T) {
		// Verify the raw JSON has "items":[] not "items":null
		raw := rec.Body.String()
		if !strings.Contains(raw, `"items":[]`) && !strings.Contains(raw, `"items": []`) {
			// Also check that items is not null
			if strings.Contains(raw, `"items":null`) || strings.Contains(raw, `"items": null`) {
				t.Errorf("expected items to be empty array, not null (body: %s)", raw)
			}
		}
		if resp.Items == nil {
			// Decoded as nil, but we want it to be empty slice in JSON
			// This checks the decoded value
		}
	})
}

func TestListCredentials_NonexistentDomain(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	// Do NOT create the domain

	rec := listCredentials(t, router, "no-such-domain.com", "")

	t.Run("returns 404", func(t *testing.T) {
		if rec.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})
}

// ---------------------------------------------------------------------------
// PUT /v3/domains/{domain_name}/credentials/{spec} -- Update Credential
// ---------------------------------------------------------------------------

func TestUpdateCredential_HappyPath(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	// Create a credential first
	rec := createCredential(t, router, "example.com", map[string]string{
		"login":    "alice",
		"password": "oldpassword",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to create credential: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Update the password
	updateReq := newMultipartRequest(t, http.MethodPut, "/v3/domains/example.com/credentials/alice", map[string]string{
		"password": "newpassword",
	})
	updateRec := httptest.NewRecorder()
	router.ServeHTTP(updateRec, updateReq)

	t.Run("returns 200", func(t *testing.T) {
		if updateRec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", updateRec.Code, updateRec.Body.String())
		}
	})

	var resp messageResponse
	decodeJSON(t, updateRec, &resp)

	t.Run("returns password changed message", func(t *testing.T) {
		if resp.Message != "Password changed" {
			t.Errorf("expected %q, got %q", "Password changed", resp.Message)
		}
	})
}

func TestUpdateCredential_ShortPassword(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	// Create a credential first
	rec := createCredential(t, router, "example.com", map[string]string{
		"login":    "alice",
		"password": "oldpassword",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to create credential: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Attempt to update with too-short password
	updateReq := newMultipartRequest(t, http.MethodPut, "/v3/domains/example.com/credentials/alice", map[string]string{
		"password": "ab",
	})
	updateRec := httptest.NewRecorder()
	router.ServeHTTP(updateRec, updateReq)

	t.Run("returns 400", func(t *testing.T) {
		if updateRec.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d (body: %s)", updateRec.Code, updateRec.Body.String())
		}
	})
}

func TestUpdateCredential_LongPassword(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	// Create a credential first
	rec := createCredential(t, router, "example.com", map[string]string{
		"login":    "alice",
		"password": "oldpassword",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to create credential: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Attempt to update with too-long password
	updateReq := newMultipartRequest(t, http.MethodPut, "/v3/domains/example.com/credentials/alice", map[string]string{
		"password": strings.Repeat("x", 33),
	})
	updateRec := httptest.NewRecorder()
	router.ServeHTTP(updateRec, updateReq)

	t.Run("returns 400", func(t *testing.T) {
		if updateRec.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d (body: %s)", updateRec.Code, updateRec.Body.String())
		}
	})
}

func TestUpdateCredential_NonexistentCredential(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	// Do NOT create any credential, try to update one
	updateReq := newMultipartRequest(t, http.MethodPut, "/v3/domains/example.com/credentials/nobody", map[string]string{
		"password": "newpassword",
	})
	updateRec := httptest.NewRecorder()
	router.ServeHTTP(updateRec, updateReq)

	t.Run("returns 404", func(t *testing.T) {
		if updateRec.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d (body: %s)", updateRec.Code, updateRec.Body.String())
		}
	})
}

func TestUpdateCredential_NonexistentDomain(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	// Do NOT create the domain

	updateReq := newMultipartRequest(t, http.MethodPut, "/v3/domains/no-such-domain.com/credentials/alice", map[string]string{
		"password": "newpassword",
	})
	updateRec := httptest.NewRecorder()
	router.ServeHTTP(updateRec, updateReq)

	t.Run("returns 404", func(t *testing.T) {
		if updateRec.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d (body: %s)", updateRec.Code, updateRec.Body.String())
		}
	})
}

// ---------------------------------------------------------------------------
// DELETE /v3/domains/{domain_name}/credentials/{spec} -- Delete Single
// ---------------------------------------------------------------------------

func TestDeleteCredential_HappyPath(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	// Create a credential first
	rec := createCredential(t, router, "example.com", map[string]string{
		"login":    "alice",
		"password": "secret123",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to create credential: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Delete it
	deleteReq := httptest.NewRequest(http.MethodDelete, "/v3/domains/example.com/credentials/alice", nil)
	deleteRec := httptest.NewRecorder()
	router.ServeHTTP(deleteRec, deleteReq)

	t.Run("returns 200", func(t *testing.T) {
		if deleteRec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", deleteRec.Code, deleteRec.Body.String())
		}
	})

	var resp deleteResponse
	decodeJSON(t, deleteRec, &resp)

	t.Run("returns correct message", func(t *testing.T) {
		if resp.Message != "Credentials have been deleted" {
			t.Errorf("expected %q, got %q", "Credentials have been deleted", resp.Message)
		}
	})

	t.Run("spec contains full email", func(t *testing.T) {
		if resp.Spec != "alice@example.com" {
			t.Errorf("expected spec=%q, got %q", "alice@example.com", resp.Spec)
		}
	})

	// Verify it is gone
	t.Run("credential is no longer listed", func(t *testing.T) {
		listRec := listCredentials(t, router, "example.com", "")
		var listResp listCredentialsResponse
		decodeJSON(t, listRec, &listResp)
		if listResp.TotalCount != 0 {
			t.Errorf("expected total_count=0 after deletion, got %d", listResp.TotalCount)
		}
	})
}

func TestDeleteCredential_NonexistentCredential(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	// Do NOT create any credential, try to delete one
	deleteReq := httptest.NewRequest(http.MethodDelete, "/v3/domains/example.com/credentials/nobody", nil)
	deleteRec := httptest.NewRecorder()
	router.ServeHTTP(deleteRec, deleteReq)

	t.Run("returns 404", func(t *testing.T) {
		if deleteRec.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d (body: %s)", deleteRec.Code, deleteRec.Body.String())
		}
	})
}

func TestDeleteCredential_NonexistentDomain(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	// Do NOT create the domain

	deleteReq := httptest.NewRequest(http.MethodDelete, "/v3/domains/no-such-domain.com/credentials/alice", nil)
	deleteRec := httptest.NewRecorder()
	router.ServeHTTP(deleteRec, deleteReq)

	t.Run("returns 404", func(t *testing.T) {
		if deleteRec.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d (body: %s)", deleteRec.Code, deleteRec.Body.String())
		}
	})
}

// ---------------------------------------------------------------------------
// DELETE /v3/domains/{domain_name}/credentials -- Delete All
// ---------------------------------------------------------------------------

func TestDeleteAllCredentials_HappyPath(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	// Create 3 credentials
	for _, name := range []string{"alice", "bob", "charlie"} {
		rec := createCredential(t, router, "example.com", map[string]string{
			"login":    name,
			"password": "password123",
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("failed to create credential %q: status=%d body=%s", name, rec.Code, rec.Body.String())
		}
	}

	// Delete all
	deleteReq := httptest.NewRequest(http.MethodDelete, "/v3/domains/example.com/credentials", nil)
	deleteRec := httptest.NewRecorder()
	router.ServeHTTP(deleteRec, deleteReq)

	t.Run("returns 200", func(t *testing.T) {
		if deleteRec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", deleteRec.Code, deleteRec.Body.String())
		}
	})

	var resp deleteAllResponse
	decodeJSON(t, deleteRec, &resp)

	t.Run("returns correct message", func(t *testing.T) {
		if resp.Message != "All domain credentials have been deleted" {
			t.Errorf("expected %q, got %q", "All domain credentials have been deleted", resp.Message)
		}
	})

	t.Run("count matches deleted credentials", func(t *testing.T) {
		if resp.Count != 3 {
			t.Errorf("expected count=3, got %d", resp.Count)
		}
	})

	// Verify they are gone
	t.Run("credentials are no longer listed", func(t *testing.T) {
		listRec := listCredentials(t, router, "example.com", "")
		var listResp listCredentialsResponse
		decodeJSON(t, listRec, &listResp)
		if listResp.TotalCount != 0 {
			t.Errorf("expected total_count=0 after delete-all, got %d", listResp.TotalCount)
		}
	})
}

func TestDeleteAllCredentials_WhenNoneExist(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	// Delete all when none exist
	deleteReq := httptest.NewRequest(http.MethodDelete, "/v3/domains/example.com/credentials", nil)
	deleteRec := httptest.NewRecorder()
	router.ServeHTTP(deleteRec, deleteReq)

	t.Run("returns 200", func(t *testing.T) {
		if deleteRec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d (body: %s)", deleteRec.Code, deleteRec.Body.String())
		}
	})

	var resp deleteAllResponse
	decodeJSON(t, deleteRec, &resp)

	t.Run("returns correct message", func(t *testing.T) {
		if resp.Message != "All domain credentials have been deleted" {
			t.Errorf("expected %q, got %q", "All domain credentials have been deleted", resp.Message)
		}
	})

	t.Run("count is 0", func(t *testing.T) {
		if resp.Count != 0 {
			t.Errorf("expected count=0, got %d", resp.Count)
		}
	})
}

func TestDeleteAllCredentials_NonexistentDomain(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	// Do NOT create the domain

	deleteReq := httptest.NewRequest(http.MethodDelete, "/v3/domains/no-such-domain.com/credentials", nil)
	deleteRec := httptest.NewRecorder()
	router.ServeHTTP(deleteRec, deleteReq)

	t.Run("returns 404", func(t *testing.T) {
		if deleteRec.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d (body: %s)", deleteRec.Code, deleteRec.Body.String())
		}
	})
}

// ---------------------------------------------------------------------------
// Cross-domain isolation
// ---------------------------------------------------------------------------

func TestCredentials_IsolatedBetweenDomains(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createTestDomain(t, router, "alpha.com")
	createTestDomain(t, router, "beta.com")

	// Create credential on alpha.com
	rec := createCredential(t, router, "alpha.com", map[string]string{
		"login":    "alice",
		"password": "secret123",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to create credential on alpha.com: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Create credential on beta.com
	rec2 := createCredential(t, router, "beta.com", map[string]string{
		"login":    "bob",
		"password": "secret456",
	})
	if rec2.Code != http.StatusOK {
		t.Fatalf("failed to create credential on beta.com: status=%d body=%s", rec2.Code, rec2.Body.String())
	}

	t.Run("alpha.com only sees its own credentials", func(t *testing.T) {
		listRec := listCredentials(t, router, "alpha.com", "")
		var resp listCredentialsResponse
		decodeJSON(t, listRec, &resp)
		if resp.TotalCount != 1 {
			t.Errorf("expected total_count=1 for alpha.com, got %d", resp.TotalCount)
		}
		if len(resp.Items) > 0 && resp.Items[0].Login != "alice@alpha.com" {
			t.Errorf("expected login %q, got %q", "alice@alpha.com", resp.Items[0].Login)
		}
	})

	t.Run("beta.com only sees its own credentials", func(t *testing.T) {
		listRec := listCredentials(t, router, "beta.com", "")
		var resp listCredentialsResponse
		decodeJSON(t, listRec, &resp)
		if resp.TotalCount != 1 {
			t.Errorf("expected total_count=1 for beta.com, got %d", resp.TotalCount)
		}
		if len(resp.Items) > 0 && resp.Items[0].Login != "bob@beta.com" {
			t.Errorf("expected login %q, got %q", "bob@beta.com", resp.Items[0].Login)
		}
	})

	t.Run("same local-part can exist on different domains", func(t *testing.T) {
		// Create alice on beta.com too
		rec3 := createCredential(t, router, "beta.com", map[string]string{
			"login":    "alice",
			"password": "different",
		})
		if rec3.Code != http.StatusOK {
			t.Errorf("expected 200 creating alice on beta.com (different domain), got %d (body: %s)", rec3.Code, rec3.Body.String())
		}
	})
}

// ---------------------------------------------------------------------------
// Full Lifecycle Test
// ---------------------------------------------------------------------------

func TestCredentialLifecycle(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	// Step 1: Create credential
	t.Run("create credential", func(t *testing.T) {
		rec := createCredential(t, router, "example.com", map[string]string{
			"login":    "alice",
			"password": "initial123",
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("create failed: status=%d body=%s", rec.Code, rec.Body.String())
		}
		var resp messageResponse
		decodeJSON(t, rec, &resp)
		if resp.Message != "Created 1 credentials pair(s)" {
			t.Errorf("expected %q, got %q", "Created 1 credentials pair(s)", resp.Message)
		}
	})

	// Step 2: List credentials and verify the created one
	t.Run("list shows created credential", func(t *testing.T) {
		rec := listCredentials(t, router, "example.com", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("list failed: status=%d body=%s", rec.Code, rec.Body.String())
		}
		var resp listCredentialsResponse
		decodeJSON(t, rec, &resp)
		if resp.TotalCount != 1 {
			t.Fatalf("expected total_count=1, got %d", resp.TotalCount)
		}
		item := resp.Items[0]
		if item.Login != "alice@example.com" {
			t.Errorf("expected login %q, got %q", "alice@example.com", item.Login)
		}
		if item.Mailbox != "alice@example.com" {
			t.Errorf("expected mailbox %q, got %q", "alice@example.com", item.Mailbox)
		}
		if item.SizeBytes != nil {
			t.Errorf("expected size_bytes to be null, got %v", item.SizeBytes)
		}
		if item.CreatedAt == "" {
			t.Error("expected non-empty created_at")
		}
	})

	// Step 3: Update the password
	t.Run("update password", func(t *testing.T) {
		req := newMultipartRequest(t, http.MethodPut, "/v3/domains/example.com/credentials/alice", map[string]string{
			"password": "changed456",
		})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("update failed: status=%d body=%s", rec.Code, rec.Body.String())
		}
		var resp messageResponse
		decodeJSON(t, rec, &resp)
		if resp.Message != "Password changed" {
			t.Errorf("expected %q, got %q", "Password changed", resp.Message)
		}
	})

	// Step 4: List again -- password must NOT be exposed
	t.Run("list after update does not expose password", func(t *testing.T) {
		rec := listCredentials(t, router, "example.com", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("list failed: status=%d body=%s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if strings.Contains(body, "initial123") || strings.Contains(body, "changed456") {
			t.Errorf("response body should not contain password, got: %s", body)
		}
		var resp listCredentialsResponse
		decodeJSON(t, rec, &resp)
		if resp.TotalCount != 1 {
			t.Errorf("expected total_count=1, got %d", resp.TotalCount)
		}
	})

	// Step 5: Create a second credential
	t.Run("create second credential", func(t *testing.T) {
		rec := createCredential(t, router, "example.com", map[string]string{
			"login":    "bob",
			"password": "bobpass12",
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("create failed: status=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	// Step 6: List should now have 2
	t.Run("list shows 2 credentials", func(t *testing.T) {
		rec := listCredentials(t, router, "example.com", "")
		var resp listCredentialsResponse
		decodeJSON(t, rec, &resp)
		if resp.TotalCount != 2 {
			t.Errorf("expected total_count=2, got %d", resp.TotalCount)
		}
	})

	// Step 7: Delete alice
	t.Run("delete alice", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/v3/domains/example.com/credentials/alice", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("delete failed: status=%d body=%s", rec.Code, rec.Body.String())
		}
		var resp deleteResponse
		decodeJSON(t, rec, &resp)
		if resp.Spec != "alice@example.com" {
			t.Errorf("expected spec=%q, got %q", "alice@example.com", resp.Spec)
		}
	})

	// Step 8: Verify alice is gone but bob remains
	t.Run("alice is gone but bob remains", func(t *testing.T) {
		rec := listCredentials(t, router, "example.com", "")
		var resp listCredentialsResponse
		decodeJSON(t, rec, &resp)
		if resp.TotalCount != 1 {
			t.Fatalf("expected total_count=1, got %d", resp.TotalCount)
		}
		if resp.Items[0].Login != "bob@example.com" {
			t.Errorf("expected remaining login %q, got %q", "bob@example.com", resp.Items[0].Login)
		}
	})

	// Step 9: Delete all
	t.Run("delete all remaining", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/v3/domains/example.com/credentials", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("delete-all failed: status=%d body=%s", rec.Code, rec.Body.String())
		}
		var resp deleteAllResponse
		decodeJSON(t, rec, &resp)
		if resp.Count != 1 {
			t.Errorf("expected count=1, got %d", resp.Count)
		}
	})

	// Step 10: Verify empty
	t.Run("list is empty after delete-all", func(t *testing.T) {
		rec := listCredentials(t, router, "example.com", "")
		var resp listCredentialsResponse
		decodeJSON(t, rec, &resp)
		if resp.TotalCount != 0 {
			t.Errorf("expected total_count=0, got %d", resp.TotalCount)
		}
	})
}

// ---------------------------------------------------------------------------
// created_at format test
// ---------------------------------------------------------------------------

func TestListCredentials_CreatedAtFormat(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createTestDomain(t, router, "example.com")

	rec := createCredential(t, router, "example.com", map[string]string{
		"login":    "alice",
		"password": "secret123",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to create credential: status=%d body=%s", rec.Code, rec.Body.String())
	}

	listRec := listCredentials(t, router, "example.com", "")
	var resp listCredentialsResponse
	decodeJSON(t, listRec, &resp)

	t.Run("created_at is RFC 2822 format", func(t *testing.T) {
		if len(resp.Items) == 0 {
			t.Fatal("expected at least one item")
		}
		createdAt := resp.Items[0].CreatedAt
		// RFC 2822 format looks like: "Wed, 08 Mar 2023 23:34:57 +0000"
		// It should contain a comma, day of week abbreviation, timezone offset
		if !strings.Contains(createdAt, ",") {
			t.Errorf("expected created_at in RFC 2822 format (containing comma), got %q", createdAt)
		}
		if !strings.Contains(createdAt, "+") && !strings.Contains(createdAt, "-") {
			t.Errorf("expected created_at in RFC 2822 format (containing timezone offset), got %q", createdAt)
		}
	})
}
