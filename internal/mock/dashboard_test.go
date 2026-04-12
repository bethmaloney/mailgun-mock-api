package mock_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bethmaloney/mailgun-mock-api/internal/domain"
	"github.com/bethmaloney/mailgun-mock-api/internal/event"
	"github.com/bethmaloney/mailgun-mock-api/internal/message"
	"github.com/bethmaloney/mailgun-mock-api/internal/mock"
	"github.com/bethmaloney/mailgun-mock-api/internal/webhook"
	"github.com/go-chi/chi/v5"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// Dashboard response types (mirroring the expected JSON shape)
// ---------------------------------------------------------------------------

type dashboardResponse struct {
	Messages  dashboardMessages  `json:"messages"`
	Events    dashboardEvents    `json:"events"`
	Domains   dashboardDomains   `json:"domains"`
	Webhooks  dashboardWebhooks  `json:"webhooks"`
}

type dashboardMessages struct {
	Total    int `json:"total"`
	LastHour int `json:"last_hour"`
}

type dashboardEvents struct {
	Accepted     int `json:"accepted"`
	Delivered    int `json:"delivered"`
	Failed       int `json:"failed"`
	Opened       int `json:"opened"`
	Clicked      int `json:"clicked"`
	Complained   int `json:"complained"`
	Unsubscribed int `json:"unsubscribed"`
}

type dashboardDomains struct {
	Total      int `json:"total"`
	Active     int `json:"active"`
	Unverified int `json:"unverified"`
}

type dashboardWebhooks struct {
	Configured       int                        `json:"configured"`
	RecentDeliveries []dashboardWebhookDelivery `json:"recent_deliveries"`
}

type dashboardWebhookDelivery struct {
	URL        string `json:"url"`
	Event      string `json:"event"`
	StatusCode int    `json:"status_code"`
	Timestamp  int64  `json:"timestamp"`
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// setupDashboardDB creates an in-memory SQLite database and runs all required
// migrations for models that the dashboard handler needs to query.
func setupDashboardDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}

	// Migrate all models the dashboard endpoint will query.
	err = db.AutoMigrate(
		&message.StoredMessage{},
		&event.Event{},
		&domain.Domain{},
		&domain.DNSRecord{},
		&webhook.DomainWebhook{},
		&webhook.AccountWebhook{},
		&webhook.WebhookDelivery{},
	)
	if err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	return db
}

// setupDashboardRouter creates a chi router with the /mock route group including
// the dashboard endpoint.
func setupDashboardRouter(h *mock.Handlers) http.Handler {
	r := chi.NewRouter()
	r.Route("/mock", func(r chi.Router) {
		r.Get("/dashboard", h.GetDashboard)
	})
	return r
}

// getDashboard makes a GET /mock/dashboard request and returns the recorder.
func getDashboard(t *testing.T, router http.Handler) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/mock/dashboard", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// decodeDashboard decodes the response body into a dashboardResponse.
func decodeDashboard(t *testing.T, rec *httptest.ResponseRecorder) dashboardResponse {
	t.Helper()
	var resp dashboardResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode dashboard response: %v\nbody: %s", err, rec.Body.String())
	}
	return resp
}

// ---------------------------------------------------------------------------
// Test 1: Empty database — all zeros, empty arrays
// ---------------------------------------------------------------------------

