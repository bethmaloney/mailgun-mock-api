package mock_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bethmaloney/mailgun-mock-api/internal/mock"
	"github.com/go-chi/chi/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupTestDB creates an in-memory SQLite database for testing.
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}
	return db
}

// setupRouter creates a chi router with all mock routes registered for testing.
func setupRouter(h *mock.Handlers) http.Handler {
	r := chi.NewRouter()
	r.Route("/mock", func(r chi.Router) {
		r.Get("/health", mock.HealthHandler)
		r.Get("/config", h.GetConfig)
		r.Put("/config", h.UpdateConfig)
		r.Post("/reset", h.ResetAll)
		r.Post("/reset/messages", h.ResetMessages)
		r.Post("/reset/{domain}", h.ResetDomain)
	})
	return r
}

// ---------------------------------------------------------------------------
// Health endpoint tests
// ---------------------------------------------------------------------------

func TestHealthHandler(t *testing.T) {
	r := chi.NewRouter()
	r.Get("/mock/health", mock.HealthHandler)

	req := httptest.NewRequest(http.MethodGet, "/mock/health", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	t.Run("returns 200 status", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}
	})

	t.Run("returns JSON content type", func(t *testing.T) {
		ct := rec.Header().Get("Content-Type")
		if ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %q", ct)
		}
	})

	t.Run("returns ok status in body", func(t *testing.T) {
		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("failed to decode response body: %v", err)
		}
		if body["status"] != "ok" {
			t.Errorf("expected status %q, got %q", "ok", body["status"])
		}
	})
}

// ---------------------------------------------------------------------------
// GET /mock/config tests
// ---------------------------------------------------------------------------

func TestGetConfig_DefaultValues(t *testing.T) {
	db := setupTestDB(t)
	h := mock.NewHandlers(db)
	router := setupRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/mock/config", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	t.Run("returns 200 status", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}
	})

	t.Run("returns JSON content type", func(t *testing.T) {
		ct := rec.Header().Get("Content-Type")
		if ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %q", ct)
		}
	})

	var config map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &config); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	t.Run("contains event_generation section", func(t *testing.T) {
		raw, ok := config["event_generation"]
		if !ok {
			t.Fatal("response missing event_generation key")
		}
		var eg struct {
			AutoDeliver              bool    `json:"auto_deliver"`
			DeliveryDelayMs          int     `json:"delivery_delay_ms"`
			DefaultDeliveryStatus    int     `json:"default_delivery_status_code"`
			AutoFailRate             float64 `json:"auto_fail_rate"`
		}
		if err := json.Unmarshal(raw, &eg); err != nil {
			t.Fatalf("failed to decode event_generation: %v", err)
		}
		if eg.AutoDeliver != true {
			t.Errorf("expected auto_deliver=true, got %v", eg.AutoDeliver)
		}
		if eg.DeliveryDelayMs != 0 {
			t.Errorf("expected delivery_delay_ms=0, got %d", eg.DeliveryDelayMs)
		}
		if eg.DefaultDeliveryStatus != 250 {
			t.Errorf("expected default_delivery_status_code=250, got %d", eg.DefaultDeliveryStatus)
		}
		if eg.AutoFailRate != 0.0 {
			t.Errorf("expected auto_fail_rate=0.0, got %f", eg.AutoFailRate)
		}
	})

	t.Run("contains domain_behavior section", func(t *testing.T) {
		raw, ok := config["domain_behavior"]
		if !ok {
			t.Fatal("response missing domain_behavior key")
		}
		var db struct {
			DomainAutoVerify bool   `json:"domain_auto_verify"`
			SandboxDomain    string `json:"sandbox_domain"`
		}
		if err := json.Unmarshal(raw, &db); err != nil {
			t.Fatalf("failed to decode domain_behavior: %v", err)
		}
		if db.DomainAutoVerify != true {
			t.Errorf("expected domain_auto_verify=true, got %v", db.DomainAutoVerify)
		}
		if db.SandboxDomain != "sandbox123.mailgun.org" {
			t.Errorf("expected sandbox_domain=%q, got %q", "sandbox123.mailgun.org", db.SandboxDomain)
		}
	})

	t.Run("contains webhook_delivery section", func(t *testing.T) {
		raw, ok := config["webhook_delivery"]
		if !ok {
			t.Fatal("response missing webhook_delivery key")
		}
		var wd struct {
			WebhookRetryMode string `json:"webhook_retry_mode"`
			WebhookTimeoutMs int    `json:"webhook_timeout_ms"`
		}
		if err := json.Unmarshal(raw, &wd); err != nil {
			t.Fatalf("failed to decode webhook_delivery: %v", err)
		}
		if wd.WebhookRetryMode != "immediate" {
			t.Errorf("expected webhook_retry_mode=%q, got %q", "immediate", wd.WebhookRetryMode)
		}
		if wd.WebhookTimeoutMs != 5000 {
			t.Errorf("expected webhook_timeout_ms=5000, got %d", wd.WebhookTimeoutMs)
		}
	})

	t.Run("contains authentication section", func(t *testing.T) {
		raw, ok := config["authentication"]
		if !ok {
			t.Fatal("response missing authentication key")
		}
		var auth struct {
			AuthMode   string `json:"auth_mode"`
			SigningKey string `json:"signing_key"`
		}
		if err := json.Unmarshal(raw, &auth); err != nil {
			t.Fatalf("failed to decode authentication: %v", err)
		}
		if auth.AuthMode != "accept_any" {
			t.Errorf("expected auth_mode=%q, got %q", "accept_any", auth.AuthMode)
		}
		if auth.SigningKey != "key-mock-signing-key-000000000000" {
			t.Errorf("expected signing_key=%q, got %q", "key-mock-signing-key-000000000000", auth.SigningKey)
		}
	})

	t.Run("contains storage section", func(t *testing.T) {
		raw, ok := config["storage"]
		if !ok {
			t.Fatal("response missing storage key")
		}
		var st struct {
			StoreAttachmentBytes bool `json:"store_attachment_bytes"`
			MaxMessages          int  `json:"max_messages"`
			MaxEvents            int  `json:"max_events"`
		}
		if err := json.Unmarshal(raw, &st); err != nil {
			t.Fatalf("failed to decode storage: %v", err)
		}
		if st.StoreAttachmentBytes != false {
			t.Errorf("expected store_attachment_bytes=false, got %v", st.StoreAttachmentBytes)
		}
		if st.MaxMessages != 0 {
			t.Errorf("expected max_messages=0, got %d", st.MaxMessages)
		}
		if st.MaxEvents != 0 {
			t.Errorf("expected max_events=0, got %d", st.MaxEvents)
		}
	})

	t.Run("contains exactly five top-level keys", func(t *testing.T) {
		if len(config) != 5 {
			t.Errorf("expected 5 top-level keys, got %d", len(config))
		}
	})
}

