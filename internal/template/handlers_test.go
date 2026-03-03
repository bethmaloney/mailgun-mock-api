package template_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bethmaloney/mailgun-mock-api/internal/domain"
	"github.com/bethmaloney/mailgun-mock-api/internal/mock"
	"github.com/bethmaloney/mailgun-mock-api/internal/template"
	"github.com/go-chi/chi/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const testDomain = "test.example.com"

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
		&template.Template{}, &template.TemplateVersion{},
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
	th := template.NewHandlers(db)
	r := chi.NewRouter()

	r.Route("/v4/domains", func(r chi.Router) {
		r.Post("/", dh.CreateDomain)
	})

	r.Route("/v3/{domain_name}/templates", func(r chi.Router) {
		r.Post("/", th.CreateTemplate)
		r.Get("/", th.ListTemplates)
		r.Delete("/", th.DeleteAllTemplates)
		r.Get("/{name}", th.GetTemplate)
		r.Put("/{name}", th.UpdateTemplate)
		r.Delete("/{name}", th.DeleteTemplate)

		// Version routes
		r.Post("/{name}/versions", th.CreateVersion)
		r.Get("/{name}/versions", th.ListVersions)
		r.Get("/{name}/versions/{tag}", th.GetVersion)
		r.Put("/{name}/versions/{tag}", th.UpdateVersion)
		r.Delete("/{name}/versions/{tag}", th.DeleteVersion)
		r.Put("/{name}/versions/{tag}/copy/{new_tag}", th.CopyVersion)
	})

	return r
}

func setup(t *testing.T) http.Handler {
	t.Helper()
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)
	createDomain(t, router, testDomain)
	return router
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
		t.Fatalf("failed to create domain %q: status %d, body=%s", name, rec.Code, rec.Body.String())
	}
}

func createTemplate(t *testing.T, router http.Handler, domainName string, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := newMultipartRequest(t, "POST", "/v3/"+domainName+"/templates", fields)
	router.ServeHTTP(rec, req)
	return rec
}

func createVersion(t *testing.T, router http.Handler, domainName, templateName string, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := newMultipartRequest(t, "POST", "/v3/"+domainName+"/templates/"+templateName+"/versions", fields)
	router.ServeHTTP(rec, req)
	return rec
}

// doRequest is a generic helper for making HTTP requests to the router.
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

// assertStatus checks that the HTTP response code matches the expected value.
func assertStatus(t *testing.T, rec *httptest.ResponseRecorder, expected int) {
	t.Helper()
	if rec.Code != expected {
		t.Errorf("expected status %d, got %d; body=%s", expected, rec.Code, rec.Body.String())
	}
}

// assertMessage checks that the JSON response contains the expected "message" field.
func assertMessage(t *testing.T, rec *httptest.ResponseRecorder, expected string) {
	t.Helper()
	var body map[string]interface{}
	decodeJSON(t, rec, &body)
	msg, ok := body["message"].(string)
	if !ok {
		t.Fatalf("expected string 'message' field in response, got %v", body["message"])
	}
	if msg != expected {
		t.Errorf("expected message %q, got %q", expected, msg)
	}
}

// =========================================================================
// Template CRUD Tests
// =========================================================================

// ---------------------------------------------------------------------------
// 1. POST /v3/{domain_name}/templates — Create template
// ---------------------------------------------------------------------------

func TestCreateTemplate_WithInitialVersion(t *testing.T) {
	router := setup(t)

	rec := createTemplate(t, router, testDomain, map[string]string{
		"name":        "test_template",
		"description": "A test template",
		"template":    "<h1>Hello {{name}}</h1>",
		"tag":         "v0",
		"engine":      "handlebars",
	})

	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	if body["message"] != "template has been stored" {
		t.Errorf("expected message 'template has been stored', got %q", body["message"])
	}

	tmpl, ok := body["template"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected 'template' object in response")
	}
	if tmpl["name"] != "test_template" {
		t.Errorf("expected template name 'test_template', got %q", tmpl["name"])
	}
	if tmpl["description"] != "A test template" {
		t.Errorf("expected description 'A test template', got %q", tmpl["description"])
	}
	if tmpl["createdAt"] == nil || tmpl["createdAt"] == "" {
		t.Error("expected non-empty createdAt")
	}
	if _, hasCreatedBy := tmpl["createdBy"]; !hasCreatedBy {
		t.Error("expected createdBy field to be present")
	}
	if tmpl["id"] == nil || tmpl["id"] == "" {
		t.Error("expected non-empty template id")
	}

	// Verify version is present
	version, ok := tmpl["version"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected 'version' object in template response")
	}
	if version["tag"] != "v0" {
		t.Errorf("expected version tag 'v0', got %q", version["tag"])
	}
	if version["template"] != "<h1>Hello {{name}}</h1>" {
		t.Errorf("expected version template content, got %q", version["template"])
	}
	if version["engine"] != "handlebars" {
		t.Errorf("expected engine 'handlebars', got %q", version["engine"])
	}
	if _, hasMjml := version["mjml"]; !hasMjml {
		t.Error("expected mjml field to be present")
	}
	if version["active"] != true {
		t.Errorf("expected version to be active, got %v", version["active"])
	}
	if version["id"] == nil || version["id"] == "" {
		t.Error("expected non-empty version id")
	}
}

func TestCreateTemplate_WithoutVersion(t *testing.T) {
	router := setup(t)

	rec := createTemplate(t, router, testDomain, map[string]string{
		"name":        "name_only_template",
		"description": "No version",
	})

	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	if body["message"] != "template has been stored" {
		t.Errorf("expected message 'template has been stored', got %q", body["message"])
	}

	tmpl, ok := body["template"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected 'template' object in response")
	}
	if tmpl["name"] != "name_only_template" {
		t.Errorf("expected template name 'name_only_template', got %q", tmpl["name"])
	}

	// Version should be null/absent when no version content is provided
	if version := tmpl["version"]; version != nil {
		t.Errorf("expected version to be null when no content provided, got %v", version)
	}
}

func TestCreateTemplate_DuplicateName(t *testing.T) {
	router := setup(t)

	rec := createTemplate(t, router, testDomain, map[string]string{
		"name": "duplicate_test",
	})
	assertStatus(t, rec, http.StatusOK)

	// Try to create again with the same name
	rec = createTemplate(t, router, testDomain, map[string]string{
		"name": "duplicate_test",
	})

	assertStatus(t, rec, http.StatusBadRequest)
	assertMessage(t, rec, "template with name 'duplicate_test' already exists")
}

