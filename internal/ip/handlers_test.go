package ip_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/bethmaloney/mailgun-mock-api/internal/domain"
	"github.com/bethmaloney/mailgun-mock-api/internal/ip"
	"github.com/bethmaloney/mailgun-mock-api/internal/mock"
	"github.com/go-chi/chi/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// Response structs for JSON decoding
// ---------------------------------------------------------------------------

// listIPsResponse represents the response from GET /v3/ips.
type listIPsResponse struct {
	Items            []string `json:"items"`
	TotalCount       int      `json:"total_count"`
	AssignableToPools []string `json:"assignable_to_pools"`
}

// ipDetailResponse represents the response from GET /v3/ips/{ip}.
type ipDetailResponse struct {
	IP        string `json:"ip"`
	RDNS      string `json:"rdns"`
	Dedicated bool   `json:"dedicated"`
}

// domainIPItem represents a single IP in a domain IP list.
type domainIPItem struct {
	IP        string `json:"ip"`
	RDNS      string `json:"rdns"`
	Dedicated bool   `json:"dedicated"`
}

// listDomainIPsResponse represents the response from GET /v3/domains/{name}/ips.
type listDomainIPsResponse struct {
	Items      []domainIPItem `json:"items"`
	TotalCount int            `json:"total_count"`
}

// messageResponse represents a generic message response.
type messageResponse struct {
	Message string `json:"message"`
}

