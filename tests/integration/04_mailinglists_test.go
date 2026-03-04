package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/mailgun/mailgun-go/v5"
	"github.com/mailgun/mailgun-go/v5/mtypes"
)

const testList = "test-list@example.com"
const testMember = "member1@example.com"

func TestMailingLists(t *testing.T) {
	resetServer(t)

	ctx := context.Background()

	// --- Mailing List CRUD ---

	t.Run("SDK_CreateMailingList", func(t *testing.T) {
		mg := newMailgunClient()

		list, err := mg.CreateMailingList(ctx, mtypes.MailingList{
			Address:     testList,
			Name:        "Test List",
			Description: "A test mailing list",
			AccessLevel: mtypes.AccessLevelReadOnly,
		})

		passed := true
		errMsg := ""
		if err != nil {
			t.Fatalf("CreateMailingList failed: %v", err)
		}
		if list.Address != testList {
			t.Errorf("expected address %q, got %q", testList, list.Address)
		}

		if t.Failed() {
			passed = false
			errMsg = "assertion failed"
		}
		reporter.Record("MailingLists", "CreateMailingList", "SDK", passed, errMsg)
	})

	t.Run("HTTP_CreateMailingList", func(t *testing.T) {
		resp, err := doFormRequest("POST", "/v3/lists", map[string]string{
			"address":     "http-list@example.com",
			"name":        "HTTP List",
			"description": "Created via HTTP",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "Mailing list has been created" {
			t.Errorf("expected message 'Mailing list has been created', got %v", result["message"])
		}

		list, ok := result["list"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected 'list' object in response")
		}
		if list["address"] != "http-list@example.com" {
			t.Errorf("expected address 'http-list@example.com', got %v", list["address"])
		}

		reporter.Record("MailingLists", "CreateMailingList", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_ListMailingListsOffset", func(t *testing.T) {
		// The SDK's ListMailingLists actually hits the /pages endpoint (cursor-based).
		// For offset-based listing, we test via HTTP. Use SDK here to just verify listing works.
		mg := newMailgunClient()

		iter := mg.ListMailingLists(&mailgun.ListOptions{Limit: 10})

		var lists []mtypes.MailingList
		if !iter.Next(ctx, &lists) {
			if iter.Err() != nil {
				t.Fatalf("ListMailingLists iteration failed: %v", iter.Err())
			}
			t.Fatalf("expected at least one page of mailing lists")
		}

		if len(lists) < 2 {
			t.Errorf("expected at least 2 mailing lists, got %d", len(lists))
		}

		reporter.Record("MailingLists", "ListMailingListsOffset", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_ListMailingListsOffset", func(t *testing.T) {
		resp, err := doRequest("GET", "/v3/lists", nil)
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
			t.Errorf("expected at least 2 mailing lists, got %d", len(items))
		}

		totalCount, ok := result["total_count"].(float64)
		if !ok {
			t.Errorf("expected 'total_count' in response")
		} else if int(totalCount) < 2 {
			t.Errorf("expected total_count >= 2, got %v", totalCount)
		}

		reporter.Record("MailingLists", "ListMailingListsOffset", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_ListMailingListsCursor", func(t *testing.T) {
		mg := newMailgunClient()

		iter := mg.ListMailingLists(&mailgun.ListOptions{Limit: 10})

		var lists []mtypes.MailingList
		if !iter.Next(ctx, &lists) {
			if iter.Err() != nil {
				t.Fatalf("ListMailingLists cursor iteration failed: %v", iter.Err())
			}
			t.Fatalf("expected at least one page of mailing lists")
		}

		found := false
		for _, l := range lists {
			if l.Address == testList {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected to find %q in cursor listing", testList)
		}

		reporter.Record("MailingLists", "ListMailingListsCursor", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_ListMailingListsCursor", func(t *testing.T) {
		resp, err := doRequest("GET", "/v3/lists/pages", nil)
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
			t.Errorf("expected at least 2 mailing lists, got %d", len(items))
		}

		_, hasPaging := result["paging"]
		if !hasPaging {
			t.Errorf("expected 'paging' object in response")
		}

		reporter.Record("MailingLists", "ListMailingListsCursor", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_GetMailingList", func(t *testing.T) {
		mg := newMailgunClient()

		list, err := mg.GetMailingList(ctx, testList)
		if err != nil {
			t.Fatalf("GetMailingList failed: %v", err)
		}

		if list.Address != testList {
			t.Errorf("expected address %q, got %q", testList, list.Address)
		}
		if list.Name != "Test List" {
			t.Errorf("expected name 'Test List', got %q", list.Name)
		}
		if list.Description != "A test mailing list" {
			t.Errorf("expected description 'A test mailing list', got %q", list.Description)
		}

		reporter.Record("MailingLists", "GetMailingList", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_GetMailingList", func(t *testing.T) {
		resp, err := doRequest("GET", "/v3/lists/"+testList, nil)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		list, ok := result["list"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected 'list' object in response")
		}
		if list["address"] != testList {
			t.Errorf("expected address %q, got %v", testList, list["address"])
		}
		if list["name"] != "Test List" {
			t.Errorf("expected name 'Test List', got %v", list["name"])
		}

		reporter.Record("MailingLists", "GetMailingList", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_UpdateMailingList", func(t *testing.T) {
		mg := newMailgunClient()

		updated, err := mg.UpdateMailingList(ctx, testList, mtypes.MailingList{
			Name:        "Updated Test List",
			Description: "Updated description",
		})
		if err != nil {
			t.Fatalf("UpdateMailingList failed: %v", err)
		}

		if updated.Name != "Updated Test List" {
			t.Errorf("expected name 'Updated Test List', got %q", updated.Name)
		}

		reporter.Record("MailingLists", "UpdateMailingList", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_UpdateMailingList", func(t *testing.T) {
		resp, err := doFormRequest("PUT", "/v3/lists/"+testList, map[string]string{
			"description": "HTTP updated description",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "Mailing list has been updated" {
			t.Errorf("expected message 'Mailing list has been updated', got %v", result["message"])
		}

		list, ok := result["list"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected 'list' object in response")
		}
		if list["address"] != testList {
			t.Errorf("expected address %q, got %v", testList, list["address"])
		}

		reporter.Record("MailingLists", "UpdateMailingList", "HTTP", !t.Failed(), "")
	})

	// --- Mailing List Members ---
	// Create members before testing delete of lists

	t.Run("SDK_CreateMember", func(t *testing.T) {
		mg := newMailgunClient()

		subscribed := true
		err := mg.CreateMember(ctx, false, testList, mtypes.Member{
			Address:    testMember,
			Name:       "Member One",
			Subscribed: &subscribed,
			Vars:       map[string]any{"role": "admin"},
		})
		if err != nil {
			t.Fatalf("CreateMember failed: %v", err)
		}

		reporter.Record("MailingLists", "CreateMember", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_CreateMember", func(t *testing.T) {
		resp, err := doFormRequest("POST", "/v3/lists/"+testList+"/members", map[string]string{
			"address":    "member2@example.com",
			"name":       "Member Two",
			"subscribed": "yes",
			"vars":       `{"role": "user"}`,
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "Mailing list member has been created" {
			t.Errorf("expected message 'Mailing list member has been created', got %v", result["message"])
		}

		member, ok := result["member"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected 'member' object in response")
		}
		if member["address"] != "member2@example.com" {
			t.Errorf("expected address 'member2@example.com', got %v", member["address"])
		}

		reporter.Record("MailingLists", "CreateMember", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_ListMembersOffset", func(t *testing.T) {
		mg := newMailgunClient()

		iter := mg.ListMembers(testList, &mailgun.ListOptions{Limit: 10})

		var members []mtypes.Member
		if !iter.Next(ctx, &members) {
			if iter.Err() != nil {
				t.Fatalf("ListMembers iteration failed: %v", iter.Err())
			}
			t.Fatalf("expected at least one page of members")
		}

		if len(members) < 2 {
			t.Errorf("expected at least 2 members, got %d", len(members))
		}

		reporter.Record("MailingLists", "ListMembersOffset", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_ListMembersOffset", func(t *testing.T) {
		resp, err := doRequest("GET", "/v3/lists/"+testList+"/members", nil)
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
			t.Errorf("expected at least 2 members, got %d", len(items))
		}

		totalCount, ok := result["total_count"].(float64)
		if !ok {
			t.Errorf("expected 'total_count' in response")
		} else if int(totalCount) < 2 {
			t.Errorf("expected total_count >= 2, got %v", totalCount)
		}

		reporter.Record("MailingLists", "ListMembersOffset", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_ListMembersCursor", func(t *testing.T) {
		mg := newMailgunClient()

		iter := mg.ListMembers(testList, &mailgun.ListOptions{Limit: 10})

		var members []mtypes.Member
		if !iter.Next(ctx, &members) {
			if iter.Err() != nil {
				t.Fatalf("ListMembers cursor iteration failed: %v", iter.Err())
			}
			t.Fatalf("expected at least one page of members")
		}

		found := false
		for _, m := range members {
			if m.Address == testMember {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected to find %q in cursor member listing", testMember)
		}

		reporter.Record("MailingLists", "ListMembersCursor", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_ListMembersCursor", func(t *testing.T) {
		resp, err := doRequest("GET", "/v3/lists/"+testList+"/members/pages", nil)
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
			t.Errorf("expected at least 2 members, got %d", len(items))
		}

		_, hasPaging := result["paging"]
		if !hasPaging {
			t.Errorf("expected 'paging' object in response")
		}

		reporter.Record("MailingLists", "ListMembersCursor", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_GetMember", func(t *testing.T) {
		mg := newMailgunClient()

		member, err := mg.GetMember(ctx, testMember, testList)
		if err != nil {
			t.Fatalf("GetMember failed: %v", err)
		}

		if member.Address != testMember {
			t.Errorf("expected address %q, got %q", testMember, member.Address)
		}
		if member.Name != "Member One" {
			t.Errorf("expected name 'Member One', got %q", member.Name)
		}

		reporter.Record("MailingLists", "GetMember", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_GetMember", func(t *testing.T) {
		resp, err := doRequest("GET", "/v3/lists/"+testList+"/members/"+testMember, nil)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		member, ok := result["member"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected 'member' object in response")
		}
		if member["address"] != testMember {
			t.Errorf("expected address %q, got %v", testMember, member["address"])
		}

		reporter.Record("MailingLists", "GetMember", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_UpdateMember", func(t *testing.T) {
		mg := newMailgunClient()

		updated, err := mg.UpdateMember(ctx, testMember, testList, mtypes.Member{
			Name: "Updated Member One",
		})
		if err != nil {
			t.Fatalf("UpdateMember failed: %v", err)
		}

		if updated.Name != "Updated Member One" {
			t.Errorf("expected name 'Updated Member One', got %q", updated.Name)
		}

		reporter.Record("MailingLists", "UpdateMember", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_UpdateMember", func(t *testing.T) {
		resp, err := doFormRequest("PUT", "/v3/lists/"+testList+"/members/member2@example.com", map[string]string{
			"name": "Updated Member Two",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "Mailing list member has been updated" {
			t.Errorf("expected message 'Mailing list member has been updated', got %v", result["message"])
		}

		member, ok := result["member"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected 'member' object in response")
		}
		if member["name"] != "Updated Member Two" {
			t.Errorf("expected name 'Updated Member Two', got %v", member["name"])
		}

		reporter.Record("MailingLists", "UpdateMember", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_DeleteMember", func(t *testing.T) {
		mg := newMailgunClient()

		err := mg.DeleteMember(ctx, testMember, testList)
		if err != nil {
			t.Fatalf("DeleteMember failed: %v", err)
		}

		// Verify member is gone
		_, err = mg.GetMember(ctx, testMember, testList)
		if err == nil {
			t.Errorf("expected error getting deleted member, but got nil")
		}

		reporter.Record("MailingLists", "DeleteMember", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_DeleteMember", func(t *testing.T) {
		resp, err := doRequest("DELETE", "/v3/lists/"+testList+"/members/member2@example.com", nil)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "Mailing list member has been deleted" {
			t.Errorf("expected message 'Mailing list member has been deleted', got %v", result["message"])
		}

		member, ok := result["member"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected 'member' object in response")
		}
		if member["address"] != "member2@example.com" {
			t.Errorf("expected address 'member2@example.com', got %v", member["address"])
		}

		reporter.Record("MailingLists", "DeleteMember", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_CreateMemberList", func(t *testing.T) {
		mg := newMailgunClient()

		upsert := true
		newMembers := []any{
			mtypes.Member{Address: "bulk1@example.com", Name: "Bulk One"},
			mtypes.Member{Address: "bulk2@example.com", Name: "Bulk Two"},
			mtypes.Member{Address: "bulk3@example.com", Name: "Bulk Three"},
		}

		err := mg.CreateMemberList(ctx, &upsert, testList, newMembers)
		if err != nil {
			t.Fatalf("CreateMemberList failed: %v", err)
		}

		reporter.Record("MailingLists", "CreateMemberList", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_BulkAddMembers", func(t *testing.T) {
		members := []map[string]interface{}{
			{"address": "httpbulk1@example.com", "name": "HTTP Bulk One"},
			{"address": "httpbulk2@example.com", "name": "HTTP Bulk Two"},
		}
		membersJSON, _ := json.Marshal(members)

		resp, err := doFormRequest("POST", "/v3/lists/"+testList+"/members.json", map[string]string{
			"members": string(membersJSON),
			"upsert":  "yes",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "Mailing list has been updated" {
			t.Errorf("expected message 'Mailing list has been updated', got %v", result["message"])
		}

		_, hasList := result["list"]
		if !hasList {
			t.Errorf("expected 'list' object in response")
		}

		reporter.Record("MailingLists", "BulkAddMembers", "HTTP", !t.Failed(), "")
	})

	// --- Delete mailing lists (last) ---

	t.Run("SDK_DeleteMailingList", func(t *testing.T) {
		mg := newMailgunClient()

		err := mg.DeleteMailingList(ctx, "http-list@example.com")
		if err != nil {
			t.Fatalf("DeleteMailingList failed: %v", err)
		}

		reporter.Record("MailingLists", "DeleteMailingList", "SDK", !t.Failed(), "")
	})

	t.Run("HTTP_DeleteMailingList", func(t *testing.T) {
		resp, err := doRequest("DELETE", "/v3/lists/"+testList, nil)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "Mailing list has been removed" {
			t.Errorf("expected message 'Mailing list has been removed', got %v", result["message"])
		}
		if result["address"] != testList {
			t.Errorf("expected address %q, got %v", testList, result["address"])
		}

		reporter.Record("MailingLists", "DeleteMailingList", "HTTP", !t.Failed(), "")
	})
}

// Ensure imports are used.
var _ mailgun.ListOptions
var _ mtypes.MailingList
