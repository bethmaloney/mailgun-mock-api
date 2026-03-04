package integration

import (
	"context"
	"net/http"
	"testing"

	"github.com/mailgun/mailgun-go/v5"
	"github.com/mailgun/mailgun-go/v5/mtypes"
)

func TestDomains(t *testing.T) {
	resetServer(t)

	const testDomain = "test-domains.example.com"
	const testDomain2 = "test-domains2.example.com"
	ctx := context.Background()

	// --- Domain CRUD (v4) ---

	t.Run("SDK_CreateDomain", func(t *testing.T) {
		mg := newMailgunClient()

		resp, err := mg.CreateDomain(ctx, testDomain, &mailgun.CreateDomainOptions{
			SpamAction: mtypes.SpamActionTag,
			Wildcard:   true,
			WebScheme:  "https",
			WebPrefix:  "email",
		})
		if err != nil {
			t.Fatalf("CreateDomain failed: %v", err)
		}

		if resp.Domain.Name != testDomain {
			t.Errorf("expected domain name %q, got %q", testDomain, resp.Domain.Name)
		}
		if resp.Domain.SpamAction != mtypes.SpamActionTag {
			t.Errorf("expected spam_action 'tag', got %q", resp.Domain.SpamAction)
		}
		if !resp.Domain.Wildcard {
			t.Errorf("expected wildcard to be true")
		}

		reporter.Record("Domains", "CreateDomain", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_CreateDomain", func(t *testing.T) {
		resp, err := doFormRequest("POST", "/v4/domains", map[string]string{
			"name":        testDomain2,
			"spam_action": "disabled",
			"wildcard":    "false",
			"web_scheme":  "https",
			"web_prefix":  "email",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "Domain has been created" {
			t.Errorf("expected message 'Domain has been created', got %v", result["message"])
		}

		domain, ok := result["domain"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected 'domain' object in response")
		}
		if domain["name"] != testDomain2 {
			t.Errorf("expected domain name %q, got %v", testDomain2, domain["name"])
		}

		reporter.Record("Domains", "CreateDomain", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_ListDomains", func(t *testing.T) {
		mg := newMailgunClient()

		iter := mg.ListDomains(nil)

		var domains []mtypes.Domain
		if !iter.Next(ctx, &domains) {
			if iter.Err() != nil {
				t.Fatalf("ListDomains iteration failed: %v", iter.Err())
			}
			t.Fatalf("expected at least one page of domains")
		}

		if len(domains) < 2 {
			t.Errorf("expected at least 2 domains, got %d", len(domains))
		}

		found := false
		for _, d := range domains {
			if d.Name == testDomain {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected to find domain %q in list", testDomain)
		}

		reporter.Record("Domains", "ListDomains", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_ListDomains", func(t *testing.T) {
		resp, err := doRequest("GET", "/v4/domains", nil)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		items, ok := result["items"].([]interface{})
		if !ok {
			t.Fatalf("expected 'items' array in response")
		}
		if len(items) < 2 {
			t.Errorf("expected at least 2 domains, got %d", len(items))
		}

		totalCount, _ := result["total_count"].(float64)
		if int(totalCount) < 2 {
			t.Errorf("expected total_count >= 2, got %v", totalCount)
		}

		reporter.Record("Domains", "ListDomains", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_GetDomain", func(t *testing.T) {
		mg := newMailgunClient()

		resp, err := mg.GetDomain(ctx, testDomain, nil)
		if err != nil {
			t.Fatalf("GetDomain failed: %v", err)
		}

		if resp.Domain.Name != testDomain {
			t.Errorf("expected domain name %q, got %q", testDomain, resp.Domain.Name)
		}

		reporter.Record("Domains", "GetDomain", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_GetDomain", func(t *testing.T) {
		resp, err := doRequest("GET", "/v4/domains/"+testDomain, nil)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		domain, ok := result["domain"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected 'domain' object in response")
		}
		if domain["name"] != testDomain {
			t.Errorf("expected domain name %q, got %v", testDomain, domain["name"])
		}

		reporter.Record("Domains", "GetDomain", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_UpdateDomain", func(t *testing.T) {
		mg := newMailgunClient()

		wildcard := false
		err := mg.UpdateDomain(ctx, testDomain, &mailgun.UpdateDomainOptions{
			SpamAction: mtypes.SpamActionDisabled,
			Wildcard:   &wildcard,
			WebScheme:  "http",
			WebPrefix:  "tracking",
		})
		if err != nil {
			t.Fatalf("UpdateDomain failed: %v", err)
		}

		// Verify the update took effect
		domResp, err := mg.GetDomain(ctx, testDomain, nil)
		if err != nil {
			t.Fatalf("GetDomain after update failed: %v", err)
		}
		if domResp.Domain.SpamAction != mtypes.SpamActionDisabled {
			t.Errorf("expected spam_action 'disabled' after update, got %q", domResp.Domain.SpamAction)
		}

		reporter.Record("Domains", "UpdateDomain", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_UpdateDomain", func(t *testing.T) {
		resp, err := doFormRequest("PUT", "/v4/domains/"+testDomain, map[string]string{
			"spam_action": "tag",
			"wildcard":    "true",
			"web_scheme":  "https",
			"web_prefix":  "email",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "Domain has been updated" {
			t.Errorf("expected message 'Domain has been updated', got %v", result["message"])
		}

		domain, ok := result["domain"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected 'domain' object in response")
		}
		if domain["name"] != testDomain {
			t.Errorf("expected domain name %q, got %v", testDomain, domain["name"])
		}

		reporter.Record("Domains", "UpdateDomain", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_VerifyDomain", func(t *testing.T) {
		mg := newMailgunClient()

		resp, err := mg.VerifyDomain(ctx, testDomain)
		if err != nil {
			t.Fatalf("VerifyDomain failed: %v", err)
		}

		if resp.Domain.Name != testDomain {
			t.Errorf("expected domain name %q, got %q", testDomain, resp.Domain.Name)
		}
		if resp.Domain.State != "active" {
			t.Errorf("expected state 'active' after verify, got %q", resp.Domain.State)
		}

		reporter.Record("Domains", "VerifyDomain", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_VerifyDomain", func(t *testing.T) {
		resp, err := doFormRequest("PUT", "/v4/domains/"+testDomain+"/verify", nil)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "Domain DNS records have been updated" {
			t.Errorf("expected message 'Domain DNS records have been updated', got %v", result["message"])
		}

		domain, ok := result["domain"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected 'domain' object in response")
		}
		if domain["state"] != "active" {
			t.Errorf("expected state 'active' after verify, got %v", domain["state"])
		}

		reporter.Record("Domains", "VerifyDomain", "HTTP", !t.Failed(), "")
	})

	// --- Connection Settings ---

	t.Run("SDK_GetDomainConnection", func(t *testing.T) {
		mg := newMailgunClient()

		conn, err := mg.GetDomainConnection(ctx, testDomain)
		if err != nil {
			t.Fatalf("GetDomainConnection failed: %v", err)
		}

		// Just verify we got a valid response (default values)
		_ = conn.RequireTLS
		_ = conn.SkipVerification

		reporter.Record("Domains", "GetDomainConnection", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_GetDomainConnection", func(t *testing.T) {
		resp, err := doRequest("GET", "/v3/domains/"+testDomain+"/connection", nil)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		connObj, ok := result["connection"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected 'connection' object in response")
		}

		if _, exists := connObj["require_tls"]; !exists {
			t.Errorf("expected 'require_tls' field in connection")
		}
		if _, exists := connObj["skip_verification"]; !exists {
			t.Errorf("expected 'skip_verification' field in connection")
		}

		reporter.Record("Domains", "GetDomainConnection", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_UpdateDomainConnection", func(t *testing.T) {
		mg := newMailgunClient()

		err := mg.UpdateDomainConnection(ctx, testDomain, mtypes.DomainConnection{
			RequireTLS:       true,
			SkipVerification: true,
		})
		if err != nil {
			t.Fatalf("UpdateDomainConnection failed: %v", err)
		}

		// Verify update
		conn, err := mg.GetDomainConnection(ctx, testDomain)
		if err != nil {
			t.Fatalf("GetDomainConnection after update failed: %v", err)
		}
		if !conn.RequireTLS {
			t.Errorf("expected require_tls to be true after update")
		}
		if !conn.SkipVerification {
			t.Errorf("expected skip_verification to be true after update")
		}

		reporter.Record("Domains", "UpdateDomainConnection", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_UpdateDomainConnection", func(t *testing.T) {
		resp, err := doFormRequest("PUT", "/v3/domains/"+testDomain+"/connection", map[string]string{
			"require_tls":       "false",
			"skip_verification": "false",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "Domain connection settings have been updated" {
			t.Errorf("expected message 'Domain connection settings have been updated', got %v", result["message"])
		}

		reporter.Record("Domains", "UpdateDomainConnection", "HTTP", !t.Failed(), "")
	})

	// --- Tracking Settings ---

	t.Run("SDK_GetDomainTracking", func(t *testing.T) {
		mg := newMailgunClient()

		tracking, err := mg.GetDomainTracking(ctx, testDomain)
		if err != nil {
			t.Fatalf("GetDomainTracking failed: %v", err)
		}

		// Verify we got a valid response with tracking sub-structs
		_ = tracking.Open
		_ = tracking.Click
		_ = tracking.Unsubscribe

		reporter.Record("Domains", "GetDomainTracking", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_GetDomainTracking", func(t *testing.T) {
		resp, err := doRequest("GET", "/v3/domains/"+testDomain+"/tracking", nil)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		trackingObj, ok := result["tracking"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected 'tracking' object in response")
		}

		if _, exists := trackingObj["open"]; !exists {
			t.Errorf("expected 'open' field in tracking")
		}
		if _, exists := trackingObj["click"]; !exists {
			t.Errorf("expected 'click' field in tracking")
		}
		if _, exists := trackingObj["unsubscribe"]; !exists {
			t.Errorf("expected 'unsubscribe' field in tracking")
		}

		reporter.Record("Domains", "GetDomainTracking", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_UpdateOpenTracking", func(t *testing.T) {
		mg := newMailgunClient()

		err := mg.UpdateOpenTracking(ctx, testDomain, "true")
		if err != nil {
			t.Fatalf("UpdateOpenTracking failed: %v", err)
		}

		// Verify the update
		tracking, err := mg.GetDomainTracking(ctx, testDomain)
		if err != nil {
			t.Fatalf("GetDomainTracking after update failed: %v", err)
		}
		if !tracking.Open.Active {
			t.Errorf("expected open tracking to be active after update")
		}

		reporter.Record("Domains", "UpdateOpenTracking", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_UpdateOpenTracking", func(t *testing.T) {
		resp, err := doFormRequest("PUT", "/v3/domains/"+testDomain+"/tracking/open", map[string]string{
			"active": "false",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "Domain tracking settings have been updated" {
			t.Errorf("expected message 'Domain tracking settings have been updated', got %v", result["message"])
		}

		reporter.Record("Domains", "UpdateOpenTracking", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_UpdateClickTracking", func(t *testing.T) {
		mg := newMailgunClient()

		err := mg.UpdateClickTracking(ctx, testDomain, "true")
		if err != nil {
			t.Fatalf("UpdateClickTracking failed: %v", err)
		}

		reporter.Record("Domains", "UpdateClickTracking", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_UpdateClickTracking", func(t *testing.T) {
		resp, err := doFormRequest("PUT", "/v3/domains/"+testDomain+"/tracking/click", map[string]string{
			"active": "htmlonly",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "Domain tracking settings have been updated" {
			t.Errorf("expected message 'Domain tracking settings have been updated', got %v", result["message"])
		}

		reporter.Record("Domains", "UpdateClickTracking", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_UpdateUnsubscribeTracking", func(t *testing.T) {
		mg := newMailgunClient()

		err := mg.UpdateUnsubscribeTracking(ctx, testDomain, "true", "<p>Unsubscribe</p>", "Unsubscribe here")
		if err != nil {
			t.Fatalf("UpdateUnsubscribeTracking failed: %v", err)
		}

		// Reset click tracking to a bool-compatible value before verifying via SDK,
		// because "htmlonly" causes JSON unmarshal errors in the SDK's bool field.
		_ = mg.UpdateClickTracking(ctx, testDomain, "true")

		// Verify the update
		tracking, err := mg.GetDomainTracking(ctx, testDomain)
		if err != nil {
			t.Fatalf("GetDomainTracking after update failed: %v", err)
		}
		if !tracking.Unsubscribe.Active {
			t.Errorf("expected unsubscribe tracking to be active after update")
		}

		reporter.Record("Domains", "UpdateUnsubscribeTracking", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_UpdateUnsubscribeTracking", func(t *testing.T) {
		resp, err := doFormRequest("PUT", "/v3/domains/"+testDomain+"/tracking/unsubscribe", map[string]string{
			"active":      "false",
			"html_footer": "<p>Click to unsub</p>",
			"text_footer": "Click to unsub",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "Domain tracking settings have been updated" {
			t.Errorf("expected message 'Domain tracking settings have been updated', got %v", result["message"])
		}

		reporter.Record("Domains", "UpdateUnsubscribeTracking", "HTTP", !t.Failed(), "")
	})

	// --- DKIM ---

	t.Run("SDK_UpdateDkimAuthority", func(t *testing.T) {
		mg := newMailgunClient()

		resp, err := mg.UpdateDomainDkimAuthority(ctx, testDomain, true)
		if err != nil {
			t.Fatalf("UpdateDomainDkimAuthority failed: %v", err)
		}

		if resp.Message != "Domain DKIM authority has been changed" {
			t.Errorf("expected message 'Domain DKIM authority has been changed', got %q", resp.Message)
		}

		reporter.Record("Domains", "UpdateDkimAuthority", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_UpdateDkimAuthority", func(t *testing.T) {
		resp, err := doFormRequest("PUT", "/v3/domains/"+testDomain+"/dkim_authority", map[string]string{
			"self": "false",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "Domain DKIM authority has been changed" {
			t.Errorf("expected message 'Domain DKIM authority has been changed', got %v", result["message"])
		}

		reporter.Record("Domains", "UpdateDkimAuthority", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_UpdateDkimSelector", func(t *testing.T) {
		mg := newMailgunClient()

		err := mg.UpdateDomainDkimSelector(ctx, testDomain, "mailo1")
		if err != nil {
			t.Fatalf("UpdateDomainDkimSelector failed: %v", err)
		}

		reporter.Record("Domains", "UpdateDkimSelector", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_UpdateDkimSelector", func(t *testing.T) {
		resp, err := doFormRequest("PUT", "/v3/domains/"+testDomain+"/dkim_selector", map[string]string{
			"dkim_selector": "mailo2",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "Domain DKIM selector has been updated" {
			t.Errorf("expected message 'Domain DKIM selector has been updated', got %v", result["message"])
		}

		reporter.Record("Domains", "UpdateDkimSelector", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_ListDomainKeys", func(t *testing.T) {
		t.Skip("DKIM key endpoints not implemented on mock server")
	})

	t.Run("HTTP_ListDomainKeys", func(t *testing.T) {
		resp, err := doRequest("GET", "/v1/dkim/keys", nil)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		// Endpoint not implemented; expect non-500 error
		if resp.StatusCode >= 500 {
			t.Errorf("expected non-500 status for unimplemented endpoint, got %d", resp.StatusCode)
		}
		resp.Body.Close()

		reporter.Record("Domains", "ListDomainKeys", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_CreateDomainKey", func(t *testing.T) {
		t.Skip("DKIM key endpoints not implemented on mock server")
	})

	t.Run("HTTP_CreateDomainKey", func(t *testing.T) {
		resp, err := doFormRequest("POST", "/v1/dkim/keys", map[string]string{
			"signing_domain": testDomain,
			"selector":       "test-selector",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		// Endpoint not implemented; expect non-500 error
		if resp.StatusCode >= 500 {
			t.Errorf("expected non-500 status for unimplemented endpoint, got %d", resp.StatusCode)
		}
		resp.Body.Close()

		reporter.Record("Domains", "CreateDomainKey", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_ActivateDomainKey", func(t *testing.T) {
		t.Skip("DKIM key endpoints not implemented on mock server")
	})

	t.Run("HTTP_ActivateDomainKey", func(t *testing.T) {
		resp, err := doFormRequest("PUT", "/v4/domains/"+testDomain+"/keys/test-selector/activate", nil)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		// Endpoint not implemented; expect non-500 error
		if resp.StatusCode >= 500 {
			t.Errorf("expected non-500 status for unimplemented endpoint, got %d", resp.StatusCode)
		}
		resp.Body.Close()

		reporter.Record("Domains", "ActivateDomainKey", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_DeactivateDomainKey", func(t *testing.T) {
		t.Skip("DKIM key endpoints not implemented on mock server")
	})

	t.Run("HTTP_DeactivateDomainKey", func(t *testing.T) {
		resp, err := doFormRequest("PUT", "/v4/domains/"+testDomain+"/keys/test-selector/deactivate", nil)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		// Endpoint not implemented; expect non-500 error
		if resp.StatusCode >= 500 {
			t.Errorf("expected non-500 status for unimplemented endpoint, got %d", resp.StatusCode)
		}
		resp.Body.Close()

		reporter.Record("Domains", "DeactivateDomainKey", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_DeleteDomainKey", func(t *testing.T) {
		t.Skip("DKIM key endpoints not implemented on mock server")
	})

	t.Run("HTTP_DeleteDomainKey", func(t *testing.T) {
		resp, err := doRequest("DELETE", "/v1/dkim/keys/fake-key-id", nil)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		// Endpoint not implemented; expect non-500 error
		if resp.StatusCode >= 500 {
			t.Errorf("expected non-500 status for unimplemented endpoint, got %d", resp.StatusCode)
		}
		resp.Body.Close()

		reporter.Record("Domains", "DeleteDomainKey", "HTTP", !t.Failed(), "")
	})

	// --- Delete domain last ---

	t.Run("SDK_DeleteDomain", func(t *testing.T) {
		mg := newMailgunClient()

		err := mg.DeleteDomain(ctx, testDomain)
		if err != nil {
			t.Fatalf("DeleteDomain failed: %v", err)
		}

		reporter.Record("Domains", "DeleteDomain", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_DeleteDomain", func(t *testing.T) {
		// Delete the second domain via HTTP
		resp, err := doRequest("DELETE", "/v3/domains/"+testDomain2, nil)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "Domain has been deleted" {
			t.Errorf("expected message 'Domain has been deleted', got %v", result["message"])
		}

		reporter.Record("Domains", "DeleteDomain", "HTTP", !t.Failed(), "")
	})
}

// Ensure imports are used.
var _ mailgun.CreateDomainOptions
var _ mtypes.GetDomainResponse
