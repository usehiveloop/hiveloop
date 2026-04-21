package handler_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

type reportingTestHarness struct {
	db      *gorm.DB
	handler *handler.ReportingHandler
	router  *chi.Mux
}

func newReportingHarness(t *testing.T) *reportingTestHarness {
	t.Helper()
	db := connectTestDB(t)
	h := handler.NewReportingHandler(db)
	r := chi.NewRouter()
	r.Get("/v1/reporting", h.Get)
	return &reportingTestHarness{db: db, handler: h, router: r}
}

func (h *reportingTestHarness) doRequest(t *testing.T, path string, org *model.Org) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if org != nil {
		req = middleware.WithOrg(req, org)
	}
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func seedReportingGenerations(t *testing.T, db *gorm.DB, orgID uuid.UUID) {
	t.Helper()
	credID := uuid.New()
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, time.UTC)

	gens := []model.Generation{
		// Today: 3 OpenAI requests
		{ID: fmt.Sprintf("gen_r1_%s", orgID.String()[:8]), OrgID: orgID, CredentialID: credID, TokenJTI: "jti_r1",
			ProviderID: "openai", Model: "gpt-4o", RequestPath: "/v1/chat/completions",
			InputTokens: 100, OutputTokens: 50, CachedTokens: 20, Cost: 0.001,
			TTFBMs: intPtr(50), TotalMs: 200, UpstreamStatus: 200,
			UserID: "user_a", Tags: pq.StringArray{"chat"}, CreatedAt: today},
		{ID: fmt.Sprintf("gen_r2_%s", orgID.String()[:8]), OrgID: orgID, CredentialID: credID, TokenJTI: "jti_r2",
			ProviderID: "openai", Model: "gpt-4o", RequestPath: "/v1/chat/completions",
			InputTokens: 200, OutputTokens: 100, Cost: 0.003,
			TTFBMs: intPtr(80), TotalMs: 300, UpstreamStatus: 200,
			UserID: "user_a", Tags: pq.StringArray{"chat", "pro"}, CreatedAt: today.Add(time.Minute)},
		{ID: fmt.Sprintf("gen_r3_%s", orgID.String()[:8]), OrgID: orgID, CredentialID: credID, TokenJTI: "jti_r3",
			ProviderID: "openai", Model: "gpt-4o-mini", RequestPath: "/v1/chat/completions",
			InputTokens: 50, OutputTokens: 25, Cost: 0.0005,
			TTFBMs: intPtr(30), TotalMs: 100, UpstreamStatus: 200,
			UserID: "user_b", Tags: pq.StringArray{"summary"}, CreatedAt: today.Add(2 * time.Minute)},

		// Today: 2 Anthropic requests (one error)
		{ID: fmt.Sprintf("gen_r4_%s", orgID.String()[:8]), OrgID: orgID, CredentialID: credID, TokenJTI: "jti_r4",
			ProviderID: "anthropic", Model: "claude-sonnet-4-20250514", RequestPath: "/v1/messages",
			InputTokens: 300, OutputTokens: 150, CachedTokens: 100, Cost: 0.005,
			TTFBMs: intPtr(120), TotalMs: 500, UpstreamStatus: 200,
			UserID: "user_a", Tags: pq.StringArray{"chat"}, CreatedAt: today.Add(3 * time.Minute)},
		{ID: fmt.Sprintf("gen_r5_%s", orgID.String()[:8]), OrgID: orgID, CredentialID: credID, TokenJTI: "jti_r5",
			ProviderID: "anthropic", Model: "claude-sonnet-4-20250514", RequestPath: "/v1/messages",
			InputTokens: 0, OutputTokens: 0, Cost: 0,
			TTFBMs: intPtr(200), TotalMs: 200, UpstreamStatus: 429,
			ErrorType: "rate_limit", ErrorMessage: "rate limited",
			UserID: "user_b", CreatedAt: today.Add(4 * time.Minute)},

		// Yesterday: 1 request
		{ID: fmt.Sprintf("gen_r6_%s", orgID.String()[:8]), OrgID: orgID, CredentialID: credID, TokenJTI: "jti_r6",
			ProviderID: "openai", Model: "gpt-4o", RequestPath: "/v1/chat/completions",
			InputTokens: 500, OutputTokens: 200, Cost: 0.01,
			TTFBMs: intPtr(60), TotalMs: 250, UpstreamStatus: 200,
			UserID: "user_a", Tags: pq.StringArray{"chat"}, CreatedAt: today.AddDate(0, 0, -1)},
	}

	if err := db.Create(&gens).Error; err != nil {
		t.Fatalf("seed reporting generations: %v", err)
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", orgID).Delete(&model.Generation{})
	})
}

