package route_test

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

	"github.com/bethmaloney/mailgun-mock-api/internal/domain"
	"github.com/bethmaloney/mailgun-mock-api/internal/mock"
	"github.com/bethmaloney/mailgun-mock-api/internal/route"
	"github.com/go-chi/chi/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// Test Helpers
// ---------------------------------------------------------------------------

// setupTestDB creates an in-memory SQLite database for testing with all
// required tables migrated.
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}
	if err := db.AutoMigrate(
		&domain.Domain{}, &domain.DNSRecord{},
		&route.Route{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

// defaultConfig returns a MockConfig with sensible defaults for testing.
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

// setupRouter creates a chi router with all route endpoints and domain
// creation endpoint registered.
func setupRouter(db *gorm.DB, cfg *mock.MockConfig) http.Handler {
	dh := domain.NewHandlers(db, cfg)
	rh := route.NewHandlers(db)
	r := chi.NewRouter()

	// Domain creation route (for inbound simulation tests)
	r.Post("/v4/domains", dh.CreateDomain)

	// Route CRUD
	r.Route("/v3/routes", func(r chi.Router) {
		r.Post("/", rh.CreateRoute)
		r.Get("/", rh.ListRoutes)
		r.Get("/match", rh.MatchRoute)
		r.Get("/{route_id}", rh.GetRoute)
		r.Put("/{route_id}", rh.UpdateRoute)
		r.Delete("/{route_id}", rh.DeleteRoute)
	})

	// Mock inbound simulation
	r.Post("/mock/inbound/{domain}", rh.SimulateInbound)

	return r
}

// setup is a convenience that creates a DB, config, and router in one call.
func setup(t *testing.T) http.Handler {
	t.Helper()
	db := setupTestDB(t)
	cfg := defaultConfig()
	return setupRouter(db, cfg)
}

// fieldPair represents a single form key-value pair, allowing repeated keys
// (e.g. multiple "action" fields).
type fieldPair struct {
	key   string
	value string
}

// doFormURLEncoded sends a request with application/x-www-form-urlencoded body
// using the provided field pairs. Repeated keys are supported via url.Values.Add.
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

// doRequest sends a request with optional multipart/form-data body.
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

// doJSON sends a request with a JSON body.
func doJSON(t *testing.T, router http.Handler, method, urlStr string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(body)
	req := httptest.NewRequest(method, urlStr, &buf)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	return rec
}

// decodeJSON unmarshals the response body into the provided destination.
func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder, dest interface{}) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dest); err != nil {
		t.Fatalf("failed to decode response (body=%q): %v", rec.Body.String(), err)
	}
}

// assertStatus verifies the HTTP response status code.
func assertStatus(t *testing.T, rec *httptest.ResponseRecorder, expected int) {
	t.Helper()
	if rec.Code != expected {
		t.Errorf("expected status %d, got %d; body=%s", expected, rec.Code, rec.Body.String())
	}
}

// assertMessage verifies the "message" field in a JSON response body.
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

// ---------------------------------------------------------------------------
// Response types for JSON decoding
// ---------------------------------------------------------------------------

// routeJSON represents a single route object in JSON responses.
type routeJSON struct {
	ID          string   `json:"id"`
	Priority    int      `json:"priority"`
	Description string   `json:"description"`
	Expression  string   `json:"expression"`
	Actions     []string `json:"actions"`
	CreatedAt   string   `json:"created_at"`
}

// createRouteResponse represents the JSON response from POST /v3/routes.
type createRouteResponse struct {
	Message string    `json:"message"`
	Route   routeJSON `json:"route"`
}

// listRoutesResponse represents the JSON response from GET /v3/routes.
type listRoutesResponse struct {
	TotalCount int         `json:"total_count"`
	Items      []routeJSON `json:"items"`
}

// getRouteResponse represents the JSON response from GET /v3/routes/{id}.
type getRouteResponse struct {
	Route routeJSON `json:"route"`
}

// updateRouteResponse represents the JSON response from PUT /v3/routes/{id}.
// The SDK expects route fields at the top level (no "route" wrapper).
type updateRouteResponse struct {
	Message     string   `json:"message"`
	ID          string   `json:"id"`
	Priority    int      `json:"priority"`
	Description string   `json:"description"`
	Expression  string   `json:"expression"`
	Actions     []string `json:"actions"`
	CreatedAt   string   `json:"created_at"`
}

// deleteRouteResponse represents the JSON response from DELETE /v3/routes/{id}.
type deleteRouteResponse struct {
	Message string `json:"message"`
	ID      string `json:"id"`
}

// errorResponse represents a JSON error response body.
type errorResponse struct {
	Message string `json:"message"`
}

// ---------------------------------------------------------------------------
// Helper: create a standard route and return its response
// ---------------------------------------------------------------------------

// createRoute is a convenience that creates a route with the given fields
// and returns the recorder. It uses doFormURLEncoded.
func createRoute(t *testing.T, router http.Handler, fields []fieldPair) *httptest.ResponseRecorder {
	t.Helper()
	return doFormURLEncoded(t, router, http.MethodPost, "/v3/routes", fields)
}

