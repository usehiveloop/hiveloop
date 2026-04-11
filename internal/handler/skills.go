package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/enqueue"
	"github.com/ziraloop/ziraloop/internal/middleware"
	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/skills"
	"github.com/ziraloop/ziraloop/internal/tasks"
)

// SkillHandler serves the skills marketplace + per-agent attach/detach API.
type SkillHandler struct {
	db       *gorm.DB
	enqueuer enqueue.TaskEnqueuer
}

// NewSkillHandler constructs a SkillHandler.
func NewSkillHandler(db *gorm.DB, enqueuer enqueue.TaskEnqueuer) *SkillHandler {
	return &SkillHandler{db: db, enqueuer: enqueuer}
}

// ---------------------------------------------------------------------------
// Request / response shapes
// ---------------------------------------------------------------------------

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
	SkillID         string    `json:"skill_id"`
	PinnedVersionID *string   `json:"pinned_version_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	Skill           skillResponse `json:"skill"`
}

// ---------------------------------------------------------------------------
// Marshaling helpers
// ---------------------------------------------------------------------------

func toSkillResponse(s model.Skill) skillResponse {
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
	if resp.Tags == nil {
		resp.Tags = []string{}
	}
	return resp
}

func toSkillDetailResponse(s model.Skill, latest *model.SkillVersion) skillDetailResponse {
	detail := skillDetailResponse{skillResponse: toSkillResponse(s)}
	if latest == nil {
		return detail
	}
	detail.HydrationError = latest.HydrationError
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

// ---------------------------------------------------------------------------
// Skill CRUD
// ---------------------------------------------------------------------------

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

// List handles GET /v1/skills.
// @Summary List skills
// @Description Lists skills visible to the current org. Use scope=public to browse the marketplace, scope=own for org skills, scope=all for both. Pass q to search by name/description.
// @Tags skills
// @Produce json
// @Param scope query string false "Filter: public, own, all (default all)"
// @Param q query string false "Free-text search over name and description"
// @Param limit query int false "Page size (default 50, max 100)"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[skillResponse]
// @Failure 401 {object} errorResponse
// @Security BearerAuth
// @Router /v1/skills [get]
func (h *SkillHandler) List(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	scope := r.URL.Query().Get("scope")
	q := h.db.Model(&model.Skill{})
	switch scope {
	case "public":
		q = q.Where("org_id IS NULL AND status = ?", model.SkillStatusPublished)
	case "own":
		q = q.Where("org_id = ?", org.ID)
	case "", "all":
		q = q.Where("org_id = ? OR (org_id IS NULL AND status = ?)", org.ID, model.SkillStatusPublished)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "scope must be public, own, or all"})
		return
	}
	if searchTerm := strings.TrimSpace(r.URL.Query().Get("q")); searchTerm != "" {
		like := "%" + searchTerm + "%"
		q = q.Where("name ILIKE ? OR description ILIKE ?", like, like)
	}
	q = applyPagination(q, cursor, limit)

	var rows []model.Skill
	if err := q.Find(&rows).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list skills"})
		return
	}

	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	resp := make([]skillResponse, len(rows))
	for i, s := range rows {
		resp[i] = toSkillResponse(s)
	}
	result := paginatedResponse[skillResponse]{Data: resp, HasMore: hasMore}
	if hasMore {
		last := rows[len(rows)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}
	writeJSON(w, http.StatusOK, result)
}

// Get handles GET /v1/skills/{id}.
// @Summary Get a skill
// @Description Returns a skill with its latest hydrated bundle.
// @Tags skills
// @Produce json
// @Param id path string true "Skill ID"
// @Success 200 {object} skillDetailResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/skills/{id} [get]
func (h *SkillHandler) Get(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	skill, err := h.loadSkillVisibleToOrg(r.Context(), chi.URLParam(r, "id"), org.ID)
	if err != nil {
		writeSkillLookupError(w, err)
		return
	}

	var latest *model.SkillVersion
	if skill.LatestVersionID != nil {
		var sv model.SkillVersion
		if err := h.db.First(&sv, "id = ?", *skill.LatestVersionID).Error; err == nil {
			latest = &sv
		}
	}
	writeJSON(w, http.StatusOK, toSkillDetailResponse(*skill, latest))
}

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
	writeJSON(w, http.StatusOK, toSkillResponse(*skill))
}

// Delete handles DELETE /v1/skills/{id}. Soft-deletes by marking archived.
// @Summary Archive a skill
// @Description Marks an org-owned skill as archived. Public skills cannot be deleted via this endpoint.
// @Tags skills
// @Produce json
// @Param id path string true "Skill ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/skills/{id} [delete]
func (h *SkillHandler) Delete(w http.ResponseWriter, r *http.Request) {
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
	if err := h.db.Model(skill).Update("status", model.SkillStatusArchived).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to archive skill"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "archived"})
}

// Hydrate handles POST /v1/skills/{id}/hydrate.
// @Summary Re-hydrate a git-sourced skill
// @Description Enqueues a fresh git pull at the tracked ref. Only valid for git-sourced skills.
// @Tags skills
// @Produce json
// @Param id path string true "Skill ID"
// @Success 202 {object} map[string]string
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/skills/{id}/hydrate [post]
func (h *SkillHandler) Hydrate(w http.ResponseWriter, r *http.Request) {
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
	if skill.SourceType != model.SkillSourceGit {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "skill is not git-sourced"})
		return
	}
	if h.enqueuer == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "hydration worker not configured"})
		return
	}
	task, err := tasks.NewSkillHydrateTask(skill.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to build hydrate task"})
		return
	}
	if _, err := h.enqueuer.Enqueue(task); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to enqueue hydrate task"})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "hydrating"})
}

// ListVersions handles GET /v1/skills/{id}/versions.
// @Summary List skill versions
// @Description Returns all SkillVersion rows for a skill, newest first.
// @Tags skills
// @Produce json
// @Param id path string true "Skill ID"
// @Success 200 {array} skillVersionResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/skills/{id}/versions [get]
func (h *SkillHandler) ListVersions(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	skill, err := h.loadSkillVisibleToOrg(r.Context(), chi.URLParam(r, "id"), org.ID)
	if err != nil {
		writeSkillLookupError(w, err)
		return
	}

	var versions []model.SkillVersion
	if err := h.db.Where("skill_id = ?", skill.ID).Order("created_at DESC").Find(&versions).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list versions"})
		return
	}
	resp := make([]skillVersionResponse, len(versions))
	for i, v := range versions {
		resp[i] = toSkillVersionResponse(v)
	}
	writeJSON(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// Agent-skill attach / detach
// ---------------------------------------------------------------------------

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

	// Best-effort install counter bump; ignore errors.
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

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

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
	if link.PinnedVersionID != nil {
		pid := link.PinnedVersionID.String()
		resp.PinnedVersionID = &pid
	}
	return resp
}
