package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

func toSubagentResponse(agent model.Agent) subagentResponse {
	resp := subagentResponse{
		ID:           agent.ID.String(),
		Name:         agent.Name,
		Description:  agent.Description,
		SystemPrompt: agent.SystemPrompt,
		Model:        agent.Model,
		Status:       agent.Status,
		CreatedAt:    agent.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:    agent.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if agent.OrgID != nil {
		orgIDStr := agent.OrgID.String()
		resp.OrgID = &orgIDStr
	}
	return resp
}

// List handles GET /v1/subagents.
// @Summary List subagents
// @Description Lists subagents visible to the current org. Use scope=public, own, or all.
// @Tags subagents
// @Produce json
// @Param scope query string false "Filter: public, own, all (default all)"
// @Param q query string false "Free-text search over name and description"
// @Param limit query int false "Page size (default 50, max 100)"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[subagentResponse]
// @Failure 401 {object} errorResponse
// @Security BearerAuth
// @Router /v1/subagents [get]
func (h *SubagentHandler) List(w http.ResponseWriter, r *http.Request) {
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

	scope := r.URL.Query().Get("scope")
	query := h.db.Model(&model.Agent{}).Where("agent_type = ?", model.AgentTypeSubagent)
	switch scope {
	case "public":
		query = query.Where("org_id IS NULL AND status = ?", "active")
	case "own":
		query = query.Where("org_id = ?", org.ID)
	case "", "all":
		query = query.Where("org_id = ? OR (org_id IS NULL AND status = ?)", org.ID, "active")
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "scope must be public, own, or all"})
		return
	}
	if searchTerm := strings.TrimSpace(r.URL.Query().Get("q")); searchTerm != "" {
		like := "%" + escapeLike(searchTerm) + "%"
		query = query.Where("name ILIKE ? OR description ILIKE ?", like, like)
	}
	query = applyPagination(query, cursor, limit)

	var rows []model.Agent
	if err := query.Find(&rows).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list subagents"})
		return
	}

	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	resp := make([]subagentResponse, len(rows))
	for index, agent := range rows {
		resp[index] = toSubagentResponse(agent)
	}
	result := paginatedResponse[subagentResponse]{Data: resp, HasMore: hasMore}
	if hasMore {
		last := rows[len(rows)-1]
		cursor := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &cursor
	}
	writeJSON(w, http.StatusOK, result)
}

// Get handles GET /v1/subagents/{id}.
// @Summary Get a subagent
// @Tags subagents
// @Produce json
// @Param id path string true "Subagent ID"
// @Success 200 {object} subagentResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/subagents/{id} [get]
func (h *SubagentHandler) Get(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	sub, err := h.loadVisibleSubagent(chi.URLParam(r, "id"), org.ID)
	if err != nil {
		writeSubagentLookupError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toSubagentResponse(*sub))
}

// Update handles PATCH /v1/subagents/{id}.
// @Summary Update a subagent
// @Tags subagents
// @Accept json
// @Produce json
// @Param id path string true "Subagent ID"
// @Param body body updateSubagentRequest true "Fields to update"
// @Success 200 {object} subagentResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/subagents/{id} [patch]
func (h *SubagentHandler) Update(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	sub, err := h.loadOwnSubagent(chi.URLParam(r, "id"), org.ID)
	if err != nil {
		writeSubagentLookupError(w, err)
		return
	}

	var req updateSubagentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	updates := map[string]any{}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.SystemPrompt != nil {
		updates["system_prompt"] = *req.SystemPrompt
	}
	if req.Model != nil {
		updates["model"] = *req.Model
	}
	if req.Tools != nil {
		updates["tools"] = req.Tools
	}
	if req.McpServers != nil {
		updates["mcp_servers"] = req.McpServers
	}
	if req.Skills != nil {
		updates["skills"] = req.Skills
	}
	if req.AgentConfig != nil {
		updates["agent_config"] = req.AgentConfig
	}
	if req.Permissions != nil {
		updates["permissions"] = req.Permissions
	}
	if req.Status != nil {
		switch *req.Status {
		case "active", "archived":
			updates["status"] = *req.Status
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "status must be active or archived"})
			return
		}
	}
	if len(updates) > 0 {
		if err := h.db.Model(sub).Updates(updates).Error; err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update subagent"})
			return
		}
		_ = h.db.First(sub, "id = ?", sub.ID).Error
	}
	writeJSON(w, http.StatusOK, toSubagentResponse(*sub))
}

// Delete handles DELETE /v1/subagents/{id}.
// @Summary Archive a subagent
// @Tags subagents
// @Produce json
// @Param id path string true "Subagent ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/subagents/{id} [delete]
func (h *SubagentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	sub, err := h.loadOwnSubagent(chi.URLParam(r, "id"), org.ID)
	if err != nil {
		writeSubagentLookupError(w, err)
		return
	}
	if err := h.db.Model(sub).Update("status", "archived").Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to archive subagent"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "archived"})
}
