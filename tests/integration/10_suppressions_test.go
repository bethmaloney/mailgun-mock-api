package integration

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/mailgun/mailgun-go/v5"
	"github.com/mailgun/mailgun-go/v5/mtypes"
)

func TestSuppressions(t *testing.T) {
	resetServer(t)

	const domain = "suppressions-test.example.com"

	// Setup: create a domain
	resp, err := doFormRequest("POST", "/v4/domains", map[string]string{"name": domain})
	if err != nil {
		t.Fatalf("setup: create domain: %v", err)
	}
	resp.Body.Close()

	// --- Bounces ---

	t.Run("SDK_AddBounce", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		err := mg.AddBounce(ctx, domain, "bounce1@example.com", "550", "mailbox unavailable")
		if err != nil {
			reporter.Record("Suppressions", "AddBounce", "SDK", false, err.Error())
			t.Fatalf("AddBounce: %v", err)
		}

		reporter.Record("Suppressions", "AddBounce", "SDK", !t.Failed(), "")
	})
	t.Run("HTTP_AddBounce", func(t *testing.T) {
		resp, err := doFormRequest("POST", "/v3/"+domain+"/bounces", map[string]string{
			"address": "bounce2@example.com",
			"code":    "552",
			"error":   "mailbox full",
		})
		if err != nil {
			reporter.Record("Suppressions", "AddBounce", "HTTP", false, err.Error())
			t.Fatalf("POST bounces: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if msg, _ := result["message"].(string); msg != "1 addresses have been added to the bounces table" {
			t.Errorf("unexpected message: %v", result["message"])
		}

		reporter.Record("Suppressions", "AddBounce", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_ListBounces", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		it := mg.ListBounces(domain, &mailgun.ListOptions{Limit: 10})
		var page []mtypes.Bounce
		found := it.Next(ctx, &page)
		if !found && it.Err() != nil {
			reporter.Record("Suppressions", "ListBounces", "SDK", false, it.Err().Error())
			t.Fatalf("ListBounces: %v", it.Err())
		}

		if len(page) < 2 {
			reporter.Record("Suppressions", "ListBounces", "SDK", false, fmt.Sprintf("expected >= 2 bounces, got %d", len(page)))
			t.Fatalf("expected >= 2 bounces, got %d", len(page))
		}

		reporter.Record("Suppressions", "ListBounces", "SDK", !t.Failed(), "")
	})
	t.Run("HTTP_ListBounces", func(t *testing.T) {
		resp, err := doRequest("GET", "/v3/"+domain+"/bounces", nil)
		if err != nil {
			reporter.Record("Suppressions", "ListBounces", "HTTP", false, err.Error())
			t.Fatalf("GET bounces: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result struct {
			Items  []map[string]interface{} `json:"items"`
			Paging map[string]interface{}   `json:"paging"`
		}
		readJSON(t, resp, &result)

		if len(result.Items) < 2 {
			t.Fatalf("expected >= 2 bounces, got %d", len(result.Items))
		}

		// Verify items have expected keys
		for _, item := range result.Items {
			if _, ok := item["address"]; !ok {
				t.Error("bounce item missing 'address' key")
			}
			if _, ok := item["code"]; !ok {
				t.Error("bounce item missing 'code' key")
			}
			if _, ok := item["created_at"]; !ok {
				t.Error("bounce item missing 'created_at' key")
			}
		}

		reporter.Record("Suppressions", "ListBounces", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_GetBounce", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		bounce, err := mg.GetBounce(ctx, domain, "bounce1@example.com")
		if err != nil {
			reporter.Record("Suppressions", "GetBounce", "SDK", false, err.Error())
			t.Fatalf("GetBounce: %v", err)
		}

		if bounce.Address != "bounce1@example.com" {
			t.Errorf("expected address bounce1@example.com, got %q", bounce.Address)
		}
		if bounce.Code != "550" {
			t.Errorf("expected code 550, got %q", bounce.Code)
		}

		reporter.Record("Suppressions", "GetBounce", "SDK", !t.Failed(), "")
	})
	t.Run("HTTP_GetBounce", func(t *testing.T) {
		resp, err := doRequest("GET", "/v3/"+domain+"/bounces/bounce2@example.com", nil)
		if err != nil {
			reporter.Record("Suppressions", "GetBounce", "HTTP", false, err.Error())
			t.Fatalf("GET bounce: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["address"] != "bounce2@example.com" {
			t.Errorf("expected address bounce2@example.com, got %v", result["address"])
		}
		if result["code"] != "552" {
			t.Errorf("expected code 552, got %v", result["code"])
		}

		reporter.Record("Suppressions", "GetBounce", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_DeleteBounce", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		err := mg.DeleteBounce(ctx, domain, "bounce1@example.com")
		if err != nil {
			reporter.Record("Suppressions", "DeleteBounce", "SDK", false, err.Error())
			t.Fatalf("DeleteBounce: %v", err)
		}

		// Verify it's gone
		_, err = mg.GetBounce(ctx, domain, "bounce1@example.com")
		if err == nil {
			t.Error("expected error getting deleted bounce, got nil")
		}

		reporter.Record("Suppressions", "DeleteBounce", "SDK", !t.Failed(), "")
	})
	t.Run("HTTP_DeleteBounce", func(t *testing.T) {
		resp, err := doRequest("DELETE", "/v3/"+domain+"/bounces/bounce2@example.com", nil)
		if err != nil {
			reporter.Record("Suppressions", "DeleteBounce", "HTTP", false, err.Error())
			t.Fatalf("DELETE bounce: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "Bounced addresses for this domain have been removed" {
			t.Errorf("unexpected message: %v", result["message"])
		}
		if result["address"] != "bounce2@example.com" {
			t.Errorf("expected address bounce2@example.com, got %v", result["address"])
		}

		// Verify it's gone
		resp2, err := doRequest("GET", "/v3/"+domain+"/bounces/bounce2@example.com", nil)
		if err != nil {
			t.Fatalf("GET deleted bounce: %v", err)
		}
		if resp2.StatusCode != http.StatusNotFound {
			resp2.Body.Close()
			t.Errorf("expected 404 for deleted bounce, got %d", resp2.StatusCode)
		} else {
			resp2.Body.Close()
		}

		reporter.Record("Suppressions", "DeleteBounce", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_DeleteBounceList", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		// Add some bounces first
		_ = mg.AddBounce(ctx, domain, "del1@example.com", "550", "test")
		_ = mg.AddBounce(ctx, domain, "del2@example.com", "550", "test")

		err := mg.DeleteBounceList(ctx, domain)
		if err != nil {
			reporter.Record("Suppressions", "DeleteBounceList", "SDK", false, err.Error())
			t.Fatalf("DeleteBounceList: %v", err)
		}

		// Verify all bounces are gone
		it := mg.ListBounces(domain, &mailgun.ListOptions{Limit: 10})
		var page []mtypes.Bounce
		found := it.Next(ctx, &page)
		if found && len(page) > 0 {
			t.Errorf("expected 0 bounces after delete all, got %d", len(page))
		}

		reporter.Record("Suppressions", "DeleteBounceList", "SDK", !t.Failed(), "")
	})
	t.Run("HTTP_DeleteAllBounces", func(t *testing.T) {
		// Add some bounces first
		doFormRequest("POST", "/v3/"+domain+"/bounces", map[string]string{
			"address": "delhttp1@example.com",
			"code":    "550",
		})
		doFormRequest("POST", "/v3/"+domain+"/bounces", map[string]string{
			"address": "delhttp2@example.com",
			"code":    "550",
		})

		resp, err := doRequest("DELETE", "/v3/"+domain+"/bounces", nil)
		if err != nil {
			reporter.Record("Suppressions", "DeleteAllBounces", "HTTP", false, err.Error())
			t.Fatalf("DELETE all bounces: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "Bounced addresses for this domain have been removed" {
			t.Errorf("unexpected message: %v", result["message"])
		}

		// Verify all gone
		resp2, err := doRequest("GET", "/v3/"+domain+"/bounces", nil)
		if err != nil {
			t.Fatalf("GET bounces after delete all: %v", err)
		}
		var listResult struct {
			Items []map[string]interface{} `json:"items"`
		}
		readJSON(t, resp2, &listResult)
		if len(listResult.Items) != 0 {
			t.Errorf("expected 0 bounces after delete all, got %d", len(listResult.Items))
		}

		reporter.Record("Suppressions", "DeleteAllBounces", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_BulkImportBounces", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		bounces := []mtypes.Bounce{
			{Address: "bulk1@example.com", Code: "550", Error: "not found"},
			{Address: "bulk2@example.com", Code: "551", Error: "rejected"},
			{Address: "bulk3@example.com", Code: "552", Error: "full"},
		}

		err := mg.AddBounces(ctx, domain, bounces)
		if err != nil {
			reporter.Record("Suppressions", "BulkImportBounces", "SDK", false, err.Error())
			t.Fatalf("AddBounces: %v", err)
		}

		// Verify they exist
		it := mg.ListBounces(domain, &mailgun.ListOptions{Limit: 10})
		var page []mtypes.Bounce
		it.Next(ctx, &page)
		if len(page) < 3 {
			t.Errorf("expected >= 3 bounces after bulk import, got %d", len(page))
		}

		reporter.Record("Suppressions", "BulkImportBounces", "SDK", !t.Failed(), "")
	})
	t.Run("HTTP_BulkImportBounces", func(t *testing.T) {
		// Clear first
		doRequest("DELETE", "/v3/"+domain+"/bounces", nil)

		body := []map[string]string{
			{"address": "httpbulk1@example.com", "code": "550", "error": "not found"},
			{"address": "httpbulk2@example.com", "code": "551", "error": "rejected"},
		}

		resp, err := doRequest("POST", "/v3/"+domain+"/bounces", body)
		if err != nil {
			reporter.Record("Suppressions", "BulkImportBounces", "HTTP", false, err.Error())
			t.Fatalf("POST bulk bounces: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if msg, _ := result["message"].(string); msg != "2 addresses have been added to the bounces table" {
			t.Errorf("unexpected message: %v", result["message"])
		}

		reporter.Record("Suppressions", "BulkImportBounces", "HTTP", !t.Failed(), "")
	})

	// --- Complaints ---

	t.Run("SDK_CreateComplaint", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		err := mg.CreateComplaint(ctx, domain, "complaint1@example.com")
		if err != nil {
			reporter.Record("Suppressions", "CreateComplaint", "SDK", false, err.Error())
			t.Fatalf("CreateComplaint: %v", err)
		}

		reporter.Record("Suppressions", "CreateComplaint", "SDK", !t.Failed(), "")
	})
	t.Run("HTTP_CreateComplaint", func(t *testing.T) {
		resp, err := doFormRequest("POST", "/v3/"+domain+"/complaints", map[string]string{
			"address": "complaint2@example.com",
		})
		if err != nil {
			reporter.Record("Suppressions", "CreateComplaint", "HTTP", false, err.Error())
			t.Fatalf("POST complaints: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if msg, _ := result["message"].(string); msg != "1 addresses have been added to the complaints table" {
			t.Errorf("unexpected message: %v", result["message"])
		}

		reporter.Record("Suppressions", "CreateComplaint", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_ListComplaints", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		it := mg.ListComplaints(domain, &mailgun.ListOptions{Limit: 10})
		var page []mtypes.Complaint
		found := it.Next(ctx, &page)
		if !found && it.Err() != nil {
			reporter.Record("Suppressions", "ListComplaints", "SDK", false, it.Err().Error())
			t.Fatalf("ListComplaints: %v", it.Err())
		}

		if len(page) < 2 {
			reporter.Record("Suppressions", "ListComplaints", "SDK", false, fmt.Sprintf("expected >= 2 complaints, got %d", len(page)))
			t.Fatalf("expected >= 2 complaints, got %d", len(page))
		}

		reporter.Record("Suppressions", "ListComplaints", "SDK", !t.Failed(), "")
	})
	t.Run("HTTP_ListComplaints", func(t *testing.T) {
		resp, err := doRequest("GET", "/v3/"+domain+"/complaints", nil)
		if err != nil {
			reporter.Record("Suppressions", "ListComplaints", "HTTP", false, err.Error())
			t.Fatalf("GET complaints: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result struct {
			Items  []map[string]interface{} `json:"items"`
			Paging map[string]interface{}   `json:"paging"`
		}
		readJSON(t, resp, &result)

		if len(result.Items) < 2 {
			t.Fatalf("expected >= 2 complaints, got %d", len(result.Items))
		}

		for _, item := range result.Items {
			if _, ok := item["address"]; !ok {
				t.Error("complaint item missing 'address' key")
			}
			if _, ok := item["created_at"]; !ok {
				t.Error("complaint item missing 'created_at' key")
			}
		}

		reporter.Record("Suppressions", "ListComplaints", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_GetComplaint", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		complaint, err := mg.GetComplaint(ctx, domain, "complaint1@example.com")
		if err != nil {
			reporter.Record("Suppressions", "GetComplaint", "SDK", false, err.Error())
			t.Fatalf("GetComplaint: %v", err)
		}

		if complaint.Address != "complaint1@example.com" {
			t.Errorf("expected address complaint1@example.com, got %q", complaint.Address)
		}

		reporter.Record("Suppressions", "GetComplaint", "SDK", !t.Failed(), "")
	})
	t.Run("HTTP_GetComplaint", func(t *testing.T) {
		resp, err := doRequest("GET", "/v3/"+domain+"/complaints/complaint2@example.com", nil)
		if err != nil {
			reporter.Record("Suppressions", "GetComplaint", "HTTP", false, err.Error())
			t.Fatalf("GET complaint: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["address"] != "complaint2@example.com" {
			t.Errorf("expected address complaint2@example.com, got %v", result["address"])
		}

		reporter.Record("Suppressions", "GetComplaint", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_DeleteComplaint", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		err := mg.DeleteComplaint(ctx, domain, "complaint1@example.com")
		if err != nil {
			reporter.Record("Suppressions", "DeleteComplaint", "SDK", false, err.Error())
			t.Fatalf("DeleteComplaint: %v", err)
		}

		// Verify it's gone
		_, err = mg.GetComplaint(ctx, domain, "complaint1@example.com")
		if err == nil {
			t.Error("expected error getting deleted complaint, got nil")
		}

		reporter.Record("Suppressions", "DeleteComplaint", "SDK", !t.Failed(), "")
	})
	t.Run("HTTP_DeleteComplaint", func(t *testing.T) {
		resp, err := doRequest("DELETE", "/v3/"+domain+"/complaints/complaint2@example.com", nil)
		if err != nil {
			reporter.Record("Suppressions", "DeleteComplaint", "HTTP", false, err.Error())
			t.Fatalf("DELETE complaint: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "Complaint addresses for this domain have been removed" {
			t.Errorf("unexpected message: %v", result["message"])
		}
		if result["address"] != "complaint2@example.com" {
			t.Errorf("expected address complaint2@example.com, got %v", result["address"])
		}

		// Verify it's gone
		resp2, err := doRequest("GET", "/v3/"+domain+"/complaints/complaint2@example.com", nil)
		if err != nil {
			t.Fatalf("GET deleted complaint: %v", err)
		}
		if resp2.StatusCode != http.StatusNotFound {
			resp2.Body.Close()
			t.Errorf("expected 404 for deleted complaint, got %d", resp2.StatusCode)
		} else {
			resp2.Body.Close()
		}

		reporter.Record("Suppressions", "DeleteComplaint", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_BulkImportComplaints", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		addresses := []string{
			"bulkcomplaint1@example.com",
			"bulkcomplaint2@example.com",
			"bulkcomplaint3@example.com",
		}

		err := mg.CreateComplaints(ctx, domain, addresses)
		if err != nil {
			reporter.Record("Suppressions", "BulkImportComplaints", "SDK", false, err.Error())
			t.Fatalf("CreateComplaints: %v", err)
		}

		// Verify they exist
		it := mg.ListComplaints(domain, &mailgun.ListOptions{Limit: 10})
		var page []mtypes.Complaint
		it.Next(ctx, &page)
		if len(page) < 3 {
			t.Errorf("expected >= 3 complaints after bulk import, got %d", len(page))
		}

		reporter.Record("Suppressions", "BulkImportComplaints", "SDK", !t.Failed(), "")
	})
	t.Run("HTTP_BulkImportComplaints", func(t *testing.T) {
		// Clear existing complaints by deleting individually or use delete all
		doRequest("DELETE", "/v3/"+domain+"/complaints", nil)

		body := []map[string]string{
			{"address": "httpbulkcomplaint1@example.com"},
			{"address": "httpbulkcomplaint2@example.com"},
		}

		resp, err := doRequest("POST", "/v3/"+domain+"/complaints", body)
		if err != nil {
			reporter.Record("Suppressions", "BulkImportComplaints", "HTTP", false, err.Error())
			t.Fatalf("POST bulk complaints: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if msg, _ := result["message"].(string); msg != "2 addresses have been added to the complaints table" {
			t.Errorf("unexpected message: %v", result["message"])
		}

		reporter.Record("Suppressions", "BulkImportComplaints", "HTTP", !t.Failed(), "")
	})

	// --- Unsubscribes ---

	t.Run("SDK_CreateUnsubscribe", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		err := mg.CreateUnsubscribe(ctx, domain, "unsub1@example.com", "newsletter")
		if err != nil {
			reporter.Record("Suppressions", "CreateUnsubscribe", "SDK", false, err.Error())
			t.Fatalf("CreateUnsubscribe: %v", err)
		}

		reporter.Record("Suppressions", "CreateUnsubscribe", "SDK", !t.Failed(), "")
	})
	t.Run("HTTP_CreateUnsubscribe", func(t *testing.T) {
		resp, err := doFormRequest("POST", "/v3/"+domain+"/unsubscribes", map[string]string{
			"address": "unsub2@example.com",
			"tag":     "promo",
		})
		if err != nil {
			reporter.Record("Suppressions", "CreateUnsubscribe", "HTTP", false, err.Error())
			t.Fatalf("POST unsubscribes: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if msg, _ := result["message"].(string); msg != "1 addresses have been added to the unsubscribes table" {
			t.Errorf("unexpected message: %v", result["message"])
		}

		reporter.Record("Suppressions", "CreateUnsubscribe", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_ListUnsubscribes", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		it := mg.ListUnsubscribes(domain, &mailgun.ListOptions{Limit: 10})
		var page []mtypes.Unsubscribe
		found := it.Next(ctx, &page)
		if !found && it.Err() != nil {
			reporter.Record("Suppressions", "ListUnsubscribes", "SDK", false, it.Err().Error())
			t.Fatalf("ListUnsubscribes: %v", it.Err())
		}

		if len(page) < 2 {
			reporter.Record("Suppressions", "ListUnsubscribes", "SDK", false, fmt.Sprintf("expected >= 2 unsubscribes, got %d", len(page)))
			t.Fatalf("expected >= 2 unsubscribes, got %d", len(page))
		}

		reporter.Record("Suppressions", "ListUnsubscribes", "SDK", !t.Failed(), "")
	})
	t.Run("HTTP_ListUnsubscribes", func(t *testing.T) {
		resp, err := doRequest("GET", "/v3/"+domain+"/unsubscribes", nil)
		if err != nil {
			reporter.Record("Suppressions", "ListUnsubscribes", "HTTP", false, err.Error())
			t.Fatalf("GET unsubscribes: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result struct {
			Items  []map[string]interface{} `json:"items"`
			Paging map[string]interface{}   `json:"paging"`
		}
		readJSON(t, resp, &result)

		if len(result.Items) < 2 {
			t.Fatalf("expected >= 2 unsubscribes, got %d", len(result.Items))
		}

		for _, item := range result.Items {
			if _, ok := item["address"]; !ok {
				t.Error("unsubscribe item missing 'address' key")
			}
			if _, ok := item["created_at"]; !ok {
				t.Error("unsubscribe item missing 'created_at' key")
			}
		}

		reporter.Record("Suppressions", "ListUnsubscribes", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_GetUnsubscribe", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		unsub, err := mg.GetUnsubscribe(ctx, domain, "unsub1@example.com")
		if err != nil {
			reporter.Record("Suppressions", "GetUnsubscribe", "SDK", false, err.Error())
			t.Fatalf("GetUnsubscribe: %v", err)
		}

		if unsub.Address != "unsub1@example.com" {
			t.Errorf("expected address unsub1@example.com, got %q", unsub.Address)
		}

		reporter.Record("Suppressions", "GetUnsubscribe", "SDK", !t.Failed(), "")
	})
	t.Run("HTTP_GetUnsubscribe", func(t *testing.T) {
		resp, err := doRequest("GET", "/v3/"+domain+"/unsubscribes/unsub2@example.com", nil)
		if err != nil {
			reporter.Record("Suppressions", "GetUnsubscribe", "HTTP", false, err.Error())
			t.Fatalf("GET unsubscribe: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["address"] != "unsub2@example.com" {
			t.Errorf("expected address unsub2@example.com, got %v", result["address"])
		}

		reporter.Record("Suppressions", "GetUnsubscribe", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_DeleteUnsubscribe", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		err := mg.DeleteUnsubscribe(ctx, domain, "unsub1@example.com")
		if err != nil {
			reporter.Record("Suppressions", "DeleteUnsubscribe", "SDK", false, err.Error())
			t.Fatalf("DeleteUnsubscribe: %v", err)
		}

		// Verify it's gone
		_, err = mg.GetUnsubscribe(ctx, domain, "unsub1@example.com")
		if err == nil {
			t.Error("expected error getting deleted unsubscribe, got nil")
		}

		reporter.Record("Suppressions", "DeleteUnsubscribe", "SDK", !t.Failed(), "")
	})
	t.Run("HTTP_DeleteUnsubscribe", func(t *testing.T) {
		resp, err := doRequest("DELETE", "/v3/"+domain+"/unsubscribes/unsub2@example.com", nil)
		if err != nil {
			reporter.Record("Suppressions", "DeleteUnsubscribe", "HTTP", false, err.Error())
			t.Fatalf("DELETE unsubscribe: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "Unsubscribe addresses for this domain have been removed" {
			t.Errorf("unexpected message: %v", result["message"])
		}
		if result["address"] != "unsub2@example.com" {
			t.Errorf("expected address unsub2@example.com, got %v", result["address"])
		}

		// Verify it's gone
		resp2, err := doRequest("GET", "/v3/"+domain+"/unsubscribes/unsub2@example.com", nil)
		if err != nil {
			t.Fatalf("GET deleted unsubscribe: %v", err)
		}
		if resp2.StatusCode != http.StatusNotFound {
			resp2.Body.Close()
			t.Errorf("expected 404 for deleted unsubscribe, got %d", resp2.StatusCode)
		} else {
			resp2.Body.Close()
		}

		reporter.Record("Suppressions", "DeleteUnsubscribe", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_DeleteUnsubscribeWithTag", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		// Create an unsubscribe with a specific tag first
		err := mg.CreateUnsubscribe(ctx, domain, "tagdel@example.com", "special-tag")
		if err != nil {
			reporter.Record("Suppressions", "DeleteUnsubscribeWithTag", "SDK", false, err.Error())
			t.Fatalf("setup CreateUnsubscribe: %v", err)
		}

		err = mg.DeleteUnsubscribeWithTag(ctx, domain, "tagdel@example.com", "special-tag")
		if err != nil {
			reporter.Record("Suppressions", "DeleteUnsubscribeWithTag", "SDK", false, err.Error())
			t.Fatalf("DeleteUnsubscribeWithTag: %v", err)
		}

		// Verify it's gone
		_, err = mg.GetUnsubscribe(ctx, domain, "tagdel@example.com")
		if err == nil {
			t.Error("expected error getting deleted unsubscribe, got nil")
		}

		reporter.Record("Suppressions", "DeleteUnsubscribeWithTag", "SDK", !t.Failed(), "")
	})
	t.Run("HTTP_DeleteUnsubscribeWithTag", func(t *testing.T) {
		// Create an unsubscribe first
		doFormRequest("POST", "/v3/"+domain+"/unsubscribes", map[string]string{
			"address": "tagdelhttp@example.com",
			"tag":     "http-tag",
		})

		resp, err := doRequest("DELETE", "/v3/"+domain+"/unsubscribes/tagdelhttp@example.com?tag=http-tag", nil)
		if err != nil {
			reporter.Record("Suppressions", "DeleteUnsubscribeWithTag", "HTTP", false, err.Error())
			t.Fatalf("DELETE unsubscribe with tag: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["message"] != "Unsubscribe addresses for this domain have been removed" {
			t.Errorf("unexpected message: %v", result["message"])
		}

		// Verify it's gone
		resp2, err := doRequest("GET", "/v3/"+domain+"/unsubscribes/tagdelhttp@example.com", nil)
		if err != nil {
			t.Fatalf("GET deleted unsubscribe: %v", err)
		}
		if resp2.StatusCode != http.StatusNotFound {
			resp2.Body.Close()
			t.Errorf("expected 404 for deleted unsubscribe, got %d", resp2.StatusCode)
		} else {
			resp2.Body.Close()
		}

		reporter.Record("Suppressions", "DeleteUnsubscribeWithTag", "HTTP", !t.Failed(), "")
	})

	t.Run("SDK_BulkImportUnsubscribes", func(t *testing.T) {
		mg := newMailgunClient()
		ctx := context.Background()

		unsubs := []mtypes.Unsubscribe{
			{Address: "bulkunsub1@example.com", Tags: []string{"tag1"}},
			{Address: "bulkunsub2@example.com", Tags: []string{"tag2"}},
			{Address: "bulkunsub3@example.com", Tags: []string{"tag3"}},
		}

		err := mg.CreateUnsubscribes(ctx, domain, unsubs)
		if err != nil {
			reporter.Record("Suppressions", "BulkImportUnsubscribes", "SDK", false, err.Error())
			t.Fatalf("CreateUnsubscribes: %v", err)
		}

		// Verify they exist
		it := mg.ListUnsubscribes(domain, &mailgun.ListOptions{Limit: 10})
		var page []mtypes.Unsubscribe
		it.Next(ctx, &page)
		if len(page) < 3 {
			t.Errorf("expected >= 3 unsubscribes after bulk import, got %d", len(page))
		}

		reporter.Record("Suppressions", "BulkImportUnsubscribes", "SDK", !t.Failed(), "")
	})
	t.Run("HTTP_BulkImportUnsubscribes", func(t *testing.T) {
		// Clear existing
		doRequest("DELETE", "/v3/"+domain+"/unsubscribes", nil)

		body := []map[string]interface{}{
			{"address": "httpbulkunsub1@example.com", "tags": []string{"t1"}},
			{"address": "httpbulkunsub2@example.com", "tags": []string{"t2"}},
		}

		resp, err := doRequest("POST", "/v3/"+domain+"/unsubscribes", body)
		if err != nil {
			reporter.Record("Suppressions", "BulkImportUnsubscribes", "HTTP", false, err.Error())
			t.Fatalf("POST bulk unsubscribes: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if msg, _ := result["message"].(string); msg != "2 addresses have been added to the unsubscribes table" {
			t.Errorf("unexpected message: %v", result["message"])
		}

		reporter.Record("Suppressions", "BulkImportUnsubscribes", "HTTP", !t.Failed(), "")
	})

	// --- Allowlist (Whitelists) ---

	t.Run("HTTP_AddToAllowlist", func(t *testing.T) {
		// Add an address entry
		resp, err := doFormRequest("POST", "/v3/"+domain+"/whitelists", map[string]string{
			"address": "allowed@example.com",
			"reason":  "VIP customer",
		})
		if err != nil {
			reporter.Record("Suppressions", "AddToAllowlist", "HTTP", false, err.Error())
			t.Fatalf("POST whitelists (address): %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if msg, _ := result["message"].(string); msg != "1 addresses have been added to the whitelists table" {
			t.Errorf("unexpected message: %v", result["message"])
		}

		// Add a domain entry
		resp2, err := doFormRequest("POST", "/v3/"+domain+"/whitelists", map[string]string{
			"domain": "trusted.example.com",
			"reason": "Partner domain",
		})
		if err != nil {
			t.Fatalf("POST whitelists (domain): %v", err)
		}
		assertStatus(t, resp2, http.StatusOK)

		var result2 map[string]interface{}
		readJSON(t, resp2, &result2)

		if msg, _ := result2["message"].(string); msg != "1 addresses have been added to the whitelists table" {
			t.Errorf("unexpected message for domain entry: %v", result2["message"])
		}

		reporter.Record("Suppressions", "AddToAllowlist", "HTTP", !t.Failed(), "")
	})

	t.Run("HTTP_ListAllowlist", func(t *testing.T) {
		resp, err := doRequest("GET", "/v3/"+domain+"/whitelists", nil)
		if err != nil {
			reporter.Record("Suppressions", "ListAllowlist", "HTTP", false, err.Error())
			t.Fatalf("GET whitelists: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result struct {
			Items  []map[string]interface{} `json:"items"`
			Paging map[string]interface{}   `json:"paging"`
		}
		readJSON(t, resp, &result)

		if len(result.Items) < 2 {
			t.Fatalf("expected >= 2 allowlist entries, got %d", len(result.Items))
		}

		// Verify items have expected keys
		for _, item := range result.Items {
			if _, ok := item["value"]; !ok {
				t.Error("allowlist item missing 'value' key")
			}
			if _, ok := item["type"]; !ok {
				t.Error("allowlist item missing 'type' key")
			}
			if _, ok := item["createdAt"]; !ok {
				t.Error("allowlist item missing 'createdAt' key")
			}
		}

		reporter.Record("Suppressions", "ListAllowlist", "HTTP", !t.Failed(), "")
	})

	t.Run("HTTP_GetAllowlistEntry", func(t *testing.T) {
		resp, err := doRequest("GET", "/v3/"+domain+"/whitelists/allowed@example.com", nil)
		if err != nil {
			reporter.Record("Suppressions", "GetAllowlistEntry", "HTTP", false, err.Error())
			t.Fatalf("GET whitelist entry: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["value"] != "allowed@example.com" {
			t.Errorf("expected value allowed@example.com, got %v", result["value"])
		}
		if result["type"] != "address" {
			t.Errorf("expected type 'address', got %v", result["type"])
		}
		if result["reason"] != "VIP customer" {
			t.Errorf("expected reason 'VIP customer', got %v", result["reason"])
		}

		reporter.Record("Suppressions", "GetAllowlistEntry", "HTTP", !t.Failed(), "")
	})

	t.Run("HTTP_DeleteFromAllowlist", func(t *testing.T) {
		resp, err := doRequest("DELETE", "/v3/"+domain+"/whitelists/allowed@example.com", nil)
		if err != nil {
			reporter.Record("Suppressions", "DeleteFromAllowlist", "HTTP", false, err.Error())
			t.Fatalf("DELETE whitelist entry: %v", err)
		}
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		readJSON(t, resp, &result)

		if result["value"] != "allowed@example.com" {
			t.Errorf("expected value allowed@example.com, got %v", result["value"])
		}

		// Verify it's gone
		resp2, err := doRequest("GET", "/v3/"+domain+"/whitelists/allowed@example.com", nil)
		if err != nil {
			t.Fatalf("GET deleted whitelist entry: %v", err)
		}
		if resp2.StatusCode != http.StatusNotFound {
			resp2.Body.Close()
			t.Errorf("expected 404 for deleted allowlist entry, got %d", resp2.StatusCode)
		} else {
			resp2.Body.Close()
		}

		reporter.Record("Suppressions", "DeleteFromAllowlist", "HTTP", !t.Failed(), "")
	})
}
