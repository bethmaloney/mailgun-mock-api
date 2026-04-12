package mock

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/bethmaloney/mailgun-mock-api/internal/response"
	ws "github.com/bethmaloney/mailgun-mock-api/internal/websocket"
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
	hub    *ws.Hub
}

// SetHub sets the WebSocket hub used for broadcasting events.
func (h *Handlers) SetHub(hub *ws.Hub) {
	h.hub = hub
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

// resetTables is the list of all tables to truncate on reset,
// ordered children-first to respect foreign key constraints.
var resetTables = []string{
	// Children / join tables first
	"sending_limits",
	"ip_pool_ips",
	"domain_ips",
	"domain_pools",
	"dns_records",
	"smtp_credentials",
	"attachments",
	"template_versions",
	"mailing_list_members",
	"webhook_deliveries",
	// Then parents / standalone tables
	"events",
	"stored_messages",
	"bounces",
	"complaints",
	"unsubscribes",
	"allowlist_entries",
	"templates",
	"tags",
	"mailing_lists",
	"domain_webhooks",
	"account_webhooks",
	"routes",
	"api_keys",
	"ip_allowlist_entries",
	"ips",
	"ip_pools",
	"subaccounts",
	"domains",
}

// ResetAll clears all stored data and resets configuration to defaults.
func (h *Handlers) ResetAll(w http.ResponseWriter, r *http.Request) {
	for _, table := range resetTables {
		if err := h.db.Exec("DELETE FROM " + table).Error; err != nil {
			// Skip tables that don't exist (e.g. in test DBs without full migrations).
			if strings.Contains(err.Error(), "no such table") {
				continue
			}
			response.RespondError(w, http.StatusInternalServerError,
				fmt.Sprintf("failed to clear table %s: %v", table, err))
			return
		}
	}
	h.config = defaultConfig()
	response.RespondSuccess(w, "All data has been reset")
	if h.hub != nil {
		h.hub.Publish(ws.BroadcastMessage{Type: "data.reset", Data: nil})
	}
}

// ResetDomain resets data for a specific domain.
func (h *Handlers) ResetDomain(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")
	// TODO: Clear stored data for the specified domain.
	response.RespondSuccess(w, fmt.Sprintf("Data for domain %s has been reset", domain))
}

// ResetMessages clears all stored messages and events.
func (h *Handlers) ResetMessages(w http.ResponseWriter, r *http.Request) {
	for _, table := range []string{"attachments", "events", "stored_messages"} {
		if err := h.db.Exec("DELETE FROM " + table).Error; err != nil {
			if strings.Contains(err.Error(), "no such table") {
				continue
			}
			response.RespondError(w, http.StatusInternalServerError,
				fmt.Sprintf("failed to clear table %s: %v", table, err))
			return
		}
	}
	response.RespondSuccess(w, "Messages and events have been reset")
}
