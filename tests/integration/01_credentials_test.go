package integration

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/mailgun/mailgun-go/v5"
	"github.com/mailgun/mailgun-go/v5/mtypes"
)

func TestCredentials(t *testing.T) {
	resetServer(t)

	const testDomain = "test.example.com"

	// --- API Keys (v1) ---

	// First, create a key so that list/delete/regenerate tests have something to work with.
	var createdKeyID string

	t.Run("HTTP_CreateAPIKey", func(t *testing.T) {
		resp, err := doFormRequest("POST", "/v1/keys", map[string]string{
			"role":        "admin",
			"description": "integration test key",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "great success" {
			t.Errorf("expected message 'great success', got %v", result["message"])
		}

		keyObj, ok := result["key"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected 'key' object in response, got %T", result["key"])
		}

		secret, _ := keyObj["secret"].(string)
		if !strings.HasPrefix(secret, "key-") {
			t.Errorf("expected secret starting with 'key-', got %q", secret)
		}

		id, _ := keyObj["id"].(string)
		if id == "" {
			t.Errorf("expected non-empty id")
		}
		createdKeyID = id

		role, _ := keyObj["role"].(string)
		if role != "admin" {
			t.Errorf("expected role 'admin', got %q", role)
		}

		reporter.Record("Credentials", "CreateAPIKey", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_CreateAPIKey", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		key, err := mg.CreateAPIKey(ctx, "sending", &mailgun.CreateAPIKeyOptions{
			Description: "sdk test key",
		})
		if err != nil {
			t.Fatalf("CreateAPIKey failed: %v", err)
		}

		if key.Role != "sending" {
			t.Errorf("expected role 'sending', got %q", key.Role)
		}
		if !strings.HasPrefix(key.Secret, "key-") {
			t.Errorf("expected secret starting with 'key-', got %q", key.Secret)
		}
		if key.ID == "" {
			t.Errorf("expected non-empty ID")
		}

		reporter.Record("Credentials", "CreateAPIKey", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_ListAPIKeys", func(t *testing.T) {
		resp, err := doRequest("GET", "/v1/keys", nil)
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
		if len(items) < 1 {
			t.Errorf("expected at least 1 API key, got %d", len(items))
		}

		reporter.Record("Credentials", "ListAPIKeys", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_ListAPIKeys", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		keys, err := mg.ListAPIKeys(ctx, nil)
		if err != nil {
			t.Fatalf("ListAPIKeys failed: %v", err)
		}

		if len(keys) < 1 {
			t.Errorf("expected at least 1 API key, got %d", len(keys))
		}

		reporter.Record("Credentials", "ListAPIKeys", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_DeleteAPIKey", func(t *testing.T) {
		if createdKeyID == "" {
			t.Skip("no key ID available from create test")
		}

		resp, err := doRequest("DELETE", "/v1/keys/"+createdKeyID, nil)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "key deleted" {
			t.Errorf("expected message 'key deleted', got %v", result["message"])
		}

		reporter.Record("Credentials", "DeleteAPIKey", "HTTP", !t.Failed(), "")
	})

	t.Run("HTTP_DeleteAPIKey_NotFound", func(t *testing.T) {
		resp, err := doRequest("DELETE", "/v1/keys/nonexistent-id", nil)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()

		reporter.Record("Credentials", "DeleteAPIKey_NotFound", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_DeleteAPIKey", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		// Create a key to delete via SDK
		key, err := mg.CreateAPIKey(ctx, "basic", nil)
		if err != nil {
			t.Fatalf("CreateAPIKey failed: %v", err)
		}

		err = mg.DeleteAPIKey(ctx, key.ID)
		if err != nil {
			t.Fatalf("DeleteAPIKey failed: %v", err)
		}

		reporter.Record("Credentials", "DeleteAPIKey", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_RegeneratePublicKey", func(t *testing.T) {
		// First get the current public key.
		getResp, err := doRequest("GET", "/v1/keys/public", nil)
		if err != nil {
			t.Fatalf("GET request failed: %v", err)
		}
		assertStatus(t, getResp, http.StatusOK)
		var getResult map[string]interface{}
		readJSON(t, getResp, &getResult)
		oldKey, _ := getResult["key"].(string)

		// POST /v1/keys/public to regenerate.
		resp, err := doRequest("POST", "/v1/keys/public", nil)
		if err != nil {
			t.Fatalf("POST request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		newKey, _ := result["key"].(string)
		if !strings.HasPrefix(newKey, "pubkey-") {
			t.Errorf("expected public key starting with 'pubkey-', got %q", newKey)
		}
		if newKey == oldKey {
			t.Errorf("expected regenerated key to differ from old key")
		}

		reporter.Record("Credentials", "RegeneratePublicKey", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_RegeneratePublicKey", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		resp, err := mg.RegeneratePublicAPIKey(ctx)
		if err != nil {
			t.Fatalf("RegeneratePublicAPIKey failed: %v", err)
		}

		if resp.Key == "" {
			t.Errorf("expected non-empty public key")
		}

		reporter.Record("Credentials", "RegeneratePublicKey", "SDK", !t.Failed(), "")
	})

	// Also test regenerating a specific key's secret via POST /v1/keys/{id}/regenerate
	t.Run("HTTP_RegenerateKeySecret", func(t *testing.T) {
		// Create a key first
		createResp, err := doFormRequest("POST", "/v1/keys", map[string]string{
			"role": "developer",
		})
		if err != nil {
			t.Fatalf("create request failed: %v", err)
		}
		assertStatus(t, createResp, http.StatusOK)

		var createResult map[string]interface{}
		readJSON(t, createResp, &createResult)

		keyObj := createResult["key"].(map[string]interface{})
		keyID := keyObj["id"].(string)
		oldSecret := keyObj["secret"].(string)

		// Regenerate
		resp, err := doRequest("POST", "/v1/keys/"+keyID+"/regenerate", nil)
		if err != nil {
			t.Fatalf("regenerate request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "key regenerated" {
			t.Errorf("expected message 'key regenerated', got %v", result["message"])
		}

		regenKey := result["key"].(map[string]interface{})
		newSecret := regenKey["secret"].(string)
		if newSecret == oldSecret {
			t.Errorf("expected new secret to differ from old secret")
		}
		if !strings.HasPrefix(newSecret, "key-") {
			t.Errorf("expected new secret starting with 'key-', got %q", newSecret)
		}

		reporter.Record("Credentials", "RegenerateKeySecret", "HTTP", !t.Failed(), "")
	})

	// --- SMTP Credentials (v3) ---
	// Create domain first (required for credential endpoints)
	t.Run("setup_domain", func(t *testing.T) {
		resp, err := doFormRequest("POST", "/v4/domains", map[string]string{
			"name": testDomain,
		})
		if err != nil {
			t.Fatalf("create domain request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})

	t.Run("HTTP_CreateCredential", func(t *testing.T) {
		resp, err := doFormRequest("POST", "/v3/domains/"+testDomain+"/credentials", map[string]string{
			"login":    "alice",
			"password": "secret12345",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "Created 1 credentials pair(s)" {
			t.Errorf("expected 'Created 1 credentials pair(s)', got %v", result["message"])
		}

		reporter.Record("Credentials", "CreateCredential", "HTTP", !t.Failed(), "")
	})

	t.Run("HTTP_CreateCredential_ShortPassword", func(t *testing.T) {
		resp, err := doFormRequest("POST", "/v3/domains/"+testDomain+"/credentials", map[string]string{
			"login":    "bob",
			"password": "ab",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		// Password too short (< 5 chars) should fail
		if resp.StatusCode == http.StatusOK {
			t.Errorf("expected error for short password, got 200")
		}
		resp.Body.Close()

		reporter.Record("Credentials", "CreateCredential_ShortPassword", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_CreateCredential", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		err := mg.CreateCredential(ctx, testDomain, "bob", "password12345")
		if err != nil {
			t.Fatalf("CreateCredential failed: %v", err)
		}

		reporter.Record("Credentials", "CreateCredential", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_ListCredentials", func(t *testing.T) {
		resp, err := doRequest("GET", "/v3/domains/"+testDomain+"/credentials", nil)
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
			t.Errorf("expected at least 2 credentials, got %d", len(items))
		}

		// Check that the items have the expected fields
		if len(items) > 0 {
			firstItem, ok := items[0].(map[string]interface{})
			if ok {
				if _, hasLogin := firstItem["login"]; !hasLogin {
					t.Errorf("expected 'login' field in credential item")
				}
			}
		}

		reporter.Record("Credentials", "ListCredentials", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_ListCredentials", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		iter := mg.ListCredentials(testDomain, nil)

		var creds []mtypes.Credential
		if !iter.Next(ctx, &creds) {
			if iter.Err() != nil {
				t.Fatalf("ListCredentials iteration failed: %v", iter.Err())
			}
			t.Fatalf("expected at least one page of credentials")
		}

		if len(creds) < 2 {
			t.Errorf("expected at least 2 credentials, got %d", len(creds))
		}

		reporter.Record("Credentials", "ListCredentials", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_ChangeCredentialPassword", func(t *testing.T) {
		resp, err := doFormRequest("PUT", "/v3/domains/"+testDomain+"/credentials/alice", map[string]string{
			"password": "newpassword123",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "Password changed" {
			t.Errorf("expected 'Password changed', got %v", result["message"])
		}

		reporter.Record("Credentials", "ChangeCredentialPassword", "HTTP", !t.Failed(), "")
	})

	t.Run("HTTP_ChangeCredentialPassword_ShortPassword", func(t *testing.T) {
		resp, err := doFormRequest("PUT", "/v3/domains/"+testDomain+"/credentials/alice", map[string]string{
			"password": "ab",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		// Password too short should fail
		if resp.StatusCode == http.StatusOK {
			t.Errorf("expected error for short password, got 200")
		}
		resp.Body.Close()

		reporter.Record("Credentials", "ChangeCredentialPassword_ShortPassword", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_ChangeCredentialPassword", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		err := mg.ChangeCredentialPassword(ctx, testDomain, "bob", "updatedpass123")
		if err != nil {
			t.Fatalf("ChangeCredentialPassword failed: %v", err)
		}

		reporter.Record("Credentials", "ChangeCredentialPassword", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_DeleteCredential", func(t *testing.T) {
		resp, err := doRequest("DELETE", "/v3/domains/"+testDomain+"/credentials/alice", nil)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "Credentials have been deleted" {
			t.Errorf("expected 'Credentials have been deleted', got %v", result["message"])
		}

		spec, _ := result["spec"].(string)
		if spec != "alice@"+testDomain {
			t.Errorf("expected spec 'alice@%s', got %q", testDomain, spec)
		}

		reporter.Record("Credentials", "DeleteCredential", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_DeleteCredential", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		err := mg.DeleteCredential(ctx, testDomain, "bob")
		if err != nil {
			t.Fatalf("DeleteCredential failed: %v", err)
		}

		reporter.Record("Credentials", "DeleteCredential", "SDK", !t.Failed(), "")
	})

	// Verify credentials list is now empty after deletions
	t.Run("HTTP_ListCredentials_Empty", func(t *testing.T) {
		resp, err := doRequest("GET", "/v3/domains/"+testDomain+"/credentials", nil)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		items, ok := result["items"].([]interface{})
		if !ok {
			// items may be null/nil when empty
			totalCount, _ := result["total_count"].(float64)
			if int(totalCount) != 0 {
				t.Errorf("expected total_count 0, got %v", result["total_count"])
			}
		} else if len(items) != 0 {
			t.Errorf("expected 0 credentials after deletion, got %d", len(items))
		}

		reporter.Record("Credentials", "ListCredentials_Empty", "HTTP", !t.Failed(), "")
	})
}
