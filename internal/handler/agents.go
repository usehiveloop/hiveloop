package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/crypto"
	"github.com/ziraloop/ziraloop/internal/enqueue"
	"github.com/ziraloop/ziraloop/internal/forge"
	"github.com/ziraloop/ziraloop/internal/middleware"
	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/registry"
	"github.com/ziraloop/ziraloop/internal/sandbox"
	"github.com/ziraloop/ziraloop/internal/tasks"
)

// AgentPusher is the interface the handler needs to push agents to Bridge.
// Satisfied by *sandbox.Pusher.
type AgentPusher interface {
	PushAgent(ctx context.Context, agent *model.Agent) error
	RemoveAgent(ctx context.Context, agent *model.Agent) error
}

type AgentHandler struct {
	db              *gorm.DB
	registry        *registry.Registry
	pusher          AgentPusher              // nil if sandbox orchestrator is not configured
	encKey          *crypto.SymmetricKey     // for encrypting env vars
	enqueuer        enqueue.TaskEnqueuer     // nil if worker not configured
	forgeController *forge.ForgeController   // nil if forge not configured
}

func NewAgentHandler(db *gorm.DB, reg *registry.Registry, pusher AgentPusher, encKey *crypto.SymmetricKey, enqueuer ...enqueue.TaskEnqueuer) *AgentHandler {
	h := &AgentHandler{db: db, registry: reg, pusher: pusher, encKey: encKey}
	if len(enqueuer) > 0 {
		h.enqueuer = enqueuer[0]
	}
	return h
}

// SetForgeController sets the forge controller for agent creation with forge=true.
func (h *AgentHandler) SetForgeController(fc *forge.ForgeController) {
	h.forgeController = fc
}

// ensure sandbox.Pusher satisfies AgentPusher
var _ AgentPusher = (*sandbox.Pusher)(nil)

type createAgentRequest struct {
	Name              string     `json:"name"`
	Description       *string    `json:"description,omitempty"`
	IdentityID        string     `json:"identity_id"`
	CredentialID      string     `json:"credential_id"`
	SandboxType       string     `json:"sandbox_type"`
	SandboxTemplateID *string    `json:"sandbox_template_id,omitempty"`
	SystemPrompt      string     `json:"system_prompt"`
	Instructions      *string    `json:"instructions,omitempty"`
	Model             string     `json:"model"`
	Tools             model.JSON `json:"tools,omitempty"`
	McpServers        model.JSON `json:"mcp_servers,omitempty"`
	Skills            model.JSON `json:"skills,omitempty"`
	Integrations      model.JSON `json:"integrations,omitempty"`
	Subagents         model.JSON `json:"subagents,omitempty"`
	AgentConfig       model.JSON `json:"agent_config,omitempty"`
	Permissions       model.JSON `json:"permissions,omitempty"`
	Team              string            `json:"team,omitempty"`
	SharedMemory      bool              `json:"shared_memory,omitempty"`
	Forge             *forgeOptions     `json:"forge,omitempty"` // triggers forge context gathering on create
}

type forgeOptions struct {
	JudgeCredentialID string `json:"judge_credential_id"`
	JudgeModel        string `json:"judge_model"`
}

type updateAgentRequest struct {
	Name              *string    `json:"name,omitempty"`
	Description       *string    `json:"description,omitempty"`
	CredentialID      *string    `json:"credential_id,omitempty"`
	SandboxType       *string    `json:"sandbox_type,omitempty"`
	SandboxTemplateID *string    `json:"sandbox_template_id,omitempty"`
	SystemPrompt      *string    `json:"system_prompt,omitempty"`
	Instructions      *string    `json:"instructions,omitempty"`
	Model             *string    `json:"model,omitempty"`
	Tools             model.JSON `json:"tools,omitempty"`
	McpServers        model.JSON `json:"mcp_servers,omitempty"`
	Skills            model.JSON `json:"skills,omitempty"`
	Integrations      model.JSON `json:"integrations,omitempty"`
	Subagents         model.JSON `json:"subagents,omitempty"`
	AgentConfig       model.JSON `json:"agent_config,omitempty"`
	Permissions       model.JSON `json:"permissions,omitempty"`
	Team              *string    `json:"team,omitempty"`
	SharedMemory      *bool      `json:"shared_memory,omitempty"`
}

