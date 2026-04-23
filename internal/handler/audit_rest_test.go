package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestAuditHandler_List_Empty(t *testing.T) {
	h := newAuditHarness(t)
	org := createTestOrg(t, h.db)

	rr := h.doRequest(t, "/v1/audit", &org)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var page struct {
		Data    []map[string]any `json:"data"`
		HasMore bool             `json:"has_more"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&page)
	if len(page.Data) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(page.Data))
	}
	if page.HasMore {
		t.Fatal("expected has_more=false for empty list")
	}
}

func TestAuditHandler_List_IsolatedByOrg(t *testing.T) {
	h := newAuditHarness(t)
	org1 := createTestOrg(t, h.db)
	org2 := createTestOrg(t, h.db)
	seedAuditEntries(t, h.db, org1.ID, 5, "api.request")

	rr := h.doRequest(t, "/v1/audit", &org2)
	var page struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&page)
	if len(page.Data) != 0 {
		t.Fatalf("org2 should see 0 entries, got %d", len(page.Data))
	}
}

func TestAuditHandler_List_FilterByAction(t *testing.T) {
	h := newAuditHarness(t)
	org := createTestOrg(t, h.db)
	seedAuditEntries(t, h.db, org.ID, 3, "proxy.request")
	seedAuditEntries(t, h.db, org.ID, 2, "api.request")

	rr := h.doRequest(t, "/v1/audit?action=proxy.request", &org)
	var page struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&page)
	if len(page.Data) != 3 {
		t.Fatalf("expected 3 proxy entries, got %d", len(page.Data))
	}
	for _, e := range page.Data {
		if e["action"] != "proxy.request" {
			t.Fatalf("expected action proxy.request, got %v", e["action"])
		}
	}

	rr = h.doRequest(t, "/v1/audit?action=api.request", &org)
	_ = json.NewDecoder(rr.Body).Decode(&page)
	if len(page.Data) != 2 {
		t.Fatalf("expected 2 api entries, got %d", len(page.Data))
	}
}

func TestAuditHandler_List_Pagination(t *testing.T) {
	h := newAuditHarness(t)
	org := createTestOrg(t, h.db)
	seedAuditEntries(t, h.db, org.ID, 5, "api.request")

	rr := h.doRequest(t, "/v1/audit?limit=2", &org)
	var page1 struct {
		Data       []map[string]any `json:"data"`
		HasMore    bool             `json:"has_more"`
		NextCursor *string          `json:"next_cursor"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&page1)
	if len(page1.Data) != 2 {
		t.Fatalf("page1: expected 2 entries, got %d", len(page1.Data))
	}
	if !page1.HasMore {
		t.Fatal("page1: expected has_more=true")
	}
	if page1.NextCursor == nil {
		t.Fatal("page1: expected next_cursor")
	}

	rr = h.doRequest(t, "/v1/audit?limit=2&cursor="+*page1.NextCursor, &org)
	var page2 struct {
		Data       []map[string]any `json:"data"`
		HasMore    bool             `json:"has_more"`
		NextCursor *string          `json:"next_cursor"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&page2)
	if len(page2.Data) != 2 {
		t.Fatalf("page2: expected 2 entries, got %d", len(page2.Data))
	}
	if !page2.HasMore {
		t.Fatal("page2: expected has_more=true")
	}

	rr = h.doRequest(t, "/v1/audit?limit=2&cursor="+*page2.NextCursor, &org)
	var page3 struct {
		Data    []map[string]any `json:"data"`
		HasMore bool             `json:"has_more"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&page3)
	if len(page3.Data) != 1 {
		t.Fatalf("page3: expected 1 entry, got %d", len(page3.Data))
	}
	if page3.HasMore {
		t.Fatal("page3: expected has_more=false")
	}

	allIDs := make(map[float64]bool)
	for _, entries := range [][]map[string]any{page1.Data, page2.Data, page3.Data} {
		for _, e := range entries {
			id := e["id"].(float64)
			if allIDs[id] {
				t.Fatalf("duplicate entry ID %v across pages", id)
			}
			allIDs[id] = true
		}
	}
	if len(allIDs) != 5 {
		t.Fatalf("expected 5 unique entries across all pages, got %d", len(allIDs))
	}
}

func TestAuditHandler_List_InvalidLimit(t *testing.T) {
	h := newAuditHarness(t)
	org := createTestOrg(t, h.db)

	rr := h.doRequest(t, "/v1/audit?limit=abc", &org)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestAuditHandler_List_InvalidCursor(t *testing.T) {
	h := newAuditHarness(t)
	org := createTestOrg(t, h.db)

	rr := h.doRequest(t, "/v1/audit?cursor=not-a-number", &org)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestAuditHandler_List_MissingOrg(t *testing.T) {
	h := newAuditHarness(t)

	rr := h.doRequest(t, "/v1/audit", nil)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}