func intPtr(v int) *int { return &v }

// --------------------------------------------------------------------------
// GET /v1/reporting
// --------------------------------------------------------------------------

func TestReportingHandler_BasicDayGrouping(t *testing.T) {
	h := newReportingHarness(t)
	org := createTestOrg(t, h.db)
	seedReportingGenerations(t, h.db, org.ID)

	rr := h.doRequest(t, "/v1/reporting?date_part=day", &org)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var rows []map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&rows)

	// Should have 2 periods (today and yesterday)
	if len(rows) != 2 {
		t.Fatalf("expected 2 day periods, got %d", len(rows))
	}

	// First row (most recent = today) should have 5 requests
	today := rows[0]
	if today["request_count"].(float64) != 5 {
		t.Errorf("today request_count = %v, want 5", today["request_count"])
	}
	if today["error_count"].(float64) != 1 {
		t.Errorf("today error_count = %v, want 1", today["error_count"])
	}
}

func TestReportingHandler_GroupByModel(t *testing.T) {
	h := newReportingHarness(t)
	org := createTestOrg(t, h.db)
	seedReportingGenerations(t, h.db, org.ID)

	rr := h.doRequest(t, "/v1/reporting?group_by=model&date_part=day", &org)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var rows []map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&rows)

	// Should have rows grouped by model per day
	if len(rows) < 3 {
		t.Fatalf("expected at least 3 rows (3 models across days), got %d", len(rows))
	}

	// Verify each row has a model field
	for _, row := range rows {
		if row["model"] == nil || row["model"] == "" {
			t.Error("expected model field in each row")
		}
	}
}

func TestReportingHandler_GroupByProvider(t *testing.T) {
	h := newReportingHarness(t)
	org := createTestOrg(t, h.db)
	seedReportingGenerations(t, h.db, org.ID)

	rr := h.doRequest(t, "/v1/reporting?group_by=provider&date_part=day", &org)
	var rows []map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&rows)

	// Today has openai + anthropic, yesterday has openai
	if len(rows) < 3 {
		t.Fatalf("expected at least 3 rows, got %d", len(rows))
	}
}

func TestReportingHandler_GroupByUser(t *testing.T) {
	h := newReportingHarness(t)
	org := createTestOrg(t, h.db)
	seedReportingGenerations(t, h.db, org.ID)

	rr := h.doRequest(t, "/v1/reporting?group_by=user&date_part=day", &org)
	var rows []map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&rows)

	// Should have user_a and user_b rows
	if len(rows) < 2 {
		t.Fatalf("expected at least 2 rows, got %d", len(rows))
	}
}

func TestReportingHandler_FilterByModel(t *testing.T) {
	h := newReportingHarness(t)
	org := createTestOrg(t, h.db)
	seedReportingGenerations(t, h.db, org.ID)

	rr := h.doRequest(t, "/v1/reporting?model=gpt-4o&date_part=day", &org)
	var rows []map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&rows)

	// gpt-4o: 2 today + 1 yesterday = 2 day rows
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

	// Only 1 generation has "pro" tag
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

	// Only today's requests
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

	// Should have at least 2 hourly periods
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
	// Sum: 100+200+50+300+0 = 650
	if row["input_tokens"].(float64) != 650 {
		t.Errorf("input_tokens = %v, want 650", row["input_tokens"])
	}
	// Sum: 50+100+25+150+0 = 325
	if row["output_tokens"].(float64) != 325 {
		t.Errorf("output_tokens = %v, want 325", row["output_tokens"])
	}
	// Sum: 20+0+0+100+0 = 120
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
	// TTFB values: 50, 80, 30, 120, 200
	// P50 should be around 80, P95 should be around 200
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

	// Should have provider+model combinations
	if len(rows) < 3 {
		t.Fatalf("expected at least 3 combined groups, got %d", len(rows))
	}
}

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