type agentResponse struct {
	ID                string     `json:"id"`
	Name              string     `json:"name"`
	Description       *string    `json:"description,omitempty"`
	IdentityID        *string    `json:"identity_id,omitempty"`
	CredentialID      string     `json:"credential_id"`
	ProviderID        string     `json:"provider_id"`
	SandboxType       string     `json:"sandbox_type"`
	SandboxID         *string    `json:"sandbox_id,omitempty"`
	SandboxTemplateID *string    `json:"sandbox_template_id,omitempty"`
	SystemPrompt      string     `json:"system_prompt"`
	Instructions      *string    `json:"instructions,omitempty"`
	Model             string     `json:"model"`
	Tools             model.JSON `json:"tools"`
	McpServers        model.JSON `json:"mcp_servers"`
	Skills            model.JSON `json:"skills"`
	Integrations      model.JSON `json:"integrations"`
	Subagents         model.JSON `json:"subagents"`
	AgentConfig       model.JSON `json:"agent_config"`
	Permissions       model.JSON `json:"permissions"`
	Team              string     `json:"team"`
	SharedMemory      bool       `json:"shared_memory"`
	Status              string             `json:"status"`
	ForgeRunID          *string            `json:"forge_run_id,omitempty"`
	ForgeConversationID *string            `json:"forge_conversation_id,omitempty"`
	ForgeRun            *forgeRunResponse  `json:"forge_run,omitempty"`
	CreatedAt           string             `json:"created_at"`
	UpdatedAt           string             `json:"updated_at"`
}

func toAgentResponse(a model.Agent) agentResponse {
	resp := agentResponse{
		ID:           a.ID.String(),
		Name:         a.Name,
		Description:  a.Description,
		SandboxType:  a.SandboxType,
		SystemPrompt: a.SystemPrompt,
		Instructions: a.Instructions,
		Model:        a.Model,
		Tools:        a.Tools,
		McpServers:   a.McpServers,
		Skills:       a.Skills,
		Integrations: a.Integrations,
		Subagents:    a.Subagents,
		AgentConfig:  a.AgentConfig,
		Permissions:  a.Permissions,
		Team:         a.Team,
		SharedMemory: a.SharedMemory,
		Status:       a.Status,
		CreatedAt:    a.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    a.UpdatedAt.Format(time.RFC3339),
	}
	if a.CredentialID != nil {
		resp.CredentialID = a.CredentialID.String()
	}
	if a.IdentityID != nil {
		s := a.IdentityID.String()
		resp.IdentityID = &s
	}
	if a.SandboxID != nil {
		s := a.SandboxID.String()
		resp.SandboxID = &s
	}
	if a.SandboxTemplateID != nil {
		s := a.SandboxTemplateID.String()
		resp.SandboxTemplateID = &s
	}
	// Include provider_id from the credential association if loaded
	if a.Credential != nil && a.Credential.ProviderID != "" {
		resp.ProviderID = a.Credential.ProviderID
	}
	return resp
}

var validSandboxTypes = map[string]bool{
	"dedicated": true,
	"shared":    true,
}

