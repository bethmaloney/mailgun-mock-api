package mock

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/bethmaloney/mailgun-mock-api/internal/response"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// HealthHandler returns a simple health check response.
func HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// EventGenerationConfig holds event generation settings.
type EventGenerationConfig struct {
	AutoDeliver               bool    `json:"auto_deliver"`
	DeliveryDelayMs           int     `json:"delivery_delay_ms"`
	DefaultDeliveryStatusCode int     `json:"default_delivery_status_code"`
	AutoFailRate              float64 `json:"auto_fail_rate"`
}

// DomainBehaviorConfig holds domain behavior settings.
type DomainBehaviorConfig struct {
	DomainAutoVerify bool   `json:"domain_auto_verify"`
	SandboxDomain    string `json:"sandbox_domain"`
}

// WebhookDeliveryConfig holds webhook delivery settings.
type WebhookDeliveryConfig struct {
	WebhookRetryMode string `json:"webhook_retry_mode"`
	WebhookTimeoutMs int    `json:"webhook_timeout_ms"`
}

// AuthenticationConfig holds authentication settings.
type AuthenticationConfig struct {
	AuthMode   string `json:"auth_mode"`
	SigningKey  string `json:"signing_key"`
}

// StorageConfig holds storage settings.
type StorageConfig struct {
	StoreAttachmentBytes bool `json:"store_attachment_bytes"`
	MaxMessages          int  `json:"max_messages"`
	MaxEvents            int  `json:"max_events"`
}

// MockConfig holds the full mock configuration.
type MockConfig struct {
	EventGeneration EventGenerationConfig `json:"event_generation"`
	DomainBehavior  DomainBehaviorConfig  `json:"domain_behavior"`
	WebhookDelivery WebhookDeliveryConfig `json:"webhook_delivery"`
	Authentication  AuthenticationConfig  `json:"authentication"`
	Storage         StorageConfig         `json:"storage"`
}

// defaultConfig returns a MockConfig initialized with default values.
func defaultConfig() MockConfig {
	return MockConfig{
		EventGeneration: EventGenerationConfig{
			AutoDeliver:               true,
			DeliveryDelayMs:           0,
			DefaultDeliveryStatusCode: 250,
			AutoFailRate:              0.0,
		},
		DomainBehavior: DomainBehaviorConfig{
			DomainAutoVerify: true,
			SandboxDomain:    "sandbox123.mailgun.org",
		},
		WebhookDelivery: WebhookDeliveryConfig{
			WebhookRetryMode: "immediate",
			WebhookTimeoutMs: 5000,
		},
		Authentication: AuthenticationConfig{
			AuthMode:   "accept_any",
			SigningKey:  "key-mock-signing-key-000000000000",
		},
		Storage: StorageConfig{
			StoreAttachmentBytes: false,
			MaxMessages:          0,
			MaxEvents:            0,
		},
	}
}

// Handlers holds the database connection and mock configuration.
type Handlers struct {
	db     *gorm.DB
	config MockConfig
}

// NewHandlers creates a new Handlers instance with default configuration.
func NewHandlers(db *gorm.DB) *Handlers {
	return &Handlers{
		db:     db,
		config: defaultConfig(),
	}
}

// Config returns a pointer to the mock configuration.
func (h *Handlers) Config() *MockConfig {
	return &h.config
}

// GetConfig returns the current mock configuration as JSON.
func (h *Handlers) GetConfig(w http.ResponseWriter, r *http.Request) {
	response.RespondJSON(w, http.StatusOK, h.config)
}

// UpdateConfig applies a partial update to the mock configuration.
func (h *Handlers) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		response.RespondError(w, http.StatusBadRequest, "Failed to read request body")
		return
	}
	defer r.Body.Close()

	if len(body) == 0 {
		response.RespondError(w, http.StatusBadRequest, "Request body is empty")
		return
	}

	// Parse the incoming JSON into a raw map to identify which sections are provided.
	var incoming map[string]json.RawMessage
	if err := json.Unmarshal(body, &incoming); err != nil {
		response.RespondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}

	// For each section present in the incoming request, merge it into the current config.
	if raw, ok := incoming["event_generation"]; ok {
		if err := mergeJSON(raw, &h.config.EventGeneration); err != nil {
			response.RespondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid event_generation: %v", err))
			return
		}
	}
	if raw, ok := incoming["domain_behavior"]; ok {
		if err := mergeJSON(raw, &h.config.DomainBehavior); err != nil {
			response.RespondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid domain_behavior: %v", err))
			return
		}
	}
	if raw, ok := incoming["webhook_delivery"]; ok {
		if err := mergeJSON(raw, &h.config.WebhookDelivery); err != nil {
			response.RespondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid webhook_delivery: %v", err))
			return
		}
	}
	if raw, ok := incoming["authentication"]; ok {
		if err := mergeJSON(raw, &h.config.Authentication); err != nil {
			response.RespondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid authentication: %v", err))
			return
		}
	}
	if raw, ok := incoming["storage"]; ok {
		if err := mergeJSON(raw, &h.config.Storage); err != nil {
			response.RespondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid storage: %v", err))
			return
		}
	}

	response.RespondJSON(w, http.StatusOK, h.config)
}

// mergeJSON unmarshals raw JSON on top of an existing struct, overwriting only
// the fields present in the JSON while keeping other fields at their current values.
// Returns an error if the JSON cannot be unmarshalled into the target.
func mergeJSON(raw json.RawMessage, target interface{}) error {
	return json.Unmarshal(raw, target)
}

// ResetAll resets all stored data but preserves configuration.
func (h *Handlers) ResetAll(w http.ResponseWriter, r *http.Request) {
	// TODO: Clear stored messages, events, and other data from the database.
	// Config is intentionally NOT reset.
	response.RespondSuccess(w, "All data has been reset")
}

// ResetDomain resets data for a specific domain.
func (h *Handlers) ResetDomain(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	// TODO: Clear stored data for the specified domain.
	response.RespondSuccess(w, fmt.Sprintf("Data for domain %s has been reset", domain))
}

// ResetMessages resets all stored messages and events.
func (h *Handlers) ResetMessages(w http.ResponseWriter, r *http.Request) {
	// TODO: Clear stored messages and events from the database.
	response.RespondSuccess(w, "Messages and events have been reset")
}
