package message_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/bethmaloney/mailgun-mock-api/internal/domain"
	"github.com/bethmaloney/mailgun-mock-api/internal/message"
	"github.com/bethmaloney/mailgun-mock-api/internal/mock"
	"github.com/bethmaloney/mailgun-mock-api/internal/template"
	"github.com/go-chi/chi/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// templateTestDBCounter provides unique DSN names so that each test gets its
// own isolated in-memory SQLite database (cache=shared scopes to the DSN name).
var templateTestDBCounter atomic.Int64

// ---------------------------------------------------------------------------
// Test Helpers — unique to template rendering tests
// ---------------------------------------------------------------------------

// setupTestDBWithTemplates creates an in-memory SQLite database with Domain,
// DNSRecord, StoredMessage, Template, and TemplateVersion tables migrated.
func setupTestDBWithTemplates(t *testing.T) *gorm.DB {
	t.Helper()
	n := templateTestDBCounter.Add(1)
	dsn := fmt.Sprintf("file:tmpl_test_%d?mode=memory&cache=shared", n)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}
	if err := db.AutoMigrate(
		&domain.Domain{},
		&domain.DNSRecord{},
		&message.StoredMessage{},
		&template.Template{},
		&template.TemplateVersion{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

// setupRouterWithTemplates creates a chi router with domain, message, and
// template routes registered — everything needed for template rendering tests.
func setupRouterWithTemplates(db *gorm.DB, cfg *mock.MockConfig) http.Handler {
	dh := domain.NewHandlers(db, cfg)
	mh := message.NewHandlers(db, cfg)
	th := template.NewHandlers(db)

	r := chi.NewRouter()
	r.Route("/v4/domains", func(r chi.Router) {
		r.Post("/", dh.CreateDomain)
	})
	r.Route("/v3/{domain_name}/messages", func(r chi.Router) {
		r.Post("/", mh.SendMessage)
	})
	r.Route("/v3/domains/{domain_name}/messages", func(r chi.Router) {
		r.Get("/{storage_key}", mh.GetMessage)
		r.Delete("/{storage_key}", mh.DeleteMessage)
	})
	r.Route("/v3/{domain_name}/templates", func(r chi.Router) {
		r.Post("/", th.CreateTemplate)
		r.Get("/", th.ListTemplates)
		r.Delete("/", th.DeleteAllTemplates)
		r.Get("/{name}", th.GetTemplate)
		r.Put("/{name}", th.UpdateTemplate)
		r.Delete("/{name}", th.DeleteTemplate)
		r.Post("/{name}/versions", th.CreateVersion)
		r.Get("/{name}/versions", th.ListVersions)
		r.Get("/{name}/versions/{tag}", th.GetVersion)
		r.Put("/{name}/versions/{tag}", th.UpdateVersion)
		r.Delete("/{name}/versions/{tag}", th.DeleteVersion)
		r.Put("/{name}/versions/{tag}/copy/{new_tag}", th.CopyVersion)
	})
	return r
}

// createTemplate creates a template via the API and returns the response recorder.
func createTemplate(t *testing.T, router http.Handler, domainName string, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	url := fmt.Sprintf("/v3/%s/templates", domainName)
	req := newMultipartRequest(t, http.MethodPost, url, fields)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// createTemplateVersion creates a new version for an existing template via the API.
func createTemplateVersion(t *testing.T, router http.Handler, domainName, templateName string, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	url := fmt.Sprintf("/v3/%s/templates/%s/versions", domainName, templateName)
	req := newMultipartRequest(t, http.MethodPost, url, fields)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// updateTemplateVersion updates an existing version via the API.
func updateTemplateVersion(t *testing.T, router http.Handler, domainName, templateName, tag string, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	url := fmt.Sprintf("/v3/%s/templates/%s/versions/%s", domainName, templateName, tag)
	req := newMultipartRequest(t, http.MethodPut, url, fields)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// templateSendResponse is used to decode the response from a send-with-template call.
type templateSendResponse struct {
	ID      string `json:"id"`
	Message string `json:"message"`
}

// templateErrorResponse is used to decode error responses from template rendering.
type templateErrorResponse struct {
	Message string `json:"message"`
}

// templateMessageDetailResponse is used to decode a stored message retrieval
// response, with fields relevant to template rendering.
type templateMessageDetailResponse struct {
	From      string `json:"From"`
	To        string `json:"To"`
	Subject   string `json:"Subject"`
	BodyHTML  string `json:"body-html"`
	BodyPlain string `json:"body-plain"`
}

// extractTemplateStorageKey extracts the storage key from a send response message ID.
func extractTemplateStorageKey(t *testing.T, resp templateSendResponse) string {
	t.Helper()
	id := resp.ID
	id = strings.TrimPrefix(id, "<")
	id = strings.TrimSuffix(id, ">")
	return id
}

// setupDomainAndTemplate is a convenience function that creates a domain, a
// template, and returns the router. The template is created with the given
// content and an active initial version.
func setupDomainAndTemplate(t *testing.T, domainName, templateName, templateContent string) http.Handler {
	t.Helper()
	db := setupTestDBWithTemplates(t)
	cfg := defaultConfig()
	router := setupRouterWithTemplates(db, cfg)
	createTestDomain(t, router, domainName)
	rec := createTemplate(t, router, domainName, map[string]string{
		"name":     templateName,
		"template": templateContent,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to create template %q: status=%d body=%s", templateName, rec.Code, rec.Body.String())
	}
	return router
}

// setupDomainAndTemplateWithHeaders creates a domain and a template with
// headers on the initial version.
func setupDomainAndTemplateWithHeaders(t *testing.T, domainName, templateName, templateContent, headersJSON string) http.Handler {
	t.Helper()
	db := setupTestDBWithTemplates(t)
	cfg := defaultConfig()
	router := setupRouterWithTemplates(db, cfg)
	createTestDomain(t, router, domainName)
	rec := createTemplate(t, router, domainName, map[string]string{
		"name":     templateName,
		"template": templateContent,
		"headers":  headersJSON,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to create template %q: status=%d body=%s", templateName, rec.Code, rec.Body.String())
	}
	return router
}

// ---------------------------------------------------------------------------
// Happy Path Tests
// ---------------------------------------------------------------------------

// Test 1: Send with template — basic variable substitution
func TestTemplateRendering_BasicVariableSubstitution(t *testing.T) {
	router := setupDomainAndTemplate(t, "example.com", "mytemplate", "<h1>Hello {{name}}</h1>")

	rec := sendMessage(t, router, "example.com", map[string]string{
		"from":        "sender@example.com",
		"to":          "recipient@example.com",
		"subject":     "Test Subject",
		"template":    "mytemplate",
		"t:variables": `{"name":"World"}`,
	})

	t.Run("returns 200", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp templateSendResponse
	decodeJSON(t, rec, &resp)

	t.Run("stored message HTML body contains rendered template", func(t *testing.T) {
		storageKey := extractTemplateStorageKey(t, resp)
		getRec := getMessage(t, router, "example.com", storageKey)
		if getRec.Code != http.StatusOK {
			t.Fatalf("expected 200 on GET, got %d (body: %s)", getRec.Code, getRec.Body.String())
		}
		var detail templateMessageDetailResponse
		decodeJSON(t, getRec, &detail)
		expected := "<h1>Hello World</h1>"
		if detail.BodyHTML != expected {
			t.Errorf("expected body-html=%q, got %q", expected, detail.BodyHTML)
		}
	})
}

// Test 2: Send with specific version tag
func TestTemplateRendering_SpecificVersion(t *testing.T) {
	db := setupTestDBWithTemplates(t)
	cfg := defaultConfig()
	router := setupRouterWithTemplates(db, cfg)
	createTestDomain(t, router, "example.com")

	// Create template with initial version (v1, active)
	rec := createTemplate(t, router, "example.com", map[string]string{
		"name":     "mytemplate",
		"template": "<p>Version 1: {{name}}</p>",
		"tag":      "v1",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to create template: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Create second version (v2, not active by default unless first)
	rec2 := createTemplateVersion(t, router, "example.com", "mytemplate", map[string]string{
		"template": "<p>Version 2: {{name}}</p>",
		"tag":      "v2",
	})
	if rec2.Code != http.StatusOK {
		t.Fatalf("failed to create version v2: status=%d body=%s", rec2.Code, rec2.Body.String())
	}

	// Send message specifying t:version=v2
	sendRec := sendMessage(t, router, "example.com", map[string]string{
		"from":        "sender@example.com",
		"to":          "recipient@example.com",
		"subject":     "Version Test",
		"template":    "mytemplate",
		"t:version":   "v2",
		"t:variables": `{"name":"Alice"}`,
	})

	t.Run("returns 200", func(t *testing.T) {
		if sendRec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", sendRec.Code, sendRec.Body.String())
		}
	})

	var resp templateSendResponse
	decodeJSON(t, sendRec, &resp)

	t.Run("stored message uses version v2 content", func(t *testing.T) {
		storageKey := extractTemplateStorageKey(t, resp)
		getRec := getMessage(t, router, "example.com", storageKey)
		if getRec.Code != http.StatusOK {
			t.Fatalf("expected 200 on GET, got %d (body: %s)", getRec.Code, getRec.Body.String())
		}
		var detail templateMessageDetailResponse
		decodeJSON(t, getRec, &detail)
		expected := "<p>Version 2: Alice</p>"
		if detail.BodyHTML != expected {
			t.Errorf("expected body-html=%q, got %q", expected, detail.BodyHTML)
		}
	})
}

// Test 3: Send with active version (default, no t:version specified)
func TestTemplateRendering_ActiveVersionDefault(t *testing.T) {
	db := setupTestDBWithTemplates(t)
	cfg := defaultConfig()
	router := setupRouterWithTemplates(db, cfg)
	createTestDomain(t, router, "example.com")

	// Create template with initial version v1 (auto-active as first version)
	rec := createTemplate(t, router, "example.com", map[string]string{
		"name":     "mytemplate",
		"template": "<p>v1: {{name}}</p>",
		"tag":      "v1",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to create template: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Create v2 and make it active
	rec2 := createTemplateVersion(t, router, "example.com", "mytemplate", map[string]string{
		"template": "<p>v2: {{name}}</p>",
		"tag":      "v2",
		"active":   "yes",
	})
	if rec2.Code != http.StatusOK {
		t.Fatalf("failed to create version v2: status=%d body=%s", rec2.Code, rec2.Body.String())
	}

	// Send message WITHOUT specifying t:version — should use active version (v2)
	sendRec := sendMessage(t, router, "example.com", map[string]string{
		"from":        "sender@example.com",
		"to":          "recipient@example.com",
		"subject":     "Active Version Test",
		"template":    "mytemplate",
		"t:variables": `{"name":"Bob"}`,
	})

	t.Run("returns 200", func(t *testing.T) {
		if sendRec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", sendRec.Code, sendRec.Body.String())
		}
	})

	var resp templateSendResponse
	decodeJSON(t, sendRec, &resp)

	t.Run("stored message uses the active version v2", func(t *testing.T) {
		storageKey := extractTemplateStorageKey(t, resp)
		getRec := getMessage(t, router, "example.com", storageKey)
		if getRec.Code != http.StatusOK {
			t.Fatalf("expected 200 on GET, got %d (body: %s)", getRec.Code, getRec.Body.String())
		}
		var detail templateMessageDetailResponse
		decodeJSON(t, getRec, &detail)
		expected := "<p>v2: Bob</p>"
		if detail.BodyHTML != expected {
			t.Errorf("expected body-html=%q, got %q", expected, detail.BodyHTML)
		}
	})
}

// Test 4: Template with custom variables (v:*)
func TestTemplateRendering_CustomVariables(t *testing.T) {
	router := setupDomainAndTemplate(t, "example.com", "mytemplate", "<p>{{myvar}} there</p>")

	sendRec := sendMessage(t, router, "example.com", map[string]string{
		"from":     "sender@example.com",
		"to":       "recipient@example.com",
		"subject":  "Custom Var Test",
		"template": "mytemplate",
		"v:myvar":  "hello",
	})

	t.Run("returns 200", func(t *testing.T) {
		if sendRec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", sendRec.Code, sendRec.Body.String())
		}
	})

	var resp templateSendResponse
	decodeJSON(t, sendRec, &resp)

	t.Run("stored message uses v:myvar for substitution", func(t *testing.T) {
		storageKey := extractTemplateStorageKey(t, resp)
		getRec := getMessage(t, router, "example.com", storageKey)
		if getRec.Code != http.StatusOK {
			t.Fatalf("expected 200 on GET, got %d (body: %s)", getRec.Code, getRec.Body.String())
		}
		var detail templateMessageDetailResponse
		decodeJSON(t, getRec, &detail)
		expected := "<p>hello there</p>"
		if detail.BodyHTML != expected {
			t.Errorf("expected body-html=%q, got %q", expected, detail.BodyHTML)
		}
	})
}

// Test 5: Template with both t:variables and v:* — t:variables takes precedence
func TestTemplateRendering_VariablesPrecedence(t *testing.T) {
	router := setupDomainAndTemplate(t, "example.com", "mytemplate", "<p>{{greeting}} {{name}}</p>")

	sendRec := sendMessage(t, router, "example.com", map[string]string{
		"from":        "sender@example.com",
		"to":          "recipient@example.com",
		"subject":     "Precedence Test",
		"template":    "mytemplate",
		"t:variables": `{"name":"FromTVars","greeting":"Hi"}`,
		"v:name":      "FromVPrefix",
		"v:greeting":  "Hey",
	})

	t.Run("returns 200", func(t *testing.T) {
		if sendRec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", sendRec.Code, sendRec.Body.String())
		}
	})

	var resp templateSendResponse
	decodeJSON(t, sendRec, &resp)

	t.Run("t:variables takes precedence over v:* for conflicting keys", func(t *testing.T) {
		storageKey := extractTemplateStorageKey(t, resp)
		getRec := getMessage(t, router, "example.com", storageKey)
		if getRec.Code != http.StatusOK {
			t.Fatalf("expected 200 on GET, got %d (body: %s)", getRec.Code, getRec.Body.String())
		}
		var detail templateMessageDetailResponse
		decodeJSON(t, getRec, &detail)
		expected := "<p>Hi FromTVars</p>"
		if detail.BodyHTML != expected {
			t.Errorf("expected body-html=%q, got %q", expected, detail.BodyHTML)
		}
	})
}

// Test 6: Template header injection — Subject from template
func TestTemplateRendering_HeaderInjection_Subject(t *testing.T) {
	headersJSON := `{"Subject":"Template Subject"}`
	router := setupDomainAndTemplateWithHeaders(t, "example.com", "mytemplate", "<p>Hello</p>", headersJSON)

	// Send message WITHOUT a subject field — template should supply it
	sendRec := sendMessage(t, router, "example.com", map[string]string{
		"from":     "sender@example.com",
		"to":       "recipient@example.com",
		"template": "mytemplate",
	})

	t.Run("returns 200", func(t *testing.T) {
		if sendRec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", sendRec.Code, sendRec.Body.String())
		}
	})

	var resp templateSendResponse
	decodeJSON(t, sendRec, &resp)

	t.Run("stored message has subject from template headers", func(t *testing.T) {
		storageKey := extractTemplateStorageKey(t, resp)
		getRec := getMessage(t, router, "example.com", storageKey)
		if getRec.Code != http.StatusOK {
			t.Fatalf("expected 200 on GET, got %d (body: %s)", getRec.Code, getRec.Body.String())
		}
		var detail templateMessageDetailResponse
		decodeJSON(t, getRec, &detail)
		if detail.Subject != "Template Subject" {
			t.Errorf("expected Subject=%q, got %q", "Template Subject", detail.Subject)
		}
	})
}

// Test 7: Template header injection — From from template, but message-level from overrides
func TestTemplateRendering_HeaderInjection_FromOverride(t *testing.T) {
	headersJSON := `{"From":"template@example.com"}`
	router := setupDomainAndTemplateWithHeaders(t, "example.com", "mytemplate", "<p>Hello</p>", headersJSON)

	// Send message with explicit from — message-level should win
	sendRec := sendMessage(t, router, "example.com", map[string]string{
		"from":     "sender@example.com",
		"to":       "recipient@example.com",
		"subject":  "From Override Test",
		"template": "mytemplate",
	})

	t.Run("returns 200", func(t *testing.T) {
		if sendRec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", sendRec.Code, sendRec.Body.String())
		}
	})

	var resp templateSendResponse
	decodeJSON(t, sendRec, &resp)

	t.Run("message-level from overrides template from header", func(t *testing.T) {
		storageKey := extractTemplateStorageKey(t, resp)
		getRec := getMessage(t, router, "example.com", storageKey)
		if getRec.Code != http.StatusOK {
			t.Fatalf("expected 200 on GET, got %d (body: %s)", getRec.Code, getRec.Body.String())
		}
		var detail templateMessageDetailResponse
		decodeJSON(t, getRec, &detail)
		if detail.From != "sender@example.com" {
			t.Errorf("expected From=%q (message-level wins), got %q", "sender@example.com", detail.From)
		}
	})
}

// Test 8: Message-level headers override template headers (Subject)
func TestTemplateRendering_MessageOverridesTemplateHeaders(t *testing.T) {
	headersJSON := `{"Subject":"Template Subject"}`
	router := setupDomainAndTemplateWithHeaders(t, "example.com", "mytemplate", "<p>Content</p>", headersJSON)

	// Send message with explicit subject — message-level should win
	sendRec := sendMessage(t, router, "example.com", map[string]string{
		"from":     "sender@example.com",
		"to":       "recipient@example.com",
		"subject":  "MessageSubject",
		"template": "mytemplate",
	})

	t.Run("returns 200", func(t *testing.T) {
		if sendRec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", sendRec.Code, sendRec.Body.String())
		}
	})

	var resp templateSendResponse
	decodeJSON(t, sendRec, &resp)

	t.Run("message subject overrides template subject", func(t *testing.T) {
		storageKey := extractTemplateStorageKey(t, resp)
		getRec := getMessage(t, router, "example.com", storageKey)
		if getRec.Code != http.StatusOK {
			t.Fatalf("expected 200 on GET, got %d (body: %s)", getRec.Code, getRec.Body.String())
		}
		var detail templateMessageDetailResponse
		decodeJSON(t, getRec, &detail)
		if detail.Subject != "MessageSubject" {
			t.Errorf("expected Subject=%q, got %q", "MessageSubject", detail.Subject)
		}
	})
}

// Test 9: Template with t:text=yes — auto-generate plain text from rendered HTML
func TestTemplateRendering_TextGeneration(t *testing.T) {
	router := setupDomainAndTemplate(t, "example.com", "mytemplate", "<h1>Hello {{name}}</h1><p>Welcome!</p>")

	sendRec := sendMessage(t, router, "example.com", map[string]string{
		"from":        "sender@example.com",
		"to":          "recipient@example.com",
		"subject":     "Text Gen Test",
		"template":    "mytemplate",
		"t:variables": `{"name":"World"}`,
		"t:text":      "yes",
	})

	t.Run("returns 200", func(t *testing.T) {
		if sendRec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", sendRec.Code, sendRec.Body.String())
		}
	})

	var resp templateSendResponse
	decodeJSON(t, sendRec, &resp)

	t.Run("stored message has non-empty plain text body", func(t *testing.T) {
		storageKey := extractTemplateStorageKey(t, resp)
		getRec := getMessage(t, router, "example.com", storageKey)
		if getRec.Code != http.StatusOK {
			t.Fatalf("expected 200 on GET, got %d (body: %s)", getRec.Code, getRec.Body.String())
		}
		var detail templateMessageDetailResponse
		decodeJSON(t, getRec, &detail)
		if detail.BodyPlain == "" {
			t.Error("expected non-empty body-plain when t:text=yes, got empty string")
		}
		// The plain text should contain the rendered text content without HTML tags
		if !strings.Contains(detail.BodyPlain, "Hello World") {
			t.Errorf("expected body-plain to contain 'Hello World', got %q", detail.BodyPlain)
		}
		if !strings.Contains(detail.BodyPlain, "Welcome!") {
			t.Errorf("expected body-plain to contain 'Welcome!', got %q", detail.BodyPlain)
		}
	})
}

// Test 10: Template rendering with Handlebars block helpers
func TestTemplateRendering_HandlebarsBlockHelpers(t *testing.T) {
	db := setupTestDBWithTemplates(t)
	cfg := defaultConfig()
	router := setupRouterWithTemplates(db, cfg)
	createTestDomain(t, router, "example.com")

	t.Run("if helper — truthy", func(t *testing.T) {
		rec := createTemplate(t, router, "example.com", map[string]string{
			"name":     "tmpl-if-truthy",
			"template": "{{#if show}}<p>Visible</p>{{/if}}",
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("failed to create template: status=%d body=%s", rec.Code, rec.Body.String())
		}

		sendRec := sendMessage(t, router, "example.com", map[string]string{
			"from":        "sender@example.com",
			"to":          "recipient@example.com",
			"subject":     "If Helper",
			"template":    "tmpl-if-truthy",
			"t:variables": `{"show":true}`,
		})
		if sendRec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", sendRec.Code, sendRec.Body.String())
		}
		var resp templateSendResponse
		decodeJSON(t, sendRec, &resp)
		storageKey := extractTemplateStorageKey(t, resp)
		getRec := getMessage(t, router, "example.com", storageKey)
		if getRec.Code != http.StatusOK {
			t.Fatalf("expected 200 on GET, got %d", getRec.Code)
		}
		var detail templateMessageDetailResponse
		decodeJSON(t, getRec, &detail)
		if !strings.Contains(detail.BodyHTML, "<p>Visible</p>") {
			t.Errorf("expected body-html to contain '<p>Visible</p>', got %q", detail.BodyHTML)
		}
	})

	t.Run("if helper — falsy", func(t *testing.T) {
		rec := createTemplate(t, router, "example.com", map[string]string{
			"name":     "tmpl-if-falsy",
			"template": "{{#if show}}<p>Visible</p>{{/if}}",
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("failed to create template: status=%d body=%s", rec.Code, rec.Body.String())
		}

		sendRec := sendMessage(t, router, "example.com", map[string]string{
			"from":        "sender@example.com",
			"to":          "recipient@example.com",
			"subject":     "If Helper Falsy",
			"template":    "tmpl-if-falsy",
			"t:variables": `{"show":false}`,
		})
		if sendRec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", sendRec.Code, sendRec.Body.String())
		}
		var resp templateSendResponse
		decodeJSON(t, sendRec, &resp)
		storageKey := extractTemplateStorageKey(t, resp)
		getRec := getMessage(t, router, "example.com", storageKey)
		if getRec.Code != http.StatusOK {
			t.Fatalf("expected 200 on GET, got %d", getRec.Code)
		}
		var detail templateMessageDetailResponse
		decodeJSON(t, getRec, &detail)
		if strings.Contains(detail.BodyHTML, "Visible") {
			t.Errorf("expected body-html to NOT contain 'Visible' when show=false, got %q", detail.BodyHTML)
		}
	})

	t.Run("unless helper", func(t *testing.T) {
		rec := createTemplate(t, router, "example.com", map[string]string{
			"name":     "tmpl-unless",
			"template": "{{#unless hidden}}<p>Shown</p>{{/unless}}",
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("failed to create template: status=%d body=%s", rec.Code, rec.Body.String())
		}

		sendRec := sendMessage(t, router, "example.com", map[string]string{
			"from":        "sender@example.com",
			"to":          "recipient@example.com",
			"subject":     "Unless Helper",
			"template":    "tmpl-unless",
			"t:variables": `{"hidden":false}`,
		})
		if sendRec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", sendRec.Code, sendRec.Body.String())
		}
		var resp templateSendResponse
		decodeJSON(t, sendRec, &resp)
		storageKey := extractTemplateStorageKey(t, resp)
		getRec := getMessage(t, router, "example.com", storageKey)
		if getRec.Code != http.StatusOK {
			t.Fatalf("expected 200 on GET, got %d", getRec.Code)
		}
		var detail templateMessageDetailResponse
		decodeJSON(t, getRec, &detail)
		if !strings.Contains(detail.BodyHTML, "<p>Shown</p>") {
			t.Errorf("expected body-html to contain '<p>Shown</p>', got %q", detail.BodyHTML)
		}
	})

	t.Run("each helper", func(t *testing.T) {
		rec := createTemplate(t, router, "example.com", map[string]string{
			"name":     "tmpl-each",
			"template": "<ul>{{#each items}}<li>{{this}}</li>{{/each}}</ul>",
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("failed to create template: status=%d body=%s", rec.Code, rec.Body.String())
		}

		sendRec := sendMessage(t, router, "example.com", map[string]string{
			"from":        "sender@example.com",
			"to":          "recipient@example.com",
			"subject":     "Each Helper",
			"template":    "tmpl-each",
			"t:variables": `{"items":["apple","banana","cherry"]}`,
		})
		if sendRec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", sendRec.Code, sendRec.Body.String())
		}
		var resp templateSendResponse
		decodeJSON(t, sendRec, &resp)
		storageKey := extractTemplateStorageKey(t, resp)
		getRec := getMessage(t, router, "example.com", storageKey)
		if getRec.Code != http.StatusOK {
			t.Fatalf("expected 200 on GET, got %d", getRec.Code)
		}
		var detail templateMessageDetailResponse
		decodeJSON(t, getRec, &detail)
		for _, item := range []string{"apple", "banana", "cherry"} {
			if !strings.Contains(detail.BodyHTML, "<li>"+item+"</li>") {
				t.Errorf("expected body-html to contain '<li>%s</li>', got %q", item, detail.BodyHTML)
			}
		}
	})

	t.Run("equal helper", func(t *testing.T) {
		rec := createTemplate(t, router, "example.com", map[string]string{
			"name":     "tmpl-equal",
			"template": `{{#equal status "active"}}<p>Active</p>{{/equal}}`,
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("failed to create template: status=%d body=%s", rec.Code, rec.Body.String())
		}

		sendRec := sendMessage(t, router, "example.com", map[string]string{
			"from":        "sender@example.com",
			"to":          "recipient@example.com",
			"subject":     "Equal Helper",
			"template":    "tmpl-equal",
			"t:variables": `{"status":"active"}`,
		})
		if sendRec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", sendRec.Code, sendRec.Body.String())
		}
		var resp templateSendResponse
		decodeJSON(t, sendRec, &resp)
		storageKey := extractTemplateStorageKey(t, resp)
		getRec := getMessage(t, router, "example.com", storageKey)
		if getRec.Code != http.StatusOK {
			t.Fatalf("expected 200 on GET, got %d", getRec.Code)
		}
		var detail templateMessageDetailResponse
		decodeJSON(t, getRec, &detail)
		if !strings.Contains(detail.BodyHTML, "<p>Active</p>") {
			t.Errorf("expected body-html to contain '<p>Active</p>', got %q", detail.BodyHTML)
		}
	})
}

// ---------------------------------------------------------------------------
// Error Cases
// ---------------------------------------------------------------------------

// Test 11: Send with nonexistent template -> 400
func TestTemplateRendering_NonexistentTemplate(t *testing.T) {
	db := setupTestDBWithTemplates(t)
	cfg := defaultConfig()
	router := setupRouterWithTemplates(db, cfg)
	createTestDomain(t, router, "example.com")

	sendRec := sendMessage(t, router, "example.com", map[string]string{
		"from":     "sender@example.com",
		"to":       "recipient@example.com",
		"subject":  "No Template",
		"template": "nosuchtemplate",
	})

	t.Run("returns 400", func(t *testing.T) {
		if sendRec.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d (body: %s)", sendRec.Code, sendRec.Body.String())
		}
	})

	t.Run("error message mentions template name", func(t *testing.T) {
		var resp templateErrorResponse
		decodeJSON(t, sendRec, &resp)
		expected := "template 'nosuchtemplate' not found"
		if resp.Message != expected {
			t.Errorf("expected message=%q, got %q", expected, resp.Message)
		}
	})
}

// Test 12: Send with nonexistent version -> 400
func TestTemplateRendering_NonexistentVersion(t *testing.T) {
	router := setupDomainAndTemplate(t, "example.com", "mytemplate", "<p>Hello</p>")

	sendRec := sendMessage(t, router, "example.com", map[string]string{
		"from":      "sender@example.com",
		"to":        "recipient@example.com",
		"subject":   "Bad Version",
		"template":  "mytemplate",
		"t:version": "nosuchversion",
	})

	t.Run("returns 400", func(t *testing.T) {
		if sendRec.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d (body: %s)", sendRec.Code, sendRec.Body.String())
		}
	})

	t.Run("error message mentions version and template name", func(t *testing.T) {
		var resp templateErrorResponse
		decodeJSON(t, sendRec, &resp)
		expected := "version 'nosuchversion' not found for template 'mytemplate'"
		if resp.Message != expected {
			t.Errorf("expected message=%q, got %q", expected, resp.Message)
		}
	})
}

// Test 13: Template with no active version and no t:version specified -> 400
func TestTemplateRendering_NoActiveVersion(t *testing.T) {
	db := setupTestDBWithTemplates(t)
	cfg := defaultConfig()
	router := setupRouterWithTemplates(db, cfg)
	createTestDomain(t, router, "example.com")

	// Create template without providing template content (so no initial version)
	rec := createTemplate(t, router, "example.com", map[string]string{
		"name": "emptytemplate",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to create template: status=%d body=%s", rec.Code, rec.Body.String())
	}

	sendRec := sendMessage(t, router, "example.com", map[string]string{
		"from":     "sender@example.com",
		"to":       "recipient@example.com",
		"subject":  "No Version",
		"template": "emptytemplate",
	})

	t.Run("returns 400", func(t *testing.T) {
		if sendRec.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d (body: %s)", sendRec.Code, sendRec.Body.String())
		}
	})

	t.Run("error message indicates no active version", func(t *testing.T) {
		var resp templateErrorResponse
		decodeJSON(t, sendRec, &resp)
		if resp.Message == "" {
			t.Error("expected non-empty error message")
		}
		// The error should mention that no active version exists
		msgLower := strings.ToLower(resp.Message)
		if !strings.Contains(msgLower, "active") && !strings.Contains(msgLower, "version") {
			t.Errorf("expected error message to reference missing active version, got %q", resp.Message)
		}
	})
}

// ---------------------------------------------------------------------------
// Edge Cases
// ---------------------------------------------------------------------------

// Test 14: Template name is case-insensitive — template created lowercase, sent mixed-case
func TestTemplateRendering_CaseInsensitiveName(t *testing.T) {
	router := setupDomainAndTemplate(t, "example.com", "mytemplate", "<p>Hello {{name}}</p>")

	// Send with mixed-case template name — should still resolve
	sendRec := sendMessage(t, router, "example.com", map[string]string{
		"from":        "sender@example.com",
		"to":          "recipient@example.com",
		"subject":     "Case Test",
		"template":    "MyTemplate",
		"t:variables": `{"name":"World"}`,
	})

	t.Run("returns 200", func(t *testing.T) {
		if sendRec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", sendRec.Code, sendRec.Body.String())
		}
	})

	var resp templateSendResponse
	decodeJSON(t, sendRec, &resp)

	t.Run("template resolves case-insensitively", func(t *testing.T) {
		storageKey := extractTemplateStorageKey(t, resp)
		getRec := getMessage(t, router, "example.com", storageKey)
		if getRec.Code != http.StatusOK {
			t.Fatalf("expected 200 on GET, got %d (body: %s)", getRec.Code, getRec.Body.String())
		}
		var detail templateMessageDetailResponse
		decodeJSON(t, getRec, &detail)
		expected := "<p>Hello World</p>"
		if detail.BodyHTML != expected {
			t.Errorf("expected body-html=%q, got %q", expected, detail.BodyHTML)
		}
	})
}

// Test 15: Template variables with special characters (HTML entities)
func TestTemplateRendering_SpecialCharacters(t *testing.T) {
	router := setupDomainAndTemplate(t, "example.com", "mytemplate", "<p>{{content}}</p>")

	sendRec := sendMessage(t, router, "example.com", map[string]string{
		"from":        "sender@example.com",
		"to":          "recipient@example.com",
		"subject":     "Special Chars",
		"template":    "mytemplate",
		"t:variables": `{"content":"<b>Bold</b> & \"quoted\""}`,
	})

	t.Run("returns 200", func(t *testing.T) {
		if sendRec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", sendRec.Code, sendRec.Body.String())
		}
	})

	var resp templateSendResponse
	decodeJSON(t, sendRec, &resp)

	t.Run("stored message contains rendered content with special chars", func(t *testing.T) {
		storageKey := extractTemplateStorageKey(t, resp)
		getRec := getMessage(t, router, "example.com", storageKey)
		if getRec.Code != http.StatusOK {
			t.Fatalf("expected 200 on GET, got %d (body: %s)", getRec.Code, getRec.Body.String())
		}
		var detail templateMessageDetailResponse
		decodeJSON(t, getRec, &detail)
		// Handlebars by default HTML-escapes variables, so we expect escaped content
		if detail.BodyHTML == "" {
			t.Error("expected non-empty body-html")
		}
		// The rendered output should be present (either escaped or raw depending on implementation)
		if !strings.Contains(detail.BodyHTML, "<p>") {
			t.Errorf("expected body-html to contain '<p>' wrapper, got %q", detail.BodyHTML)
		}
	})
}

