package handler

import (
	"time"

	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/skills"
)

type adminSkillResponse struct {
	ID           string   `json:"id"`
	OrgID        *string  `json:"org_id"`
	PublisherID  *string  `json:"publisher_id"`
	Slug         string   `json:"slug"`
	Name         string   `json:"name"`
	Description  *string  `json:"description"`
	SourceType   string   `json:"source_type"`
	RepoURL      *string  `json:"repo_url"`
	RepoSubpath  *string  `json:"repo_subpath"`
	RepoRef      string   `json:"repo_ref"`
	Tags         []string `json:"tags"`
	InstallCount int      `json:"install_count"`
	Featured     bool     `json:"featured"`
	Status       string   `json:"status"`
	CreatedAt    string   `json:"created_at"`
	UpdatedAt    string   `json:"updated_at"`
}

func toAdminSkillResponse(skill model.Skill) adminSkillResponse {
	resp := adminSkillResponse{
		ID:           skill.ID.String(),
		Slug:         skill.Slug,
		Name:         skill.Name,
		Description:  skill.Description,
		SourceType:   skill.SourceType,
		RepoURL:      skill.RepoURL,
		RepoSubpath:  skill.RepoSubpath,
		RepoRef:      skill.RepoRef,
		Tags:         skill.Tags,
		InstallCount: skill.InstallCount,
		Featured:     skill.Featured,
		Status:       skill.Status,
		CreatedAt:    skill.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    skill.UpdatedAt.Format(time.RFC3339),
	}
	if skill.OrgID != nil {
		orgIDStr := skill.OrgID.String()
		resp.OrgID = &orgIDStr
	}
	if skill.PublisherID != nil {
		pubIDStr := skill.PublisherID.String()
		resp.PublisherID = &pubIDStr
	}
	if resp.Tags == nil {
		resp.Tags = []string{}
	}
	return resp
}

type adminCreateSkillRequest struct {
	Name        string         `json:"name"`
	Description *string        `json:"description,omitempty"`
	SourceType  string         `json:"source_type"`
	Tags        []string       `json:"tags,omitempty"`
	Status      string         `json:"status,omitempty"`
	Featured    bool           `json:"featured,omitempty"`
	Bundle      *skills.Bundle `json:"bundle,omitempty"`
	RepoURL     *string        `json:"repo_url,omitempty"`
	RepoSubpath *string        `json:"repo_subpath,omitempty"`
	RepoRef     *string        `json:"repo_ref,omitempty"`
}

type adminUpdateSkillRequest struct {
	Name        *string   `json:"name,omitempty"`
	Description *string   `json:"description,omitempty"`
	Status      *string   `json:"status,omitempty"`
	Featured    *bool     `json:"featured,omitempty"`
	Tags        *[]string `json:"tags,omitempty"`
	RepoRef     *string   `json:"repo_ref,omitempty"`
}