func TestCreateTemplate_MissingName(t *testing.T) {
	router := setup(t)

	rec := createTemplate(t, router, testDomain, map[string]string{
		"description": "No name provided",
	})

	assertStatus(t, rec, http.StatusBadRequest)
	assertMessage(t, rec, "name is required")
}

func TestCreateTemplate_NameLowercased(t *testing.T) {
	router := setup(t)

	rec := createTemplate(t, router, testDomain, map[string]string{
		"name": "MyTemplate",
	})
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	tmpl := body["template"].(map[string]interface{})
	if tmpl["name"] != "mytemplate" {
		t.Errorf("expected name to be lowercased to 'mytemplate', got %q", tmpl["name"])
	}

	// Verify it can be retrieved with the lowercased name
	rec = doRequest(t, router, "GET", "/v3/"+testDomain+"/templates/mytemplate", nil)
	assertStatus(t, rec, http.StatusOK)
}

func TestCreateTemplate_MaxLimitPerDomain(t *testing.T) {
	router := setup(t)

	// Create 100 templates (the maximum)
	for i := 0; i < 100; i++ {
		rec := createTemplate(t, router, testDomain, map[string]string{
			"name": fmt.Sprintf("template_%03d", i),
		})
		assertStatus(t, rec, http.StatusOK)
	}

	// The 101st should fail
	rec := createTemplate(t, router, testDomain, map[string]string{
		"name": "one_too_many",
	})
	assertStatus(t, rec, http.StatusBadRequest)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	msg, _ := body["message"].(string)
	if msg == "" {
		t.Error("expected error message about max templates limit")
	}
}

// ---------------------------------------------------------------------------
// 2. GET /v3/{domain_name}/templates — List templates
// ---------------------------------------------------------------------------

func TestListTemplates_Empty(t *testing.T) {
	router := setup(t)

	rec := doRequest(t, router, "GET", "/v3/"+testDomain+"/templates", nil)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	items, ok := body["items"].([]interface{})
	if !ok {
		t.Fatal("expected 'items' array in response")
	}
	if len(items) != 0 {
		t.Errorf("expected empty items array, got %d items", len(items))
	}

	if _, hasPaging := body["paging"]; !hasPaging {
		t.Error("expected 'paging' object in response")
	}
}

func TestListTemplates_ReturnsItemsWithPaging(t *testing.T) {
	router := setup(t)

	// Create a few templates
	createTemplate(t, router, testDomain, map[string]string{"name": "alpha"})
	createTemplate(t, router, testDomain, map[string]string{"name": "beta"})
	createTemplate(t, router, testDomain, map[string]string{"name": "gamma"})

	rec := doRequest(t, router, "GET", "/v3/"+testDomain+"/templates", nil)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	items, ok := body["items"].([]interface{})
	if !ok {
		t.Fatal("expected 'items' array in response")
	}
	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d", len(items))
	}

	// Verify item structure
	for _, item := range items {
		tmpl := item.(map[string]interface{})
		if tmpl["name"] == nil || tmpl["name"] == "" {
			t.Error("expected non-empty name in template item")
		}
		if tmpl["id"] == nil || tmpl["id"] == "" {
			t.Error("expected non-empty id in template item")
		}
		if _, hasCreatedAt := tmpl["createdAt"]; !hasCreatedAt {
			t.Error("expected createdAt field in template item")
		}
		if _, hasCreatedBy := tmpl["createdBy"]; !hasCreatedBy {
			t.Error("expected createdBy field in template item")
		}
	}

	// Verify paging object exists
	paging, ok := body["paging"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'paging' object in response")
	}
	if _, hasFirst := paging["first"]; !hasFirst {
		t.Error("expected 'first' in paging")
	}
	if _, hasLast := paging["last"]; !hasLast {
		t.Error("expected 'last' in paging")
	}
}

func TestListTemplates_WithoutActiveFlag_NoVersion(t *testing.T) {
	router := setup(t)

	createTemplate(t, router, testDomain, map[string]string{
		"name":     "with_version",
		"template": "<p>content</p>",
		"tag":      "v0",
	})

	// List without ?active=yes
	rec := doRequest(t, router, "GET", "/v3/"+testDomain+"/templates", nil)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	items := body["items"].([]interface{})
	if len(items) == 0 {
		t.Fatal("expected at least 1 item")
	}

	tmpl := items[0].(map[string]interface{})
	if _, hasVersion := tmpl["version"]; hasVersion {
		t.Error("expected no version field when ?active=yes is not set")
	}
}

func TestListTemplates_WithActiveFlag_IncludesVersion(t *testing.T) {
	router := setup(t)

	createTemplate(t, router, testDomain, map[string]string{
		"name":     "versioned",
		"template": "<p>active content</p>",
		"tag":      "v0",
		"engine":   "handlebars",
	})

	// List with ?active=yes
	rec := doRequest(t, router, "GET", "/v3/"+testDomain+"/templates?active=yes", nil)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	items := body["items"].([]interface{})
	if len(items) == 0 {
		t.Fatal("expected at least 1 item")
	}

	tmpl := items[0].(map[string]interface{})
	version, ok := tmpl["version"].(map[string]interface{})
	if !ok {
		t.Fatal("expected version object when ?active=yes is set")
	}
	if version["tag"] != "v0" {
		t.Errorf("expected version tag 'v0', got %q", version["tag"])
	}
	if version["active"] != true {
		t.Errorf("expected version to be active, got %v", version["active"])
	}
}

func TestListTemplates_Pagination(t *testing.T) {
	router := setup(t)

	// Create enough templates to require pagination
	for i := 0; i < 5; i++ {
		createTemplate(t, router, testDomain, map[string]string{
			"name": fmt.Sprintf("paginate_%02d", i),
		})
	}

	// Request with a small limit
	rec := doRequest(t, router, "GET", "/v3/"+testDomain+"/templates?limit=2", nil)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	items := body["items"].([]interface{})
	if len(items) != 2 {
		t.Errorf("expected 2 items with limit=2, got %d", len(items))
	}

	paging := body["paging"].(map[string]interface{})
	if paging["first"] == nil || paging["first"] == "" {
		t.Error("expected non-empty 'first' paging URL")
	}
	if paging["last"] == nil || paging["last"] == "" {
		t.Error("expected non-empty 'last' paging URL")
	}
	if paging["next"] == nil || paging["next"] == "" {
		t.Error("expected non-empty 'next' paging URL when more results exist")
	}
}