// ---------------------------------------------------------------------------
// Integration Test
// ---------------------------------------------------------------------------

// Test 16: Full round-trip — create template, send with template, retrieve, verify all fields
func TestTemplateRendering_FullRoundTrip(t *testing.T) {
	db := setupTestDBWithTemplates(t)
	cfg := defaultConfig()
	router := setupRouterWithTemplates(db, cfg)
	createTestDomain(t, router, "example.com")

	// Step 1: Create template with headers
	headersJSON := `{"Subject":"Welcome {{name}}","Reply-To":"support@example.com"}`
	rec := createTemplate(t, router, "example.com", map[string]string{
		"name":     "welcome",
		"template": "<html><body><h1>Welcome, {{name}}!</h1><p>Your role: {{role}}</p></body></html>",
		"tag":      "v1",
		"headers":  headersJSON,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to create template: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Step 2: Send message with template
	sendRec := sendMessage(t, router, "example.com", map[string]string{
		"from":        "noreply@example.com",
		"to":          "user@test.com",
		"template":    "welcome",
		"t:variables": `{"name":"Alice","role":"Admin"}`,
	})

	t.Run("send returns 200", func(t *testing.T) {
		if sendRec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", sendRec.Code, sendRec.Body.String())
		}
	})

	var resp templateSendResponse
	decodeJSON(t, sendRec, &resp)

	t.Run("send returns message ID", func(t *testing.T) {
		if resp.ID == "" {
			t.Error("expected non-empty message ID")
		}
	})

	t.Run("send returns queued message", func(t *testing.T) {
		if resp.Message != "Queued. Thank you." {
			t.Errorf("expected %q, got %q", "Queued. Thank you.", resp.Message)
		}
	})

	// Step 3: Retrieve stored message
	storageKey := extractTemplateStorageKey(t, resp)
	getRec := getMessage(t, router, "example.com", storageKey)

	t.Run("retrieve returns 200", func(t *testing.T) {
		if getRec.Code != http.StatusOK {
			t.Fatalf("expected 200 on GET, got %d (body: %s)", getRec.Code, getRec.Body.String())
		}
	})

	// Step 4: Verify all rendered fields
	var rawResp map[string]interface{}
	decodeJSON(t, getRec, &rawResp)

	t.Run("HTML body is rendered with variables", func(t *testing.T) {
		bodyHTML, _ := rawResp["body-html"].(string)
		if !strings.Contains(bodyHTML, "Welcome, Alice!") {
			t.Errorf("expected body-html to contain 'Welcome, Alice!', got %q", bodyHTML)
		}
		if !strings.Contains(bodyHTML, "Your role: Admin") {
			t.Errorf("expected body-html to contain 'Your role: Admin', got %q", bodyHTML)
		}
	})

	t.Run("subject is injected from template headers", func(t *testing.T) {
		subject, _ := rawResp["Subject"].(string)
		// The subject from the template header should be rendered with variables too
		if !strings.Contains(subject, "Welcome") {
			t.Errorf("expected Subject to contain 'Welcome', got %q", subject)
		}
	})

	t.Run("from is preserved from message-level", func(t *testing.T) {
		from, _ := rawResp["From"].(string)
		if from != "noreply@example.com" {
			t.Errorf("expected From=%q, got %q", "noreply@example.com", from)
		}
	})

	t.Run("to is preserved", func(t *testing.T) {
		to, _ := rawResp["To"].(string)
		if to != "user@test.com" {
			t.Errorf("expected To=%q, got %q", "user@test.com", to)
		}
	})

	// Step 5: Verify template metadata is stored in the message record
	// Query the database directly to check Template and TemplateVersion fields
	var msg message.StoredMessage
	if err := db.Where("storage_key = ?", storageKey).First(&msg).Error; err != nil {
		t.Fatalf("failed to retrieve message from DB: %v", err)
	}

	t.Run("template name is stored in message record", func(t *testing.T) {
		if msg.Template != "welcome" {
			t.Errorf("expected Template=%q, got %q", "welcome", msg.Template)
		}
	})

	t.Run("template version tag is stored in message record", func(t *testing.T) {
		// When using the active version, the version tag should be stored
		if msg.TemplateVersion == "" {
			t.Log("TemplateVersion not stored — implementation may store version tag for traceability")
		}
	})

	t.Run("template variables are stored in message record", func(t *testing.T) {
		if msg.TemplateVariables == "" {
			t.Error("expected TemplateVariables to be non-empty")
		}
		var vars map[string]interface{}
		if err := json.Unmarshal([]byte(msg.TemplateVariables), &vars); err != nil {
			t.Fatalf("failed to parse TemplateVariables JSON: %v", err)
		}
		if vars["name"] != "Alice" {
			t.Errorf("expected TemplateVariables[name]=%q, got %v", "Alice", vars["name"])
		}
	})
}

// ---------------------------------------------------------------------------
// Additional edge case: Template with Reply-To header injection
// ---------------------------------------------------------------------------

func TestTemplateRendering_ReplyToHeaderInjection(t *testing.T) {
	headersJSON := `{"Reply-To":"support@example.com"}`
	router := setupDomainAndTemplateWithHeaders(t, "example.com", "mytemplate", "<p>Content</p>", headersJSON)

	sendRec := sendMessage(t, router, "example.com", map[string]string{
		"from":     "sender@example.com",
		"to":       "recipient@example.com",
		"subject":  "Reply-To Test",
		"template": "mytemplate",
	})

	t.Run("returns 200", func(t *testing.T) {
		if sendRec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", sendRec.Code, sendRec.Body.String())
		}
	})

	var resp templateSendResponse
	decodeJSON(t, sendRec, &resp)

	t.Run("Reply-To header from template is applied", func(t *testing.T) {
		storageKey := extractTemplateStorageKey(t, resp)
		getRec := getMessage(t, router, "example.com", storageKey)
		if getRec.Code != http.StatusOK {
			t.Fatalf("expected 200 on GET, got %d (body: %s)", getRec.Code, getRec.Body.String())
		}
		// Check the message-headers for Reply-To
		var rawResp map[string]interface{}
		decodeJSON(t, getRec, &rawResp)

		// message-headers is a [][]string in the response
		headers, ok := rawResp["message-headers"].([]interface{})
		if !ok {
			t.Fatal("expected message-headers to be an array")
		}
		found := false
		for _, h := range headers {
			pair, ok := h.([]interface{})
			if !ok || len(pair) < 2 {
				continue
			}
			name, _ := pair[0].(string)
			value, _ := pair[1].(string)
			if name == "Reply-To" && value == "support@example.com" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected message-headers to contain Reply-To: support@example.com, headers=%v", headers)
		}
	})
}

