package domain_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bethmaloney/mailgun-mock-api/internal/domain"
	"github.com/bethmaloney/mailgun-mock-api/internal/mock"
	"github.com/go-chi/chi/v5"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// Tracking Test Helpers
// ---------------------------------------------------------------------------

// setupTrackingTestDB creates an in-memory SQLite database for tracking tests
// with Domain and DNSRecord tables migrated.
func setupTrackingTestDB(t *testing.T) *gorm.DB {
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

// setupTrackingRouter creates a chi router with domain CRUD and tracking routes.
func setupTrackingRouter(db *gorm.DB, cfg *mock.MockConfig) http.Handler {
	h := domain.NewHandlers(db, cfg)
	r := chi.NewRouter()
	// Domain CRUD routes needed to create test domains.
	r.Route("/v4/domains", func(r chi.Router) {
		r.Post("/", h.CreateDomain)
	})
	// Tracking routes under /v3/domains.
	r.Route("/v3/domains/{name}/tracking", func(r chi.Router) {
		r.Get("/", h.GetTracking)
		r.Put("/open", h.UpdateOpenTracking)
		r.Put("/click", h.UpdateClickTracking)
		r.Put("/unsubscribe", h.UpdateUnsubscribeTracking)
	})
	return r
}

// createTestDomain is a convenience helper that creates a domain via the
// multipart POST endpoint and fatals if it does not succeed.
func createTestDomain(t *testing.T, router http.Handler, name string) {
	t.Helper()
	rec := createDomainViaMultipart(t, router, map[string]string{"name": name})
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to create test domain %q: status %d, body %s", name, rec.Code, rec.Body.String())
	}
}