// ---------------------------------------------------------------------------
// 3. GET /v3/{domain_name}/templates/{name} — Get single template
// ---------------------------------------------------------------------------

func TestGetTemplate_Basic(t *testing.T) {
	router := setup(t)

	createTemplate(t, router, testDomain, map[string]string{
		"name":        "get_test",
		"description": "Template for get test",
	})

	rec := doRequest(t, router, "GET", "/v3/"+testDomain+"/templates/get_test", nil)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	tmpl, ok := body["template"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'template' object in response")
	}
	if tmpl["name"] != "get_test" {
		t.Errorf("expected name 'get_test', got %q", tmpl["name"])
	}
	if tmpl["description"] != "Template for get test" {
		t.Errorf("expected description 'Template for get test', got %q", tmpl["description"])
	}
	if tmpl["id"] == nil || tmpl["id"] == "" {
		t.Error("expected non-empty id")
	}
	if tmpl["createdAt"] == nil || tmpl["createdAt"] == "" {
		t.Error("expected non-empty createdAt")
	}
	if _, hasCreatedBy := tmpl["createdBy"]; !hasCreatedBy {
		t.Error("expected createdBy field")
	}
}

func TestGetTemplate_WithoutActiveFlag_NoVersion(t *testing.T) {
	router := setup(t)

	createTemplate(t, router, testDomain, map[string]string{
		"name":     "no_active_flag",
		"template": "<p>content</p>",
		"tag":      "v0",
	})

	rec := doRequest(t, router, "GET", "/v3/"+testDomain+"/templates/no_active_flag", nil)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	tmpl := body["template"].(map[string]interface{})
	if _, hasVersion := tmpl["version"]; hasVersion {
		t.Error("expected no version field without ?active=yes")
	}
}

func TestGetTemplate_WithActiveFlag_IncludesVersion(t *testing.T) {
	router := setup(t)

	createTemplate(t, router, testDomain, map[string]string{
		"name":     "active_flag",
		"template": "<p>active</p>",
		"tag":      "v0",
		"engine":   "handlebars",
	})

	rec := doRequest(t, router, "GET", "/v3/"+testDomain+"/templates/active_flag?active=yes", nil)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	tmpl := body["template"].(map[string]interface{})
	version, ok := tmpl["version"].(map[string]interface{})
	if !ok {
		t.Fatal("expected version object with ?active=yes")
	}
	if version["tag"] != "v0" {
		t.Errorf("expected tag 'v0', got %q", version["tag"])
	}
	if version["template"] != "<p>active</p>" {
		t.Errorf("expected template content '<p>active</p>', got %q", version["template"])
	}
	if version["engine"] != "handlebars" {
		t.Errorf("expected engine 'handlebars', got %q", version["engine"])
	}
	if version["active"] != true {
		t.Errorf("expected active=true, got %v", version["active"])
	}
}

