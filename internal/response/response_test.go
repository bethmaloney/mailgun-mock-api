package response_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bethmaloney/mailgun-mock-api/internal/response"
)

// ---------------------------------------------------------------------------
// RespondJSON tests
// ---------------------------------------------------------------------------

func TestRespondJSON(t *testing.T) {
	t.Run("sets Content-Type to application/json", func(t *testing.T) {
		rec := httptest.NewRecorder()
		response.RespondJSON(rec, http.StatusOK, map[string]string{"key": "value"})

		ct := rec.Header().Get("Content-Type")
		if ct != "application/json" {
			t.Errorf("expected Content-Type %q, got %q", "application/json", ct)
		}
	})

	t.Run("sets correct status code 200", func(t *testing.T) {
		rec := httptest.NewRecorder()
		response.RespondJSON(rec, http.StatusOK, map[string]string{"key": "value"})

		if rec.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
		}
	})

	t.Run("sets correct status code 201", func(t *testing.T) {
		rec := httptest.NewRecorder()
		response.RespondJSON(rec, http.StatusCreated, map[string]string{"id": "123"})

		if rec.Code != http.StatusCreated {
			t.Errorf("expected status %d, got %d", http.StatusCreated, rec.Code)
		}
	})

	t.Run("sets correct status code 404", func(t *testing.T) {
		rec := httptest.NewRecorder()
		response.RespondJSON(rec, http.StatusNotFound, map[string]string{"message": "not found"})

		if rec.Code != http.StatusNotFound {
			t.Errorf("expected status %d, got %d", http.StatusNotFound, rec.Code)
		}
	})

	t.Run("encodes map to JSON correctly", func(t *testing.T) {
		rec := httptest.NewRecorder()
		data := map[string]string{"foo": "bar", "baz": "qux"}
		response.RespondJSON(rec, http.StatusOK, data)

		var result map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
			t.Fatalf("failed to decode JSON body: %v", err)
		}
		if result["foo"] != "bar" {
			t.Errorf("expected foo=%q, got %q", "bar", result["foo"])
		}
		if result["baz"] != "qux" {
			t.Errorf("expected baz=%q, got %q", "qux", result["baz"])
		}
	})

	t.Run("encodes struct to JSON correctly", func(t *testing.T) {
		type testStruct struct {
			Name  string `json:"name"`
			Count int    `json:"count"`
		}
		rec := httptest.NewRecorder()
		data := testStruct{Name: "test", Count: 42}
		response.RespondJSON(rec, http.StatusOK, data)

		var result testStruct
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
			t.Fatalf("failed to decode JSON body: %v", err)
		}
		if result.Name != "test" {
			t.Errorf("expected name=%q, got %q", "test", result.Name)
		}
		if result.Count != 42 {
			t.Errorf("expected count=%d, got %d", 42, result.Count)
		}
	})

	t.Run("encodes nested structs correctly", func(t *testing.T) {
		data := map[string]interface{}{
			"outer": map[string]string{
				"inner": "value",
			},
		}
		rec := httptest.NewRecorder()
		response.RespondJSON(rec, http.StatusOK, data)

		var result map[string]map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
			t.Fatalf("failed to decode JSON body: %v", err)
		}
		if result["outer"]["inner"] != "value" {
			t.Errorf("expected nested value %q, got %q", "value", result["outer"]["inner"])
		}
	})

	t.Run("produces valid JSON output", func(t *testing.T) {
		rec := httptest.NewRecorder()
		response.RespondJSON(rec, http.StatusOK, map[string]int{"count": 5})

		if !json.Valid(rec.Body.Bytes()) {
			t.Errorf("response body is not valid JSON: %s", rec.Body.String())
		}
	})
}

// ---------------------------------------------------------------------------
// RespondError tests
// ---------------------------------------------------------------------------

