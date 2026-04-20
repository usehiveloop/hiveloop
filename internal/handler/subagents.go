package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// SubagentHandler serves subagent CRUD + per-agent attach/detach.
type SubagentHandler struct {
	db *gorm.DB
}

func NewSubagentHandler(db *gorm.DB) *SubagentHandler {
	return &SubagentHandler{db: db}
}

// ---------------------------------------------------------------------------
// Request / response shapes
// ---------------------------------------------------------------------------

type createSubagentRequest struct {
	Name         string     `json:"name"`
	Description  *string    `json:"description,omitempty"`
	SystemPrompt string     `json:"system_prompt"`
	Model        string     `json:"model,omitempty"`
	Tools        model.JSON `json:"tools,omitempty"`
	McpServers   model.JSON `json:"mcp_servers,omitempty"`
	Skills       model.JSON `json:"skills,omitempty"`
	AgentConfig  model.JSON `json:"agent_config,omitempty"`
	Permissions  model.JSON `json:"permissions,omitempty"`
	Tags         []string   `json:"tags,omitempty"`
}

type updateSubagentRequest struct {
	Name         *string    `json:"name,omitempty"`
	Description  *string    `json:"description,omitempty"`
	SystemPrompt *string    `json:"system_prompt,omitempty"`
	Model        *string    `json:"model,omitempty"`
	Tools        model.JSON `json:"tools,omitempty"`
	McpServers   model.JSON `json:"mcp_servers,omitempty"`
	Skills       model.JSON `json:"skills,omitempty"`
	AgentConfig  model.JSON `json:"agent_config,omitempty"`
	Permissions  model.JSON `json:"permissions,omitempty"`
	Status       *string    `json:"status,omitempty"`
}

type subagentResponse struct {
	ID           string   `json:"id"`
	OrgID        *string  `json:"org_id,omitempty"`
	Name         string   `json:"name"`
	Description  *string  `json:"description,omitempty"`
	SystemPrompt string   `json:"system_prompt"`
	Model        string   `json:"model"`
	Status       string   `json:"status"`
	CreatedAt    string   `json:"created_at"`
	UpdatedAt    string   `json:"updated_at"`
}

type attachSubagentRequest struct {
	SubagentID string `json:"subagent_id"`
}

type agentSubagentResponse struct {
	SubagentID string           `json:"subagent_id"`
	CreatedAt  string           `json:"created_at"`
	Subagent   subagentResponse `json:"subagent"`
}

func toSubagentResponse(agent model.Agent) subagentResponse {
	resp := subagentResponse{
		ID:           agent.ID.String(),
		Name:         agent.Name,
		Description:  agent.Description,
		SystemPrompt: agent.SystemPrompt,
		Model:        agent.Model,
		Status:       agent.Status,
		CreatedAt:    agent.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:    agent.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if agent.OrgID != nil {
		orgIDStr := agent.OrgID.String()
		resp.OrgID = &orgIDStr
	}
	return resp
}

// ---------------------------------------------------------------------------
// Subagent CRUD
// ---------------------------------------------------------------------------

// Create handles POST /v1/subagents.
// @Summary Create a subagent
// @Description Creates a reusable subagent that parent agents can invoke. Does not require sandbox_type or credential.
// @Tags subagents
// @Accept json
// @Produce json
// @Param body body createSubagentRequest true "Subagent definition"
// @Success 201 {object} subagentResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Security BearerAuth
// @Router /v1/subagents [post]
func (h *SubagentHandler) Create(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	var req createSubagentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	if req.SystemPrompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "system_prompt is required"})
		return
	}

	orgID := org.ID
	agent := model.Agent{
		OrgID:        &orgID,
		Name:         req.Name,
		Description:  req.Description,
		SystemPrompt: req.SystemPrompt,
		Model:        req.Model,
		SandboxType:  "",
		Tools:        defaultJSON(req.Tools),
		McpServers:   defaultJSON(req.McpServers),
		Skills:       defaultJSON(req.Skills),
		AgentConfig:  defaultJSON(req.AgentConfig),
		Permissions:  defaultJSON(req.Permissions),
		AgentType:    model.AgentTypeSubagent,
		Status:       "active",
	}

	if err := h.db.Create(&agent).Error; err != nil {
		if isDuplicateKeyError(err) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": fmt.Sprintf("subagent with name %q already exists", req.Name)})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create subagent"})
		return
	}

	writeJSON(w, http.StatusCreated, toSubagentResponse(agent))
}

