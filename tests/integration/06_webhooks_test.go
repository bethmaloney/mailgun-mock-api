package integration

import (
	"context"
	"net/http"
	"net/url"
	"testing"
)

func TestWebhooks(t *testing.T) {
	resetServer(t)

	const domain = "webhook-test.example.com"

	// Create a domain first so webhook endpoints have something to attach to.
	resp, err := doFormRequest("POST", "/v4/domains", map[string]string{"name": domain})
	if err != nil {
		t.Fatalf("setup: create domain: %v", err)
	}
	resp.Body.Close()

	// --- Domain Webhooks (v3) ---

	t.Run("SDK_CreateWebhook", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		err := mg.CreateWebhook(ctx, domain, "delivered", []string{"http://example.com/sdk-hook1"})
		if err != nil {
			reporter.Record("Webhooks", "CreateWebhook", "SDK", false, err.Error())
			t.Fatalf("CreateWebhook: %v", err)
		}

		// Verify it was created by fetching it
		urls, err := mg.GetWebhook(ctx, domain, "delivered")
		if err != nil {
			reporter.Record("Webhooks", "CreateWebhook", "SDK", false, err.Error())
			t.Fatalf("GetWebhook after create: %v", err)
		}
		if len(urls) == 0 {
			t.Fatal("expected at least one URL for delivered webhook")
		}
		found := false
		for _, u := range urls {
			if u == "http://example.com/sdk-hook1" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected URL http://example.com/sdk-hook1 in %v", urls)
		}

		reporter.Record("Webhooks", "CreateWebhook", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_CreateWebhook", func(t *testing.T) {
		vals := url.Values{}
		vals.Set("id", "clicked")
		vals.Add("url", "http://example.com/http-hook1")

		resp, err := doRepeatedFormRequest("POST", "/v3/domains/"+domain+"/webhooks", vals)
		if err != nil {
			reporter.Record("Webhooks", "CreateWebhook", "HTTP", false, err.Error())
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "Webhook has been created" {
			t.Errorf("expected creation message, got %v", result["message"])
		}

		webhook, ok := result["webhook"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected webhook object in response, got %v", result["webhook"])
		}
		urls, ok := webhook["urls"].([]interface{})
		if !ok || len(urls) == 0 {
			t.Fatalf("expected non-empty urls array, got %v", webhook["urls"])
		}

		reporter.Record("Webhooks", "CreateWebhook", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_ListWebhooks", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		webhooks, err := mg.ListWebhooks(ctx, domain)
		if err != nil {
			reporter.Record("Webhooks", "ListWebhooks", "SDK", false, err.Error())
			t.Fatalf("ListWebhooks: %v", err)
		}

		// We created "delivered" and "clicked" above, so both should be present
		if _, ok := webhooks["delivered"]; !ok {
			t.Errorf("expected 'delivered' webhook in list, got keys: %v", webhooks)
		}
		if _, ok := webhooks["clicked"]; !ok {
			t.Errorf("expected 'clicked' webhook in list, got keys: %v", webhooks)
		}

		reporter.Record("Webhooks", "ListWebhooks", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_ListWebhooks", func(t *testing.T) {
		resp, err := doRequest("GET", "/v3/domains/"+domain+"/webhooks", nil)
		if err != nil {
			reporter.Record("Webhooks", "ListWebhooks", "HTTP", false, err.Error())
			t.Fatalf("GET webhooks: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result struct {
			Webhooks map[string]interface{} `json:"webhooks"`
		}
		readJSON(t, resp, &result)

		if result.Webhooks == nil {
			t.Fatal("expected webhooks map in response")
		}

		// All 8 event types should be present in the response
		eventTypes := []string{"accepted", "delivered", "opened", "clicked", "unsubscribed", "complained", "temporary_fail", "permanent_fail"}
		for _, et := range eventTypes {
			if _, ok := result.Webhooks[et]; !ok {
				t.Errorf("expected event type %q in webhooks map", et)
			}
		}

		reporter.Record("Webhooks", "ListWebhooks", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_GetWebhook", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		urls, err := mg.GetWebhook(ctx, domain, "clicked")
		if err != nil {
			reporter.Record("Webhooks", "GetWebhook", "SDK", false, err.Error())
			t.Fatalf("GetWebhook: %v", err)
		}

		if len(urls) == 0 {
			t.Fatal("expected at least one URL for clicked webhook")
		}
		found := false
		for _, u := range urls {
			if u == "http://example.com/http-hook1" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected http://example.com/http-hook1 in %v", urls)
		}

		reporter.Record("Webhooks", "GetWebhook", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_GetWebhook", func(t *testing.T) {
		resp, err := doRequest("GET", "/v3/domains/"+domain+"/webhooks/clicked", nil)
		if err != nil {
			reporter.Record("Webhooks", "GetWebhook", "HTTP", false, err.Error())
			t.Fatalf("GET webhook: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result struct {
			Webhook struct {
				URLs []string `json:"urls"`
			} `json:"webhook"`
		}
		readJSON(t, resp, &result)

		if len(result.Webhook.URLs) == 0 {
			t.Fatal("expected at least one URL in webhook response")
		}

		reporter.Record("Webhooks", "GetWebhook", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_UpdateWebhook", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		err := mg.UpdateWebhook(ctx, domain, "clicked", []string{"http://example.com/updated-sdk-hook"})
		if err != nil {
			reporter.Record("Webhooks", "UpdateWebhook", "SDK", false, err.Error())
			t.Fatalf("UpdateWebhook: %v", err)
		}

		// Verify the update
		urls, err := mg.GetWebhook(ctx, domain, "clicked")
		if err != nil {
			t.Fatalf("GetWebhook after update: %v", err)
		}
		found := false
		for _, u := range urls {
			if u == "http://example.com/updated-sdk-hook" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected updated URL in %v", urls)
		}

		reporter.Record("Webhooks", "UpdateWebhook", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_UpdateWebhook", func(t *testing.T) {
		vals := url.Values{}
		vals.Add("url", "http://example.com/updated-http-hook")

		resp, err := doRepeatedFormRequest("PUT", "/v3/domains/"+domain+"/webhooks/clicked", vals)
		if err != nil {
			reporter.Record("Webhooks", "UpdateWebhook", "HTTP", false, err.Error())
			t.Fatalf("PUT webhook: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "Webhook has been updated" {
			t.Errorf("expected update message, got %v", result["message"])
		}

		webhook, ok := result["webhook"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected webhook object in response")
		}
		urls, ok := webhook["urls"].([]interface{})
		if !ok || len(urls) == 0 {
			t.Fatalf("expected non-empty urls array, got %v", webhook["urls"])
		}

		reporter.Record("Webhooks", "UpdateWebhook", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_DeleteWebhook", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		err := mg.DeleteWebhook(ctx, domain, "delivered")
		if err != nil {
			reporter.Record("Webhooks", "DeleteWebhook", "SDK", false, err.Error())
			t.Fatalf("DeleteWebhook: %v", err)
		}

		// Verify deletion - GetWebhook should return empty or error
		urls, err := mg.GetWebhook(ctx, domain, "delivered")
		if err == nil && len(urls) > 0 {
			t.Fatalf("expected empty URLs after delete, got %v", urls)
		}

		reporter.Record("Webhooks", "DeleteWebhook", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_DeleteWebhook", func(t *testing.T) {
		resp, err := doRequest("DELETE", "/v3/domains/"+domain+"/webhooks/clicked", nil)
		if err != nil {
			reporter.Record("Webhooks", "DeleteWebhook", "HTTP", false, err.Error())
			t.Fatalf("DELETE webhook: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "Webhook has been deleted" {
			t.Errorf("expected deletion message, got %v", result["message"])
		}

		webhook, ok := result["webhook"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected webhook object in response")
		}
		urls, ok := webhook["urls"].([]interface{})
		if !ok || len(urls) != 0 {
			t.Errorf("expected empty urls array after delete, got %v", webhook["urls"])
		}

		reporter.Record("Webhooks", "DeleteWebhook", "HTTP", !t.Failed(), "")
	})

	// --- Account Webhooks (v1) ---

	var createdAccountWebhookID string

	t.Run("HTTP_CreateAccountWebhook", func(t *testing.T) {
		vals := url.Values{}
		vals.Set("url", "http://example.com/account-hook")
		vals.Add("event_types", "delivered")
		vals.Add("event_types", "opened")
		vals.Set("description", "test account webhook")

		resp, err := doRepeatedFormRequest("POST", "/v1/webhooks", vals)
		if err != nil {
			reporter.Record("Webhooks", "CreateAccountWebhook", "HTTP", false, err.Error())
			t.Fatalf("POST /v1/webhooks: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		webhookID, ok := result["webhook_id"].(string)
		if !ok || webhookID == "" {
			t.Fatalf("expected webhook_id in response, got %v", result)
		}
		createdAccountWebhookID = webhookID

		reporter.Record("Webhooks", "CreateAccountWebhook", "HTTP", !t.Failed(), "")
	})

	t.Run("HTTP_ListAccountWebhooks", func(t *testing.T) {
		resp, err := doRequest("GET", "/v1/webhooks", nil)
		if err != nil {
			reporter.Record("Webhooks", "ListAccountWebhooks", "HTTP", false, err.Error())
			t.Fatalf("GET /v1/webhooks: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result struct {
			Webhooks []map[string]interface{} `json:"webhooks"`
		}
		readJSON(t, resp, &result)

		if len(result.Webhooks) < 1 {
			t.Fatalf("expected at least 1 webhook, got %d", len(result.Webhooks))
		}

		// Find the one we created
		found := false
		for _, wh := range result.Webhooks {
			if wh["webhook_id"] == createdAccountWebhookID {
				found = true
				if wh["url"] != "http://example.com/account-hook" {
					t.Errorf("expected url http://example.com/account-hook, got %v", wh["url"])
				}
				if wh["description"] != "test account webhook" {
					t.Errorf("expected description 'test account webhook', got %v", wh["description"])
				}
				break
			}
		}
		if !found && createdAccountWebhookID != "" {
			t.Errorf("created webhook %s not found in list", createdAccountWebhookID)
		}

		reporter.Record("Webhooks", "ListAccountWebhooks", "HTTP", !t.Failed(), "")
	})

	t.Run("HTTP_GetAccountWebhook", func(t *testing.T) {
		if createdAccountWebhookID == "" {
			t.Skip("no account webhook ID from create test")
		}

		resp, err := doRequest("GET", "/v1/webhooks/"+createdAccountWebhookID, nil)
		if err != nil {
			reporter.Record("Webhooks", "GetAccountWebhook", "HTTP", false, err.Error())
			t.Fatalf("GET /v1/webhooks/%s: %v", createdAccountWebhookID, err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["webhook_id"] != createdAccountWebhookID {
			t.Errorf("expected webhook_id %s, got %v", createdAccountWebhookID, result["webhook_id"])
		}
		if result["url"] != "http://example.com/account-hook" {
			t.Errorf("expected url http://example.com/account-hook, got %v", result["url"])
		}
		if result["description"] != "test account webhook" {
			t.Errorf("expected description 'test account webhook', got %v", result["description"])
		}

		eventTypes, ok := result["event_types"].([]interface{})
		if !ok {
			t.Errorf("expected event_types array, got %v", result["event_types"])
		} else if len(eventTypes) != 2 {
			t.Errorf("expected 2 event types, got %d", len(eventTypes))
		}

		if result["created_at"] == nil || result["created_at"] == "" {
			t.Errorf("expected non-empty created_at field")
		}

		reporter.Record("Webhooks", "GetAccountWebhook", "HTTP", !t.Failed(), "")
	})

	t.Run("HTTP_UpdateAccountWebhook", func(t *testing.T) {
		if createdAccountWebhookID == "" {
			t.Skip("no account webhook ID from create test")
		}

		vals := url.Values{}
		vals.Set("url", "http://example.com/updated-account-hook")
		vals.Add("event_types", "clicked")
		vals.Set("description", "updated account webhook")

		resp, err := doRepeatedFormRequest("PUT", "/v1/webhooks/"+createdAccountWebhookID, vals)
		if err != nil {
			reporter.Record("Webhooks", "UpdateAccountWebhook", "HTTP", false, err.Error())
			t.Fatalf("PUT /v1/webhooks/%s: %v", createdAccountWebhookID, err)
		}
		assertStatus(t, resp, http.StatusNoContent)
		resp.Body.Close()

		// Verify the update by fetching
		getResp, err := doRequest("GET", "/v1/webhooks/"+createdAccountWebhookID, nil)
		if err != nil {
			t.Fatalf("verify update: %v", err)
		}
		assertStatus(t, getResp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, getResp, &result)

		if result["url"] != "http://example.com/updated-account-hook" {
			t.Errorf("expected updated url, got %v", result["url"])
		}
		if result["description"] != "updated account webhook" {
			t.Errorf("expected updated description, got %v", result["description"])
		}

		reporter.Record("Webhooks", "UpdateAccountWebhook", "HTTP", !t.Failed(), "")
	})

	t.Run("HTTP_DeleteAccountWebhook", func(t *testing.T) {
		if createdAccountWebhookID == "" {
			t.Skip("no account webhook ID from create test")
		}

		resp, err := doRequest("DELETE", "/v1/webhooks/"+createdAccountWebhookID, nil)
		if err != nil {
			reporter.Record("Webhooks", "DeleteAccountWebhook", "HTTP", false, err.Error())
			t.Fatalf("DELETE /v1/webhooks/%s: %v", createdAccountWebhookID, err)
		}
		assertStatus(t, resp, http.StatusNoContent)
		resp.Body.Close()

		// Verify it's gone
		getResp, err := doRequest("GET", "/v1/webhooks/"+createdAccountWebhookID, nil)
		if err != nil {
			t.Fatalf("verify delete: %v", err)
		}
		if getResp.StatusCode == http.StatusOK {
			t.Errorf("expected non-200 status for deleted webhook, got 200")
		}
		getResp.Body.Close()

		reporter.Record("Webhooks", "DeleteAccountWebhook", "HTTP", !t.Failed(), "")
	})

	// --- Webhook Signing Key (v5) ---

	var originalSigningKey string

	t.Run("HTTP_GetSigningKey", func(t *testing.T) {
		resp, err := doRequest("GET", "/v5/accounts/http_signing_key", nil)
		if err != nil {
			reporter.Record("Webhooks", "GetSigningKey", "HTTP", false, err.Error())
			t.Fatalf("GET signing key: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "success" {
			t.Errorf("expected message 'success', got %v", result["message"])
		}

		key, ok := result["http_signing_key"].(string)
		if !ok || key == "" {
			t.Fatalf("expected non-empty http_signing_key, got %v", result["http_signing_key"])
		}
		originalSigningKey = key

		reporter.Record("Webhooks", "GetSigningKey", "HTTP", !t.Failed(), "")
	})

	t.Run("HTTP_RotateSigningKey", func(t *testing.T) {
		resp, err := doRequest("POST", "/v5/accounts/http_signing_key", nil)
		if err != nil {
			reporter.Record("Webhooks", "RotateSigningKey", "HTTP", false, err.Error())
			t.Fatalf("POST signing key: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "success" {
			t.Errorf("expected message 'success', got %v", result["message"])
		}

		newKey, ok := result["http_signing_key"].(string)
		if !ok || newKey == "" {
			t.Fatalf("expected non-empty http_signing_key, got %v", result["http_signing_key"])
		}

		if originalSigningKey != "" && newKey == originalSigningKey {
			t.Errorf("expected rotated key to differ from original key %q", originalSigningKey)
		}

		// Verify GET returns the new key
		getResp, err := doRequest("GET", "/v5/accounts/http_signing_key", nil)
		if err != nil {
			t.Fatalf("verify rotation: %v", err)
		}
		assertStatus(t, getResp, http.StatusOK)

		var getResult map[string]interface{}
		readJSON(t, getResp, &getResult)

		gotKey, ok := getResult["http_signing_key"].(string)
		if !ok {
			t.Fatalf("expected http_signing_key in GET response")
		}
		if gotKey != newKey {
			t.Errorf("GET after rotate returned %q, expected %q", gotKey, newKey)
		}

		reporter.Record("Webhooks", "RotateSigningKey", "HTTP", !t.Failed(), "")
	})
}

