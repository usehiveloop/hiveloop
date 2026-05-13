package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

type TeamHandler struct {
	db *gorm.DB
}

func NewTeamHandler(db *gorm.DB) *TeamHandler {
	return &TeamHandler{db: db}
}

type createTeamRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	PromptTeam  string `json:"prompt_team,omitempty"`
}

type updateTeamRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	PromptTeam  *string `json:"prompt_team,omitempty"`
}

type teamResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	PromptTeam  string `json:"prompt_team,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

func toTeamResponse(t model.Team) teamResponse {
	return teamResponse{
		ID:          t.ID.String(),
		Name:        t.Name,
		Description: t.Description,
		PromptTeam:  t.PromptTeam,
		CreatedAt:   t.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   t.UpdatedAt.Format(time.RFC3339),
	}
}

// Create handles POST /v1/teams.
// @Summary Create a team
// @Description Creates a new team in the current organization. Admin/owner only.
// @Tags teams
// @Accept json
// @Produce json
// @Param body body createTeamRequest true "Team details"
// @Success 201 {object} teamResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Security BearerAuth
// @Router /v1/teams [post]
func (h *TeamHandler) Create(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	var req createTeamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	if exists, err := h.teamNameExists(org.ID, name, nil); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create team"})
		return
	} else if exists {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "a team with this name already exists"})
		return
	}

	team := model.Team{
		OrgID:       org.ID,
		Name:        name,
		Description: strings.TrimSpace(req.Description),
		PromptTeam:  strings.TrimSpace(req.PromptTeam),
	}

	if err := h.db.Create(&team).Error; err != nil {
		if isUniqueViolation(err) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "a team with this name already exists"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create team"})
		return
	}

	writeJSON(w, http.StatusCreated, toTeamResponse(team))
}

// List handles GET /v1/teams.
// @Summary List teams
// @Description Returns teams in the current organization.
// @Tags teams
// @Produce json
// @Param limit query int false "Page size (default 50, max 100)"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[teamResponse]
// @Failure 401 {object} errorResponse
// @Security BearerAuth
// @Router /v1/teams [get]
func (h *TeamHandler) List(w http.ResponseWriter, r *http.Request) {
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

	q := h.db.Where("org_id = ? AND deleted_at IS NULL", org.ID)
	q = applyPagination(q, cursor, limit)

	var teams []model.Team
	if err := q.Find(&teams).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list teams"})
		return
	}

	hasMore := len(teams) > limit
	if hasMore {
		teams = teams[:limit]
	}

	resp := make([]teamResponse, len(teams))
	for i, t := range teams {
		resp[i] = toTeamResponse(t)
	}

	result := paginatedResponse[teamResponse]{Data: resp, HasMore: hasMore}
	if hasMore {
		last := teams[len(teams)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}

	writeJSON(w, http.StatusOK, result)
}

// Get handles GET /v1/teams/{id}.
// @Summary Get a team
// @Tags teams
// @Produce json
// @Param id path string true "Team ID"
// @Success 200 {object} teamResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/teams/{id} [get]
func (h *TeamHandler) Get(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "team not found"})
		return
	}

	var team model.Team
	if err := h.db.Where("id = ? AND org_id = ? AND deleted_at IS NULL", id, org.ID).First(&team).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "team not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get team"})
		return
	}

	writeJSON(w, http.StatusOK, toTeamResponse(team))
}

// Update handles PATCH /v1/teams/{id}.
// @Summary Update a team
// @Tags teams
// @Accept json
// @Produce json
// @Param id path string true "Team ID"
// @Param body body updateTeamRequest true "Team updates"
// @Success 200 {object} teamResponse
// @Failure 400 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Security BearerAuth
// @Router /v1/teams/{id} [patch]
func (h *TeamHandler) Update(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "team not found"})
		return
	}

	var req updateTeamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	var team model.Team
	if err := h.db.Where("id = ? AND org_id = ? AND deleted_at IS NULL", id, org.ID).First(&team).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "team not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update team"})
		return
	}

	updates := map[string]any{}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name cannot be empty"})
			return
		}
		if exists, err := h.teamNameExists(org.ID, name, &team.ID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update team"})
			return
		} else if exists {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "a team with this name already exists"})
			return
		}
		updates["name"] = name
	}
	if req.Description != nil {
		updates["description"] = strings.TrimSpace(*req.Description)
	}
	if req.PromptTeam != nil {
		updates["prompt_team"] = strings.TrimSpace(*req.PromptTeam)
	}

	if len(updates) == 0 {
		writeJSON(w, http.StatusOK, toTeamResponse(team))
		return
	}

	if err := h.db.Model(&team).Updates(updates).Error; err != nil {
		if isUniqueViolation(err) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "a team with this name already exists"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update team"})
		return
	}

	writeJSON(w, http.StatusOK, toTeamResponse(team))
}

// Delete handles DELETE /v1/teams/{id}.
// @Summary Delete a team
// @Tags teams
// @Produce json
// @Param id path string true "Team ID"
// @Success 200 {object} map[string]string
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/teams/{id} [delete]
func (h *TeamHandler) Delete(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "team not found"})
		return
	}

	now := time.Now()
	res := h.db.Model(&model.Team{}).
		Where("id = ? AND org_id = ? AND deleted_at IS NULL", id, org.ID).
		Update("deleted_at", &now)
	if res.Error != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete team"})
		return
	}
	if res.RowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "team not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "SQLSTATE 23505") ||
		strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "duplicate key value")
}

func (h *TeamHandler) teamNameExists(orgID uuid.UUID, name string, excludeID *uuid.UUID) (bool, error) {
	var count int64
	query := h.db.Model(&model.Team{}).
		Where("org_id = ? AND name = ? AND deleted_at IS NULL", orgID, name)
	if excludeID != nil {
		query = query.Where("id <> ?", *excludeID)
	}
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