// poolItem represents a single IP pool in a list response.
type poolItem struct {
	PoolID      string   `json:"pool_id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	IPs         []string `json:"ips"`
	IsLinked    bool     `json:"is_linked"`
	IsInherited bool     `json:"is_inherited"`
}

// listPoolsResponse represents the response from GET /v1/ip_pools.
type listPoolsResponse struct {
	IPPools []poolItem `json:"ip_pools"`
	Message string     `json:"message"`
}

// createPoolResponse represents the response from POST /v1/ip_pools.
type createPoolResponse struct {
	Message string `json:"message"`
	PoolID  string `json:"pool_id"`
}

// linkedDomain represents a domain linked to a pool.
type linkedDomain struct {
	Name string `json:"name"`
}

// getPoolResponse represents the response from GET /v1/ip_pools/{pool_id}.
type getPoolResponse struct {
	PoolID        string         `json:"pool_id"`
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	IPs           []string       `json:"ips"`
	IsLinked      bool           `json:"is_linked"`
	IsInherited   bool           `json:"is_inherited"`
	LinkedDomains []linkedDomain `json:"linked_domains"`
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
		&ip.IP{}, &ip.IPPool{}, &ip.IPPoolIP{}, &ip.DomainIP{}, &ip.DomainPool{},
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
	ih := ip.NewHandlers(db)
	r := chi.NewRouter()

	// Domain routes (for creating test domains)
	r.Post("/v4/domains", dh.CreateDomain)

	// IP routes (account-level)
	r.Get("/v3/ips", ih.ListIPs)
	r.Get("/v3/ips/{ip}", ih.GetIP)

	// IP routes (domain-level)
	r.Get("/v3/domains/{name}/ips", ih.ListDomainIPs)
	r.Post("/v3/domains/{name}/ips", ih.AssignDomainIP)
	r.Delete("/v3/domains/{name}/ips/{ip}", ih.UnassignDomainIP)

	// IP Pool routes (v1 prefix)
	r.Get("/v1/ip_pools", ih.ListPools)
	r.Post("/v1/ip_pools", ih.CreatePool)
	r.Get("/v1/ip_pools/{pool_id}", ih.GetPool)
	r.Patch("/v1/ip_pools/{pool_id}", ih.UpdatePool)
	r.Delete("/v1/ip_pools/{pool_id}", ih.DeletePool)

	// IP Pool routes (v3 prefix -- same handlers)
	r.Get("/v3/ip_pools", ih.ListPools)
	r.Post("/v3/ip_pools", ih.CreatePool)
	r.Get("/v3/ip_pools/{pool_id}", ih.GetPool)
	r.Patch("/v3/ip_pools/{pool_id}", ih.UpdatePool)
	r.Delete("/v3/ip_pools/{pool_id}", ih.DeletePool)

	return r
}

func setup(t *testing.T) http.Handler {
	t.Helper()
	db := setupTestDB(t)
	cfg := defaultConfig()
	return setupRouter(db, cfg)
}

type fieldPair struct {
	key   string
	value string
}

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

func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder, dest interface{}) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dest); err != nil {
		t.Fatalf("failed to decode response (body=%q): %v", rec.Body.String(), err)
	}
}

func assertStatus(t *testing.T, rec *httptest.ResponseRecorder, expected int) {
	t.Helper()
	if rec.Code != expected {
		t.Errorf("expected status %d, got %d; body=%s", expected, rec.Code, rec.Body.String())
	}
}

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

func createDomainHelper(t *testing.T, router http.Handler, name string) {
	t.Helper()
	rec := doRequest(t, router, http.MethodPost, "/v4/domains", map[string]string{"name": name})
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to create domain %q: status %d; body=%s", name, rec.Code, rec.Body.String())
	}
}

// createPoolHelper creates an IP pool and returns the pool_id.
func createPoolHelper(t *testing.T, router http.Handler, name, description string) string {
	t.Helper()
	rec := doRequest(t, router, http.MethodPost, "/v1/ip_pools", map[string]string{
		"name":        name,
		"description": description,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to create pool %q: status %d; body=%s", name, rec.Code, rec.Body.String())
	}
	var resp createPoolResponse
	decodeJSON(t, rec, &resp)
	if resp.PoolID == "" {
		t.Fatalf("expected non-empty pool_id in create pool response")
	}
	return resp.PoolID
}

// =========================================================================
// IP Management Tests (Account-Level)
// =========================================================================

// ---------------------------------------------------------------------------
// 1. TestListIPs_Default -- List IPs returns seeded default IPs
// ---------------------------------------------------------------------------

func TestListIPs_Default(t *testing.T) {
	router := setup(t)

	rec := doRequest(t, router, http.MethodGet, "/v3/ips", nil)
	assertStatus(t, rec, http.StatusOK)

	var resp listIPsResponse
	decodeJSON(t, rec, &resp)

	// Expect seeded default IPs: 127.0.0.1 and 10.0.0.1
	if resp.TotalCount < 2 {
		t.Errorf("expected at least 2 default IPs, got total_count=%d", resp.TotalCount)
	}

	found127 := false
	found10 := false
	for _, item := range resp.Items {
		if item == "127.0.0.1" {
			found127 = true
		}
		if item == "10.0.0.1" {
			found10 = true
		}
	}
	if !found127 {
		t.Error("expected 127.0.0.1 in default IPs list")
	}
	if !found10 {
		t.Error("expected 10.0.0.1 in default IPs list")
	}

	// assignable_to_pools should include 10.0.0.1
	foundAssignable := false
	for _, a := range resp.AssignableToPools {
		if a == "10.0.0.1" {
			foundAssignable = true
		}
	}
	if !foundAssignable {
		t.Error("expected 10.0.0.1 in assignable_to_pools")
	}
}

// ---------------------------------------------------------------------------
// 2. TestListIPs_DedicatedFilter -- Filter by dedicated=true
// ---------------------------------------------------------------------------

func TestListIPs_DedicatedFilter(t *testing.T) {
	router := setup(t)

	rec := doRequest(t, router, http.MethodGet, "/v3/ips?dedicated=true", nil)
	assertStatus(t, rec, http.StatusOK)

	var resp listIPsResponse
	decodeJSON(t, rec, &resp)

	// All returned IPs should be dedicated (at minimum, the seeded dedicated IP)
	// The exact count depends on implementation, but we should get at least one
	if resp.TotalCount < 1 {
		t.Errorf("expected at least 1 dedicated IP, got total_count=%d", resp.TotalCount)
	}
}

// ---------------------------------------------------------------------------
// 3. TestGetIP_Found -- Get a known IP returns its details
// ---------------------------------------------------------------------------

func TestGetIP_Found(t *testing.T) {
	router := setup(t)

	rec := doRequest(t, router, http.MethodGet, "/v3/ips/127.0.0.1", nil)
	assertStatus(t, rec, http.StatusOK)

	var resp ipDetailResponse
	decodeJSON(t, rec, &resp)

	if resp.IP != "127.0.0.1" {
		t.Errorf("expected ip %q, got %q", "127.0.0.1", resp.IP)
	}
	if resp.RDNS == "" {
		t.Error("expected non-empty rdns")
	}
}

// ---------------------------------------------------------------------------
// 4. TestGetIP_NotFound -- Get unknown IP returns 404
// ---------------------------------------------------------------------------

func TestGetIP_NotFound(t *testing.T) {
	router := setup(t)

	rec := doRequest(t, router, http.MethodGet, "/v3/ips/192.168.99.99", nil)
	assertStatus(t, rec, http.StatusNotFound)
}

// =========================================================================
// Domain-IP Assignment Tests
// =========================================================================

// ---------------------------------------------------------------------------
// 5. TestListDomainIPs_Empty -- No IPs assigned initially
// ---------------------------------------------------------------------------

func TestListDomainIPs_Empty(t *testing.T) {
	router := setup(t)

	createDomainHelper(t, router, "example.com")

	rec := doRequest(t, router, http.MethodGet, "/v3/domains/example.com/ips", nil)
	assertStatus(t, rec, http.StatusOK)

	var resp listDomainIPsResponse
	decodeJSON(t, rec, &resp)

	if resp.TotalCount != 0 {
		t.Errorf("expected total_count 0 for new domain, got %d", resp.TotalCount)
	}
	if len(resp.Items) != 0 {
		t.Errorf("expected 0 items for new domain, got %d", len(resp.Items))
	}
}

// ---------------------------------------------------------------------------
// 6. TestAssignDomainIP_ByIP -- Assign IP to domain, verify it appears
// ---------------------------------------------------------------------------

func TestAssignDomainIP_ByIP(t *testing.T) {
	router := setup(t)

	createDomainHelper(t, router, "example.com")

	// Assign an IP to the domain
	rec := doRequest(t, router, http.MethodPost, "/v3/domains/example.com/ips", map[string]string{
		"ip": "127.0.0.1",
	})
	assertStatus(t, rec, http.StatusOK)
	assertMessage(t, rec, "success")

	// Verify IP appears in domain IP list
	rec = doRequest(t, router, http.MethodGet, "/v3/domains/example.com/ips", nil)
	assertStatus(t, rec, http.StatusOK)

	var resp listDomainIPsResponse
	decodeJSON(t, rec, &resp)

	if resp.TotalCount != 1 {
		t.Errorf("expected total_count 1 after assignment, got %d", resp.TotalCount)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("expected 1 item after assignment, got %d", len(resp.Items))
	}
	if resp.Items[0].IP != "127.0.0.1" {
		t.Errorf("expected assigned IP %q, got %q", "127.0.0.1", resp.Items[0].IP)
	}
}

// ---------------------------------------------------------------------------
// 7. TestAssignDomainIP_ByPoolID -- Assign by pool_id
// ---------------------------------------------------------------------------

func TestAssignDomainIP_ByPoolID(t *testing.T) {
	router := setup(t)

	createDomainHelper(t, router, "example.com")

	// Create a pool first
	poolID := createPoolHelper(t, router, "test-pool", "A test pool")

	// Assign the pool to the domain
	rec := doRequest(t, router, http.MethodPost, "/v3/domains/example.com/ips", map[string]string{
		"pool_id": poolID,
	})
	assertStatus(t, rec, http.StatusOK)
	assertMessage(t, rec, "success")
}

// ---------------------------------------------------------------------------
// 8. TestAssignDomainIP_NotFoundDomain -- 404 for unknown domain
// ---------------------------------------------------------------------------

func TestAssignDomainIP_NotFoundDomain(t *testing.T) {
	router := setup(t)

	rec := doRequest(t, router, http.MethodPost, "/v3/domains/nonexistent.com/ips", map[string]string{
		"ip": "127.0.0.1",
	})
	assertStatus(t, rec, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// 9. TestUnassignDomainIP_Specific -- Remove specific IP from domain
// ---------------------------------------------------------------------------

func TestUnassignDomainIP_Specific(t *testing.T) {
	router := setup(t)

	createDomainHelper(t, router, "example.com")

	// Assign IP first
	rec := doRequest(t, router, http.MethodPost, "/v3/domains/example.com/ips", map[string]string{
		"ip": "127.0.0.1",
	})
	assertStatus(t, rec, http.StatusOK)

	// Remove specific IP
	rec = doRequest(t, router, http.MethodDelete, "/v3/domains/example.com/ips/127.0.0.1", nil)
	assertStatus(t, rec, http.StatusOK)
	assertMessage(t, rec, "success")

	// Verify the IP is no longer assigned
	rec = doRequest(t, router, http.MethodGet, "/v3/domains/example.com/ips", nil)
	assertStatus(t, rec, http.StatusOK)

	var resp listDomainIPsResponse
	decodeJSON(t, rec, &resp)

	if resp.TotalCount != 0 {
		t.Errorf("expected total_count 0 after removal, got %d", resp.TotalCount)
	}
}

// ---------------------------------------------------------------------------
// 10. TestUnassignDomainIP_All -- Remove all IPs from domain using "all"
// ---------------------------------------------------------------------------

func TestUnassignDomainIP_All(t *testing.T) {
	router := setup(t)

	createDomainHelper(t, router, "example.com")

	// Assign multiple IPs
	rec := doRequest(t, router, http.MethodPost, "/v3/domains/example.com/ips", map[string]string{
		"ip": "127.0.0.1",
	})
	assertStatus(t, rec, http.StatusOK)

	rec = doRequest(t, router, http.MethodPost, "/v3/domains/example.com/ips", map[string]string{
		"ip": "10.0.0.1",
	})
	assertStatus(t, rec, http.StatusOK)

	// Remove all IPs using "all" as the IP parameter
	rec = doRequest(t, router, http.MethodDelete, "/v3/domains/example.com/ips/all", nil)
	assertStatus(t, rec, http.StatusOK)
	assertMessage(t, rec, "success")

	// Verify no IPs remain
	rec = doRequest(t, router, http.MethodGet, "/v3/domains/example.com/ips", nil)
	assertStatus(t, rec, http.StatusOK)

	var resp listDomainIPsResponse
	decodeJSON(t, rec, &resp)

	if resp.TotalCount != 0 {
		t.Errorf("expected total_count 0 after removing all, got %d", resp.TotalCount)
	}
}

// ---------------------------------------------------------------------------
// 11. TestUnassignDomainIP_Pool -- Unlink pool using "ip_pool" as {ip} param
// ---------------------------------------------------------------------------

func TestUnassignDomainIP_Pool(t *testing.T) {
	router := setup(t)

	createDomainHelper(t, router, "example.com")

	// Create and assign a pool
	poolID := createPoolHelper(t, router, "test-pool", "A test pool")

	rec := doRequest(t, router, http.MethodPost, "/v3/domains/example.com/ips", map[string]string{
		"pool_id": poolID,
	})
	assertStatus(t, rec, http.StatusOK)

	// Unlink pool using "ip_pool" as the IP parameter
	rec = doRequest(t, router, http.MethodDelete, "/v3/domains/example.com/ips/ip_pool", nil)
	assertStatus(t, rec, http.StatusOK)
	assertMessage(t, rec, "success")
}

// =========================================================================
// IP Pool CRUD Tests
// =========================================================================

// ---------------------------------------------------------------------------
// 12. TestCreatePool_Basic -- Create pool with name, description
// ---------------------------------------------------------------------------

func TestCreatePool_Basic(t *testing.T) {
	router := setup(t)

	rec := doRequest(t, router, http.MethodPost, "/v1/ip_pools", map[string]string{
		"name":        "my-pool",
		"description": "My first IP pool",
	})
	assertStatus(t, rec, http.StatusOK)

	var resp createPoolResponse
	decodeJSON(t, rec, &resp)

	if resp.Message != "success" {
		t.Errorf("expected message %q, got %q", "success", resp.Message)
	}
	if resp.PoolID == "" {
		t.Fatal("expected non-empty pool_id")
	}
	// pool_id should be a 24-character hex string
	if len(resp.PoolID) != 24 {
		t.Errorf("expected pool_id to be 24 characters, got %d (%q)", len(resp.PoolID), resp.PoolID)
	}
}

// ---------------------------------------------------------------------------
// 13. TestCreatePool_WithIPs -- Create pool with IPs attached
// ---------------------------------------------------------------------------

func TestCreatePool_WithIPs(t *testing.T) {
	router := setup(t)

	// Use form-url-encoded with repeatable ip field
	rec := doFormURLEncoded(t, router, http.MethodPost, "/v1/ip_pools", []fieldPair{
		{key: "name", value: "pool-with-ips"},
		{key: "description", value: "Pool with IPs attached"},
		{key: "ip", value: "127.0.0.1"},
		{key: "ip", value: "10.0.0.1"},
	})
	assertStatus(t, rec, http.StatusOK)

	var resp createPoolResponse
	decodeJSON(t, rec, &resp)

	if resp.Message != "success" {
		t.Errorf("expected message %q, got %q", "success", resp.Message)
	}
	if resp.PoolID == "" {
		t.Fatal("expected non-empty pool_id")
	}

	// Verify the pool has the IPs by fetching it
	rec = doRequest(t, router, http.MethodGet, "/v1/ip_pools/"+resp.PoolID, nil)
	assertStatus(t, rec, http.StatusOK)

	var getResp getPoolResponse
	decodeJSON(t, rec, &getResp)

	if len(getResp.IPs) != 2 {
		t.Errorf("expected 2 IPs in pool, got %d", len(getResp.IPs))
	}
}

// ---------------------------------------------------------------------------
// 14. TestCreatePool_MissingName -- Returns 400 if name is missing
// ---------------------------------------------------------------------------

func TestCreatePool_MissingName(t *testing.T) {
	router := setup(t)

	rec := doRequest(t, router, http.MethodPost, "/v1/ip_pools", map[string]string{
		"description": "No name provided",
	})
	assertStatus(t, rec, http.StatusBadRequest)
}

// ---------------------------------------------------------------------------
// 15. TestListPools_Empty -- Returns empty list
// ---------------------------------------------------------------------------

func TestListPools_Empty(t *testing.T) {
	router := setup(t)

	rec := doRequest(t, router, http.MethodGet, "/v1/ip_pools", nil)
	assertStatus(t, rec, http.StatusOK)

	var resp listPoolsResponse
	decodeJSON(t, rec, &resp)

	if resp.Message != "success" {
		t.Errorf("expected message %q, got %q", "success", resp.Message)
	}
	if len(resp.IPPools) != 0 {
		t.Errorf("expected 0 pools, got %d", len(resp.IPPools))
	}
}

// ---------------------------------------------------------------------------
// 16. TestListPools_WithData -- Returns created pools
// ---------------------------------------------------------------------------

func TestListPools_WithData(t *testing.T) {
	router := setup(t)

	// Create two pools
	createPoolHelper(t, router, "pool-alpha", "Alpha pool")
	createPoolHelper(t, router, "pool-beta", "Beta pool")

	rec := doRequest(t, router, http.MethodGet, "/v1/ip_pools", nil)
	assertStatus(t, rec, http.StatusOK)

	var resp listPoolsResponse
	decodeJSON(t, rec, &resp)

	if resp.Message != "success" {
		t.Errorf("expected message %q, got %q", "success", resp.Message)
	}
	if len(resp.IPPools) != 2 {
		t.Fatalf("expected 2 pools, got %d", len(resp.IPPools))
	}

	// Verify pool names are present
	names := map[string]bool{}
	for _, p := range resp.IPPools {
		names[p.Name] = true
	}
	if !names["pool-alpha"] {
		t.Error("expected pool-alpha in list")
	}
	if !names["pool-beta"] {
		t.Error("expected pool-beta in list")
	}
}

// ---------------------------------------------------------------------------
// 17. TestListPools_V3Prefix -- Same as above but using /v3/ip_pools path
// ---------------------------------------------------------------------------

func TestListPools_V3Prefix(t *testing.T) {
	router := setup(t)

	// Create a pool via v1
	createPoolHelper(t, router, "pool-v3test", "V3 test pool")

	// List via v3 prefix
	rec := doRequest(t, router, http.MethodGet, "/v3/ip_pools", nil)
	assertStatus(t, rec, http.StatusOK)

	var resp listPoolsResponse
	decodeJSON(t, rec, &resp)

	if resp.Message != "success" {
		t.Errorf("expected message %q, got %q", "success", resp.Message)
	}
	if len(resp.IPPools) != 1 {
		t.Fatalf("expected 1 pool via v3 prefix, got %d", len(resp.IPPools))
	}
	if resp.IPPools[0].Name != "pool-v3test" {
		t.Errorf("expected pool name %q, got %q", "pool-v3test", resp.IPPools[0].Name)
	}
}

// ---------------------------------------------------------------------------
// 18. TestGetPool_Found -- Returns pool details including linked_domains
// ---------------------------------------------------------------------------

func TestGetPool_Found(t *testing.T) {
	router := setup(t)

	poolID := createPoolHelper(t, router, "my-pool", "My pool description")

	rec := doRequest(t, router, http.MethodGet, "/v1/ip_pools/"+poolID, nil)
	assertStatus(t, rec, http.StatusOK)

	var resp getPoolResponse
	decodeJSON(t, rec, &resp)

	if resp.PoolID != poolID {
		t.Errorf("expected pool_id %q, got %q", poolID, resp.PoolID)
	}
	if resp.Name != "my-pool" {
		t.Errorf("expected name %q, got %q", "my-pool", resp.Name)
	}
	if resp.Description != "My pool description" {
		t.Errorf("expected description %q, got %q", "My pool description", resp.Description)
	}
	// linked_domains should be present (even if empty)
	if resp.LinkedDomains == nil {
		t.Error("expected linked_domains to be present (even if empty)")
	}
}

// ---------------------------------------------------------------------------
// 19. TestGetPool_NotFound -- 404 for unknown pool
// ---------------------------------------------------------------------------

func TestGetPool_NotFound(t *testing.T) {
	router := setup(t)

	rec := doRequest(t, router, http.MethodGet, "/v1/ip_pools/nonexistent000000000000", nil)
	assertStatus(t, rec, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// 20. TestUpdatePool_Name -- Update pool name
// ---------------------------------------------------------------------------

func TestUpdatePool_Name(t *testing.T) {
	router := setup(t)

	poolID := createPoolHelper(t, router, "old-name", "Description")

	rec := doRequest(t, router, http.MethodPatch, "/v1/ip_pools/"+poolID, map[string]string{
		"name": "new-name",
	})
	assertStatus(t, rec, http.StatusOK)
	assertMessage(t, rec, "success")

	// Verify the name was updated
	rec = doRequest(t, router, http.MethodGet, "/v1/ip_pools/"+poolID, nil)
	assertStatus(t, rec, http.StatusOK)

	var resp getPoolResponse
	decodeJSON(t, rec, &resp)

	if resp.Name != "new-name" {
		t.Errorf("expected updated name %q, got %q", "new-name", resp.Name)
	}
	// Description should remain unchanged
	if resp.Description != "Description" {
		t.Errorf("expected description %q (unchanged), got %q", "Description", resp.Description)
	}
}

// ---------------------------------------------------------------------------
// 21. TestUpdatePool_AddIP -- Add IP via add_ip field
// ---------------------------------------------------------------------------

func TestUpdatePool_AddIP(t *testing.T) {
	router := setup(t)

	poolID := createPoolHelper(t, router, "ip-pool", "Pool for IP tests")

	rec := doRequest(t, router, http.MethodPatch, "/v1/ip_pools/"+poolID, map[string]string{
		"add_ip": "127.0.0.1",
	})
	assertStatus(t, rec, http.StatusOK)
	assertMessage(t, rec, "success")

	// Verify IP was added
	rec = doRequest(t, router, http.MethodGet, "/v1/ip_pools/"+poolID, nil)
	assertStatus(t, rec, http.StatusOK)

	var resp getPoolResponse
	decodeJSON(t, rec, &resp)

	found := false
	for _, ipAddr := range resp.IPs {
		if ipAddr == "127.0.0.1" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 127.0.0.1 to be in pool IPs, got %v", resp.IPs)
	}
}

// ---------------------------------------------------------------------------
// 22. TestUpdatePool_RemoveIP -- Remove IP via remove_ip field
// ---------------------------------------------------------------------------

func TestUpdatePool_RemoveIP(t *testing.T) {
	router := setup(t)

	// Create pool with an IP
	rec := doFormURLEncoded(t, router, http.MethodPost, "/v1/ip_pools", []fieldPair{
		{key: "name", value: "removal-pool"},
		{key: "description", value: "Pool to test IP removal"},
		{key: "ip", value: "127.0.0.1"},
	})
	assertStatus(t, rec, http.StatusOK)

	var createResp createPoolResponse
	decodeJSON(t, rec, &createResp)
	poolID := createResp.PoolID

	// Remove the IP
	rec = doRequest(t, router, http.MethodPatch, "/v1/ip_pools/"+poolID, map[string]string{
		"remove_ip": "127.0.0.1",
	})
	assertStatus(t, rec, http.StatusOK)
	assertMessage(t, rec, "success")

	// Verify IP was removed
	rec = doRequest(t, router, http.MethodGet, "/v1/ip_pools/"+poolID, nil)
	assertStatus(t, rec, http.StatusOK)

	var resp getPoolResponse
	decodeJSON(t, rec, &resp)

	for _, ipAddr := range resp.IPs {
		if ipAddr == "127.0.0.1" {
			t.Error("expected 127.0.0.1 to be removed from pool IPs")
		}
	}
}

// ---------------------------------------------------------------------------
// 23. TestDeletePool_Success -- Delete pool, verify "started" message
// ---------------------------------------------------------------------------

func TestDeletePool_Success(t *testing.T) {
	router := setup(t)

	poolID := createPoolHelper(t, router, "doomed-pool", "About to be deleted")

	rec := doRequest(t, router, http.MethodDelete, "/v1/ip_pools/"+poolID, nil)
	assertStatus(t, rec, http.StatusOK)

	// Delete returns "started", NOT "success"
	assertMessage(t, rec, "started")

	// Verify pool is gone from list
	rec = doRequest(t, router, http.MethodGet, "/v1/ip_pools", nil)
	assertStatus(t, rec, http.StatusOK)

	var listResp listPoolsResponse
	decodeJSON(t, rec, &listResp)

	for _, p := range listResp.IPPools {
		if p.PoolID == poolID {
			t.Errorf("expected pool %q to be deleted, but it still appears in list", poolID)
		}
	}

	// Verify pool is not found by ID
	rec = doRequest(t, router, http.MethodGet, "/v1/ip_pools/"+poolID, nil)
	assertStatus(t, rec, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// 24. TestDeletePool_NotFound -- 404 for unknown pool
// ---------------------------------------------------------------------------

func TestDeletePool_NotFound(t *testing.T) {
	router := setup(t)

	rec := doRequest(t, router, http.MethodDelete, "/v1/ip_pools/nonexistent000000000000", nil)
	assertStatus(t, rec, http.StatusNotFound)
}

// =========================================================================
// Integration Tests
// =========================================================================

// ---------------------------------------------------------------------------
// 25. TestAssignIP_ThenListDomainIPs -- Full lifecycle
// ---------------------------------------------------------------------------

func TestAssignIP_ThenListDomainIPs(t *testing.T) {
	router := setup(t)

	t.Run("create domain", func(t *testing.T) {
		createDomainHelper(t, router, "integration.example.com")
	})

	t.Run("list domain IPs is initially empty", func(t *testing.T) {
		rec := doRequest(t, router, http.MethodGet, "/v3/domains/integration.example.com/ips", nil)
		assertStatus(t, rec, http.StatusOK)

		var resp listDomainIPsResponse
		decodeJSON(t, rec, &resp)

		if resp.TotalCount != 0 {
			t.Errorf("expected total_count 0, got %d", resp.TotalCount)
		}
	})

	t.Run("assign IP to domain", func(t *testing.T) {
		rec := doRequest(t, router, http.MethodPost, "/v3/domains/integration.example.com/ips", map[string]string{
			"ip": "127.0.0.1",
		})
		assertStatus(t, rec, http.StatusOK)
		assertMessage(t, rec, "success")
	})

	t.Run("list domain IPs shows assigned IP", func(t *testing.T) {
		rec := doRequest(t, router, http.MethodGet, "/v3/domains/integration.example.com/ips", nil)
		assertStatus(t, rec, http.StatusOK)

		var resp listDomainIPsResponse
		decodeJSON(t, rec, &resp)

		if resp.TotalCount != 1 {
			t.Errorf("expected total_count 1, got %d", resp.TotalCount)
		}
		if len(resp.Items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(resp.Items))
		}
		if resp.Items[0].IP != "127.0.0.1" {
			t.Errorf("expected IP %q, got %q", "127.0.0.1", resp.Items[0].IP)
		}
	})

	t.Run("unassign IP from domain", func(t *testing.T) {
		rec := doRequest(t, router, http.MethodDelete, "/v3/domains/integration.example.com/ips/127.0.0.1", nil)
		assertStatus(t, rec, http.StatusOK)
		assertMessage(t, rec, "success")
	})

	t.Run("list domain IPs is empty again", func(t *testing.T) {
		rec := doRequest(t, router, http.MethodGet, "/v3/domains/integration.example.com/ips", nil)
		assertStatus(t, rec, http.StatusOK)

		var resp listDomainIPsResponse
		decodeJSON(t, rec, &resp)

		if resp.TotalCount != 0 {
			t.Errorf("expected total_count 0 after unassign, got %d", resp.TotalCount)
		}
	})
}

// ---------------------------------------------------------------------------
// 26. TestPoolLinking -- Create pool, assign to domain, verify linked_domains
// ---------------------------------------------------------------------------

func TestPoolLinking(t *testing.T) {
	router := setup(t)

	t.Run("create domain and pool", func(t *testing.T) {
		createDomainHelper(t, router, "linked.example.com")
	})

	var poolID string
	t.Run("create pool", func(t *testing.T) {
		poolID = createPoolHelper(t, router, "linkable-pool", "Pool for linking test")
	})

	t.Run("assign pool to domain", func(t *testing.T) {
		rec := doRequest(t, router, http.MethodPost, "/v3/domains/linked.example.com/ips", map[string]string{
			"pool_id": poolID,
		})
		assertStatus(t, rec, http.StatusOK)
		assertMessage(t, rec, "success")
	})

	t.Run("get pool shows domain in linked_domains", func(t *testing.T) {
		rec := doRequest(t, router, http.MethodGet, "/v1/ip_pools/"+poolID, nil)
		assertStatus(t, rec, http.StatusOK)

		var resp getPoolResponse
		decodeJSON(t, rec, &resp)

		if resp.PoolID != poolID {
			t.Errorf("expected pool_id %q, got %q", poolID, resp.PoolID)
		}

		foundDomain := false
		for _, ld := range resp.LinkedDomains {
			if ld.Name == "linked.example.com" {
				foundDomain = true
				break
			}
		}
		if !foundDomain {
			t.Errorf("expected linked.example.com in linked_domains, got %v", resp.LinkedDomains)
		}

		if !resp.IsLinked {
			t.Error("expected is_linked to be true when pool is linked to a domain")
		}
	})

	t.Run("pool appears as linked in list response", func(t *testing.T) {
		rec := doRequest(t, router, http.MethodGet, "/v1/ip_pools", nil)
		assertStatus(t, rec, http.StatusOK)

		var resp listPoolsResponse
		decodeJSON(t, rec, &resp)

		found := false
		for _, p := range resp.IPPools {
			if p.PoolID == poolID {
				found = true
				if !p.IsLinked {
					t.Error("expected is_linked to be true in list response")
				}
			}
		}
		if !found {
			t.Errorf("expected pool %q in list response", poolID)
		}
	})
}