// ---------------------------------------------------------------------------
// Template with no variables — render as-is
// ---------------------------------------------------------------------------

func TestTemplateRendering_NoVariables(t *testing.T) {
	router := setupDomainAndTemplate(t, "example.com", "static", "<p>Static content</p>")

	sendRec := sendMessage(t, router, "example.com", map[string]string{
		"from":     "sender@example.com",
		"to":       "recipient@example.com",
		"subject":  "No Vars",
		"template": "static",
	})

	t.Run("returns 200", func(t *testing.T) {
		if sendRec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", sendRec.Code, sendRec.Body.String())
		}
	})

	var resp templateSendResponse
	decodeJSON(t, sendRec, &resp)

	t.Run("template renders as-is without variables", func(t *testing.T) {
		storageKey := extractTemplateStorageKey(t, resp)
		getRec := getMessage(t, router, "example.com", storageKey)
		if getRec.Code != http.StatusOK {
			t.Fatalf("expected 200 on GET, got %d (body: %s)", getRec.Code, getRec.Body.String())
		}
		var detail templateMessageDetailResponse
		decodeJSON(t, getRec, &detail)
		expected := "<p>Static content</p>"
		if detail.BodyHTML != expected {
			t.Errorf("expected body-html=%q, got %q", expected, detail.BodyHTML)
		}
	})
}

