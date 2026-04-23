package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)



// ListSkills handles GET /admin/v1/skills.
// @Summary List skills
// @Description Lists all skills with optional filters.
// @Tags admin
// @Produce json
// @Param status query string false "Filter by status"
// @Param scope query string false "Filter scope (global)"
// @Param source_type query string false "Filter by source type"
// @Param q query string false "Search by name"
// @Success 200 {object} paginatedResponse[adminSkillResponse]
// @Security BearerAuth
// @Router /admin/v1/skills [get]
func (h *AdminHandler) ListSkills(w http.ResponseWriter, r *http.Request) {
	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Model(&model.Skill{})
	if orgID := r.URL.Query().Get("org_id"); orgID != "" {
		q = q.Where("org_id = ?", orgID)
	}
	if scope := r.URL.Query().Get("scope"); scope == "global" {
		q = q.Where("org_id IS NULL")
	}
	if status := r.URL.Query().Get("status"); status != "" {
		q = q.Where("status = ?", status)
	}
	if sourceType := r.URL.Query().Get("source_type"); sourceType != "" {
		q = q.Where("source_type = ?", sourceType)
	}
	if search := r.URL.Query().Get("q"); search != "" {
		q = q.Where("name ILIKE ? OR slug ILIKE ?", "%"+search+"%", "%"+search+"%")
	}

	q = applyPagination(q, cursor, limit)

	var skills []model.Skill
	if err := q.Find(&skills).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list skills"})
		return
	}

	hasMore := len(skills) > limit
	if hasMore {
		skills = skills[:limit]
	}

	resp := make([]adminSkillResponse, len(skills))
	for index, skill := range skills {
		resp[index] = toAdminSkillResponse(skill)
	}

	result := paginatedResponse[adminSkillResponse]{Data: resp, HasMore: hasMore}
	if hasMore {
		last := skills[len(skills)-1]
		cursorStr := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &cursorStr
	}
	writeJSON(w, http.StatusOK, result)
}

// GetSkill handles GET /admin/v1/skills/{id}.
// @Summary Get skill details
// @Description Returns a skill by ID.
// @Tags admin
// @Produce json
// @Param id path string true "Skill ID"
// @Success 200 {object} adminSkillResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/skills/{id} [get]
func (h *AdminHandler) GetSkill(w http.ResponseWriter, r *http.Request) {
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

	writeJSON(w, http.StatusOK, toAdminSkillResponse(skill))
}

