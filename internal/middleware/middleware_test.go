package middleware_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bethmaloney/mailgun-mock-api/internal/middleware"
	"github.com/bethmaloney/mailgun-mock-api/internal/subaccount"
	"github.com/go-chi/chi/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// Helpers for SubaccountScoping tests
// ---------------------------------------------------------------------------

// setupSubaccountDB creates an in-memory SQLite database for testing with the
// subaccounts table migrated.
func setupSubaccountDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}
	if err := db.AutoMigrate(&subaccount.Subaccount{}); err != nil {
		t.Fatalf("failed to migrate subaccounts table: %v", err)
	}
	return db
}

// createTestSubaccount inserts a subaccount record directly into the database
// and returns it. The SubaccountID and Status fields are required.
func createTestSubaccount(t *testing.T, db *gorm.DB, subaccountID, name, status string) subaccount.Subaccount {
	t.Helper()
	sa := subaccount.Subaccount{
		SubaccountID: subaccountID,
		Name:         name,
		Status:       status,
	}
	if err := db.Create(&sa).Error; err != nil {
		t.Fatalf("failed to create test subaccount %q: %v", subaccountID, err)
	}
	return sa
}

// subaccountEchoHandler is a handler that reads SubaccountFromContext and writes
// the value to the response body as JSON. This lets tests verify the context
// value set by the middleware.
var subaccountEchoHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	saID := middleware.SubaccountFromContext(r.Context())
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"subaccount_id": saID})
})

// newSubaccountScopedRouter creates a chi router with SubaccountScoping middleware
// applied, and a test handler at GET /test.
func newSubaccountScopedRouter(db *gorm.DB) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.SubaccountScoping(db))
	r.Get("/test", subaccountEchoHandler)
	return r
}

// parseSubaccountResponse decodes the subaccount_id from the JSON response body.
func parseSubaccountResponse(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	return body["subaccount_id"]
}

// parseMessageResponse decodes the message from the JSON error response body.
func parseMessageResponse(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode error response body: %v", err)
	}
	msg, ok := body["message"]
	if !ok {
		t.Fatal("error response missing 'message' key")
	}
	return msg
}

// ---------------------------------------------------------------------------
// SubaccountScoping Middleware Tests
// ---------------------------------------------------------------------------

func TestSubaccountScoping_NoHeader_HandlerRuns(t *testing.T) {
	db := setupSubaccountDB(t)
	router := newSubaccountScopedRouter(db)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	t.Run("returns 200 when no header is present", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("SubaccountFromContext returns empty string", func(t *testing.T) {
		saID := parseSubaccountResponse(t, rec)
		if saID != "" {
			t.Errorf("expected empty subaccount_id, got %q", saID)
		}
	})
}

func TestSubaccountScoping_ValidSubaccount_HandlerRuns(t *testing.T) {
	db := setupSubaccountDB(t)
	createTestSubaccount(t, db, "sa_abc123def456789012", "Test Subaccount", "open")
	router := newSubaccountScopedRouter(db)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Mailgun-On-Behalf-Of", "sa_abc123def456789012")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	t.Run("returns 200 for valid subaccount", func(t *testing.T) {
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("SubaccountFromContext returns the subaccount ID", func(t *testing.T) {
		saID := parseSubaccountResponse(t, rec)
		if saID != "sa_abc123def456789012" {
			t.Errorf("expected subaccount_id %q, got %q", "sa_abc123def456789012", saID)
		}
	})
}

func TestSubaccountScoping_NonExistentSubaccount_Returns400(t *testing.T) {
	db := setupSubaccountDB(t)
	router := newSubaccountScopedRouter(db)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Mailgun-On-Behalf-Of", "sa_doesnotexist000000")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	t.Run("returns 400 for non-existent subaccount", func(t *testing.T) {
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("returns Invalid subaccount message", func(t *testing.T) {
		msg := parseMessageResponse(t, rec)
		if msg != "Invalid subaccount" {
			t.Errorf("expected message %q, got %q", "Invalid subaccount", msg)
		}
	})
}

func TestSubaccountScoping_DisabledSubaccount_Returns403(t *testing.T) {
	db := setupSubaccountDB(t)
	createTestSubaccount(t, db, "sa_disabled123456789", "Disabled Account", "disabled")
	router := newSubaccountScopedRouter(db)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Mailgun-On-Behalf-Of", "sa_disabled123456789")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	t.Run("returns 403 for disabled subaccount", func(t *testing.T) {
		if rec.Code != http.StatusForbidden {
			t.Errorf("expected status 403, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("returns Subaccount is disabled message", func(t *testing.T) {
		msg := parseMessageResponse(t, rec)
		if msg != "Subaccount is disabled" {
			t.Errorf("expected message %q, got %q", "Subaccount is disabled", msg)
		}
	})
}

func TestSubaccountScoping_ReenabledSubaccount_HandlerRuns(t *testing.T) {
	db := setupSubaccountDB(t)

	// Create a subaccount that starts as disabled.
	sa := createTestSubaccount(t, db, "sa_reenable12345678901", "Reenable Account", "disabled")

	router := newSubaccountScopedRouter(db)

	// First request with disabled subaccount should fail.
	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req1.Header.Set("X-Mailgun-On-Behalf-Of", "sa_reenable12345678901")
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)

	t.Run("initially returns 403 for disabled subaccount", func(t *testing.T) {
		if rec1.Code != http.StatusForbidden {
			t.Errorf("expected status 403, got %d (body: %s)", rec1.Code, rec1.Body.String())
		}
	})

	// Re-enable the subaccount directly in the database.
	sa.Status = "open"
	if err := db.Save(&sa).Error; err != nil {
		t.Fatalf("failed to re-enable subaccount: %v", err)
	}

	// Second request with the now-enabled subaccount should succeed.
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.Header.Set("X-Mailgun-On-Behalf-Of", "sa_reenable12345678901")
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)

	t.Run("returns 200 after subaccount is re-enabled", func(t *testing.T) {
		if rec2.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d (body: %s)", rec2.Code, rec2.Body.String())
		}
	})

	t.Run("SubaccountFromContext returns the subaccount ID after re-enable", func(t *testing.T) {
		saID := parseSubaccountResponse(t, rec2)
		if saID != "sa_reenable12345678901" {
			t.Errorf("expected subaccount_id %q, got %q", "sa_reenable12345678901", saID)
		}
	})
}

// ---------------------------------------------------------------------------
// SubaccountFromContext — bare context returns empty string
// ---------------------------------------------------------------------------

func TestSubaccountFromContext_EmptyWhenNotSet(t *testing.T) {
	ctx := context.Background()
	saID := middleware.SubaccountFromContext(ctx)

	t.Run("returns empty string for bare context", func(t *testing.T) {
		if saID != "" {
			t.Errorf("expected empty string from bare context, got %q", saID)
		}
	})
}
