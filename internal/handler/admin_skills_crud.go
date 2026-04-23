package handler

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/skills"
	"github.com/usehiveloop/hiveloop/internal/tasks"
)

// CreateSkill handles POST /admin/v1/skills.
// @Summary Create a global skill
// @Description Creates a global skill (org_id = nil) visible to all users.
// @Tags admin
// @Accept json
// @Produce json
// @Param body body adminCreateSkillRequest true "Skill to create"
// @Success 201 {object} adminSkillResponse
// @Failure 400 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/skills [post]
func (h *AdminHandler) CreateSkill(w http.ResponseWriter, r *http.Request) {
	var req adminCreateSkillRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	if req.SourceType == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "source_type is required (inline or git)"})
		return
	}
	if req.SourceType != model.SkillSourceInline && req.SourceType != model.SkillSourceGit {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "source_type must be 'inline' or 'git'"})
		return
	}
	if req.SourceType == model.SkillSourceGit && (req.RepoURL == nil || *req.RepoURL == "") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "repo_url is required for git skills"})
		return
	}

	status := req.Status
	if status == "" {
		status = model.SkillStatusPublished
	}

	slug := model.GenerateSlug(req.Name)
	repoRef := "main"
	if req.RepoRef != nil && *req.RepoRef != "" {
		repoRef = *req.RepoRef
	}

	skill := model.Skill{
		OrgID:       nil, // global skill
		Slug:        slug,
		Name:        req.Name,
		Description: req.Description,
		SourceType:  req.SourceType,
		RepoURL:     req.RepoURL,
		RepoSubpath: req.RepoSubpath,
		RepoRef:     repoRef,
		Tags:        req.Tags,
		Featured:    req.Featured,
		Status:      status,
	}

	if err := h.db.Create(&skill).Error; err != nil {
		if isDuplicateKeyError(err) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "a skill with this name already exists"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create skill"})
		return
	}

	if req.SourceType == model.SkillSourceInline && req.Bundle != nil {
		if req.Bundle.ID == "" {
			req.Bundle.ID = skill.Slug
		}
		if req.Bundle.Title == "" {
			req.Bundle.Title = skill.Name
		}
		if req.Bundle.Description == "" && skill.Description != nil {
			req.Bundle.Description = *skill.Description
		}
		if _, err := skills.HydrateInline(r.Context(), h.db, skill.ID, req.Bundle, "v1"); err != nil {
			slog.Error("admin: failed to hydrate inline skill", "skill_id", skill.ID, "error", err)
		}
		_ = h.db.First(&skill, "id = ?", skill.ID).Error
	} else if req.SourceType == model.SkillSourceGit {
		if h.enqueuer != nil {
			task, err := tasks.NewSkillHydrateTask(skill.ID)
			if err == nil {
				_, _ = h.enqueuer.Enqueue(task)
			}
		}
	}

	writeJSON(w, http.StatusCreated, toAdminSkillResponse(skill))
}

type adminUpdateSkillRequest struct {
	Name        *string   `json:"name,omitempty"`
	Description *string   `json:"description,omitempty"`
	Status      *string   `json:"status,omitempty"`
	Featured    *bool     `json:"featured,omitempty"`
	Tags        *[]string `json:"tags,omitempty"`
	RepoRef     *string   `json:"repo_ref,omitempty"`
}

// UpdateSkill handles PUT /admin/v1/skills/{id}.
// @Summary Update a skill
// @Description Updates skill properties.
// @Tags admin
// @Accept json
// @Produce json
// @Param id path string true "Skill ID"
// @Param body body adminUpdateSkillRequest true "Fields to update"
// @Success 200 {object} adminSkillResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/skills/{id} [put]
func (h *AdminHandler) UpdateSkill(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var skill model.Skill
	if err := h.db.Where("id = ?", id).First(&skill).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "skill not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get skill"})
		return
	}

	var req adminUpdateSkillRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	updates := map[string]any{}
	if req.Name != nil {
		updates["name"] = *req.Name
		updates["slug"] = model.GenerateSlug(*req.Name)
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	if req.Featured != nil {
		updates["featured"] = *req.Featured
	}
	if req.Tags != nil {
		updates["tags"] = pq.StringArray(*req.Tags)
	}
	if req.RepoRef != nil {
		updates["repo_ref"] = *req.RepoRef
	}

	if len(updates) > 0 {
		if err := h.db.Model(&skill).Updates(updates).Error; err != nil {
			if isDuplicateKeyError(err) {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "a skill with this name already exists"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update skill"})
			return
		}
	}

	h.db.Where("id = ?", id).First(&skill)
	writeJSON(w, http.StatusOK, toAdminSkillResponse(skill))
}

// DeleteSkill handles DELETE /admin/v1/skills/{id}.
// @Summary Delete a skill
// @Description Permanently deletes a skill and all its versions.
// @Tags admin
// @Produce json
// @Param id path string true "Skill ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/skills/{id} [delete]
func (h *AdminHandler) DeleteSkill(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var skill model.Skill
	if err := h.db.Where("id = ?", id).First(&skill).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "skill not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get skill"})
		return
	}

	if err := h.db.Delete(&skill).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete skill"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}