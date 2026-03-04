package integration

import "testing"

func TestTemplates(t *testing.T) {
	resetServer(t)

	// --- Template CRUD ---

	t.Run("SDK_CreateTemplate", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_CreateTemplate", func(t *testing.T) {
		t.Skip("TODO: implement — POST /v3/{domain}/templates")
	})

	t.Run("SDK_ListTemplates", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_ListTemplates", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/{domain}/templates")
	})

	t.Run("SDK_GetTemplate", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_GetTemplate", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/{domain}/templates/{name}")
	})

	t.Run("SDK_UpdateTemplate", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_UpdateTemplate", func(t *testing.T) {
		t.Skip("TODO: implement — PUT /v3/{domain}/templates/{name}")
	})

	t.Run("SDK_DeleteTemplate", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_DeleteTemplate", func(t *testing.T) {
		t.Skip("TODO: implement — DELETE /v3/{domain}/templates/{name}")
	})

	// --- Template Versions ---

	t.Run("SDK_AddTemplateVersion", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_AddTemplateVersion", func(t *testing.T) {
		t.Skip("TODO: implement — POST /v3/{domain}/templates/{name}/versions")
	})

	t.Run("SDK_ListTemplateVersions", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_ListTemplateVersions", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/{domain}/templates/{name}/versions")
	})

	t.Run("SDK_GetTemplateVersion", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_GetTemplateVersion", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/{domain}/templates/{name}/versions/{tag}")
	})

	t.Run("SDK_UpdateTemplateVersion", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_UpdateTemplateVersion", func(t *testing.T) {
		t.Skip("TODO: implement — PUT /v3/{domain}/templates/{name}/versions/{tag}")
	})

	t.Run("SDK_DeleteTemplateVersion", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_DeleteTemplateVersion", func(t *testing.T) {
		t.Skip("TODO: implement — DELETE /v3/{domain}/templates/{name}/versions/{tag}")
	})
}
