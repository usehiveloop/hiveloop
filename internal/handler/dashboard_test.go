package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

func TestDashboardHandler_Get_ReturnsOrgSummary(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	org.PlanSlug = "pro"
	if err := db.Save(&org).Error; err != nil {
		t.Fatalf("update org: %v", err)
	}

	periodStart := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	user := createTestUser(t, db, "dashboard-"+uuid.NewString()[:8]+"@test.com")
	seedDashboardPlanAndSubscription(t, db, org.ID, "pro", periodStart, periodEnd)
	seedDashboardConnection(t, db, org.ID, user.ID, "slack")
	seedDashboardConnection(t, db, org.ID, user.ID, "github-app")
	seedDashboardConnection(t, db, org.ID, user.ID, "linear")
	seedDashboardSchedule(t, db, org.ID)
	seedDashboardCredits(t, db, org.ID, periodStart)

	h := handler.NewDashboardHandler(db, billing.NewCreditsService(db))
	req := httptest.NewRequest(http.MethodGet, "/v1/dashboard", nil)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Credits struct {
			Balance         int64  `json:"balance"`
			SpentThisPeriod int64  `json:"spent_this_period"`
			PeriodStart     string `json:"period_start"`
			PeriodEnd       string `json:"period_end"`
		} `json:"credits"`
		Connections struct {
			Total             int64 `json:"total"`
			SlackConnected    bool  `json:"slack_connected"`
			NonSlackConnected int64 `json:"non_slack_connected"`
		} `json:"connections"`
		Onboarding struct {
			PlanSelected        bool  `json:"plan_selected"`
			ExtraToolsConnected int64 `json:"extra_tools_connected"`
			ExtraToolsRequired  int64 `json:"extra_tools_required"`
		} `json:"onboarding"`
		Schedules struct {
			Total int64 `json:"total"`
		} `json:"schedules"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Credits.Balance != 750 {
		t.Fatalf("balance = %d, want 750", resp.Credits.Balance)
	}
	if resp.Credits.SpentThisPeriod != 200 {
		t.Fatalf("spent = %d, want 200", resp.Credits.SpentThisPeriod)
	}
	if resp.Credits.PeriodStart != periodStart.Format(time.RFC3339) || resp.Credits.PeriodEnd != periodEnd.Format(time.RFC3339) {
		t.Fatalf("period = %s/%s", resp.Credits.PeriodStart, resp.Credits.PeriodEnd)
	}
	if resp.Connections.Total != 3 || !resp.Connections.SlackConnected || resp.Connections.NonSlackConnected != 2 {
		t.Fatalf("connections = %#v", resp.Connections)
	}
	if !resp.Onboarding.PlanSelected || resp.Onboarding.ExtraToolsConnected != 2 || resp.Onboarding.ExtraToolsRequired != 3 {
		t.Fatalf("onboarding = %#v", resp.Onboarding)
	}
	if resp.Schedules.Total != 1 {
		t.Fatalf("schedules = %d, want 1", resp.Schedules.Total)
	}
}

func TestDashboardHandler_Get_UsesCalendarMonthWithoutSubscription(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	now := time.Now().UTC()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	if err := db.Create(&model.CreditLedgerEntry{OrgID: org.ID, Amount: 100, Reason: "grant", CreatedAt: monthStart}).Error; err != nil {
		t.Fatalf("create grant: %v", err)
	}
	if err := db.Create(&model.CreditLedgerEntry{OrgID: org.ID, Amount: -25, Reason: "spend", CreatedAt: monthStart.Add(time.Hour)}).Error; err != nil {
		t.Fatalf("create spend: %v", err)
	}

	h := handler.NewDashboardHandler(db, billing.NewCreditsService(db))
	req := middleware.WithOrg(httptest.NewRequest(http.MethodGet, "/v1/dashboard", nil), &org)
	rr := httptest.NewRecorder()
	h.Get(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Credits struct {
			SpentThisPeriod int64  `json:"spent_this_period"`
			PeriodStart     string `json:"period_start"`
		} `json:"credits"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Credits.SpentThisPeriod != 25 {
		t.Fatalf("spent = %d, want 25", resp.Credits.SpentThisPeriod)
	}
	if resp.Credits.PeriodStart != monthStart.Format(time.RFC3339) {
		t.Fatalf("period_start = %s, want %s", resp.Credits.PeriodStart, monthStart.Format(time.RFC3339))
	}
}