// List handles GET /v1/subagents.
// @Summary List subagents
// @Description Lists subagents visible to the current org. Use scope=public, own, or all.
// @Tags subagents
// @Produce json
// @Param scope query string false "Filter: public, own, all (default all)"
// @Param q query string false "Free-text search over name and description"
// @Param limit query int false "Page size (default 50, max 100)"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[subagentResponse]
// @Failure 401 {object} errorResponse
// @Security BearerAuth
// @Router /v1/subagents [get]
func (h *SubagentHandler) List(w http.ResponseWriter, r *http.Request) {
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
	query := h.db.Model(&model.Agent{}).Where("agent_type = ?", model.AgentTypeSubagent)
	switch scope {
	case "public":
		query = query.Where("org_id IS NULL AND status = ?", "active")
	case "own":
		query = query.Where("org_id = ?", org.ID)
	case "", "all":
		query = query.Where("org_id = ? OR (org_id IS NULL AND status = ?)", org.ID, "active")
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "scope must be public, own, or all"})
		return
	}
	if searchTerm := strings.TrimSpace(r.URL.Query().Get("q")); searchTerm != "" {
		like := "%" + searchTerm + "%"
		query = query.Where("name ILIKE ? OR description ILIKE ?", like, like)
	}
	query = applyPagination(query, cursor, limit)

	var rows []model.Agent
	if err := query.Find(&rows).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list subagents"})
		return
	}

	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	resp := make([]subagentResponse, len(rows))
	for index, agent := range rows {
		resp[index] = toSubagentResponse(agent)
	}
	result := paginatedResponse[subagentResponse]{Data: resp, HasMore: hasMore}
	if hasMore {
		last := rows[len(rows)-1]
		cursor := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &cursor
	}
	writeJSON(w, http.StatusOK, result)
}

// Get handles GET /v1/subagents/{id}.
// @Summary Get a subagent
// @Tags subagents
// @Produce json
// @Param id path string true "Subagent ID"
// @Success 200 {object} subagentResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/subagents/{id} [get]
func (h *SubagentHandler) Get(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	sub, err := h.loadVisibleSubagent(chi.URLParam(r, "id"), org.ID)
	if err != nil {
		writeSubagentLookupError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toSubagentResponse(*sub))
}

// Update handles PATCH /v1/subagents/{id}.
// @Summary Update a subagent
// @Tags subagents
// @Accept json
// @Produce json
// @Param id path string true "Subagent ID"
// @Param body body updateSubagentRequest true "Fields to update"
// @Success 200 {object} subagentResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/subagents/{id} [patch]
func (h *SubagentHandler) Update(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	sub, err := h.loadOwnSubagent(chi.URLParam(r, "id"), org.ID)
	if err != nil {
		writeSubagentLookupError(w, err)
		return
	}

	var req updateSubagentRequest
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
	if req.SystemPrompt != nil {
		updates["system_prompt"] = *req.SystemPrompt
	}
	if req.Model != nil {
		updates["model"] = *req.Model
	}
	if req.Tools != nil {
		updates["tools"] = req.Tools
	}
	if req.McpServers != nil {
		updates["mcp_servers"] = req.McpServers
	}
	if req.Skills != nil {
		updates["skills"] = req.Skills
	}
	if req.AgentConfig != nil {
		updates["agent_config"] = req.AgentConfig
	}
	if req.Permissions != nil {
		updates["permissions"] = req.Permissions
	}
	if req.Status != nil {
		switch *req.Status {
		case "active", "archived":
			updates["status"] = *req.Status
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "status must be active or archived"})
			return
		}
	}
	if len(updates) > 0 {
		if err := h.db.Model(sub).Updates(updates).Error; err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update subagent"})
			return
		}
		_ = h.db.First(sub, "id = ?", sub.ID).Error
	}
	writeJSON(w, http.StatusOK, toSubagentResponse(*sub))
}

// Delete handles DELETE /v1/subagents/{id}.
// @Summary Archive a subagent
// @Tags subagents
// @Produce json
// @Param id path string true "Subagent ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/subagents/{id} [delete]
func (h *SubagentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	sub, err := h.loadOwnSubagent(chi.URLParam(r, "id"), org.ID)
	if err != nil {
		writeSubagentLookupError(w, err)
		return
	}
	if err := h.db.Model(sub).Update("status", "archived").Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to archive subagent"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "archived"})
}

// ---------------------------------------------------------------------------
// Agent-subagent attach / detach
// ---------------------------------------------------------------------------