// ---------------------------------------------------------------------------
// PUT /mock/config tests
// ---------------------------------------------------------------------------

func TestUpdateConfig_PartialUpdate(t *testing.T) {
	db := setupTestDB(t)
	h := mock.NewHandlers(db)
	router := setupRouter(h)

	t.Run("partial update changes only specified fields", func(t *testing.T) {
		body := `{"event_generation": {"auto_deliver": false}}`
		req := httptest.NewRequest(http.MethodPut, "/mock/config", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}

		var result map[string]json.RawMessage
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		// Check the updated field
		var eg struct {
			AutoDeliver           bool    `json:"auto_deliver"`
			DeliveryDelayMs       int     `json:"delivery_delay_ms"`
			DefaultDeliveryStatus int     `json:"default_delivery_status_code"`
			AutoFailRate          float64 `json:"auto_fail_rate"`
		}
		if err := json.Unmarshal(result["event_generation"], &eg); err != nil {
			t.Fatalf("failed to decode event_generation: %v", err)
		}
		if eg.AutoDeliver != false {
			t.Errorf("expected auto_deliver=false after update, got %v", eg.AutoDeliver)
		}
		// Other fields in event_generation should remain at defaults
		if eg.DeliveryDelayMs != 0 {
			t.Errorf("expected delivery_delay_ms=0 (unchanged), got %d", eg.DeliveryDelayMs)
		}
		if eg.DefaultDeliveryStatus != 250 {
			t.Errorf("expected default_delivery_status_code=250 (unchanged), got %d", eg.DefaultDeliveryStatus)
		}

		// Other sections should remain at defaults
		var auth struct {
			AuthMode   string `json:"auth_mode"`
			SigningKey string `json:"signing_key"`
		}
		if err := json.Unmarshal(result["authentication"], &auth); err != nil {
			t.Fatalf("failed to decode authentication: %v", err)
		}
		if auth.AuthMode != "accept_any" {
			t.Errorf("expected auth_mode=%q (unchanged), got %q", "accept_any", auth.AuthMode)
		}
	})

	t.Run("update multiple sections at once", func(t *testing.T) {
		body := `{
			"event_generation": {"delivery_delay_ms": 500},
			"authentication": {"auth_mode": "strict"}
		}`
		req := httptest.NewRequest(http.MethodPut, "/mock/config", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}

		var result map[string]json.RawMessage
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		var eg struct {
			DeliveryDelayMs int `json:"delivery_delay_ms"`
		}
		if err := json.Unmarshal(result["event_generation"], &eg); err != nil {
			t.Fatalf("failed to decode event_generation: %v", err)
		}
		if eg.DeliveryDelayMs != 500 {
			t.Errorf("expected delivery_delay_ms=500, got %d", eg.DeliveryDelayMs)
		}

		var auth struct {
			AuthMode string `json:"auth_mode"`
		}
		if err := json.Unmarshal(result["authentication"], &auth); err != nil {
			t.Fatalf("failed to decode authentication: %v", err)
		}
		if auth.AuthMode != "strict" {
			t.Errorf("expected auth_mode=%q, got %q", "strict", auth.AuthMode)
		}
	})

	t.Run("returns full config after partial update", func(t *testing.T) {
		body := `{"storage": {"max_messages": 1000}}`
		req := httptest.NewRequest(http.MethodPut, "/mock/config", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}

		var result map[string]json.RawMessage
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		// Verify all five sections are present in response
		expectedKeys := []string{"event_generation", "domain_behavior", "webhook_delivery", "authentication", "storage"}
		for _, key := range expectedKeys {
			if _, ok := result[key]; !ok {
				t.Errorf("response missing key %q after partial update", key)
			}
		}

		var st struct {
			MaxMessages int `json:"max_messages"`
		}
		if err := json.Unmarshal(result["storage"], &st); err != nil {
			t.Fatalf("failed to decode storage: %v", err)
		}
		if st.MaxMessages != 1000 {
			t.Errorf("expected max_messages=1000, got %d", st.MaxMessages)
		}
	})
}

