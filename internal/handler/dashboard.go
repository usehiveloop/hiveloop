package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/billing"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

type DashboardHandler struct {
	db      *gorm.DB
	credits *billing.CreditsService
}

func NewDashboardHandler(db *gorm.DB, credits *billing.CreditsService) *DashboardHandler {
	return &DashboardHandler{db: db, credits: credits}
}

type dashboardCreditsResponse struct {
	Balance         int64  `json:"balance"`
	SpentThisPeriod int64  `json:"spent_this_period"`
	PeriodStart     string `json:"period_start"`
	PeriodEnd       string `json:"period_end"`
}

type dashboardConnectionsResponse struct {
	Total             int64 `json:"total"`
	SlackConnected    bool  `json:"slack_connected"`
	NonSlackConnected int64 `json:"non_slack_connected"`
}

type dashboardOnboardingResponse struct {
	PlanSelected        bool  `json:"plan_selected"`
	ExtraToolsConnected int64 `json:"extra_tools_connected"`
	ExtraToolsRequired  int64 `json:"extra_tools_required"`
}

type dashboardSchedulesResponse struct {
	Total int64 `json:"total"`
}

type dashboardResponse struct {
	Credits     dashboardCreditsResponse     `json:"credits"`
	Connections dashboardConnectionsResponse `json:"connections"`
	Onboarding  dashboardOnboardingResponse  `json:"onboarding"`
	Schedules   dashboardSchedulesResponse   `json:"schedules"`
}

// Get returns the authenticated org dashboard summary.
// @Summary Get dashboard summary
// @Description Returns Hivy dashboard metrics for the current organization.
// @Tags dashboard
// @Produce json
// @Success 200 {object} dashboardResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/dashboard [get]
func (h *DashboardHandler) Get(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	balance, err := h.creditBalance(org.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load credit balance"})
		return
	}

	periodStart, periodEnd, err := h.currentCreditPeriod(r.Context(), org.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load billing period"})
		return
	}
	spent, err := h.creditSpend(r.Context(), org.ID, periodStart, periodEnd)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load credit spend"})
		return
	}

	connections, err := h.connectionSummary(r.Context(), org.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load connections"})
		return
	}

	scheduleCount, err := h.scheduleCount(r.Context(), org.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load schedules"})
		return
	}

	resp := dashboardResponse{
		Credits: dashboardCreditsResponse{
			Balance:         balance,
			SpentThisPeriod: spent,
			PeriodStart:     periodStart.Format(time.RFC3339),
			PeriodEnd:       periodEnd.Format(time.RFC3339),
		},
		Connections: connections,
		Onboarding: dashboardOnboardingResponse{
			PlanSelected:        org.PlanSlug != "",
			ExtraToolsConnected: connections.NonSlackConnected,
			ExtraToolsRequired:  3,
		},
		Schedules: dashboardSchedulesResponse{Total: scheduleCount},
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *DashboardHandler) creditBalance(orgID uuid.UUID) (int64, error) {
	if h.credits != nil {
		return h.credits.Balance(orgID)
	}
	var total int64
	if err := h.db.Model(&model.CreditLedgerEntry{}).
		Where("org_id = ?", orgID).
		Select("COALESCE(SUM(amount), 0)").
		Scan(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

func (h *DashboardHandler) currentCreditPeriod(ctx context.Context, orgID uuid.UUID) (time.Time, time.Time, error) {
	var sub model.Subscription
	if err := h.db.WithContext(ctx).
		Where("org_id = ? AND status = ?", orgID, string(billing.StatusActive)).
		Order("created_at DESC").
		First(&sub).Error; err == nil && !sub.CurrentPeriodStart.IsZero() && !sub.CurrentPeriodEnd.IsZero() {
		return sub.CurrentPeriodStart.UTC(), sub.CurrentPeriodEnd.UTC(), nil
	} else if err != nil && err != gorm.ErrRecordNotFound {
		return time.Time{}, time.Time{}, err
	}
	now := time.Now().UTC()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	return start, start.AddDate(0, 1, 0), nil
}

func (h *DashboardHandler) creditSpend(ctx context.Context, orgID uuid.UUID, start, end time.Time) (int64, error) {
	var spent int64
	if err := h.db.WithContext(ctx).Model(&model.CreditLedgerEntry{}).
		Where("org_id = ? AND amount < 0 AND created_at >= ? AND created_at < ?", orgID, start, end).
		Select("COALESCE(SUM(-amount), 0)").
		Scan(&spent).Error; err != nil {
		return 0, err
	}
	return spent, nil
}

func (h *DashboardHandler) connectionSummary(ctx context.Context, orgID uuid.UUID) (dashboardConnectionsResponse, error) {
	type row struct {
		Provider string
		Count    int64
	}
	var rows []row
	if err := h.db.WithContext(ctx).
		Model(&model.Connection{}).
		Select("integrations.provider AS provider, COUNT(*) AS count").
		Joins("JOIN integrations ON integrations.id = connections.integration_id AND integrations.deleted_at IS NULL").
		Where("connections.org_id = ? AND connections.revoked_at IS NULL", orgID).
		Group("integrations.provider").
		Scan(&rows).Error; err != nil {
		return dashboardConnectionsResponse{}, err
	}
	var out dashboardConnectionsResponse
	for _, row := range rows {
		out.Total += row.Count
		if row.Provider == "slack" {
			out.SlackConnected = row.Count > 0
			continue
		}
		out.NonSlackConnected += row.Count
	}
	return out, nil
}

func (h *DashboardHandler) scheduleCount(ctx context.Context, orgID uuid.UUID) (int64, error) {
	var count int64
	if err := h.db.WithContext(ctx).Model(&model.EmployeeSchedule{}).
		Where("org_id = ?", orgID).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}