func TestGetDashboard_EmptyDatabase(t *testing.T) {
	db := setupDashboardDB(t)
	h := mock.NewHandlers(db, nil)
	router := setupDashboardRouter(h)

	rec := getDashboard(t, router)

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

	resp := decodeDashboard(t, rec)

	t.Run("messages counts are zero", func(t *testing.T) {
		if resp.Messages.Total != 0 {
			t.Errorf("expected messages.total=0, got %d", resp.Messages.Total)
		}
		if resp.Messages.LastHour != 0 {
			t.Errorf("expected messages.last_hour=0, got %d", resp.Messages.LastHour)
		}
	})

	t.Run("event counts are zero", func(t *testing.T) {
		if resp.Events.Accepted != 0 {
			t.Errorf("expected events.accepted=0, got %d", resp.Events.Accepted)
		}
		if resp.Events.Delivered != 0 {
			t.Errorf("expected events.delivered=0, got %d", resp.Events.Delivered)
		}
		if resp.Events.Failed != 0 {
			t.Errorf("expected events.failed=0, got %d", resp.Events.Failed)
		}
		if resp.Events.Opened != 0 {
			t.Errorf("expected events.opened=0, got %d", resp.Events.Opened)
		}
		if resp.Events.Clicked != 0 {
			t.Errorf("expected events.clicked=0, got %d", resp.Events.Clicked)
		}
		if resp.Events.Complained != 0 {
			t.Errorf("expected events.complained=0, got %d", resp.Events.Complained)
		}
		if resp.Events.Unsubscribed != 0 {
			t.Errorf("expected events.unsubscribed=0, got %d", resp.Events.Unsubscribed)
		}
	})

	t.Run("domain counts are zero", func(t *testing.T) {
		if resp.Domains.Total != 0 {
			t.Errorf("expected domains.total=0, got %d", resp.Domains.Total)
		}
		if resp.Domains.Active != 0 {
			t.Errorf("expected domains.active=0, got %d", resp.Domains.Active)
		}
		if resp.Domains.Unverified != 0 {
			t.Errorf("expected domains.unverified=0, got %d", resp.Domains.Unverified)
		}
	})

	t.Run("webhooks configured is zero with empty deliveries", func(t *testing.T) {
		if resp.Webhooks.Configured != 0 {
			t.Errorf("expected webhooks.configured=0, got %d", resp.Webhooks.Configured)
		}
		if resp.Webhooks.RecentDeliveries == nil {
			t.Fatal("expected webhooks.recent_deliveries to be an empty array, got nil")
		}
		if len(resp.Webhooks.RecentDeliveries) != 0 {
			t.Errorf("expected webhooks.recent_deliveries to be empty, got %d items", len(resp.Webhooks.RecentDeliveries))
		}
	})
}

// ---------------------------------------------------------------------------
// Test 2: With messages — verify total and last_hour counts
// ---------------------------------------------------------------------------

func TestGetDashboard_WithMessages(t *testing.T) {
	db := setupDashboardDB(t)
	h := mock.NewHandlers(db, nil)
	router := setupDashboardRouter(h)

	now := time.Now()

	// Create 5 messages: 3 within the last hour, 2 older than 1 hour.
	recentMessages := []message.StoredMessage{
		{DomainName: "example.com", MessageID: "<msg1@example.com>", StorageKey: "msg1@example.com", From: "a@example.com", To: "b@example.com", Subject: "Test 1"},
		{DomainName: "example.com", MessageID: "<msg2@example.com>", StorageKey: "msg2@example.com", From: "a@example.com", To: "b@example.com", Subject: "Test 2"},
		{DomainName: "other.com", MessageID: "<msg3@other.com>", StorageKey: "msg3@other.com", From: "a@other.com", To: "b@other.com", Subject: "Test 3"},
	}
	for _, m := range recentMessages {
		if err := db.Create(&m).Error; err != nil {
			t.Fatalf("failed to create recent message: %v", err)
		}
	}

	oldMessages := []message.StoredMessage{
		{DomainName: "example.com", MessageID: "<msg4@example.com>", StorageKey: "msg4@example.com", From: "a@example.com", To: "b@example.com", Subject: "Old 1"},
		{DomainName: "example.com", MessageID: "<msg5@example.com>", StorageKey: "msg5@example.com", From: "a@example.com", To: "b@example.com", Subject: "Old 2"},
	}
	for _, m := range oldMessages {
		if err := db.Create(&m).Error; err != nil {
			t.Fatalf("failed to create old message: %v", err)
		}
		// Manually set created_at to 2 hours ago
		db.Model(&message.StoredMessage{}).Where("id = ?", m.ID).Update("created_at", now.Add(-2*time.Hour))
	}

	rec := getDashboard(t, router)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	resp := decodeDashboard(t, rec)

	t.Run("total message count", func(t *testing.T) {
		if resp.Messages.Total != 5 {
			t.Errorf("expected messages.total=5, got %d", resp.Messages.Total)
		}
	})

	t.Run("last hour message count", func(t *testing.T) {
		if resp.Messages.LastHour != 3 {
			t.Errorf("expected messages.last_hour=3, got %d", resp.Messages.LastHour)
		}
	})
}

