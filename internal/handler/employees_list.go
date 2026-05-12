package handler

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

type employeeSubagentSummary struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	AvatarURL   *string `json:"avatar_url,omitempty"`
	Description *string `json:"description,omitempty"`
	Status      string  `json:"status"`
}

type employeeSandboxSummary struct {
	ID           string  `json:"id"`
	Status       string  `json:"status"`
	ExternalID   string  `json:"external_id"`
	ErrorMessage *string `json:"error_message,omitempty"`
	LastActiveAt *string `json:"last_active_at,omitempty"`
	CreatedAt    string  `json:"created_at"`
}

type employeeListItem struct {
	agentResponse
	Subagents []employeeSubagentSummary `json:"subagents"`
	Sandbox   *employeeSandboxSummary   `json:"sandbox,omitempty"`
}

// @Summary List AI employees
// @Description Returns all employee agents in the org with sub-agents,
// @Description skills (metadata only — no bundle content), profiles,
// @Description triggers, and the latest sandbox row.
// @Tags employees
// @Produce json
// @Param status query string false "Filter by status (active, archived)"
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

	items := make([]employeeListItem, len(agents))
	for i, a := range agents {
		base := toAgentResponse(a)
		base.Triggers = triggers[a.ID]
		base.AttachedSkills = skills[a.ID]
		base.Profiles = profiles[a.ID]
		subs := subagents[a.ID]
		base.SubagentIDs = make([]string, len(subs))
		for j, s := range subs {
			base.SubagentIDs[j] = s.ID
		}
		items[i] = employeeListItem{
			agentResponse: base,
			Subagents:     subs,
			Sandbox:       sandboxes[a.ID],
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

func loadEmployeeSubagents(db *gorm.DB, agentIDs []uuid.UUID) map[uuid.UUID][]employeeSubagentSummary {
	if len(agentIDs) == 0 {
		return nil
	}
	var links []model.AgentSubagent
	if err := db.Where("agent_id IN ?", agentIDs).Find(&links).Error; err != nil {
		return nil
	}
	if len(links) == 0 {
		return nil
	}
	subIDs := make([]uuid.UUID, 0, len(links))
	for _, l := range links {
		subIDs = append(subIDs, l.SubagentID)
	}
	var subs []model.Agent
	if err := db.Select("id, name, avatar_url, description, status").
		Where("id IN ?", subIDs).
		Find(&subs).Error; err != nil {
		return nil
	}
	byID := make(map[uuid.UUID]model.Agent, len(subs))
	for _, s := range subs {
		byID[s.ID] = s
	}
	out := make(map[uuid.UUID][]employeeSubagentSummary, len(agentIDs))
	for _, l := range links {
		s, ok := byID[l.SubagentID]
		if !ok {
			continue
		}
		out[l.AgentID] = append(out[l.AgentID], employeeSubagentSummary{
			ID:          s.ID.String(),
			Name:        s.Name,
			AvatarURL:   s.AvatarURL,
			Description: s.Description,
			Status:      s.Status,
		})
	}
	return out
}

func loadLatestSandboxPerAgent(db *gorm.DB, orgID uuid.UUID, agentIDs []uuid.UUID) map[uuid.UUID]*employeeSandboxSummary {
	if len(agentIDs) == 0 {
		return nil
	}
	var rows []model.Sandbox
	if err := db.
		Where("org_id = ? AND agent_id IN ?", orgID, agentIDs).
		Order("created_at DESC").
		Find(&rows).Error; err != nil {
		return nil
	}
	out := make(map[uuid.UUID]*employeeSandboxSummary, len(agentIDs))
	for _, sb := range rows {
		if sb.AgentID == nil {
			continue
		}
		if _, seen := out[*sb.AgentID]; seen {
			continue
		}
		summary := &employeeSandboxSummary{
			ID:           sb.ID.String(),
			Status:       sb.Status,
			ExternalID:   sb.ExternalID,
			ErrorMessage: sb.ErrorMessage,
			CreatedAt:    sb.CreatedAt.Format(time.RFC3339),
		}
		if sb.LastActiveAt != nil {
			t := sb.LastActiveAt.Format(time.RFC3339)
			summary.LastActiveAt = &t
		}
		out[*sb.AgentID] = summary
	}
	return out
}
