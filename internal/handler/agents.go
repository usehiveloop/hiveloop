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
	"github.com/ziraloop/ziraloop/internal/mcp/catalog"
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
	actionsCatalog  *catalog.Catalog         // nil if not configured
}

func NewAgentHandler(db *gorm.DB, reg *registry.Registry, pusher AgentPusher, encKey *crypto.SymmetricKey, enqueuer ...enqueue.TaskEnqueuer) *AgentHandler {
	h := &AgentHandler{db: db, registry: reg, pusher: pusher, encKey: encKey}
	if len(enqueuer) > 0 {
		h.enqueuer = enqueuer[0]
	}
	return h
}

// SetCatalog sets the actions catalog for trigger validation during agent creation.
func (h *AgentHandler) SetCatalog(c *catalog.Catalog) {
	h.actionsCatalog = c
}

// ensure sandbox.Pusher satisfies AgentPusher
var _ AgentPusher = (*sandbox.Pusher)(nil)

// agentTriggerInput defines a webhook trigger to create alongside the agent.
// Each entry creates a RouterTrigger + RoutingRule that automatically invokes
// this agent when the matching webhook event fires.
type agentTriggerInput struct {
	ConnectionID string              `json:"connection_id"`
	TriggerKeys  []string            `json:"trigger_keys"`
	Conditions   *model.TriggerMatch `json:"conditions,omitempty"`
}

// agentTriggerResponse is the trigger data returned in agent responses.
type agentTriggerResponse struct {
	ID           string   `json:"id"`
	ConnectionID string   `json:"connection_id"`
	Provider     string   `json:"provider"`
	TriggerKeys  []string `json:"trigger_keys"`
	Enabled      bool     `json:"enabled"`
	Conditions   any      `json:"conditions"`
}

type createAgentRequest struct {
	Name              string     `json:"name"`
	Description       *string    `json:"description,omitempty"`
	CredentialID      string     `json:"credential_id"`
	SandboxType       string     `json:"sandbox_type"`
	SandboxTemplateID *string    `json:"sandbox_template_id,omitempty"`
	SystemPrompt      string                      `json:"system_prompt"`
	ProviderPrompts   model.ProviderPromptsMap    `json:"provider_prompts,omitempty"`
	Instructions      *string                     `json:"instructions,omitempty"`
	Model             string                      `json:"model"`
	Tools             model.JSON                  `json:"tools,omitempty"`
	McpServers        model.JSON                  `json:"mcp_servers,omitempty"`
	Skills            model.JSON                  `json:"skills,omitempty"`
	Integrations      model.JSON                  `json:"integrations,omitempty"`
	AgentConfig       model.JSON                  `json:"agent_config,omitempty"`
	Permissions       model.JSON                  `json:"permissions,omitempty"`
	Resources         model.JSON                  `json:"resources,omitempty"`
	Team              string                      `json:"team,omitempty"`
	SharedMemory      bool                        `json:"shared_memory,omitempty"`
	SandboxTools      []string                    `json:"sandbox_tools,omitempty"`  // tools to enable in dedicated sandbox (e.g. "chrome")
	SkillIDs          []string                    `json:"skill_ids,omitempty"`      // skills from /v1/skills to attach on create
	SubagentIDs       []string                    `json:"subagent_ids,omitempty"`   // subagents from /v1/subagents to attach on create
	Triggers          []agentTriggerInput         `json:"triggers,omitempty"`       // webhook triggers to create
}