func TestGetTemplate_NotFound(t *testing.T) {
	router := setup(t)

	rec := doRequest(t, router, "GET", "/v3/"+testDomain+"/templates/nonexistent", nil)
	assertStatus(t, rec, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// 4. PUT /v3/{domain_name}/templates/{name} — Update template
// ---------------------------------------------------------------------------

func TestUpdateTemplate_Description(t *testing.T) {
	router := setup(t)

	createTemplate(t, router, testDomain, map[string]string{
		"name":        "update_test",
		"description": "Old description",
	})

	rec := doRequest(t, router, "PUT", "/v3/"+testDomain+"/templates/update_test", map[string]string{
		"description": "New description",
	})
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	if body["message"] != "template has been updated" {
		t.Errorf("expected message 'template has been updated', got %q", body["message"])
	}

	tmpl, ok := body["template"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'template' object in response")
	}
	if tmpl["name"] != "update_test" {
		t.Errorf("expected name 'update_test', got %q", tmpl["name"])
	}

	// Verify the description was actually updated by fetching the template
	rec = doRequest(t, router, "GET", "/v3/"+testDomain+"/templates/update_test", nil)
	assertStatus(t, rec, http.StatusOK)

	decodeJSON(t, rec, &body)
	tmpl = body["template"].(map[string]interface{})
	if tmpl["description"] != "New description" {
		t.Errorf("expected updated description 'New description', got %q", tmpl["description"])
	}
}

func TestUpdateTemplate_NotFound(t *testing.T) {
	router := setup(t)

	rec := doRequest(t, router, "PUT", "/v3/"+testDomain+"/templates/nonexistent", map[string]string{
		"description": "Updated",
	})
	assertStatus(t, rec, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// 5. DELETE /v3/{domain_name}/templates/{name} — Delete template
// ---------------------------------------------------------------------------

func TestDeleteTemplate_Success(t *testing.T) {
	router := setup(t)

	createTemplate(t, router, testDomain, map[string]string{
		"name":     "delete_me",
		"template": "<p>delete this</p>",
		"tag":      "v0",
	})

	rec := doRequest(t, router, "DELETE", "/v3/"+testDomain+"/templates/delete_me", nil)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	if body["message"] != "template has been deleted" {
		t.Errorf("expected message 'template has been deleted', got %q", body["message"])
	}

	tmpl, ok := body["template"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'template' object in response")
	}
	if tmpl["name"] != "delete_me" {
		t.Errorf("expected name 'delete_me', got %q", tmpl["name"])
	}

	// Verify template no longer exists
	rec = doRequest(t, router, "GET", "/v3/"+testDomain+"/templates/delete_me", nil)
	assertStatus(t, rec, http.StatusNotFound)
}

func TestDeleteTemplate_AlsoDeletesVersions(t *testing.T) {
	router := setup(t)

	createTemplate(t, router, testDomain, map[string]string{
		"name":     "del_with_versions",
		"template": "<p>v1</p>",
		"tag":      "v1",
	})

	// Add another version
	createVersion(t, router, testDomain, "del_with_versions", map[string]string{
		"template": "<p>v2</p>",
		"tag":      "v2",
	})

	// Delete the template
	rec := doRequest(t, router, "DELETE", "/v3/"+testDomain+"/templates/del_with_versions", nil)
	assertStatus(t, rec, http.StatusOK)

	// Verify template is gone
	rec = doRequest(t, router, "GET", "/v3/"+testDomain+"/templates/del_with_versions", nil)
	assertStatus(t, rec, http.StatusNotFound)
}

func TestDeleteTemplate_NotFound(t *testing.T) {
	router := setup(t)

	rec := doRequest(t, router, "DELETE", "/v3/"+testDomain+"/templates/nonexistent", nil)
	assertStatus(t, rec, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// 6. DELETE /v3/{domain_name}/templates — Delete all templates
// ---------------------------------------------------------------------------

func TestDeleteAllTemplates(t *testing.T) {
	router := setup(t)

	createTemplate(t, router, testDomain, map[string]string{"name": "all_del_1"})
	createTemplate(t, router, testDomain, map[string]string{"name": "all_del_2"})
	createTemplate(t, router, testDomain, map[string]string{"name": "all_del_3"})

	rec := doRequest(t, router, "DELETE", "/v3/"+testDomain+"/templates", nil)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	if body["message"] != "templates have been deleted" {
		t.Errorf("expected message 'templates have been deleted', got %q", body["message"])
	}

	// Verify all templates are gone
	rec = doRequest(t, router, "GET", "/v3/"+testDomain+"/templates", nil)
	assertStatus(t, rec, http.StatusOK)

	decodeJSON(t, rec, &body)
	items := body["items"].([]interface{})
	if len(items) != 0 {
		t.Errorf("expected 0 templates after delete all, got %d", len(items))
	}
}

// =========================================================================
// Version Management Tests
// =========================================================================

// ---------------------------------------------------------------------------
// 7. POST /v3/{domain_name}/templates/{name}/versions — Create version
// ---------------------------------------------------------------------------

func TestCreateVersion_Basic(t *testing.T) {
	router := setup(t)

	createTemplate(t, router, testDomain, map[string]string{"name": "ver_create"})

	rec := createVersion(t, router, testDomain, "ver_create", map[string]string{
		"template": "<p>Version 1</p>",
		"tag":      "v1",
		"engine":   "handlebars",
		"comment":  "Initial version",
	})
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	if body["message"] != "new version of the template has been stored" {
		t.Errorf("expected message 'new version of the template has been stored', got %q", body["message"])
	}

	tmpl, ok := body["template"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'template' object in response")
	}
	if tmpl["name"] != "ver_create" {
		t.Errorf("expected template name 'ver_create', got %q", tmpl["name"])
	}

	version, ok := tmpl["version"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'version' object in template response")
	}
	if version["tag"] != "v1" {
		t.Errorf("expected tag 'v1', got %q", version["tag"])
	}
	if version["template"] != "<p>Version 1</p>" {
		t.Errorf("expected template content, got %q", version["template"])
	}
	if version["engine"] != "handlebars" {
		t.Errorf("expected engine 'handlebars', got %q", version["engine"])
	}
	if version["comment"] != "Initial version" {
		t.Errorf("expected comment 'Initial version', got %q", version["comment"])
	}
	if _, hasMjml := version["mjml"]; !hasMjml {
		t.Error("expected mjml field to be present")
	}
	if version["id"] == nil || version["id"] == "" {
		t.Error("expected non-empty version id")
	}
	if version["createdAt"] == nil || version["createdAt"] == "" {
		t.Error("expected non-empty createdAt")
	}
}

func TestCreateVersion_FirstVersionAutoActive(t *testing.T) {
	router := setup(t)

	createTemplate(t, router, testDomain, map[string]string{"name": "auto_active"})

	// Create first version without specifying active
	rec := createVersion(t, router, testDomain, "auto_active", map[string]string{
		"template": "<p>first</p>",
		"tag":      "v1",
	})
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	tmpl := body["template"].(map[string]interface{})
	version := tmpl["version"].(map[string]interface{})
	if version["active"] != true {
		t.Errorf("expected first version to be auto-active, got %v", version["active"])
	}
}

func TestCreateVersion_SetActiveDeactivatesPrevious(t *testing.T) {
	router := setup(t)

	createTemplate(t, router, testDomain, map[string]string{
		"name":     "active_switch",
		"template": "<p>v1</p>",
		"tag":      "v1",
	})

	// Create second version with active=yes
	rec := createVersion(t, router, testDomain, "active_switch", map[string]string{
		"template": "<p>v2</p>",
		"tag":      "v2",
		"active":   "yes",
	})
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	tmpl := body["template"].(map[string]interface{})
	version := tmpl["version"].(map[string]interface{})
	if version["active"] != true {
		t.Errorf("expected new version to be active, got %v", version["active"])
	}

	// Verify the first version is no longer active
	rec = doRequest(t, router, "GET", "/v3/"+testDomain+"/templates/active_switch/versions/v1", nil)
	assertStatus(t, rec, http.StatusOK)

	decodeJSON(t, rec, &body)
	tmpl = body["template"].(map[string]interface{})
	v1 := tmpl["version"].(map[string]interface{})
	if v1["active"] != false {
		t.Errorf("expected v1 to be deactivated after setting v2 as active, got %v", v1["active"])
	}
}

func TestCreateVersion_TagLowercased(t *testing.T) {
	router := setup(t)

	createTemplate(t, router, testDomain, map[string]string{"name": "tag_lower"})

	rec := createVersion(t, router, testDomain, "tag_lower", map[string]string{
		"template": "<p>content</p>",
		"tag":      "V1",
	})
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	tmpl := body["template"].(map[string]interface{})
	version := tmpl["version"].(map[string]interface{})
	if version["tag"] != "v1" {
		t.Errorf("expected tag to be lowercased to 'v1', got %q", version["tag"])
	}

	// Verify it can be retrieved with the lowercased tag
	rec = doRequest(t, router, "GET", "/v3/"+testDomain+"/templates/tag_lower/versions/v1", nil)
	assertStatus(t, rec, http.StatusOK)
}

func TestCreateVersion_MissingTemplate(t *testing.T) {
	router := setup(t)

	createTemplate(t, router, testDomain, map[string]string{"name": "missing_tmpl"})

	rec := createVersion(t, router, testDomain, "missing_tmpl", map[string]string{
		"tag": "v1",
		// no "template" field
	})
	assertStatus(t, rec, http.StatusBadRequest)
}

func TestCreateVersion_MissingTag(t *testing.T) {
	router := setup(t)

	createTemplate(t, router, testDomain, map[string]string{"name": "missing_tag"})

	rec := createVersion(t, router, testDomain, "missing_tag", map[string]string{
		"template": "<p>content</p>",
		// no "tag" field
	})
	assertStatus(t, rec, http.StatusBadRequest)
}

func TestCreateVersion_DuplicateTag(t *testing.T) {
	router := setup(t)

	createTemplate(t, router, testDomain, map[string]string{"name": "dup_tag"})

	rec := createVersion(t, router, testDomain, "dup_tag", map[string]string{
		"template": "<p>first</p>",
		"tag":      "v1",
	})
	assertStatus(t, rec, http.StatusOK)

	// Try to create another version with the same tag
	rec = createVersion(t, router, testDomain, "dup_tag", map[string]string{
		"template": "<p>second</p>",
		"tag":      "v1",
	})
	assertStatus(t, rec, http.StatusBadRequest)
}

func TestCreateVersion_MaxLimit(t *testing.T) {
	router := setup(t)

	createTemplate(t, router, testDomain, map[string]string{"name": "max_versions"})

	// Create 40 versions (the maximum)
	for i := 0; i < 40; i++ {
		rec := createVersion(t, router, testDomain, "max_versions", map[string]string{
			"template": fmt.Sprintf("<p>Version %d</p>", i),
			"tag":      fmt.Sprintf("v%d", i),
		})
		assertStatus(t, rec, http.StatusOK)
	}

	// The 41st should fail
	rec := createVersion(t, router, testDomain, "max_versions", map[string]string{
		"template": "<p>One too many</p>",
		"tag":      "v_overflow",
	})
	assertStatus(t, rec, http.StatusBadRequest)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	msg, _ := body["message"].(string)
	if msg == "" {
		t.Error("expected error message about max versions limit")
	}
}

// ---------------------------------------------------------------------------
// 8. GET /v3/{domain_name}/templates/{name}/versions — List versions
// ---------------------------------------------------------------------------

func TestListVersions_ReturnsTemplateWithVersionsAndPaging(t *testing.T) {
	router := setup(t)

	createTemplate(t, router, testDomain, map[string]string{
		"name":        "list_versions",
		"description": "Template with versions",
		"template":    "<p>v0</p>",
		"tag":         "v0",
	})

	createVersion(t, router, testDomain, "list_versions", map[string]string{
		"template": "<p>v1</p>",
		"tag":      "v1",
	})

	rec := doRequest(t, router, "GET", "/v3/"+testDomain+"/templates/list_versions/versions", nil)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	tmpl, ok := body["template"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'template' object in response")
	}
	if tmpl["name"] != "list_versions" {
		t.Errorf("expected name 'list_versions', got %q", tmpl["name"])
	}
	if tmpl["id"] == nil || tmpl["id"] == "" {
		t.Error("expected non-empty template id")
	}
	if _, hasCreatedAt := tmpl["createdAt"]; !hasCreatedAt {
		t.Error("expected createdAt field")
	}
	if _, hasCreatedBy := tmpl["createdBy"]; !hasCreatedBy {
		t.Error("expected createdBy field")
	}

	versions, ok := tmpl["versions"].([]interface{})
	if !ok {
		t.Fatal("expected 'versions' array in template object")
	}
	if len(versions) != 2 {
		t.Errorf("expected 2 versions, got %d", len(versions))
	}

	// Verify paging
	if _, hasPaging := body["paging"]; !hasPaging {
		t.Error("expected 'paging' object in response")
	}
}

func TestListVersions_DoesNotIncludeTemplateContent(t *testing.T) {
	router := setup(t)

	createTemplate(t, router, testDomain, map[string]string{
		"name":     "no_content_in_list",
		"template": "<p>should not appear in list</p>",
		"tag":      "v0",
		"engine":   "handlebars",
	})

	rec := doRequest(t, router, "GET", "/v3/"+testDomain+"/templates/no_content_in_list/versions", nil)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	tmpl := body["template"].(map[string]interface{})
	versions := tmpl["versions"].([]interface{})
	if len(versions) == 0 {
		t.Fatal("expected at least 1 version")
	}

	ver := versions[0].(map[string]interface{})

	// Version in list should NOT include template content
	if _, hasTemplate := ver["template"]; hasTemplate {
		t.Error("expected version in list to NOT include 'template' content field")
	}

	// Should include metadata
	if ver["tag"] == nil || ver["tag"] == "" {
		t.Error("expected tag field")
	}
	if ver["engine"] == nil {
		t.Error("expected engine field")
	}
	if _, hasMjml := ver["mjml"]; !hasMjml {
		t.Error("expected mjml field")
	}
	if ver["id"] == nil || ver["id"] == "" {
		t.Error("expected id field")
	}
	if _, hasCreatedAt := ver["createdAt"]; !hasCreatedAt {
		t.Error("expected createdAt field")
	}
	if ver["active"] == nil {
		t.Error("expected active field")
	}
}

// ---------------------------------------------------------------------------
// 9. GET /v3/{domain_name}/templates/{name}/versions/{tag} — Get version
// ---------------------------------------------------------------------------

func TestGetVersion_ReturnsFullVersion(t *testing.T) {
	router := setup(t)

	createTemplate(t, router, testDomain, map[string]string{
		"name":     "get_version",
		"template": "<p>Get me</p>",
		"tag":      "v0",
		"engine":   "handlebars",
		"comment":  "A comment",
	})

	rec := doRequest(t, router, "GET", "/v3/"+testDomain+"/templates/get_version/versions/v0", nil)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	tmpl, ok := body["template"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'template' object in response")
	}
	if tmpl["name"] != "get_version" {
		t.Errorf("expected name 'get_version', got %q", tmpl["name"])
	}

	version, ok := tmpl["version"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'version' object in template response")
	}
	if version["tag"] != "v0" {
		t.Errorf("expected tag 'v0', got %q", version["tag"])
	}
	if version["template"] != "<p>Get me</p>" {
		t.Errorf("expected template content '<p>Get me</p>', got %q", version["template"])
	}
	if version["engine"] != "handlebars" {
		t.Errorf("expected engine 'handlebars', got %q", version["engine"])
	}
	if _, hasMjml := version["mjml"]; !hasMjml {
		t.Error("expected mjml field")
	}
	if version["comment"] != "A comment" {
		t.Errorf("expected comment 'A comment', got %q", version["comment"])
	}
	if version["active"] == nil {
		t.Error("expected active field")
	}
	if version["id"] == nil || version["id"] == "" {
		t.Error("expected non-empty id")
	}
	if version["createdAt"] == nil || version["createdAt"] == "" {
		t.Error("expected non-empty createdAt")
	}
}

func TestGetVersion_NotFound(t *testing.T) {
	router := setup(t)

	createTemplate(t, router, testDomain, map[string]string{"name": "ver_404"})

	rec := doRequest(t, router, "GET", "/v3/"+testDomain+"/templates/ver_404/versions/nonexistent", nil)
	assertStatus(t, rec, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// 10. PUT /v3/{domain_name}/templates/{name}/versions/{tag} — Update version
// ---------------------------------------------------------------------------

func TestUpdateVersion_Content(t *testing.T) {
	router := setup(t)

	createTemplate(t, router, testDomain, map[string]string{
		"name":     "update_version",
		"template": "<p>Old</p>",
		"tag":      "v0",
	})

	rec := doRequest(t, router, "PUT", "/v3/"+testDomain+"/templates/update_version/versions/v0", map[string]string{
		"template": "<p>New</p>",
	})
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	if body["message"] != "version has been updated" {
		t.Errorf("expected message 'version has been updated', got %q", body["message"])
	}

	tmpl, ok := body["template"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'template' object in response")
	}

	version, ok := tmpl["version"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'version' object in template response")
	}
	if version["tag"] != "v0" {
		t.Errorf("expected tag 'v0', got %q", version["tag"])
	}

	// Verify the content was actually updated
	rec = doRequest(t, router, "GET", "/v3/"+testDomain+"/templates/update_version/versions/v0", nil)
	assertStatus(t, rec, http.StatusOK)

	decodeJSON(t, rec, &body)
	tmpl = body["template"].(map[string]interface{})
	ver := tmpl["version"].(map[string]interface{})
	if ver["template"] != "<p>New</p>" {
		t.Errorf("expected updated content '<p>New</p>', got %q", ver["template"])
	}
}

func TestUpdateVersion_SetActiveDeactivatesOthers(t *testing.T) {
	router := setup(t)

	createTemplate(t, router, testDomain, map[string]string{
		"name":     "update_active",
		"template": "<p>v1</p>",
		"tag":      "v1",
	})

	// Create v2, not active
	createVersion(t, router, testDomain, "update_active", map[string]string{
		"template": "<p>v2</p>",
		"tag":      "v2",
	})

	// Update v2 to be active
	rec := doRequest(t, router, "PUT", "/v3/"+testDomain+"/templates/update_active/versions/v2", map[string]string{
		"active": "yes",
	})
	assertStatus(t, rec, http.StatusOK)

	// Verify v1 is no longer active
	rec = doRequest(t, router, "GET", "/v3/"+testDomain+"/templates/update_active/versions/v1", nil)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)
	tmpl := body["template"].(map[string]interface{})
	v1 := tmpl["version"].(map[string]interface{})
	if v1["active"] != false {
		t.Errorf("expected v1 to be deactivated, got %v", v1["active"])
	}

	// Verify v2 is active
	rec = doRequest(t, router, "GET", "/v3/"+testDomain+"/templates/update_active/versions/v2", nil)
	assertStatus(t, rec, http.StatusOK)

	decodeJSON(t, rec, &body)
	tmpl = body["template"].(map[string]interface{})
	v2 := tmpl["version"].(map[string]interface{})
	if v2["active"] != true {
		t.Errorf("expected v2 to be active, got %v", v2["active"])
	}
}

func TestUpdateVersion_NotFound(t *testing.T) {
	router := setup(t)

	createTemplate(t, router, testDomain, map[string]string{"name": "update_ver_404"})

	rec := doRequest(t, router, "PUT", "/v3/"+testDomain+"/templates/update_ver_404/versions/nonexistent", map[string]string{
		"template": "<p>New</p>",
	})
	assertStatus(t, rec, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// 11. DELETE /v3/{domain_name}/templates/{name}/versions/{tag} — Delete version
// ---------------------------------------------------------------------------

func TestDeleteVersion_Success(t *testing.T) {
	router := setup(t)

	createTemplate(t, router, testDomain, map[string]string{
		"name":     "del_version",
		"template": "<p>v0</p>",
		"tag":      "v0",
	})

	rec := doRequest(t, router, "DELETE", "/v3/"+testDomain+"/templates/del_version/versions/v0", nil)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	if body["message"] != "version has been deleted" {
		t.Errorf("expected message 'version has been deleted', got %q", body["message"])
	}

	tmpl, ok := body["template"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'template' object in response")
	}

	version, ok := tmpl["version"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'version' object in template response")
	}
	if version["tag"] != "v0" {
		t.Errorf("expected tag 'v0', got %q", version["tag"])
	}

	// Verify version no longer exists
	rec = doRequest(t, router, "GET", "/v3/"+testDomain+"/templates/del_version/versions/v0", nil)
	assertStatus(t, rec, http.StatusNotFound)
}

func TestDeleteVersion_NotFound(t *testing.T) {
	router := setup(t)

	createTemplate(t, router, testDomain, map[string]string{"name": "del_ver_404"})

	rec := doRequest(t, router, "DELETE", "/v3/"+testDomain+"/templates/del_ver_404/versions/nonexistent", nil)
	assertStatus(t, rec, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// 12. PUT /v3/{domain_name}/templates/{name}/versions/{tag}/copy/{new_tag}
//     — Copy version
// ---------------------------------------------------------------------------

func TestCopyVersion_Success(t *testing.T) {
	router := setup(t)

	createTemplate(t, router, testDomain, map[string]string{
		"name":     "copy_test",
		"template": "<p>Original content</p>",
		"tag":      "v1",
		"engine":   "handlebars",
		"comment":  "Original comment",
	})

	rec := doRequest(t, router, "PUT", "/v3/"+testDomain+"/templates/copy_test/versions/v1/copy/v2", nil)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	if body["message"] != "version has been copied" {
		t.Errorf("expected message 'version has been copied', got %q", body["message"])
	}

	version, ok := body["version"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'version' object in response")
	}
	if version["tag"] != "v2" {
		t.Errorf("expected new tag 'v2', got %q", version["tag"])
	}
	if version["template"] != "<p>Original content</p>" {
		t.Errorf("expected copied content, got %q", version["template"])
	}
	if version["engine"] != "handlebars" {
		t.Errorf("expected engine 'handlebars', got %q", version["engine"])
	}
	if version["id"] == nil || version["id"] == "" {
		t.Error("expected non-empty version id")
	}
	if _, hasMjml := version["mjml"]; !hasMjml {
		t.Error("expected mjml field")
	}
	// Copied version should not be active
	if version["active"] != false {
		t.Errorf("expected copied version to not be active, got %v", version["active"])
	}

	// Verify template reference in response
	tmplRef, ok := body["template"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'template' object in response")
	}
	if tmplRef["tag"] != "v2" {
		t.Errorf("expected template.tag 'v2', got %q", tmplRef["tag"])
	}

	// Verify the copied version can be fetched
	rec = doRequest(t, router, "GET", "/v3/"+testDomain+"/templates/copy_test/versions/v2", nil)
	assertStatus(t, rec, http.StatusOK)

	decodeJSON(t, rec, &body)
	tmpl := body["template"].(map[string]interface{})
	v2 := tmpl["version"].(map[string]interface{})
	if v2["template"] != "<p>Original content</p>" {
		t.Errorf("expected copied content, got %q", v2["template"])
	}
}

func TestCopyVersion_OverwritesExistingTag(t *testing.T) {
	router := setup(t)

	createTemplate(t, router, testDomain, map[string]string{
		"name":     "copy_overwrite",
		"template": "<p>v1 content</p>",
		"tag":      "v1",
	})

	// Create v2
	createVersion(t, router, testDomain, "copy_overwrite", map[string]string{
		"template": "<p>v2 content</p>",
		"tag":      "v2",
	})

	// Copy v1 to v2 (overwrite)
	rec := doRequest(t, router, "PUT", "/v3/"+testDomain+"/templates/copy_overwrite/versions/v1/copy/v2", nil)
	assertStatus(t, rec, http.StatusOK)

	// Verify v2 now has v1's content
	rec = doRequest(t, router, "GET", "/v3/"+testDomain+"/templates/copy_overwrite/versions/v2", nil)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	tmpl := body["template"].(map[string]interface{})
	v2 := tmpl["version"].(map[string]interface{})
	if v2["template"] != "<p>v1 content</p>" {
		t.Errorf("expected v2 to have v1's content after copy, got %q", v2["template"])
	}
}

func TestCopyVersion_SourceNotFound(t *testing.T) {
	router := setup(t)

	createTemplate(t, router, testDomain, map[string]string{"name": "copy_404"})

	rec := doRequest(t, router, "PUT", "/v3/"+testDomain+"/templates/copy_404/versions/nonexistent/copy/v2", nil)
	assertStatus(t, rec, http.StatusNotFound)
}

// =========================================================================
// Edge Case / Integration Tests
// =========================================================================

func TestCreateTemplate_DuplicateNameCaseInsensitive(t *testing.T) {
	router := setup(t)

	rec := createTemplate(t, router, testDomain, map[string]string{
		"name": "CaseTest",
	})
	assertStatus(t, rec, http.StatusOK)

	// Try creating with different case — should fail as duplicate
	rec = createTemplate(t, router, testDomain, map[string]string{
		"name": "casetest",
	})
	assertStatus(t, rec, http.StatusBadRequest)
	assertMessage(t, rec, "template with name 'casetest' already exists")
}

func TestCreateTemplate_WithVersionDefaultEngine(t *testing.T) {
	router := setup(t)

	// Create template with version but without specifying engine
	rec := createTemplate(t, router, testDomain, map[string]string{
		"name":     "default_engine",
		"template": "<p>Content</p>",
		"tag":      "v0",
	})
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	tmpl := body["template"].(map[string]interface{})
	version := tmpl["version"].(map[string]interface{})

	// Engine should default to "handlebars"
	if version["engine"] != "handlebars" {
		t.Errorf("expected default engine 'handlebars', got %q", version["engine"])
	}
}

func TestTemplateIsolationBetweenDomains(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	createDomain(t, router, "domain-a.example.com")
	createDomain(t, router, "domain-b.example.com")

	// Create template in domain A
	rec := createTemplate(t, router, "domain-a.example.com", map[string]string{
		"name": "shared_name",
	})
	assertStatus(t, rec, http.StatusOK)

	// Same name in domain B should work
	rec = createTemplate(t, router, "domain-b.example.com", map[string]string{
		"name": "shared_name",
	})
	assertStatus(t, rec, http.StatusOK)

	// Verify domain A has its template
	rec = doRequest(t, router, "GET", "/v3/domain-a.example.com/templates/shared_name", nil)
	assertStatus(t, rec, http.StatusOK)

	// Verify domain B has its template
	rec = doRequest(t, router, "GET", "/v3/domain-b.example.com/templates/shared_name", nil)
	assertStatus(t, rec, http.StatusOK)

	// Deleting from domain A should not affect domain B
	rec = doRequest(t, router, "DELETE", "/v3/domain-a.example.com/templates/shared_name", nil)
	assertStatus(t, rec, http.StatusOK)

	rec = doRequest(t, router, "GET", "/v3/domain-b.example.com/templates/shared_name", nil)
	assertStatus(t, rec, http.StatusOK)
}

func TestDeleteAllTemplates_DoesNotAffectOtherDomains(t *testing.T) {
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	createDomain(t, router, "domain-x.example.com")
	createDomain(t, router, "domain-y.example.com")

	createTemplate(t, router, "domain-x.example.com", map[string]string{"name": "tmpl_x"})
	createTemplate(t, router, "domain-y.example.com", map[string]string{"name": "tmpl_y"})

	// Delete all templates for domain X
	rec := doRequest(t, router, "DELETE", "/v3/domain-x.example.com/templates", nil)
	assertStatus(t, rec, http.StatusOK)

	// Domain Y templates should still exist
	rec = doRequest(t, router, "GET", "/v3/domain-y.example.com/templates", nil)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)
	items := body["items"].([]interface{})
	if len(items) != 1 {
		t.Errorf("expected 1 template in domain-y, got %d", len(items))
	}
}

func TestCreateVersion_TemplateNotFound(t *testing.T) {
	router := setup(t)

	rec := createVersion(t, router, testDomain, "nonexistent_template", map[string]string{
		"template": "<p>content</p>",
		"tag":      "v1",
	})

	// Should return 404 since the template does not exist
	assertStatus(t, rec, http.StatusNotFound)
}

func TestGetVersion_WithHeaders(t *testing.T) {
	router := setup(t)

	createTemplate(t, router, testDomain, map[string]string{
		"name":     "headers_test",
		"template": "<p>With headers</p>",
		"tag":      "v0",
	})

	// Update version with headers
	rec := doRequest(t, router, "PUT", "/v3/"+testDomain+"/templates/headers_test/versions/v0", map[string]string{
		"headers": `{"Subject": "Test Subject"}`,
	})
	assertStatus(t, rec, http.StatusOK)

	// Fetch the version and verify headers
	rec = doRequest(t, router, "GET", "/v3/"+testDomain+"/templates/headers_test/versions/v0", nil)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	tmpl := body["template"].(map[string]interface{})
	version := tmpl["version"].(map[string]interface{})

	headers, ok := version["headers"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'headers' object in version response")
	}
	if headers["Subject"] != "Test Subject" {
		t.Errorf("expected header Subject='Test Subject', got %q", headers["Subject"])
	}
}

func TestListVersions_Pagination(t *testing.T) {
	router := setup(t)

	createTemplate(t, router, testDomain, map[string]string{"name": "ver_paginate"})

	for i := 0; i < 5; i++ {
		createVersion(t, router, testDomain, "ver_paginate", map[string]string{
			"template": fmt.Sprintf("<p>v%d</p>", i),
			"tag":      fmt.Sprintf("v%d", i),
		})
	}

	rec := doRequest(t, router, "GET", "/v3/"+testDomain+"/templates/ver_paginate/versions?limit=2", nil)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	tmpl := body["template"].(map[string]interface{})
	versions := tmpl["versions"].([]interface{})
	if len(versions) != 2 {
		t.Errorf("expected 2 versions with limit=2, got %d", len(versions))
	}

	paging := body["paging"].(map[string]interface{})
	if paging["first"] == nil || paging["first"] == "" {
		t.Error("expected non-empty 'first' paging URL")
	}
	if paging["last"] == nil || paging["last"] == "" {
		t.Error("expected non-empty 'last' paging URL")
	}
	if paging["next"] == nil || paging["next"] == "" {
		t.Error("expected non-empty 'next' paging URL when more results exist")
	}
}

func TestUpdateVersion_Comment(t *testing.T) {
	router := setup(t)

	createTemplate(t, router, testDomain, map[string]string{
		"name":     "update_comment",
		"template": "<p>content</p>",
		"tag":      "v0",
		"comment":  "old comment",
	})

	rec := doRequest(t, router, "PUT", "/v3/"+testDomain+"/templates/update_comment/versions/v0", map[string]string{
		"comment": "new comment",
	})
	assertStatus(t, rec, http.StatusOK)

	// Verify the comment was updated
	rec = doRequest(t, router, "GET", "/v3/"+testDomain+"/templates/update_comment/versions/v0", nil)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	tmpl := body["template"].(map[string]interface{})
	ver := tmpl["version"].(map[string]interface{})
	if ver["comment"] != "new comment" {
		t.Errorf("expected comment 'new comment', got %q", ver["comment"])
	}
}

func TestMultipleVersions_ActiveManagement(t *testing.T) {
	router := setup(t)

	// Create template with first version (auto-active)
	createTemplate(t, router, testDomain, map[string]string{
		"name":     "multi_active",
		"template": "<p>v1</p>",
		"tag":      "v1",
	})

	// Create v2 without active flag — should NOT auto-become active
	rec := createVersion(t, router, testDomain, "multi_active", map[string]string{
		"template": "<p>v2</p>",
		"tag":      "v2",
	})
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)
	tmpl := body["template"].(map[string]interface{})
	v2 := tmpl["version"].(map[string]interface{})
	if v2["active"] != false {
		t.Errorf("expected v2 to NOT be active (only first version auto-activates), got %v", v2["active"])
	}

	// v1 should still be active
	rec = doRequest(t, router, "GET", "/v3/"+testDomain+"/templates/multi_active/versions/v1", nil)
	assertStatus(t, rec, http.StatusOK)

	decodeJSON(t, rec, &body)
	tmpl = body["template"].(map[string]interface{})
	v1 := tmpl["version"].(map[string]interface{})
	if v1["active"] != true {
		t.Errorf("expected v1 to still be active, got %v", v1["active"])
	}

	// Create v3 with active=yes — should deactivate v1
	rec = createVersion(t, router, testDomain, "multi_active", map[string]string{
		"template": "<p>v3</p>",
		"tag":      "v3",
		"active":   "yes",
	})
	assertStatus(t, rec, http.StatusOK)

	// Verify v1 is now deactivated
	rec = doRequest(t, router, "GET", "/v3/"+testDomain+"/templates/multi_active/versions/v1", nil)
	assertStatus(t, rec, http.StatusOK)

	decodeJSON(t, rec, &body)
	tmpl = body["template"].(map[string]interface{})
	v1 = tmpl["version"].(map[string]interface{})
	if v1["active"] != false {
		t.Errorf("expected v1 to be deactivated after v3 set as active, got %v", v1["active"])
	}

	// Verify v3 is active
	rec = doRequest(t, router, "GET", "/v3/"+testDomain+"/templates/multi_active/versions/v3", nil)
	assertStatus(t, rec, http.StatusOK)

	decodeJSON(t, rec, &body)
	tmpl = body["template"].(map[string]interface{})
	v3 := tmpl["version"].(map[string]interface{})
	if v3["active"] != true {
		t.Errorf("expected v3 to be active, got %v", v3["active"])
	}
}

func TestGetTemplate_ActiveVersionAfterMultipleCreations(t *testing.T) {
	router := setup(t)

	// Create template with initial v1 (auto-active)
	createTemplate(t, router, testDomain, map[string]string{
		"name":     "active_tracking",
		"template": "<p>v1</p>",
		"tag":      "v1",
	})

	// Create v2 with active=yes
	createVersion(t, router, testDomain, "active_tracking", map[string]string{
		"template": "<p>v2</p>",
		"tag":      "v2",
		"active":   "yes",
	})

	// Get template with ?active=yes — should return v2
	rec := doRequest(t, router, "GET", "/v3/"+testDomain+"/templates/active_tracking?active=yes", nil)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	tmpl := body["template"].(map[string]interface{})
	version := tmpl["version"].(map[string]interface{})
	if version["tag"] != "v2" {
		t.Errorf("expected active version tag 'v2', got %q", version["tag"])
	}
	if version["template"] != "<p>v2</p>" {
		t.Errorf("expected active version content '<p>v2</p>', got %q", version["template"])
	}
}

func TestCopyVersion_TagLowercased(t *testing.T) {
	router := setup(t)

	createTemplate(t, router, testDomain, map[string]string{
		"name":     "copy_lower",
		"template": "<p>content</p>",
		"tag":      "v1",
	})

	// The new_tag in the URL is used as-is, but should be lowercased
	rec := doRequest(t, router, "PUT", "/v3/"+testDomain+"/templates/copy_lower/versions/v1/copy/V2", nil)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	version := body["version"].(map[string]interface{})
	if version["tag"] != "v2" {
		t.Errorf("expected copied tag to be lowercased to 'v2', got %q", version["tag"])
	}
}
