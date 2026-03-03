package mock

import (
	"net/http"
	"time"

	"github.com/bethmaloney/mailgun-mock-api/internal/database"
	"github.com/bethmaloney/mailgun-mock-api/internal/response"
)

// ---------------------------------------------------------------------------
// Lookup structs — lightweight models used to query other packages' tables
// without importing those packages (avoids circular imports).
// ---------------------------------------------------------------------------

type messageLookup struct {
	database.BaseModel
}

func (messageLookup) TableName() string { return "stored_messages" }

type eventLookup struct {
	database.BaseModel
	EventType string
}

func (eventLookup) TableName() string { return "events" }

type domainLookup struct {
	database.BaseModel
	State string
}

func (domainLookup) TableName() string { return "domains" }

type domainWebhookLookup struct {
	database.BaseModel
}

func (domainWebhookLookup) TableName() string { return "domain_webhooks" }

type accountWebhookLookup struct {
	database.BaseModel
}

func (accountWebhookLookup) TableName() string { return "account_webhooks" }

type webhookDeliveryLookup struct {
	database.BaseModel
	WebhookURL string
	EventType  string
	StatusCode int
}

func (webhookDeliveryLookup) TableName() string { return "webhook_deliveries" }

// ---------------------------------------------------------------------------
// Dashboard response types
// ---------------------------------------------------------------------------

type dashboardMessagesResponse struct {
	Total    int `json:"total"`
	LastHour int `json:"last_hour"`
}

type dashboardEventsResponse struct {
	Accepted     int `json:"accepted"`
	Delivered    int `json:"delivered"`
	Failed       int `json:"failed"`
	Opened       int `json:"opened"`
	Clicked      int `json:"clicked"`
	Complained   int `json:"complained"`
	Unsubscribed int `json:"unsubscribed"`
}

type dashboardDomainsResponse struct {
	Total      int `json:"total"`
	Active     int `json:"active"`
	Unverified int `json:"unverified"`
}

type dashboardWebhookDeliveryResponse struct {
	URL        string `json:"url"`
	Event      string `json:"event"`
	StatusCode int    `json:"status_code"`
	Timestamp  int64  `json:"timestamp"`
}

type dashboardWebhooksResponse struct {
	Configured       int                                `json:"configured"`
	RecentDeliveries []dashboardWebhookDeliveryResponse `json:"recent_deliveries"`
}

type dashboardResponse struct {
	Messages dashboardMessagesResponse `json:"messages"`
	Events   dashboardEventsResponse   `json:"events"`
	Domains  dashboardDomainsResponse  `json:"domains"`
	Webhooks dashboardWebhooksResponse `json:"webhooks"`
}

// GetDashboard returns aggregate counts for the web UI dashboard.
func (h *Handlers) GetDashboard(w http.ResponseWriter, r *http.Request) {
	var resp dashboardResponse

	// --- Messages ---
	var msgTotal int64
	h.db.Model(&messageLookup{}).Count(&msgTotal)
	resp.Messages.Total = int(msgTotal)

	var msgLastHour int64
	oneHourAgo := time.Now().Add(-1 * time.Hour)
	h.db.Model(&messageLookup{}).Where("created_at > ?", oneHourAgo).Count(&msgLastHour)
	resp.Messages.LastHour = int(msgLastHour)

	// --- Events — group by event_type ---
	type eventCount struct {
		EventType string
		Count     int
	}
	var eventCounts []eventCount
	h.db.Model(&eventLookup{}).
		Select("event_type, COUNT(*) as count").
		Group("event_type").
		Find(&eventCounts)

	for _, ec := range eventCounts {
		switch ec.EventType {
		case "accepted":
			resp.Events.Accepted = ec.Count
		case "delivered":
			resp.Events.Delivered = ec.Count
		case "failed":
			resp.Events.Failed = ec.Count
		case "opened":
			resp.Events.Opened = ec.Count
		case "clicked":
			resp.Events.Clicked = ec.Count
		case "complained":
			resp.Events.Complained = ec.Count
		case "unsubscribed":
			resp.Events.Unsubscribed = ec.Count
		}
	}

	// --- Domains ---
	var domTotal int64
	h.db.Model(&domainLookup{}).Count(&domTotal)
	resp.Domains.Total = int(domTotal)

	var domActive int64
	h.db.Model(&domainLookup{}).Where("state = ?", "active").Count(&domActive)
	resp.Domains.Active = int(domActive)

	var domUnverified int64
	h.db.Model(&domainLookup{}).Where("state = ?", "unverified").Count(&domUnverified)
	resp.Domains.Unverified = int(domUnverified)

	// --- Webhooks: configured (domain + account) ---
	var domainWHCount int64
	h.db.Model(&domainWebhookLookup{}).Count(&domainWHCount)

	var accountWHCount int64
	h.db.Model(&accountWebhookLookup{}).Count(&accountWHCount)

	resp.Webhooks.Configured = int(domainWHCount + accountWHCount)

	// --- Webhook recent deliveries (last 5, most recent first) ---
	var deliveries []webhookDeliveryLookup
	h.db.Model(&webhookDeliveryLookup{}).
		Order("created_at DESC").
		Limit(5).
		Find(&deliveries)

	resp.Webhooks.RecentDeliveries = make([]dashboardWebhookDeliveryResponse, 0, len(deliveries))
	for _, d := range deliveries {
		resp.Webhooks.RecentDeliveries = append(resp.Webhooks.RecentDeliveries, dashboardWebhookDeliveryResponse{
			URL:        d.WebhookURL,
			Event:      d.EventType,
			StatusCode: d.StatusCode,
			Timestamp:  d.CreatedAt.Unix(),
		})
	}

	response.RespondJSON(w, http.StatusOK, resp)
}