// AttachToAgent handles POST /v1/agents/{agentID}/subagents.
// @Summary Attach a subagent to an agent
// @Tags subagents
// @Accept json
// @Produce json
// @Param agentID path string true "Parent agent ID"
// @Param body body attachSubagentRequest true "Subagent to attach"
// @Success 201 {object} agentSubagentResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{agentID}/subagents [post]
func (h *SubagentHandler) AttachToAgent(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	parentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
		return
	}
	var parent model.Agent
	if err := h.db.Where("id = ? AND org_id = ? AND agent_type = ?", parentID, org.ID, model.AgentTypeAgent).First(&parent).Error; err != nil {
		writeSubagentLookupError(w, err)
		return
	}

	var req attachSubagentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	subID, err := uuid.Parse(req.SubagentID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid subagent_id"})
		return
	}
	sub, err := h.loadVisibleSubagent(subID.String(), org.ID)
	if err != nil {
		writeSubagentLookupError(w, err)
		return
	}

	link := model.AgentSubagent{AgentID: parent.ID, SubagentID: sub.ID}
	if err := h.db.Save(&link).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to attach subagent"})
		return
	}

	writeJSON(w, http.StatusCreated, agentSubagentResponse{
		SubagentID: sub.ID.String(),
		CreatedAt:  link.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		Subagent:   toSubagentResponse(*sub),
	})
}

// DetachFromAgent handles DELETE /v1/agents/{agentID}/subagents/{subagentID}.
// @Summary Detach a subagent from an agent
// @Tags subagents
// @Produce json
// @Param agentID path string true "Parent agent ID"
// @Param subagentID path string true "Subagent ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{agentID}/subagents/{subagentID} [delete]
func (h *SubagentHandler) DetachFromAgent(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	parentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
		return
	}
	var parent model.Agent
	if err := h.db.Where("id = ? AND org_id = ?", parentID, org.ID).First(&parent).Error; err != nil {
		writeSubagentLookupError(w, err)
		return
	}
	subID, err := uuid.Parse(chi.URLParam(r, "subagentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid subagent id"})
		return
	}
	result := h.db.Where("agent_id = ? AND subagent_id = ?", parent.ID, subID).Delete(&model.AgentSubagent{})
	if result.Error != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to detach subagent"})
		return
	}
	if result.RowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "subagent not attached to agent"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "detached"})
}

// ListAgentSubagents handles GET /v1/agents/{agentID}/subagents.
// @Summary List subagents attached to an agent
// @Tags subagents
// @Produce json
// @Param agentID path string true "Parent agent ID"
// @Success 200 {array} agentSubagentResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{agentID}/subagents [get]
func (h *SubagentHandler) ListAgentSubagents(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	parentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
		return
	}
	var parent model.Agent
	if err := h.db.Where("id = ? AND org_id = ?", parentID, org.ID).First(&parent).Error; err != nil {
		writeSubagentLookupError(w, err)
		return
	}

	var links []model.AgentSubagent
	if err := h.db.Where("agent_id = ?", parent.ID).Find(&links).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list subagents"})
		return
	}
	if len(links) == 0 {
		writeJSON(w, http.StatusOK, []agentSubagentResponse{})
		return
	}

	subIDs := make([]uuid.UUID, len(links))
	for index, link := range links {
		subIDs[index] = link.SubagentID
	}
	var subs []model.Agent
	if err := h.db.Where("id IN ?", subIDs).Find(&subs).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load subagents"})
		return
	}
	subByID := make(map[uuid.UUID]model.Agent, len(subs))
	for _, agent := range subs {
		subByID[agent.ID] = agent
	}

	resp := make([]agentSubagentResponse, 0, len(links))
	for _, link := range links {
		agent, ok := subByID[link.SubagentID]
		if !ok {
			continue
		}
		resp = append(resp, agentSubagentResponse{
			SubagentID: link.SubagentID.String(),
			CreatedAt:  link.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			Subagent:   toSubagentResponse(agent),
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

func (h *SubagentHandler) loadVisibleSubagent(id string, orgID uuid.UUID) (*model.Agent, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return nil, gorm.ErrRecordNotFound
	}
	var agent model.Agent
	err = h.db.
		Where("id = ? AND agent_type = ? AND (org_id = ? OR (org_id IS NULL AND status = ?))",
			parsed, model.AgentTypeSubagent, orgID, "active").
		First(&agent).Error
	if err != nil {
		return nil, err
	}
	return &agent, nil
}

func (h *SubagentHandler) loadOwnSubagent(id string, orgID uuid.UUID) (*model.Agent, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return nil, gorm.ErrRecordNotFound
	}
	var agent model.Agent
	err = h.db.
		Where("id = ? AND agent_type = ? AND org_id = ?", parsed, model.AgentTypeSubagent, orgID).
		First(&agent).Error
	if err != nil {
		return nil, err
	}
	return &agent, nil
}

func writeSubagentLookupError(w http.ResponseWriter, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "lookup failed"})
}
