package integration

import "testing"

func TestRoutes(t *testing.T) {
	resetServer(t)

	t.Run("SDK_CreateRoute", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_CreateRoute", func(t *testing.T) {
		t.Skip("TODO: implement — POST /v3/routes")
	})

	t.Run("SDK_ListRoutes", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_ListRoutes", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/routes")
	})

	t.Run("SDK_GetRoute", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_GetRoute", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/routes/{id}")
	})

	t.Run("SDK_UpdateRoute", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_UpdateRoute", func(t *testing.T) {
		t.Skip("TODO: implement — PUT /v3/routes/{id}")
	})

	t.Run("SDK_DeleteRoute", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_DeleteRoute", func(t *testing.T) {
		t.Skip("TODO: implement — DELETE /v3/routes/{id}")
	})
}
