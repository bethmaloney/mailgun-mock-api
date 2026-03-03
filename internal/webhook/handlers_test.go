package webhook_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/bethmaloney/mailgun-mock-api/internal/domain"
	"github.com/bethmaloney/mailgun-mock-api/internal/mock"
	"github.com/bethmaloney/mailgun-mock-api/internal/webhook"
	"github.com/go-chi/chi/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const testDomain = "test.example.com"

// allEventTypes is the canonical list of Mailgun webhook event types.
var allEventTypes = []string{
	"accepted", "delivered", "opened", "clicked",
	"unsubscribed", "complained", "temporary_fail", "permanent_fail",
}

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
		&webhook.DomainWebhook{},
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
	dh := domain.NewHandlers(db, cfg)
	wh := webhook.NewHandlers(db, cfg)
	r := chi.NewRouter()

	// Domain creation
	r.Route("/v4/domains", func(r chi.Router) {
		r.Post("/", dh.CreateDomain)
		// v4 webhook routes
		r.Post("/{domain}/webhooks", wh.V4CreateWebhook)
		r.Put("/{domain}/webhooks", wh.V4UpdateWebhook)
		r.Delete("/{domain}/webhooks", wh.V4DeleteWebhook)
	})

	// v3 domain webhook routes
	r.Route("/v3/domains/{domain_name}/webhooks", func(r chi.Router) {
		r.Get("/", wh.ListWebhooks)
		r.Post("/", wh.CreateWebhook)
		r.Get("/{webhook_name}", wh.GetWebhook)
		r.Put("/{webhook_name}", wh.UpdateWebhook)
		r.Delete("/{webhook_name}", wh.DeleteWebhook)
	})

	// v5 signing key
	r.Get("/v5/accounts/http_signing_key", wh.GetSigningKey)
	r.Post("/v5/accounts/http_signing_key", wh.RegenerateSigningKey)

	return r
}

// setup creates a fresh database, router, and test domain. Returns the router
// ready for requests.
func setup(t *testing.T) http.Handler {
	t.Helper()
	db := setupTestDB(t)
	cfg := defaultConfig()
	router := setupRouter(db, cfg)

	// Create the test domain
	rec := doRequest(t, router, "POST", "/v4/domains", map[string]string{
		"name": testDomain,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to create test domain: %s", rec.Body.String())
	}

	return router
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

// fieldPair represents a key-value pair for multipart or form-encoded requests,
// allowing duplicate keys (which map[string]string does not support).
type fieldPair struct {
	key   string
	value string
}

// doRequestMultiValue sends a multipart/form-data request with potentially
// repeated keys (e.g. multiple "url" fields).
func doRequestMultiValue(t *testing.T, router http.Handler, method, urlStr string, fields []fieldPair) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for _, f := range fields {
		if err := writer.WriteField(f.key, f.value); err != nil {
			t.Fatalf("failed to write field %q: %v", f.key, err)
		}
	}
	writer.Close()
	req := httptest.NewRequest(method, urlStr, &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	router.ServeHTTP(rec, req)
	return rec
}

// doFormURLEncoded sends an application/x-www-form-urlencoded request with
// potentially repeated keys.
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

// doRequest sends a request with optional multipart/form-data fields. Pass nil
// for a body-less request.
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

// decodeJSON unmarshals the response recorder body into dest.
func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder, dest interface{}) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dest); err != nil {
		t.Fatalf("failed to decode response (body=%q): %v", rec.Body.String(), err)
	}
}

// assertStatus checks that the response status code matches expected.
func assertStatus(t *testing.T, rec *httptest.ResponseRecorder, expected int) {
	t.Helper()
	if rec.Code != expected {
		t.Errorf("expected status %d, got %d; body=%s", expected, rec.Code, rec.Body.String())
	}
}

// assertMessage checks that the JSON response has a "message" field equal to expected.
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

// v3WebhooksURL returns the URL for v3 webhook operations on the test domain.
func v3WebhooksURL() string {
	return fmt.Sprintf("/v3/domains/%s/webhooks", testDomain)
}

// v3WebhookURL returns the URL for a specific v3 webhook event type.
func v3WebhookURL(eventType string) string {
	return fmt.Sprintf("/v3/domains/%s/webhooks/%s", testDomain, eventType)
}

