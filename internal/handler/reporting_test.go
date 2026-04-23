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

	if len(rows) != 2 {
		t.Fatalf("expected 2 day periods, got %d", len(rows))
	}

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

	if len(rows) < 3 {
		t.Fatalf("expected at least 3 rows (3 models across days), got %d", len(rows))
	}

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

	if len(rows) < 2 {
		t.Fatalf("expected at least 2 rows, got %d", len(rows))
	}
}