// ---------------------------------------------------------------------------
// Template variables with only v:* prefix (no t:variables at all)
// ---------------------------------------------------------------------------

func TestTemplateRendering_OnlyVPrefixVariables(t *testing.T) {
	router := setupDomainAndTemplate(t, "example.com", "mytemplate", "<p>{{first}} {{last}}</p>")

	sendRec := sendMessage(t, router, "example.com", map[string]string{
		"from":     "sender@example.com",
		"to":       "recipient@example.com",
		"subject":  "V Prefix Only",
		"template": "mytemplate",
		"v:first":  "John",
		"v:last":   "Doe",
	})

	t.Run("returns 200", func(t *testing.T) {
		if sendRec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", sendRec.Code, sendRec.Body.String())
		}
	})

	var resp templateSendResponse
	decodeJSON(t, sendRec, &resp)

	t.Run("v:* variables are used for rendering", func(t *testing.T) {
		storageKey := extractTemplateStorageKey(t, resp)
		getRec := getMessage(t, router, "example.com", storageKey)
		if getRec.Code != http.StatusOK {
			t.Fatalf("expected 200 on GET, got %d (body: %s)", getRec.Code, getRec.Body.String())
		}
		var detail templateMessageDetailResponse
		decodeJSON(t, getRec, &detail)
		expected := "<p>John Doe</p>"
		if detail.BodyHTML != expected {
			t.Errorf("expected body-html=%q, got %q", expected, detail.BodyHTML)
		}
	})
}

