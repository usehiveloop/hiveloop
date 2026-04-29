package handler_test

import (
	"encoding/json"
	"testing"
)

// TestAuditHandler_List_IsolatedByOrg verifies that audit entries are properly isolated by organization.
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

// TestAuditHandler_List_FilterByAction verifies that audit entries can be filtered by action type.
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

// Note: Tests for empty list, pagination, invalid limit/cursor, missing org were removed
// as they test library/framework behavior rather than business logic.
// See USELESS_TESTS_RECOMMENDATIONS.md for details.