type updateAgentRequest struct {
	Name              *string    `json:"name,omitempty"`
	Description       *string    `json:"description,omitempty"`
	CredentialID      *string    `json:"credential_id,omitempty"`
	SandboxType       *string    `json:"sandbox_type,omitempty"`
	SandboxTemplateID *string    `json:"sandbox_template_id,omitempty"`
	SystemPrompt      *string                     `json:"system_prompt,omitempty"`
	ProviderPrompts   model.ProviderPromptsMap    `json:"provider_prompts,omitempty"`
	Instructions      *string                     `json:"instructions,omitempty"`
	Model             *string    `json:"model,omitempty"`
	Tools             model.JSON `json:"tools,omitempty"`
	McpServers        model.JSON `json:"mcp_servers,omitempty"`
	Skills            model.JSON `json:"skills,omitempty"`
	Integrations      model.JSON `json:"integrations,omitempty"`
	AgentConfig       model.JSON `json:"agent_config,omitempty"`
	Permissions       model.JSON `json:"permissions,omitempty"`
	Resources         model.JSON `json:"resources,omitempty"`
	Team              *string    `json:"team,omitempty"`
	SharedMemory      *bool      `json:"shared_memory,omitempty"`
	SandboxTools      []string   `json:"sandbox_tools,omitempty"` // tools to enable in dedicated sandbox
	SkillIDs          *[]string  `json:"skill_ids,omitempty"`     // nil=don't touch, []=detach all, [ids]=sync to these
	Triggers          *[]agentTriggerInput `json:"triggers,omitempty"` // nil=don't touch, []=remove all
}

// agentSubagentSummary is a lightweight subagent included in agent responses.
type agentSubagentSummary struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	Model       string  `json:"model"`
}

type setupRequest struct {
	SetupCommands []string          `json:"setup_commands"`
	EnvVars       map[string]string `json:"env_vars"`
}

type setupResponse struct {
	SetupCommands []string `json:"setup_commands"`
	EnvVarKeys    []string `json:"env_var_keys"`
}

// agentSkillSummary is a lightweight skill included in agent responses.
type agentSkillSummary struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	SourceType  string  `json:"source_type"`
}

type agentResponse struct {
	ID                string     `json:"id"`
	Name              string     `json:"name"`
	Description       *string    `json:"description,omitempty"`
	CredentialID      string     `json:"credential_id"`
	ProviderID        string     `json:"provider_id"`
	SandboxType       string     `json:"sandbox_type"`
	SandboxID         *string    `json:"sandbox_id,omitempty"`
	SandboxTemplateID *string    `json:"sandbox_template_id,omitempty"`
	SystemPrompt      string                      `json:"system_prompt"`
	ProviderPrompts   model.ProviderPromptsMap    `json:"provider_prompts"`
	Instructions      *string                     `json:"instructions,omitempty"`
	Model             string                      `json:"model"`
	Tools             model.JSON                  `json:"tools"`
	McpServers        model.JSON                  `json:"mcp_servers"`
	Skills            model.JSON                  `json:"skills"`
	Integrations      model.JSON                  `json:"integrations"`
	AgentConfig       model.JSON                  `json:"agent_config"`
	Permissions       model.JSON                  `json:"permissions"`
	Resources         model.JSON                  `json:"resources"`
	Team              string     `json:"team"`
	SharedMemory      bool       `json:"shared_memory"`
	SandboxTools      []string   `json:"sandbox_tools"`
	Status              string                  `json:"status"`
	Triggers            []agentTriggerResponse  `json:"triggers"`
	AttachedSubagents   []agentSubagentSummary  `json:"subagents"`
	AttachedSkills      []agentSkillSummary     `json:"attached_skills"`
	CreatedAt           string                  `json:"created_at"`
	UpdatedAt           string                  `json:"updated_at"`
}