// v4WebhooksURL returns the URL for v4 webhook operations on the test domain.
func v4WebhooksURL() string {
	return fmt.Sprintf("/v4/domains/%s/webhooks", testDomain)
}

// ---------------------------------------------------------------------------
// v3 Domain Webhooks Tests
// ---------------------------------------------------------------------------

func TestListWebhooks_Empty(t *testing.T) {
	router := setup(t)

	rec := doRequest(t, router, "GET", v3WebhooksURL(), nil)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	webhooks, ok := body["webhooks"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected 'webhooks' object in response, got: %v", body)
	}

	// All 8 event types should be present with null values.
	for _, et := range allEventTypes {
		val, exists := webhooks[et]
		if !exists {
			t.Errorf("expected event type %q in webhooks map, but it was missing", et)
			continue
		}
		if val != nil {
			t.Errorf("expected null for unconfigured event type %q, got: %v", et, val)
		}
	}
}

func TestCreateWebhook_SingleURL(t *testing.T) {
	router := setup(t)

	rec := doRequestMultiValue(t, router, "POST", v3WebhooksURL(), []fieldPair{
		{key: "id", value: "delivered"},
		{key: "url", value: "https://example.com/hook1"},
	})
	assertStatus(t, rec, http.StatusOK)
	assertMessage(t, rec, "Webhook has been created")

	var body struct {
		Message string `json:"message"`
		Webhook struct {
			URLs []string `json:"urls"`
		} `json:"webhook"`
	}
	decodeJSON(t, rec, &body)

	if len(body.Webhook.URLs) != 1 {
		t.Fatalf("expected 1 URL, got %d: %v", len(body.Webhook.URLs), body.Webhook.URLs)
	}
	if body.Webhook.URLs[0] != "https://example.com/hook1" {
		t.Errorf("expected URL %q, got %q", "https://example.com/hook1", body.Webhook.URLs[0])
	}
}

func TestCreateWebhook_MultipleURLs(t *testing.T) {
	router := setup(t)

	rec := doRequestMultiValue(t, router, "POST", v3WebhooksURL(), []fieldPair{
		{key: "id", value: "opened"},
		{key: "url", value: "https://example.com/hook-a"},
		{key: "url", value: "https://example.com/hook-b"},
	})
	assertStatus(t, rec, http.StatusOK)
	assertMessage(t, rec, "Webhook has been created")

	var body struct {
		Webhook struct {
			URLs []string `json:"urls"`
		} `json:"webhook"`
	}
	decodeJSON(t, rec, &body)

	if len(body.Webhook.URLs) != 2 {
		t.Fatalf("expected 2 URLs, got %d: %v", len(body.Webhook.URLs), body.Webhook.URLs)
	}

	// Verify both URLs are present (order may vary).
	urlSet := map[string]bool{}
	for _, u := range body.Webhook.URLs {
		urlSet[u] = true
	}
	if !urlSet["https://example.com/hook-a"] {
		t.Errorf("expected URL https://example.com/hook-a in response")
	}
	if !urlSet["https://example.com/hook-b"] {
		t.Errorf("expected URL https://example.com/hook-b in response")
	}
}

func TestCreateWebhook_InvalidEventType(t *testing.T) {
	router := setup(t)

	rec := doRequestMultiValue(t, router, "POST", v3WebhooksURL(), []fieldPair{
		{key: "id", value: "nonexistent_event"},
		{key: "url", value: "https://example.com/hook"},
	})
	assertStatus(t, rec, http.StatusBadRequest)
}

func TestCreateWebhook_MaxURLsExceeded(t *testing.T) {
	router := setup(t)

	// Try to create a webhook with 4 URLs (max is 3).
	rec := doRequestMultiValue(t, router, "POST", v3WebhooksURL(), []fieldPair{
		{key: "id", value: "delivered"},
		{key: "url", value: "https://example.com/hook1"},
		{key: "url", value: "https://example.com/hook2"},
		{key: "url", value: "https://example.com/hook3"},
		{key: "url", value: "https://example.com/hook4"},
	})
	assertStatus(t, rec, http.StatusBadRequest)
}

