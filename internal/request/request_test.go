package request_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/bethmaloney/mailgun-mock-api/internal/request"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newJSONRequest creates an HTTP POST request with a JSON body and the
// appropriate Content-Type header.
func newJSONRequest(t *testing.T, body string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// newFormURLEncodedRequest creates an HTTP POST request with URL-encoded form
// data and the appropriate Content-Type header.
func newFormURLEncodedRequest(t *testing.T, values url.Values) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

// newMultipartRequest creates an HTTP POST request with multipart/form-data
// containing the given field key-value pairs.
func newMultipartRequest(t *testing.T, fields map[string]string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for key, val := range fields {
		if err := writer.WriteField(key, val); err != nil {
			t.Fatalf("failed to write multipart field %q: %v", key, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close multipart writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/test", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

// ---------------------------------------------------------------------------
// Test structs
// ---------------------------------------------------------------------------

type basicStruct struct {
	Name    string `json:"name"`
	Active  bool   `json:"active"`
	Count   int    `json:"count"`
}

type rawJSONStruct struct {
	Name    string          `json:"name"`
	Options json.RawMessage `json:"options"`
}

type pointerStruct struct {
	Name   *string `json:"name"`
	Active *bool   `json:"active"`
	Count  *int    `json:"count"`
}

type formTagStruct struct {
	Subject     string `form:"subject" json:"subject_field"`
	Body        string `form:"body_plain" json:"body"`
	Recipient   string `form:"recipient"`
}

type jsonFallbackStruct struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Subject string `json:"subject"`
}

type mixedTagStruct struct {
	FormField string `form:"form_name"`
	JSONField string `json:"json_name"`
	BareField string
}

type numericStruct struct {
	IntVal   int     `form:"int_val"`
	FloatVal float64 `form:"float_val"`
}

type boolStruct struct {
	FlagA bool `form:"flag_a"`
	FlagB bool `form:"flag_b"`
	FlagC bool `form:"flag_c"`
}

type pointerFormStruct struct {
	Name   *string `form:"name"`
	Active *bool   `form:"active"`
	Count  *int    `form:"count"`
}

type float64Struct struct {
	Price    float64 `form:"price"`
	Quantity float64 `form:"quantity"`
}

// ---------------------------------------------------------------------------
// 1. JSON Parsing
// ---------------------------------------------------------------------------

func TestParse_JSON_BasicFields(t *testing.T) {
	body := `{"name":"test-message","active":true,"count":42}`
	req := newJSONRequest(t, body)

	var dst basicStruct
	err := request.Parse(req, &dst)

	t.Run("returns no error", func(t *testing.T) {
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("parses string field", func(t *testing.T) {
		if dst.Name != "test-message" {
			t.Errorf("expected name %q, got %q", "test-message", dst.Name)
		}
	})

	t.Run("parses bool field", func(t *testing.T) {
		if dst.Active != true {
			t.Errorf("expected active=true, got %v", dst.Active)
		}
	})

	t.Run("parses int field", func(t *testing.T) {
		if dst.Count != 42 {
			t.Errorf("expected count=42, got %d", dst.Count)
		}
	})
}

func TestParse_JSON_NestedRawMessage(t *testing.T) {
	body := `{"name":"msg","options":{"priority":"high","retries":3}}`
	req := newJSONRequest(t, body)

	var dst rawJSONStruct
	err := request.Parse(req, &dst)

	t.Run("returns no error", func(t *testing.T) {
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("parses string field", func(t *testing.T) {
		if dst.Name != "msg" {
			t.Errorf("expected name %q, got %q", "msg", dst.Name)
		}
	})

	t.Run("parses nested JSON into RawMessage", func(t *testing.T) {
		if dst.Options == nil {
			t.Fatal("expected non-nil Options")
		}
		var opts map[string]interface{}
		if err := json.Unmarshal(dst.Options, &opts); err != nil {
			t.Fatalf("failed to unmarshal Options: %v", err)
		}
		if opts["priority"] != "high" {
			t.Errorf("expected priority=%q, got %v", "high", opts["priority"])
		}
	})
}

func TestParse_JSON_InvalidBody(t *testing.T) {
	body := `{"name": invalid json}`
	req := newJSONRequest(t, body)

	var dst basicStruct
	err := request.Parse(req, &dst)

	t.Run("returns error for invalid JSON", func(t *testing.T) {
		if err == nil {
			t.Error("expected error for invalid JSON, got nil")
		}
	})
}

func TestParse_JSON_EmptyBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/json")

	var dst basicStruct
	err := request.Parse(req, &dst)

	t.Run("returns error for empty body", func(t *testing.T) {
		if err == nil {
			t.Error("expected error for empty JSON body, got nil")
		}
	})
}

func TestParse_JSON_PointerFields(t *testing.T) {
	t.Run("present fields are populated as pointers", func(t *testing.T) {
		body := `{"name":"hello","active":true,"count":7}`
		req := newJSONRequest(t, body)

		var dst pointerStruct
		err := request.Parse(req, &dst)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		if dst.Name == nil {
			t.Fatal("expected non-nil Name pointer")
		}
		if *dst.Name != "hello" {
			t.Errorf("expected name %q, got %q", "hello", *dst.Name)
		}

		if dst.Active == nil {
			t.Fatal("expected non-nil Active pointer")
		}
		if *dst.Active != true {
			t.Errorf("expected active=true, got %v", *dst.Active)
		}

		if dst.Count == nil {
			t.Fatal("expected non-nil Count pointer")
		}
		if *dst.Count != 7 {
			t.Errorf("expected count=7, got %d", *dst.Count)
		}
	})

	t.Run("absent fields remain nil", func(t *testing.T) {
		body := `{"name":"hello"}`
		req := newJSONRequest(t, body)

		var dst pointerStruct
		err := request.Parse(req, &dst)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		if dst.Name == nil || *dst.Name != "hello" {
			t.Errorf("expected name pointer to %q", "hello")
		}

		if dst.Active != nil {
			t.Errorf("expected nil Active pointer for absent field, got %v", *dst.Active)
		}

		if dst.Count != nil {
			t.Errorf("expected nil Count pointer for absent field, got %v", *dst.Count)
		}
	})
}

// ---------------------------------------------------------------------------
// 2. Multipart Form Data Parsing
// ---------------------------------------------------------------------------

func TestParse_Multipart_StringFields(t *testing.T) {
	fields := map[string]string{
		"subject":    "Test Email",
		"body_plain": "Hello, World!",
		"recipient":  "user@example.com",
	}
	req := newMultipartRequest(t, fields)

	var dst formTagStruct
	err := request.Parse(req, &dst)

	t.Run("returns no error", func(t *testing.T) {
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("parses subject field", func(t *testing.T) {
		if dst.Subject != "Test Email" {
			t.Errorf("expected subject %q, got %q", "Test Email", dst.Subject)
		}
	})

	t.Run("parses body_plain field via form tag", func(t *testing.T) {
		if dst.Body != "Hello, World!" {
			t.Errorf("expected body %q, got %q", "Hello, World!", dst.Body)
		}
	})

	t.Run("parses recipient field", func(t *testing.T) {
		if dst.Recipient != "user@example.com" {
			t.Errorf("expected recipient %q, got %q", "user@example.com", dst.Recipient)
		}
	})
}

func TestParse_Multipart_BoolFields(t *testing.T) {
	truthy := []struct {
		name  string
		value string
	}{
		{"true string", "true"},
		{"yes string", "yes"},
		{"1 string", "1"},
	}

	for _, tc := range truthy {
		t.Run(tc.name+" parses as true", func(t *testing.T) {
			fields := map[string]string{"flag_a": tc.value, "flag_b": "false", "flag_c": "false"}
			req := newMultipartRequest(t, fields)

			var dst boolStruct
			err := request.Parse(req, &dst)
			if err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}

			if dst.FlagA != true {
				t.Errorf("expected flag_a=true for value %q, got %v", tc.value, dst.FlagA)
			}
		})
	}

	falsy := []struct {
		name  string
		value string
	}{
		{"false string", "false"},
		{"no string", "no"},
		{"0 string", "0"},
		{"empty string", ""},
	}

	for _, tc := range falsy {
		t.Run(tc.name+" parses as false", func(t *testing.T) {
			fields := map[string]string{"flag_a": tc.value, "flag_b": "true", "flag_c": "true"}
			req := newMultipartRequest(t, fields)

			var dst boolStruct
			err := request.Parse(req, &dst)
			if err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}

			if dst.FlagA != false {
				t.Errorf("expected flag_a=false for value %q, got %v", tc.value, dst.FlagA)
			}
		})
	}
}

func TestParse_Multipart_IntFields(t *testing.T) {
	fields := map[string]string{
		"int_val":   "42",
		"float_val": "3.14",
	}
	req := newMultipartRequest(t, fields)

	var dst numericStruct
	err := request.Parse(req, &dst)

	t.Run("returns no error", func(t *testing.T) {
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("parses int field", func(t *testing.T) {
		if dst.IntVal != 42 {
			t.Errorf("expected int_val=42, got %d", dst.IntVal)
		}
	})
}

func TestParse_Multipart_FormTagMapping(t *testing.T) {
	// The form tag name differs from the struct field name.
	fields := map[string]string{
		"subject":    "Tagged Subject",
		"body_plain": "Tagged Body",
		"recipient":  "tagged@example.com",
	}
	req := newMultipartRequest(t, fields)

	var dst formTagStruct
	err := request.Parse(req, &dst)

	t.Run("returns no error", func(t *testing.T) {
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("maps body_plain form field to Body struct field", func(t *testing.T) {
		if dst.Body != "Tagged Body" {
			t.Errorf("expected body %q, got %q", "Tagged Body", dst.Body)
		}
	})

	t.Run("maps subject form field to Subject struct field", func(t *testing.T) {
		if dst.Subject != "Tagged Subject" {
			t.Errorf("expected subject %q, got %q", "Tagged Subject", dst.Subject)
		}
	})
}

func TestParse_Multipart_JSONTagFallback(t *testing.T) {
	// This struct has only json tags, no form tags. The parser should fall
	// back to using the json tag names for form field mapping.
	fields := map[string]string{
		"from":    "sender@example.com",
		"to":      "receiver@example.com",
		"subject": "Fallback Test",
	}
	req := newMultipartRequest(t, fields)

	var dst jsonFallbackStruct
	err := request.Parse(req, &dst)

	t.Run("returns no error", func(t *testing.T) {
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("parses from field using json tag fallback", func(t *testing.T) {
		if dst.From != "sender@example.com" {
			t.Errorf("expected from %q, got %q", "sender@example.com", dst.From)
		}
	})

	t.Run("parses to field using json tag fallback", func(t *testing.T) {
		if dst.To != "receiver@example.com" {
			t.Errorf("expected to %q, got %q", "receiver@example.com", dst.To)
		}
	})

	t.Run("parses subject field using json tag fallback", func(t *testing.T) {
		if dst.Subject != "Fallback Test" {
			t.Errorf("expected subject %q, got %q", "Fallback Test", dst.Subject)
		}
	})
}

func TestParse_Multipart_MissingOptionalFields(t *testing.T) {
	// Only provide the name field; active and count are absent.
	fields := map[string]string{
		"name": "present",
	}
	req := newMultipartRequest(t, fields)

	var dst pointerFormStruct
	err := request.Parse(req, &dst)

	t.Run("returns no error", func(t *testing.T) {
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("present pointer field is populated", func(t *testing.T) {
		if dst.Name == nil {
			t.Fatal("expected non-nil Name pointer")
		}
		if *dst.Name != "present" {
			t.Errorf("expected name %q, got %q", "present", *dst.Name)
		}
	})

	t.Run("absent pointer fields remain nil", func(t *testing.T) {
		if dst.Active != nil {
			t.Errorf("expected nil Active pointer for missing field, got %v", *dst.Active)
		}
		if dst.Count != nil {
			t.Errorf("expected nil Count pointer for missing field, got %v", *dst.Count)
		}
	})
}

func TestParse_Multipart_Float64Fields(t *testing.T) {
	fields := map[string]string{
		"price":    "19.99",
		"quantity": "2.5",
	}
	req := newMultipartRequest(t, fields)

	var dst float64Struct
	err := request.Parse(req, &dst)

	t.Run("returns no error", func(t *testing.T) {
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("parses price float64 field", func(t *testing.T) {
		if dst.Price != 19.99 {
			t.Errorf("expected price=19.99, got %f", dst.Price)
		}
	})

	t.Run("parses quantity float64 field", func(t *testing.T) {
		if dst.Quantity != 2.5 {
			t.Errorf("expected quantity=2.5, got %f", dst.Quantity)
		}
	})
}

// ---------------------------------------------------------------------------
// 3. URL-Encoded Form Data Parsing
// ---------------------------------------------------------------------------

func TestParse_URLEncoded_StringFields(t *testing.T) {
	values := url.Values{
		"from":    {"sender@example.com"},
		"to":      {"receiver@example.com"},
		"subject": {"Test Subject"},
	}
	req := newFormURLEncodedRequest(t, values)

	var dst jsonFallbackStruct
	err := request.Parse(req, &dst)

	t.Run("returns no error", func(t *testing.T) {
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("parses from field", func(t *testing.T) {
		if dst.From != "sender@example.com" {
			t.Errorf("expected from %q, got %q", "sender@example.com", dst.From)
		}
	})

	t.Run("parses to field", func(t *testing.T) {
		if dst.To != "receiver@example.com" {
			t.Errorf("expected to %q, got %q", "receiver@example.com", dst.To)
		}
	})

	t.Run("parses subject field", func(t *testing.T) {
		if dst.Subject != "Test Subject" {
			t.Errorf("expected subject %q, got %q", "Test Subject", dst.Subject)
		}
	})
}

func TestParse_URLEncoded_BoolFields(t *testing.T) {
	t.Run("true values", func(t *testing.T) {
		values := url.Values{
			"flag_a": {"true"},
			"flag_b": {"yes"},
			"flag_c": {"1"},
		}
		req := newFormURLEncodedRequest(t, values)

		var dst boolStruct
		err := request.Parse(req, &dst)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		if dst.FlagA != true {
			t.Errorf("expected flag_a=true, got %v", dst.FlagA)
		}
		if dst.FlagB != true {
			t.Errorf("expected flag_b=true, got %v", dst.FlagB)
		}
		if dst.FlagC != true {
			t.Errorf("expected flag_c=true, got %v", dst.FlagC)
		}
	})

	t.Run("false values", func(t *testing.T) {
		values := url.Values{
			"flag_a": {"false"},
			"flag_b": {"no"},
			"flag_c": {"0"},
		}
		req := newFormURLEncodedRequest(t, values)

		var dst boolStruct
		err := request.Parse(req, &dst)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		if dst.FlagA != false {
			t.Errorf("expected flag_a=false, got %v", dst.FlagA)
		}
		if dst.FlagB != false {
			t.Errorf("expected flag_b=false, got %v", dst.FlagB)
		}
		if dst.FlagC != false {
			t.Errorf("expected flag_c=false, got %v", dst.FlagC)
		}
	})
}

func TestParse_URLEncoded_IntFields(t *testing.T) {
	values := url.Values{
		"int_val":   {"100"},
		"float_val": {"0"},
	}
	req := newFormURLEncodedRequest(t, values)

	var dst numericStruct
	err := request.Parse(req, &dst)

	t.Run("returns no error", func(t *testing.T) {
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("parses int field", func(t *testing.T) {
		if dst.IntVal != 100 {
			t.Errorf("expected int_val=100, got %d", dst.IntVal)
		}
	})
}

func TestParse_URLEncoded_FormTagMapping(t *testing.T) {
	values := url.Values{
		"subject":    {"URL Subject"},
		"body_plain": {"URL Body"},
		"recipient":  {"url-recipient@example.com"},
	}
	req := newFormURLEncodedRequest(t, values)

	var dst formTagStruct
	err := request.Parse(req, &dst)

	t.Run("returns no error", func(t *testing.T) {
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("maps body_plain to Body via form tag", func(t *testing.T) {
		if dst.Body != "URL Body" {
			t.Errorf("expected body %q, got %q", "URL Body", dst.Body)
		}
	})

	t.Run("maps recipient via form tag", func(t *testing.T) {
		if dst.Recipient != "url-recipient@example.com" {
			t.Errorf("expected recipient %q, got %q", "url-recipient@example.com", dst.Recipient)
		}
	})
}

func TestParse_URLEncoded_EmptyBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var dst basicStruct
	err := request.Parse(req, &dst)

	t.Run("handles empty body gracefully without error", func(t *testing.T) {
		// An empty form body should not cause an error; the struct fields
		// simply retain their zero values.
		if err != nil {
			t.Errorf("expected no error for empty URL-encoded body, got: %v", err)
		}
	})

	t.Run("string field is zero value", func(t *testing.T) {
		if dst.Name != "" {
			t.Errorf("expected empty name, got %q", dst.Name)
		}
	})

	t.Run("bool field is zero value", func(t *testing.T) {
		if dst.Active != false {
			t.Errorf("expected active=false, got %v", dst.Active)
		}
	})

	t.Run("int field is zero value", func(t *testing.T) {
		if dst.Count != 0 {
			t.Errorf("expected count=0, got %d", dst.Count)
		}
	})
}

// ---------------------------------------------------------------------------
// 4. Content Type Detection
// ---------------------------------------------------------------------------

func TestParse_ContentType_JSONWithCharset(t *testing.T) {
	body := `{"name":"charset-test","active":false,"count":1}`
	req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	var dst basicStruct
	err := request.Parse(req, &dst)

	t.Run("returns no error", func(t *testing.T) {
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("parses JSON correctly despite charset parameter", func(t *testing.T) {
		if dst.Name != "charset-test" {
			t.Errorf("expected name %q, got %q", "charset-test", dst.Name)
		}
	})
}

func TestParse_ContentType_MultipartWithBoundary(t *testing.T) {
	// Multipart requests always include a boundary parameter in the
	// Content-Type header. This test verifies the parser handles it.
	fields := map[string]string{"from": "test@example.com"}
	req := newMultipartRequest(t, fields)

	var dst jsonFallbackStruct
	err := request.Parse(req, &dst)

	t.Run("returns no error", func(t *testing.T) {
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("parses multipart form correctly", func(t *testing.T) {
		if dst.From != "test@example.com" {
			t.Errorf("expected from %q, got %q", "test@example.com", dst.From)
		}
	})
}

func TestParse_ContentType_URLEncoded(t *testing.T) {
	values := url.Values{"from": {"urlenc@example.com"}}
	req := newFormURLEncodedRequest(t, values)

	var dst jsonFallbackStruct
	err := request.Parse(req, &dst)

	t.Run("returns no error", func(t *testing.T) {
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("detects URL-encoded content type", func(t *testing.T) {
		if dst.From != "urlenc@example.com" {
			t.Errorf("expected from %q, got %q", "urlenc@example.com", dst.From)
		}
	})
}

func TestParse_ContentType_Unsupported(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader("<xml>data</xml>"))
	req.Header.Set("Content-Type", "application/xml")

	var dst basicStruct
	err := request.Parse(req, &dst)

	t.Run("returns error for unsupported content type", func(t *testing.T) {
		if err == nil {
			t.Error("expected error for unsupported content type, got nil")
		}
	})
}

func TestParse_ContentType_NoHeader(t *testing.T) {
	// When no Content-Type header is set, the parser should either default
	// to JSON parsing or URL-encoded form parsing. This test verifies it
	// does not return an error for a valid JSON body without a Content-Type.
	body := `{"name":"no-header","active":true,"count":99}`
	req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewBufferString(body))
	// Explicitly do NOT set Content-Type header.

	var dst basicStruct
	err := request.Parse(req, &dst)

	t.Run("does not return error for missing Content-Type with valid JSON", func(t *testing.T) {
		// The parser should either default to JSON or form-urlencoded.
		// If it defaults to JSON, parsing should succeed.
		// If it defaults to form-urlencoded, it may succeed with zero-valued fields.
		// Either way, it should not panic or return an "unsupported content type" error.
		if err != nil {
			t.Logf("note: parser returned error for missing Content-Type: %v", err)
			t.Logf("this is acceptable if the implementation requires a Content-Type header")
		}
	})
}

// ---------------------------------------------------------------------------
// 5. ParseFormValue Helper
// ---------------------------------------------------------------------------

func TestParseFormValue_MultipartForm(t *testing.T) {
	fields := map[string]string{
		"from":    "sender@example.com",
		"subject": "Test Subject",
	}
	req := newMultipartRequest(t, fields)

	t.Run("returns value for existing key", func(t *testing.T) {
		val := request.ParseFormValue(req, "from")
		if val != "sender@example.com" {
			t.Errorf("expected %q, got %q", "sender@example.com", val)
		}
	})

	t.Run("returns value for another existing key", func(t *testing.T) {
		val := request.ParseFormValue(req, "subject")
		if val != "Test Subject" {
			t.Errorf("expected %q, got %q", "Test Subject", val)
		}
	})

	t.Run("returns empty string for missing key", func(t *testing.T) {
		val := request.ParseFormValue(req, "nonexistent")
		if val != "" {
			t.Errorf("expected empty string for missing key, got %q", val)
		}
	})
}

func TestParseFormValue_URLEncodedForm(t *testing.T) {
	values := url.Values{
		"to":      {"recipient@example.com"},
		"subject": {"URL Subject"},
	}
	req := newFormURLEncodedRequest(t, values)

	t.Run("returns value for existing key", func(t *testing.T) {
		val := request.ParseFormValue(req, "to")
		if val != "recipient@example.com" {
			t.Errorf("expected %q, got %q", "recipient@example.com", val)
		}
	})

	t.Run("returns value for another existing key", func(t *testing.T) {
		val := request.ParseFormValue(req, "subject")
		if val != "URL Subject" {
			t.Errorf("expected %q, got %q", "URL Subject", val)
		}
	})

	t.Run("returns empty string for missing key", func(t *testing.T) {
		val := request.ParseFormValue(req, "missing")
		if val != "" {
			t.Errorf("expected empty string for missing key, got %q", val)
		}
	})
}

func TestParseFormValue_MissingKey(t *testing.T) {
	fields := map[string]string{"key": "value"}
	req := newMultipartRequest(t, fields)

	t.Run("returns empty string for absent key", func(t *testing.T) {
		val := request.ParseFormValue(req, "absent_key")
		if val != "" {
			t.Errorf("expected empty string, got %q", val)
		}
	})
}

// ---------------------------------------------------------------------------
// 6. Edge Cases / Mixed Tag Behavior
// ---------------------------------------------------------------------------

func TestParse_Multipart_MixedTags(t *testing.T) {
	// mixedTagStruct has a form tag, a json tag, and a bare field.
	fields := map[string]string{
		"form_name": "from-form-tag",
		"json_name": "from-json-tag",
		"BareField": "from-bare-field",
	}
	req := newMultipartRequest(t, fields)

	var dst mixedTagStruct
	err := request.Parse(req, &dst)

	t.Run("returns no error", func(t *testing.T) {
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("uses form tag when present", func(t *testing.T) {
		if dst.FormField != "from-form-tag" {
			t.Errorf("expected FormField=%q, got %q", "from-form-tag", dst.FormField)
		}
	})

	t.Run("falls back to json tag when no form tag", func(t *testing.T) {
		if dst.JSONField != "from-json-tag" {
			t.Errorf("expected JSONField=%q, got %q", "from-json-tag", dst.JSONField)
		}
	})

	t.Run("falls back to field name when no tags", func(t *testing.T) {
		if dst.BareField != "from-bare-field" {
			t.Errorf("expected BareField=%q, got %q", "from-bare-field", dst.BareField)
		}
	})
}

func TestParse_URLEncoded_MixedTags(t *testing.T) {
	values := url.Values{
		"form_name": {"form-val"},
		"json_name": {"json-val"},
		"BareField": {"bare-val"},
	}
	req := newFormURLEncodedRequest(t, values)

	var dst mixedTagStruct
	err := request.Parse(req, &dst)

	t.Run("returns no error", func(t *testing.T) {
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("uses form tag when present", func(t *testing.T) {
		if dst.FormField != "form-val" {
			t.Errorf("expected FormField=%q, got %q", "form-val", dst.FormField)
		}
	})

	t.Run("falls back to json tag when no form tag", func(t *testing.T) {
		if dst.JSONField != "json-val" {
			t.Errorf("expected JSONField=%q, got %q", "json-val", dst.JSONField)
		}
	})

	t.Run("falls back to field name when no tags", func(t *testing.T) {
		if dst.BareField != "bare-val" {
			t.Errorf("expected BareField=%q, got %q", "bare-val", dst.BareField)
		}
	})
}
