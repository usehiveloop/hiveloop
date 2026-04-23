package handler_test

import (
	"encoding/json"
	"testing"
	"time"
)

func TestReportingHandler_FilterByModel(t *testing.T) {
	h := newReportingHarness(t)
	org := createTestOrg(t, h.db)
	seedReportingGenerations(t, h.db, org.ID)

	rr := h.doRequest(t, "/v1/reporting?model=gpt-4o&date_part=day", &org)
	var rows []map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&rows)

	if len(rows) != 2 {
		t.Fatalf("expected 2 rows for gpt-4o, got %d", len(rows))
	}

	totalRequests := int64(0)
	for _, row := range rows {
		totalRequests += int64(row["request_count"].(float64))
	}
	if totalRequests != 3 {
		t.Errorf("total gpt-4o requests = %d, want 3", totalRequests)
	}
}

func TestReportingHandler_FilterByProvider(t *testing.T) {
	h := newReportingHarness(t)
	org := createTestOrg(t, h.db)
	seedReportingGenerations(t, h.db, org.ID)

	rr := h.doRequest(t, "/v1/reporting?provider_id=anthropic&date_part=day", &org)
	var rows []map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&rows)

	if len(rows) != 1 {
		t.Fatalf("expected 1 day with anthropic, got %d", len(rows))
	}
	if rows[0]["request_count"].(float64) != 2 {
		t.Errorf("anthropic requests = %v, want 2", rows[0]["request_count"])
	}
}

func TestReportingHandler_FilterByTags(t *testing.T) {
	h := newReportingHarness(t)
	org := createTestOrg(t, h.db)
	seedReportingGenerations(t, h.db, org.ID)

	rr := h.doRequest(t, "/v1/reporting?tags=pro&date_part=day", &org)
	var rows []map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&rows)

	totalRequests := int64(0)
	for _, row := range rows {
		totalRequests += int64(row["request_count"].(float64))
	}
	if totalRequests != 1 {
		t.Errorf("pro-tagged requests = %d, want 1", totalRequests)
	}
}

func TestReportingHandler_DateRange(t *testing.T) {
	h := newReportingHarness(t)
	org := createTestOrg(t, h.db)
	seedReportingGenerations(t, h.db, org.ID)

	today := time.Now().UTC().Format("2006-01-02")
	rr := h.doRequest(t, "/v1/reporting?start_date="+today+"&end_date="+today, &org)
	var rows []map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&rows)

	totalRequests := int64(0)
	for _, row := range rows {
		totalRequests += int64(row["request_count"].(float64))
	}
	if totalRequests != 5 {
		t.Errorf("today's requests = %d, want 5", totalRequests)
	}
}

func TestReportingHandler_HourlyGranularity(t *testing.T) {
	h := newReportingHarness(t)
	org := createTestOrg(t, h.db)
	seedReportingGenerations(t, h.db, org.ID)

	rr := h.doRequest(t, "/v1/reporting?date_part=hour", &org)
	var rows []map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&rows)

	if len(rows) < 2 {
		t.Fatalf("expected at least 2 hourly periods, got %d", len(rows))
	}
}

func TestReportingHandler_TokenAggregation(t *testing.T) {
	h := newReportingHarness(t)
	org := createTestOrg(t, h.db)
	seedReportingGenerations(t, h.db, org.ID)

	today := time.Now().UTC().Format("2006-01-02")
	rr := h.doRequest(t, "/v1/reporting?start_date="+today+"&end_date="+today, &org)
	var rows []map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&rows)

	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}

	row := rows[0]
	if row["input_tokens"].(float64) != 650 {
		t.Errorf("input_tokens = %v, want 650", row["input_tokens"])
	}
	if row["output_tokens"].(float64) != 325 {
		t.Errorf("output_tokens = %v, want 325", row["output_tokens"])
	}
	if row["cached_tokens"].(float64) != 120 {
		t.Errorf("cached_tokens = %v, want 120", row["cached_tokens"])
	}
}

func TestReportingHandler_PercentileTTFB(t *testing.T) {
	h := newReportingHarness(t)
	org := createTestOrg(t, h.db)
	seedReportingGenerations(t, h.db, org.ID)

	today := time.Now().UTC().Format("2006-01-02")
	rr := h.doRequest(t, "/v1/reporting?start_date="+today+"&end_date="+today, &org)
	var rows []map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&rows)

	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}

	row := rows[0]
	if row["p50_ttfb_ms"].(float64) <= 0 {
		t.Error("p50_ttfb_ms should be positive")
	}
	if row["p95_ttfb_ms"].(float64) <= 0 {
		t.Error("p95_ttfb_ms should be positive")
	}
	if row["p95_ttfb_ms"].(float64) < row["p50_ttfb_ms"].(float64) {
		t.Error("p95 should be >= p50")
	}
}

func TestReportingHandler_CombinedGroupBy(t *testing.T) {
	h := newReportingHarness(t)
	org := createTestOrg(t, h.db)
	seedReportingGenerations(t, h.db, org.ID)

	rr := h.doRequest(t, "/v1/reporting?group_by=model,provider&date_part=day", &org)
	var rows []map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&rows)

	if len(rows) < 3 {
		t.Fatalf("expected at least 3 combined groups, got %d", len(rows))
	}
}
