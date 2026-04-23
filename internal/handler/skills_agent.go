package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// AttachToAgent handles POST /v1/agents/{agentID}/skills.
// @Summary Attach a skill to an agent
// @Description Creates an agent_skills row. PinnedVersionID is optional — when null the agent follows the skill's latest version.
// @Tags skills
// @Accept json
// @Produce json
// @Param agentID path string true "Agent ID"
// @Param body body attachSkillRequest true "Skill to attach"
// @Success 201 {object} agentSkillResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{agentID}/skills [post]
func (h *SkillHandler) AttachToAgent(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	agent, err := h.loadAgent(r.Context(), chi.URLParam(r, "agentID"), org.ID)
	if err != nil {
		writeSkillLookupError(w, err)
		return
	}

	var req attachSkillRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	skillID, err := uuid.Parse(req.SkillID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid skill_id"})
		return
	}

	skill, err := h.loadSkillVisibleToOrg(r.Context(), skillID.String(), org.ID)
	if err != nil {
		writeSkillLookupError(w, err)
		return
	}

	var pinnedID *uuid.UUID
	if req.PinnedVersionID != nil && *req.PinnedVersionID != "" {
		pid, err := uuid.Parse(*req.PinnedVersionID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid pinned_version_id"})
			return
		}
		var sv model.SkillVersion
		if err := h.db.Where("id = ? AND skill_id = ?", pid, skill.ID).First(&sv).Error; err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "pinned version does not belong to skill"})
			return
		}
		pinnedID = &pid
	}

	link := model.AgentSkill{
		AgentID:         agent.ID,
		SkillID:         skill.ID,
		PinnedVersionID: pinnedID,
	}
	if err := h.db.Save(&link).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to attach skill"})
		return
	}

	h.db.Model(&model.Skill{}).
		Where("id = ?", skill.ID).
		UpdateColumn("install_count", gorm.Expr("install_count + 1"))

	writeJSON(w, http.StatusCreated, toAgentSkillResponse(link, *skill))
}

// DetachFromAgent handles DELETE /v1/agents/{agentID}/skills/{skillID}.
// @Summary Detach a skill from an agent
// @Description Removes an agent_skills row.
// @Tags skills
// @Produce json
// @Param agentID path string true "Agent ID"
// @Param skillID path string true "Skill ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{agentID}/skills/{skillID} [delete]
func (h *SkillHandler) DetachFromAgent(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	agent, err := h.loadAgent(r.Context(), chi.URLParam(r, "agentID"), org.ID)
	if err != nil {
		writeSkillLookupError(w, err)
		return
	}
	skillID, err := uuid.Parse(chi.URLParam(r, "skillID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid skill_id"})
		return
	}
	result := h.db.Where("agent_id = ? AND skill_id = ?", agent.ID, skillID).Delete(&model.AgentSkill{})
	if result.Error != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to detach skill"})
		return
	}
	if result.RowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "skill not attached to agent"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "detached"})
}

// ListAgentSkills handles GET /v1/agents/{agentID}/skills.
// @Summary List skills attached to an agent
// @Tags skills
// @Produce json
// @Param agentID path string true "Agent ID"
// @Success 200 {array} agentSkillResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{agentID}/skills [get]
func (h *SkillHandler) ListAgentSkills(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	agent, err := h.loadAgent(r.Context(), chi.URLParam(r, "agentID"), org.ID)
	if err != nil {
		writeSkillLookupError(w, err)
		return
	}

	var links []model.AgentSkill
	if err := h.db.Where("agent_id = ?", agent.ID).Find(&links).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list agent skills"})
		return
	}
	if len(links) == 0 {
		writeJSON(w, http.StatusOK, []agentSkillResponse{})
		return
	}

	skillIDs := make([]uuid.UUID, len(links))
	for i, l := range links {
		skillIDs[i] = l.SkillID
	}
	var rows []model.Skill
	if err := h.db.Where("id IN ?", skillIDs).Find(&rows).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load skills"})
		return
	}
	skillByID := make(map[uuid.UUID]model.Skill, len(rows))
	for _, s := range rows {
		skillByID[s.ID] = s
	}

	resp := make([]agentSkillResponse, 0, len(links))
	for _, l := range links {
		s, ok := skillByID[l.SkillID]
		if !ok {
			continue
		}
		resp = append(resp, toAgentSkillResponse(l, s))
	}
	writeJSON(w, http.StatusOK, resp)
}
