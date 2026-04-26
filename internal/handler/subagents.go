package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

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
	ID           string  `json:"id"`
	OrgID        *string `json:"org_id,omitempty"`
	Name         string  `json:"name"`
	Description  *string `json:"description,omitempty"`
	SystemPrompt string  `json:"system_prompt"`
	Model        string  `json:"model"`
	Status       string  `json:"status"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
}

type attachSubagentRequest struct {
	SubagentID string `json:"subagent_id"`
}

type agentSubagentResponse struct {
	SubagentID string           `json:"subagent_id"`
	CreatedAt  string           `json:"created_at"`
	Subagent   subagentResponse `json:"subagent"`
}

// Create handles POST /v1/subagents.
// @Summary Create a subagent
// @Description Creates a reusable subagent that parent agents can invoke. Does not require a credential.
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