func TestCreateWebhook_MissingFields(t *testing.T) {
	router := setup(t)

	// Missing both id and url
	rec := doRequestMultiValue(t, router, "POST", v3WebhooksURL(), []fieldPair{})
	assertStatus(t, rec, http.StatusBadRequest)

	// Missing url
	rec = doRequestMultiValue(t, router, "POST", v3WebhooksURL(), []fieldPair{
		{key: "id", value: "delivered"},
	})
	assertStatus(t, rec, http.StatusBadRequest)

	// Missing id
	rec = doRequestMultiValue(t, router, "POST", v3WebhooksURL(), []fieldPair{
		{key: "url", value: "https://example.com/hook"},
	})
	assertStatus(t, rec, http.StatusBadRequest)
}

func TestGetWebhook(t *testing.T) {
	router := setup(t)

	// Create a webhook first.
	createRec := doRequestMultiValue(t, router, "POST", v3WebhooksURL(), []fieldPair{
		{key: "id", value: "clicked"},
		{key: "url", value: "https://example.com/click-hook"},
	})
	assertStatus(t, createRec, http.StatusOK)

	// GET the webhook by event type.
	rec := doRequest(t, router, "GET", v3WebhookURL("clicked"), nil)
	assertStatus(t, rec, http.StatusOK)

	var body struct {
		Webhook struct {
			URLs []string `json:"urls"`
		} `json:"webhook"`
	}
	decodeJSON(t, rec, &body)

	if len(body.Webhook.URLs) != 1 {
		t.Fatalf("expected 1 URL, got %d: %v", len(body.Webhook.URLs), body.Webhook.URLs)
	}
	if body.Webhook.URLs[0] != "https://example.com/click-hook" {
		t.Errorf("expected URL %q, got %q", "https://example.com/click-hook", body.Webhook.URLs[0])
	}
}

func TestGetWebhook_NotConfigured(t *testing.T) {
	router := setup(t)

	// GET an event type that has no URLs configured.
	rec := doRequest(t, router, "GET", v3WebhookURL("delivered"), nil)
	assertStatus(t, rec, http.StatusNotFound)
}

func TestUpdateWebhook(t *testing.T) {
	router := setup(t)

	// Create a webhook.
	createRec := doRequestMultiValue(t, router, "POST", v3WebhooksURL(), []fieldPair{
		{key: "id", value: "delivered"},
		{key: "url", value: "https://example.com/old-hook"},
	})
	assertStatus(t, createRec, http.StatusOK)

	// Update the webhook with new URLs (replaces all existing).
	rec := doRequestMultiValue(t, router, "PUT", v3WebhookURL("delivered"), []fieldPair{
		{key: "url", value: "https://example.com/new-hook-1"},
		{key: "url", value: "https://example.com/new-hook-2"},
	})
	assertStatus(t, rec, http.StatusOK)
	assertMessage(t, rec, "Webhook has been updated")

	var body struct {
		Webhook struct {
			URLs []string `json:"urls"`
		} `json:"webhook"`
	}
	decodeJSON(t, rec, &body)

	if len(body.Webhook.URLs) != 2 {
		t.Fatalf("expected 2 URLs after update, got %d: %v", len(body.Webhook.URLs), body.Webhook.URLs)
	}

	// Verify old URL is gone and new URLs are present.
	urlSet := map[string]bool{}
	for _, u := range body.Webhook.URLs {
		urlSet[u] = true
	}
	if urlSet["https://example.com/old-hook"] {
		t.Errorf("old URL should have been replaced but was still present")
	}
	if !urlSet["https://example.com/new-hook-1"] {
		t.Errorf("expected URL https://example.com/new-hook-1 in response")
	}
	if !urlSet["https://example.com/new-hook-2"] {
		t.Errorf("expected URL https://example.com/new-hook-2 in response")
	}
}

func TestDeleteWebhook(t *testing.T) {
	router := setup(t)

	// Create a webhook.
	createRec := doRequestMultiValue(t, router, "POST", v3WebhooksURL(), []fieldPair{
		{key: "id", value: "complained"},
		{key: "url", value: "https://example.com/complaint-hook"},
	})
	assertStatus(t, createRec, http.StatusOK)

	// Delete the webhook.
	rec := doRequest(t, router, "DELETE", v3WebhookURL("complained"), nil)
	assertStatus(t, rec, http.StatusOK)
	assertMessage(t, rec, "Webhook has been deleted")

	var body struct {
		Webhook struct {
			URLs []string `json:"urls"`
		} `json:"webhook"`
	}
	decodeJSON(t, rec, &body)

	if len(body.Webhook.URLs) != 0 {
		t.Errorf("expected empty URLs array after delete, got %v", body.Webhook.URLs)
	}

	// Verify it is gone: GET should return 404.
	getRec := doRequest(t, router, "GET", v3WebhookURL("complained"), nil)
	assertStatus(t, getRec, http.StatusNotFound)
}