func toAgentResponse(a model.Agent) agentResponse {
	resp := agentResponse{
		ID:           a.ID.String(),
		Name:         a.Name,
		Description:  a.Description,
		SandboxType:  a.SandboxType,
		SystemPrompt:    a.SystemPrompt,
		ProviderPrompts: a.ProviderPrompts,
		Instructions:    a.Instructions,
		Model:           a.Model,
		Tools:        a.Tools,
		McpServers:   a.McpServers,
		Skills:       a.Skills,
		Integrations: a.Integrations,
		AgentConfig:  a.AgentConfig,
		Permissions:  a.Permissions,
		Resources:    a.Resources,
		Team:         a.Team,
		SharedMemory: a.SharedMemory,
		SandboxTools: ensureStringSlice(a.SandboxTools),
		Status:       a.Status,
		CreatedAt:    a.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    a.UpdatedAt.Format(time.RFC3339),
	}
	if a.CredentialID != nil {
		resp.CredentialID = a.CredentialID.String()
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

// loadAgentTriggers loads the routing triggers configured for one or more agents.
// Returns a map from agent ID to trigger responses. Uses a single query with
// joins to avoid N+1.
func (h *AgentHandler) loadAgentTriggers(agentIDs ...uuid.UUID) map[uuid.UUID][]agentTriggerResponse {
	if len(agentIDs) == 0 {
		return nil
	}

	type triggerRow struct {
		RuleID      uuid.UUID      `gorm:"column:rule_id"`
		AgentID     uuid.UUID      `gorm:"column:agent_id"`
		TriggerID   uuid.UUID      `gorm:"column:trigger_id"`
		ConnID      uuid.UUID      `gorm:"column:conn_id"`
		Provider    string         `gorm:"column:provider"`
		TriggerKeys pq.StringArray `gorm:"column:trigger_keys"`
		Enabled     bool           `gorm:"column:enabled"`
		Conditions  model.RawJSON  `gorm:"column:conditions"`
	}

	var rows []triggerRow
	h.db.Raw(`
		SELECT
			rr.id AS rule_id,
			rr.agent_id,
			rt.id AS trigger_id,
			rt.connection_id AS conn_id,
			ii.provider,
			rt.trigger_keys,
			rt.enabled,
			rr.conditions
		FROM routing_rules rr
		JOIN router_triggers rt ON rt.id = rr.router_trigger_id
		JOIN in_connections ic ON ic.id = rt.connection_id
		JOIN in_integrations ii ON ii.id = ic.in_integration_id
		WHERE rr.agent_id IN ?
		ORDER BY rt.id ASC
	`, agentIDs).Scan(&rows)

	result := make(map[uuid.UUID][]agentTriggerResponse, len(agentIDs))
	for _, row := range rows {
		var conditions any
		if len(row.Conditions) > 0 {
			var parsed model.TriggerMatch
			if err := json.Unmarshal(row.Conditions, &parsed); err == nil && len(parsed.Conditions) > 0 {
				conditions = parsed
			}
		}

		result[row.AgentID] = append(result[row.AgentID], agentTriggerResponse{
			ID:           row.TriggerID.String(),
			ConnectionID: row.ConnID.String(),
			Provider:     row.Provider,
			TriggerKeys:  []string(row.TriggerKeys),
			Enabled:      row.Enabled,
			Conditions:   conditions,
		})
	}
	return result
}

// loadAgentSubagents batch-loads attached subagent summaries for one or more agents.
func (h *AgentHandler) loadAgentSubagents(agentIDs ...uuid.UUID) map[uuid.UUID][]agentSubagentSummary {
	if len(agentIDs) == 0 {
		return nil
	}
	var links []model.AgentSubagent
	if err := h.db.Where("agent_id IN ?", agentIDs).Find(&links).Error; err != nil {
		return nil
	}
	if len(links) == 0 {
		return nil
	}
	subIDs := make([]uuid.UUID, len(links))
	for index, link := range links {
		subIDs[index] = link.SubagentID
	}
	var subs []model.Agent
	if err := h.db.Select("id, name, description, model").Where("id IN ?", subIDs).Find(&subs).Error; err != nil {
		return nil
	}
	subByID := make(map[uuid.UUID]model.Agent, len(subs))
	for _, sub := range subs {
		subByID[sub.ID] = sub
	}
	result := make(map[uuid.UUID][]agentSubagentSummary, len(agentIDs))
	for _, link := range links {
		sub, ok := subByID[link.SubagentID]
		if !ok {
			continue
		}
		result[link.AgentID] = append(result[link.AgentID], agentSubagentSummary{
			ID:          sub.ID.String(),
			Name:        sub.Name,
			Description: sub.Description,
			Model:       sub.Model,
		})
	}
	return result
}

// loadAgentSkills batch-loads attached skill summaries for one or more agents.
func (h *AgentHandler) loadAgentSkills(agentIDs ...uuid.UUID) map[uuid.UUID][]agentSkillSummary {
	if len(agentIDs) == 0 {
		return nil
	}
	var links []model.AgentSkill
	if err := h.db.Where("agent_id IN ?", agentIDs).Find(&links).Error; err != nil {
		return nil
	}
	if len(links) == 0 {
		return nil
	}
	skillIDs := make([]uuid.UUID, len(links))
	for index, link := range links {
		skillIDs[index] = link.SkillID
	}
	var skills []model.Skill
	if err := h.db.Select("id, name, description, source_type").Where("id IN ?", skillIDs).Find(&skills).Error; err != nil {
		return nil
	}
	skillByID := make(map[uuid.UUID]model.Skill, len(skills))
	for _, skill := range skills {
		skillByID[skill.ID] = skill
	}
	result := make(map[uuid.UUID][]agentSkillSummary, len(agentIDs))
	for _, link := range links {
		skill, ok := skillByID[link.SkillID]
		if !ok {
			continue
		}
		result[link.AgentID] = append(result[link.AgentID], agentSkillSummary{
			ID:          skill.ID.String(),
			Name:        skill.Name,
			Description: skill.Description,
			SourceType:  skill.SourceType,
		})
	}
	return result
}

// createAgentTriggers creates RouterTrigger + RoutingRule records for an agent
// inside an existing transaction. The router is found or created for the org.
// Connection IDs are in_connections IDs from the frontend.
func createAgentTriggers(tx *gorm.DB, orgID, agentID uuid.UUID, triggers []agentTriggerInput) error {
	if len(triggers) == 0 {
		return nil
	}

	var router model.Router
	if err := tx.Where("org_id = ?", orgID).FirstOrCreate(&router, model.Router{
		OrgID: orgID,
		Name:  "Zira",
	}).Error; err != nil {
		return fmt.Errorf("find or create router: %w", err)
	}

	for _, input := range triggers {
		connectionID, err := uuid.Parse(input.ConnectionID)
		if err != nil {
			return fmt.Errorf("invalid connection_id %q: %w", input.ConnectionID, err)
		}

		trigger := model.RouterTrigger{
			OrgID:        orgID,
			RouterID:     router.ID,
			ConnectionID: connectionID,
			TriggerKeys:  pq.StringArray(input.TriggerKeys),
			Enabled:      true,
			RoutingMode:  "rule",
		}
		if err := tx.Create(&trigger).Error; err != nil {
			return fmt.Errorf("create router trigger: %w", err)
		}

		var conditionsJSON model.RawJSON
		if input.Conditions != nil && len(input.Conditions.Conditions) > 0 {
			conditionsJSON, _ = json.Marshal(input.Conditions)
		}

		rule := model.RoutingRule{
			RouterTriggerID: trigger.ID,
			AgentID:         agentID,
			Priority:        1,
			Conditions:      conditionsJSON,
		}
		if err := tx.Create(&rule).Error; err != nil {
			return fmt.Errorf("create routing rule: %w", err)
		}
	}
	return nil
}

// deleteAgentTriggers removes all RouterTrigger + RoutingRule records owned by
// an agent. CASCADE on the RouterTrigger FK deletes the RoutingRule rows.
func deleteAgentTriggers(db *gorm.DB, agentID uuid.UUID) error {
	var triggerIDs []uuid.UUID
	if err := db.Model(&model.RoutingRule{}).
		Where("agent_id = ?", agentID).
		Pluck("router_trigger_id", &triggerIDs).Error; err != nil {
		return fmt.Errorf("find agent triggers: %w", err)
	}
	if len(triggerIDs) == 0 {
		return nil
	}
	if err := db.Where("id IN ?", triggerIDs).Delete(&model.RouterTrigger{}).Error; err != nil {
		return fmt.Errorf("delete agent triggers: %w", err)
	}
	return nil
}

var validSandboxTypes = map[string]bool{
	"dedicated": true,
	"shared":    true,
}

// Create handles POST /v1/agents.
// @Summary Create an agent
// @Description Creates a new agent tied to a credential. Shared agents are pushed to Bridge immediately.
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
	if req.SystemPrompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "system_prompt is required"})
		return
	}
	if !validSandboxTypes[req.SandboxType] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "sandbox_type must be 'dedicated' or 'shared'"})
		return
	}

	// Validate sandbox tools (only meaningful for dedicated, but validate regardless)
	if len(req.SandboxTools) > 0 {
		if invalid := model.ValidateSandboxTools(req.SandboxTools); invalid != "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("invalid sandbox tool: %q", invalid)})
			return
		}
	}

	// Validate permission keys are recognized built-in tool IDs
	if len(req.Permissions) > 0 {
		permKeys := make(map[string]string, len(req.Permissions))
		for key, val := range req.Permissions {
			str, _ := val.(string)
			permKeys[key] = str
		}
		if invalid := model.ValidatePermissionKeys(permKeys); invalid != "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("invalid permission tool: %q", invalid)})
			return
		}
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
		SystemPrompt:    req.SystemPrompt,
		ProviderPrompts: req.ProviderPrompts,
		Instructions:    req.Instructions,
		Model:           req.Model,
		Tools:        defaultJSON(req.Tools),
		McpServers:   defaultJSON(req.McpServers),
		Skills:       defaultJSON(req.Skills),
		Integrations: defaultJSON(req.Integrations),
		AgentConfig:  defaultJSON(req.AgentConfig),
		Permissions:  defaultJSON(req.Permissions),
		Resources:    defaultJSON(req.Resources),
		Team:         req.Team,
		SharedMemory: req.SharedMemory,
		SandboxTools: pq.StringArray(req.SandboxTools),
		Status:       "active",
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

	// Parse skill IDs up front so a bad input fails the whole create cleanly.
	// Validate trigger inputs. Connection IDs are in_connections IDs from the frontend.
	for triggerIndex, triggerInput := range req.Triggers {
		if triggerInput.ConnectionID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("triggers[%d]: connection_id is required", triggerIndex)})
			return
		}
		if _, parseErr := uuid.Parse(triggerInput.ConnectionID); parseErr != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("triggers[%d]: invalid connection_id", triggerIndex)})
			return
		}
		if len(triggerInput.TriggerKeys) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("triggers[%d]: trigger_keys is required", triggerIndex)})
			return
		}
		var inConn model.InConnection
		if err := h.db.Where("id = ? AND org_id = ?", triggerInput.ConnectionID, org.ID).First(&inConn).Error; err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("triggers[%d]: connection not found", triggerIndex)})
			return
		}
	}

	var skillUUIDs []uuid.UUID
	if len(req.SkillIDs) > 0 {
		skillUUIDs = make([]uuid.UUID, 0, len(req.SkillIDs))
		seen := make(map[uuid.UUID]struct{}, len(req.SkillIDs))
		for _, raw := range req.SkillIDs {
			parsed, parseErr := uuid.Parse(raw)
			if parseErr != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("invalid skill_id %q", raw)})
				return
			}
			if _, dup := seen[parsed]; dup {
				continue
			}
			seen[parsed] = struct{}{}
			skillUUIDs = append(skillUUIDs, parsed)
		}
	}

	// Parse subagent IDs up front.
	var subagentUUIDs []uuid.UUID
	if len(req.SubagentIDs) > 0 {
		subagentUUIDs = make([]uuid.UUID, 0, len(req.SubagentIDs))
		seen := make(map[uuid.UUID]struct{}, len(req.SubagentIDs))
		for _, raw := range req.SubagentIDs {
			parsed, parseErr := uuid.Parse(raw)
			if parseErr != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("invalid subagent_id %q", raw)})
				return
			}
			if _, dup := seen[parsed]; dup {
				continue
			}
			seen[parsed] = struct{}{}
			subagentUUIDs = append(subagentUUIDs, parsed)
		}
	}

	// Use a transaction so agent + trigger + skill + subagent attachments are created atomically.
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&agent).Error; err != nil {
			return err
		}

		if len(skillUUIDs) > 0 {
			var visibleSkills []model.Skill
			if err := tx.
				Select("id").
				Where("id IN ? AND (org_id = ? OR (org_id IS NULL AND status = ?))",
					skillUUIDs, org.ID, model.SkillStatusPublished).
				Find(&visibleSkills).Error; err != nil {
				return fmt.Errorf("validate skill_ids: %w", err)
			}
			if len(visibleSkills) != len(skillUUIDs) {
				return fmt.Errorf("one or more skill_ids are not visible to this org")
			}
			links := make([]model.AgentSkill, len(visibleSkills))
			for i, skill := range visibleSkills {
				links[i] = model.AgentSkill{AgentID: agent.ID, SkillID: skill.ID}
			}
			if err := tx.Create(&links).Error; err != nil {
				return fmt.Errorf("attach skills: %w", err)
			}
			if err := tx.Model(&model.Skill{}).
				Where("id IN ?", skillUUIDs).
				UpdateColumn("install_count", gorm.Expr("install_count + 1")).Error; err != nil {
				return fmt.Errorf("bump install_count: %w", err)
			}
		}

		if len(subagentUUIDs) > 0 {
			var visibleSubs []model.Agent
			if err := tx.
				Select("id").
				Where("id IN ? AND agent_type = ? AND (org_id = ? OR (org_id IS NULL AND status = ?))",
					subagentUUIDs, model.AgentTypeSubagent, org.ID, "active").
				Find(&visibleSubs).Error; err != nil {
				return fmt.Errorf("validate subagent_ids: %w", err)
			}
			if len(visibleSubs) != len(subagentUUIDs) {
				return fmt.Errorf("one or more subagent_ids are not visible to this org")
			}
			subLinks := make([]model.AgentSubagent, len(visibleSubs))
			for index, sub := range visibleSubs {
				subLinks[index] = model.AgentSubagent{AgentID: agent.ID, SubagentID: sub.ID}
			}
			if err := tx.Create(&subLinks).Error; err != nil {
				return fmt.Errorf("attach subagents: %w", err)
			}
		}

		// Create webhook triggers.
		if err := createAgentTriggers(tx, org.ID, agent.ID, req.Triggers); err != nil {
			return fmt.Errorf("create triggers: %w", err)
		}

		return nil
	})
	if err != nil {
		if isDuplicateKeyError(err) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": fmt.Sprintf("agent with name %q already exists in this workspace", req.Name)})
			return
		}
		slog.Error("failed to create agent",
			"error", err,
			"org_id", org.ID,
			"agent_name", req.Name,
			"trigger_count", len(req.Triggers),
		)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create agent"})
		return
	}

	// Reload with credential for response
	h.db.Preload("Credential").Where("id = ?", agent.ID).First(&agent)

	// Push shared agents to Bridge (dedicated agents are pushed lazily on conversation create)
	if h.pusher != nil && agent.SandboxType == "shared" {
		if err := h.pusher.PushAgent(r.Context(), &agent); err != nil {
			slog.Error("failed to push agent to bridge", "agent_id", agent.ID, "error", err)
			// Agent is created in DB — return it but log the push failure.
			// The agent can be re-pushed on next update or via retry.
		}
	}

	resp := toAgentResponse(agent)
	resp.Triggers = h.loadAgentTriggers(agent.ID)[agent.ID]

	writeJSON(w, http.StatusCreated, resp)
}