// Create handles POST /v1/agents.
// @Summary Create an agent
// @Description Creates a new agent tied to an identity and credential. Shared agents are pushed to Bridge immediately.
// @Tags agents
// @Accept json
// @Produce json
// @Param body body createAgentRequest true "Agent definition"
// @Success 201 {object} agentResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents [post]
func (h *AgentHandler) Create(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	var req createAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	// Validate required fields — system_prompt is not required when forge is used
	// (forge generates it automatically).
	if req.Name == "" || req.CredentialID == "" || req.Model == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name, credential_id, and model are required"})
		return
	}
	if req.SystemPrompt == "" && req.Forge == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "system_prompt is required (or use forge to generate one)"})
		return
	}
	if req.Forge != nil {
		if req.Forge.JudgeCredentialID == "" || req.Forge.JudgeModel == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "forge.judge_credential_id and forge.judge_model are required"})
			return
		}
	}
	if !validSandboxTypes[req.SandboxType] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "sandbox_type must be 'dedicated' or 'shared'"})
		return
	}

	// Validate identity exists and belongs to org (optional)
	var identity *model.Identity
	if req.IdentityID != "" {
		var ident model.Identity
		if err := h.db.Where("id = ? AND org_id = ?", req.IdentityID, org.ID).First(&ident).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "identity not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to validate identity"})
			return
		}
		identity = &ident
	}

	// Validate credential exists, belongs to org, and is not revoked
	var cred model.Credential
	if err := h.db.Where("id = ? AND org_id = ? AND revoked_at IS NULL", req.CredentialID, org.ID).First(&cred).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "credential not found or revoked"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to validate credential"})
		return
	}

	// Validate forge judge credential if forge is requested
	if req.Forge != nil {
		var judgeCred model.Credential
		if err := h.db.Where("id = ? AND org_id = ? AND revoked_at IS NULL", req.Forge.JudgeCredentialID, org.ID).First(&judgeCred).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "forge judge credential not found or revoked"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to validate forge judge credential"})
			return
		}
	}

	// Validate model is supported by the credential's provider
	if cred.ProviderID != "" {
		provider, ok := h.registry.GetProvider(cred.ProviderID)
		if ok {
			if _, modelExists := provider.Models[req.Model]; !modelExists {
				writeJSON(w, http.StatusBadRequest, map[string]string{
					"error": fmt.Sprintf("model %q is not supported by provider %q", req.Model, cred.ProviderID),
				})
				return
			}
		}
	}

	// Validate json_schema in agent_config if present
	if errMsg := validateJSONSchema(req.AgentConfig); errMsg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": errMsg})
		return
	}

	// Validate sandbox template if provided
	var sandboxTemplateID *interface{ String() string }
	_ = sandboxTemplateID // unused, we parse directly
	agent := model.Agent{
		OrgID:        &org.ID,
		Name:         req.Name,
		Description:  req.Description,
		CredentialID: &cred.ID,
		SandboxType:  req.SandboxType,
		SystemPrompt: req.SystemPrompt,
		Instructions: req.Instructions,
		Model:        req.Model,
		Tools:        defaultJSON(req.Tools),
		McpServers:   defaultJSON(req.McpServers),
		Skills:       defaultJSON(req.Skills),
		Integrations: defaultJSON(req.Integrations),
		Subagents:    defaultJSON(req.Subagents),
		AgentConfig:  defaultJSON(req.AgentConfig),
		Permissions:  defaultJSON(req.Permissions),
		Team:         req.Team,
		SharedMemory: req.SharedMemory,
		Status:       "active",
	}
	if identity != nil {
		agent.IdentityID = &identity.ID
	}

	// Set default tool permissions if not explicitly provided.
	if len(agent.Permissions) == 0 {
		agent.Permissions = defaultToolPermissions(agent.SandboxType)
	}

	if req.SandboxTemplateID != nil && *req.SandboxTemplateID != "" {
		var tmpl model.SandboxTemplate
		if err := h.db.Where("id = ? AND org_id = ?", *req.SandboxTemplateID, org.ID).First(&tmpl).Error; err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "sandbox template not found"})
			return
		}
		if tmpl.BuildStatus != "ready" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("sandbox template is not ready (status: %s)", tmpl.BuildStatus)})
			return
		}
		agent.SandboxTemplateID = &tmpl.ID
	}

	if err := h.db.Create(&agent).Error; err != nil {
		if isDuplicateKeyError(err) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": fmt.Sprintf("agent with name %q already exists in this workspace", req.Name)})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create agent"})
		return
	}

	// Reload with credential for response
	h.db.Preload("Credential").Preload("Identity").Where("id = ?", agent.ID).First(&agent)

	// Push shared agents to Bridge (dedicated agents are pushed lazily on conversation create)
	if h.pusher != nil && agent.SandboxType == "shared" {
		if err := h.pusher.PushAgent(r.Context(), &agent); err != nil {
			slog.Error("failed to push agent to bridge", "agent_id", agent.ID, "error", err)
			// Agent is created in DB — return it but log the push failure.
			// The agent can be re-pushed on next update or via retry.
		}
	}

	resp := toAgentResponse(agent)

	// Auto-create forge run with context gathering if requested.
	if req.Forge != nil && h.forgeController != nil {
		judgeCredID, _ := uuid.Parse(req.Forge.JudgeCredentialID)
		forgeRun := model.ForgeRun{
			OrgID:                    org.ID,
			AgentID:                  agent.ID,
			ArchitectCredentialID:    cred.ID,
			ArchitectModel:           agent.Model,
			EvalDesignerCredentialID: cred.ID,
			EvalDesignerModel:        agent.Model,
			JudgeCredentialID:        judgeCredID,
			JudgeModel:               req.Forge.JudgeModel,
			Status:                   model.ForgeStatusGatheringContext,
		}
		if err := h.db.Create(&forgeRun).Error; err != nil {
			slog.Error("failed to create forge run", "agent_id", agent.ID, "error", err)
		} else {
			result, err := h.forgeController.SetupContextGathering(r.Context(), &agent, &cred, &forgeRun)
			if err != nil {
				slog.Error("failed to setup context gathering", "agent_id", agent.ID, "forge_run_id", forgeRun.ID, "error", err)
			} else {
				runID := result.ForgeRunID.String()
				convID := result.ConversationID
				resp.ForgeRunID = &runID
				resp.ForgeConversationID = &convID
			}
		}
	}

	writeJSON(w, http.StatusCreated, resp)
}

