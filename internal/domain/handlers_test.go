package domain_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
// Domain and DNSRecord tables migrated.
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}
	if err := db.AutoMigrate(&domain.Domain{}, &domain.DNSRecord{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

// setupRouter creates a chi router with all domain routes registered.
func setupRouter(db *gorm.DB, cfg *mock.MockConfig) http.Handler {
	domain.ResetForTests(db)
	h := domain.NewHandlers(db, cfg)
	r := chi.NewRouter()
	r.Route("/v4/domains", func(r chi.Router) {
		r.Post("/", h.CreateDomain)
		r.Get("/", h.ListDomains)
		r.Get("/{name}", h.GetDomain)
		r.Put("/{name}", h.UpdateDomain)
		r.Put("/{name}/verify", h.VerifyDomain)
	})
	// DELETE is v3, not v4!
	r.Delete("/v3/domains/{name}", h.DeleteDomain)
	return r
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

// manualVerifyConfig returns a MockConfig with auto-verify disabled.
func manualVerifyConfig() *mock.MockConfig {
	return &mock.MockConfig{
		DomainBehavior: mock.DomainBehaviorConfig{
			DomainAutoVerify: false,
			SandboxDomain:    "sandbox123.mailgun.org",
		},
	}
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

// createDomainResponse represents the JSON response from create/get/update/verify endpoints.
type createDomainResponse struct {
	Message             string           `json:"message"`
	Domain              domainJSON       `json:"domain"`
	ReceivingDNSRecords []dnsRecordJSON  `json:"receiving_dns_records"`
	SendingDNSRecords   []dnsRecordJSON  `json:"sending_dns_records"`
}

// domainJSON represents the domain object in JSON responses.
type domainJSON struct {
	ID                         string      `json:"id"`
	Name                       string      `json:"name"`
	State                      string      `json:"state"`
	Type                       string      `json:"type"`
	CreatedAt                  string      `json:"created_at"`
	SMTPLogin                  string      `json:"smtp_login"`
	SMTPPassword               interface{} `json:"smtp_password"`
	SpamAction                 string      `json:"spam_action"`
	Wildcard                   bool        `json:"wildcard"`
	RequireTLS                 bool        `json:"require_tls"`
	SkipVerification           bool        `json:"skip_verification"`
	IsDisabled                 bool        `json:"is_disabled"`
	WebPrefix                  string      `json:"web_prefix"`
	WebScheme                  string      `json:"web_scheme"`
	UseAutomaticSenderSecurity bool        `json:"use_automatic_sender_security"`
	MessageTTL                 int         `json:"message_ttl"`
}

// dnsRecordJSON represents a DNS record in JSON responses.
type dnsRecordJSON struct {
	RecordType string      `json:"record_type"`
	Name       string      `json:"name"`
	Value      string      `json:"value"`
	Priority   interface{} `json:"priority"`
	Valid      string      `json:"valid"`
	IsActive   bool        `json:"is_active"`
	Cached     []string    `json:"cached"`
}

// listDomainsResponse represents the JSON response from the list domains endpoint.
type listDomainsResponse struct {
	TotalCount int          `json:"total_count"`
	Items      []domainJSON `json:"items"`
}

// errorResponse represents a JSON error response body.
type errorResponse struct {
	Message string `json:"message"`
}

// deleteResponse represents the JSON response from the delete endpoint.
type deleteResponse struct {
	Message string `json:"message"`
}

// createDomainViaMultipart is a convenience helper that creates a domain using
// multipart form data and returns the recorder.
func createDomainViaMultipart(t *testing.T, router http.Handler, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := newMultipartRequest(t, http.MethodPost, "/v4/domains", fields)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// createDomainViaJSON is a convenience helper that creates a domain using JSON
// and returns the recorder.
func createDomainViaJSON(t *testing.T, router http.Handler, body map[string]interface{}) *httptest.ResponseRecorder {
	t.Helper()
	req := newJSONRequest(t, http.MethodPost, "/v4/domains", body)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// ---------------------------------------------------------------------------
// POST /v4/domains — Create Domain
// ---------------------------------------------------------------------------

func TestCreateDomain_MinimalFields(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	rec := createDomainViaMultipart(t, router, map[string]string{
		"name": "example.com",
	})

	t.Run("returns 200 status", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp createDomainResponse
	decodeJSON(t, rec, &resp)

	t.Run("returns success message", func(t *testing.T) {
		if resp.Message != "Domain has been created" {
			t.Errorf("expected message %q, got %q", "Domain has been created", resp.Message)
		}
	})

	t.Run("domain name matches", func(t *testing.T) {
		if resp.Domain.Name != "example.com" {
			t.Errorf("expected name %q, got %q", "example.com", resp.Domain.Name)
		}
	})

	t.Run("domain has an ID", func(t *testing.T) {
		if resp.Domain.ID == "" {
			t.Error("expected non-empty domain ID")
		}
	})

	t.Run("domain type is custom", func(t *testing.T) {
		if resp.Domain.Type != "custom" {
			t.Errorf("expected type %q, got %q", "custom", resp.Domain.Type)
		}
	})

	t.Run("smtp_login is auto-generated", func(t *testing.T) {
		expected := "postmaster@example.com"
		if resp.Domain.SMTPLogin != expected {
			t.Errorf("expected smtp_login %q, got %q", expected, resp.Domain.SMTPLogin)
		}
	})

	t.Run("spam_action defaults to disabled", func(t *testing.T) {
		if resp.Domain.SpamAction != "disabled" {
			t.Errorf("expected spam_action %q, got %q", "disabled", resp.Domain.SpamAction)
		}
	})

	t.Run("wildcard defaults to false", func(t *testing.T) {
		if resp.Domain.Wildcard != false {
			t.Errorf("expected wildcard=false, got %v", resp.Domain.Wildcard)
		}
	})

	t.Run("web_scheme defaults to https", func(t *testing.T) {
		if resp.Domain.WebScheme != "https" {
			t.Errorf("expected web_scheme %q, got %q", "https", resp.Domain.WebScheme)
		}
	})

	t.Run("web_prefix defaults to email", func(t *testing.T) {
		if resp.Domain.WebPrefix != "email" {
			t.Errorf("expected web_prefix %q, got %q", "email", resp.Domain.WebPrefix)
		}
	})

	t.Run("require_tls defaults to false", func(t *testing.T) {
		if resp.Domain.RequireTLS != false {
			t.Errorf("expected require_tls=false, got %v", resp.Domain.RequireTLS)
		}
	})

	t.Run("skip_verification defaults to false", func(t *testing.T) {
		if resp.Domain.SkipVerification != false {
			t.Errorf("expected skip_verification=false, got %v", resp.Domain.SkipVerification)
		}
	})

	t.Run("use_automatic_sender_security defaults to true", func(t *testing.T) {
		if resp.Domain.UseAutomaticSenderSecurity != true {
			t.Errorf("expected use_automatic_sender_security=true, got %v", resp.Domain.UseAutomaticSenderSecurity)
		}
	})

	t.Run("message_ttl defaults to 259200", func(t *testing.T) {
		if resp.Domain.MessageTTL != 259200 {
			t.Errorf("expected message_ttl=259200, got %d", resp.Domain.MessageTTL)
		}
	})

	t.Run("is_disabled defaults to false", func(t *testing.T) {
		if resp.Domain.IsDisabled != false {
			t.Errorf("expected is_disabled=false, got %v", resp.Domain.IsDisabled)
		}
	})

	t.Run("created_at is non-empty", func(t *testing.T) {
		if resp.Domain.CreatedAt == "" {
			t.Error("expected non-empty created_at")
		}
	})
}

func TestCreateDomain_AllOptionalFields(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	rec := createDomainViaMultipart(t, router, map[string]string{
		"name":                          "custom.example.com",
		"smtp_password":                 "secret123",
		"spam_action":                   "tag",
		"wildcard":                      "true",
		"force_dkim_authority":          "true",
		"dkim_key_size":                 "1024",
		"web_scheme":                    "http",
		"web_prefix":                    "tracking",
		"require_tls":                   "true",
		"skip_verification":             "true",
		"use_automatic_sender_security": "false",
		"message_ttl":                   "86400",
	})

	t.Run("returns 200 status", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp createDomainResponse
	decodeJSON(t, rec, &resp)

	t.Run("domain name matches", func(t *testing.T) {
		if resp.Domain.Name != "custom.example.com" {
			t.Errorf("expected name %q, got %q", "custom.example.com", resp.Domain.Name)
		}
	})

	t.Run("spam_action is tag", func(t *testing.T) {
		if resp.Domain.SpamAction != "tag" {
			t.Errorf("expected spam_action %q, got %q", "tag", resp.Domain.SpamAction)
		}
	})

	t.Run("wildcard is true", func(t *testing.T) {
		if resp.Domain.Wildcard != true {
			t.Errorf("expected wildcard=true, got %v", resp.Domain.Wildcard)
		}
	})

	t.Run("web_scheme is http", func(t *testing.T) {
		if resp.Domain.WebScheme != "http" {
			t.Errorf("expected web_scheme %q, got %q", "http", resp.Domain.WebScheme)
		}
	})

	t.Run("web_prefix is tracking", func(t *testing.T) {
		if resp.Domain.WebPrefix != "tracking" {
			t.Errorf("expected web_prefix %q, got %q", "tracking", resp.Domain.WebPrefix)
		}
	})

	t.Run("require_tls is true", func(t *testing.T) {
		if resp.Domain.RequireTLS != true {
			t.Errorf("expected require_tls=true, got %v", resp.Domain.RequireTLS)
		}
	})

	t.Run("skip_verification is true", func(t *testing.T) {
		if resp.Domain.SkipVerification != true {
			t.Errorf("expected skip_verification=true, got %v", resp.Domain.SkipVerification)
		}
	})

	t.Run("use_automatic_sender_security is false", func(t *testing.T) {
		if resp.Domain.UseAutomaticSenderSecurity != false {
			t.Errorf("expected use_automatic_sender_security=false, got %v", resp.Domain.UseAutomaticSenderSecurity)
		}
	})

	t.Run("message_ttl is 86400", func(t *testing.T) {
		if resp.Domain.MessageTTL != 86400 {
			t.Errorf("expected message_ttl=86400, got %d", resp.Domain.MessageTTL)
		}
	})

	t.Run("smtp_login is auto-generated", func(t *testing.T) {
		expected := "postmaster@custom.example.com"
		if resp.Domain.SMTPLogin != expected {
			t.Errorf("expected smtp_login %q, got %q", expected, resp.Domain.SMTPLogin)
		}
	})
}

func TestCreateDomain_DuplicateName(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	// Create the first domain.
	createDomainViaMultipart(t, router, map[string]string{
		"name": "duplicate.com",
	})

	// Attempt to create a duplicate.
	rec := createDomainViaMultipart(t, router, map[string]string{
		"name": "duplicate.com",
	})

	t.Run("returns 400 status", func(t *testing.T) {
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("returns error message", func(t *testing.T) {
		var resp errorResponse
		decodeJSON(t, rec, &resp)
		if resp.Message == "" {
			t.Error("expected non-empty error message for duplicate domain")
		}
	})
}

func TestCreateDomain_MissingName(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	// Create without the required "name" field.
	rec := createDomainViaMultipart(t, router, map[string]string{
		"spam_action": "disabled",
	})

	t.Run("returns 400 status", func(t *testing.T) {
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("returns error message", func(t *testing.T) {
		var resp errorResponse
		decodeJSON(t, rec, &resp)
		if resp.Message == "" {
			t.Error("expected non-empty error message for missing name")
		}
	})
}

func TestCreateDomain_InvalidSpamAction(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	rec := createDomainViaMultipart(t, router, map[string]string{
		"name":        "spam-test.com",
		"spam_action": "invalid_action",
	})

	t.Run("returns 400 status", func(t *testing.T) {
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("returns error message", func(t *testing.T) {
		var resp errorResponse
		decodeJSON(t, rec, &resp)
		if resp.Message == "" {
			t.Error("expected non-empty error message for invalid spam_action")
		}
	})
}

func TestCreateDomain_DNSRecordsGenerated(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	rec := createDomainViaMultipart(t, router, map[string]string{
		"name": "dns-test.com",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp createDomainResponse
	decodeJSON(t, rec, &resp)

	t.Run("receiving DNS records are present", func(t *testing.T) {
		if len(resp.ReceivingDNSRecords) == 0 {
			t.Error("expected at least one receiving DNS record")
		}
	})

	t.Run("sending DNS records are present", func(t *testing.T) {
		if len(resp.SendingDNSRecords) == 0 {
			t.Error("expected at least one sending DNS record")
		}
	})

	t.Run("has MX receiving records", func(t *testing.T) {
		mxCount := 0
		for _, rec := range resp.ReceivingDNSRecords {
			if rec.RecordType == "MX" {
				mxCount++
			}
		}
		if mxCount < 2 {
			t.Errorf("expected at least 2 MX receiving records, got %d", mxCount)
		}
	})

	t.Run("has SPF TXT sending record", func(t *testing.T) {
		found := false
		for _, rec := range resp.SendingDNSRecords {
			if rec.RecordType == "TXT" && strings.Contains(rec.Value, "v=spf1") {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected an SPF TXT sending record")
		}
	})

	t.Run("has DKIM TXT sending record", func(t *testing.T) {
		found := false
		for _, rec := range resp.SendingDNSRecords {
			if rec.RecordType == "TXT" && strings.Contains(rec.Name, "domainkey") {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected a DKIM TXT sending record")
		}
	})

	t.Run("has tracking CNAME sending record", func(t *testing.T) {
		found := false
		for _, rec := range resp.SendingDNSRecords {
			if rec.RecordType == "CNAME" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected a tracking CNAME sending record")
		}
	})

	t.Run("DNS records have cached field as empty array", func(t *testing.T) {
		allRecords := append(resp.SendingDNSRecords, resp.ReceivingDNSRecords...)
		for i, rec := range allRecords {
			if rec.Cached == nil {
				t.Errorf("record %d: expected cached to be empty array, got nil", i)
			}
		}
	})
}

func TestCreateDomain_AutoVerifyMode_StateActive(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig() // auto-verify enabled
	router := setupRouter(db, cfg)

	rec := createDomainViaMultipart(t, router, map[string]string{
		"name": "auto-verify.com",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp createDomainResponse
	decodeJSON(t, rec, &resp)

	t.Run("domain state is active in auto-verify mode", func(t *testing.T) {
		if resp.Domain.State != "active" {
			t.Errorf("expected state %q in auto-verify mode, got %q", "active", resp.Domain.State)
		}
	})

	t.Run("DNS records are valid in auto-verify mode", func(t *testing.T) {
		allRecords := append(resp.SendingDNSRecords, resp.ReceivingDNSRecords...)
		for _, rec := range allRecords {
			if rec.Valid != "valid" {
				t.Errorf("expected DNS record valid=%q in auto-verify mode, got %q (record: %s %s)",
					"valid", rec.Valid, rec.RecordType, rec.Name)
			}
		}
	})
}

func TestCreateDomain_ManualVerifyMode_StateUnverified(t *testing.T) {
	db := setupTestDB(t)
	cfg := manualVerifyConfig() // auto-verify disabled
	router := setupRouter(db, cfg)

	rec := createDomainViaMultipart(t, router, map[string]string{
		"name": "manual-verify.com",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp createDomainResponse
	decodeJSON(t, rec, &resp)

	t.Run("domain state is unverified in manual mode", func(t *testing.T) {
		if resp.Domain.State != "unverified" {
			t.Errorf("expected state %q in manual mode, got %q", "unverified", resp.Domain.State)
		}
	})

	t.Run("DNS records are unknown in manual mode", func(t *testing.T) {
		allRecords := append(resp.SendingDNSRecords, resp.ReceivingDNSRecords...)
		for _, rec := range allRecords {
			if rec.Valid != "unknown" {
				t.Errorf("expected DNS record valid=%q in manual mode, got %q (record: %s %s)",
					"unknown", rec.Valid, rec.RecordType, rec.Name)
			}
		}
	})
}

func TestCreateDomain_ViaJSON(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	rec := createDomainViaJSON(t, router, map[string]interface{}{
		"name":        "json-create.com",
		"spam_action": "block",
		"wildcard":    true,
		"message_ttl": 3600,
	})

	t.Run("returns 200 status", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp createDomainResponse
	decodeJSON(t, rec, &resp)

	t.Run("domain name matches", func(t *testing.T) {
		if resp.Domain.Name != "json-create.com" {
			t.Errorf("expected name %q, got %q", "json-create.com", resp.Domain.Name)
		}
	})

	t.Run("spam_action is block", func(t *testing.T) {
		if resp.Domain.SpamAction != "block" {
			t.Errorf("expected spam_action %q, got %q", "block", resp.Domain.SpamAction)
		}
	})

	t.Run("wildcard is true", func(t *testing.T) {
		if resp.Domain.Wildcard != true {
			t.Errorf("expected wildcard=true, got %v", resp.Domain.Wildcard)
		}
	})

	t.Run("message_ttl is 3600", func(t *testing.T) {
		if resp.Domain.MessageTTL != 3600 {
			t.Errorf("expected message_ttl=3600, got %d", resp.Domain.MessageTTL)
		}
	})

	t.Run("success message matches", func(t *testing.T) {
		if resp.Message != "Domain has been created" {
			t.Errorf("expected message %q, got %q", "Domain has been created", resp.Message)
		}
	})
}

// ---------------------------------------------------------------------------
// GET /v4/domains — List Domains
// ---------------------------------------------------------------------------

func TestListDomains_Empty(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	req := httptest.NewRequest(http.MethodGet, "/v4/domains", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	t.Run("returns 200 status", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}
	})

	var resp listDomainsResponse
	decodeJSON(t, rec, &resp)

	t.Run("total_count is 0", func(t *testing.T) {
		if resp.TotalCount != 0 {
			t.Errorf("expected total_count=0, got %d", resp.TotalCount)
		}
	})

	t.Run("items is empty array", func(t *testing.T) {
		if resp.Items == nil {
			t.Error("expected items to be empty array, got nil")
		}
		if len(resp.Items) != 0 {
			t.Errorf("expected 0 items, got %d", len(resp.Items))
		}
	})
}

func TestListDomains_MultipleDomains(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	// Create 3 domains.
	domainNames := []string{"alpha.com", "beta.com", "gamma.com"}
	for _, name := range domainNames {
		rec := createDomainViaMultipart(t, router, map[string]string{"name": name})
		if rec.Code != http.StatusOK {
			t.Fatalf("failed to create domain %q: status %d", name, rec.Code)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/v4/domains", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	var resp listDomainsResponse
	decodeJSON(t, rec, &resp)

	t.Run("returns 200 status", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}
	})

	t.Run("total_count is 3", func(t *testing.T) {
		if resp.TotalCount != 3 {
			t.Errorf("expected total_count=3, got %d", resp.TotalCount)
		}
	})

	t.Run("items has 3 entries", func(t *testing.T) {
		if len(resp.Items) != 3 {
			t.Errorf("expected 3 items, got %d", len(resp.Items))
		}
	})

	t.Run("all created domains are in the list", func(t *testing.T) {
		names := make(map[string]bool)
		for _, item := range resp.Items {
			names[item.Name] = true
		}
		for _, expected := range domainNames {
			if !names[expected] {
				t.Errorf("expected domain %q in list, but not found", expected)
			}
		}
	})
}

func TestListDomains_Pagination(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	// Create 5 domains.
	for i := 0; i < 5; i++ {
		name := fmt.Sprintf("page-%d.com", i)
		rec := createDomainViaMultipart(t, router, map[string]string{"name": name})
		if rec.Code != http.StatusOK {
			t.Fatalf("failed to create domain %q: status %d", name, rec.Code)
		}
	}

	t.Run("skip and limit work correctly", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v4/domains?skip=2&limit=2", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		var resp listDomainsResponse
		decodeJSON(t, rec, &resp)

		// total_count should still reflect total number of domains, not the page size.
		if resp.TotalCount != 5 {
			t.Errorf("expected total_count=5, got %d", resp.TotalCount)
		}

		if len(resp.Items) != 2 {
			t.Errorf("expected 2 items with limit=2, got %d", len(resp.Items))
		}
	})

	t.Run("skip beyond total returns empty items", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v4/domains?skip=100", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		var resp listDomainsResponse
		decodeJSON(t, rec, &resp)

		if resp.TotalCount != 5 {
			t.Errorf("expected total_count=5, got %d", resp.TotalCount)
		}

		if len(resp.Items) != 0 {
			t.Errorf("expected 0 items when skip exceeds total, got %d", len(resp.Items))
		}
	})

	t.Run("limit=1 returns exactly one item", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v4/domains?limit=1", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		var resp listDomainsResponse
		decodeJSON(t, rec, &resp)

		if len(resp.Items) != 1 {
			t.Errorf("expected 1 item with limit=1, got %d", len(resp.Items))
		}
	})

	t.Run("default limit returns all when under limit", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v4/domains", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		var resp listDomainsResponse
		decodeJSON(t, rec, &resp)

		if len(resp.Items) != 5 {
			t.Errorf("expected 5 items with default limit, got %d", len(resp.Items))
		}
	})
}

func TestListDomains_FilterByState(t *testing.T) {
	db := setupTestDB(t)
	cfg := manualVerifyConfig() // domains start as "unverified"
	router := setupRouter(db, cfg)

	// Create two domains in manual mode (both unverified).
	createDomainViaMultipart(t, router, map[string]string{"name": "unverified1.com"})
	createDomainViaMultipart(t, router, map[string]string{"name": "unverified2.com"})

	// Verify one domain to make it active.
	verifyReq := httptest.NewRequest(http.MethodPut, "/v4/domains/unverified1.com/verify", nil)
	verifyRec := httptest.NewRecorder()
	router.ServeHTTP(verifyRec, verifyReq)
	if verifyRec.Code != http.StatusOK {
		t.Fatalf("failed to verify domain: status %d", verifyRec.Code)
	}

	t.Run("filter state=active returns only active domains", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v4/domains?state=active", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		var resp listDomainsResponse
		decodeJSON(t, rec, &resp)

		if resp.TotalCount != 1 {
			t.Errorf("expected total_count=1 for state=active, got %d", resp.TotalCount)
		}
		if len(resp.Items) != 1 {
			t.Errorf("expected 1 item for state=active, got %d", len(resp.Items))
		}
		if len(resp.Items) > 0 && resp.Items[0].Name != "unverified1.com" {
			t.Errorf("expected active domain to be %q, got %q", "unverified1.com", resp.Items[0].Name)
		}
	})

	t.Run("filter state=unverified returns only unverified domains", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v4/domains?state=unverified", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		var resp listDomainsResponse
		decodeJSON(t, rec, &resp)

		if resp.TotalCount != 1 {
			t.Errorf("expected total_count=1 for state=unverified, got %d", resp.TotalCount)
		}
		if len(resp.Items) > 0 && resp.Items[0].Name != "unverified2.com" {
			t.Errorf("expected unverified domain to be %q, got %q", "unverified2.com", resp.Items[0].Name)
		}
	})
}

func TestListDomains_SearchByName(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	createDomainViaMultipart(t, router, map[string]string{"name": "search-alpha.com"})
	createDomainViaMultipart(t, router, map[string]string{"name": "search-beta.com"})
	createDomainViaMultipart(t, router, map[string]string{"name": "other.org"})

	t.Run("search substring matches relevant domains", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v4/domains?search=search", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		var resp listDomainsResponse
		decodeJSON(t, rec, &resp)

		if resp.TotalCount != 2 {
			t.Errorf("expected total_count=2 for search=search, got %d", resp.TotalCount)
		}
		if len(resp.Items) != 2 {
			t.Errorf("expected 2 items for search=search, got %d", len(resp.Items))
		}
	})

	t.Run("search with no match returns empty", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v4/domains?search=nonexistent", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		var resp listDomainsResponse
		decodeJSON(t, rec, &resp)

		if resp.TotalCount != 0 {
			t.Errorf("expected total_count=0 for no match, got %d", resp.TotalCount)
		}
		if len(resp.Items) != 0 {
			t.Errorf("expected 0 items for no match, got %d", len(resp.Items))
		}
	})
}

func TestListDomains_LimitClampedToMax(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	// Create a single domain to have something to list.
	createDomainViaMultipart(t, router, map[string]string{"name": "clamp-test.com"})

	t.Run("limit exceeding 1000 is clamped to 1000", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v4/domains?limit=5000", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		// The request should succeed (200) regardless of the over-limit value.
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}

		var resp listDomainsResponse
		decodeJSON(t, rec, &resp)

		// We only have 1 domain, so items length should be 1 regardless of limit.
		if len(resp.Items) != 1 {
			t.Errorf("expected 1 item, got %d", len(resp.Items))
		}
	})
}

// ---------------------------------------------------------------------------
// GET /v4/domains/{name} — Get Single Domain
// ---------------------------------------------------------------------------

func TestGetDomain_Exists(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	createDomainViaMultipart(t, router, map[string]string{"name": "get-test.com"})

	req := httptest.NewRequest(http.MethodGet, "/v4/domains/get-test.com", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	t.Run("returns 200 status", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp createDomainResponse
	decodeJSON(t, rec, &resp)

	t.Run("domain name matches", func(t *testing.T) {
		if resp.Domain.Name != "get-test.com" {
			t.Errorf("expected name %q, got %q", "get-test.com", resp.Domain.Name)
		}
	})

	t.Run("includes receiving DNS records", func(t *testing.T) {
		if resp.ReceivingDNSRecords == nil {
			t.Error("expected receiving_dns_records to be present")
		}
	})

	t.Run("includes sending DNS records", func(t *testing.T) {
		if resp.SendingDNSRecords == nil {
			t.Error("expected sending_dns_records to be present")
		}
	})

	t.Run("domain has an ID", func(t *testing.T) {
		if resp.Domain.ID == "" {
			t.Error("expected non-empty domain ID")
		}
	})
}

func TestGetDomain_NotFound(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	req := httptest.NewRequest(http.MethodGet, "/v4/domains/nonexistent.com", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	t.Run("returns 404 status", func(t *testing.T) {
		if rec.Code != http.StatusNotFound {
			t.Errorf("expected status 404, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("returns domain not found message", func(t *testing.T) {
		var resp errorResponse
		decodeJSON(t, rec, &resp)
		if resp.Message != "Domain not found" {
			t.Errorf("expected message %q, got %q", "Domain not found", resp.Message)
		}
	})
}

// ---------------------------------------------------------------------------
// PUT /v4/domains/{name} — Update Domain
// ---------------------------------------------------------------------------

func TestUpdateDomain_SingleField(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	createDomainViaMultipart(t, router, map[string]string{"name": "update-test.com"})

	req := newMultipartRequest(t, http.MethodPut, "/v4/domains/update-test.com", map[string]string{
		"spam_action": "tag",
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	t.Run("returns 200 status", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp createDomainResponse
	decodeJSON(t, rec, &resp)

	t.Run("returns success message", func(t *testing.T) {
		if resp.Message != "Domain has been updated" {
			t.Errorf("expected message %q, got %q", "Domain has been updated", resp.Message)
		}
	})

	t.Run("updated field is changed", func(t *testing.T) {
		if resp.Domain.SpamAction != "tag" {
			t.Errorf("expected spam_action %q, got %q", "tag", resp.Domain.SpamAction)
		}
	})

	t.Run("other fields remain unchanged", func(t *testing.T) {
		if resp.Domain.Name != "update-test.com" {
			t.Errorf("expected name %q unchanged, got %q", "update-test.com", resp.Domain.Name)
		}
		if resp.Domain.WebScheme != "https" {
			t.Errorf("expected web_scheme %q unchanged, got %q", "https", resp.Domain.WebScheme)
		}
		if resp.Domain.WebPrefix != "email" {
			t.Errorf("expected web_prefix %q unchanged, got %q", "email", resp.Domain.WebPrefix)
		}
		if resp.Domain.MessageTTL != 259200 {
			t.Errorf("expected message_ttl=259200 unchanged, got %d", resp.Domain.MessageTTL)
		}
	})

	t.Run("includes DNS records in response", func(t *testing.T) {
		if resp.ReceivingDNSRecords == nil {
			t.Error("expected receiving_dns_records to be present")
		}
		if resp.SendingDNSRecords == nil {
			t.Error("expected sending_dns_records to be present")
		}
	})
}

func TestUpdateDomain_MultipleFields(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	createDomainViaMultipart(t, router, map[string]string{"name": "multi-update.com"})

	req := newMultipartRequest(t, http.MethodPut, "/v4/domains/multi-update.com", map[string]string{
		"spam_action":  "block",
		"wildcard":     "true",
		"web_scheme":   "http",
		"web_prefix":   "track",
		"require_tls":  "true",
		"message_ttl":  "7200",
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp createDomainResponse
	decodeJSON(t, rec, &resp)

	t.Run("spam_action updated", func(t *testing.T) {
		if resp.Domain.SpamAction != "block" {
			t.Errorf("expected spam_action %q, got %q", "block", resp.Domain.SpamAction)
		}
	})

	t.Run("wildcard updated", func(t *testing.T) {
		if resp.Domain.Wildcard != true {
			t.Errorf("expected wildcard=true, got %v", resp.Domain.Wildcard)
		}
	})

	t.Run("web_scheme updated", func(t *testing.T) {
		if resp.Domain.WebScheme != "http" {
			t.Errorf("expected web_scheme %q, got %q", "http", resp.Domain.WebScheme)
		}
	})

	t.Run("web_prefix updated", func(t *testing.T) {
		if resp.Domain.WebPrefix != "track" {
			t.Errorf("expected web_prefix %q, got %q", "track", resp.Domain.WebPrefix)
		}
	})

	t.Run("require_tls updated", func(t *testing.T) {
		if resp.Domain.RequireTLS != true {
			t.Errorf("expected require_tls=true, got %v", resp.Domain.RequireTLS)
		}
	})

	t.Run("message_ttl updated", func(t *testing.T) {
		if resp.Domain.MessageTTL != 7200 {
			t.Errorf("expected message_ttl=7200, got %d", resp.Domain.MessageTTL)
		}
	})
}

func TestUpdateDomain_NotFound(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	req := newMultipartRequest(t, http.MethodPut, "/v4/domains/nonexistent.com", map[string]string{
		"spam_action": "tag",
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	t.Run("returns 404 status", func(t *testing.T) {
		if rec.Code != http.StatusNotFound {
			t.Errorf("expected status 404, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})
}

func TestUpdateDomain_ImmutableFieldsUnchanged(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	// Create a domain and capture its original values.
	createRec := createDomainViaMultipart(t, router, map[string]string{"name": "immutable-test.com"})
	var createResp createDomainResponse
	decodeJSON(t, createRec, &createResp)

	originalID := createResp.Domain.ID
	originalName := createResp.Domain.Name
	originalType := createResp.Domain.Type
	originalState := createResp.Domain.State

	// Attempt to update with a mutable field, and also supply immutable fields
	// (which should be ignored by the implementation).
	req := newMultipartRequest(t, http.MethodPut, "/v4/domains/immutable-test.com", map[string]string{
		"spam_action": "block",
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp createDomainResponse
	decodeJSON(t, rec, &resp)

	t.Run("name is unchanged", func(t *testing.T) {
		if resp.Domain.Name != originalName {
			t.Errorf("expected name %q unchanged, got %q", originalName, resp.Domain.Name)
		}
	})

	t.Run("ID is unchanged", func(t *testing.T) {
		if resp.Domain.ID != originalID {
			t.Errorf("expected ID %q unchanged, got %q", originalID, resp.Domain.ID)
		}
	})

	t.Run("type is unchanged", func(t *testing.T) {
		if resp.Domain.Type != originalType {
			t.Errorf("expected type %q unchanged, got %q", originalType, resp.Domain.Type)
		}
	})

	t.Run("state is unchanged", func(t *testing.T) {
		if resp.Domain.State != originalState {
			t.Errorf("expected state %q unchanged, got %q", originalState, resp.Domain.State)
		}
	})
}

func TestUpdateDomain_ViaJSON(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	createDomainViaMultipart(t, router, map[string]string{"name": "json-update.com"})

	req := newJSONRequest(t, http.MethodPut, "/v4/domains/json-update.com", map[string]interface{}{
		"spam_action": "tag",
		"message_ttl": 1800,
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	t.Run("returns 200 status", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp createDomainResponse
	decodeJSON(t, rec, &resp)

	t.Run("spam_action updated via JSON", func(t *testing.T) {
		if resp.Domain.SpamAction != "tag" {
			t.Errorf("expected spam_action %q, got %q", "tag", resp.Domain.SpamAction)
		}
	})

	t.Run("message_ttl updated via JSON", func(t *testing.T) {
		if resp.Domain.MessageTTL != 1800 {
			t.Errorf("expected message_ttl=1800, got %d", resp.Domain.MessageTTL)
		}
	})
}

// ---------------------------------------------------------------------------
// DELETE /v3/domains/{name} — Delete Domain
// ---------------------------------------------------------------------------

func TestDeleteDomain_Exists(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	createDomainViaMultipart(t, router, map[string]string{"name": "delete-me.com"})

	req := httptest.NewRequest(http.MethodDelete, "/v3/domains/delete-me.com", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	t.Run("returns 200 status", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("returns deletion message", func(t *testing.T) {
		var resp deleteResponse
		decodeJSON(t, rec, &resp)
		if resp.Message != "Domain has been deleted" {
			t.Errorf("expected message %q, got %q", "Domain has been deleted", resp.Message)
		}
	})
}

func TestDeleteDomain_NotFound(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	req := httptest.NewRequest(http.MethodDelete, "/v3/domains/nonexistent.com", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	t.Run("returns 404 status", func(t *testing.T) {
		if rec.Code != http.StatusNotFound {
			t.Errorf("expected status 404, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})
}

func TestDeleteDomain_NotInListAfterDeletion(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	createDomainViaMultipart(t, router, map[string]string{"name": "to-delete.com"})
	createDomainViaMultipart(t, router, map[string]string{"name": "to-keep.com"})

	// Delete one domain.
	delReq := httptest.NewRequest(http.MethodDelete, "/v3/domains/to-delete.com", nil)
	delRec := httptest.NewRecorder()
	router.ServeHTTP(delRec, delReq)
	if delRec.Code != http.StatusOK {
		t.Fatalf("delete failed with status %d", delRec.Code)
	}

	// List domains and verify the deleted one is gone.
	listReq := httptest.NewRequest(http.MethodGet, "/v4/domains", nil)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)

	var resp listDomainsResponse
	decodeJSON(t, listRec, &resp)

	t.Run("total_count is 1 after deletion", func(t *testing.T) {
		if resp.TotalCount != 1 {
			t.Errorf("expected total_count=1 after deletion, got %d", resp.TotalCount)
		}
	})

	t.Run("deleted domain is not in list", func(t *testing.T) {
		for _, item := range resp.Items {
			if item.Name == "to-delete.com" {
				t.Error("expected deleted domain to not appear in list")
			}
		}
	})

	t.Run("remaining domain is still in list", func(t *testing.T) {
		found := false
		for _, item := range resp.Items {
			if item.Name == "to-keep.com" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected remaining domain to still appear in list")
		}
	})
}

func TestDeleteDomain_GetReturns404AfterDeletion(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	createDomainViaMultipart(t, router, map[string]string{"name": "delete-then-get.com"})

	// Delete the domain.
	delReq := httptest.NewRequest(http.MethodDelete, "/v3/domains/delete-then-get.com", nil)
	delRec := httptest.NewRecorder()
	router.ServeHTTP(delRec, delReq)
	if delRec.Code != http.StatusOK {
		t.Fatalf("delete failed with status %d", delRec.Code)
	}

	// Attempt to get the deleted domain.
	getReq := httptest.NewRequest(http.MethodGet, "/v4/domains/delete-then-get.com", nil)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)

	t.Run("get returns 404 after deletion", func(t *testing.T) {
		if getRec.Code != http.StatusNotFound {
			t.Errorf("expected status 404 after deletion, got %d (body: %s)", getRec.Code, getRec.Body.String())
		}
	})
}

// ---------------------------------------------------------------------------
// PUT /v4/domains/{name}/verify — Verify Domain DNS
// ---------------------------------------------------------------------------

func TestVerifyDomain_AutoVerifyMode(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig() // auto-verify enabled
	router := setupRouter(db, cfg)

	createDomainViaMultipart(t, router, map[string]string{"name": "verify-auto.com"})

	req := httptest.NewRequest(http.MethodPut, "/v4/domains/verify-auto.com/verify", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	t.Run("returns 200 status", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp createDomainResponse
	decodeJSON(t, rec, &resp)

	t.Run("returns DNS update message", func(t *testing.T) {
		if resp.Message != "Domain DNS records have been updated" {
			t.Errorf("expected message %q, got %q", "Domain DNS records have been updated", resp.Message)
		}
	})

	t.Run("domain state is active", func(t *testing.T) {
		if resp.Domain.State != "active" {
			t.Errorf("expected state %q, got %q", "active", resp.Domain.State)
		}
	})

	t.Run("DNS records are valid", func(t *testing.T) {
		allRecords := append(resp.SendingDNSRecords, resp.ReceivingDNSRecords...)
		for _, rec := range allRecords {
			if rec.Valid != "valid" {
				t.Errorf("expected DNS record valid=%q, got %q (record: %s %s)",
					"valid", rec.Valid, rec.RecordType, rec.Name)
			}
		}
	})

	t.Run("includes receiving DNS records", func(t *testing.T) {
		if resp.ReceivingDNSRecords == nil {
			t.Error("expected receiving_dns_records to be present")
		}
	})

	t.Run("includes sending DNS records", func(t *testing.T) {
		if resp.SendingDNSRecords == nil {
			t.Error("expected sending_dns_records to be present")
		}
	})
}

func TestVerifyDomain_ManualMode_TransitionsToActive(t *testing.T) {
	db := setupTestDB(t)
	cfg := manualVerifyConfig() // auto-verify disabled
	router := setupRouter(db, cfg)

	// Create domain (will be unverified in manual mode).
	createRec := createDomainViaMultipart(t, router, map[string]string{"name": "verify-manual.com"})
	var createResp createDomainResponse
	decodeJSON(t, createRec, &createResp)

	// Confirm the domain starts as unverified.
	if createResp.Domain.State != "unverified" {
		t.Fatalf("precondition failed: expected initial state %q, got %q", "unverified", createResp.Domain.State)
	}

	// Confirm DNS records start as unknown.
	allCreateRecords := append(createResp.SendingDNSRecords, createResp.ReceivingDNSRecords...)
	for _, rec := range allCreateRecords {
		if rec.Valid != "unknown" {
			t.Fatalf("precondition failed: expected initial DNS valid=%q, got %q", "unknown", rec.Valid)
		}
	}

	// Verify the domain.
	req := httptest.NewRequest(http.MethodPut, "/v4/domains/verify-manual.com/verify", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	t.Run("returns 200 status", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp createDomainResponse
	decodeJSON(t, rec, &resp)

	t.Run("domain transitions from unverified to active", func(t *testing.T) {
		if resp.Domain.State != "active" {
			t.Errorf("expected state %q after verify, got %q", "active", resp.Domain.State)
		}
	})

	t.Run("DNS records become valid after verify", func(t *testing.T) {
		allRecords := append(resp.SendingDNSRecords, resp.ReceivingDNSRecords...)
		for _, rec := range allRecords {
			if rec.Valid != "valid" {
				t.Errorf("expected DNS record valid=%q after verify, got %q (record: %s %s)",
					"valid", rec.Valid, rec.RecordType, rec.Name)
			}
		}
	})

	t.Run("returns DNS update message", func(t *testing.T) {
		if resp.Message != "Domain DNS records have been updated" {
			t.Errorf("expected message %q, got %q", "Domain DNS records have been updated", resp.Message)
		}
	})
}

func TestVerifyDomain_NotFound(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	req := httptest.NewRequest(http.MethodPut, "/v4/domains/nonexistent.com/verify", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	t.Run("returns 404 status", func(t *testing.T) {
		if rec.Code != http.StatusNotFound {
			t.Errorf("expected status 404, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})
}

// ---------------------------------------------------------------------------
// Verify correct endpoint paths (v3 vs v4)
// ---------------------------------------------------------------------------

func TestEndpointPaths_DeleteIsV3(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	createDomainViaMultipart(t, router, map[string]string{"name": "v3-delete.com"})

	t.Run("DELETE /v3/domains/{name} succeeds", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/v3/domains/v3-delete.com", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200 for v3 delete, got %d", rec.Code)
		}
	})

	t.Run("DELETE /v4/domains/{name} returns 405 or 404", func(t *testing.T) {
		// v4 delete should not exist; the router should return 405 Method Not
		// Allowed or 404 Not Found.
		createDomainViaMultipart(t, router, map[string]string{"name": "v4-delete-fail.com"})
		req := httptest.NewRequest(http.MethodDelete, "/v4/domains/v4-delete-fail.com", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code == http.StatusOK {
			t.Error("DELETE /v4/domains/{name} should not succeed (should be v3 only)")
		}
	})
}

// ---------------------------------------------------------------------------
// GET after update verifies persistence
// ---------------------------------------------------------------------------

func TestUpdateDomain_PersistsOnSubsequentGet(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	createDomainViaMultipart(t, router, map[string]string{"name": "persist-test.com"})

	// Update the domain.
	updateReq := newMultipartRequest(t, http.MethodPut, "/v4/domains/persist-test.com", map[string]string{
		"spam_action": "block",
		"web_prefix":  "custom",
	})
	updateRec := httptest.NewRecorder()
	router.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("update failed: status %d", updateRec.Code)
	}

	// Get the domain and verify the update persisted.
	getReq := httptest.NewRequest(http.MethodGet, "/v4/domains/persist-test.com", nil)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)

	var resp createDomainResponse
	decodeJSON(t, getRec, &resp)

	t.Run("spam_action persists", func(t *testing.T) {
		if resp.Domain.SpamAction != "block" {
			t.Errorf("expected spam_action %q after update+get, got %q", "block", resp.Domain.SpamAction)
		}
	})

	t.Run("web_prefix persists", func(t *testing.T) {
		if resp.Domain.WebPrefix != "custom" {
			t.Errorf("expected web_prefix %q after update+get, got %q", "custom", resp.Domain.WebPrefix)
		}
	})
}

// ---------------------------------------------------------------------------
// Verify domain after manual verify persists state in GET
// ---------------------------------------------------------------------------

func TestVerifyDomain_PersistsActiveStateOnGet(t *testing.T) {
	db := setupTestDB(t)
	cfg := manualVerifyConfig()
	router := setupRouter(db, cfg)

	createDomainViaMultipart(t, router, map[string]string{"name": "verify-persist.com"})

	// Verify the domain.
	verifyReq := httptest.NewRequest(http.MethodPut, "/v4/domains/verify-persist.com/verify", nil)
	verifyRec := httptest.NewRecorder()
	router.ServeHTTP(verifyRec, verifyReq)
	if verifyRec.Code != http.StatusOK {
		t.Fatalf("verify failed: status %d", verifyRec.Code)
	}

	// Get the domain and confirm it is active.
	getReq := httptest.NewRequest(http.MethodGet, "/v4/domains/verify-persist.com", nil)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)

	var resp createDomainResponse
	decodeJSON(t, getRec, &resp)

	t.Run("state is active after verify + get", func(t *testing.T) {
		if resp.Domain.State != "active" {
			t.Errorf("expected state %q after verify+get, got %q", "active", resp.Domain.State)
		}
	})

	t.Run("DNS records are valid after verify + get", func(t *testing.T) {
		allRecords := append(resp.SendingDNSRecords, resp.ReceivingDNSRecords...)
		for _, rec := range allRecords {
			if rec.Valid != "valid" {
				t.Errorf("expected DNS record valid=%q after verify+get, got %q", "valid", rec.Valid)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// created_at RFC-1123 format
// ---------------------------------------------------------------------------

func TestCreateDomain_CreatedAtIsRFC1123(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	rec := createDomainViaMultipart(t, router, map[string]string{
		"name": "rfc1123-test.com",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp createDomainResponse
	decodeJSON(t, rec, &resp)

	t.Run("created_at is non-empty", func(t *testing.T) {
		if resp.Domain.CreatedAt == "" {
			t.Fatal("expected non-empty created_at")
		}
	})

	t.Run("created_at is valid RFC-1123 format", func(t *testing.T) {
		// RFC-1123 format example: "Mon, 02 Jan 2006 15:04:05 MST"
		_, err := time.Parse(time.RFC1123, resp.Domain.CreatedAt)
		if err != nil {
			t.Errorf("expected created_at in RFC-1123 format, got %q (parse error: %v)",
				resp.Domain.CreatedAt, err)
		}
	})

	t.Run("created_at ends with UTC timezone", func(t *testing.T) {
		if !strings.HasSuffix(resp.Domain.CreatedAt, "UTC") {
			t.Errorf("expected created_at to end with UTC, got %q", resp.Domain.CreatedAt)
		}
	})
}

// ---------------------------------------------------------------------------
// smtp_password visibility rules
// ---------------------------------------------------------------------------

func TestCreateDomain_SMTPPasswordInCreateResponse(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	rec := createDomainViaMultipart(t, router, map[string]string{
		"name":          "password-test.com",
		"smtp_password": "my-secret-password",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	// Decode into raw JSON to check for smtp_password presence.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	var domainRaw map[string]json.RawMessage
	if err := json.Unmarshal(raw["domain"], &domainRaw); err != nil {
		t.Fatalf("failed to decode domain object: %v", err)
	}

	t.Run("smtp_password is present in create response", func(t *testing.T) {
		pwRaw, ok := domainRaw["smtp_password"]
		if !ok {
			t.Fatal("expected smtp_password key in create response domain object")
		}
		var pw string
		if err := json.Unmarshal(pwRaw, &pw); err != nil {
			t.Fatalf("failed to decode smtp_password: %v", err)
		}
		if pw != "my-secret-password" {
			t.Errorf("expected smtp_password %q, got %q", "my-secret-password", pw)
		}
	})
}

func TestGetDomain_SMTPPasswordOmitted(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	// Create a domain with an explicit smtp_password.
	createRec := createDomainViaMultipart(t, router, map[string]string{
		"name":          "get-no-password.com",
		"smtp_password": "super-secret",
	})
	if createRec.Code != http.StatusOK {
		t.Fatalf("create failed: status %d", createRec.Code)
	}

	// GET the domain.
	req := httptest.NewRequest(http.MethodGet, "/v4/domains/get-no-password.com", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	// Decode into raw JSON to check that smtp_password is absent.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	var domainRaw map[string]json.RawMessage
	if err := json.Unmarshal(raw["domain"], &domainRaw); err != nil {
		t.Fatalf("failed to decode domain object: %v", err)
	}

	t.Run("smtp_password is NOT present in get response", func(t *testing.T) {
		pwRaw, ok := domainRaw["smtp_password"]
		if ok {
			// If the key is present, it should be null or empty string at most.
			var pw interface{}
			json.Unmarshal(pwRaw, &pw)
			if pw != nil && pw != "" {
				t.Errorf("expected smtp_password to be absent or empty in get response, got %v", pw)
			}
		}
		// If the key is absent entirely, that's the expected behavior.
	})
}

func TestListDomains_SMTPPasswordOmitted(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	// Create a domain with an explicit smtp_password.
	createRec := createDomainViaMultipart(t, router, map[string]string{
		"name":          "list-no-password.com",
		"smtp_password": "super-secret",
	})
	if createRec.Code != http.StatusOK {
		t.Fatalf("create failed: status %d", createRec.Code)
	}

	// List all domains.
	req := httptest.NewRequest(http.MethodGet, "/v4/domains", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	// Decode into raw JSON to inspect the items array.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	var items []map[string]json.RawMessage
	if err := json.Unmarshal(raw["items"], &items); err != nil {
		t.Fatalf("failed to decode items: %v", err)
	}

	t.Run("smtp_password is NOT present in list items", func(t *testing.T) {
		for i, item := range items {
			pwRaw, ok := item["smtp_password"]
			if ok {
				var pw interface{}
				json.Unmarshal(pwRaw, &pw)
				if pw != nil && pw != "" {
					t.Errorf("item %d: expected smtp_password to be absent or empty in list response, got %v", i, pw)
				}
			}
		}
	})
}

// ---------------------------------------------------------------------------
// List domains does NOT include DNS records
// ---------------------------------------------------------------------------

func TestListDomains_NoDNSRecordsInItems(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	createDomainViaMultipart(t, router, map[string]string{"name": "no-dns-list.com"})

	req := httptest.NewRequest(http.MethodGet, "/v4/domains", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	// The top-level response should NOT contain receiving_dns_records or
	// sending_dns_records. Only "total_count" and "items" should be present.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	t.Run("response does not contain receiving_dns_records", func(t *testing.T) {
		if _, ok := raw["receiving_dns_records"]; ok {
			t.Error("expected list response to NOT contain receiving_dns_records")
		}
	})

	t.Run("response does not contain sending_dns_records", func(t *testing.T) {
		if _, ok := raw["sending_dns_records"]; ok {
			t.Error("expected list response to NOT contain sending_dns_records")
		}
	})
}

// ---------------------------------------------------------------------------
// DNS record details validation
// ---------------------------------------------------------------------------

func TestCreateDomain_DNSRecordNames(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	rec := createDomainViaMultipart(t, router, map[string]string{
		"name": "mg.example.com",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp createDomainResponse
	decodeJSON(t, rec, &resp)

	t.Run("MX records point to domain name", func(t *testing.T) {
		for _, rec := range resp.ReceivingDNSRecords {
			if rec.RecordType == "MX" && rec.Name != "mg.example.com" {
				t.Errorf("expected MX record name %q, got %q", "mg.example.com", rec.Name)
			}
		}
	})

	t.Run("SPF TXT record uses domain name", func(t *testing.T) {
		found := false
		for _, rec := range resp.SendingDNSRecords {
			if rec.RecordType == "TXT" && strings.Contains(rec.Value, "v=spf1") {
				found = true
				if rec.Name != "mg.example.com" {
					t.Errorf("expected SPF record name %q, got %q", "mg.example.com", rec.Name)
				}
			}
		}
		if !found {
			t.Error("SPF TXT record not found")
		}
	})

	t.Run("DKIM TXT record name contains domainkey and domain", func(t *testing.T) {
		found := false
		for _, rec := range resp.SendingDNSRecords {
			if rec.RecordType == "TXT" && strings.Contains(rec.Name, "domainkey") {
				found = true
				if !strings.Contains(rec.Name, "mg.example.com") {
					t.Errorf("expected DKIM record name to contain domain, got %q", rec.Name)
				}
			}
		}
		if !found {
			t.Error("DKIM TXT record not found")
		}
	})

	t.Run("tracking CNAME uses web_prefix and domain", func(t *testing.T) {
		found := false
		for _, rec := range resp.SendingDNSRecords {
			if rec.RecordType == "CNAME" {
				found = true
				expected := "email.mg.example.com"
				if rec.Name != expected {
					t.Errorf("expected tracking CNAME name %q, got %q", expected, rec.Name)
				}
			}
		}
		if !found {
			t.Error("tracking CNAME record not found")
		}
	})

	t.Run("MX records have priority set", func(t *testing.T) {
		for _, rec := range resp.ReceivingDNSRecords {
			if rec.RecordType == "MX" {
				if rec.Priority == nil || rec.Priority == "" {
					t.Errorf("expected MX record to have a priority, got %v", rec.Priority)
				}
			}
		}
	})
}

func TestCreateDomain_CustomWebPrefix_TrackingCNAME(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	rec := createDomainViaMultipart(t, router, map[string]string{
		"name":       "cname-test.com",
		"web_prefix": "track",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp createDomainResponse
	decodeJSON(t, rec, &resp)

	t.Run("tracking CNAME uses custom web_prefix", func(t *testing.T) {
		found := false
		for _, rec := range resp.SendingDNSRecords {
			if rec.RecordType == "CNAME" {
				found = true
				expected := "track.cname-test.com"
				if rec.Name != expected {
					t.Errorf("expected tracking CNAME name %q with custom prefix, got %q", expected, rec.Name)
				}
			}
		}
		if !found {
			t.Error("tracking CNAME record not found")
		}
	})
}

// ---------------------------------------------------------------------------
// Domain ID is a UUID
// ---------------------------------------------------------------------------

func TestCreateDomain_IDIsUUID(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	rec := createDomainViaMultipart(t, router, map[string]string{
		"name": "uuid-test.com",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp createDomainResponse
	decodeJSON(t, rec, &resp)

	t.Run("domain ID is a 36-char UUID", func(t *testing.T) {
		if len(resp.Domain.ID) != 36 {
			t.Errorf("expected UUID length 36, got %d (%q)", len(resp.Domain.ID), resp.Domain.ID)
		}
	})

	t.Run("domain ID matches UUID format", func(t *testing.T) {
		// UUID v4 format: 8-4-4-4-12 hex chars separated by hyphens.
		parts := strings.Split(resp.Domain.ID, "-")
		if len(parts) != 5 {
			t.Errorf("expected 5 UUID parts separated by hyphens, got %d (%q)", len(parts), resp.Domain.ID)
		}
		if len(parts) == 5 {
			expectedLens := []int{8, 4, 4, 4, 12}
			for i, part := range parts {
				if len(part) != expectedLens[i] {
					t.Errorf("UUID part %d: expected length %d, got %d (%q)", i, expectedLens[i], len(part), part)
				}
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Pagination edge cases
// ---------------------------------------------------------------------------

func TestListDomains_SkipOneLimitOne(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	// Create exactly 3 domains.
	createDomainViaMultipart(t, router, map[string]string{"name": "first.com"})
	createDomainViaMultipart(t, router, map[string]string{"name": "second.com"})
	createDomainViaMultipart(t, router, map[string]string{"name": "third.com"})

	req := httptest.NewRequest(http.MethodGet, "/v4/domains?skip=1&limit=1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	var resp listDomainsResponse
	decodeJSON(t, rec, &resp)

	t.Run("returns 200 status", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}
	})

	t.Run("total_count reflects all domains", func(t *testing.T) {
		if resp.TotalCount != 3 {
			t.Errorf("expected total_count=3, got %d", resp.TotalCount)
		}
	})

	t.Run("returns exactly one item", func(t *testing.T) {
		if len(resp.Items) != 1 {
			t.Errorf("expected 1 item with skip=1&limit=1, got %d", len(resp.Items))
		}
	})
}

// ---------------------------------------------------------------------------
// Verify DNS record is_active field
// ---------------------------------------------------------------------------

func TestVerifyDomain_DNSRecordsIsActiveAfterVerify(t *testing.T) {
	db := setupTestDB(t)
	cfg := manualVerifyConfig()
	router := setupRouter(db, cfg)

	// Create domain (starts unverified).
	createRec := createDomainViaMultipart(t, router, map[string]string{"name": "isactive-test.com"})
	if createRec.Code != http.StatusOK {
		t.Fatalf("create failed: status %d", createRec.Code)
	}

	// Check that DNS records start with is_active=false.
	var createResp createDomainResponse
	decodeJSON(t, createRec, &createResp)

	t.Run("DNS records start with is_active=false in manual mode", func(t *testing.T) {
		allRecords := append(createResp.SendingDNSRecords, createResp.ReceivingDNSRecords...)
		for _, rec := range allRecords {
			if rec.IsActive != false {
				t.Errorf("expected is_active=false before verify, got %v (record: %s %s)",
					rec.IsActive, rec.RecordType, rec.Name)
			}
		}
	})

	// Verify the domain.
	verifyReq := httptest.NewRequest(http.MethodPut, "/v4/domains/isactive-test.com/verify", nil)
	verifyRec := httptest.NewRecorder()
	router.ServeHTTP(verifyRec, verifyReq)

	if verifyRec.Code != http.StatusOK {
		t.Fatalf("verify failed: status %d", verifyRec.Code)
	}

	var verifyResp createDomainResponse
	decodeJSON(t, verifyRec, &verifyResp)

	t.Run("DNS records have is_active=true after verify", func(t *testing.T) {
		allRecords := append(verifyResp.SendingDNSRecords, verifyResp.ReceivingDNSRecords...)
		for _, rec := range allRecords {
			if rec.IsActive != true {
				t.Errorf("expected is_active=true after verify, got %v (record: %s %s)",
					rec.IsActive, rec.RecordType, rec.Name)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Create domain auto-generates smtp_password when not provided
// ---------------------------------------------------------------------------

func TestCreateDomain_AutoGeneratesSMTPPassword(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	rec := createDomainViaMultipart(t, router, map[string]string{
		"name": "auto-pwd.com",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp createDomainResponse
	decodeJSON(t, rec, &resp)

	t.Run("smtp_password is present in create response even without explicit password", func(t *testing.T) {
		// When no smtp_password is provided, the server should auto-generate one.
		// The SMTPPassword field in domainJSON is interface{} so it could be string or nil.
		if resp.Domain.SMTPPassword == nil || resp.Domain.SMTPPassword == "" {
			t.Error("expected smtp_password to be auto-generated in create response")
		}
	})
}

// ---------------------------------------------------------------------------
// Search combined with pagination
// ---------------------------------------------------------------------------

func TestListDomains_SearchWithPagination(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	// Create domains with specific patterns.
	createDomainViaMultipart(t, router, map[string]string{"name": "app-one.example.com"})
	createDomainViaMultipart(t, router, map[string]string{"name": "app-two.example.com"})
	createDomainViaMultipart(t, router, map[string]string{"name": "app-three.example.com"})
	createDomainViaMultipart(t, router, map[string]string{"name": "other.org"})

	t.Run("search with limit returns correct count and items", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v4/domains?search=app&limit=2", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		var resp listDomainsResponse
		decodeJSON(t, rec, &resp)

		// total_count should be the total matching the search (3), not the page size.
		if resp.TotalCount != 3 {
			t.Errorf("expected total_count=3 for search=app, got %d", resp.TotalCount)
		}

		if len(resp.Items) != 2 {
			t.Errorf("expected 2 items with limit=2, got %d", len(resp.Items))
		}
	})

	t.Run("search with skip returns remaining items", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v4/domains?search=app&skip=2", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		var resp listDomainsResponse
		decodeJSON(t, rec, &resp)

		if resp.TotalCount != 3 {
			t.Errorf("expected total_count=3 for search=app, got %d", resp.TotalCount)
		}

		if len(resp.Items) != 1 {
			t.Errorf("expected 1 item with skip=2 of 3 matches, got %d", len(resp.Items))
		}
	})
}

// ---------------------------------------------------------------------------
// State filter combined with pagination
// ---------------------------------------------------------------------------

func TestListDomains_StateFilterWithPagination(t *testing.T) {
	db := setupTestDB(t)
	cfg := manualVerifyConfig() // manual mode: domains start as "unverified"
	router := setupRouter(db, cfg)

	// Create 3 domains (all start unverified).
	createDomainViaMultipart(t, router, map[string]string{"name": "state-page-1.com"})
	createDomainViaMultipart(t, router, map[string]string{"name": "state-page-2.com"})
	createDomainViaMultipart(t, router, map[string]string{"name": "state-page-3.com"})

	t.Run("state filter with limit restricts returned items", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v4/domains?state=unverified&limit=2", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		var resp listDomainsResponse
		decodeJSON(t, rec, &resp)

		if resp.TotalCount != 3 {
			t.Errorf("expected total_count=3 for state=unverified, got %d", resp.TotalCount)
		}

		if len(resp.Items) != 2 {
			t.Errorf("expected 2 items with limit=2, got %d", len(resp.Items))
		}
	})
}
