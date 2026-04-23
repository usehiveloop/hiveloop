package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestReportingHandler_EmptyResult(t *testing.T) {
	h := newReportingHarness(t)
	org := createTestOrg(t, h.db)

	rr := h.doRequest(t, "/v1/reporting", &org)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var rows []map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&rows)
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(rows))
	}
}

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

func TestReportingHandler_InvalidDatePart(t *testing.T) {
	h := newReportingHarness(t)
	org := createTestOrg(t, h.db)

	rr := h.doRequest(t, "/v1/reporting?date_part=week", &org)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestReportingHandler_InvalidGroupBy(t *testing.T) {
	h := newReportingHarness(t)
	org := createTestOrg(t, h.db)

	rr := h.doRequest(t, "/v1/reporting?group_by=invalid", &org)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestReportingHandler_MissingOrg(t *testing.T) {
	h := newReportingHarness(t)
	rr := h.doRequest(t, "/v1/reporting", nil)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}
