package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

type employeeSessionResponse struct {
	ID              string                   `json:"id"`
	Channel         string                   `json:"channel"`
	ThreadTS        string                   `json:"thread_ts"`
	AgentSessionID  string                   `json:"agent_session_id"`
	Status          string                   `json:"status"`
	CreatedAt       string                   `json:"created_at"`
	LastActivityAt  string                   `json:"last_activity_at"`
	TriggerDelivery *triggerDeliveryResponse `json:"trigger_delivery,omitempty"`
}

// ListSessions handles GET /v1/employees/{id}/sessions.
// @Summary List sessions for an employee
// @Description Returns employee runtime sessions with optional trigger delivery metadata.
// @Tags employees
// @Produce json
// @Param id path string true "Employee agent ID"
// @Param status query string false "Filter by status (active, completed, errored)"
// @Param session_id query string false "Exact session ID filter"
// @Param channel query string false "Exact channel filter"
// @Param thread_ts query string false "Exact thread timestamp filter"
// @Param agent_session_id query string false "Exact agent session ID filter"
// @Param q query string false "Prefix search over session identifiers"
// @Param limit query int false "Page size"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[employeeSessionResponse]
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/employees/{id}/sessions [get]
func (h *EmployeeHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org, ok := middleware.OrgFromContext(ctx)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	agentID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid employee id"})
		return
	}

	var agent model.Employee
	if err := h.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND status <> ?", agentID, org.ID, "archived").
		First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "employee not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load employee"})
		return
	}

	sb, ok := h.loadLatestEmployeeSandbox(w, r, org.ID, agentID)
	if !ok {
		return
	}
	if h.compileDeps.EncKey == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "employee runtime encryption is not configured"})
		return
	}
	apiKey, err := h.compileDeps.EncKey.DecryptString(sb.EncryptedBridgeAPIKey)
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "decrypt employee runtime key", "error", err, "employee_id", agentID, "sandbox_id", sb.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load employee runtime credentials"})
		return
	}

	resp, err := employeeruntime.NewClient(sb.BridgeURL, apiKey).ListSessions(ctx, employeeSessionListParams(r))
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "list employee runtime sessions", "error", err, "employee_id", agentID, "sandbox_id", sb.ID)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to list employee sessions"})
		return
	}

	deliveries := h.loadTriggerDeliveriesForRuntimeSessions(ctx, org.ID, agentID, runtimeSessionIDs(resp.Items))
	items := make([]employeeSessionResponse, len(resp.Items))
	for i, session := range resp.Items {
		items[i] = employeeSessionToResponse(session, deliveries[session.ID])
	}
	writeJSON(w, http.StatusOK, paginatedResponse[employeeSessionResponse]{
		Data:       items,
		NextCursor: resp.NextCursor,
		HasMore:    resp.NextCursor != nil && *resp.NextCursor != "",
	})
}

func (h *EmployeeHandler) loadLatestEmployeeSandbox(w http.ResponseWriter, r *http.Request, orgID, agentID uuid.UUID) (*model.Sandbox, bool) {
	ctx := r.Context()
	var sb model.Sandbox
	if err := h.db.WithContext(ctx).
		Where("employee_id = ? AND org_id = ? AND status <> ?", agentID, orgID, "error").
		Order("created_at DESC").
		First(&sb).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "employee sandbox not found"})
			return nil, false
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load employee sandbox"})
		return nil, false
	}
	if h.orchestrator != nil && h.orchestrator.NeedsURLRefresh(&sb) {
		if err := h.orchestrator.RefreshEmployeeSandboxURL(ctx, &sb); err != nil {
			logging.FromContext(ctx).ErrorContext(ctx, "refresh employee sandbox url", "error", err, "employee_id", agentID, "sandbox_id", sb.ID)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to refresh employee sandbox URL"})
			return nil, false
		}
	}
	return &sb, true
}

func employeeSessionListParams(r *http.Request) employeeruntime.ListSessionsParams {
	q := r.URL.Query()
	return employeeruntime.ListSessionsParams{
		Cursor:         strings.TrimSpace(q.Get("cursor")),
		Status:         strings.TrimSpace(q.Get("status")),
		Limit:          parseEmployeeSessionLimit(q.Get("limit")),
		SessionID:      strings.TrimSpace(q.Get("session_id")),
		Channel:        strings.TrimSpace(q.Get("channel")),
		ThreadTS:       strings.TrimSpace(q.Get("thread_ts")),
		AgentSessionID: strings.TrimSpace(q.Get("agent_session_id")),
		Search:         strings.TrimSpace(q.Get("q")),
	}
}

func parseEmployeeSessionLimit(raw string) int {
	if raw == "" {
		return 50
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 {
		return 50
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func runtimeSessionIDs(sessions []employeeruntime.Session) []string {
	ids := make([]string, 0, len(sessions))
	seen := map[string]struct{}{}
	for _, session := range sessions {
		if session.ID == "" {
			continue
		}
		if _, ok := seen[session.ID]; ok {
			continue
		}
		seen[session.ID] = struct{}{}
		ids = append(ids, session.ID)
	}
	return ids
}

func (h *EmployeeHandler) loadTriggerDeliveriesForRuntimeSessions(ctx context.Context, orgID, agentID uuid.UUID, sessionIDs []string) map[string]*triggerDeliveryResponse {
	out := map[string]*triggerDeliveryResponse{}
	if len(sessionIDs) == 0 {
		return out
	}
	var rows []model.EmployeeTriggerDelivery
	if err := h.db.WithContext(ctx).
		Where("org_id = ? AND employee_id = ? AND runtime_session_id IN ?", orgID, agentID, sessionIDs).
		Order("created_at DESC").
		Find(&rows).Error; err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "load trigger deliveries for employee sessions", "error", err, "employee_id", agentID)
		return out
	}
	for _, row := range rows {
		if _, exists := out[row.RuntimeSessionID]; exists {
			continue
		}
		resp := triggerDeliveryToResponse(row)
		out[row.RuntimeSessionID] = &resp
	}
	return out
}

func employeeSessionToResponse(session employeeruntime.Session, delivery *triggerDeliveryResponse) employeeSessionResponse {
	return employeeSessionResponse{
		ID:              session.ID,
		Channel:         session.Channel,
		ThreadTS:        session.ThreadTS,
		AgentSessionID:  session.AgentSessionID,
		Status:          session.Status,
		CreatedAt:       formatRuntimeTime(session.CreatedAt),
		LastActivityAt:  formatRuntimeTime(session.LastActivityAt),
		TriggerDelivery: delivery,
	}
}

func formatRuntimeTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}
