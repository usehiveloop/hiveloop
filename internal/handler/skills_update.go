package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/skills"
)

// Update handles PATCH /v1/skills/{id}.
// @Summary Update a skill
// @Description Updates metadata on an org-owned skill. Public skills are read-only.
// @Tags skills
// @Accept json
// @Produce json
// @Param id path string true "Skill ID"
// @Param body body updateSkillRequest true "Fields to update"
// @Success 200 {object} skillResponse
// @Failure 400 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/skills/{id} [patch]
func (h *SkillHandler) Update(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	skill, err := h.loadOwnSkill(r.Context(), chi.URLParam(r, "id"), org.ID)
	if err != nil {
		writeSkillLookupError(w, err)
		return
	}

	var req updateSkillRequest
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
	if req.Tags != nil {
		updates["tags"] = *req.Tags
	}
	if req.RepoRef != nil && skill.SourceType == model.SkillSourceGit {
		updates["repo_ref"] = *req.RepoRef
	}
	if req.Status != nil {
		switch *req.Status {
		case model.SkillStatusDraft, model.SkillStatusPublished, model.SkillStatusArchived:
			updates["status"] = *req.Status
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid status"})
			return
		}
	}

	if len(updates) > 0 {
		if err := h.db.Model(skill).Updates(updates).Error; err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update skill"})
			return
		}
		_ = h.db.First(skill, "id = ?", skill.ID).Error
	}
	var latestVersion *model.SkillVersion
	if skill.LatestVersionID != nil {
		var sv model.SkillVersion
		if err := h.db.First(&sv, "id = ?", *skill.LatestVersionID).Error; err == nil {
			latestVersion = &sv
		}
	}
	writeJSON(w, http.StatusOK, toSkillResponse(*skill, latestVersion))
}

// UpdateContent handles PUT /v1/skills/{id}/content.
// @Summary Push a new inline version for a skill
// @Description Creates a new SkillVersion with the provided bundle. Works for both inline and git-sourced skills. The new version becomes the latest.
// @Tags skills
// @Accept json
// @Produce json
// @Param id path string true "Skill ID"
// @Param body body updateContentRequest true "Bundle content"
// @Success 200 {object} skillDetailResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/skills/{id}/content [put]
func (h *SkillHandler) UpdateContent(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	skill, err := h.loadOwnSkill(r.Context(), chi.URLParam(r, "id"), org.ID)
	if err != nil {
		writeSkillLookupError(w, err)
		return
	}

	var req updateContentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.Bundle.ID == "" {
		req.Bundle.ID = skill.Slug
	}
	if req.Bundle.Title == "" {
		req.Bundle.Title = skill.Name
	}
	if req.Bundle.Description == "" && skill.Description != nil {
		req.Bundle.Description = *skill.Description
	}

	var versionCount int64
	h.db.Model(&model.SkillVersion{}).Where("skill_id = ?", skill.ID).Count(&versionCount)
	versionLabel := fmt.Sprintf("v%d", versionCount+1)

	latest, err := skills.HydrateInline(r.Context(), h.db, skill.ID, &req.Bundle, versionLabel)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create version"})
		return
	}

	_ = h.db.First(skill, "id = ?", skill.ID).Error

	writeJSON(w, http.StatusOK, toSkillDetailResponse(*skill, latest))
}
