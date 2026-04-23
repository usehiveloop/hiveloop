package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// Publish handles POST /v1/skills/{id}/publish.
// @Summary Publish a skill to the public marketplace
// @Description Clones an org-owned skill into a public skill (OrgID=nil) with a reference back to the original. The original skill gets a reference to the public clone.
// @Tags skills
// @Produce json
// @Param id path string true "Skill ID"
// @Success 201 {object} skillDetailResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/skills/{id}/publish [post]
func (h *SkillHandler) Publish(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	user, _ := middleware.UserFromContext(r.Context())

	skill, err := h.loadOwnSkill(r.Context(), chi.URLParam(r, "id"), org.ID)
	if err != nil {
		writeSkillLookupError(w, err)
		return
	}

	if skill.PublicSkillID != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "skill is already published publicly"})
		return
	}
	if skill.LatestVersionID == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "skill has no version to publish"})
		return
	}

	var latestVersion model.SkillVersion
	if err := h.db.First(&latestVersion, "id = ?", *skill.LatestVersionID).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load latest version"})
		return
	}

	now := time.Now()
	originOrgID := org.ID
	originSkillID := skill.ID

	var publisherID *uuid.UUID
	if user != nil {
		publisherID = &user.ID
	}

	publicSkill := model.Skill{
		OrgID:         nil,
		Slug:          model.GenerateSlug(skill.Name),
		Name:          skill.Name,
		Description:   skill.Description,
		SourceType:    skill.SourceType,
		RepoURL:       skill.RepoURL,
		RepoSubpath:   skill.RepoSubpath,
		RepoRef:       skill.RepoRef,
		Tags:          skill.Tags,
		Status:        model.SkillStatusPublished,
		PublisherID:   publisherID,
		OriginSkillID: &originSkillID,
		OriginOrgID:   &originOrgID,
		PublishedAt:   &now,
	}

	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&publicSkill).Error; err != nil {
			return err
		}

		publicVersion := model.SkillVersion{
			SkillID:    publicSkill.ID,
			Version:    latestVersion.Version,
			CommitSHA:  latestVersion.CommitSHA,
			Bundle:     latestVersion.Bundle,
			HydratedAt: latestVersion.HydratedAt,
			CreatedAt:  now,
		}
		if err := tx.Create(&publicVersion).Error; err != nil {
			return err
		}

		if err := tx.Model(&publicSkill).Update("latest_version_id", publicVersion.ID).Error; err != nil {
			return err
		}

		if err := tx.Model(skill).Update("public_skill_id", publicSkill.ID).Error; err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to publish skill"})
		return
	}

	_ = h.db.First(&publicSkill, "id = ?", publicSkill.ID).Error
	var publicVersion model.SkillVersion
	var publicLatest *model.SkillVersion
	if publicSkill.LatestVersionID != nil {
		if err := h.db.First(&publicVersion, "id = ?", *publicSkill.LatestVersionID).Error; err == nil {
			publicLatest = &publicVersion
		}
	}

	writeJSON(w, http.StatusCreated, toSkillDetailResponse(publicSkill, publicLatest))
}

// Unpublish handles DELETE /v1/skills/{id}/publish.
// @Summary Remove a skill from the public marketplace
// @Description Archives the public clone and removes the reference from the original skill.
// @Tags skills
// @Produce json
// @Param id path string true "Skill ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/skills/{id}/publish [delete]
func (h *SkillHandler) Unpublish(w http.ResponseWriter, r *http.Request) {
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

	if skill.PublicSkillID == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "skill is not published publicly"})
		return
	}

	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.Skill{}).Where("id = ?", *skill.PublicSkillID).Update("status", model.SkillStatusArchived).Error; err != nil {
			return err
		}
		if err := tx.Model(skill).Update("public_skill_id", nil).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to unpublish skill"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "unpublished"})
}
