package integration

import (
	"context"
	"net/http"
	"testing"

	"github.com/mailgun/mailgun-go/v5"
	"github.com/mailgun/mailgun-go/v5/mtypes"
)

func TestTemplates(t *testing.T) {
	resetServer(t)

	const domain = "test-templates.example.com"
	ctx := context.Background()

	// Setup: create a domain first
	resp, err := doFormRequest("POST", "/v4/domains", map[string]string{"name": domain})
	if err != nil {
		t.Fatalf("setup: create domain failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("setup: create domain returned status %d", resp.StatusCode)
	}

	const templateName = "my-template"
	const templateDesc = "A test template"
	const templateContent = "<h1>Hello {{name}}</h1>"

	// --- Template CRUD ---

	t.Run("SDK_CreateTemplate", func(t *testing.T) {
		mg := newMailgunClient()

		tmpl := mtypes.Template{
			Name:        templateName,
			Description: templateDesc,
			Version: mtypes.TemplateVersion{
				Template: templateContent,
				Engine:   mtypes.TemplateEngineHandlebars,
				Tag:      "initial",
				Comment:  "first version",
			},
		}

		err := mg.CreateTemplate(ctx, domain, &tmpl)
		if err != nil {
			t.Fatalf("CreateTemplate failed: %v", err)
		}

		if tmpl.Name != templateName {
			t.Errorf("expected template name %q, got %q", templateName, tmpl.Name)
		}

		reporter.Record("Templates", "CreateTemplate", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_CreateTemplate", func(t *testing.T) {
		resp, err := doFormRequest("POST", "/v3/"+domain+"/templates", map[string]string{
			"name":        "http-template",
			"description": "Created via HTTP",
			"template":    "<p>Hello {{user}}</p>",
			"engine":      "handlebars",
			"tag":         "initial",
			"comment":     "http version",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "template has been stored" {
			t.Errorf("expected message 'template has been stored', got %v", result["message"])
		}

		tmpl, ok := result["template"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected 'template' object in response")
		}
		if tmpl["name"] != "http-template" {
			t.Errorf("expected template name 'http-template', got %v", tmpl["name"])
		}

		reporter.Record("Templates", "CreateTemplate", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_ListTemplates", func(t *testing.T) {
		mg := newMailgunClient()

		iter := mg.ListTemplates(domain, &mailgun.ListTemplateOptions{
			Limit:  10,
			Active: true,
		})

		var templates []mtypes.Template
		if !iter.Next(ctx, &templates) {
			if iter.Err() != nil {
				t.Fatalf("ListTemplates iteration failed: %v", iter.Err())
			}
			t.Fatalf("expected at least one page of templates")
		}

		if len(templates) < 2 {
			t.Errorf("expected at least 2 templates, got %d", len(templates))
		}

		found := false
		for _, tmpl := range templates {
			if tmpl.Name == templateName {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected to find template %q in list", templateName)
		}

		reporter.Record("Templates", "ListTemplates", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_ListTemplates", func(t *testing.T) {
		resp, err := doRequest("GET", "/v3/"+domain+"/templates?active=yes", nil)
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
			t.Errorf("expected at least 2 templates, got %d", len(items))
		}

		reporter.Record("Templates", "ListTemplates", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_GetTemplate", func(t *testing.T) {
		mg := newMailgunClient()

		tmpl, err := mg.GetTemplate(ctx, domain, templateName)
		if err != nil {
			t.Fatalf("GetTemplate failed: %v", err)
		}

		if tmpl.Name != templateName {
			t.Errorf("expected template name %q, got %q", templateName, tmpl.Name)
		}
		if tmpl.Description != templateDesc {
			t.Errorf("expected description %q, got %q", templateDesc, tmpl.Description)
		}
		// GetTemplate with active=yes should include the active version
		if tmpl.Version.Tag == "" {
			t.Errorf("expected active version to be included")
		}

		reporter.Record("Templates", "GetTemplate", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_GetTemplate", func(t *testing.T) {
		resp, err := doRequest("GET", "/v3/"+domain+"/templates/"+templateName+"?active=yes", nil)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		tmpl, ok := result["template"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected 'template' object in response")
		}
		if tmpl["name"] != templateName {
			t.Errorf("expected template name %q, got %v", templateName, tmpl["name"])
		}
		if tmpl["description"] != templateDesc {
			t.Errorf("expected description %q, got %v", templateDesc, tmpl["description"])
		}

		reporter.Record("Templates", "GetTemplate", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_UpdateTemplate", func(t *testing.T) {
		mg := newMailgunClient()

		tmpl := &mtypes.Template{
			Name:        templateName,
			Description: "Updated description",
		}

		err := mg.UpdateTemplate(ctx, domain, tmpl)
		if err != nil {
			t.Fatalf("UpdateTemplate failed: %v", err)
		}

		// Verify the update
		got, err := mg.GetTemplate(ctx, domain, templateName)
		if err != nil {
			t.Fatalf("GetTemplate after update failed: %v", err)
		}
		if got.Description != "Updated description" {
			t.Errorf("expected description 'Updated description', got %q", got.Description)
		}

		reporter.Record("Templates", "UpdateTemplate", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_UpdateTemplate", func(t *testing.T) {
		resp, err := doFormRequest("PUT", "/v3/"+domain+"/templates/"+templateName, map[string]string{
			"description": "HTTP updated description",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "template has been updated" {
			t.Errorf("expected message 'template has been updated', got %v", result["message"])
		}

		tmpl, ok := result["template"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected 'template' object in response")
		}
		if tmpl["name"] != templateName {
			t.Errorf("expected template name %q, got %v", templateName, tmpl["name"])
		}

		reporter.Record("Templates", "UpdateTemplate", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_DeleteTemplate", func(t *testing.T) {
		mg := newMailgunClient()

		// Delete the http-template (keep my-template for version tests)
		err := mg.DeleteTemplate(ctx, domain, "http-template")
		if err != nil {
			t.Fatalf("DeleteTemplate failed: %v", err)
		}

		reporter.Record("Templates", "DeleteTemplate", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_DeleteTemplate", func(t *testing.T) {
		// Create a throwaway template to delete via HTTP
		_, err := doFormRequest("POST", "/v3/"+domain+"/templates", map[string]string{
			"name":        "delete-me",
			"description": "to be deleted",
		})
		if err != nil {
			t.Fatalf("setup create template failed: %v", err)
		}

		resp, err := doRequest("DELETE", "/v3/"+domain+"/templates/delete-me", nil)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "template has been deleted" {
			t.Errorf("expected message 'template has been deleted', got %v", result["message"])
		}

		tmpl, ok := result["template"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected 'template' object in response")
		}
		if tmpl["name"] != "delete-me" {
			t.Errorf("expected template name 'delete-me', got %v", tmpl["name"])
		}

		reporter.Record("Templates", "DeleteTemplate", "HTTP", !t.Failed(), "")
	})

	// --- Template Versions ---

	t.Run("SDK_AddTemplateVersion", func(t *testing.T) {
		mg := newMailgunClient()

		version := &mtypes.TemplateVersion{
			Tag:      "v2",
			Template: "<h1>Hello {{name}} v2</h1>",
			Engine:   mtypes.TemplateEngineHandlebars,
			Comment:  "second version",
			Active:   true,
		}

		err := mg.AddTemplateVersion(ctx, domain, templateName, version)
		if err != nil {
			t.Fatalf("AddTemplateVersion failed: %v", err)
		}

		if version.Tag != "v2" {
			t.Errorf("expected version tag 'v2', got %q", version.Tag)
		}

		reporter.Record("Templates", "AddTemplateVersion", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_AddTemplateVersion", func(t *testing.T) {
		resp, err := doFormRequest("POST", "/v3/"+domain+"/templates/"+templateName+"/versions", map[string]string{
			"tag":      "v3",
			"template": "<h1>Hello {{name}} v3</h1>",
			"engine":   "handlebars",
			"comment":  "third version via HTTP",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "new version of the template has been stored" {
			t.Errorf("expected message 'new version of the template has been stored', got %v", result["message"])
		}

		tmpl, ok := result["template"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected 'template' object in response")
		}
		if tmpl["name"] != templateName {
			t.Errorf("expected template name %q, got %v", templateName, tmpl["name"])
		}

		reporter.Record("Templates", "AddTemplateVersion", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_ListTemplateVersions", func(t *testing.T) {
		mg := newMailgunClient()

		iter := mg.ListTemplateVersions(domain, templateName, &mailgun.ListOptions{
			Limit: 10,
		})

		var versions []mtypes.TemplateVersion
		if !iter.Next(ctx, &versions) {
			if iter.Err() != nil {
				t.Fatalf("ListTemplateVersions iteration failed: %v", iter.Err())
			}
			t.Fatalf("expected at least one page of versions")
		}

		if len(versions) < 3 {
			t.Errorf("expected at least 3 versions (initial, v2, v3), got %d", len(versions))
		}

		reporter.Record("Templates", "ListTemplateVersions", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_ListTemplateVersions", func(t *testing.T) {
		resp, err := doRequest("GET", "/v3/"+domain+"/templates/"+templateName+"/versions", nil)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		tmpl, ok := result["template"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected 'template' object in response")
		}

		versions, ok := tmpl["versions"].([]interface{})
		if !ok {
			t.Fatalf("expected 'versions' array in template")
		}
		if len(versions) < 3 {
			t.Errorf("expected at least 3 versions, got %d", len(versions))
		}

		reporter.Record("Templates", "ListTemplateVersions", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_GetTemplateVersion", func(t *testing.T) {
		mg := newMailgunClient()

		version, err := mg.GetTemplateVersion(ctx, domain, templateName, "v2")
		if err != nil {
			t.Fatalf("GetTemplateVersion failed: %v", err)
		}

		if version.Tag != "v2" {
			t.Errorf("expected version tag 'v2', got %q", version.Tag)
		}
		if version.Template != "<h1>Hello {{name}} v2</h1>" {
			t.Errorf("expected template content '<h1>Hello {{name}} v2</h1>', got %q", version.Template)
		}
		if version.Engine != mtypes.TemplateEngineHandlebars {
			t.Errorf("expected engine 'handlebars', got %q", version.Engine)
		}

		reporter.Record("Templates", "GetTemplateVersion", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_GetTemplateVersion", func(t *testing.T) {
		resp, err := doRequest("GET", "/v3/"+domain+"/templates/"+templateName+"/versions/v3", nil)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		tmpl, ok := result["template"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected 'template' object in response")
		}
		if tmpl["name"] != templateName {
			t.Errorf("expected template name %q, got %v", templateName, tmpl["name"])
		}

		version, ok := tmpl["version"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected 'version' object in template")
		}
		if version["tag"] != "v3" {
			t.Errorf("expected version tag 'v3', got %v", version["tag"])
		}
		if version["template"] != "<h1>Hello {{name}} v3</h1>" {
			t.Errorf("expected template content, got %v", version["template"])
		}

		reporter.Record("Templates", "GetTemplateVersion", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_UpdateTemplateVersion", func(t *testing.T) {
		mg := newMailgunClient()

		version := &mtypes.TemplateVersion{
			Tag:      "v2",
			Template: "<h1>Hello {{name}} v2 updated</h1>",
			Comment:  "updated comment",
			Active:   true,
		}

		err := mg.UpdateTemplateVersion(ctx, domain, templateName, version)
		if err != nil {
			t.Fatalf("UpdateTemplateVersion failed: %v", err)
		}

		if version.Tag != "v2" {
			t.Errorf("expected version tag 'v2', got %q", version.Tag)
		}

		reporter.Record("Templates", "UpdateTemplateVersion", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_UpdateTemplateVersion", func(t *testing.T) {
		resp, err := doFormRequest("PUT", "/v3/"+domain+"/templates/"+templateName+"/versions/v3", map[string]string{
			"template": "<h1>Hello {{name}} v3 updated</h1>",
			"comment":  "updated via HTTP",
			"active":   "yes",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "version has been updated" {
			t.Errorf("expected message 'version has been updated', got %v", result["message"])
		}

		tmpl, ok := result["template"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected 'template' object in response")
		}
		if tmpl["name"] != templateName {
			t.Errorf("expected template name %q, got %v", templateName, tmpl["name"])
		}

		version, ok := tmpl["version"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected 'version' object in template")
		}
		if version["tag"] != "v3" {
			t.Errorf("expected version tag 'v3', got %v", version["tag"])
		}

		reporter.Record("Templates", "UpdateTemplateVersion", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_DeleteTemplateVersion", func(t *testing.T) {
		mg := newMailgunClient()

		err := mg.DeleteTemplateVersion(ctx, domain, templateName, "v3")
		if err != nil {
			t.Fatalf("DeleteTemplateVersion failed: %v", err)
		}

		reporter.Record("Templates", "DeleteTemplateVersion", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_DeleteTemplateVersion", func(t *testing.T) {
		resp, err := doRequest("DELETE", "/v3/"+domain+"/templates/"+templateName+"/versions/v2", nil)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "version has been deleted" {
			t.Errorf("expected message 'version has been deleted', got %v", result["message"])
		}

		tmpl, ok := result["template"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected 'template' object in response")
		}
		if tmpl["name"] != templateName {
			t.Errorf("expected template name %q, got %v", templateName, tmpl["name"])
		}

		version, ok := tmpl["version"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected 'version' object in template")
		}
		if version["tag"] != "v2" {
			t.Errorf("expected version tag 'v2', got %v", version["tag"])
		}

		reporter.Record("Templates", "DeleteTemplateVersion", "HTTP", !t.Failed(), "")
	})
}

// Ensure imports are used.
var _ mailgun.ListTemplateOptions
var _ mtypes.Template
