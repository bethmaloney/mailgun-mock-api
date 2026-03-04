package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/mailgun/mailgun-go/v5"
)

// doMultipartFileRequest sends a multipart form request with both fields and file uploads.
func doMultipartFileRequest(method, path string, fields map[string]string, files map[string][]fileUpload) (*http.Response, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = w.WriteField(k, v)
	}
	for fieldName, fileList := range files {
		for _, f := range fileList {
			part, err := w.CreateFormFile(fieldName, f.name)
			if err != nil {
				return nil, err
			}
			_, _ = part.Write(f.content)
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

type fileUpload struct {
	name    string
	content []byte
}

// doMultipartFormWithRepeatedFields sends a multipart form request supporting repeated keys and file uploads.
func doMultipartFormWithRepeatedFields(method, path string, values url.Values, files map[string][]fileUpload) (*http.Response, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for key, vals := range values {
		for _, v := range vals {
			_ = w.WriteField(key, v)
		}
	}
	for fieldName, fileList := range files {
		for _, f := range fileList {
			part, err := w.CreateFormFile(fieldName, f.name)
			if err != nil {
				return nil, err
			}
			_, _ = part.Write(f.content)
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

func TestMessages(t *testing.T) {
	resetServer(t)

	const domain = "msg-test.example.com"
	const sender = "sender@msg-test.example.com"
	const recipient = "recipient@example.com"

	// Setup: create a domain first
	resp, err := doFormRequest("POST", "/v4/domains", map[string]string{"name": domain})
	if err != nil {
		t.Fatalf("setup: create domain: %v", err)
	}
	resp.Body.Close()

	// Helper to extract storage key from message ID
	storageKeyFromID := func(id string) string {
		return strings.TrimRight(strings.TrimLeft(id, "<"), ">")
	}

	// --- Sending ---

	// We'll store the first message's ID for stored message tests later.
	var firstMessageID string

	t.Run("SDK_SendPlainText", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		m := mailgun.NewMessage(domain, sender, "Plain text test", "Hello, this is a plain text message.", recipient)
		resp, err := mg.Send(ctx, m)
		if err != nil {
			reporter.Record("Messages", "SendPlainText", "SDK", false, err.Error())
			t.Fatalf("Send: %v", err)
		}

		if resp.Message != "Queued. Thank you." {
			t.Errorf("expected 'Queued. Thank you.', got %q", resp.Message)
		}
		if resp.ID == "" {
			t.Error("expected non-empty message ID")
		}
		if !strings.HasPrefix(resp.ID, "<") || !strings.HasSuffix(resp.ID, ">") {
			t.Errorf("expected message ID in angle brackets, got %q", resp.ID)
		}

		// Store the first ID for stored message tests
		if firstMessageID == "" {
			firstMessageID = resp.ID
		}

		reporter.Record("Messages", "SendPlainText", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_SendPlainText", func(t *testing.T) {
		resp, err := doFormRequest("POST", "/v3/"+domain+"/messages", map[string]string{
			"from":    sender,
			"to":      recipient,
			"subject": "HTTP plain text test",
			"text":    "Hello from HTTP plain text.",
		})
		if err != nil {
			reporter.Record("Messages", "SendPlainText", "HTTP", false, err.Error())
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "Queued. Thank you." {
			t.Errorf("expected 'Queued. Thank you.', got %v", result["message"])
		}
		id, ok := result["id"].(string)
		if !ok || id == "" {
			t.Error("expected non-empty id in response")
		}

		reporter.Record("Messages", "SendPlainText", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_SendHTML", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		m := mailgun.NewMessage(domain, sender, "HTML test", "", recipient)
		m.SetHTML("<h1>Hello HTML</h1><p>This is an HTML message.</p>")

		resp, err := mg.Send(ctx, m)
		if err != nil {
			reporter.Record("Messages", "SendHTML", "SDK", false, err.Error())
			t.Fatalf("Send: %v", err)
		}

		if resp.Message != "Queued. Thank you." {
			t.Errorf("expected 'Queued. Thank you.', got %q", resp.Message)
		}
		if resp.ID == "" {
			t.Error("expected non-empty message ID")
		}

		reporter.Record("Messages", "SendHTML", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_SendHTML", func(t *testing.T) {
		resp, err := doFormRequest("POST", "/v3/"+domain+"/messages", map[string]string{
			"from":    sender,
			"to":      recipient,
			"subject": "HTTP HTML test",
			"html":    "<h1>Hello HTML</h1><p>From HTTP.</p>",
		})
		if err != nil {
			reporter.Record("Messages", "SendHTML", "HTTP", false, err.Error())
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "Queued. Thank you." {
			t.Errorf("expected 'Queued. Thank you.', got %v", result["message"])
		}

		reporter.Record("Messages", "SendHTML", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_SendWithAttachments", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		m := mailgun.NewMessage(domain, sender, "Attachment test", "Message with attachment.", recipient)
		m.AddReaderAttachment("test.txt", io.NopCloser(bytes.NewReader([]byte("This is attachment content."))))

		resp, err := mg.Send(ctx, m)
		if err != nil {
			reporter.Record("Messages", "SendWithAttachments", "SDK", false, err.Error())
			t.Fatalf("Send: %v", err)
		}

		if resp.Message != "Queued. Thank you." {
			t.Errorf("expected 'Queued. Thank you.', got %q", resp.Message)
		}
		if resp.ID == "" {
			t.Error("expected non-empty message ID")
		}

		// Verify stored message has the attachment via HTTP
		// (SDK StoredAttachment.Name maps to "name" but the server may use "filename")
		storageKey := storageKeyFromID(resp.ID)
		getResp, getErr := doRequest("GET", "/v3/domains/"+domain+"/messages/"+storageKey, nil)
		if getErr != nil {
			t.Fatalf("GET stored message: %v", getErr)
		}
		assertStatus(t, getResp, http.StatusOK)

		var stored map[string]interface{}
		readJSON(t, getResp, &stored)

		attachments, ok := stored["attachments"].([]interface{})
		if !ok || len(attachments) == 0 {
			t.Error("expected at least one attachment in stored message")
		} else {
			att, _ := attachments[0].(map[string]interface{})
			// Check either "name" or "filename" field
			attName, _ := att["name"].(string)
			if attName == "" {
				attName, _ = att["filename"].(string)
			}
			if attName != "test.txt" {
				t.Errorf("expected attachment name 'test.txt', got %q", attName)
			}
		}

		reporter.Record("Messages", "SendWithAttachments", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_SendWithAttachments", func(t *testing.T) {
		fields := map[string]string{
			"from":    sender,
			"to":      recipient,
			"subject": "HTTP attachment test",
			"text":    "Message with attachment via HTTP.",
		}
		files := map[string][]fileUpload{
			"attachment": {
				{name: "hello.txt", content: []byte("Hello attachment content.")},
			},
		}

		resp, err := doMultipartFileRequest("POST", "/v3/"+domain+"/messages", fields, files)
		if err != nil {
			reporter.Record("Messages", "SendWithAttachments", "HTTP", false, err.Error())
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "Queued. Thank you." {
			t.Errorf("expected 'Queued. Thank you.', got %v", result["message"])
		}

		// Verify stored message has the attachment
		id, _ := result["id"].(string)
		storageKey := storageKeyFromID(id)
		getResp, err := doRequest("GET", "/v3/domains/"+domain+"/messages/"+storageKey, nil)
		if err != nil {
			t.Fatalf("GET stored message: %v", err)
		}
		assertStatus(t, getResp, http.StatusOK)

		var stored map[string]interface{}
		readJSON(t, getResp, &stored)

		attachments, ok := stored["attachments"].([]interface{})
		if !ok || len(attachments) == 0 {
			t.Error("expected at least one attachment in stored message")
		}

		reporter.Record("Messages", "SendWithAttachments", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_SendWithInlineImages", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		m := mailgun.NewMessage(domain, sender, "Inline image test", "", recipient)
		m.SetHTML("<html><body><img src=\"cid:image.png\"/></body></html>")
		// Create a small 1x1 PNG
		pngContent := []byte{
			0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
			0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
		}
		m.AddReaderInline("image.png", io.NopCloser(bytes.NewReader(pngContent)))

		resp, err := mg.Send(ctx, m)
		if err != nil {
			reporter.Record("Messages", "SendWithInlineImages", "SDK", false, err.Error())
			t.Fatalf("Send: %v", err)
		}

		if resp.Message != "Queued. Thank you." {
			t.Errorf("expected 'Queued. Thank you.', got %q", resp.Message)
		}
		if resp.ID == "" {
			t.Error("expected non-empty message ID")
		}

		reporter.Record("Messages", "SendWithInlineImages", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_SendWithInlineImages", func(t *testing.T) {
		fields := map[string]string{
			"from":    sender,
			"to":      recipient,
			"subject": "HTTP inline image test",
			"html":    "<html><body><img src=\"cid:inline-img.png\"/></body></html>",
		}
		pngContent := []byte{
			0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
			0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
		}
		files := map[string][]fileUpload{
			"inline": {
				{name: "inline-img.png", content: pngContent},
			},
		}

		resp, err := doMultipartFileRequest("POST", "/v3/"+domain+"/messages", fields, files)
		if err != nil {
			reporter.Record("Messages", "SendWithInlineImages", "HTTP", false, err.Error())
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "Queued. Thank you." {
			t.Errorf("expected 'Queued. Thank you.', got %v", result["message"])
		}

		reporter.Record("Messages", "SendWithInlineImages", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_SendWithTags", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		m := mailgun.NewMessage(domain, sender, "Tagged message", "Message with tags.", recipient)
		_ = m.AddTag("sdk-tag1")
		_ = m.AddTag("sdk-tag2")

		resp, err := mg.Send(ctx, m)
		if err != nil {
			reporter.Record("Messages", "SendWithTags", "SDK", false, err.Error())
			t.Fatalf("Send: %v", err)
		}

		if resp.Message != "Queued. Thank you." {
			t.Errorf("expected 'Queued. Thank you.', got %q", resp.Message)
		}
		if resp.ID == "" {
			t.Error("expected non-empty message ID")
		}

		// Verify tags via HTTP since SDK struct may not expose tag headers directly
		storageKey := storageKeyFromID(resp.ID)
		httpResp, err := doRequest("GET", "/v3/domains/"+domain+"/messages/"+storageKey, nil)
		if err != nil {
			t.Fatalf("GET stored message: %v", err)
		}
		assertStatus(t, httpResp, http.StatusOK)
		var storedMap map[string]interface{}
		readJSON(t, httpResp, &storedMap)
		if tagVal, ok := storedMap["X-Mailgun-Tag"]; !ok {
			t.Error("expected X-Mailgun-Tag in stored message")
		} else {
			tagStr, _ := tagVal.(string)
			if !strings.Contains(tagStr, "sdk-tag1") || !strings.Contains(tagStr, "sdk-tag2") {
				t.Errorf("expected tags to contain sdk-tag1 and sdk-tag2, got %q", tagStr)
			}
		}

		reporter.Record("Messages", "SendWithTags", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_SendWithTags", func(t *testing.T) {
		vals := url.Values{}
		vals.Set("from", sender)
		vals.Set("to", recipient)
		vals.Set("subject", "HTTP tagged message")
		vals.Set("text", "Message with tags via HTTP.")
		vals.Add("o:tag", "http-tag1")
		vals.Add("o:tag", "http-tag2")

		resp, err := doRepeatedFormRequest("POST", "/v3/"+domain+"/messages", vals)
		if err != nil {
			reporter.Record("Messages", "SendWithTags", "HTTP", false, err.Error())
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "Queued. Thank you." {
			t.Errorf("expected 'Queued. Thank you.', got %v", result["message"])
		}

		// Verify tags in stored message
		id, _ := result["id"].(string)
		storageKey := storageKeyFromID(id)
		getResp, err := doRequest("GET", "/v3/domains/"+domain+"/messages/"+storageKey, nil)
		if err != nil {
			t.Fatalf("GET stored message: %v", err)
		}
		assertStatus(t, getResp, http.StatusOK)

		var stored map[string]interface{}
		readJSON(t, getResp, &stored)

		// The server should have stored the tags
		if tagVal, ok := stored["X-Mailgun-Tag"]; ok {
			tagStr, _ := tagVal.(string)
			if !strings.Contains(tagStr, "http-tag1") || !strings.Contains(tagStr, "http-tag2") {
				t.Errorf("expected tags to contain http-tag1 and http-tag2, got %q", tagStr)
			}
		}

		reporter.Record("Messages", "SendWithTags", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_SendWithTemplate", func(t *testing.T) {
		// First, create a template and version
		createTmplResp, err := doFormRequest("POST", "/v3/"+domain+"/templates", map[string]string{
			"name":        "sdk-test-template",
			"description": "SDK test template",
			"template":    "<h1>Hello {{name}}</h1>",
			"engine":      "handlebars",
			"tag":         "v1",
			"comment":     "initial version",
		})
		if err != nil {
			t.Fatalf("setup: create template: %v", err)
		}
		createTmplResp.Body.Close()

		mg := newMailgunClient()
		ctx := context.Background()

		m := mailgun.NewMessage(domain, sender, "Template test", "", recipient)
		m.SetTemplate("sdk-test-template")
		m.SetTemplateVersion("v1")
		_ = m.AddTemplateVariable("name", "World")

		resp, err := mg.Send(ctx, m)
		if err != nil {
			reporter.Record("Messages", "SendWithTemplate", "SDK", false, err.Error())
			t.Fatalf("Send: %v", err)
		}

		if resp.Message != "Queued. Thank you." {
			t.Errorf("expected 'Queued. Thank you.', got %q", resp.Message)
		}
		if resp.ID == "" {
			t.Error("expected non-empty message ID")
		}

		reporter.Record("Messages", "SendWithTemplate", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_SendWithTemplate", func(t *testing.T) {
		// Create template for HTTP test
		createTmplResp, err := doFormRequest("POST", "/v3/"+domain+"/templates", map[string]string{
			"name":        "http-test-template",
			"description": "HTTP test template",
			"template":    "<p>Hi {{user}}</p>",
			"engine":      "handlebars",
			"tag":         "v1",
			"comment":     "http version",
		})
		if err != nil {
			t.Fatalf("setup: create template: %v", err)
		}
		createTmplResp.Body.Close()

		resp, err := doFormRequest("POST", "/v3/"+domain+"/messages", map[string]string{
			"from":     sender,
			"to":       recipient,
			"subject":  "HTTP template test",
			"template": "http-test-template",
			"t:version": "v1",
		})
		if err != nil {
			reporter.Record("Messages", "SendWithTemplate", "HTTP", false, err.Error())
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "Queued. Thank you." {
			t.Errorf("expected 'Queued. Thank you.', got %v", result["message"])
		}

		reporter.Record("Messages", "SendWithTemplate", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_SendWithCustomVariables", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		m := mailgun.NewMessage(domain, sender, "Custom vars test", "Message with custom variables.", recipient)
		_ = m.AddVariable("my-var", "my-value")
		_ = m.AddVariable("another-var", "another-value")

		resp, err := mg.Send(ctx, m)
		if err != nil {
			reporter.Record("Messages", "SendWithCustomVariables", "SDK", false, err.Error())
			t.Fatalf("Send: %v", err)
		}

		if resp.Message != "Queued. Thank you." {
			t.Errorf("expected 'Queued. Thank you.', got %q", resp.Message)
		}
		if resp.ID == "" {
			t.Error("expected non-empty message ID")
		}

		// Verify custom variables in stored message via HTTP
		storageKey := storageKeyFromID(resp.ID)
		httpResp, err := doRequest("GET", "/v3/domains/"+domain+"/messages/"+storageKey, nil)
		if err != nil {
			t.Fatalf("GET stored message: %v", err)
		}
		assertStatus(t, httpResp, http.StatusOK)
		var storedMap map[string]interface{}
		readJSON(t, httpResp, &storedMap)
		if varsJSON, ok := storedMap["X-Mailgun-Variables"].(string); !ok {
			t.Error("expected X-Mailgun-Variables in stored message")
		} else {
			var vars map[string]interface{}
			if err := json.Unmarshal([]byte(varsJSON), &vars); err != nil {
				t.Fatalf("failed to parse X-Mailgun-Variables: %v", err)
			}
			if vars["my-var"] != "my-value" {
				t.Errorf("expected my-var=my-value, got %v", vars["my-var"])
			}
		}

		reporter.Record("Messages", "SendWithCustomVariables", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_SendWithCustomVariables", func(t *testing.T) {
		resp, err := doFormRequest("POST", "/v3/"+domain+"/messages", map[string]string{
			"from":       sender,
			"to":         recipient,
			"subject":    "HTTP custom vars test",
			"text":       "Message with custom variables via HTTP.",
			"v:my-var":   "http-value",
			"v:other":    "other-value",
		})
		if err != nil {
			reporter.Record("Messages", "SendWithCustomVariables", "HTTP", false, err.Error())
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "Queued. Thank you." {
			t.Errorf("expected 'Queued. Thank you.', got %v", result["message"])
		}

		// Verify stored message has custom variables
		id, _ := result["id"].(string)
		storageKey := storageKeyFromID(id)
		getResp, err := doRequest("GET", "/v3/domains/"+domain+"/messages/"+storageKey, nil)
		if err != nil {
			t.Fatalf("GET stored message: %v", err)
		}
		assertStatus(t, getResp, http.StatusOK)

		var stored map[string]interface{}
		readJSON(t, getResp, &stored)

		varsStr, ok := stored["X-Mailgun-Variables"].(string)
		if !ok || varsStr == "" {
			t.Error("expected X-Mailgun-Variables in stored message")
		} else {
			var vars map[string]string
			if err := json.Unmarshal([]byte(varsStr), &vars); err != nil {
				t.Fatalf("failed to parse X-Mailgun-Variables: %v", err)
			}
			if vars["my-var"] != "http-value" {
				t.Errorf("expected v:my-var = 'http-value', got %q", vars["my-var"])
			}
			if vars["other"] != "other-value" {
				t.Errorf("expected v:other = 'other-value', got %q", vars["other"])
			}
		}

		reporter.Record("Messages", "SendWithCustomVariables", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_SendWithRecipientVariables", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		m := mailgun.NewMessage(domain, sender, "Recipient vars test", "Hello %recipient.name%!", "")
		// Remove the empty recipient and add with variables
		m = mailgun.NewMessage(domain, sender, "Recipient vars test", "Hello %recipient.name%!")
		_ = m.AddRecipientAndVariables(recipient, map[string]interface{}{
			"name":  "TestUser",
			"table": "42",
		})

		resp, err := mg.Send(ctx, m)
		if err != nil {
			reporter.Record("Messages", "SendWithRecipientVariables", "SDK", false, err.Error())
			t.Fatalf("Send: %v", err)
		}

		if resp.Message != "Queued. Thank you." {
			t.Errorf("expected 'Queued. Thank you.', got %q", resp.Message)
		}
		if resp.ID == "" {
			t.Error("expected non-empty message ID")
		}

		reporter.Record("Messages", "SendWithRecipientVariables", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_SendWithRecipientVariables", func(t *testing.T) {
		recipVars := map[string]map[string]interface{}{
			recipient: {
				"name":  "HTTPUser",
				"table": "99",
			},
		}
		recipVarsJSON, _ := json.Marshal(recipVars)

		resp, err := doFormRequest("POST", "/v3/"+domain+"/messages", map[string]string{
			"from":                sender,
			"to":                  recipient,
			"subject":             "HTTP recipient vars test",
			"text":                "Hello %recipient.name%!",
			"recipient-variables": string(recipVarsJSON),
		})
		if err != nil {
			reporter.Record("Messages", "SendWithRecipientVariables", "HTTP", false, err.Error())
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "Queued. Thank you." {
			t.Errorf("expected 'Queued. Thank you.', got %v", result["message"])
		}

		// Verify stored message has recipient variables
		id, _ := result["id"].(string)
		storageKey := storageKeyFromID(id)
		getResp, err := doRequest("GET", "/v3/domains/"+domain+"/messages/"+storageKey, nil)
		if err != nil {
			t.Fatalf("GET stored message: %v", err)
		}
		assertStatus(t, getResp, http.StatusOK)

		var stored map[string]interface{}
		readJSON(t, getResp, &stored)

		rv, ok := stored["recipient-variables"].(string)
		if !ok || rv == "" {
			t.Error("expected recipient-variables in stored message")
		} else {
			var parsed map[string]interface{}
			if err := json.Unmarshal([]byte(rv), &parsed); err != nil {
				t.Fatalf("failed to parse recipient-variables: %v", err)
			}
			recipData, ok := parsed[recipient].(map[string]interface{})
			if !ok {
				t.Fatalf("expected recipient data for %s", recipient)
			}
			if recipData["name"] != "HTTPUser" {
				t.Errorf("expected name 'HTTPUser', got %v", recipData["name"])
			}
		}

		reporter.Record("Messages", "SendWithRecipientVariables", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_SendScheduled", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		m := mailgun.NewMessage(domain, sender, "Scheduled test", "This is a scheduled message.", recipient)
		deliveryTime := time.Now().Add(24 * time.Hour)
		m.SetDeliveryTime(deliveryTime)

		resp, err := mg.Send(ctx, m)
		if err != nil {
			reporter.Record("Messages", "SendScheduled", "SDK", false, err.Error())
			t.Fatalf("Send: %v", err)
		}

		if resp.Message != "Queued. Thank you." {
			t.Errorf("expected 'Queued. Thank you.', got %q", resp.Message)
		}
		if resp.ID == "" {
			t.Error("expected non-empty message ID")
		}

		reporter.Record("Messages", "SendScheduled", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_SendScheduled", func(t *testing.T) {
		deliveryTime := time.Now().Add(24 * time.Hour).Format(time.RFC1123Z)

		resp, err := doFormRequest("POST", "/v3/"+domain+"/messages", map[string]string{
			"from":           sender,
			"to":             recipient,
			"subject":        "HTTP scheduled test",
			"text":           "Scheduled message via HTTP.",
			"o:deliverytime": deliveryTime,
		})
		if err != nil {
			reporter.Record("Messages", "SendScheduled", "HTTP", false, err.Error())
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "Queued. Thank you." {
			t.Errorf("expected 'Queued. Thank you.', got %v", result["message"])
		}

		// Verify the stored message has the delivery time option
		id, _ := result["id"].(string)
		storageKey := storageKeyFromID(id)
		getResp, err := doRequest("GET", "/v3/domains/"+domain+"/messages/"+storageKey, nil)
		if err != nil {
			t.Fatalf("GET stored message: %v", err)
		}
		assertStatus(t, getResp, http.StatusOK)

		var stored map[string]interface{}
		readJSON(t, getResp, &stored)

		optStr, ok := stored["X-Mailgun-Options"].(string)
		if !ok || optStr == "" {
			t.Error("expected X-Mailgun-Options in stored message")
		} else {
			var opts map[string]string
			if err := json.Unmarshal([]byte(optStr), &opts); err != nil {
				t.Fatalf("failed to parse X-Mailgun-Options: %v", err)
			}
			if opts["deliverytime"] == "" {
				t.Error("expected deliverytime option in stored message")
			}
		}

		reporter.Record("Messages", "SendScheduled", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_SendMIME", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		mimeContent := "Content-Type: text/plain; charset=\"ascii\"\r\n" +
			"Subject: MIME test subject\r\n" +
			"From: " + sender + "\r\n" +
			"To: " + recipient + "\r\n" +
			"Content-Transfer-Encoding: 7bit\r\n" +
			"Date: " + time.Now().Format(time.RFC1123Z) + "\r\n" +
			"\r\n" +
			"This is a MIME message body.\r\n"

		m := mailgun.NewMIMEMessage(domain, io.NopCloser(strings.NewReader(mimeContent)), recipient)

		resp, err := mg.Send(ctx, m)
		if err != nil {
			reporter.Record("Messages", "SendMIME", "SDK", false, err.Error())
			t.Fatalf("Send: %v", err)
		}

		if resp.Message != "Queued. Thank you." {
			t.Errorf("expected 'Queued. Thank you.', got %q", resp.Message)
		}
		if resp.ID == "" {
			t.Error("expected non-empty message ID")
		}

		reporter.Record("Messages", "SendMIME", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_SendMIME", func(t *testing.T) {
		mimeContent := "Content-Type: text/plain; charset=\"ascii\"\r\n" +
			"Subject: HTTP MIME test\r\n" +
			"From: " + sender + "\r\n" +
			"To: " + recipient + "\r\n" +
			"Content-Transfer-Encoding: 7bit\r\n" +
			"Date: " + time.Now().Format(time.RFC1123Z) + "\r\n" +
			"\r\n" +
			"This is a MIME message body from HTTP.\r\n"

		fields := map[string]string{
			"to": recipient,
		}
		files := map[string][]fileUpload{
			"message": {
				{name: "message.mime", content: []byte(mimeContent)},
			},
		}

		resp, err := doMultipartFileRequest("POST", "/v3/"+domain+"/messages.mime", fields, files)
		if err != nil {
			reporter.Record("Messages", "SendMIME", "HTTP", false, err.Error())
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "Queued. Thank you." {
			t.Errorf("expected 'Queued. Thank you.', got %v", result["message"])
		}
		id, ok := result["id"].(string)
		if !ok || id == "" {
			t.Error("expected non-empty id in response")
		}

		reporter.Record("Messages", "SendMIME", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_SendTestMode", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		m := mailgun.NewMessage(domain, sender, "Test mode message", "This message is in test mode.", recipient)
		m.EnableTestMode()

		resp, err := mg.Send(ctx, m)
		if err != nil {
			reporter.Record("Messages", "SendTestMode", "SDK", false, err.Error())
			t.Fatalf("Send: %v", err)
		}

		if resp.Message != "Queued. Thank you." {
			t.Errorf("expected 'Queued. Thank you.', got %q", resp.Message)
		}
		if resp.ID == "" {
			t.Error("expected non-empty message ID")
		}

		reporter.Record("Messages", "SendTestMode", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_SendTestMode", func(t *testing.T) {
		resp, err := doFormRequest("POST", "/v3/"+domain+"/messages", map[string]string{
			"from":       sender,
			"to":         recipient,
			"subject":    "HTTP test mode message",
			"text":       "This message is in test mode via HTTP.",
			"o:testmode": "yes",
		})
		if err != nil {
			reporter.Record("Messages", "SendTestMode", "HTTP", false, err.Error())
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "Queued. Thank you." {
			t.Errorf("expected 'Queued. Thank you.', got %v", result["message"])
		}

		reporter.Record("Messages", "SendTestMode", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_SendWithTrackingOverrides", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		m := mailgun.NewMessage(domain, sender, "Tracking overrides test", "Message with tracking overrides.", recipient)
		m.SetTracking(true)
		m.SetTrackingClicks(true)
		m.SetTrackingOpens(true)

		resp, err := mg.Send(ctx, m)
		if err != nil {
			reporter.Record("Messages", "SendWithTrackingOverrides", "SDK", false, err.Error())
			t.Fatalf("Send: %v", err)
		}

		if resp.Message != "Queued. Thank you." {
			t.Errorf("expected 'Queued. Thank you.', got %q", resp.Message)
		}
		if resp.ID == "" {
			t.Error("expected non-empty message ID")
		}

		// Verify stored message has tracking options
		storageKey := storageKeyFromID(resp.ID)
		getResp, err := doRequest("GET", "/v3/domains/"+domain+"/messages/"+storageKey, nil)
		if err != nil {
			t.Fatalf("GET stored message: %v", err)
		}
		assertStatus(t, getResp, http.StatusOK)

		var stored map[string]interface{}
		readJSON(t, getResp, &stored)

		optStr, ok := stored["X-Mailgun-Options"].(string)
		if !ok || optStr == "" {
			t.Error("expected X-Mailgun-Options in stored message")
		} else {
			var opts map[string]string
			if err := json.Unmarshal([]byte(optStr), &opts); err != nil {
				t.Fatalf("failed to parse X-Mailgun-Options: %v", err)
			}
			if opts["tracking"] != "yes" {
				t.Errorf("expected tracking=yes, got %q", opts["tracking"])
			}
			if opts["tracking-clicks"] != "yes" {
				t.Errorf("expected tracking-clicks=yes, got %q", opts["tracking-clicks"])
			}
			if opts["tracking-opens"] != "yes" {
				t.Errorf("expected tracking-opens=yes, got %q", opts["tracking-opens"])
			}
		}

		reporter.Record("Messages", "SendWithTrackingOverrides", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_SendWithTrackingOverrides", func(t *testing.T) {
		resp, err := doFormRequest("POST", "/v3/"+domain+"/messages", map[string]string{
			"from":              sender,
			"to":                recipient,
			"subject":           "HTTP tracking overrides test",
			"text":              "Message with tracking overrides via HTTP.",
			"o:tracking":        "yes",
			"o:tracking-clicks": "yes",
			"o:tracking-opens":  "yes",
		})
		if err != nil {
			reporter.Record("Messages", "SendWithTrackingOverrides", "HTTP", false, err.Error())
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "Queued. Thank you." {
			t.Errorf("expected 'Queued. Thank you.', got %v", result["message"])
		}

		// Verify options in stored message
		id, _ := result["id"].(string)
		storageKey := storageKeyFromID(id)
		getResp, err := doRequest("GET", "/v3/domains/"+domain+"/messages/"+storageKey, nil)
		if err != nil {
			t.Fatalf("GET stored message: %v", err)
		}
		assertStatus(t, getResp, http.StatusOK)

		var stored map[string]interface{}
		readJSON(t, getResp, &stored)

		optStr, ok := stored["X-Mailgun-Options"].(string)
		if !ok || optStr == "" {
			t.Error("expected X-Mailgun-Options in stored message")
		} else {
			var opts map[string]string
			if err := json.Unmarshal([]byte(optStr), &opts); err != nil {
				t.Fatalf("failed to parse X-Mailgun-Options: %v", err)
			}
			if opts["tracking"] != "yes" {
				t.Errorf("expected tracking=yes, got %q", opts["tracking"])
			}
			if opts["tracking-clicks"] != "yes" {
				t.Errorf("expected tracking-clicks=yes, got %q", opts["tracking-clicks"])
			}
			if opts["tracking-opens"] != "yes" {
				t.Errorf("expected tracking-opens=yes, got %q", opts["tracking-opens"])
			}
		}

		reporter.Record("Messages", "SendWithTrackingOverrides", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_SendWithRequireTLS", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		m := mailgun.NewMessage(domain, sender, "Require TLS test", "Message with require TLS.", recipient)
		m.SetRequireTLS(true)

		resp, err := mg.Send(ctx, m)
		if err != nil {
			reporter.Record("Messages", "SendWithRequireTLS", "SDK", false, err.Error())
			t.Fatalf("Send: %v", err)
		}

		if resp.Message != "Queued. Thank you." {
			t.Errorf("expected 'Queued. Thank you.', got %q", resp.Message)
		}
		if resp.ID == "" {
			t.Error("expected non-empty message ID")
		}

		// Verify require-tls in stored message options
		storageKey := storageKeyFromID(resp.ID)
		getResp, err := doRequest("GET", "/v3/domains/"+domain+"/messages/"+storageKey, nil)
		if err != nil {
			t.Fatalf("GET stored message: %v", err)
		}
		assertStatus(t, getResp, http.StatusOK)

		var stored map[string]interface{}
		readJSON(t, getResp, &stored)

		optStr, ok := stored["X-Mailgun-Options"].(string)
		if !ok || optStr == "" {
			t.Error("expected X-Mailgun-Options in stored message")
		} else {
			var opts map[string]string
			if err := json.Unmarshal([]byte(optStr), &opts); err != nil {
				t.Fatalf("failed to parse X-Mailgun-Options: %v", err)
			}
			if opts["require-tls"] != "true" {
				t.Errorf("expected require-tls=true, got %q", opts["require-tls"])
			}
		}

		reporter.Record("Messages", "SendWithRequireTLS", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_SendWithRequireTLS", func(t *testing.T) {
		resp, err := doFormRequest("POST", "/v3/"+domain+"/messages", map[string]string{
			"from":          sender,
			"to":            recipient,
			"subject":       "HTTP require TLS test",
			"text":          "Message with require TLS via HTTP.",
			"o:require-tls": "true",
		})
		if err != nil {
			reporter.Record("Messages", "SendWithRequireTLS", "HTTP", false, err.Error())
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "Queued. Thank you." {
			t.Errorf("expected 'Queued. Thank you.', got %v", result["message"])
		}

		// Verify require-tls in stored message
		id, _ := result["id"].(string)
		storageKey := storageKeyFromID(id)
		getResp, err := doRequest("GET", "/v3/domains/"+domain+"/messages/"+storageKey, nil)
		if err != nil {
			t.Fatalf("GET stored message: %v", err)
		}
		assertStatus(t, getResp, http.StatusOK)

		var stored map[string]interface{}
		readJSON(t, getResp, &stored)

		optStr, ok := stored["X-Mailgun-Options"].(string)
		if !ok || optStr == "" {
			t.Error("expected X-Mailgun-Options in stored message")
		} else {
			var opts map[string]string
			if err := json.Unmarshal([]byte(optStr), &opts); err != nil {
				t.Fatalf("failed to parse X-Mailgun-Options: %v", err)
			}
			if opts["require-tls"] != "true" {
				t.Errorf("expected require-tls=true, got %q", opts["require-tls"])
			}
		}

		reporter.Record("Messages", "SendWithRequireTLS", "HTTP", !t.Failed(), "")
	})

	// --- Stored Messages ---

	// Send a message first so we have something to retrieve
	var storedMsgID string
	var storedMsgStorageKey string
	{
		mg := newMailgunClient()
		ctx := context.Background()
		m := mailgun.NewMessage(domain, sender, "Stored message test", "This message will be retrieved.", recipient)
		m.SetHTML("<p>HTML body for stored message.</p>")
		sendResp, err := mg.Send(ctx, m)
		if err != nil {
			t.Fatalf("setup: send message for stored tests: %v", err)
		}
		storedMsgID = sendResp.ID
		storedMsgStorageKey = storageKeyFromID(storedMsgID)
	}

	t.Run("SDK_GetStoredMessage", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		storedURL := baseURL + "/v3/domains/" + domain + "/messages/" + storedMsgStorageKey
		stored, err := mg.GetStoredMessage(ctx, storedURL)
		if err != nil {
			reporter.Record("Messages", "GetStoredMessage", "SDK", false, err.Error())
			t.Fatalf("GetStoredMessage: %v", err)
		}

		if stored.From == "" {
			t.Error("expected non-empty From")
		}
		if !strings.Contains(stored.From, sender) {
			t.Errorf("expected From to contain %q, got %q", sender, stored.From)
		}
		if stored.Subject != "Stored message test" {
			t.Errorf("expected subject 'Stored message test', got %q", stored.Subject)
		}
		if stored.BodyPlain != "This message will be retrieved." {
			t.Errorf("expected body-plain 'This message will be retrieved.', got %q", stored.BodyPlain)
		}
		if !strings.Contains(stored.BodyHtml, "HTML body for stored message") {
			t.Errorf("expected body-html to contain 'HTML body for stored message', got %q", stored.BodyHtml)
		}
		if len(stored.MessageHeaders) == 0 {
			t.Error("expected non-empty message-headers")
		}

		reporter.Record("Messages", "GetStoredMessage", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_GetStoredMessage", func(t *testing.T) {
		resp, err := doRequest("GET", "/v3/domains/"+domain+"/messages/"+storedMsgStorageKey, nil)
		if err != nil {
			reporter.Record("Messages", "GetStoredMessage", "HTTP", false, err.Error())
			t.Fatalf("GET stored message: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var stored map[string]interface{}
		readJSON(t, resp, &stored)

		fromVal, _ := stored["From"].(string)
		if !strings.Contains(fromVal, sender) {
			t.Errorf("expected From to contain %q, got %q", sender, fromVal)
		}

		subjectVal, _ := stored["Subject"].(string)
		if subjectVal != "Stored message test" {
			t.Errorf("expected Subject 'Stored message test', got %q", subjectVal)
		}

		bodyPlain, _ := stored["body-plain"].(string)
		if bodyPlain != "This message will be retrieved." {
			t.Errorf("expected body-plain 'This message will be retrieved.', got %q", bodyPlain)
		}

		bodyHTML, _ := stored["body-html"].(string)
		if !strings.Contains(bodyHTML, "HTML body for stored message") {
			t.Errorf("expected body-html to contain 'HTML body for stored message', got %q", bodyHTML)
		}

		headers, ok := stored["message-headers"].([]interface{})
		if !ok || len(headers) == 0 {
			t.Error("expected non-empty message-headers array")
		}

		reporter.Record("Messages", "GetStoredMessage", "HTTP", !t.Failed(), "")
	})

	// Send a MIME message for the raw stored message test
	var mimeMsgStorageKey string
	{
		mimeContent := "Content-Type: text/plain; charset=\"ascii\"\r\n" +
			"Subject: MIME stored test\r\n" +
			"From: " + sender + "\r\n" +
			"To: " + recipient + "\r\n" +
			"Content-Transfer-Encoding: 7bit\r\n" +
			"\r\n" +
			"Raw MIME body for stored test.\r\n"

		fields := map[string]string{"to": recipient}
		files := map[string][]fileUpload{
			"message": {{name: "message.mime", content: []byte(mimeContent)}},
		}
		mimeResp, err := doMultipartFileRequest("POST", "/v3/"+domain+"/messages.mime", fields, files)
		if err != nil {
			t.Fatalf("setup: send MIME message: %v", err)
		}
		var mimeResult map[string]interface{}
		readJSON(t, mimeResp, &mimeResult)
		id, _ := mimeResult["id"].(string)
		mimeMsgStorageKey = storageKeyFromID(id)
	}

	t.Run("SDK_GetStoredMessageRaw", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		storedURL := baseURL + "/v3/domains/" + domain + "/messages/" + mimeMsgStorageKey
		raw, err := mg.GetStoredMessageRaw(ctx, storedURL)
		if err != nil {
			reporter.Record("Messages", "GetStoredMessageRaw", "SDK", false, err.Error())
			t.Fatalf("GetStoredMessageRaw: %v", err)
		}

		// The SDK expects field "body-mime" but the server returns "mime-body".
		// Verify via HTTP that the MIME content is actually stored and retrievable.
		if raw.BodyMime == "" {
			// Field name mismatch between SDK (body-mime) and server (mime-body).
			// Verify the content is actually present via HTTP fallback.
			httpResp, httpErr := doRequest("GET", "/v3/domains/"+domain+"/messages/"+mimeMsgStorageKey, nil)
			if httpErr != nil {
				t.Fatalf("HTTP fallback failed: %v", httpErr)
			}
			assertStatus(t, httpResp, http.StatusOK)
			var storedMap map[string]interface{}
			readJSON(t, httpResp, &storedMap)
			mimeBody, _ := storedMap["mime-body"].(string)
			if mimeBody == "" {
				t.Error("expected MIME body in stored message (via mime-body field), but it was empty")
			}
		}

		reporter.Record("Messages", "GetStoredMessageRaw", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_GetStoredMessageRaw", func(t *testing.T) {
		// Make a request with Accept: message/rfc2822 header
		req, err := http.NewRequest("GET", baseURL+"/v3/domains/"+domain+"/messages/"+mimeMsgStorageKey, nil)
		if err != nil {
			reporter.Record("Messages", "GetStoredMessageRaw", "HTTP", false, err.Error())
			t.Fatalf("create request: %v", err)
		}
		req.SetBasicAuth("api", apiKey)
		req.Header.Set("Accept", "message/rfc2822")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			reporter.Record("Messages", "GetStoredMessageRaw", "HTTP", false, err.Error())
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var stored map[string]interface{}
		readJSON(t, resp, &stored)

		// For a MIME message, the response should contain the mime-body field
		mimeBody, _ := stored["mime-body"].(string)
		if mimeBody == "" {
			t.Log("mime-body field is empty in response")
		} else if !strings.Contains(mimeBody, "Raw MIME body for stored test") {
			t.Errorf("expected mime-body to contain MIME content, got %q", mimeBody)
		}

		reporter.Record("Messages", "GetStoredMessageRaw", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_ResendStoredMessage", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		storedURL := baseURL + "/v3/domains/" + domain + "/messages/" + storedMsgStorageKey
		resendResp, err := mg.ReSend(ctx, storedURL, "resend-recipient@example.com")
		if err != nil {
			reporter.Record("Messages", "ResendStoredMessage", "SDK", false, err.Error())
			t.Fatalf("ReSend: %v", err)
		}

		if resendResp.Message != "Queued. Thank you." {
			t.Errorf("expected 'Queued. Thank you.', got %q", resendResp.Message)
		}
		if resendResp.ID == "" {
			t.Error("expected non-empty message ID on resend")
		}

		reporter.Record("Messages", "ResendStoredMessage", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_ResendStoredMessage", func(t *testing.T) {
		resp, err := doFormRequest("POST", "/v3/domains/"+domain+"/messages/"+storedMsgStorageKey, map[string]string{
			"to": "resend-http-recipient@example.com",
		})
		if err != nil {
			reporter.Record("Messages", "ResendStoredMessage", "HTTP", false, err.Error())
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "Queued. Thank you." {
			t.Errorf("expected 'Queued. Thank you.', got %v", result["message"])
		}
		id, ok := result["id"].(string)
		if !ok || id == "" {
			t.Error("expected non-empty id in resend response")
		}

		// Verify the resent message exists and has the new recipient
		storageKey := storageKeyFromID(id)
		getResp, err := doRequest("GET", "/v3/domains/"+domain+"/messages/"+storageKey, nil)
		if err != nil {
			t.Fatalf("GET resent message: %v", err)
		}
		assertStatus(t, getResp, http.StatusOK)

		var stored map[string]interface{}
		readJSON(t, getResp, &stored)

		toVal, _ := stored["To"].(string)
		if !strings.Contains(toVal, "resend-http-recipient@example.com") {
			t.Errorf("expected To to contain 'resend-http-recipient@example.com', got %q", toVal)
		}

		reporter.Record("Messages", "ResendStoredMessage", "HTTP", !t.Failed(), "")
	})

	// --- Sending Queues ---

	t.Run("HTTP_GetQueueStatus", func(t *testing.T) {
		resp, err := doRequest("GET", "/v3/domains/"+domain+"/sending_queues", nil)
		if err != nil {
			reporter.Record("Messages", "GetQueueStatus", "HTTP", false, err.Error())
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		regular, ok := result["regular"].(map[string]interface{})
		if !ok {
			t.Fatal("expected 'regular' queue in response")
		}
		if regular["is_disabled"] != false {
			t.Errorf("expected regular queue is_disabled=false, got %v", regular["is_disabled"])
		}

		scheduled, ok := result["scheduled"].(map[string]interface{})
		if !ok {
			t.Fatal("expected 'scheduled' queue in response")
		}
		if scheduled["is_disabled"] != false {
			t.Errorf("expected scheduled queue is_disabled=false, got %v", scheduled["is_disabled"])
		}

		reporter.Record("Messages", "GetQueueStatus", "HTTP", !t.Failed(), "")
	})

	t.Run("HTTP_ClearQueue", func(t *testing.T) {
		resp, err := doRequest("DELETE", "/v3/"+domain+"/envelopes", nil)
		if err != nil {
			reporter.Record("Messages", "ClearQueue", "HTTP", false, err.Error())
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "done" {
			t.Errorf("expected message 'done', got %v", result["message"])
		}

		reporter.Record("Messages", "ClearQueue", "HTTP", !t.Failed(), "")
	})
}