func TestDeleteWebhook_NotConfigured(t *testing.T) {
	router := setup(t)

	// DELETE an event type that has no URLs configured.
	rec := doRequest(t, router, "DELETE", v3WebhookURL("permanent_fail"), nil)
	assertStatus(t, rec, http.StatusNotFound)
}

func TestListWebhooks_AfterCreation(t *testing.T) {
	router := setup(t)

	// Create webhooks for "delivered" and "opened".
	rec := doRequestMultiValue(t, router, "POST", v3WebhooksURL(), []fieldPair{
		{key: "id", value: "delivered"},
		{key: "url", value: "https://example.com/delivered-hook"},
	})
	assertStatus(t, rec, http.StatusOK)

	rec = doRequestMultiValue(t, router, "POST", v3WebhooksURL(), []fieldPair{
		{key: "id", value: "opened"},
		{key: "url", value: "https://example.com/opened-hook-1"},
		{key: "url", value: "https://example.com/opened-hook-2"},
	})
	assertStatus(t, rec, http.StatusOK)

	// List all webhooks.
	listRec := doRequest(t, router, "GET", v3WebhooksURL(), nil)
	assertStatus(t, listRec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, listRec, &body)

	webhooks, ok := body["webhooks"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected 'webhooks' object, got: %v", body)
	}

	// "delivered" should have URLs.
	deliveredVal, exists := webhooks["delivered"]
	if !exists {
		t.Fatal("expected 'delivered' key in webhooks map")
	}
	if deliveredVal == nil {
		t.Fatal("expected non-null value for 'delivered'")
	}
	deliveredObj, ok := deliveredVal.(map[string]interface{})
	if !ok {
		t.Fatalf("expected 'delivered' to be an object, got: %T", deliveredVal)
	}
	deliveredURLs, ok := deliveredObj["urls"].([]interface{})
	if !ok {
		t.Fatalf("expected 'urls' to be an array, got: %T", deliveredObj["urls"])
	}
	if len(deliveredURLs) != 1 {
		t.Errorf("expected 1 URL for 'delivered', got %d", len(deliveredURLs))
	}

	// "opened" should have 2 URLs.
	openedVal, exists := webhooks["opened"]
	if !exists {
		t.Fatal("expected 'opened' key in webhooks map")
	}
	if openedVal == nil {
		t.Fatal("expected non-null value for 'opened'")
	}
	openedObj, ok := openedVal.(map[string]interface{})
	if !ok {
		t.Fatalf("expected 'opened' to be an object, got: %T", openedVal)
	}
	openedURLs, ok := openedObj["urls"].([]interface{})
	if !ok {
		t.Fatalf("expected 'urls' to be an array, got: %T", openedObj["urls"])
	}
	if len(openedURLs) != 2 {
		t.Errorf("expected 2 URLs for 'opened', got %d", len(openedURLs))
	}

	// Unconfigured event types should be null.
	nullTypes := []string{"accepted", "clicked", "unsubscribed", "complained", "temporary_fail", "permanent_fail"}
	for _, et := range nullTypes {
		val, exists := webhooks[et]
		if !exists {
			t.Errorf("expected event type %q in webhooks map", et)
			continue
		}
		if val != nil {
			t.Errorf("expected null for unconfigured event type %q, got: %v", et, val)
		}
	}
}

// ---------------------------------------------------------------------------
// v4 Domain Webhooks Tests
// ---------------------------------------------------------------------------