func TestUpdateConfig_InvalidJSON(t *testing.T) {
	db := setupTestDB(t)
	h := mock.NewHandlers(db)
	router := setupRouter(h)

	t.Run("returns 400 for malformed JSON", func(t *testing.T) {
		body := `{this is not valid json}`
		req := httptest.NewRequest(http.MethodPut, "/mock/config", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", rec.Code)
		}

		var errResp map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("failed to decode error response: %v", err)
		}
		if _, ok := errResp["message"]; !ok {
			t.Error("error response missing 'message' key")
		}
	})

	t.Run("returns 400 for empty body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/mock/config", bytes.NewBufferString(""))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", rec.Code)
		}
	})
}

func TestGetConfig_AfterUpdate(t *testing.T) {
	db := setupTestDB(t)
	h := mock.NewHandlers(db)
	router := setupRouter(h)

	// First, update a config value
	updateBody := `{"event_generation": {"auto_deliver": false, "delivery_delay_ms": 250}}`
	putReq := httptest.NewRequest(http.MethodPut, "/mock/config", bytes.NewBufferString(updateBody))
	putReq.Header.Set("Content-Type", "application/json")
	putRec := httptest.NewRecorder()
	router.ServeHTTP(putRec, putReq)

	if putRec.Code != http.StatusOK {
		t.Fatalf("PUT failed with status %d", putRec.Code)
	}

	// Now GET the config and verify the update persisted
	getReq := httptest.NewRequest(http.MethodGet, "/mock/config", nil)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("GET failed with status %d", getRec.Code)
	}

	var config map[string]json.RawMessage
	if err := json.Unmarshal(getRec.Body.Bytes(), &config); err != nil {
		t.Fatalf("failed to decode GET response: %v", err)
	}

	var eg struct {
		AutoDeliver     bool `json:"auto_deliver"`
		DeliveryDelayMs int  `json:"delivery_delay_ms"`
	}
	if err := json.Unmarshal(config["event_generation"], &eg); err != nil {
		t.Fatalf("failed to decode event_generation: %v", err)
	}

	t.Run("GET reflects updated auto_deliver", func(t *testing.T) {
		if eg.AutoDeliver != false {
			t.Errorf("expected auto_deliver=false after PUT+GET, got %v", eg.AutoDeliver)
		}
	})

	t.Run("GET reflects updated delivery_delay_ms", func(t *testing.T) {
		if eg.DeliveryDelayMs != 250 {
			t.Errorf("expected delivery_delay_ms=250 after PUT+GET, got %d", eg.DeliveryDelayMs)
		}
	})
}

