package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// AttachToEmployee handles POST /v1/employees/{id}/skills.
// @Summary Attach a skill to an employee
// @Description Creates an employee skill attachment.
// @Tags skills
// @Accept json
// @Produce json
// @Param id path string true "Employee ID"
// @Param body body attachSkillRequest true "Skill to attach"
// @Success 201 {object} agentSkillResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/employees/{id}/skills [post]
func (h *SkillHandler) AttachToEmployee(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	agent, err := h.loadAgent(r.Context(), chi.URLParam(r, "id"), org.ID)
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

	link, err := h.attachSkillToEmployee(r.Context(), agent.ID, skill.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to attach skill"})
		return
	}

	h.db.Model(&model.Skill{}).
		Where("id = ?", skill.ID).
		UpdateColumn("install_count", gorm.Expr("install_count + 1"))

	writeJSON(w, http.StatusCreated, toAgentSkillResponse(link, *skill))
}

func (h *SkillHandler) attachSkillToEmployee(ctx context.Context, employeeID, skillID uuid.UUID) (model.AgentSkill, error) {
	link := model.AgentSkill{
		AgentID: employeeID,
		SkillID: skillID,
	}
	err := h.db.WithContext(ctx).Save(&link).Error
	return link, err
}

// DetachFromEmployee handles DELETE /v1/employees/{id}/skills/{skillID}.
// @Summary Detach a skill from an employee
// @Description Removes an employee skill attachment.
// @Tags skills
// @Produce json
// @Param id path string true "Employee ID"
// @Param skillID path string true "Skill ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/employees/{id}/skills/{skillID} [delete]
func (h *SkillHandler) DetachFromEmployee(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	agent, err := h.loadAgent(r.Context(), chi.URLParam(r, "id"), org.ID)
	if err != nil {
		writeSkillLookupError(w, err)
		return
	}
	skillID, err := uuid.Parse(chi.URLParam(r, "skillID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid skill_id"})
		return
	}
	lockedSkillIDs, err := employeeLockedSkillIDs(r.Context(), h.db, org.ID, agent)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to check connection-managed employee skills"})
		return
	}
	if lockedSkillIDs[skillID] {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "connection-managed employee skill cannot be removed while the connection is active"})
		return
	}
	result := h.db.Where("agent_id = ? AND skill_id = ?", agent.ID, skillID).Delete(&model.AgentSkill{})
	if result.Error != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to detach skill"})
		return
	}
	if result.RowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "skill not attached to employee"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "detached"})
}

// ListEmployeeSkills handles GET /v1/employees/{id}/skills.
// @Summary List skills attached to an employee
// @Tags skills
// @Produce json
// @Param id path string true "Employee ID"
// @Success 200 {array} agentSkillResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/employees/{id}/skills [get]
func (h *SkillHandler) ListEmployeeSkills(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	agent, err := h.loadAgent(r.Context(), chi.URLParam(r, "id"), org.ID)
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
	if err := h.db.Where("id IN ? AND hidden = false", skillIDs).Find(&rows).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load skills"})
		return
	}
	skillByID := make(map[uuid.UUID]model.Skill, len(rows))
	for _, s := range rows {
		skillByID[s.ID] = s
	}

	resp := make([]agentSkillResponse, 0, len(links))
	lockedSkillIDs, _ := employeeLockedSkillIDs(r.Context(), h.db, org.ID, agent)
	for _, l := range links {
		s, ok := skillByID[l.SkillID]
		if !ok {
			continue
		}
		item := toAgentSkillResponse(l, s)
		if lockedSkillIDs[l.SkillID] {
			item.Locked = true
			item.Required = true
		}
		resp = append(resp, item)
	}
	writeJSON(w, http.StatusOK, resp)
}
