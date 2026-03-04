package integration

import "testing"

func TestSubaccounts(t *testing.T) {
	resetServer(t)

	t.Run("SDK_CreateSubaccount", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_CreateSubaccount", func(t *testing.T) {
		t.Skip("TODO: implement — POST /v5/accounts/subaccounts")
	})

	t.Run("SDK_ListSubaccounts", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_ListSubaccounts", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v5/accounts/subaccounts")
	})

	t.Run("SDK_GetSubaccount", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_GetSubaccount", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v5/accounts/subaccounts/{id}")
	})

	t.Run("SDK_EnableSubaccount", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_EnableSubaccount", func(t *testing.T) {
		t.Skip("TODO: implement — POST /v5/accounts/subaccounts/{id}/enable")
	})

	t.Run("SDK_DisableSubaccount", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_DisableSubaccount", func(t *testing.T) {
		t.Skip("TODO: implement — POST /v5/accounts/subaccounts/{id}/disable")
	})

	t.Run("HTTP_OnBehalfOfHeader", func(t *testing.T) {
		t.Skip("TODO: implement — verify X-Mailgun-On-Behalf-Of header scoping")
	})
}