// doRequest executes a request against the router and returns the recorder.
func doRequest(t *testing.T, router http.Handler, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// ---------------------------------------------------------------------------
// Tracking Response Structs
// ---------------------------------------------------------------------------

type trackingResponse struct {
	Tracking trackingJSON `json:"tracking"`
}

type trackingJSON struct {
	Open        openTrackingJSON        `json:"open"`
	Click       clickTrackingJSON       `json:"click"`
	Unsubscribe unsubscribeTrackingJSON `json:"unsubscribe"`
}

type openTrackingJSON struct {
	Active        bool `json:"active"`
	PlaceAtTheTop bool `json:"place_at_the_top"`
}

type clickTrackingJSON struct {
	Active interface{} `json:"active"` // can be bool or string "htmlonly"
}

type unsubscribeTrackingJSON struct {
	Active     bool   `json:"active"`
	HTMLFooter string `json:"html_footer"`
	TextFooter string `json:"text_footer"`
}

type updateOpenTrackingResponse struct {
	Message string           `json:"message"`
	Open    openTrackingJSON `json:"open"`
}

type updateClickTrackingResponse struct {
	Message string            `json:"message"`
	Click   clickTrackingJSON `json:"click"`
}

type updateUnsubscribeTrackingResponse struct {
	Message     string                  `json:"message"`
	Unsubscribe unsubscribeTrackingJSON `json:"unsubscribe"`
}

// ---------------------------------------------------------------------------
// GET /v3/domains/{name}/tracking — Get Tracking Settings
// ---------------------------------------------------------------------------

func TestGetTracking_DefaultSettings(t *testing.T) {
	db := setupTrackingTestDB(t)
	cfg := defaultConfig()
	router := setupTrackingRouter(db, cfg)

	// Create a domain first.
	createTestDomain(t, router, "example.com")

	// Request tracking settings.
	req := httptest.NewRequest(http.MethodGet, "/v3/domains/example.com/tracking", nil)
	rec := doRequest(t, router, req)

	t.Run("returns 200 status", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp trackingResponse
	decodeJSON(t, rec, &resp)

	t.Run("open tracking defaults to active", func(t *testing.T) {
		if !resp.Tracking.Open.Active {
			t.Errorf("expected open.active=true, got %v", resp.Tracking.Open.Active)
		}
	})

	t.Run("open place_at_the_top defaults to false", func(t *testing.T) {
		if resp.Tracking.Open.PlaceAtTheTop {
			t.Errorf("expected open.place_at_the_top=false, got %v", resp.Tracking.Open.PlaceAtTheTop)
		}
	})

	t.Run("click tracking defaults to active", func(t *testing.T) {
		active, ok := resp.Tracking.Click.Active.(bool)
		if !ok {
			t.Fatalf("expected click.active to be bool, got %T", resp.Tracking.Click.Active)
		}
		if !active {
			t.Errorf("expected click.active=true, got %v", active)
		}
	})

	t.Run("unsubscribe tracking defaults to active", func(t *testing.T) {
		if !resp.Tracking.Unsubscribe.Active {
			t.Errorf("expected unsubscribe.active=true, got %v", resp.Tracking.Unsubscribe.Active)
		}
	})

	t.Run("unsubscribe has default html_footer", func(t *testing.T) {
		if resp.Tracking.Unsubscribe.HTMLFooter == "" {
			t.Error("expected non-empty default html_footer")
		}
	})

	t.Run("unsubscribe has default text_footer", func(t *testing.T) {
		if resp.Tracking.Unsubscribe.TextFooter == "" {
			t.Error("expected non-empty default text_footer")
		}
	})
}

func TestGetTracking_NonexistentDomain(t *testing.T) {
	db := setupTrackingTestDB(t)
	cfg := defaultConfig()
	router := setupTrackingRouter(db, cfg)

	req := httptest.NewRequest(http.MethodGet, "/v3/domains/nonexistent.com/tracking", nil)
	rec := doRequest(t, router, req)

	t.Run("returns 404 status", func(t *testing.T) {
		if rec.Code != http.StatusNotFound {
			t.Errorf("expected status 404, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp errorResponse
	decodeJSON(t, rec, &resp)

	t.Run("returns domain not found message", func(t *testing.T) {
		if resp.Message != "Domain not found" {
			t.Errorf("expected message %q, got %q", "Domain not found", resp.Message)
		}
	})
}

func TestGetTracking_ReflectsUpdatedSettings(t *testing.T) {
	db := setupTrackingTestDB(t)
	cfg := defaultConfig()
	router := setupTrackingRouter(db, cfg)

	createTestDomain(t, router, "updated.com")

	// Disable open tracking.
	updateReq := newMultipartRequest(t, http.MethodPut, "/v3/domains/updated.com/tracking/open", map[string]string{
		"active": "false",
	})
	doRequest(t, router, updateReq)

	// Now GET tracking settings and verify the update is reflected.
	getReq := httptest.NewRequest(http.MethodGet, "/v3/domains/updated.com/tracking", nil)
	getRec := doRequest(t, router, getReq)

	t.Run("returns 200 status", func(t *testing.T) {
		if getRec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d (body: %s)", getRec.Code, getRec.Body.String())
		}
	})

	var resp trackingResponse
	decodeJSON(t, getRec, &resp)

	t.Run("open tracking reflects disabled state", func(t *testing.T) {
		if resp.Tracking.Open.Active {
			t.Errorf("expected open.active=false after update, got %v", resp.Tracking.Open.Active)
		}
	})

	t.Run("click tracking remains at default", func(t *testing.T) {
		active, ok := resp.Tracking.Click.Active.(bool)
		if !ok {
			t.Fatalf("expected click.active to be bool, got %T", resp.Tracking.Click.Active)
		}
		if !active {
			t.Errorf("expected click.active=true (unchanged), got %v", active)
		}
	})

	t.Run("unsubscribe tracking remains at default", func(t *testing.T) {
		if !resp.Tracking.Unsubscribe.Active {
			t.Errorf("expected unsubscribe.active=true (unchanged), got %v", resp.Tracking.Unsubscribe.Active)
		}
	})
}

// ---------------------------------------------------------------------------
// PUT /v3/domains/{name}/tracking/open — Update Open Tracking
// ---------------------------------------------------------------------------

func TestUpdateOpenTracking_DisableOpen(t *testing.T) {
	db := setupTrackingTestDB(t)
	cfg := defaultConfig()
	router := setupTrackingRouter(db, cfg)

	createTestDomain(t, router, "example.com")

	req := newMultipartRequest(t, http.MethodPut, "/v3/domains/example.com/tracking/open", map[string]string{
		"active": "false",
	})
	rec := doRequest(t, router, req)

	t.Run("returns 200 status", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp updateOpenTrackingResponse
	decodeJSON(t, rec, &resp)

	t.Run("returns success message", func(t *testing.T) {
		expected := "Domain tracking settings have been updated"
		if resp.Message != expected {
			t.Errorf("expected message %q, got %q", expected, resp.Message)
		}
	})

	t.Run("open active is false", func(t *testing.T) {
		if resp.Open.Active {
			t.Errorf("expected open.active=false, got %v", resp.Open.Active)
		}
	})
}

func TestUpdateOpenTracking_EnableOpen(t *testing.T) {
	db := setupTrackingTestDB(t)
	cfg := defaultConfig()
	router := setupTrackingRouter(db, cfg)

	createTestDomain(t, router, "example.com")

	// First disable open tracking.
	disableReq := newMultipartRequest(t, http.MethodPut, "/v3/domains/example.com/tracking/open", map[string]string{
		"active": "false",
	})
	doRequest(t, router, disableReq)

	// Now re-enable it.
	enableReq := newMultipartRequest(t, http.MethodPut, "/v3/domains/example.com/tracking/open", map[string]string{
		"active": "true",
	})
	enableRec := doRequest(t, router, enableReq)

	t.Run("returns 200 status", func(t *testing.T) {
		if enableRec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d (body: %s)", enableRec.Code, enableRec.Body.String())
		}
	})

	var resp updateOpenTrackingResponse
	decodeJSON(t, enableRec, &resp)

	t.Run("open active is true", func(t *testing.T) {
		if !resp.Open.Active {
			t.Errorf("expected open.active=true, got %v", resp.Open.Active)
		}
	})
}

func TestUpdateOpenTracking_PlaceAtTheTop(t *testing.T) {
	db := setupTrackingTestDB(t)
	cfg := defaultConfig()
	router := setupTrackingRouter(db, cfg)

	createTestDomain(t, router, "example.com")

	req := newMultipartRequest(t, http.MethodPut, "/v3/domains/example.com/tracking/open", map[string]string{
		"active":           "true",
		"place_at_the_top": "true",
	})
	rec := doRequest(t, router, req)

	t.Run("returns 200 status", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp updateOpenTrackingResponse
	decodeJSON(t, rec, &resp)

	t.Run("returns success message", func(t *testing.T) {
		expected := "Domain tracking settings have been updated"
		if resp.Message != expected {
			t.Errorf("expected message %q, got %q", expected, resp.Message)
		}
	})

	t.Run("place_at_the_top is true", func(t *testing.T) {
		if !resp.Open.PlaceAtTheTop {
			t.Errorf("expected open.place_at_the_top=true, got %v", resp.Open.PlaceAtTheTop)
		}
	})

	t.Run("active is true", func(t *testing.T) {
		if !resp.Open.Active {
			t.Errorf("expected open.active=true, got %v", resp.Open.Active)
		}
	})
}

func TestUpdateOpenTracking_NonexistentDomain(t *testing.T) {
	db := setupTrackingTestDB(t)
	cfg := defaultConfig()
	router := setupTrackingRouter(db, cfg)

	req := newMultipartRequest(t, http.MethodPut, "/v3/domains/nonexistent.com/tracking/open", map[string]string{
		"active": "true",
	})
	rec := doRequest(t, router, req)

	t.Run("returns 404 status", func(t *testing.T) {
		if rec.Code != http.StatusNotFound {
			t.Errorf("expected status 404, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp errorResponse
	decodeJSON(t, rec, &resp)

	t.Run("returns domain not found message", func(t *testing.T) {
		if resp.Message != "Domain not found" {
			t.Errorf("expected message %q, got %q", "Domain not found", resp.Message)
		}
	})
}

func TestUpdateOpenTracking_VerifyGetReflectsChange(t *testing.T) {
	db := setupTrackingTestDB(t)
	cfg := defaultConfig()
	router := setupTrackingRouter(db, cfg)

	createTestDomain(t, router, "example.com")

	// Update open tracking with place_at_the_top=true and active=false.
	updateReq := newMultipartRequest(t, http.MethodPut, "/v3/domains/example.com/tracking/open", map[string]string{
		"active":           "false",
		"place_at_the_top": "true",
	})
	doRequest(t, router, updateReq)

	// GET tracking and verify the change is reflected.
	getReq := httptest.NewRequest(http.MethodGet, "/v3/domains/example.com/tracking", nil)
	getRec := doRequest(t, router, getReq)

	var resp trackingResponse
	decodeJSON(t, getRec, &resp)

	t.Run("GET reflects open active=false", func(t *testing.T) {
		if resp.Tracking.Open.Active {
			t.Errorf("expected open.active=false, got %v", resp.Tracking.Open.Active)
		}
	})

	t.Run("GET reflects place_at_the_top=true", func(t *testing.T) {
		if !resp.Tracking.Open.PlaceAtTheTop {
			t.Errorf("expected open.place_at_the_top=true, got %v", resp.Tracking.Open.PlaceAtTheTop)
		}
	})
}

// ---------------------------------------------------------------------------
// PUT /v3/domains/{name}/tracking/click — Update Click Tracking
// ---------------------------------------------------------------------------

func TestUpdateClickTracking_DisableClick(t *testing.T) {
	db := setupTrackingTestDB(t)
	cfg := defaultConfig()
	router := setupTrackingRouter(db, cfg)

	createTestDomain(t, router, "example.com")

	req := newMultipartRequest(t, http.MethodPut, "/v3/domains/example.com/tracking/click", map[string]string{
		"active": "false",
	})
	rec := doRequest(t, router, req)

	t.Run("returns 200 status", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp updateClickTrackingResponse
	decodeJSON(t, rec, &resp)

	t.Run("returns success message", func(t *testing.T) {
		expected := "Domain tracking settings have been updated"
		if resp.Message != expected {
			t.Errorf("expected message %q, got %q", expected, resp.Message)
		}
	})

	t.Run("click active is false", func(t *testing.T) {
		active, ok := resp.Click.Active.(bool)
		if !ok {
			t.Fatalf("expected click.active to be bool, got %T", resp.Click.Active)
		}
		if active {
			t.Errorf("expected click.active=false, got %v", active)
		}
	})
}

func TestUpdateClickTracking_EnableClick(t *testing.T) {
	db := setupTrackingTestDB(t)
	cfg := defaultConfig()
	router := setupTrackingRouter(db, cfg)

	createTestDomain(t, router, "example.com")

	// First disable click tracking.
	disableReq := newMultipartRequest(t, http.MethodPut, "/v3/domains/example.com/tracking/click", map[string]string{
		"active": "false",
	})
	doRequest(t, router, disableReq)

	// Re-enable click tracking.
	enableReq := newMultipartRequest(t, http.MethodPut, "/v3/domains/example.com/tracking/click", map[string]string{
		"active": "true",
	})
	enableRec := doRequest(t, router, enableReq)

	t.Run("returns 200 status", func(t *testing.T) {
		if enableRec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d (body: %s)", enableRec.Code, enableRec.Body.String())
		}
	})

	var resp updateClickTrackingResponse
	decodeJSON(t, enableRec, &resp)

	t.Run("click active is true", func(t *testing.T) {
		active, ok := resp.Click.Active.(bool)
		if !ok {
			t.Fatalf("expected click.active to be bool, got %T", resp.Click.Active)
		}
		if !active {
			t.Errorf("expected click.active=true, got %v", active)
		}
	})
}

func TestUpdateClickTracking_HTMLOnly(t *testing.T) {
	db := setupTrackingTestDB(t)
	cfg := defaultConfig()
	router := setupTrackingRouter(db, cfg)

	createTestDomain(t, router, "example.com")

	req := newMultipartRequest(t, http.MethodPut, "/v3/domains/example.com/tracking/click", map[string]string{
		"active": "htmlonly",
	})
	rec := doRequest(t, router, req)

	t.Run("returns 200 status", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp updateClickTrackingResponse
	decodeJSON(t, rec, &resp)

	t.Run("returns success message", func(t *testing.T) {
		expected := "Domain tracking settings have been updated"
		if resp.Message != expected {
			t.Errorf("expected message %q, got %q", expected, resp.Message)
		}
	})

	t.Run("click active is htmlonly string", func(t *testing.T) {
		active, ok := resp.Click.Active.(string)
		if !ok {
			t.Fatalf("expected click.active to be string for htmlonly, got %T (%v)", resp.Click.Active, resp.Click.Active)
		}
		if active != "htmlonly" {
			t.Errorf("expected click.active=%q, got %q", "htmlonly", active)
		}
	})
}

func TestUpdateClickTracking_NonexistentDomain(t *testing.T) {
	db := setupTrackingTestDB(t)
	cfg := defaultConfig()
	router := setupTrackingRouter(db, cfg)

	req := newMultipartRequest(t, http.MethodPut, "/v3/domains/nonexistent.com/tracking/click", map[string]string{
		"active": "true",
	})
	rec := doRequest(t, router, req)

	t.Run("returns 404 status", func(t *testing.T) {
		if rec.Code != http.StatusNotFound {
			t.Errorf("expected status 404, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp errorResponse
	decodeJSON(t, rec, &resp)

	t.Run("returns domain not found message", func(t *testing.T) {
		if resp.Message != "Domain not found" {
			t.Errorf("expected message %q, got %q", "Domain not found", resp.Message)
		}
	})
}

// ---------------------------------------------------------------------------
// PUT /v3/domains/{name}/tracking/unsubscribe — Update Unsubscribe Tracking
// ---------------------------------------------------------------------------

func TestUpdateUnsubscribeTracking_Disable(t *testing.T) {
	db := setupTrackingTestDB(t)
	cfg := defaultConfig()
	router := setupTrackingRouter(db, cfg)

	createTestDomain(t, router, "example.com")

	req := newMultipartRequest(t, http.MethodPut, "/v3/domains/example.com/tracking/unsubscribe", map[string]string{
		"active": "false",
	})
	rec := doRequest(t, router, req)

	t.Run("returns 200 status", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp updateUnsubscribeTrackingResponse
	decodeJSON(t, rec, &resp)

	t.Run("returns success message", func(t *testing.T) {
		expected := "Domain tracking settings have been updated"
		if resp.Message != expected {
			t.Errorf("expected message %q, got %q", expected, resp.Message)
		}
	})

	t.Run("unsubscribe active is false", func(t *testing.T) {
		if resp.Unsubscribe.Active {
			t.Errorf("expected unsubscribe.active=false, got %v", resp.Unsubscribe.Active)
		}
	})
}

func TestUpdateUnsubscribeTracking_CustomFooters(t *testing.T) {
	db := setupTrackingTestDB(t)
	cfg := defaultConfig()
	router := setupTrackingRouter(db, cfg)

	createTestDomain(t, router, "example.com")

	customHTML := `<a href="%unsubscribe_url%">unsubscribe</a>`
	customText := `To unsubscribe: <%unsubscribe_url%>`

	req := newMultipartRequest(t, http.MethodPut, "/v3/domains/example.com/tracking/unsubscribe", map[string]string{
		"active":      "true",
		"html_footer": customHTML,
		"text_footer": customText,
	})
	rec := doRequest(t, router, req)

	t.Run("returns 200 status", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp updateUnsubscribeTrackingResponse
	decodeJSON(t, rec, &resp)

	t.Run("returns success message", func(t *testing.T) {
		expected := "Domain tracking settings have been updated"
		if resp.Message != expected {
			t.Errorf("expected message %q, got %q", expected, resp.Message)
		}
	})

	t.Run("unsubscribe active is true", func(t *testing.T) {
		if !resp.Unsubscribe.Active {
			t.Errorf("expected unsubscribe.active=true, got %v", resp.Unsubscribe.Active)
		}
	})

	t.Run("html_footer matches custom value", func(t *testing.T) {
		if resp.Unsubscribe.HTMLFooter != customHTML {
			t.Errorf("expected html_footer %q, got %q", customHTML, resp.Unsubscribe.HTMLFooter)
		}
	})

	t.Run("text_footer matches custom value", func(t *testing.T) {
		if resp.Unsubscribe.TextFooter != customText {
			t.Errorf("expected text_footer %q, got %q", customText, resp.Unsubscribe.TextFooter)
		}
	})
}

func TestUpdateUnsubscribeTracking_NonexistentDomain(t *testing.T) {
	db := setupTrackingTestDB(t)
	cfg := defaultConfig()
	router := setupTrackingRouter(db, cfg)

	req := newMultipartRequest(t, http.MethodPut, "/v3/domains/nonexistent.com/tracking/unsubscribe", map[string]string{
		"active": "true",
	})
	rec := doRequest(t, router, req)

	t.Run("returns 404 status", func(t *testing.T) {
		if rec.Code != http.StatusNotFound {
			t.Errorf("expected status 404, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	var resp errorResponse
	decodeJSON(t, rec, &resp)

	t.Run("returns domain not found message", func(t *testing.T) {
		if resp.Message != "Domain not found" {
			t.Errorf("expected message %q, got %q", "Domain not found", resp.Message)
		}
	})
}

func TestUpdateUnsubscribeTracking_UpdateOnlyHTMLFooter(t *testing.T) {
	db := setupTrackingTestDB(t)
	cfg := defaultConfig()
	router := setupTrackingRouter(db, cfg)

	createTestDomain(t, router, "example.com")

	// First set both footers to custom values.
	initialReq := newMultipartRequest(t, http.MethodPut, "/v3/domains/example.com/tracking/unsubscribe", map[string]string{
		"active":      "true",
		"html_footer": "<p>initial html</p>",
		"text_footer": "initial text",
	})
	doRequest(t, router, initialReq)

	// Now update only html_footer.
	updateReq := newMultipartRequest(t, http.MethodPut, "/v3/domains/example.com/tracking/unsubscribe", map[string]string{
		"active":      "true",
		"html_footer": "<p>updated html</p>",
	})
	updateRec := doRequest(t, router, updateReq)

	t.Run("returns 200 status", func(t *testing.T) {
		if updateRec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d (body: %s)", updateRec.Code, updateRec.Body.String())
		}
	})

	var resp updateUnsubscribeTrackingResponse
	decodeJSON(t, updateRec, &resp)

	t.Run("html_footer is updated", func(t *testing.T) {
		expected := "<p>updated html</p>"
		if resp.Unsubscribe.HTMLFooter != expected {
			t.Errorf("expected html_footer %q, got %q", expected, resp.Unsubscribe.HTMLFooter)
		}
	})
}

func TestUpdateUnsubscribeTracking_VerifyGetReflectsCustomFooters(t *testing.T) {
	db := setupTrackingTestDB(t)
	cfg := defaultConfig()
	router := setupTrackingRouter(db, cfg)

	createTestDomain(t, router, "example.com")

	customHTML := `<div><a href="%unsubscribe_url%">Click to unsubscribe</a></div>`
	customText := `Unsubscribe here: <%unsubscribe_url%>`

	// Update unsubscribe tracking with custom footers.
	updateReq := newMultipartRequest(t, http.MethodPut, "/v3/domains/example.com/tracking/unsubscribe", map[string]string{
		"active":      "true",
		"html_footer": customHTML,
		"text_footer": customText,
	})
	doRequest(t, router, updateReq)

	// GET tracking and verify the custom footers are reflected.
	getReq := httptest.NewRequest(http.MethodGet, "/v3/domains/example.com/tracking", nil)
	getRec := doRequest(t, router, getReq)

	t.Run("returns 200 status", func(t *testing.T) {
		if getRec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d (body: %s)", getRec.Code, getRec.Body.String())
		}
	})

	var resp trackingResponse
	decodeJSON(t, getRec, &resp)

	t.Run("GET reflects custom html_footer", func(t *testing.T) {
		if resp.Tracking.Unsubscribe.HTMLFooter != customHTML {
			t.Errorf("expected html_footer %q, got %q", customHTML, resp.Tracking.Unsubscribe.HTMLFooter)
		}
	})

	t.Run("GET reflects custom text_footer", func(t *testing.T) {
		if resp.Tracking.Unsubscribe.TextFooter != customText {
			t.Errorf("expected text_footer %q, got %q", customText, resp.Tracking.Unsubscribe.TextFooter)
		}
	})

	t.Run("GET reflects unsubscribe active=true", func(t *testing.T) {
		if !resp.Tracking.Unsubscribe.Active {
			t.Errorf("expected unsubscribe.active=true, got %v", resp.Tracking.Unsubscribe.Active)
		}
	})
}

// ---------------------------------------------------------------------------
// Cross-endpoint: multiple tracking types updated together
// ---------------------------------------------------------------------------

func TestGetTracking_AfterMultipleUpdates(t *testing.T) {
	db := setupTrackingTestDB(t)
	cfg := defaultConfig()
	router := setupTrackingRouter(db, cfg)

	createTestDomain(t, router, "multi.com")

	// Disable open tracking with place_at_the_top=true.
	openReq := newMultipartRequest(t, http.MethodPut, "/v3/domains/multi.com/tracking/open", map[string]string{
		"active":           "false",
		"place_at_the_top": "true",
	})
	doRequest(t, router, openReq)

	// Set click tracking to htmlonly.
	clickReq := newMultipartRequest(t, http.MethodPut, "/v3/domains/multi.com/tracking/click", map[string]string{
		"active": "htmlonly",
	})
	doRequest(t, router, clickReq)

	// Set custom unsubscribe footers and disable.
	unsubReq := newMultipartRequest(t, http.MethodPut, "/v3/domains/multi.com/tracking/unsubscribe", map[string]string{
		"active":      "false",
		"html_footer": "<p>bye</p>",
		"text_footer": "bye",
	})
	doRequest(t, router, unsubReq)

	// GET all tracking settings.
	getReq := httptest.NewRequest(http.MethodGet, "/v3/domains/multi.com/tracking", nil)
	getRec := doRequest(t, router, getReq)

	t.Run("returns 200 status", func(t *testing.T) {
		if getRec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d (body: %s)", getRec.Code, getRec.Body.String())
		}
	})

	var resp trackingResponse
	decodeJSON(t, getRec, &resp)

	t.Run("open active is false", func(t *testing.T) {
		if resp.Tracking.Open.Active {
			t.Errorf("expected open.active=false, got %v", resp.Tracking.Open.Active)
		}
	})

	t.Run("open place_at_the_top is true", func(t *testing.T) {
		if !resp.Tracking.Open.PlaceAtTheTop {
			t.Errorf("expected open.place_at_the_top=true, got %v", resp.Tracking.Open.PlaceAtTheTop)
		}
	})

	t.Run("click active is htmlonly", func(t *testing.T) {
		active, ok := resp.Tracking.Click.Active.(string)
		if !ok {
			t.Fatalf("expected click.active to be string for htmlonly, got %T (%v)", resp.Tracking.Click.Active, resp.Tracking.Click.Active)
		}
		if active != "htmlonly" {
			t.Errorf("expected click.active=%q, got %q", "htmlonly", active)
		}
	})

	t.Run("unsubscribe active is false", func(t *testing.T) {
		if resp.Tracking.Unsubscribe.Active {
			t.Errorf("expected unsubscribe.active=false, got %v", resp.Tracking.Unsubscribe.Active)
		}
	})

	t.Run("unsubscribe html_footer is custom", func(t *testing.T) {
		expected := "<p>bye</p>"
		if resp.Tracking.Unsubscribe.HTMLFooter != expected {
			t.Errorf("expected html_footer %q, got %q", expected, resp.Tracking.Unsubscribe.HTMLFooter)
		}
	})

	t.Run("unsubscribe text_footer is custom", func(t *testing.T) {
		expected := "bye"
		if resp.Tracking.Unsubscribe.TextFooter != expected {
			t.Errorf("expected text_footer %q, got %q", expected, resp.Tracking.Unsubscribe.TextFooter)
		}
	})
}

// ---------------------------------------------------------------------------
// Isolation: tracking settings are per-domain
// ---------------------------------------------------------------------------

func TestGetTracking_IsolatedPerDomain(t *testing.T) {
	db := setupTrackingTestDB(t)
	cfg := defaultConfig()
	router := setupTrackingRouter(db, cfg)

	// Create two domains.
	createTestDomain(t, router, "domain-a.com")
	createTestDomain(t, router, "domain-b.com")

	// Update open tracking on domain-a only.
	updateReq := newMultipartRequest(t, http.MethodPut, "/v3/domains/domain-a.com/tracking/open", map[string]string{
		"active": "false",
	})
	doRequest(t, router, updateReq)

	// Verify domain-a has open tracking disabled.
	getReqA := httptest.NewRequest(http.MethodGet, "/v3/domains/domain-a.com/tracking", nil)
	getRecA := doRequest(t, router, getReqA)

	var respA trackingResponse
	decodeJSON(t, getRecA, &respA)

	t.Run("domain-a open tracking is disabled", func(t *testing.T) {
		if respA.Tracking.Open.Active {
			t.Errorf("expected domain-a open.active=false, got %v", respA.Tracking.Open.Active)
		}
	})

	// Verify domain-b still has default open tracking.
	getReqB := httptest.NewRequest(http.MethodGet, "/v3/domains/domain-b.com/tracking", nil)
	getRecB := doRequest(t, router, getReqB)

	var respB trackingResponse
	decodeJSON(t, getRecB, &respB)

	t.Run("domain-b open tracking remains at default", func(t *testing.T) {
		if !respB.Tracking.Open.Active {
			t.Errorf("expected domain-b open.active=true (unmodified), got %v", respB.Tracking.Open.Active)
		}
	})
}