func TestRespondError(t *testing.T) {
	t.Run("returns correct status code 400", func(t *testing.T) {
		rec := httptest.NewRecorder()
		response.RespondError(rec, http.StatusBadRequest, "invalid input")

		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
		}
	})

	t.Run("returns correct status code 404", func(t *testing.T) {
		rec := httptest.NewRecorder()
		response.RespondError(rec, http.StatusNotFound, "not found")

		if rec.Code != http.StatusNotFound {
			t.Errorf("expected status %d, got %d", http.StatusNotFound, rec.Code)
		}
	})

	t.Run("returns correct status code 500", func(t *testing.T) {
		rec := httptest.NewRecorder()
		response.RespondError(rec, http.StatusInternalServerError, "server error")

		if rec.Code != http.StatusInternalServerError {
			t.Errorf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
		}
	})

	t.Run("sets Content-Type to application/json", func(t *testing.T) {
		rec := httptest.NewRecorder()
		response.RespondError(rec, http.StatusBadRequest, "bad request")

		ct := rec.Header().Get("Content-Type")
		if ct != "application/json" {
			t.Errorf("expected Content-Type %q, got %q", "application/json", ct)
		}
	})

	t.Run("body contains message key", func(t *testing.T) {
		rec := httptest.NewRecorder()
		response.RespondError(rec, http.StatusBadRequest, "something went wrong")

		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if _, ok := body["message"]; !ok {
			t.Error("response body missing 'message' key")
		}
	})

	t.Run("message matches provided text", func(t *testing.T) {
		rec := httptest.NewRecorder()
		msg := "validation failed: missing required field"
		response.RespondError(rec, http.StatusBadRequest, msg)

		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if body["message"] != msg {
			t.Errorf("expected message %q, got %q", msg, body["message"])
		}
	})

	t.Run("body has only message key", func(t *testing.T) {
		rec := httptest.NewRecorder()
		response.RespondError(rec, http.StatusBadRequest, "error")

		var body map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(body) != 1 {
			t.Errorf("expected exactly 1 key in error response, got %d", len(body))
		}
	})

	t.Run("produces valid JSON", func(t *testing.T) {
		rec := httptest.NewRecorder()
		response.RespondError(rec, http.StatusBadRequest, "error")

		if !json.Valid(rec.Body.Bytes()) {
			t.Errorf("response body is not valid JSON: %s", rec.Body.String())
		}
	})
}

// ---------------------------------------------------------------------------
// RespondSuccess tests
// ---------------------------------------------------------------------------

func TestRespondSuccess(t *testing.T) {
	t.Run("returns 200 status code", func(t *testing.T) {
		rec := httptest.NewRecorder()
		response.RespondSuccess(rec, "operation completed")

		if rec.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
		}
	})

	t.Run("sets Content-Type to application/json", func(t *testing.T) {
		rec := httptest.NewRecorder()
		response.RespondSuccess(rec, "done")

		ct := rec.Header().Get("Content-Type")
		if ct != "application/json" {
			t.Errorf("expected Content-Type %q, got %q", "application/json", ct)
		}
	})

	t.Run("body contains message key", func(t *testing.T) {
		rec := httptest.NewRecorder()
		response.RespondSuccess(rec, "success")

		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if _, ok := body["message"]; !ok {
			t.Error("response body missing 'message' key")
		}
	})

	t.Run("message matches provided text", func(t *testing.T) {
		rec := httptest.NewRecorder()
		msg := "All data has been reset"
		response.RespondSuccess(rec, msg)

		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if body["message"] != msg {
			t.Errorf("expected message %q, got %q", msg, body["message"])
		}
	})

	t.Run("body has only message key", func(t *testing.T) {
		rec := httptest.NewRecorder()
		response.RespondSuccess(rec, "ok")

		var body map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(body) != 1 {
			t.Errorf("expected exactly 1 key in success response, got %d", len(body))
		}
	})

	t.Run("produces valid JSON", func(t *testing.T) {
		rec := httptest.NewRecorder()
		response.RespondSuccess(rec, "it worked")

		if !json.Valid(rec.Body.Bytes()) {
			t.Errorf("response body is not valid JSON: %s", rec.Body.String())
		}
	})

	t.Run("works with empty message", func(t *testing.T) {
		rec := httptest.NewRecorder()
		response.RespondSuccess(rec, "")

		if rec.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
		}

		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if body["message"] != "" {
			t.Errorf("expected empty message, got %q", body["message"])
		}
	})
}