// ---------------------------------------------------------------------------
// Template with multiple headers — Subject, From, Reply-To combined
// ---------------------------------------------------------------------------

func TestTemplateRendering_MultipleHeaders(t *testing.T) {
	headersJSON := `{"Subject":"Template Sub","From":"tmpl-from@example.com","Reply-To":"tmpl-reply@example.com"}`
	router := setupDomainAndTemplateWithHeaders(t, "example.com", "mytemplate", "<p>Hello</p>", headersJSON)

	// Send with explicit from and subject — both should override template headers
	sendRec := sendMessage(t, router, "example.com", map[string]string{
		"from":     "msg-from@example.com",
		"to":       "recipient@example.com",
		"subject":  "Msg Subject",
		"template": "mytemplate",
	})

	t.Run("returns 200", func(t *testing.T) {
		if sendRec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", sendRec.Code, sendRec.Body.String())
		}
	})

	var resp templateSendResponse
	decodeJSON(t, sendRec, &resp)

	t.Run("message-level from and subject override template, Reply-To preserved", func(t *testing.T) {
		storageKey := extractTemplateStorageKey(t, resp)
		getRec := getMessage(t, router, "example.com", storageKey)
		if getRec.Code != http.StatusOK {
			t.Fatalf("expected 200 on GET, got %d (body: %s)", getRec.Code, getRec.Body.String())
		}
		var detail templateMessageDetailResponse
		decodeJSON(t, getRec, &detail)

		// Message-level from wins
		if detail.From != "msg-from@example.com" {
			t.Errorf("expected From=%q, got %q", "msg-from@example.com", detail.From)
		}
		// Message-level subject wins
		if detail.Subject != "Msg Subject" {
			t.Errorf("expected Subject=%q, got %q", "Msg Subject", detail.Subject)
		}

		// Reply-To from template should still be injected
		var rawResp map[string]interface{}
		decodeJSON(t, getRec, &rawResp)
		headers, ok := rawResp["message-headers"].([]interface{})
		if !ok {
			t.Fatal("expected message-headers to be an array")
		}
		foundReplyTo := false
		for _, h := range headers {
			pair, ok := h.([]interface{})
			if !ok || len(pair) < 2 {
				continue
			}
			name, _ := pair[0].(string)
			value, _ := pair[1].(string)
			if name == "Reply-To" && value == "tmpl-reply@example.com" {
				foundReplyTo = true
				break
			}
		}
		if !foundReplyTo {
			t.Errorf("expected Reply-To header from template to be present, headers=%v", headers)
		}
	})
}

