package handler

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/tasks"
)

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
	versionMap := h.loadVersionMap(rows)
	resp := make([]skillResponse, len(rows))
	for i, s := range rows {
		var version *model.SkillVersion
		if s.LatestVersionID != nil {
			if sv, ok := versionMap[*s.LatestVersionID]; ok {
				version = &sv
			}
		}
		resp[i] = toSkillResponse(s, version)
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
