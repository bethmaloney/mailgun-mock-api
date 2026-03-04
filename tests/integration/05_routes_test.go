package integration

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/mailgun/mailgun-go/v5/mtypes"
)

// doRepeatedFormRequest sends a URL-encoded form request that supports repeated keys.
func doRepeatedFormRequest(method, path string, values url.Values) (*http.Response, error) {
	body := values.Encode()
	req, err := http.NewRequest(method, baseURL+path, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth("api", apiKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return http.DefaultClient.Do(req)
}

func TestRoutes(t *testing.T) {
	resetServer(t)

	var createdRouteID string

	// --- Create Route ---

	t.Run("SDK_CreateRoute", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		route, err := mg.CreateRoute(ctx, mtypes.Route{
			Priority:    1,
			Description: "test route",
			Expression:  `match_recipient(".*@example.com")`,
			Actions:     []string{"forward(\"http://example.com/hook\")", "stop()"},
		})
		if err != nil {
			reporter.Record("Routes", "CreateRoute", "SDK", false, err.Error())
			t.Fatalf("CreateRoute: %v", err)
		}

		if route.Id == "" {
			t.Fatal("expected non-empty route ID")
		}
		if route.Description != "test route" {
			t.Fatalf("expected description 'test route', got %q", route.Description)
		}
		if route.Expression != `match_recipient(".*@example.com")` {
			t.Fatalf("expression mismatch: %q", route.Expression)
		}
		if len(route.Actions) != 2 {
			t.Fatalf("expected 2 actions, got %d", len(route.Actions))
		}
		createdRouteID = route.Id
		reporter.Record("Routes", "CreateRoute", "SDK", true, "")
	})

	t.Run("HTTP_CreateRoute", func(t *testing.T) {
		vals := url.Values{}
		vals.Set("priority", "2")
		vals.Set("description", "http route")
		vals.Set("expression", `match_recipient(".*@test.com")`)
		vals.Add("action", "forward(\"http://test.com/hook\")")
		vals.Add("action", "stop()")

		resp, err := doRepeatedFormRequest("POST", "/v3/routes", vals)
		if err != nil {
			reporter.Record("Routes", "CreateRoute", "HTTP", false, err.Error())
			t.Fatalf("POST /v3/routes: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result struct {
			Message string                 `json:"message"`
			Route   map[string]interface{} `json:"route"`
		}
		readJSON(t, resp, &result)
		if result.Route["id"] == nil || result.Route["id"].(string) == "" {
			t.Fatal("expected route id in response")
		}
		if result.Route["description"] != "http route" {
			t.Fatalf("expected description 'http route', got %v", result.Route["description"])
		}
		actions, ok := result.Route["actions"].([]interface{})
		if !ok || len(actions) != 2 {
			t.Fatalf("expected 2 actions, got %v", result.Route["actions"])
		}
		reporter.Record("Routes", "CreateRoute", "HTTP", true, "")
	})

	// --- List Routes ---

	t.Run("SDK_ListRoutes", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		iter := mg.ListRoutes(nil)
		var routes []mtypes.Route
		if !iter.First(ctx, &routes) {
			if iter.Err() != nil {
				reporter.Record("Routes", "ListRoutes", "SDK", false, iter.Err().Error())
				t.Fatalf("ListRoutes: %v", iter.Err())
			}
			reporter.Record("Routes", "ListRoutes", "SDK", false, "no routes returned")
			t.Fatal("expected routes, got none")
		}

		if len(routes) < 2 {
			t.Fatalf("expected at least 2 routes, got %d", len(routes))
		}
		reporter.Record("Routes", "ListRoutes", "SDK", true, "")
	})

	t.Run("HTTP_ListRoutes", func(t *testing.T) {
		resp, err := doRequest("GET", "/v3/routes", nil)
		if err != nil {
			reporter.Record("Routes", "ListRoutes", "HTTP", false, err.Error())
			t.Fatalf("GET /v3/routes: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result struct {
			TotalCount int                      `json:"total_count"`
			Items      []map[string]interface{} `json:"items"`
		}
		readJSON(t, resp, &result)
		if result.TotalCount < 2 {
			t.Fatalf("expected total_count >= 2, got %d", result.TotalCount)
		}
		if len(result.Items) < 2 {
			t.Fatalf("expected >= 2 items, got %d", len(result.Items))
		}
		reporter.Record("Routes", "ListRoutes", "HTTP", true, "")
	})

	// --- Get Route ---

	t.Run("SDK_GetRoute", func(t *testing.T) {
		if createdRouteID == "" {
			t.Skip("no route ID from SDK_CreateRoute")
		}
		mg := newMailgunClient()
		ctx := context.Background()

		route, err := mg.GetRoute(ctx, createdRouteID)
		if err != nil {
			reporter.Record("Routes", "GetRoute", "SDK", false, err.Error())
			t.Fatalf("GetRoute: %v", err)
		}

		if route.Id != createdRouteID {
			t.Fatalf("expected route ID %q, got %q", createdRouteID, route.Id)
		}
		if route.Description != "test route" {
			t.Fatalf("expected description 'test route', got %q", route.Description)
		}
		reporter.Record("Routes", "GetRoute", "SDK", true, "")
	})

	t.Run("HTTP_GetRoute", func(t *testing.T) {
		if createdRouteID == "" {
			t.Skip("no route ID from SDK_CreateRoute")
		}
		resp, err := doRequest("GET", fmt.Sprintf("/v3/routes/%s", createdRouteID), nil)
		if err != nil {
			reporter.Record("Routes", "GetRoute", "HTTP", false, err.Error())
			t.Fatalf("GET /v3/routes/%s: %v", createdRouteID, err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result struct {
			Route map[string]interface{} `json:"route"`
		}
		readJSON(t, resp, &result)
		if result.Route["id"] != createdRouteID {
			t.Fatalf("expected route id %q, got %v", createdRouteID, result.Route["id"])
		}
		reporter.Record("Routes", "GetRoute", "HTTP", true, "")
	})

	// --- Update Route ---

	t.Run("SDK_UpdateRoute", func(t *testing.T) {
		if createdRouteID == "" {
			t.Skip("no route ID from SDK_CreateRoute")
		}
		mg := newMailgunClient()
		ctx := context.Background()

		updated, err := mg.UpdateRoute(ctx, createdRouteID, mtypes.Route{
			Description: "updated route",
		})
		if err != nil {
			reporter.Record("Routes", "UpdateRoute", "SDK", false, err.Error())
			t.Fatalf("UpdateRoute: %v", err)
		}

		if updated.Description != "updated route" {
			t.Fatalf("expected description 'updated route', got %q", updated.Description)
		}
		reporter.Record("Routes", "UpdateRoute", "SDK", true, "")
	})

	t.Run("HTTP_UpdateRoute", func(t *testing.T) {
		if createdRouteID == "" {
			t.Skip("no route ID from SDK_CreateRoute")
		}

		vals := url.Values{}
		vals.Set("description", "http updated route")
		resp, err := doRepeatedFormRequest("PUT", fmt.Sprintf("/v3/routes/%s", createdRouteID), vals)
		if err != nil {
			reporter.Record("Routes", "UpdateRoute", "HTTP", false, err.Error())
			t.Fatalf("PUT /v3/routes/%s: %v", createdRouteID, err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)
		if result["description"] != "http updated route" {
			t.Fatalf("expected 'http updated route', got %v", result["description"])
		}
		reporter.Record("Routes", "UpdateRoute", "HTTP", true, "")
	})

	// --- Delete Route ---

	t.Run("SDK_DeleteRoute", func(t *testing.T) {
		if createdRouteID == "" {
			t.Skip("no route ID from SDK_CreateRoute")
		}
		mg := newMailgunClient()
		ctx := context.Background()

		err := mg.DeleteRoute(ctx, createdRouteID)
		if err != nil {
			reporter.Record("Routes", "DeleteRoute", "SDK", false, err.Error())
			t.Fatalf("DeleteRoute: %v", err)
		}

		// Verify it's gone
		_, err = mg.GetRoute(ctx, createdRouteID)
		if err == nil {
			t.Fatal("expected error getting deleted route")
		}
		reporter.Record("Routes", "DeleteRoute", "SDK", true, "")
	})

	t.Run("HTTP_DeleteRoute", func(t *testing.T) {
		// Create a route to delete
		vals := url.Values{}
		vals.Set("priority", "5")
		vals.Set("description", "delete me")
		vals.Set("expression", `match_recipient(".*@delete.com")`)
		vals.Add("action", "stop()")

		resp, err := doRepeatedFormRequest("POST", "/v3/routes", vals)
		if err != nil {
			t.Fatalf("setup: create route: %v", err)
		}
		var createResult struct {
			Route map[string]interface{} `json:"route"`
		}
		readJSON(t, resp, &createResult)
		deleteID := createResult.Route["id"].(string)

		resp, err = doRequest("DELETE", fmt.Sprintf("/v3/routes/%s", deleteID), nil)
		if err != nil {
			reporter.Record("Routes", "DeleteRoute", "HTTP", false, err.Error())
			t.Fatalf("DELETE /v3/routes/%s: %v", deleteID, err)
		}
		assertStatus(t, resp, http.StatusOK)

		var delResult struct {
			Message string `json:"message"`
			ID      string `json:"id"`
		}
		readJSON(t, resp, &delResult)
		if delResult.ID != deleteID {
			t.Fatalf("expected deleted id %q, got %q", deleteID, delResult.ID)
		}

		// Verify it's gone
		resp, err = doRequest("GET", fmt.Sprintf("/v3/routes/%s", deleteID), nil)
		if err != nil {
			t.Fatalf("verify delete: %v", err)
		}
		assertStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()

		reporter.Record("Routes", "DeleteRoute", "HTTP", true, "")
	})
}
