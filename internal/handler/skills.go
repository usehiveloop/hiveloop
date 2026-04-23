package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/enqueue"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/skills"
	"github.com/usehiveloop/hiveloop/internal/tasks"
)

// SkillHandler serves the skills marketplace + per-agent attach/detach API.
type SkillHandler struct {
	db       *gorm.DB
	enqueuer enqueue.TaskEnqueuer
}

func NewSkillHandler(db *gorm.DB, enqueuer enqueue.TaskEnqueuer) *SkillHandler {
	return &SkillHandler{db: db, enqueuer: enqueuer}
}

type createSkillRequest struct {
	Name        string   `json:"name"`
	Description *string  `json:"description,omitempty"`
	SourceType  string   `json:"source_type"` // "inline" | "git"
	Tags        []string `json:"tags,omitempty"`

	// Inline source
	Bundle *skills.Bundle `json:"bundle,omitempty"`

	// Git source
	RepoURL     *string `json:"repo_url,omitempty"`
	RepoSubpath *string `json:"repo_subpath,omitempty"`
	RepoRef     *string `json:"repo_ref,omitempty"`
}

type updateSkillRequest struct {
	Name        *string   `json:"name,omitempty"`
	Description *string   `json:"description,omitempty"`
	Tags        *[]string `json:"tags,omitempty"`
	RepoRef     *string   `json:"repo_ref,omitempty"`
	Status      *string   `json:"status,omitempty"`
}

type updateContentRequest struct {
	Bundle skills.Bundle `json:"bundle"`
}

type skillResponse struct {
	ID              string    `json:"id"`
	OrgID           *string   `json:"org_id,omitempty"`
	Slug            string    `json:"slug"`
	Name            string    `json:"name"`
	Description     *string   `json:"description,omitempty"`
	SourceType      string    `json:"source_type"`
	RepoURL         *string   `json:"repo_url,omitempty"`
	RepoSubpath     *string   `json:"repo_subpath,omitempty"`
	RepoRef         string    `json:"repo_ref"`
	LatestVersionID *string   `json:"latest_version_id,omitempty"`
	Tags            []string  `json:"tags"`
	InstallCount    int       `json:"install_count"`
	Featured        bool      `json:"featured"`
	Status          string    `json:"status"`
	PublicSkillID   *string   `json:"public_skill_id,omitempty"`
	HydrationStatus string    `json:"hydration_status"`          // pending, ready, error
	HydrationError  *string   `json:"hydration_error,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type skillDetailResponse struct {
	skillResponse
	Bundle         *skills.Bundle `json:"bundle,omitempty"`
	HydrationError *string        `json:"hydration_error,omitempty"`
}

type skillVersionResponse struct {
	ID             string    `json:"id"`
	Version        string    `json:"version"`
	CommitSHA      *string   `json:"commit_sha,omitempty"`
	HydratedAt     *string   `json:"hydrated_at,omitempty"`
	HydrationError *string   `json:"hydration_error,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type attachSkillRequest struct {
	SkillID         string  `json:"skill_id"`
	PinnedVersionID *string `json:"pinned_version_id,omitempty"`
}

type agentSkillResponse struct {
	SkillID         string        `json:"skill_id"`
	PinnedVersionID *string       `json:"pinned_version_id,omitempty"`
	CreatedAt       time.Time     `json:"created_at"`
	Skill           skillResponse `json:"skill"`
}

// Create handles POST /v1/skills.
// @Summary Create a skill
// @Description Creates an inline-authored skill or registers a git-sourced skill for hydration.
// @Tags skills
// @Accept json
// @Produce json
// @Param body body createSkillRequest true "Skill details"
// @Success 201 {object} skillDetailResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/skills [post]
func (h *SkillHandler) Create(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	var req createSkillRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	switch req.SourceType {
	case model.SkillSourceInline, model.SkillSourceGit:
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "source_type must be 'inline' or 'git'"})
		return
	}

	orgID := org.ID
	skill := model.Skill{
		OrgID:       &orgID,
		Slug:        model.GenerateSlug(req.Name),
		Name:        req.Name,
		Description: req.Description,
		SourceType:  req.SourceType,
		Tags:        req.Tags,
		Status:      model.SkillStatusDraft,
	}

	if req.SourceType == model.SkillSourceGit {
		if req.RepoURL == nil || *req.RepoURL == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "repo_url is required for git skills"})
			return
		}
		skill.RepoURL = req.RepoURL
		skill.RepoSubpath = req.RepoSubpath
		if req.RepoRef != nil && *req.RepoRef != "" {
			skill.RepoRef = *req.RepoRef
		} else {
			skill.RepoRef = "main"
		}
	}

	if err := h.db.Create(&skill).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create skill"})
		return
	}

	var latest *model.SkillVersion

	if req.SourceType == model.SkillSourceInline {
		if req.Bundle == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bundle is required for inline skills"})
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
		sv, err := skills.HydrateInline(r.Context(), h.db, skill.ID, req.Bundle, "v1")
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create skill version"})
			return
		}
		latest = sv
	} else {
		if h.enqueuer != nil {
			task, err := tasks.NewSkillHydrateTask(skill.ID)
			if err == nil {
				_, _ = h.enqueuer.Enqueue(task)
			}
		}
	}

	// Reload to pick up latest_version_id if we hydrated inline.
	_ = h.db.First(&skill, "id = ?", skill.ID).Error

	writeJSON(w, http.StatusCreated, toSkillDetailResponse(skill, latest))
}