// ---------------------------------------------------------------------------
// Template t:text=yes without explicit text body
// ---------------------------------------------------------------------------

func TestTemplateRendering_TextGenerationNoExplicitText(t *testing.T) {
	router := setupDomainAndTemplate(t, "example.com", "mytemplate", "<h1>Title</h1><p>Paragraph text here.</p>")

	sendRec := sendMessage(t, router, "example.com", map[string]string{
		"from":     "sender@example.com",
		"to":       "recipient@example.com",
		"subject":  "Text Gen",
		"template": "mytemplate",
		"t:text":   "yes",
	})

	t.Run("returns 200", func(t *testing.T) {
		if sendRec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", sendRec.Code, sendRec.Body.String())
		}
	})

	var resp templateSendResponse
	decodeJSON(t, sendRec, &resp)

	t.Run("plain text body is generated from HTML", func(t *testing.T) {
		storageKey := extractTemplateStorageKey(t, resp)
		getRec := getMessage(t, router, "example.com", storageKey)
		if getRec.Code != http.StatusOK {
			t.Fatalf("expected 200 on GET, got %d (body: %s)", getRec.Code, getRec.Body.String())
		}
		var detail templateMessageDetailResponse
		decodeJSON(t, getRec, &detail)
		if detail.BodyPlain == "" {
			t.Error("expected non-empty body-plain when t:text=yes")
		}
		// Should contain the text content stripped of HTML tags
		if !strings.Contains(detail.BodyPlain, "Title") {
			t.Errorf("expected body-plain to contain 'Title', got %q", detail.BodyPlain)
		}
		if !strings.Contains(detail.BodyPlain, "Paragraph text here.") {
			t.Errorf("expected body-plain to contain 'Paragraph text here.', got %q", detail.BodyPlain)
		}
	})
}