// ---------------------------------------------------------------------------
// Test 3: With events — verify each event type count
// ---------------------------------------------------------------------------

func TestGetDashboard_WithEvents(t *testing.T) {
	db := setupDashboardDB(t)
	h := mock.NewHandlers(db, nil)
	router := setupDashboardRouter(h)

	now := float64(time.Now().UnixMicro()) / 1e6

	// Create events of various types.
	events := []event.Event{
		{DomainName: "example.com", EventType: "accepted", Timestamp: now, LogLevel: "info", Recipient: "a@b.com"},
		{DomainName: "example.com", EventType: "accepted", Timestamp: now, LogLevel: "info", Recipient: "b@b.com"},
		{DomainName: "example.com", EventType: "accepted", Timestamp: now, LogLevel: "info", Recipient: "c@b.com"},
		{DomainName: "example.com", EventType: "delivered", Timestamp: now, LogLevel: "info", Recipient: "a@b.com"},
		{DomainName: "example.com", EventType: "delivered", Timestamp: now, LogLevel: "info", Recipient: "b@b.com"},
		{DomainName: "example.com", EventType: "failed", Timestamp: now, LogLevel: "error", Recipient: "c@b.com", Severity: "permanent", Reason: "bounce"},
		{DomainName: "example.com", EventType: "opened", Timestamp: now, LogLevel: "info", Recipient: "a@b.com"},
		{DomainName: "example.com", EventType: "clicked", Timestamp: now, LogLevel: "info", Recipient: "a@b.com"},
		{DomainName: "example.com", EventType: "complained", Timestamp: now, LogLevel: "warn", Recipient: "d@b.com"},
		{DomainName: "example.com", EventType: "unsubscribed", Timestamp: now, LogLevel: "warn", Recipient: "e@b.com"},
		{DomainName: "example.com", EventType: "unsubscribed", Timestamp: now, LogLevel: "warn", Recipient: "f@b.com"},
	}
	for _, ev := range events {
		if err := db.Create(&ev).Error; err != nil {
			t.Fatalf("failed to create event: %v", err)
		}
	}

	rec := getDashboard(t, router)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	resp := decodeDashboard(t, rec)

	t.Run("accepted count", func(t *testing.T) {
		if resp.Events.Accepted != 3 {
			t.Errorf("expected events.accepted=3, got %d", resp.Events.Accepted)
		}
	})

	t.Run("delivered count", func(t *testing.T) {
		if resp.Events.Delivered != 2 {
			t.Errorf("expected events.delivered=2, got %d", resp.Events.Delivered)
		}
	})

	t.Run("failed count", func(t *testing.T) {
		if resp.Events.Failed != 1 {
			t.Errorf("expected events.failed=1, got %d", resp.Events.Failed)
		}
	})

	t.Run("opened count", func(t *testing.T) {
		if resp.Events.Opened != 1 {
			t.Errorf("expected events.opened=1, got %d", resp.Events.Opened)
		}
	})

	t.Run("clicked count", func(t *testing.T) {
		if resp.Events.Clicked != 1 {
			t.Errorf("expected events.clicked=1, got %d", resp.Events.Clicked)
		}
	})

	t.Run("complained count", func(t *testing.T) {
		if resp.Events.Complained != 1 {
			t.Errorf("expected events.complained=1, got %d", resp.Events.Complained)
		}
	})

	t.Run("unsubscribed count", func(t *testing.T) {
		if resp.Events.Unsubscribed != 2 {
			t.Errorf("expected events.unsubscribed=2, got %d", resp.Events.Unsubscribed)
		}
	})
}

// ---------------------------------------------------------------------------
// Test 4: With domains — verify total, active, unverified
// ---------------------------------------------------------------------------

