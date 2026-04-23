package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/skills"
)

func toSkillResponse(s model.Skill, latestVersion *model.SkillVersion) skillResponse {
	resp := skillResponse{
		ID:           s.ID.String(),
		Slug:         s.Slug,
		Name:         s.Name,
		Description:  s.Description,
		SourceType:   s.SourceType,
		RepoURL:      s.RepoURL,
		RepoSubpath:  s.RepoSubpath,
		RepoRef:      s.RepoRef,
		Tags:         []string(s.Tags),
		InstallCount: s.InstallCount,
		Featured:     s.Featured,
		Status:       s.Status,
		CreatedAt:    s.CreatedAt,
		UpdatedAt:    s.UpdatedAt,
	}
	if s.OrgID != nil {
		orgIDStr := s.OrgID.String()
		resp.OrgID = &orgIDStr
	}
	if s.LatestVersionID != nil {
		latestIDStr := s.LatestVersionID.String()
		resp.LatestVersionID = &latestIDStr
	}
	if s.PublicSkillID != nil {
		publicIDStr := s.PublicSkillID.String()
		resp.PublicSkillID = &publicIDStr
	}
	if resp.Tags == nil {
		resp.Tags = []string{}
	}

	switch {
	case s.LatestVersionID == nil:
		resp.HydrationStatus = "pending"
	case latestVersion != nil && latestVersion.HydrationError != nil:
		resp.HydrationStatus = "error"
		resp.HydrationError = latestVersion.HydrationError
	default:
		resp.HydrationStatus = "ready"
	}

	return resp
}

// loadVersionMap batch-loads SkillVersions for the given skills and returns
// them keyed by version ID. Skills without a LatestVersionID are skipped.
func (h *SkillHandler) loadVersionMap(skills []model.Skill) map[uuid.UUID]model.SkillVersion {
	versionIDs := make([]uuid.UUID, 0, len(skills))
	for _, s := range skills {
		if s.LatestVersionID != nil {
			versionIDs = append(versionIDs, *s.LatestVersionID)
		}
	}
	if len(versionIDs) == 0 {
		return nil
	}
	var versions []model.SkillVersion
	if err := h.db.Where("id IN ?", versionIDs).Find(&versions).Error; err != nil {
		return nil
	}
	result := make(map[uuid.UUID]model.SkillVersion, len(versions))
	for _, sv := range versions {
		result[sv.ID] = sv
	}
	return result
}

func toSkillDetailResponse(s model.Skill, latest *model.SkillVersion) skillDetailResponse {
	detail := skillDetailResponse{skillResponse: toSkillResponse(s, latest)}
	if latest == nil {
		return detail
	}
	if len(latest.Bundle) > 0 {
		var bundle skills.Bundle
		if err := json.Unmarshal(latest.Bundle, &bundle); err == nil {
			detail.Bundle = &bundle
		}
	}
	return detail
}

func toSkillVersionResponse(sv model.SkillVersion) skillVersionResponse {
	resp := skillVersionResponse{
		ID:             sv.ID.String(),
		Version:        sv.Version,
		CommitSHA:      sv.CommitSHA,
		HydrationError: sv.HydrationError,
		CreatedAt:      sv.CreatedAt,
	}
	if sv.HydratedAt != nil {
		formatted := sv.HydratedAt.Format(time.RFC3339)
		resp.HydratedAt = &formatted
	}
	return resp
}

// loadSkillVisibleToOrg returns a skill if it is public-and-published or owned
// by the given org. Otherwise returns gorm.ErrRecordNotFound so the caller can
// respond with a 404 instead of leaking existence.
func (h *SkillHandler) loadSkillVisibleToOrg(ctx context.Context, id string, orgID uuid.UUID) (*model.Skill, error) {
	skillID, err := uuid.Parse(id)
	if err != nil {
		return nil, gorm.ErrRecordNotFound
	}
	var skill model.Skill
	err = h.db.WithContext(ctx).
		Where("id = ? AND (org_id = ? OR (org_id IS NULL AND status = ?))", skillID, orgID, model.SkillStatusPublished).
		First(&skill).Error
	if err != nil {
		return nil, err
	}
	return &skill, nil
}

// loadOwnSkill returns a skill only if the current org owns it. Public skills
// are not editable via this helper.
func (h *SkillHandler) loadOwnSkill(ctx context.Context, id string, orgID uuid.UUID) (*model.Skill, error) {
	skillID, err := uuid.Parse(id)
	if err != nil {
		return nil, gorm.ErrRecordNotFound
	}
	var skill model.Skill
	err = h.db.WithContext(ctx).
		Where("id = ? AND org_id = ?", skillID, orgID).
		First(&skill).Error
	if err != nil {
		return nil, err
	}
	return &skill, nil
}

func (h *SkillHandler) loadAgent(ctx context.Context, id string, orgID uuid.UUID) (*model.Agent, error) {
	agentID, err := uuid.Parse(id)
	if err != nil {
		return nil, gorm.ErrRecordNotFound
	}
	var agent model.Agent
	err = h.db.WithContext(ctx).
		Where("id = ? AND org_id = ?", agentID, orgID).
		First(&agent).Error
	if err != nil {
		return nil, err
	}
	return &agent, nil
}

func writeSkillLookupError(w http.ResponseWriter, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "lookup failed"})
}

func toAgentSkillResponse(link model.AgentSkill, skill model.Skill) agentSkillResponse {
	resp := agentSkillResponse{
		SkillID:   link.SkillID.String(),
		CreatedAt: link.CreatedAt,
		Skill:     toSkillResponse(skill, nil),
	}
	if link.PinnedVersionID != nil {
		pid := link.PinnedVersionID.String()
		resp.PinnedVersionID = &pid
	}
	return resp
}