func seedDashboardPlanAndSubscription(t *testing.T, db *gorm.DB, orgID uuid.UUID, slug string, start, end time.Time) {
	t.Helper()
	plan := model.Plan{ID: uuid.New(), Slug: slug + "-" + uuid.NewString()[:8], Name: "Pro", Active: true, MonthlyCredits: 1000}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if err := db.Model(&model.Org{}).Where("id = ?", orgID).Update("plan_slug", slug).Error; err != nil {
		t.Fatalf("update plan slug: %v", err)
	}
	sub := model.Subscription{
		ID:                  uuid.New(),
		OrgID:               orgID,
		PlanID:              plan.ID,
		Provider:            "paystack",
		ExternalCustomerID:  "cus-test",
		Status:              string(billing.StatusActive),
		CurrentPeriodStart:  start,
		CurrentPeriodEnd:    end,
		LastChargeReference: "ref-test",
	}
	if err := db.Create(&sub).Error; err != nil {
		t.Fatalf("create subscription: %v", err)
	}
}

func seedDashboardConnection(t *testing.T, db *gorm.DB, orgID, userID uuid.UUID, provider string) {
	t.Helper()
	integ := createTestInIntegration(t, db, provider)
	conn := model.InConnection{
		ID:                uuid.New(),
		OrgID:             orgID,
		UserID:            userID,
		InIntegrationID:   integ.ID,
		NangoConnectionID: provider + "-conn",
	}
	if err := db.Create(&conn).Error; err != nil {
		t.Fatalf("create connection %s: %v", provider, err)
	}
}

func seedDashboardSchedule(t *testing.T, db *gorm.DB, orgID uuid.UUID) {
	t.Helper()
	employee := model.Agent{ID: uuid.New(), OrgID: &orgID, Model: "gpt-5.4", Status: "active"}
	if err := db.Create(&employee).Error; err != nil {
		t.Fatalf("create employee: %v", err)
	}
	sandbox := model.Sandbox{
		ID:                    uuid.New(),
		OrgID:                 &orgID,
		AgentID:               &employee.ID,
		ExternalID:            "sb-test",
		BridgeURL:             "https://bridge.test",
		EncryptedBridgeAPIKey: []byte("encrypted"),
		Status:                "running",
	}
	if err := db.Create(&sandbox).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	schedule := model.EmployeeSchedule{
		ID:          uuid.New(),
		OrgID:       orgID,
		AgentID:     employee.ID,
		SandboxID:   sandbox.ID,
		BridgeJobID: "cron-test",
		Status:      "cancelled",
	}
	if err := db.Create(&schedule).Error; err != nil {
		t.Fatalf("create schedule: %v", err)
	}
}

func seedDashboardCredits(t *testing.T, db *gorm.DB, orgID uuid.UUID, periodStart time.Time) {
	t.Helper()
	rows := []model.CreditLedgerEntry{
		{ID: uuid.New(), OrgID: orgID, Amount: 1000, Reason: "grant", CreatedAt: periodStart.Add(time.Hour)},
		{ID: uuid.New(), OrgID: orgID, Amount: -200, Reason: "spend", CreatedAt: periodStart.Add(2 * time.Hour)},
		{ID: uuid.New(), OrgID: orgID, Amount: -50, Reason: "old-spend", CreatedAt: periodStart.AddDate(0, -1, 0)},
	}
	for i := range rows {
		if err := db.Create(&rows[i]).Error; err != nil {
			t.Fatalf("create credit row: %v", err)
		}
	}
}