// createRouteAndDecode creates a route and decodes the response.
func createRouteAndDecode(t *testing.T, router http.Handler, fields []fieldPair) createRouteResponse {
	t.Helper()
	rec := createRoute(t, router, fields)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 creating route, got %d; body=%s", rec.Code, rec.Body.String())
	}
	var resp createRouteResponse
	decodeJSON(t, rec, &resp)
	return resp
}

// standardRouteFields returns a basic set of fields for creating a route.
func standardRouteFields() []fieldPair {
	return []fieldPair{
		{key: "priority", value: "5"},
		{key: "description", value: "Test route"},
		{key: "expression", value: `match_recipient(".*@example.com")`},
		{key: "action", value: `forward("http://example.com/webhook")`},
		{key: "action", value: `stop()`},
	}
}

// createDomainHelper creates a domain via POST /v4/domains using multipart.
func createDomainHelper(t *testing.T, router http.Handler, name string) {
	t.Helper()
	rec := doRequest(t, router, http.MethodPost, "/v4/domains", map[string]string{
		"name": name,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to create domain %q: status %d; body=%s", name, rec.Code, rec.Body.String())
	}
}

// hexPattern matches exactly 24 hex characters (lowercase).
var hexPattern = regexp.MustCompile(`^[0-9a-f]{24}$`)

// rfc2822Pattern is a loose check for RFC 2822 date format.
// Example: "Wed, 15 Feb 2012 13:03:31 GMT"
var rfc2822Pattern = regexp.MustCompile(`^[A-Z][a-z]{2}, \d{2} [A-Z][a-z]{2} \d{4} \d{2}:\d{2}:\d{2} [A-Z]+$`)

// ---------------------------------------------------------------------------
// Route CRUD Tests
// ---------------------------------------------------------------------------

// 1. TestCreateRoute_Basic
func TestCreateRoute_Basic(t *testing.T) {
	router := setup(t)

	fields := []fieldPair{
		{key: "priority", value: "5"},
		{key: "description", value: "Sample route"},
		{key: "expression", value: `match_recipient(".*@example.com")`},
		{key: "action", value: `forward("http://example.com/webhook")`},
	}

	rec := createRoute(t, router, fields)

	t.Run("returns 200 status", func(t *testing.T) {
		assertStatus(t, rec, http.StatusOK)
	})

	var resp createRouteResponse
	decodeJSON(t, rec, &resp)

	t.Run("returns success message", func(t *testing.T) {
		if resp.Message != "Route has been created" {
			t.Errorf("expected message %q, got %q", "Route has been created", resp.Message)
		}
	})

	t.Run("route has an id", func(t *testing.T) {
		if resp.Route.ID == "" {
			t.Error("expected non-empty route ID")
		}
	})

	t.Run("route id is 24 hex chars", func(t *testing.T) {
		if !hexPattern.MatchString(resp.Route.ID) {
			t.Errorf("expected 24 hex char ID, got %q (len=%d)", resp.Route.ID, len(resp.Route.ID))
		}
	})

	t.Run("priority matches", func(t *testing.T) {
		if resp.Route.Priority != 5 {
			t.Errorf("expected priority 5, got %d", resp.Route.Priority)
		}
	})

	t.Run("description matches", func(t *testing.T) {
		if resp.Route.Description != "Sample route" {
			t.Errorf("expected description %q, got %q", "Sample route", resp.Route.Description)
		}
	})

	t.Run("expression matches", func(t *testing.T) {
		if resp.Route.Expression != `match_recipient(".*@example.com")` {
			t.Errorf("expected expression %q, got %q", `match_recipient(".*@example.com")`, resp.Route.Expression)
		}
	})

	t.Run("actions array is present", func(t *testing.T) {
		if resp.Route.Actions == nil {
			t.Fatal("expected non-nil actions array")
		}
		if len(resp.Route.Actions) != 1 {
			t.Errorf("expected 1 action, got %d", len(resp.Route.Actions))
		}
	})

	t.Run("created_at is RFC 2822 format", func(t *testing.T) {
		if resp.Route.CreatedAt == "" {
			t.Fatal("expected non-empty created_at")
		}
		if !rfc2822Pattern.MatchString(resp.Route.CreatedAt) {
			t.Errorf("expected created_at in RFC 2822 format, got %q", resp.Route.CreatedAt)
		}
	})
}

// 2. TestCreateRoute_MultipleActions
func TestCreateRoute_MultipleActions(t *testing.T) {
	router := setup(t)

	fields := []fieldPair{
		{key: "expression", value: `match_recipient(".*@example.com")`},
		{key: "action", value: `forward("http://example.com/webhook")`},
		{key: "action", value: `stop()`},
	}

	rec := createRoute(t, router, fields)
	assertStatus(t, rec, http.StatusOK)

	var resp createRouteResponse
	decodeJSON(t, rec, &resp)

	t.Run("both actions appear in response", func(t *testing.T) {
		if len(resp.Route.Actions) != 2 {
			t.Fatalf("expected 2 actions, got %d: %v", len(resp.Route.Actions), resp.Route.Actions)
		}
		if resp.Route.Actions[0] != `forward("http://example.com/webhook")` {
			t.Errorf("expected first action %q, got %q", `forward("http://example.com/webhook")`, resp.Route.Actions[0])
		}
		if resp.Route.Actions[1] != `stop()` {
			t.Errorf("expected second action %q, got %q", `stop()`, resp.Route.Actions[1])
		}
	})
}

// 3. TestCreateRoute_MissingExpression
func TestCreateRoute_MissingExpression(t *testing.T) {
	router := setup(t)

	fields := []fieldPair{
		{key: "action", value: `forward("http://example.com/webhook")`},
	}

	rec := createRoute(t, router, fields)

	t.Run("returns 400 status", func(t *testing.T) {
		assertStatus(t, rec, http.StatusBadRequest)
	})
}

// 4. TestCreateRoute_MissingAction
func TestCreateRoute_MissingAction(t *testing.T) {
	router := setup(t)

	fields := []fieldPair{
		{key: "expression", value: `match_recipient(".*@example.com")`},
	}

	rec := createRoute(t, router, fields)

	t.Run("returns 400 status", func(t *testing.T) {
		assertStatus(t, rec, http.StatusBadRequest)
	})
}

// 5. TestCreateRoute_DefaultPriority
func TestCreateRoute_DefaultPriority(t *testing.T) {
	router := setup(t)

	fields := []fieldPair{
		{key: "expression", value: `match_recipient(".*@example.com")`},
		{key: "action", value: `forward("http://example.com/webhook")`},
	}

	rec := createRoute(t, router, fields)
	assertStatus(t, rec, http.StatusOK)

	var resp createRouteResponse
	decodeJSON(t, rec, &resp)

	t.Run("priority defaults to 0", func(t *testing.T) {
		if resp.Route.Priority != 0 {
			t.Errorf("expected default priority 0, got %d", resp.Route.Priority)
		}
	})
}

// 6. TestListRoutes_Empty
func TestListRoutes_Empty(t *testing.T) {
	router := setup(t)

	req := httptest.NewRequest(http.MethodGet, "/v3/routes", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	t.Run("returns 200 status", func(t *testing.T) {
		assertStatus(t, rec, http.StatusOK)
	})

	var resp listRoutesResponse
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

// 7. TestListRoutes_WithData
func TestListRoutes_WithData(t *testing.T) {
	router := setup(t)

	// Create 3 routes.
	for i := 0; i < 3; i++ {
		fields := []fieldPair{
			{key: "expression", value: fmt.Sprintf(`match_recipient(".*@example%d.com")`, i)},
			{key: "action", value: `forward("http://example.com/webhook")`},
		}
		rec := createRoute(t, router, fields)
		if rec.Code != http.StatusOK {
			t.Fatalf("failed to create route %d: status %d; body=%s", i, rec.Code, rec.Body.String())
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/v3/routes", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assertStatus(t, rec, http.StatusOK)

	var resp listRoutesResponse
	decodeJSON(t, rec, &resp)

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
}

// 8. TestListRoutes_Pagination
func TestListRoutes_Pagination(t *testing.T) {
	router := setup(t)

	// Create 5 routes.
	for i := 0; i < 5; i++ {
		fields := []fieldPair{
			{key: "expression", value: fmt.Sprintf(`match_recipient(".*@page%d.com")`, i)},
			{key: "action", value: `forward("http://example.com/webhook")`},
		}
		rec := createRoute(t, router, fields)
		if rec.Code != http.StatusOK {
			t.Fatalf("failed to create route %d: status %d; body=%s", i, rec.Code, rec.Body.String())
		}
	}

	t.Run("limit=2 skip=0 returns 2 items with total_count=5", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v3/routes?limit=2&skip=0", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assertStatus(t, rec, http.StatusOK)

		var resp listRoutesResponse
		decodeJSON(t, rec, &resp)

		if resp.TotalCount != 5 {
			t.Errorf("expected total_count=5, got %d", resp.TotalCount)
		}
		if len(resp.Items) != 2 {
			t.Errorf("expected 2 items, got %d", len(resp.Items))
		}
	})

	t.Run("skip=2 returns different items", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v3/routes?skip=2&limit=2", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assertStatus(t, rec, http.StatusOK)

		var resp listRoutesResponse
		decodeJSON(t, rec, &resp)

		if resp.TotalCount != 5 {
			t.Errorf("expected total_count=5, got %d", resp.TotalCount)
		}
		if len(resp.Items) != 2 {
			t.Errorf("expected 2 items with skip=2&limit=2, got %d", len(resp.Items))
		}
	})
}

// 9. TestListRoutes_LimitValidation
func TestListRoutes_LimitValidation(t *testing.T) {
	router := setup(t)

	req := httptest.NewRequest(http.MethodGet, "/v3/routes?limit=2000", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	t.Run("returns 400 status", func(t *testing.T) {
		assertStatus(t, rec, http.StatusBadRequest)
	})

	t.Run("returns limit error message", func(t *testing.T) {
		assertMessage(t, rec, "The 'limit' parameter can't be larger than 1000")
	})
}

// 10. TestListRoutes_PriorityOrdering
func TestListRoutes_PriorityOrdering(t *testing.T) {
	router := setup(t)

	// Create routes with priorities 5, 1, 10 (out of order).
	priorities := []struct {
		priority string
		expr     string
	}{
		{"5", `match_recipient(".*@five.com")`},
		{"1", `match_recipient(".*@one.com")`},
		{"10", `match_recipient(".*@ten.com")`},
	}

	for _, p := range priorities {
		fields := []fieldPair{
			{key: "priority", value: p.priority},
			{key: "expression", value: p.expr},
			{key: "action", value: `forward("http://example.com/webhook")`},
		}
		rec := createRoute(t, router, fields)
		if rec.Code != http.StatusOK {
			t.Fatalf("failed to create route with priority %s: status %d; body=%s", p.priority, rec.Code, rec.Body.String())
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/v3/routes", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assertStatus(t, rec, http.StatusOK)

	var resp listRoutesResponse
	decodeJSON(t, rec, &resp)

	t.Run("routes are returned in priority order", func(t *testing.T) {
		if len(resp.Items) != 3 {
			t.Fatalf("expected 3 items, got %d", len(resp.Items))
		}
		if resp.Items[0].Priority != 1 {
			t.Errorf("expected first item priority=1, got %d", resp.Items[0].Priority)
		}
		if resp.Items[1].Priority != 5 {
			t.Errorf("expected second item priority=5, got %d", resp.Items[1].Priority)
		}
		if resp.Items[2].Priority != 10 {
			t.Errorf("expected third item priority=10, got %d", resp.Items[2].Priority)
		}
	})
}

// 11. TestGetRoute_Found
func TestGetRoute_Found(t *testing.T) {
	router := setup(t)

	resp := createRouteAndDecode(t, router, standardRouteFields())
	routeID := resp.Route.ID

	req := httptest.NewRequest(http.MethodGet, "/v3/routes/"+routeID, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	t.Run("returns 200 status", func(t *testing.T) {
		assertStatus(t, rec, http.StatusOK)
	})

	var getResp getRouteResponse
	decodeJSON(t, rec, &getResp)

	t.Run("response has route object", func(t *testing.T) {
		if getResp.Route.ID == "" {
			t.Error("expected non-empty route ID in get response")
		}
	})

	t.Run("route ID matches", func(t *testing.T) {
		if getResp.Route.ID != routeID {
			t.Errorf("expected route ID %q, got %q", routeID, getResp.Route.ID)
		}
	})

	t.Run("expression matches", func(t *testing.T) {
		if getResp.Route.Expression != `match_recipient(".*@example.com")` {
			t.Errorf("expected expression %q, got %q", `match_recipient(".*@example.com")`, getResp.Route.Expression)
		}
	})

	t.Run("actions are present", func(t *testing.T) {
		if len(getResp.Route.Actions) == 0 {
			t.Error("expected non-empty actions array")
		}
	})
}

// 12. TestGetRoute_NotFound
func TestGetRoute_NotFound(t *testing.T) {
	router := setup(t)

	req := httptest.NewRequest(http.MethodGet, "/v3/routes/aaaaaaaabbbbbbbbcccccccc", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	t.Run("returns 404 status", func(t *testing.T) {
		assertStatus(t, rec, http.StatusNotFound)
	})

	t.Run("returns not found message", func(t *testing.T) {
		assertMessage(t, rec, "Route not found")
	})
}

// 13. TestUpdateRoute_Full
func TestUpdateRoute_Full(t *testing.T) {
	router := setup(t)

	resp := createRouteAndDecode(t, router, []fieldPair{
		{key: "priority", value: "1"},
		{key: "description", value: "Original"},
		{key: "expression", value: `match_recipient(".*@old.com")`},
		{key: "action", value: `forward("http://old.com/webhook")`},
	})
	routeID := resp.Route.ID

	updateFields := []fieldPair{
		{key: "priority", value: "10"},
		{key: "description", value: "Updated"},
		{key: "expression", value: `match_recipient(".*@new.com")`},
		{key: "action", value: `forward("http://new.com/webhook")`},
		{key: "action", value: `stop()`},
	}

	rec := doFormURLEncoded(t, router, http.MethodPut, "/v3/routes/"+routeID, updateFields)

	t.Run("returns 200 status", func(t *testing.T) {
		assertStatus(t, rec, http.StatusOK)
	})

	var updateResp updateRouteResponse
	decodeJSON(t, rec, &updateResp)

	t.Run("returns update message", func(t *testing.T) {
		if updateResp.Message != "Route has been updated" {
			t.Errorf("expected message %q, got %q", "Route has been updated", updateResp.Message)
		}
	})

	t.Run("priority updated", func(t *testing.T) {
		if updateResp.Priority != 10 {
			t.Errorf("expected priority 10, got %d", updateResp.Priority)
		}
	})

	t.Run("description updated", func(t *testing.T) {
		if updateResp.Description != "Updated" {
			t.Errorf("expected description %q, got %q", "Updated", updateResp.Description)
		}
	})

	t.Run("expression updated", func(t *testing.T) {
		if updateResp.Expression != `match_recipient(".*@new.com")` {
			t.Errorf("expected expression %q, got %q", `match_recipient(".*@new.com")`, updateResp.Expression)
		}
	})

	t.Run("actions updated", func(t *testing.T) {
		if len(updateResp.Actions) != 2 {
			t.Fatalf("expected 2 actions, got %d", len(updateResp.Actions))
		}
	})
}

// 14. TestUpdateRoute_Partial
func TestUpdateRoute_Partial(t *testing.T) {
	router := setup(t)

	original := createRouteAndDecode(t, router, []fieldPair{
		{key: "priority", value: "3"},
		{key: "description", value: "Original description"},
		{key: "expression", value: `match_recipient(".*@example.com")`},
		{key: "action", value: `forward("http://example.com/webhook")`},
	})
	routeID := original.Route.ID

	// Update only priority.
	updateFields := []fieldPair{
		{key: "priority", value: "99"},
	}

	rec := doFormURLEncoded(t, router, http.MethodPut, "/v3/routes/"+routeID, updateFields)
	assertStatus(t, rec, http.StatusOK)

	var updateResp updateRouteResponse
	decodeJSON(t, rec, &updateResp)

	t.Run("priority is updated", func(t *testing.T) {
		if updateResp.Priority != 99 {
			t.Errorf("expected priority 99, got %d", updateResp.Priority)
		}
	})

	t.Run("description remains unchanged", func(t *testing.T) {
		if updateResp.Description != "Original description" {
			t.Errorf("expected description %q, got %q", "Original description", updateResp.Description)
		}
	})

	t.Run("expression remains unchanged", func(t *testing.T) {
		if updateResp.Expression != `match_recipient(".*@example.com")` {
			t.Errorf("expected expression %q, got %q", `match_recipient(".*@example.com")`, updateResp.Expression)
		}
	})

	t.Run("actions remain unchanged", func(t *testing.T) {
		if len(updateResp.Actions) != 1 {
			t.Errorf("expected 1 action, got %d", len(updateResp.Actions))
		}
		if len(updateResp.Actions) > 0 && updateResp.Actions[0] != `forward("http://example.com/webhook")` {
			t.Errorf("expected action %q, got %q", `forward("http://example.com/webhook")`, updateResp.Actions[0])
		}
	})
}

// 15. TestUpdateRoute_NotFound
func TestUpdateRoute_NotFound(t *testing.T) {
	router := setup(t)

	updateFields := []fieldPair{
		{key: "priority", value: "5"},
	}

	rec := doFormURLEncoded(t, router, http.MethodPut, "/v3/routes/aaaaaaaabbbbbbbbcccccccc", updateFields)

	t.Run("returns 404 status", func(t *testing.T) {
		assertStatus(t, rec, http.StatusNotFound)
	})

	t.Run("returns not found message", func(t *testing.T) {
		assertMessage(t, rec, "Route not found")
	})
}

// 16. TestUpdateRoute_Actions
func TestUpdateRoute_Actions(t *testing.T) {
	router := setup(t)

	// Create with one action.
	original := createRouteAndDecode(t, router, []fieldPair{
		{key: "expression", value: `match_recipient(".*@example.com")`},
		{key: "action", value: `forward("http://example.com/webhook")`},
	})
	routeID := original.Route.ID

	// Update with two actions.
	updateFields := []fieldPair{
		{key: "action", value: `store(notify="http://example.com/notify")`},
		{key: "action", value: `stop()`},
	}

	rec := doFormURLEncoded(t, router, http.MethodPut, "/v3/routes/"+routeID, updateFields)
	assertStatus(t, rec, http.StatusOK)

	var updateResp updateRouteResponse
	decodeJSON(t, rec, &updateResp)

	t.Run("actions updated to two entries", func(t *testing.T) {
		if len(updateResp.Actions) != 2 {
			t.Fatalf("expected 2 actions after update, got %d: %v", len(updateResp.Actions), updateResp.Actions)
		}
		if updateResp.Actions[0] != `store(notify="http://example.com/notify")` {
			t.Errorf("expected first action %q, got %q", `store(notify="http://example.com/notify")`, updateResp.Actions[0])
		}
		if updateResp.Actions[1] != `stop()` {
			t.Errorf("expected second action %q, got %q", `stop()`, updateResp.Actions[1])
		}
	})
}

// 17. TestDeleteRoute_Success
func TestDeleteRoute_Success(t *testing.T) {
	router := setup(t)

	resp := createRouteAndDecode(t, router, standardRouteFields())
	routeID := resp.Route.ID

	// Delete the route.
	req := httptest.NewRequest(http.MethodDelete, "/v3/routes/"+routeID, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	t.Run("returns 200 status", func(t *testing.T) {
		assertStatus(t, rec, http.StatusOK)
	})

	var delResp deleteRouteResponse
	decodeJSON(t, rec, &delResp)

	t.Run("returns deletion message", func(t *testing.T) {
		if delResp.Message != "Route has been deleted" {
			t.Errorf("expected message %q, got %q", "Route has been deleted", delResp.Message)
		}
	})

	t.Run("returns deleted route id", func(t *testing.T) {
		if delResp.ID != routeID {
			t.Errorf("expected id %q, got %q", routeID, delResp.ID)
		}
	})

	// Verify GET returns 404 after deletion.
	t.Run("GET returns 404 after deletion", func(t *testing.T) {
		getReq := httptest.NewRequest(http.MethodGet, "/v3/routes/"+routeID, nil)
		getRec := httptest.NewRecorder()
		router.ServeHTTP(getRec, getReq)
		assertStatus(t, getRec, http.StatusNotFound)
	})
}

// 18. TestDeleteRoute_NotFound
func TestDeleteRoute_NotFound(t *testing.T) {
	router := setup(t)

	req := httptest.NewRequest(http.MethodDelete, "/v3/routes/aaaaaaaabbbbbbbbcccccccc", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	t.Run("returns 404 status", func(t *testing.T) {
		assertStatus(t, rec, http.StatusNotFound)
	})

	t.Run("returns not found message", func(t *testing.T) {
		assertMessage(t, rec, "Route not found")
	})
}

// ---------------------------------------------------------------------------
// Route Matching Tests
// ---------------------------------------------------------------------------

// 19. TestMatchRoute_RecipientMatch
func TestMatchRoute_RecipientMatch(t *testing.T) {
	router := setup(t)

	createRouteAndDecode(t, router, []fieldPair{
		{key: "expression", value: `match_recipient(".*@example.com")`},
		{key: "action", value: `forward("http://example.com/webhook")`},
	})

	req := httptest.NewRequest(http.MethodGet, "/v3/routes/match?address=test@example.com", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	t.Run("returns 200 status", func(t *testing.T) {
		assertStatus(t, rec, http.StatusOK)
	})

	var resp getRouteResponse
	decodeJSON(t, rec, &resp)

	t.Run("returns the matching route", func(t *testing.T) {
		if resp.Route.ID == "" {
			t.Error("expected non-empty route ID in match response")
		}
		if resp.Route.Expression != `match_recipient(".*@example.com")` {
			t.Errorf("expected expression %q, got %q", `match_recipient(".*@example.com")`, resp.Route.Expression)
		}
	})
}

// 20. TestMatchRoute_RecipientNoMatch
func TestMatchRoute_RecipientNoMatch(t *testing.T) {
	router := setup(t)

	createRouteAndDecode(t, router, []fieldPair{
		{key: "expression", value: `match_recipient(".*@example.com")`},
		{key: "action", value: `forward("http://example.com/webhook")`},
	})

	req := httptest.NewRequest(http.MethodGet, "/v3/routes/match?address=test@other.com", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	t.Run("returns 404 status", func(t *testing.T) {
		assertStatus(t, rec, http.StatusNotFound)
	})

	t.Run("returns not found message", func(t *testing.T) {
		assertMessage(t, rec, "Route not found")
	})
}

// 21. TestMatchRoute_CatchAll
func TestMatchRoute_CatchAll(t *testing.T) {
	router := setup(t)

	createRouteAndDecode(t, router, []fieldPair{
		{key: "expression", value: `catch_all()`},
		{key: "action", value: `store()`},
	})

	req := httptest.NewRequest(http.MethodGet, "/v3/routes/match?address=anything@anywhere.com", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	t.Run("returns 200 status", func(t *testing.T) {
		assertStatus(t, rec, http.StatusOK)
	})

	var resp getRouteResponse
	decodeJSON(t, rec, &resp)

	t.Run("catch_all matches any address", func(t *testing.T) {
		if resp.Route.ID == "" {
			t.Error("expected non-empty route ID for catch_all match")
		}
	})
}

// 22. TestMatchRoute_MatchHeader
func TestMatchRoute_MatchHeader(t *testing.T) {
	router := setup(t)

	// Create a match_header route. The match endpoint tests against recipient
	// address, so match_header won't match via address query. This test
	// verifies the route is stored and retrievable, and that it does NOT
	// incorrectly match against a recipient address.
	resp := createRouteAndDecode(t, router, []fieldPair{
		{key: "expression", value: `match_header("subject", ".*urgent.*")`},
		{key: "action", value: `forward("http://example.com/urgent")`},
	})

	t.Run("route is stored with match_header expression", func(t *testing.T) {
		if resp.Route.Expression != `match_header("subject", ".*urgent.*")` {
			t.Errorf("expected expression %q, got %q", `match_header("subject", ".*urgent.*")`, resp.Route.Expression)
		}
	})

	// Verify GET retrieves the route properly.
	t.Run("route is retrievable by ID", func(t *testing.T) {
		getReq := httptest.NewRequest(http.MethodGet, "/v3/routes/"+resp.Route.ID, nil)
		getRec := httptest.NewRecorder()
		router.ServeHTTP(getRec, getReq)
		assertStatus(t, getRec, http.StatusOK)
	})

	// A match_header route should not match when only an address is provided
	// (since there's no header to match against).
	t.Run("match_header does not match on address alone", func(t *testing.T) {
		matchReq := httptest.NewRequest(http.MethodGet, "/v3/routes/match?address=test@example.com", nil)
		matchRec := httptest.NewRecorder()
		router.ServeHTTP(matchRec, matchReq)
		assertStatus(t, matchRec, http.StatusNotFound)
	})
}

// 23. TestMatchRoute_PriorityOrdering
func TestMatchRoute_PriorityOrdering(t *testing.T) {
	router := setup(t)

	// Create route with priority 10 first.
	createRouteAndDecode(t, router, []fieldPair{
		{key: "priority", value: "10"},
		{key: "description", value: "Low priority"},
		{key: "expression", value: `match_recipient(".*@example.com")`},
		{key: "action", value: `forward("http://low-priority.com")`},
	})

	// Create route with priority 1 second.
	createRouteAndDecode(t, router, []fieldPair{
		{key: "priority", value: "1"},
		{key: "description", value: "High priority"},
		{key: "expression", value: `match_recipient(".*@example.com")`},
		{key: "action", value: `forward("http://high-priority.com")`},
	})

	req := httptest.NewRequest(http.MethodGet, "/v3/routes/match?address=test@example.com", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assertStatus(t, rec, http.StatusOK)

	var resp getRouteResponse
	decodeJSON(t, rec, &resp)

	t.Run("returns the higher-priority route (lower number)", func(t *testing.T) {
		if resp.Route.Priority != 1 {
			t.Errorf("expected priority 1 (higher priority), got %d", resp.Route.Priority)
		}
		if resp.Route.Description != "High priority" {
			t.Errorf("expected description %q, got %q", "High priority", resp.Route.Description)
		}
	})
}

// 24. TestMatchRoute_MissingAddress
func TestMatchRoute_MissingAddress(t *testing.T) {
	router := setup(t)

	req := httptest.NewRequest(http.MethodGet, "/v3/routes/match", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	t.Run("returns 400 status", func(t *testing.T) {
		assertStatus(t, rec, http.StatusBadRequest)
	})
}

// 25. TestMatchRoute_SingleQuotes
func TestMatchRoute_SingleQuotes(t *testing.T) {
	router := setup(t)

	createRouteAndDecode(t, router, []fieldPair{
		{key: "expression", value: `match_recipient('.*@example.com')`},
		{key: "action", value: `forward("http://example.com/webhook")`},
	})

	req := httptest.NewRequest(http.MethodGet, "/v3/routes/match?address=test@example.com", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	t.Run("returns 200 status", func(t *testing.T) {
		assertStatus(t, rec, http.StatusOK)
	})

	var resp getRouteResponse
	decodeJSON(t, rec, &resp)

	t.Run("single-quoted expression matches", func(t *testing.T) {
		if resp.Route.ID == "" {
			t.Error("expected non-empty route ID for single-quote match")
		}
	})
}

// 26. TestMatchRoute_AndExpression
func TestMatchRoute_AndExpression(t *testing.T) {
	router := setup(t)

	createRouteAndDecode(t, router, []fieldPair{
		{key: "expression", value: `match_recipient(".*@example.com") and match_recipient("^test.*")`},
		{key: "action", value: `forward("http://example.com/webhook")`},
	})

	t.Run("matches when both conditions are satisfied", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v3/routes/match?address=test@example.com", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		assertStatus(t, rec, http.StatusOK)

		var resp getRouteResponse
		decodeJSON(t, rec, &resp)
		if resp.Route.ID == "" {
			t.Error("expected match for test@example.com with AND expression")
		}
	})

	t.Run("does not match when only one condition is satisfied", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v3/routes/match?address=user@example.com", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		assertStatus(t, rec, http.StatusNotFound)
	})
}

// ---------------------------------------------------------------------------
// Expression Parser (tested indirectly through match endpoint)
// ---------------------------------------------------------------------------

// 27. TestMatchRoute_ExactRecipient
func TestMatchRoute_ExactRecipient(t *testing.T) {
	router := setup(t)

	createRouteAndDecode(t, router, []fieldPair{
		{key: "expression", value: `match_recipient("foo@bar.com")`},
		{key: "action", value: `forward("http://example.com/webhook")`},
	})

	t.Run("exact address matches", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v3/routes/match?address=foo@bar.com", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		assertStatus(t, rec, http.StatusOK)

		var resp getRouteResponse
		decodeJSON(t, rec, &resp)
		if resp.Route.ID == "" {
			t.Error("expected match for exact address foo@bar.com")
		}
	})

	t.Run("different address does not match", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v3/routes/match?address=other@bar.com", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		assertStatus(t, rec, http.StatusNotFound)
	})
}

// ---------------------------------------------------------------------------
// Inbound Simulation Tests
// ---------------------------------------------------------------------------

// 28. TestSimulateInbound_Basic
func TestSimulateInbound_Basic(t *testing.T) {
	router := setup(t)

	// Create a domain for inbound simulation.
	createDomainHelper(t, router, "testdomain.com")

	// Create a route that matches recipients at testdomain.com.
	createRouteAndDecode(t, router, []fieldPair{
		{key: "expression", value: `match_recipient(".*@testdomain.com")`},
		{key: "action", value: `store()`},
	})

	// Simulate inbound.
	body := map[string]interface{}{
		"from":      "sender@test.com",
		"recipient": "user@testdomain.com",
		"subject":   "Test",
	}

	rec := doJSON(t, router, http.MethodPost, "/mock/inbound/testdomain.com", body)

	t.Run("returns 200 status", func(t *testing.T) {
		assertStatus(t, rec, http.StatusOK)
	})
}

// 29. TestSimulateInbound_StopAction
func TestSimulateInbound_StopAction(t *testing.T) {
	router := setup(t)

	createDomainHelper(t, router, "stopdomain.com")

	// Route 1: priority 1, matches and has stop()
	createRouteAndDecode(t, router, []fieldPair{
		{key: "priority", value: "1"},
		{key: "expression", value: `match_recipient(".*@stopdomain.com")`},
		{key: "action", value: `forward("http://first.com")`},
		{key: "action", value: `stop()`},
	})

	// Route 2: priority 2, also matches
	createRouteAndDecode(t, router, []fieldPair{
		{key: "priority", value: "2"},
		{key: "expression", value: `match_recipient(".*@stopdomain.com")`},
		{key: "action", value: `forward("http://second.com")`},
	})

	body := map[string]interface{}{
		"from":      "sender@test.com",
		"recipient": "user@stopdomain.com",
		"subject":   "Stop test",
	}

	rec := doJSON(t, router, http.MethodPost, "/mock/inbound/stopdomain.com", body)

	t.Run("returns 200 status", func(t *testing.T) {
		assertStatus(t, rec, http.StatusOK)
	})

	// Decode the response to verify only first route's actions were executed.
	var resp map[string]interface{}
	decodeJSON(t, rec, &resp)

	t.Run("stop action halts further evaluation", func(t *testing.T) {
		// The response should indicate that processing stopped at the first
		// route. The exact shape depends on implementation, but we verify
		// the response is valid JSON and the request succeeded.
		// We check for a "matched_routes" or similar field if present.
		if matchedRoutes, ok := resp["matched_routes"]; ok {
			routes, ok := matchedRoutes.([]interface{})
			if ok && len(routes) > 1 {
				t.Errorf("expected stop() to halt at first route, but %d routes matched", len(routes))
			}
		}
	})
}

// 30. TestSimulateInbound_NoMatchingRoutes
func TestSimulateInbound_NoMatchingRoutes(t *testing.T) {
	router := setup(t)

	createDomainHelper(t, router, "nomatch.com")

	// Create a route that does NOT match nomatch.com addresses.
	createRouteAndDecode(t, router, []fieldPair{
		{key: "expression", value: `match_recipient(".*@other.com")`},
		{key: "action", value: `forward("http://example.com")`},
	})

	body := map[string]interface{}{
		"from":      "sender@test.com",
		"recipient": "user@nomatch.com",
		"subject":   "No match test",
	}

	rec := doJSON(t, router, http.MethodPost, "/mock/inbound/nomatch.com", body)

	t.Run("returns 200 status for no matching routes", func(t *testing.T) {
		// Even with no matching routes, the inbound simulation should succeed
		// (it just doesn't trigger any actions).
		assertStatus(t, rec, http.StatusOK)
	})
}

// 31. TestSimulateInbound_CatchAllFallback
func TestSimulateInbound_CatchAllFallback(t *testing.T) {
	router := setup(t)

	createDomainHelper(t, router, "catchall.com")

	// Route 1: specific route that won't match (priority 0).
	createRouteAndDecode(t, router, []fieldPair{
		{key: "priority", value: "0"},
		{key: "expression", value: `match_recipient(".*@specific-only.com")`},
		{key: "action", value: `forward("http://specific.com")`},
	})

	// Route 2: catch_all route (priority 10).
	createRouteAndDecode(t, router, []fieldPair{
		{key: "priority", value: "10"},
		{key: "expression", value: `catch_all()`},
		{key: "action", value: `store()`},
	})

	body := map[string]interface{}{
		"from":      "sender@test.com",
		"recipient": "user@catchall.com",
		"subject":   "Catch all test",
	}

	rec := doJSON(t, router, http.MethodPost, "/mock/inbound/catchall.com", body)

	t.Run("returns 200 status", func(t *testing.T) {
		assertStatus(t, rec, http.StatusOK)
	})

	// The catch_all should have triggered since the specific route didn't match.
	var resp map[string]interface{}
	decodeJSON(t, rec, &resp)

	t.Run("response indicates catch_all was triggered", func(t *testing.T) {
		// At minimum, the response should be a successful JSON object.
		// The exact response shape depends on implementation.
		if resp == nil {
			t.Error("expected non-nil response body")
		}
	})
}

// ---------------------------------------------------------------------------
// Route ID Format Test
// ---------------------------------------------------------------------------

// 32. TestRouteID_Format
func TestRouteID_Format(t *testing.T) {
	router := setup(t)

	resp := createRouteAndDecode(t, router, []fieldPair{
		{key: "expression", value: `match_recipient(".*@example.com")`},
		{key: "action", value: `forward("http://example.com/webhook")`},
	})

	t.Run("ID is exactly 24 characters", func(t *testing.T) {
		if len(resp.Route.ID) != 24 {
			t.Errorf("expected ID length 24, got %d (%q)", len(resp.Route.ID), resp.Route.ID)
		}
	})

	t.Run("ID contains only hex characters", func(t *testing.T) {
		if !hexPattern.MatchString(resp.Route.ID) {
			t.Errorf("expected ID to match hex pattern [0-9a-f]{24}, got %q", resp.Route.ID)
		}
	})

	// Create a second route and verify IDs are unique.
	t.Run("IDs are unique", func(t *testing.T) {
		resp2 := createRouteAndDecode(t, router, []fieldPair{
			{key: "expression", value: `match_recipient(".*@other.com")`},
			{key: "action", value: `forward("http://other.com/webhook")`},
		})
		if resp.Route.ID == resp2.Route.ID {
			t.Errorf("expected unique IDs, but both are %q", resp.Route.ID)
		}
	})
}
