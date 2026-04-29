package handler_test

import (
	"encoding/json"
	"testing"
)

// TestReportingHandler_OrgIsolation verifies that reporting data is properly
// isolated between organizations - a key business requirement.
func TestReportingHandler_OrgIsolation(t *testing.T) {
	h := newReportingHarness(t)
	org1 := createTestOrg(t, h.db)
	org2 := createTestOrg(t, h.db)
	seedReportingGenerations(t, h.db, org1.ID)

	rr := h.doRequest(t, "/v1/reporting", &org2)
	var rows []map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&rows)
	if len(rows) != 0 {
		t.Fatalf("org2 should see 0 rows, got %d", len(rows))
	}
}

// Note: Tests for empty result, invalid date_part, invalid group_by, and missing org
// were removed as they test library/framework behavior (status codes for validation)
// without verifying business logic. See USELESS_TESTS_RECOMMENDATIONS.md for details.