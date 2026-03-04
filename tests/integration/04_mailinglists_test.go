package integration

import "testing"

func TestMailingLists(t *testing.T) {
	resetServer(t)

	// --- Mailing List CRUD ---

	t.Run("SDK_CreateMailingList", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_CreateMailingList", func(t *testing.T) {
		t.Skip("TODO: implement — POST /v3/lists")
	})

	t.Run("SDK_ListMailingListsOffset", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_ListMailingListsOffset", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/lists")
	})

	t.Run("SDK_ListMailingListsCursor", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_ListMailingListsCursor", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/lists/pages")
	})

	t.Run("SDK_GetMailingList", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_GetMailingList", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/lists/{address}")
	})

	t.Run("SDK_UpdateMailingList", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_UpdateMailingList", func(t *testing.T) {
		t.Skip("TODO: implement — PUT /v3/lists/{address}")
	})

	t.Run("SDK_DeleteMailingList", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_DeleteMailingList", func(t *testing.T) {
		t.Skip("TODO: implement — DELETE /v3/lists/{address}")
	})

	// --- Mailing List Members ---

	t.Run("SDK_CreateMember", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_CreateMember", func(t *testing.T) {
		t.Skip("TODO: implement — POST /v3/lists/{address}/members")
	})

	t.Run("SDK_ListMembersOffset", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_ListMembersOffset", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/lists/{address}/members")
	})

	t.Run("SDK_ListMembersCursor", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_ListMembersCursor", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/lists/{address}/members/pages")
	})

	t.Run("SDK_GetMember", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_GetMember", func(t *testing.T) {
		t.Skip("TODO: implement — GET /v3/lists/{address}/members/{member}")
	})

	t.Run("SDK_UpdateMember", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_UpdateMember", func(t *testing.T) {
		t.Skip("TODO: implement — PUT /v3/lists/{address}/members/{member}")
	})

	t.Run("SDK_DeleteMember", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_DeleteMember", func(t *testing.T) {
		t.Skip("TODO: implement — DELETE /v3/lists/{address}/members/{member}")
	})

	t.Run("SDK_CreateMemberList", func(t *testing.T) {
		t.Skip("TODO: implement")
	})
	t.Run("HTTP_BulkAddMembers", func(t *testing.T) {
		t.Skip("TODO: implement — POST /v3/lists/{address}/members.json")
	})
}