func TestV4CreateWebhook(t *testing.T) {
	router := setup(t)

	// Create a webhook via v4 with a URL and multiple event types.
	rec := doFormURLEncoded(t, router, "POST", v4WebhooksURL(), []fieldPair{
		{key: "url", value: "https://example.com/v4-hook"},
		{key: "event_types", value: "delivered"},
		{key: "event_types", value: "opened"},
	})
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	webhooks, ok := body["webhooks"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected 'webhooks' object in response, got: %v", body)
	}

	// The URL should appear under both "delivered" and "opened".
	for _, et := range []string{"delivered", "opened"} {
		val, exists := webhooks[et]
		if !exists {
			t.Errorf("expected event type %q in response", et)
			continue
		}
		if val == nil {
			t.Errorf("expected non-null value for %q", et)
			continue
		}
		obj, ok := val.(map[string]interface{})
		if !ok {
			t.Errorf("expected %q to be an object, got %T", et, val)
			continue
		}
		urls, ok := obj["urls"].([]interface{})
		if !ok {
			t.Errorf("expected 'urls' array for %q, got %T", et, obj["urls"])
			continue
		}
		found := false
		for _, u := range urls {
			if u == "https://example.com/v4-hook" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected URL https://example.com/v4-hook under %q, got: %v", et, urls)
		}
	}
}

func TestV4UpdateWebhook(t *testing.T) {
	router := setup(t)

	// First create via v4.
	createRec := doFormURLEncoded(t, router, "POST", v4WebhooksURL(), []fieldPair{
		{key: "url", value: "https://example.com/v4-update"},
		{key: "event_types", value: "delivered"},
		{key: "event_types", value: "opened"},
	})
	assertStatus(t, createRec, http.StatusOK)

	// Update: change the event types for the same URL.
	rec := doFormURLEncoded(t, router, "PUT", v4WebhooksURL(), []fieldPair{
		{key: "url", value: "https://example.com/v4-update"},
		{key: "event_types", value: "clicked"},
		{key: "event_types", value: "complained"},
	})
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	webhooks, ok := body["webhooks"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected 'webhooks' object, got: %v", body)
	}

	// The URL should now be under "clicked" and "complained", not "delivered" or "opened".
	for _, et := range []string{"clicked", "complained"} {
		val := webhooks[et]
		if val == nil {
			t.Errorf("expected non-null value for %q after update", et)
			continue
		}
		obj, ok := val.(map[string]interface{})
		if !ok {
			t.Errorf("expected %q to be an object, got %T", et, val)
			continue
		}
		urls, ok := obj["urls"].([]interface{})
		if !ok {
			t.Errorf("expected 'urls' array for %q", et)
			continue
		}
		found := false
		for _, u := range urls {
			if u == "https://example.com/v4-update" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected URL https://example.com/v4-update under %q after update", et)
		}
	}

	// Old event types should no longer contain the URL.
	for _, et := range []string{"delivered", "opened"} {
		val := webhooks[et]
		if val == nil {
			// null is fine -- event type is no longer configured
			continue
		}
		obj, ok := val.(map[string]interface{})
		if !ok {
			continue
		}
		urls, ok := obj["urls"].([]interface{})
		if !ok {
			continue
		}
		for _, u := range urls {
			if u == "https://example.com/v4-update" {
				t.Errorf("URL should have been removed from %q after update, but was still present", et)
			}
		}
	}
}

func TestV4DeleteWebhook(t *testing.T) {
	router := setup(t)

	// Create via v4.
	createRec := doFormURLEncoded(t, router, "POST", v4WebhooksURL(), []fieldPair{
		{key: "url", value: "https://example.com/v4-delete"},
		{key: "event_types", value: "delivered"},
	})
	assertStatus(t, createRec, http.StatusOK)

	// Delete by URL query param.
	deleteURL := v4WebhooksURL() + "?url=" + url.QueryEscape("https://example.com/v4-delete")
	rec := doRequest(t, router, "DELETE", deleteURL, nil)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	webhooks, ok := body["webhooks"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected 'webhooks' object, got: %v", body)
	}

	// "delivered" should be null or have empty URLs after deleting the only URL.
	deliveredVal := webhooks["delivered"]
	if deliveredVal != nil {
		obj, ok := deliveredVal.(map[string]interface{})
		if ok {
			urls, ok := obj["urls"].([]interface{})
			if ok {
				for _, u := range urls {
					if u == "https://example.com/v4-delete" {
						t.Errorf("deleted URL should no longer appear under 'delivered'")
					}
				}
			}
		}
	}
}