// List handles GET /v1/agents.
// @Summary List agents
// @Description Returns agents for the current organization with optional filters.
// @Tags agents
// @Produce json
// @Param identity_id query string false "Filter by identity ID"
// @Param status query string false "Filter by status (active, archived)"
// @Param sandbox_type query string false "Filter by sandbox type (shared, dedicated)"
// @Param limit query int false "Page size (default 50, max 100)"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[agentResponse]
// @Failure 401 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents [get]
func (h *AgentHandler) List(w http.ResponseWriter, r *http.Request) {
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

	q := h.db.Preload("Credential").Preload("Identity").Where("agents.org_id = ? AND agents.is_system = false AND agents.deleted_at IS NULL", org.ID)

	if identityID := r.URL.Query().Get("identity_id"); identityID != "" {
		q = q.Where("agents.identity_id = ?", identityID)
	}
	if status := r.URL.Query().Get("status"); status != "" {
		q = q.Where("agents.status = ?", status)
	}
	if sandboxType := r.URL.Query().Get("sandbox_type"); sandboxType != "" {
		q = q.Where("agents.sandbox_type = ?", sandboxType)
	}

	q = applyPagination(q, cursor, limit)

	var agents []model.Agent
	if err := q.Find(&agents).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list agents"})
		return
	}

	hasMore := len(agents) > limit
	if hasMore {
		agents = agents[:limit]
	}

	resp := make([]agentResponse, len(agents))
	for i, a := range agents {
		resp[i] = toAgentResponse(a)
	}

	result := paginatedResponse[agentResponse]{Data: resp, HasMore: hasMore}
	if hasMore {
		last := agents[len(agents)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}

	writeJSON(w, http.StatusOK, result)
}