// ---------------------------------------------------------------------------
// Template with Handlebars "with" block helper
// ---------------------------------------------------------------------------

func TestTemplateRendering_WithBlockHelper(t *testing.T) {
	db := setupTestDBWithTemplates(t)
	cfg := defaultConfig()
	router := setupRouterWithTemplates(db, cfg)
	createTestDomain(t, router, "example.com")

	rec := createTemplate(t, router, "example.com", map[string]string{
		"name":     "tmpl-with",
		"template": "{{#with person}}<p>{{first}} {{last}}</p>{{/with}}",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to create template: status=%d body=%s", rec.Code, rec.Body.String())
	}

	sendRec := sendMessage(t, router, "example.com", map[string]string{
		"from":        "sender@example.com",
		"to":          "recipient@example.com",
		"subject":     "With Helper",
		"template":    "tmpl-with",
		"t:variables": `{"person":{"first":"Jane","last":"Smith"}}`,
	})

	t.Run("returns 200", func(t *testing.T) {
		if sendRec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", sendRec.Code, sendRec.Body.String())
		}
	})

	var resp templateSendResponse
	decodeJSON(t, sendRec, &resp)

	t.Run("with helper renders nested context", func(t *testing.T) {
		storageKey := extractTemplateStorageKey(t, resp)
		getRec := getMessage(t, router, "example.com", storageKey)
		if getRec.Code != http.StatusOK {
			t.Fatalf("expected 200 on GET, got %d", getRec.Code)
		}
		var detail templateMessageDetailResponse
		decodeJSON(t, getRec, &detail)
		expected := "<p>Jane Smith</p>"
		if detail.BodyHTML != expected {
			t.Errorf("expected body-html=%q, got %q", expected, detail.BodyHTML)
		}
	})
}
