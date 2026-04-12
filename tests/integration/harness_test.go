package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/bethmaloney/mailgun-mock-api/internal/server"
	"github.com/mailgun/mailgun-go/v5"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

var (
	testServer *httptest.Server
	baseURL    string
	apiKey     = "test-api-key"
)

func TestMain(m *testing.M) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open test database: %v\n", err)
		os.Exit(1)
	}

	handler := server.New(context.Background(), db, nil, nil)
	testServer = httptest.NewServer(handler)
	baseURL = testServer.URL

	code := m.Run()

	testServer.Close()
	os.Exit(code)
}

// resetServer calls POST /mock/reset to clear all data between test sections.
func resetServer(t *testing.T) {
	t.Helper()
	resp, err := doRequest("POST", "/mock/reset", nil)
	if err != nil {
		t.Fatalf("resetServer: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("resetServer: unexpected status %d", resp.StatusCode)
	}
}

// doRequest sends an HTTP request with Basic Auth to the test server.
func doRequest(method, path string, body interface{}) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal body: %w", err)
		}
		reader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, baseURL+path, reader)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth("api", apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return http.DefaultClient.Do(req)
}

// doFormRequest sends a multipart form request with Basic Auth.
func doFormRequest(method, path string, fields map[string]string) (*http.Response, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			return nil, fmt.Errorf("write field %s: %w", k, err)
		}
	}
	w.Close()

	req, err := http.NewRequest(method, baseURL+path, &buf)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth("api", apiKey)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return http.DefaultClient.Do(req)
}

// doRawFormRequest sends a URL-encoded form request with Basic Auth.
func doRawFormRequest(method, path string, fields map[string]string) (*http.Response, error) {
	values := make([]string, 0, len(fields))
	for k, v := range fields {
		values = append(values, fmt.Sprintf("%s=%s", k, v))
	}
	body := strings.Join(values, "&")

	req, err := http.NewRequest(method, baseURL+path, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth("api", apiKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return http.DefaultClient.Do(req)
}

// newMailgunClient returns a mailgun.Client configured to point at the test server.
func newMailgunClient() *mailgun.Client {
	mg := mailgun.NewMailgun(apiKey)
	mg.SetAPIBase(baseURL)
	return mg
}

// readJSON decodes an HTTP response body into the provided target struct.
func readJSON(t *testing.T, resp *http.Response, target interface{}) {
	t.Helper()
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("readJSON: read body: %v", err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("readJSON: unmarshal: %v\nbody: %s", err, string(data))
	}
}

// assertStatus checks that the response has the expected HTTP status code.
func assertStatus(t *testing.T, resp *http.Response, expected int) {
	t.Helper()
	if resp.StatusCode != expected {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status %d, got %d; body: %s", expected, resp.StatusCode, string(body))
	}
}

// --- Contract Reporter ---

// ContractResult records whether an endpoint test passed or failed.
type ContractResult struct {
	Section  string
	Endpoint string
	Method   string // "SDK" or "HTTP"
	Passed   bool
	Error    string
}

// ContractReporter collects results and prints a summary table.
type ContractReporter struct {
	mu      sync.Mutex
	results []ContractResult
}

var reporter = &ContractReporter{}

// Record adds a contract test result.
func (cr *ContractReporter) Record(section, endpoint, method string, passed bool, err string) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	cr.results = append(cr.results, ContractResult{
		Section:  section,
		Endpoint: endpoint,
		Method:   method,
		Passed:   passed,
		Error:    err,
	})
}

// Report prints a summary table of all recorded results.
func (cr *ContractReporter) Report(t *testing.T) {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	if len(cr.results) == 0 {
		return
	}

	t.Logf("\n%-20s %-30s %-6s %-10s %s", "Section", "Endpoint", "Method", "Status", "Error")
	t.Logf("%s", strings.Repeat("-", 90))

	passed, failed := 0, 0
	for _, r := range cr.results {
		status := "PASS"
		if !r.Passed {
			status = "FAIL"
			failed++
		} else {
			passed++
		}
		t.Logf("%-20s %-30s %-6s %-10s %s", r.Section, r.Endpoint, r.Method, status, r.Error)
	}
	t.Logf("%s", strings.Repeat("-", 90))
	t.Logf("Total: %d passed, %d failed, %d total", passed, failed, passed+failed)
}

// Ensure the SDK import is used (compile check).
var _ context.Context
var _ = mailgun.NewMailgun
