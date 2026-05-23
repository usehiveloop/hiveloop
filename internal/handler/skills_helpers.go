package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/skills"
)

func toSkillResponse(s model.Skill) skillResponse {
	resp := skillResponse{
		ID:             s.ID.String(),
		Slug:           s.Slug,
		Name:           s.Name,
		Description:    s.Description,
		SourceType:     s.SourceType,
		RepoURL:        s.RepoURL,
		RepoSubpath:    s.RepoSubpath,
		RepoRef:        s.RepoRef,
		Tags:           []string(s.Tags),
		IntegrationIDs: []string(s.IntegrationIDs),
		InstallCount:   s.InstallCount,
		Featured:       s.Featured,
		Status:         s.Status,
		CreatedAt:      s.CreatedAt,
		UpdatedAt:      s.UpdatedAt,
	}
	if s.OrgID != nil {
		orgIDStr := s.OrgID.String()
		resp.OrgID = &orgIDStr
	}
	if s.PublicSkillID != nil {
		publicIDStr := s.PublicSkillID.String()
		resp.PublicSkillID = &publicIDStr
	}
	if resp.Tags == nil {
		resp.Tags = []string{}
	}
	if resp.IntegrationIDs == nil {
		resp.IntegrationIDs = []string{}
	}

	switch {
	case len(s.Bundle) == 0 || string(s.Bundle) == "{}" || string(s.Bundle) == "null":
		resp.HydrationStatus = "pending"
	case s.HydrationError != nil:
		resp.HydrationStatus = "error"
		resp.HydrationError = s.HydrationError
	default:
		resp.HydrationStatus = "ready"
	}

	return resp
}

func toSkillDetailResponse(s model.Skill) skillDetailResponse {
	detail := skillDetailResponse{skillResponse: toSkillResponse(s)}
	if len(s.Bundle) == 0 {
		return detail
	}
	var bundle skills.Bundle
	if err := json.Unmarshal(s.Bundle, &bundle); err == nil {
		detail.Bundle = &bundle
	}
	return detail
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
		Skill:     toSkillResponse(skill),
	}
	return resp
}