func TestGetDashboard_WithDomains(t *testing.T) {
	db := setupDashboardDB(t)
	h := mock.NewHandlers(db, nil)
	router := setupDashboardRouter(h)

	// Create 3 domains: 2 active, 1 unverified.
	domains := []domain.Domain{
		{Name: "active1.com", State: "active", Type: "custom", SMTPLogin: "postmaster@active1.com", SpamAction: "disabled"},
		{Name: "active2.com", State: "active", Type: "custom", SMTPLogin: "postmaster@active2.com", SpamAction: "disabled"},
		{Name: "pending.com", State: "unverified", Type: "custom", SMTPLogin: "postmaster@pending.com", SpamAction: "disabled"},
	}
	for _, d := range domains {
		if err := db.Create(&d).Error; err != nil {
			t.Fatalf("failed to create domain: %v", err)
		}
	}

	rec := getDashboard(t, router)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	resp := decodeDashboard(t, rec)

	t.Run("total domain count", func(t *testing.T) {
		if resp.Domains.Total != 3 {
			t.Errorf("expected domains.total=3, got %d", resp.Domains.Total)
		}
	})

	t.Run("active domain count", func(t *testing.T) {
		if resp.Domains.Active != 2 {
			t.Errorf("expected domains.active=2, got %d", resp.Domains.Active)
		}
	})

	t.Run("unverified domain count", func(t *testing.T) {
		if resp.Domains.Unverified != 1 {
			t.Errorf("expected domains.unverified=1, got %d", resp.Domains.Unverified)
		}
	})
}

// ---------------------------------------------------------------------------
// Test 5: With webhook deliveries — verify configured and recent_deliveries
// ---------------------------------------------------------------------------