// Get handles GET /v1/agents/{id}.
// @Summary Get an agent
// @Description Returns a single agent by ID, including the latest forge run if one exists.
// @Tags agents
// @Produce json
// @Param id path string true "Agent ID"
// @Success 200 {object} agentResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{id} [get]
func (h *AgentHandler) Get(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	id := chi.URLParam(r, "id")
	var agent model.Agent
	if err := h.db.Preload("Credential").Preload("Identity").Where("id = ? AND org_id = ? AND is_system = false AND deleted_at IS NULL", id, org.ID).First(&agent).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get agent"})
		return
	}

	resp := toAgentResponse(agent)

	// Include the latest forge run if one exists.
	var forgeRun model.ForgeRun
	if err := h.db.Where("agent_id = ? AND org_id = ?", agent.ID, org.ID).
		Order("created_at DESC").
		First(&forgeRun).Error; err == nil {
		fr := toForgeRunResponse(forgeRun)
		resp.ForgeRun = &fr
	}

	writeJSON(w, http.StatusOK, resp)
}

// Update handles PUT /v1/agents/{id}.
// @Summary Update an agent
// @Description Updates an agent. Re-validates credential/model compatibility. Shared agents are re-pushed to Bridge.
// @Tags agents
// @Accept json
// @Produce json
// @Param id path string true "Agent ID"
// @Param body body updateAgentRequest true "Fields to update"
// @Success 200 {object} agentResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{id} [put]
func (h *AgentHandler) Update(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	id := chi.URLParam(r, "id")
	var agent model.Agent
	if err := h.db.Where("id = ? AND org_id = ? AND is_system = false AND deleted_at IS NULL", id, org.ID).First(&agent).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get agent"})
		return
	}

	var req updateAgentRequest
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
	if req.SandboxType != nil {
		if !validSandboxTypes[*req.SandboxType] {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "sandbox_type must be 'dedicated' or 'shared'"})
			return
		}
		updates["sandbox_type"] = *req.SandboxType
	}
	if req.SystemPrompt != nil {
		updates["system_prompt"] = *req.SystemPrompt
	}
	if req.Instructions != nil {
		updates["instructions"] = *req.Instructions
	}

	// If credential or model changes, re-validate compatibility
	credID := agent.CredentialID
	modelName := agent.Model
	if req.CredentialID != nil {
		var cred model.Credential
		if err := h.db.Where("id = ? AND org_id = ? AND revoked_at IS NULL", *req.CredentialID, org.ID).First(&cred).Error; err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "credential not found or revoked"})
			return
		}
		credID = &cred.ID
		updates["credential_id"] = cred.ID
	}
	if req.Model != nil {
		modelName = *req.Model
		updates["model"] = *req.Model
	}

	// Validate model/provider compatibility if either changed
	if (req.CredentialID != nil || req.Model != nil) && credID != nil {
		var cred model.Credential
		h.db.Where("id = ?", *credID).First(&cred)
		if cred.ProviderID != "" {
			provider, ok := h.registry.GetProvider(cred.ProviderID)
			if ok {
				if _, exists := provider.Models[modelName]; !exists {
					writeJSON(w, http.StatusBadRequest, map[string]string{
						"error": fmt.Sprintf("model %q is not supported by provider %q", modelName, cred.ProviderID),
					})
					return
				}
			}
		}
	}

	if req.SandboxTemplateID != nil {
		if *req.SandboxTemplateID == "" {
			updates["sandbox_template_id"] = nil
		} else {
			var tmpl model.SandboxTemplate
			if err := h.db.Where("id = ? AND org_id = ?", *req.SandboxTemplateID, org.ID).First(&tmpl).Error; err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "sandbox template not found"})
				return
			}
			updates["sandbox_template_id"] = tmpl.ID
		}
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
	if req.Integrations != nil {
		updates["integrations"] = req.Integrations
	}
	if req.Subagents != nil {
		updates["subagents"] = req.Subagents
	}
	if req.AgentConfig != nil {
		if errMsg := validateJSONSchema(req.AgentConfig); errMsg != "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": errMsg})
			return
		}
		updates["agent_config"] = req.AgentConfig
	}
	if req.Permissions != nil {
		updates["permissions"] = req.Permissions
	}
	if req.Team != nil {
		updates["team"] = *req.Team
	}
	if req.SharedMemory != nil {
		updates["shared_memory"] = *req.SharedMemory
	}

	// Detect sandbox_type transition before applying update
	oldSandboxType := agent.SandboxType
	newSandboxType := oldSandboxType
	if v, ok := updates["sandbox_type"]; ok {
		newSandboxType = v.(string)
	}

	// If transitioning shared → dedicated, remove from pool sandbox first
	if h.pusher != nil && oldSandboxType == "shared" && newSandboxType == "dedicated" {
		if err := h.pusher.RemoveAgent(r.Context(), &agent); err != nil {
			slog.Error("failed to remove agent from pool sandbox during type transition", "agent_id", agent.ID, "error", err)
		}
	}

	if len(updates) > 0 {
		if err := h.db.Model(&agent).Updates(updates).Error; err != nil {
			if isDuplicateKeyError(err) {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "agent with that name already exists in this workspace"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update agent"})
			return
		}
	}

	// Reload with credential
	h.db.Preload("Credential").Preload("Identity").Where("id = ?", agent.ID).First(&agent)

	// Re-push shared agents to Bridge on update (including dedicated → shared transition)
	if h.pusher != nil && agent.SandboxType == "shared" && len(updates) > 0 {
		if err := h.pusher.PushAgent(r.Context(), &agent); err != nil {
			slog.Error("failed to push agent update to bridge", "agent_id", agent.ID, "error", err)
		}
	}

	writeJSON(w, http.StatusOK, toAgentResponse(agent))
}

