package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

type employeeSubagentSummary struct {
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	AvatarURL         *string `json:"avatar_url,omitempty"`
	Description       *string `json:"description,omitempty"`
	Status            string  `json:"status"`
	TemplateSlug      *string `json:"template_slug,omitempty"`
	TemplateAgentType *string `json:"template_agent_type,omitempty"`
}

type employeeSandboxSummary struct {
	ID           string  `json:"id"`
	Status       string  `json:"status"`
	ExternalID   string  `json:"external_id"`
	ErrorMessage *string `json:"error_message,omitempty"`
	LastActiveAt *string `json:"last_active_at,omitempty"`
	CreatedAt    string  `json:"created_at"`
	snapshotID   *string
}

type employeeListItem struct {
	agentResponse
	UpgradeAvailable bool                      `json:"upgrade_available"`
	Subagents        []employeeSubagentSummary `json:"subagents"`
	Sandbox          *employeeSandboxSummary   `json:"sandbox,omitempty"`
}

// @Summary List AI employees
// @Description Returns all employee agents in the org with sub-agents,
// @Description skills (metadata only — no bundle content), profiles,
// @Description triggers, and the latest sandbox row.
// @Tags employees
// @Produce json
// @Param status query string false "Filter by status (draft, active, archived)"
// @Param limit query int false "Page size (default 50, max 100)"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[employeeListItem]
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/employees [get]
func (h *EmployeeHandler) List(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.WithContext(r.Context()).
		Preload("Credential").
		Preload("TeamRef").
		Where("agents.org_id = ? AND agents.is_employee = true AND agents.is_system = false", org.ID)

	if status := r.URL.Query().Get("status"); status != "" {
		q = q.Where("agents.status = ?", status)
	}

	q = applyPagination(q, cursor, limit)

	var agents []model.Agent
	if err := q.Find(&agents).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list employees"})
		return
	}

	hasMore := len(agents) > limit
	if hasMore {
		agents = agents[:limit]
	}

	agentIDs := make([]uuid.UUID, len(agents))
	for i, a := range agents {
		agentIDs[i] = a.ID
	}

	triggers := h.agents.loadAgentTriggers(agentIDs...)
	skills := h.agents.loadAgentSkills(agentIDs...)
	profiles := h.agents.loadAgentProfiles(agentIDs...)
	subagents := loadEmployeeSubagents(h.db, agentIDs)
	sandboxes := loadLatestSandboxPerAgent(h.db, org.ID, agentIDs)
	currentSnapshotID := h.currentEmployeeSandboxSnapshotID()

	items := make([]employeeListItem, len(agents))
	for i, a := range agents {
		base := toAgentResponse(a)
		base.Triggers = triggers[a.ID]
		base.AttachedSkills = h.markEmployeeSkillLocks(r.Context(), org.ID, &a, skills[a.ID])
		base.Profiles = profiles[a.ID]
		subs := subagents[a.ID]
		base.SubagentIDs = make([]string, len(subs))
		for j, s := range subs {
			base.SubagentIDs[j] = s.ID
		}
		items[i] = employeeListItem{
			agentResponse:    base,
			UpgradeAvailable: employeeSandboxUpgradeAvailable(sandboxes[a.ID], currentSnapshotID),
			Subagents:        subs,
			Sandbox:          sandboxes[a.ID],
		}
	}

	result := paginatedResponse[employeeListItem]{Data: items, HasMore: hasMore}
	if hasMore {
		last := agents[len(agents)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}
	writeJSON(w, http.StatusOK, result)
}

// Get handles GET /v1/employees/{id}.
// @Summary Get an AI employee
// @Description Returns one employee agent in the org with sub-agents,
// @Description skills (metadata only — no bundle content), profiles,
// @Description triggers, and the latest sandbox row.
// @Tags employees
// @Produce json
// @Param id path string true "Employee agent ID"
// @Success 200 {object} employeeListItem
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/employees/{id} [get]
func (h *EmployeeHandler) Get(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	agentID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid employee id"})
		return
	}

	var agent model.Agent
	if err := h.db.WithContext(r.Context()).
		Preload("Credential").
		Preload("TeamRef").
		Where("agents.id = ? AND agents.org_id = ? AND agents.is_employee = true AND agents.is_system = false", agentID, org.ID).
		First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "employee not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get employee"})
		return
	}

	base := toAgentResponse(agent)
	base.Triggers = h.agents.loadAgentTriggers(agent.ID)[agent.ID]
	base.AttachedSkills = h.markEmployeeSkillLocks(r.Context(), org.ID, &agent, h.agents.loadAgentSkills(agent.ID)[agent.ID])
	base.Profiles = h.agents.loadAgentProfiles(agent.ID)[agent.ID]
	subagents := loadEmployeeSubagents(h.db, []uuid.UUID{agent.ID})[agent.ID]
	base.SubagentIDs = make([]string, len(subagents))
	for i, subagent := range subagents {
		base.SubagentIDs[i] = subagent.ID
	}
	sandbox := loadLatestSandboxPerAgent(h.db, org.ID, []uuid.UUID{agent.ID})[agent.ID]
	currentSnapshotID := h.currentEmployeeSandboxSnapshotID()

	writeJSON(w, http.StatusOK, employeeListItem{
		agentResponse:    base,
		UpgradeAvailable: employeeSandboxUpgradeAvailable(sandbox, currentSnapshotID),
		Subagents:        subagents,
		Sandbox:          sandbox,
	})
}

func (h *EmployeeHandler) currentEmployeeSandboxSnapshotID() string {
	if h == nil || h.compileDeps.Cfg == nil {
		return ""
	}
	return h.compileDeps.Cfg.EmployeeSandboxBaseImagePrefix
}

func (h *EmployeeHandler) employeeListItem(ctx context.Context, orgID uuid.UUID, agent model.Agent) employeeListItem {
	base := toAgentResponse(agent)
	base.Triggers = h.agents.loadAgentTriggers(agent.ID)[agent.ID]
	base.AttachedSkills = h.markEmployeeSkillLocks(ctx, orgID, &agent, h.agents.loadAgentSkills(agent.ID)[agent.ID])
	base.Profiles = h.agents.loadAgentProfiles(agent.ID)[agent.ID]
	subagents := loadEmployeeSubagents(h.db, []uuid.UUID{agent.ID})[agent.ID]
	base.SubagentIDs = make([]string, len(subagents))
	for i, subagent := range subagents {
		base.SubagentIDs[i] = subagent.ID
	}
	sandbox := loadLatestSandboxPerAgent(h.db, orgID, []uuid.UUID{agent.ID})[agent.ID]
	return employeeListItem{
		agentResponse:    base,
		UpgradeAvailable: employeeSandboxUpgradeAvailable(sandbox, h.currentEmployeeSandboxSnapshotID()),
		Subagents:        subagents,
		Sandbox:          sandbox,
	}
}

func employeeSandboxUpgradeAvailable(summary *employeeSandboxSummary, currentSnapshotID string) bool {
	if summary == nil {
		return false
	}
	if summary.snapshotID == nil || *summary.snapshotID == "" {
		return currentSnapshotID != ""
	}
	return *summary.snapshotID != currentSnapshotID
}
