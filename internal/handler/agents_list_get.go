package handler

import (
	"gorm.io/gorm"
	chi "github.com/go-chi/chi/v5"
	"net/http"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// List handles GET /v1/agents.
// @Summary List agents
// @Description Returns agents for the current organization with optional filters.
// @Tags agents
// @Produce json
// @Param status query string false "Filter by status (active, archived)"
// @Param sandbox_type query string false "Filter by sandbox type (shared, dedicated)"
// @Param limit query int false "Page size (default 50, max 100)"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[agentResponse]
// @Failure 401 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents [get]
func (h *AgentHandler) List(w http.ResponseWriter, r *http.Request) {
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

	q := h.db.Preload("Credential").Where("agents.org_id = ? AND agents.is_system = false AND agents.deleted_at IS NULL", org.ID)

	if status := r.URL.Query().Get("status"); status != "" {
		q = q.Where("agents.status = ?", status)
	}
	if sandboxType := r.URL.Query().Get("sandbox_type"); sandboxType != "" {
		q = q.Where("agents.sandbox_type = ?", sandboxType)
	}

	q = applyPagination(q, cursor, limit)

	var agents []model.Agent
	if err := q.Find(&agents).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list agents"})
		return
	}

	hasMore := len(agents) > limit
	if hasMore {
		agents = agents[:limit]
	}

	resp := make([]agentResponse, len(agents))
	agentIDs := make([]uuid.UUID, len(agents))
	for index, agent := range agents {
		resp[index] = toAgentResponse(agent)
		agentIDs[index] = agent.ID
	}

	// Batch load triggers, subagents, and skills for all agents.
	triggersMap := h.loadAgentTriggers(agentIDs...)
	subagentsMap := h.loadAgentSubagents(org.ID, agentIDs...)
	skillsMap := h.loadAgentSkills(org.ID, agentIDs...)
	for index, agent := range agents {
		resp[index].Triggers = triggersMap[agent.ID]
		resp[index].AttachedSubagents = subagentsMap[agent.ID]
		resp[index].AttachedSkills = skillsMap[agent.ID]
	}

	result := paginatedResponse[agentResponse]{Data: resp, HasMore: hasMore}
	if hasMore {
		last := agents[len(agents)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}

	writeJSON(w, http.StatusOK, result)
}

// Get handles GET /v1/agents/{id}.
// @Summary Get an agent
// @Description Returns a single agent by ID, including the latest forge run if one exists.
// @Tags agents
// @Produce json
// @Param id path string true "Agent ID"
// @Success 200 {object} agentResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{id} [get]
func (h *AgentHandler) Get(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	id := chi.URLParam(r, "id")
	var agent model.Agent
	if err := h.db.Preload("Credential").Where("id = ? AND org_id = ? AND is_system = false AND deleted_at IS NULL", id, org.ID).First(&agent).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get agent"})
		return
	}

	resp := toAgentResponse(agent)
	resp.Triggers = h.loadAgentTriggers(agent.ID)[agent.ID]
	resp.AttachedSubagents = h.loadAgentSubagents(org.ID, agent.ID)[agent.ID]
	resp.AttachedSkills = h.loadAgentSkills(org.ID, agent.ID)[agent.ID]

	writeJSON(w, http.StatusOK, resp)
}