func TestGetConfig_JSONStructure(t *testing.T) {
	db := setupTestDB(t)
	h := mock.NewHandlers(db)
	router := setupRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/mock/config", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	// Verify the full JSON structure matches the spec exactly
	var fullConfig struct {
		EventGeneration struct {
			AutoDeliver              bool    `json:"auto_deliver"`
			DeliveryDelayMs          int     `json:"delivery_delay_ms"`
			DefaultDeliveryStatusCode int    `json:"default_delivery_status_code"`
			AutoFailRate             float64 `json:"auto_fail_rate"`
		} `json:"event_generation"`
		DomainBehavior struct {
			DomainAutoVerify bool   `json:"domain_auto_verify"`
			SandboxDomain    string `json:"sandbox_domain"`
		} `json:"domain_behavior"`
		WebhookDelivery struct {
			WebhookRetryMode string `json:"webhook_retry_mode"`
			WebhookTimeoutMs int    `json:"webhook_timeout_ms"`
		} `json:"webhook_delivery"`
		Authentication struct {
			AuthMode   string `json:"auth_mode"`
			SigningKey string `json:"signing_key"`
		} `json:"authentication"`
		Storage struct {
			StoreAttachmentBytes bool `json:"store_attachment_bytes"`
			MaxMessages          int  `json:"max_messages"`
			MaxEvents            int  `json:"max_events"`
		} `json:"storage"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &fullConfig); err != nil {
		t.Fatalf("response does not match expected JSON structure: %v", err)
	}

	t.Run("event_generation defaults", func(t *testing.T) {
		if fullConfig.EventGeneration.AutoDeliver != true {
			t.Errorf("auto_deliver: want true, got %v", fullConfig.EventGeneration.AutoDeliver)
		}
		if fullConfig.EventGeneration.DeliveryDelayMs != 0 {
			t.Errorf("delivery_delay_ms: want 0, got %d", fullConfig.EventGeneration.DeliveryDelayMs)
		}
		if fullConfig.EventGeneration.DefaultDeliveryStatusCode != 250 {
			t.Errorf("default_delivery_status_code: want 250, got %d", fullConfig.EventGeneration.DefaultDeliveryStatusCode)
		}
		if fullConfig.EventGeneration.AutoFailRate != 0.0 {
			t.Errorf("auto_fail_rate: want 0.0, got %f", fullConfig.EventGeneration.AutoFailRate)
		}
	})

	t.Run("domain_behavior defaults", func(t *testing.T) {
		if fullConfig.DomainBehavior.DomainAutoVerify != true {
			t.Errorf("domain_auto_verify: want true, got %v", fullConfig.DomainBehavior.DomainAutoVerify)
		}
		if fullConfig.DomainBehavior.SandboxDomain != "sandbox123.mailgun.org" {
			t.Errorf("sandbox_domain: want %q, got %q", "sandbox123.mailgun.org", fullConfig.DomainBehavior.SandboxDomain)
		}
	})

	t.Run("webhook_delivery defaults", func(t *testing.T) {
		if fullConfig.WebhookDelivery.WebhookRetryMode != "immediate" {
			t.Errorf("webhook_retry_mode: want %q, got %q", "immediate", fullConfig.WebhookDelivery.WebhookRetryMode)
		}
		if fullConfig.WebhookDelivery.WebhookTimeoutMs != 5000 {
			t.Errorf("webhook_timeout_ms: want 5000, got %d", fullConfig.WebhookDelivery.WebhookTimeoutMs)
		}
	})

	t.Run("authentication defaults", func(t *testing.T) {
		if fullConfig.Authentication.AuthMode != "accept_any" {
			t.Errorf("auth_mode: want %q, got %q", "accept_any", fullConfig.Authentication.AuthMode)
		}
		if fullConfig.Authentication.SigningKey != "key-mock-signing-key-000000000000" {
			t.Errorf("signing_key: want %q, got %q", "key-mock-signing-key-000000000000", fullConfig.Authentication.SigningKey)
		}
	})

	t.Run("storage defaults", func(t *testing.T) {
		if fullConfig.Storage.StoreAttachmentBytes != false {
			t.Errorf("store_attachment_bytes: want false, got %v", fullConfig.Storage.StoreAttachmentBytes)
		}
		if fullConfig.Storage.MaxMessages != 0 {
			t.Errorf("max_messages: want 0, got %d", fullConfig.Storage.MaxMessages)
		}
		if fullConfig.Storage.MaxEvents != 0 {
			t.Errorf("max_events: want 0, got %d", fullConfig.Storage.MaxEvents)
		}
	})
}

// ---------------------------------------------------------------------------
// POST /mock/reset tests
// ---------------------------------------------------------------------------

func TestResetAll(t *testing.T) {
	db := setupTestDB(t)
	h := mock.NewHandlers(db)
	router := setupRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/mock/reset", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	t.Run("returns 200 status", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}
	})

	t.Run("returns JSON content type", func(t *testing.T) {
		ct := rec.Header().Get("Content-Type")
		if ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %q", ct)
		}
	})

	t.Run("returns correct success message", func(t *testing.T) {
		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		expected := "All data has been reset"
		if body["message"] != expected {
			t.Errorf("expected message %q, got %q", expected, body["message"])
		}
	})
}

func TestResetDomain(t *testing.T) {
	db := setupTestDB(t)
	h := mock.NewHandlers(db)
	router := setupRouter(h)

	t.Run("returns success message with domain name", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/mock/reset/example.com", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}

		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		expected := "Data for domain example.com has been reset"
		if body["message"] != expected {
			t.Errorf("expected message %q, got %q", expected, body["message"])
		}
	})

	t.Run("works with different domain names", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/mock/reset/test.mailgun.org", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}

		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		expected := "Data for domain test.mailgun.org has been reset"
		if body["message"] != expected {
			t.Errorf("expected message %q, got %q", expected, body["message"])
		}
	})

	t.Run("returns JSON content type", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/mock/reset/example.com", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		ct := rec.Header().Get("Content-Type")
		if ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %q", ct)
		}
	})
}

func TestResetMessages(t *testing.T) {
	db := setupTestDB(t)
	h := mock.NewHandlers(db)
	router := setupRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/mock/reset/messages", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	t.Run("returns 200 status", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}
	})

	t.Run("returns correct success message", func(t *testing.T) {
		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		expected := "Messages and events have been reset"
		if body["message"] != expected {
			t.Errorf("expected message %q, got %q", expected, body["message"])
		}
	})

	t.Run("returns JSON content type", func(t *testing.T) {
		ct := rec.Header().Get("Content-Type")
		if ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %q", ct)
		}
	})
}

// ---------------------------------------------------------------------------
// Reset does not affect config
// ---------------------------------------------------------------------------

func TestResetAll_DoesNotAffectConfig(t *testing.T) {
	db := setupTestDB(t)
	h := mock.NewHandlers(db)
	router := setupRouter(h)

	// First, update the config
	updateBody := `{"event_generation": {"auto_deliver": false}}`
	putReq := httptest.NewRequest(http.MethodPut, "/mock/config", bytes.NewBufferString(updateBody))
	putReq.Header.Set("Content-Type", "application/json")
	putRec := httptest.NewRecorder()
	router.ServeHTTP(putRec, putReq)

	if putRec.Code != http.StatusOK {
		t.Fatalf("PUT config failed with status %d", putRec.Code)
	}

	// Reset all data
	resetReq := httptest.NewRequest(http.MethodPost, "/mock/reset", nil)
	resetRec := httptest.NewRecorder()
	router.ServeHTTP(resetRec, resetReq)

	if resetRec.Code != http.StatusOK {
		t.Fatalf("POST reset failed with status %d", resetRec.Code)
	}

	// Verify config is still updated (reset should not affect config)
	getReq := httptest.NewRequest(http.MethodGet, "/mock/config", nil)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)

	var config map[string]json.RawMessage
	if err := json.Unmarshal(getRec.Body.Bytes(), &config); err != nil {
		t.Fatalf("failed to decode config: %v", err)
	}

	var eg struct {
		AutoDeliver bool `json:"auto_deliver"`
	}
	if err := json.Unmarshal(config["event_generation"], &eg); err != nil {
		t.Fatalf("failed to decode event_generation: %v", err)
	}

	// Config should retain the updated value after reset
	if eg.AutoDeliver != false {
		t.Errorf("expected auto_deliver=false to persist after reset, got %v", eg.AutoDeliver)
	}
}