// List handles GET /v1/agents.
// @Summary List agents
// @Description Returns agents for the current organization with optional filters.
// @Tags agents
// @Produce json
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

	q := h.db.Preload("Credential").Where("agents.org_id = ? AND agents.is_system = false AND agents.deleted_at IS NULL", org.ID)

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
	agentIDs := make([]uuid.UUID, len(agents))
	for index, agent := range agents {
		resp[index] = toAgentResponse(agent)
		agentIDs[index] = agent.ID
	}

	// Batch load triggers, subagents, and skills for all agents.
	triggersMap := h.loadAgentTriggers(agentIDs...)
	subagentsMap := h.loadAgentSubagents(agentIDs...)
	skillsMap := h.loadAgentSkills(agentIDs...)
	for index, agent := range agents {
		resp[index].Triggers = triggersMap[agent.ID]
		resp[index].AttachedSubagents = subagentsMap[agent.ID]
		resp[index].AttachedSkills = skillsMap[agent.ID]
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
	if err := h.db.Preload("Credential").Where("id = ? AND org_id = ? AND is_system = false AND deleted_at IS NULL", id, org.ID).First(&agent).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get agent"})
		return
	}

	resp := toAgentResponse(agent)
	resp.Triggers = h.loadAgentTriggers(agent.ID)[agent.ID]
	resp.AttachedSubagents = h.loadAgentSubagents(agent.ID)[agent.ID]
	resp.AttachedSkills = h.loadAgentSkills(agent.ID)[agent.ID]

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
	if len(req.ProviderPrompts) > 0 {
		updates["provider_prompts"] = req.ProviderPrompts
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
	if req.AgentConfig != nil {
		if errMsg := validateJSONSchema(req.AgentConfig); errMsg != "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": errMsg})
			return
		}
		updates["agent_config"] = req.AgentConfig
	}
	if req.Permissions != nil {
		permKeys := make(map[string]string, len(req.Permissions))
		for key, val := range req.Permissions {
			str, _ := val.(string)
			permKeys[key] = str
		}
		if invalid := model.ValidatePermissionKeys(permKeys); invalid != "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("invalid permission tool: %q", invalid)})
			return
		}
		updates["permissions"] = req.Permissions
	}
	if req.Resources != nil {
		updates["resources"] = req.Resources
	}
	if req.Team != nil {
		updates["team"] = *req.Team
	}
	if req.SharedMemory != nil {
		updates["shared_memory"] = *req.SharedMemory
	}
	if req.SandboxTools != nil {
		if invalid := model.ValidateSandboxTools(req.SandboxTools); invalid != "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("invalid sandbox tool: %q", invalid)})
			return
		}
		updates["sandbox_tools"] = pq.StringArray(req.SandboxTools)
	}

	// Validate trigger inputs if provided. Connection IDs are in_connections IDs.
	if req.Triggers != nil {
		for triggerIndex, triggerInput := range *req.Triggers {
			if triggerInput.ConnectionID == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("triggers[%d]: connection_id is required", triggerIndex)})
				return
			}
			if _, parseErr := uuid.Parse(triggerInput.ConnectionID); parseErr != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("triggers[%d]: invalid connection_id", triggerIndex)})
				return
			}
			if len(triggerInput.TriggerKeys) == 0 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("triggers[%d]: trigger_keys is required", triggerIndex)})
				return
			}
			var inConn model.InConnection
			if err := h.db.Where("id = ? AND org_id = ?", triggerInput.ConnectionID, org.ID).First(&inConn).Error; err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("triggers[%d]: connection not found", triggerIndex)})
				return
			}
		}
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
	h.db.Preload("Credential").Where("id = ?", agent.ID).First(&agent)

	// Re-push shared agents to Bridge on update (including dedicated → shared transition)
	if h.pusher != nil && agent.SandboxType == "shared" && len(updates) > 0 {
		if err := h.pusher.PushAgent(r.Context(), &agent); err != nil {
			slog.Error("failed to push agent update to bridge", "agent_id", agent.ID, "error", err)
		}
	}

	// Reconcile triggers: delete old, create new.
	if req.Triggers != nil {
		if err := deleteAgentTriggers(h.db, agent.ID); err != nil {
			slog.Error("failed to delete old triggers during update", "agent_id", agent.ID, "error", err)
		}
		if err := createAgentTriggers(h.db, org.ID, agent.ID, *req.Triggers); err != nil {
			slog.Error("failed to create new triggers during update", "agent_id", agent.ID, "error", err)
		}
	}

	// Reconcile skills: sync to the provided list.
	if req.SkillIDs != nil {
		if err := h.db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("agent_id = ?", agent.ID).Delete(&model.AgentSkill{}).Error; err != nil {
				return err
			}
			for _, rawID := range *req.SkillIDs {
				skillUUID, parseErr := uuid.Parse(rawID)
				if parseErr != nil {
					continue
				}
				if err := tx.Create(&model.AgentSkill{
					AgentID: agent.ID,
					SkillID: skillUUID,
				}).Error; err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			slog.Error("failed to sync skills during update", "agent_id", agent.ID, "error", err)
		}
	}

	resp := toAgentResponse(agent)
	resp.Triggers = h.loadAgentTriggers(agent.ID)[agent.ID]
	resp.AttachedSkills = h.loadAgentSkills(agent.ID)[agent.ID]
	writeJSON(w, http.StatusOK, resp)
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

	// Clean up routing triggers so a soft-deleted agent is never dispatched to.
	if err := deleteAgentTriggers(h.db, agent.ID); err != nil {
		slog.Error("failed to clean up agent triggers on delete", "agent_id", agent.ID, "error", err)
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

func ensureStringSlice(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// sharedAgentTools are the only tools available to shared sandbox agents.
// No filesystem, shell, or code intelligence tools.
var sharedAgentTools = []string{
	// Web
	"web_fetch", "web_search", "web_crawl", "web_get_links", "web_screenshot", "web_transform",
	// Agent orchestration
	"agent", "sub_agent", "batch",
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
	"Read", "write", "edit", "multiedit", "apply_patch", "Glob", "RipGrep", "AstGrep", "LS",
	// Shell
	"bash",
	// Web
	"web_fetch", "web_search", "web_crawl", "web_get_links", "web_screenshot", "web_transform",
	// Agent orchestration
	"agent", "sub_agent", "batch",
	// Task management
	"todowrite", "todoread",
	// Journal
	"journal_write", "journal_read",
	// Code intelligence
	"lsp", "skill",
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

// ListSandboxTools handles GET /v1/agents/sandbox-tools.
// @Summary List available sandbox tools
// @Description Returns the fixed list of tools and services that can be enabled in a dedicated agent sandbox.
// @Tags agents
// @Produce json
// @Success 200 {array} model.SandboxToolDefinition
// @Security BearerAuth
// @Router /v1/agents/sandbox-tools [get]
func (h *AgentHandler) ListSandboxTools(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, model.ValidSandboxTools)
}

// ListBuiltInTools handles GET /v1/agents/built-in-tools.
// @Summary List all built-in tools
// @Description Returns the complete list of built-in tools that can be granted to agents via permissions. Each tool includes its category and whether it is locked (cannot be toggled off).
// @Tags agents
// @Produce json
// @Success 200 {array} model.BuiltInToolDefinition
// @Security BearerAuth
// @Router /v1/agents/built-in-tools [get]
func (h *AgentHandler) ListBuiltInTools(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, model.ValidBuiltInTools)
}