func TestGetDashboard_WithWebhookDeliveries(t *testing.T) {
	db := setupDashboardDB(t)
	h := mock.NewHandlers(db, nil)
	router := setupDashboardRouter(h)

	// Create some domain webhooks (4 unique domain+event_type combos => configured=4).
	domainWebhooks := []webhook.DomainWebhook{
		{DomainName: "example.com", EventType: "delivered", URL: "http://localhost:3000/hooks"},
		{DomainName: "example.com", EventType: "opened", URL: "http://localhost:3000/hooks"},
		{DomainName: "example.com", EventType: "clicked", URL: "http://localhost:3000/hooks"},
		{DomainName: "other.com", EventType: "delivered", URL: "http://other.com/hooks"},
	}
	for _, wh := range domainWebhooks {
		if err := db.Create(&wh).Error; err != nil {
			t.Fatalf("failed to create domain webhook: %v", err)
		}
	}

	// Create 7 webhook deliveries; the response should only include the 5 most recent.
	now := time.Now()
	deliveries := []webhook.WebhookDelivery{
		{WebhookURL: "http://localhost:3000/hooks", EventType: "delivered", DomainName: "example.com", StatusCode: 200, Attempt: 1},
		{WebhookURL: "http://localhost:3000/hooks", EventType: "opened", DomainName: "example.com", StatusCode: 200, Attempt: 1},
		{WebhookURL: "http://localhost:3000/hooks", EventType: "clicked", DomainName: "example.com", StatusCode: 200, Attempt: 1},
		{WebhookURL: "http://other.com/hooks", EventType: "delivered", DomainName: "other.com", StatusCode: 500, Attempt: 1},
		{WebhookURL: "http://localhost:3000/hooks", EventType: "delivered", DomainName: "example.com", StatusCode: 200, Attempt: 1},
		{WebhookURL: "http://other.com/hooks", EventType: "delivered", DomainName: "other.com", StatusCode: 200, Attempt: 1},
		{WebhookURL: "http://localhost:3000/hooks", EventType: "opened", DomainName: "example.com", StatusCode: 200, Attempt: 1},
	}
	for i, d := range deliveries {
		if err := db.Create(&d).Error; err != nil {
			t.Fatalf("failed to create webhook delivery: %v", err)
		}
		// Stagger created_at so ordering is deterministic
		db.Model(&webhook.WebhookDelivery{}).Where("id = ?", d.ID).Update("created_at", now.Add(time.Duration(i)*time.Minute))
	}

	rec := getDashboard(t, router)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	resp := decodeDashboard(t, rec)

	t.Run("configured webhook count", func(t *testing.T) {
		if resp.Webhooks.Configured != 4 {
			t.Errorf("expected webhooks.configured=4, got %d", resp.Webhooks.Configured)
		}
	})

	t.Run("recent deliveries limited to 5", func(t *testing.T) {
		if len(resp.Webhooks.RecentDeliveries) != 5 {
			t.Errorf("expected 5 recent_deliveries, got %d", len(resp.Webhooks.RecentDeliveries))
		}
	})

	t.Run("recent deliveries are ordered most recent first", func(t *testing.T) {
		if len(resp.Webhooks.RecentDeliveries) < 2 {
			t.Skip("not enough deliveries to check ordering")
		}
		// The most recent delivery should be the last one we created (index 6, latest created_at)
		first := resp.Webhooks.RecentDeliveries[0]
		second := resp.Webhooks.RecentDeliveries[1]
		if first.Timestamp < second.Timestamp {
			t.Errorf("expected recent_deliveries sorted descending by timestamp, got first=%d < second=%d", first.Timestamp, second.Timestamp)
		}
	})

	t.Run("each delivery has url and event fields", func(t *testing.T) {
		for i, d := range resp.Webhooks.RecentDeliveries {
			if d.URL == "" {
				t.Errorf("delivery[%d].url is empty", i)
			}
			if d.Event == "" {
				t.Errorf("delivery[%d].event is empty", i)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Test 6: Combined data — verify the full response shape
// ---------------------------------------------------------------------------

func TestGetDashboard_CombinedData(t *testing.T) {
	db := setupDashboardDB(t)
	h := mock.NewHandlers(db, nil)
	router := setupDashboardRouter(h)

	now := time.Now()
	nowTS := float64(now.UnixMicro()) / 1e6

	// -- Messages: 4 total, 2 in last hour --
	msgs := []message.StoredMessage{
		{DomainName: "example.com", MessageID: "<c1@example.com>", StorageKey: "c1@example.com", From: "a@example.com", To: "b@example.com", Subject: "Combined 1"},
		{DomainName: "example.com", MessageID: "<c2@example.com>", StorageKey: "c2@example.com", From: "a@example.com", To: "b@example.com", Subject: "Combined 2"},
		{DomainName: "example.com", MessageID: "<c3@example.com>", StorageKey: "c3@example.com", From: "a@example.com", To: "b@example.com", Subject: "Combined 3"},
		{DomainName: "example.com", MessageID: "<c4@example.com>", StorageKey: "c4@example.com", From: "a@example.com", To: "b@example.com", Subject: "Combined 4"},
	}
	for i, m := range msgs {
		if err := db.Create(&m).Error; err != nil {
			t.Fatalf("failed to create message: %v", err)
		}
		// Make the last 2 messages older than 1 hour
		if i >= 2 {
			db.Model(&message.StoredMessage{}).Where("id = ?", m.ID).Update("created_at", now.Add(-3*time.Hour))
		}
	}

	// -- Events --
	evts := []event.Event{
		{DomainName: "example.com", EventType: "accepted", Timestamp: nowTS, LogLevel: "info", Recipient: "a@b.com"},
		{DomainName: "example.com", EventType: "accepted", Timestamp: nowTS, LogLevel: "info", Recipient: "b@b.com"},
		{DomainName: "example.com", EventType: "delivered", Timestamp: nowTS, LogLevel: "info", Recipient: "a@b.com"},
		{DomainName: "example.com", EventType: "failed", Timestamp: nowTS, LogLevel: "error", Recipient: "c@b.com", Severity: "permanent"},
		{DomainName: "example.com", EventType: "opened", Timestamp: nowTS, LogLevel: "info", Recipient: "a@b.com"},
		{DomainName: "example.com", EventType: "clicked", Timestamp: nowTS, LogLevel: "info", Recipient: "a@b.com"},
		{DomainName: "example.com", EventType: "complained", Timestamp: nowTS, LogLevel: "warn", Recipient: "d@b.com"},
		{DomainName: "example.com", EventType: "unsubscribed", Timestamp: nowTS, LogLevel: "warn", Recipient: "e@b.com"},
	}
	for _, ev := range evts {
		if err := db.Create(&ev).Error; err != nil {
			t.Fatalf("failed to create event: %v", err)
		}
	}

	// -- Domains: 3 total, 2 active, 1 unverified --
	doms := []domain.Domain{
		{Name: "combined1.com", State: "active", Type: "custom", SMTPLogin: "postmaster@combined1.com", SpamAction: "disabled"},
		{Name: "combined2.com", State: "active", Type: "custom", SMTPLogin: "postmaster@combined2.com", SpamAction: "disabled"},
		{Name: "combined3.com", State: "unverified", Type: "custom", SMTPLogin: "postmaster@combined3.com", SpamAction: "disabled"},
	}
	for _, d := range doms {
		if err := db.Create(&d).Error; err != nil {
			t.Fatalf("failed to create domain: %v", err)
		}
	}

	// -- Domain webhooks: 2 configured --
	whs := []webhook.DomainWebhook{
		{DomainName: "combined1.com", EventType: "delivered", URL: "http://hooks.example.com/a"},
		{DomainName: "combined2.com", EventType: "opened", URL: "http://hooks.example.com/b"},
	}
	for _, wh := range whs {
		if err := db.Create(&wh).Error; err != nil {
			t.Fatalf("failed to create webhook: %v", err)
		}
	}

	// -- Webhook deliveries: 3 --
	dels := []webhook.WebhookDelivery{
		{WebhookURL: "http://hooks.example.com/a", EventType: "delivered", DomainName: "combined1.com", StatusCode: 200, Attempt: 1},
		{WebhookURL: "http://hooks.example.com/b", EventType: "opened", DomainName: "combined2.com", StatusCode: 200, Attempt: 1},
		{WebhookURL: "http://hooks.example.com/a", EventType: "delivered", DomainName: "combined1.com", StatusCode: 500, Attempt: 1},
	}
	for i, d := range dels {
		if err := db.Create(&d).Error; err != nil {
			t.Fatalf("failed to create webhook delivery: %v", err)
		}
		db.Model(&webhook.WebhookDelivery{}).Where("id = ?", d.ID).Update("created_at", now.Add(time.Duration(i)*time.Minute))
	}

	rec := getDashboard(t, router)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	resp := decodeDashboard(t, rec)

	// Verify every section of the response.

	t.Run("messages", func(t *testing.T) {
		if resp.Messages.Total != 4 {
			t.Errorf("expected messages.total=4, got %d", resp.Messages.Total)
		}
		if resp.Messages.LastHour != 2 {
			t.Errorf("expected messages.last_hour=2, got %d", resp.Messages.LastHour)
		}
	})

	t.Run("events", func(t *testing.T) {
		if resp.Events.Accepted != 2 {
			t.Errorf("expected events.accepted=2, got %d", resp.Events.Accepted)
		}
		if resp.Events.Delivered != 1 {
			t.Errorf("expected events.delivered=1, got %d", resp.Events.Delivered)
		}
		if resp.Events.Failed != 1 {
			t.Errorf("expected events.failed=1, got %d", resp.Events.Failed)
		}
		if resp.Events.Opened != 1 {
			t.Errorf("expected events.opened=1, got %d", resp.Events.Opened)
		}
		if resp.Events.Clicked != 1 {
			t.Errorf("expected events.clicked=1, got %d", resp.Events.Clicked)
		}
		if resp.Events.Complained != 1 {
			t.Errorf("expected events.complained=1, got %d", resp.Events.Complained)
		}
		if resp.Events.Unsubscribed != 1 {
			t.Errorf("expected events.unsubscribed=1, got %d", resp.Events.Unsubscribed)
		}
	})

	t.Run("domains", func(t *testing.T) {
		if resp.Domains.Total != 3 {
			t.Errorf("expected domains.total=3, got %d", resp.Domains.Total)
		}
		if resp.Domains.Active != 2 {
			t.Errorf("expected domains.active=2, got %d", resp.Domains.Active)
		}
		if resp.Domains.Unverified != 1 {
			t.Errorf("expected domains.unverified=1, got %d", resp.Domains.Unverified)
		}
	})

	t.Run("webhooks", func(t *testing.T) {
		if resp.Webhooks.Configured != 2 {
			t.Errorf("expected webhooks.configured=2, got %d", resp.Webhooks.Configured)
		}
		if len(resp.Webhooks.RecentDeliveries) != 3 {
			t.Errorf("expected 3 recent_deliveries, got %d", len(resp.Webhooks.RecentDeliveries))
		}
	})
}

// ---------------------------------------------------------------------------
// Test 7: Response JSON structure has all expected top-level keys
// ---------------------------------------------------------------------------

func TestGetDashboard_ResponseStructure(t *testing.T) {
	db := setupDashboardDB(t)
	h := mock.NewHandlers(db, nil)
	router := setupDashboardRouter(h)

	rec := getDashboard(t, router)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	// Decode as raw map to verify structural keys.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("failed to decode response as raw JSON: %v", err)
	}

	expectedKeys := []string{"messages", "events", "domains", "webhooks"}
	for _, key := range expectedKeys {
		t.Run("has top-level key: "+key, func(t *testing.T) {
			if _, ok := raw[key]; !ok {
				t.Errorf("response missing top-level key %q", key)
			}
		})
	}

	t.Run("has exactly 4 top-level keys", func(t *testing.T) {
		if len(raw) != 4 {
			t.Errorf("expected 4 top-level keys, got %d: %v", len(raw), keys(raw))
		}
	})

	// Verify messages sub-structure
	t.Run("messages has expected fields", func(t *testing.T) {
		var msgRaw map[string]json.RawMessage
		if err := json.Unmarshal(raw["messages"], &msgRaw); err != nil {
			t.Fatalf("failed to decode messages: %v", err)
		}
		for _, field := range []string{"total", "last_hour"} {
			if _, ok := msgRaw[field]; !ok {
				t.Errorf("messages missing field %q", field)
			}
		}
	})

	// Verify events sub-structure
	t.Run("events has expected fields", func(t *testing.T) {
		var evtRaw map[string]json.RawMessage
		if err := json.Unmarshal(raw["events"], &evtRaw); err != nil {
			t.Fatalf("failed to decode events: %v", err)
		}
		for _, field := range []string{"accepted", "delivered", "failed", "opened", "clicked", "complained", "unsubscribed"} {
			if _, ok := evtRaw[field]; !ok {
				t.Errorf("events missing field %q", field)
			}
		}
	})

	// Verify domains sub-structure
	t.Run("domains has expected fields", func(t *testing.T) {
		var domRaw map[string]json.RawMessage
		if err := json.Unmarshal(raw["domains"], &domRaw); err != nil {
			t.Fatalf("failed to decode domains: %v", err)
		}
		for _, field := range []string{"total", "active", "unverified"} {
			if _, ok := domRaw[field]; !ok {
				t.Errorf("domains missing field %q", field)
			}
		}
	})

	// Verify webhooks sub-structure
	t.Run("webhooks has expected fields", func(t *testing.T) {
		var whRaw map[string]json.RawMessage
		if err := json.Unmarshal(raw["webhooks"], &whRaw); err != nil {
			t.Fatalf("failed to decode webhooks: %v", err)
		}
		for _, field := range []string{"configured", "recent_deliveries"} {
			if _, ok := whRaw[field]; !ok {
				t.Errorf("webhooks missing field %q", field)
			}
		}
	})
}

// keys returns the keys of a map for diagnostics.
func keys(m map[string]json.RawMessage) []string {
	result := make([]string, 0, len(m))
	for k := range m {
		result = append(result, k)
	}
	return result
}

// ---------------------------------------------------------------------------
// Test 8: Domains with disabled state are not counted as active or unverified
// ---------------------------------------------------------------------------

func TestGetDashboard_DisabledDomainsCountedInTotal(t *testing.T) {
	db := setupDashboardDB(t)
	h := mock.NewHandlers(db, nil)
	router := setupDashboardRouter(h)

	// Create domains with mixed states including "disabled".
	domains := []domain.Domain{
		{Name: "active.com", State: "active", Type: "custom", SMTPLogin: "postmaster@active.com", SpamAction: "disabled"},
		{Name: "unverified.com", State: "unverified", Type: "custom", SMTPLogin: "postmaster@unverified.com", SpamAction: "disabled"},
		{Name: "disabled.com", State: "disabled", Type: "custom", SMTPLogin: "postmaster@disabled.com", SpamAction: "disabled", IsDisabled: true},
	}
	for _, d := range domains {
		if err := db.Create(&d).Error; err != nil {
			t.Fatalf("failed to create domain: %v", err)
		}
	}

	rec := getDashboard(t, router)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	resp := decodeDashboard(t, rec)

	t.Run("total includes all domains regardless of state", func(t *testing.T) {
		if resp.Domains.Total != 3 {
			t.Errorf("expected domains.total=3, got %d", resp.Domains.Total)
		}
	})

	t.Run("active count only includes active domains", func(t *testing.T) {
		if resp.Domains.Active != 1 {
			t.Errorf("expected domains.active=1, got %d", resp.Domains.Active)
		}
	})

	t.Run("unverified count only includes unverified domains", func(t *testing.T) {
		if resp.Domains.Unverified != 1 {
			t.Errorf("expected domains.unverified=1, got %d", resp.Domains.Unverified)
		}
	})
}

// ---------------------------------------------------------------------------
// Test 9: Webhook configured counts both domain and account webhooks
// ---------------------------------------------------------------------------

func TestGetDashboard_WebhookConfiguredCountsAccountWebhooks(t *testing.T) {
	db := setupDashboardDB(t)
	h := mock.NewHandlers(db, nil)
	router := setupDashboardRouter(h)

	// Create domain webhooks.
	domainWHs := []webhook.DomainWebhook{
		{DomainName: "example.com", EventType: "delivered", URL: "http://example.com/hook1"},
		{DomainName: "example.com", EventType: "opened", URL: "http://example.com/hook2"},
	}
	for _, wh := range domainWHs {
		if err := db.Create(&wh).Error; err != nil {
			t.Fatalf("failed to create domain webhook: %v", err)
		}
	}

	// Create account-level webhooks.
	accountWHs := []webhook.AccountWebhook{
		{WebhookID: "wh_test1", URL: "http://account.com/hook1", EventTypes: `["delivered","opened"]`, Description: "Test 1"},
		{WebhookID: "wh_test2", URL: "http://account.com/hook2", EventTypes: `["failed"]`, Description: "Test 2"},
	}
	for _, aw := range accountWHs {
		if err := db.Create(&aw).Error; err != nil {
			t.Fatalf("failed to create account webhook: %v", err)
		}
	}

	rec := getDashboard(t, router)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	resp := decodeDashboard(t, rec)

	t.Run("configured counts domain plus account webhooks", func(t *testing.T) {
		// 2 domain webhooks + 2 account webhooks = 4 total configured
		if resp.Webhooks.Configured != 4 {
			t.Errorf("expected webhooks.configured=4 (2 domain + 2 account), got %d", resp.Webhooks.Configured)
		}
	})
}

// ---------------------------------------------------------------------------
// Test 10: Webhook recent_deliveries contains correct fields
// ---------------------------------------------------------------------------

func TestGetDashboard_WebhookDeliveryFields(t *testing.T) {
	db := setupDashboardDB(t)
	h := mock.NewHandlers(db, nil)
	router := setupDashboardRouter(h)

	// Create a single webhook delivery with known values.
	delivery := webhook.WebhookDelivery{
		WebhookURL: "http://localhost:3000/hooks",
		EventType:  "delivered",
		DomainName: "example.com",
		StatusCode: 200,
		Attempt:    1,
	}
	if err := db.Create(&delivery).Error; err != nil {
		t.Fatalf("failed to create webhook delivery: %v", err)
	}

	rec := getDashboard(t, router)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	resp := decodeDashboard(t, rec)

	if len(resp.Webhooks.RecentDeliveries) != 1 {
		t.Fatalf("expected 1 recent delivery, got %d", len(resp.Webhooks.RecentDeliveries))
	}

	d := resp.Webhooks.RecentDeliveries[0]

	t.Run("url field", func(t *testing.T) {
		if d.URL != "http://localhost:3000/hooks" {
			t.Errorf("expected url=%q, got %q", "http://localhost:3000/hooks", d.URL)
		}
	})

	t.Run("event field", func(t *testing.T) {
		if d.Event != "delivered" {
			t.Errorf("expected event=%q, got %q", "delivered", d.Event)
		}
	})

	t.Run("status_code field", func(t *testing.T) {
		if d.StatusCode != 200 {
			t.Errorf("expected status_code=200, got %d", d.StatusCode)
		}
	})

	t.Run("timestamp field is nonzero", func(t *testing.T) {
		if d.Timestamp == 0 {
			t.Error("expected timestamp to be nonzero")
		}
	})
}