func TestV4DeleteWebhook_MultipleURLs(t *testing.T) {
	router := setup(t)

	// Create two webhooks via v4.
	rec := doFormURLEncoded(t, router, "POST", v4WebhooksURL(), []fieldPair{
		{key: "url", value: "https://example.com/v4-multi-1"},
		{key: "event_types", value: "delivered"},
	})
	assertStatus(t, rec, http.StatusOK)

	rec = doFormURLEncoded(t, router, "POST", v4WebhooksURL(), []fieldPair{
		{key: "url", value: "https://example.com/v4-multi-2"},
		{key: "event_types", value: "delivered"},
	})
	assertStatus(t, rec, http.StatusOK)

	// Delete both URLs in one request (comma-separated).
	commaURLs := "https://example.com/v4-multi-1,https://example.com/v4-multi-2"
	deleteURL := v4WebhooksURL() + "?url=" + url.QueryEscape(commaURLs)
	rec = doRequest(t, router, "DELETE", deleteURL, nil)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	webhooks, ok := body["webhooks"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected 'webhooks' object, got: %v", body)
	}

	// "delivered" should be null or have no matching URLs.
	deliveredVal := webhooks["delivered"]
	if deliveredVal != nil {
		obj, ok := deliveredVal.(map[string]interface{})
		if ok {
			urls, ok := obj["urls"].([]interface{})
			if ok {
				for _, u := range urls {
					if u == "https://example.com/v4-multi-1" || u == "https://example.com/v4-multi-2" {
						t.Errorf("deleted URL %v should no longer appear under 'delivered'", u)
					}
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// v3/v4 Interop Tests
// ---------------------------------------------------------------------------

func TestV3V4Interop_CreateV3ReadV4(t *testing.T) {
	router := setup(t)

	// Create a webhook via v3.
	createRec := doRequestMultiValue(t, router, "POST", v3WebhooksURL(), []fieldPair{
		{key: "id", value: "delivered"},
		{key: "url", value: "https://example.com/interop-v3"},
	})
	assertStatus(t, createRec, http.StatusOK)

	// Read via v4 — create a second webhook via v4 POST to trigger a response
	// that includes the full webhooks map. Alternatively, we can re-create via v4
	// to get the map. Instead, let's verify through the v3 list endpoint that
	// the underlying data is shared, then check v4 by creating a v4 webhook
	// and verifying the v3-created data appears.

	// Add another webhook via v4 to get the full map response.
	v4Rec := doFormURLEncoded(t, router, "POST", v4WebhooksURL(), []fieldPair{
		{key: "url", value: "https://example.com/interop-v4"},
		{key: "event_types", value: "opened"},
	})
	assertStatus(t, v4Rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, v4Rec, &body)

	webhooks, ok := body["webhooks"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected 'webhooks' object in v4 response, got: %v", body)
	}

	// The v3-created "delivered" webhook should appear in the v4 response.
	deliveredVal := webhooks["delivered"]
	if deliveredVal == nil {
		t.Fatal("expected 'delivered' to be non-null in v4 response (created via v3)")
	}
	obj, ok := deliveredVal.(map[string]interface{})
	if !ok {
		t.Fatalf("expected 'delivered' to be an object, got %T", deliveredVal)
	}
	urls, ok := obj["urls"].([]interface{})
	if !ok {
		t.Fatalf("expected 'urls' array for 'delivered', got %T", obj["urls"])
	}
	found := false
	for _, u := range urls {
		if u == "https://example.com/interop-v3" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("v3-created URL https://example.com/interop-v3 not found in v4 response; urls=%v", urls)
	}
}

func TestV3V4Interop_CreateV4ReadV3(t *testing.T) {
	router := setup(t)

	// Create a webhook via v4.
	createRec := doFormURLEncoded(t, router, "POST", v4WebhooksURL(), []fieldPair{
		{key: "url", value: "https://example.com/interop-from-v4"},
		{key: "event_types", value: "clicked"},
	})
	assertStatus(t, createRec, http.StatusOK)

	// Read via v3 GET by event type.
	rec := doRequest(t, router, "GET", v3WebhookURL("clicked"), nil)
	assertStatus(t, rec, http.StatusOK)

	var body struct {
		Webhook struct {
			URLs []string `json:"urls"`
		} `json:"webhook"`
	}
	decodeJSON(t, rec, &body)

	if len(body.Webhook.URLs) < 1 {
		t.Fatal("expected at least 1 URL for 'clicked' via v3 after v4 creation")
	}

	found := false
	for _, u := range body.Webhook.URLs {
		if u == "https://example.com/interop-from-v4" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("v4-created URL https://example.com/interop-from-v4 not found in v3 response; urls=%v", body.Webhook.URLs)
	}
}

// ---------------------------------------------------------------------------
// Webhook Signing Key Tests (v5)
// ---------------------------------------------------------------------------

func TestGetSigningKey_Default(t *testing.T) {
	router := setup(t)

	rec := doRequest(t, router, "GET", "/v5/accounts/http_signing_key", nil)
	assertStatus(t, rec, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, rec, &body)

	msg, ok := body["message"].(string)
	if !ok || msg != "success" {
		t.Errorf("expected message 'success', got %q", msg)
	}

	key, ok := body["http_signing_key"].(string)
	if !ok {
		t.Fatalf("expected 'http_signing_key' string in response, got: %v", body)
	}
	if key != "key-mock-signing-key-000000000000" {
		t.Errorf("expected default signing key %q, got %q", "key-mock-signing-key-000000000000", key)
	}
}

func TestRegenerateSigningKey(t *testing.T) {
	router := setup(t)

	// Regenerate the signing key.
	regenRec := doRequest(t, router, "POST", "/v5/accounts/http_signing_key", nil)
	assertStatus(t, regenRec, http.StatusOK)

	var regenBody map[string]interface{}
	decodeJSON(t, regenRec, &regenBody)

	msg, ok := regenBody["message"].(string)
	if !ok || msg != "success" {
		t.Errorf("expected message 'success' from regenerate, got %q", msg)
	}

	newKey, ok := regenBody["http_signing_key"].(string)
	if !ok {
		t.Fatalf("expected 'http_signing_key' string from regenerate, got: %v", regenBody)
	}

	// The new key should be different from the default.
	if newKey == "key-mock-signing-key-000000000000" {
		t.Errorf("regenerated key should differ from the default, got same value: %q", newKey)
	}

	// Subsequent GET should return the new key.
	getRec := doRequest(t, router, "GET", "/v5/accounts/http_signing_key", nil)
	assertStatus(t, getRec, http.StatusOK)

	var getBody map[string]interface{}
	decodeJSON(t, getRec, &getBody)

	gotKey, ok := getBody["http_signing_key"].(string)
	if !ok {
		t.Fatalf("expected 'http_signing_key' string from GET, got: %v", getBody)
	}
	if gotKey != newKey {
		t.Errorf("GET after regenerate should return new key %q, got %q", newKey, gotKey)
	}
}

func TestRegenerateSigningKey_DifferentEachTime(t *testing.T) {
	router := setup(t)

	// Regenerate twice and verify each produces a different key.
	firstRec := doRequest(t, router, "POST", "/v5/accounts/http_signing_key", nil)
	assertStatus(t, firstRec, http.StatusOK)

	var firstBody map[string]interface{}
	decodeJSON(t, firstRec, &firstBody)
	firstKey, ok := firstBody["http_signing_key"].(string)
	if !ok {
		t.Fatalf("expected 'http_signing_key' string from first regenerate, got: %v", firstBody)
	}

	secondRec := doRequest(t, router, "POST", "/v5/accounts/http_signing_key", nil)
	assertStatus(t, secondRec, http.StatusOK)

	var secondBody map[string]interface{}
	decodeJSON(t, secondRec, &secondBody)
	secondKey, ok := secondBody["http_signing_key"].(string)
	if !ok {
		t.Fatalf("expected 'http_signing_key' string from second regenerate, got: %v", secondBody)
	}

	if firstKey == secondKey {
		t.Errorf("two consecutive regenerations should produce different keys, both got: %q", firstKey)
	}

	// Both should be different from the default.
	if firstKey == "key-mock-signing-key-000000000000" {
		t.Errorf("first regenerated key should differ from default")
	}
	if secondKey == "key-mock-signing-key-000000000000" {
		t.Errorf("second regenerated key should differ from default")
	}
}