// Delete handles DELETE /v1/agents/{id}.
// @Summary Delete an agent
// @Description Deletes an agent and removes it from Bridge.
// @Tags agents
// @Produce json
// @Param id path string true "Agent ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{id} [delete]
func (h *AgentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	id := chi.URLParam(r, "id")

	var agent model.Agent
	if err := h.db.Where("id = ? AND org_id = ? AND is_system = false AND deleted_at IS NULL", id, org.ID).First(&agent).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get agent"})
		return
	}

	// Soft-delete: set deleted_at timestamp
	now := time.Now()
	if err := h.db.Model(&agent).Update("deleted_at", &now).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete agent"})
		return
	}

	// Enqueue async cleanup (sandbox teardown + hard delete)
	if h.enqueuer != nil {
		task, err := tasks.NewAgentCleanupTask(agent.ID)
		if err != nil {
			slog.Error("failed to create agent cleanup task", "agent_id", agent.ID, "error", err)
		} else if _, err := h.enqueuer.Enqueue(task); err != nil {
			slog.Error("failed to enqueue agent cleanup", "agent_id", agent.ID, "error", err)
		}
	}

	slog.Info("agent soft-deleted", "agent_id", agent.ID, "org_id", org.ID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func defaultJSON(j model.JSON) model.JSON {
	if j == nil {
		return model.JSON{}
	}
	return j
}

// sharedAgentTools are the only tools available to shared sandbox agents.
// No filesystem, shell, code intelligence, or CodeDB tools.
var sharedAgentTools = []string{
	// Web
	"web_fetch", "web_search", "web_crawl", "web_get_links", "web_screenshot", "web_transform",
	// Agent orchestration
	"agent", "sub_agent", "parallel_agent", "batch", "join",
	// Task management
	"todowrite", "todoread",
	// Journal
	"journal_write", "journal_read",
	// Skills
	"skill",
}

// dedicatedAgentTools are all tools available to dedicated sandbox agents.
var dedicatedAgentTools = []string{
	// Filesystem
	"Read", "write", "edit", "multiedit", "apply_patch", "Glob", "Grep", "LS",
	// Shell
	"bash",
	// Web
	"web_fetch", "web_search", "web_crawl", "web_get_links", "web_screenshot", "web_transform",
	// Agent orchestration
	"agent", "sub_agent", "parallel_agent", "batch", "join",
	// Task management
	"todowrite", "todoread",
	// Journal
	"journal_write", "journal_read",
	// Code intelligence
	"lsp", "skill",
	// CodeDB
	"codedb_outline", "codedb_tree", "codedb_symbol", "codedb_search", "codedb_word",
	"codedb_find", "codedb_read", "codedb_edit", "codedb_hot", "codedb_deps",
	"codedb_changes", "codedb_status", "codedb_bundle", "codedb_snapshot",
	"codedb_remote", "codedb_projects", "codedb_index",
}

// defaultToolPermissions returns the default permission map for an agent based
// on its sandbox type. Shared agents get web, orchestration, task, journal, and
// skill tools only. Dedicated agents get all tools.
func defaultToolPermissions(sandboxType string) model.JSON {
	tools := dedicatedAgentTools
	if sandboxType == "shared" {
		tools = sharedAgentTools
	}
	perms := model.JSON{}
	for _, tool := range tools {
		perms[tool] = "allow"
	}
	return perms
}

// validateJSONSchema validates the json_schema field inside agent_config.
// Returns an error message if invalid, empty string if valid or absent.
func validateJSONSchema(agentConfig model.JSON) string {
	if agentConfig == nil {
		return ""
	}

	raw, ok := agentConfig["json_schema"]
	if !ok || raw == nil {
		return ""
	}

	schema, ok := raw.(map[string]any)
	if !ok {
		return "json_schema must be an object"
	}

	// Require "name" field
	name, _ := schema["name"].(string)
	if name == "" {
		return "json_schema.name is required and must be a non-empty string"
	}

	// Require "schema" field
	schemaDef, ok := schema["schema"].(map[string]any)
	if !ok {
		return "json_schema.schema is required and must be an object"
	}

	// Top-level type must be "object"
	schemaType, _ := schemaDef["type"].(string)
	if schemaType != "object" {
		return "json_schema.schema.type must be \"object\""
	}

	// Validate nesting depth and property count
	if err := validateSchemaDepthAndProperties(schemaDef, 1, new(int)); err != "" {
		return err
	}

	// Reject unsupported keywords at any level
	if err := validateSchemaKeywords(schemaDef); err != "" {
		return err
	}

	return ""
}

// validateSchemaDepthAndProperties walks a JSON Schema object checking depth <= 5 and total properties <= 100.
func validateSchemaDepthAndProperties(schema map[string]any, depth int, propCount *int) string {
	if depth > 5 {
		return "json_schema.schema exceeds maximum nesting depth of 5"
	}

	props, _ := schema["properties"].(map[string]any)
	*propCount += len(props)
	if *propCount > 100 {
		return "json_schema.schema exceeds maximum of 100 total properties"
	}

	for _, v := range props {
		if obj, ok := v.(map[string]any); ok {
			propType, _ := obj["type"].(string)
			if propType == "object" {
				if err := validateSchemaDepthAndProperties(obj, depth+1, propCount); err != "" {
					return err
				}
			}
			if propType == "array" {
				if items, ok := obj["items"].(map[string]any); ok {
					itemType, _ := items["type"].(string)
					if itemType == "object" {
						if err := validateSchemaDepthAndProperties(items, depth+1, propCount); err != "" {
							return err
						}
					}
				}
			}
		}
	}
	return ""
}

// validateSchemaKeywords rejects non-portable JSON Schema keywords.
func validateSchemaKeywords(schema map[string]any) string {
	rejected := []string{"$ref", "$defs", "oneOf", "allOf", "not", "if", "then", "else",
		"pattern", "format", "minLength", "maxLength", "minimum", "maximum",
		"minItems", "maxItems", "patternProperties"}

	return walkSchemaKeywords(schema, rejected)
}

func walkSchemaKeywords(obj map[string]any, rejected []string) string {
	for _, kw := range rejected {
		if _, exists := obj[kw]; exists {
			return fmt.Sprintf("json_schema.schema contains unsupported keyword %q (not portable across providers)", kw)
		}
	}
	if props, ok := obj["properties"].(map[string]any); ok {
		for _, v := range props {
			if sub, ok := v.(map[string]any); ok {
				if err := walkSchemaKeywords(sub, rejected); err != "" {
					return err
				}
			}
		}
	}
	if items, ok := obj["items"].(map[string]any); ok {
		if err := walkSchemaKeywords(items, rejected); err != "" {
			return err
		}
	}
	if anyOf, ok := obj["anyOf"].([]any); ok {
		for _, item := range anyOf {
			if sub, ok := item.(map[string]any); ok {
				if err := walkSchemaKeywords(sub, rejected); err != "" {
					return err
				}
			}
		}
	}
	return ""
}

// GetSetup handles GET /v1/agents/{id}/setup.
// @Summary Get agent sandbox setup config
// @Description Returns setup commands and env var key names for dedicated agents.
// @Tags agents
// @Produce json
// @Param id path string true "Agent ID"
// @Success 200 {object} setupResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{id}/setup [get]
func (h *AgentHandler) GetSetup(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	var agent model.Agent
	if err := h.db.Where("id = ? AND org_id = ? AND is_system = false AND deleted_at IS NULL", chi.URLParam(r, "id"), org.ID).First(&agent).Error; err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
		return
	}

	resp := setupResponse{
		SetupCommands: []string(agent.SetupCommands),
		EnvVarKeys:    []string{},
	}
	if resp.SetupCommands == nil {
		resp.SetupCommands = []string{}
	}

	if h.encKey != nil && len(agent.EncryptedEnvVars) > 0 {
		if decrypted, err := h.encKey.DecryptString(agent.EncryptedEnvVars); err == nil {
			var envMap map[string]string
			if json.Unmarshal([]byte(decrypted), &envMap) == nil {
				for k := range envMap {
					resp.EnvVarKeys = append(resp.EnvVarKeys, k)
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// UpdateSetup handles PUT /v1/agents/{id}/setup.
// @Summary Update agent sandbox setup config
// @Description Sets setup commands and encrypted environment variables. Only available for dedicated sandbox agents.
// @Tags agents
// @Accept json
// @Produce json
// @Param id path string true "Agent ID"
// @Param body body setupRequest true "Setup configuration"
// @Success 200 {object} setupResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{id}/setup [put]
func (h *AgentHandler) UpdateSetup(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	var agent model.Agent
	if err := h.db.Where("id = ? AND org_id = ? AND is_system = false AND deleted_at IS NULL", chi.URLParam(r, "id"), org.ID).First(&agent).Error; err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
		return
	}

	if agent.SandboxType != "dedicated" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "setup configuration is only available for dedicated sandbox agents"})
		return
	}

	var req setupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	for k := range req.EnvVars {
		if strings.HasPrefix(strings.ToUpper(k), "BRIDGE_") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment variable names starting with BRIDGE_ are reserved"})
			return
		}
	}

	updates := map[string]any{}

	if req.SetupCommands != nil {
		updates["setup_commands"] = pq.StringArray(req.SetupCommands)
	}

	if req.EnvVars != nil && h.encKey != nil {
		envJSON, err := json.Marshal(req.EnvVars)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid env_vars"})
			return
		}
		encrypted, err := h.encKey.EncryptString(string(envJSON))
		if err != nil {
			slog.Error("failed to encrypt env vars", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to encrypt environment variables"})
			return
		}
		updates["encrypted_env_vars"] = encrypted
	}

	if len(updates) > 0 {
		if err := h.db.Model(&agent).Updates(updates).Error; err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update setup"})
			return
		}
	}

	resp := setupResponse{
		SetupCommands: req.SetupCommands,
		EnvVarKeys:    []string{},
	}
	if resp.SetupCommands == nil {
		resp.SetupCommands = []string{}
	}
	for k := range req.EnvVars {
		resp.EnvVarKeys = append(resp.EnvVarKeys, k)
	}

	writeJSON(w, http.StatusOK, resp)
}

