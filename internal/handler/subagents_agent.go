package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// AttachToAgent handles POST /v1/agents/{agentID}/subagents.
// @Summary Attach a subagent to an agent
// @Tags subagents
// @Accept json
// @Produce json
// @Param agentID path string true "Parent agent ID"
// @Param body body attachSubagentRequest true "Subagent to attach"
// @Success 201 {object} agentSubagentResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{agentID}/subagents [post]
func (h *SubagentHandler) AttachToAgent(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	parentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
		return
	}
	var parent model.Agent
	if err := h.db.Where("id = ? AND org_id = ? AND agent_type = ?", parentID, org.ID, model.AgentTypeAgent).First(&parent).Error; err != nil {
		writeSubagentLookupError(w, err)
		return
	}

	var req attachSubagentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	subID, err := uuid.Parse(req.SubagentID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid subagent_id"})
		return
	}
	sub, err := h.loadVisibleSubagent(subID.String(), org.ID)
	if err != nil {
		writeSubagentLookupError(w, err)
		return
	}

	link := model.AgentSubagent{AgentID: parent.ID, SubagentID: sub.ID}
	if err := h.db.Save(&link).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to attach subagent"})
		return
	}

	writeJSON(w, http.StatusCreated, agentSubagentResponse{
		SubagentID: sub.ID.String(),
		CreatedAt:  link.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		Subagent:   toSubagentResponse(*sub),
	})
}

// DetachFromAgent handles DELETE /v1/agents/{agentID}/subagents/{subagentID}.
// @Summary Detach a subagent from an agent
// @Tags subagents
// @Produce json
// @Param agentID path string true "Parent agent ID"
// @Param subagentID path string true "Subagent ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{agentID}/subagents/{subagentID} [delete]
func (h *SubagentHandler) DetachFromAgent(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	parentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
		return
	}
	var parent model.Agent
	if err := h.db.Where("id = ? AND org_id = ?", parentID, org.ID).First(&parent).Error; err != nil {
		writeSubagentLookupError(w, err)
		return
	}
	subID, err := uuid.Parse(chi.URLParam(r, "subagentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid subagent id"})
		return
	}
	result := h.db.Where("agent_id = ? AND subagent_id = ?", parent.ID, subID).Delete(&model.AgentSubagent{})
	if result.Error != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to detach subagent"})
		return
	}
	if result.RowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "subagent not attached to agent"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "detached"})
}

// ListAgentSubagents handles GET /v1/agents/{agentID}/subagents.
// @Summary List subagents attached to an agent
// @Tags subagents
// @Produce json
// @Param agentID path string true "Parent agent ID"
// @Success 200 {array} agentSubagentResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{agentID}/subagents [get]
func (h *SubagentHandler) ListAgentSubagents(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	parentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
		return
	}
	var parent model.Agent
	if err := h.db.Where("id = ? AND org_id = ?", parentID, org.ID).First(&parent).Error; err != nil {
		writeSubagentLookupError(w, err)
		return
	}

	var links []model.AgentSubagent
	if err := h.db.Where("agent_id = ?", parent.ID).Find(&links).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list subagents"})
		return
	}
	if len(links) == 0 {
		writeJSON(w, http.StatusOK, []agentSubagentResponse{})
		return
	}

	subIDs := make([]uuid.UUID, len(links))
	for index, link := range links {
		subIDs[index] = link.SubagentID
	}
	var subs []model.Agent
	if err := h.db.Where("id IN ?", subIDs).Find(&subs).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load subagents"})
		return
	}
	subByID := make(map[uuid.UUID]model.Agent, len(subs))
	for _, agent := range subs {
		subByID[agent.ID] = agent
	}

	resp := make([]agentSubagentResponse, 0, len(links))
	for _, link := range links {
		agent, ok := subByID[link.SubagentID]
		if !ok {
			continue
		}
		resp = append(resp, agentSubagentResponse{
			SubagentID: link.SubagentID.String(),
			CreatedAt:  link.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			Subagent:   toSubagentResponse(agent),
		})
	}
	writeJSON(w, http.StatusOK, resp)
}
