package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	bridgepkg "github.com/ziraloop/ziraloop/internal/bridge"
	"github.com/ziraloop/ziraloop/internal/config"
	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/registry"
	subagents "github.com/ziraloop/ziraloop/internal/sub-agents"
	"github.com/ziraloop/ziraloop/internal/token"
)

// providerTypeMap maps our credential provider IDs to Bridge ProviderType values.
var providerTypeMap = map[string]bridgepkg.ProviderType{
	"openai":      bridgepkg.OpenAi,
	"anthropic":   bridgepkg.Anthropic,
	"google":      bridgepkg.Google,
	"groq":        bridgepkg.Groq,
	"fireworks":   bridgepkg.Fireworks,
	"openrouter":  bridgepkg.OpenAi, // OpenRouter uses OpenAI-compatible API
	"moonshotai":  bridgepkg.OpenAi, // Kimi uses OpenAI-compatible API
	"zai":         bridgepkg.OpenAi, // Z.AI uses OpenAI-compatible API
	"zhipuai":     bridgepkg.OpenAi, // Zhipu AI uses OpenAI-compatible API
	"fireworks-ai": bridgepkg.Fireworks,
	"ollama":      bridgepkg.Ollama,
}

const (
	agentTokenTTL      = 24 * time.Hour
	tokenRotationWindow = 3 * time.Hour // rotate when within 3h of expiry
)

// Pusher constructs Bridge AgentDefinitions from our Agent model
// and pushes them to Bridge instances running in sandboxes.
type Pusher struct {
	db           *gorm.DB
	orchestrator *Orchestrator
	signingKey   []byte // JWT signing key for minting proxy tokens
	cfg          *config.Config
	pushed       sync.Map // key: "{sandboxID}:{agentID}" → true
}

// NewPusher creates an agent pusher.
func NewPusher(db *gorm.DB, orchestrator *Orchestrator, signingKey []byte, cfg *config.Config) *Pusher {
	return &Pusher{
		db:           db,
		orchestrator: orchestrator,
		signingKey:   signingKey,
		cfg:          cfg,
	}
}

// isPushed checks if an agent has already been pushed to a sandbox (in-memory cache).
func (p *Pusher) isPushed(sandboxID, agentID string) bool {
	_, ok := p.pushed.Load(sandboxID + ":" + agentID)
	return ok
}

// markPushed records that an agent has been pushed to a sandbox.
func (p *Pusher) markPushed(sandboxID, agentID string) {
	p.pushed.Store(sandboxID+":"+agentID, true)
}

// PushAgent assigns a pool sandbox to the agent and pushes the agent definition to Bridge.
// For shared agents only — called on agent create/update.
//
// System agents are a no-op here: they live in the singleton system sandbox
// which is provisioned and populated at worker startup, then refreshed by
// the periodic SystemAgentSync task. Their sandbox_id is already set.
func (p *Pusher) PushAgent(ctx context.Context, agent *model.Agent) error {
	if agent.IsSystem {
		return nil
	}
	if agent.SandboxType != "shared" {
		return nil // dedicated agents are pushed lazily on conversation create
	}

	// Assign a pool sandbox (reuses existing if already assigned)
	sb, err := p.orchestrator.AssignPoolSandbox(ctx, agent)
	if err != nil {
		return fmt.Errorf("assigning pool sandbox: %w", err)
	}

	// Build and push
	return p.pushAgentToSandbox(ctx, agent, sb)
}

// PushAgentToSandbox pushes an agent definition to a specific sandbox.
// Uses a two-layer check to avoid redundant pushes that would cause Bridge
// to reload the agent and wipe active conversations:
//  1. In-memory cache (instant, survives within process lifetime)
//  2. Bridge API check (survives server restarts)
//
// System agents are a no-op here: they're pre-loaded into the singleton
// system sandbox at worker startup and re-pushed by the periodic
// SystemAgentSync task. Per-request pushes would defeat the periodic strategy.
func (p *Pusher) PushAgentToSandbox(ctx context.Context, agent *model.Agent, sb *model.Sandbox) error {
	if agent.IsSystem {
		return nil
	}

	sandboxID := sb.ID.String()
	agentID := agent.ID.String()

	// Layer 1: in-memory cache
	if p.isPushed(sandboxID, agentID) {
		return nil
	}

	// Layer 2: check Bridge directly
	client, err := p.orchestrator.GetBridgeClient(ctx, sb)
	if err == nil {
		if exists, checkErr := client.HasAgent(ctx, agentID); checkErr == nil && exists {
			p.markPushed(sandboxID, agentID)
			return nil
		}
	}

	// Not found in either layer — do the full push.
	// System agents return at the top of this function, so this is
	// always a non-system push.
	if err := p.pushAgentToSandbox(ctx, agent, sb); err != nil {
		return err
	}
	p.markPushed(sandboxID, agentID)
	return nil
}

// RemoveAgent removes an agent from Bridge and releases its pool sandbox assignment.
// For shared agents only — dedicated sandboxes are deleted entirely.
func (p *Pusher) RemoveAgent(ctx context.Context, agent *model.Agent) error {
	if agent.SandboxType != "shared" {
		return nil
	}

	if agent.SandboxID == nil {
		return nil // not assigned to any sandbox
	}

	// Load the assigned sandbox
	var sb model.Sandbox
	if err := p.db.Where("id = ? AND status = 'running'", *agent.SandboxID).First(&sb).Error; err != nil {
		// Sandbox not found or not running — just release the assignment
		_ = p.orchestrator.ReleasePoolSandbox(ctx, agent)
		return nil
	}

	// Remove from Bridge
	client, err := p.orchestrator.GetBridgeClient(ctx, &sb)
	if err != nil {
		slog.Warn("failed to get bridge client for agent removal", "agent_id", agent.ID, "sandbox_id", sb.ID, "error", err)
	} else {
		if err := client.RemoveAgentDefinition(ctx, agent.ID.String()); err != nil {
			slog.Warn("failed to remove agent from bridge", "agent_id", agent.ID, "sandbox_id", sb.ID, "error", err)
		}
	}

	// Clear in-memory push cache
	p.pushed.Delete(sb.ID.String() + ":" + agent.ID.String())

	// Release pool sandbox assignment (decrements agent count, clears agent.SandboxID)
	return p.orchestrator.ReleasePoolSandbox(ctx, agent)
}

// RotateAgentToken mints a new proxy token for an agent and pushes it to Bridge.
// Called lazily when a token is near expiry.
func (p *Pusher) RotateAgentToken(ctx context.Context, agent *model.Agent, sb *model.Sandbox) error {
	if agent.CredentialID == nil || agent.OrgID == nil {
		return fmt.Errorf("cannot rotate token for agent without credential and org")
	}

	var cred model.Credential
	if err := p.db.Where("id = ?", *agent.CredentialID).First(&cred).Error; err != nil {
		return fmt.Errorf("loading credential: %w", err)
	}

	// Mint new token
	proxyToken, jti, err := p.mintAgentToken(agent, &cred)
	if err != nil {
		return fmt.Errorf("minting new token: %w", err)
	}

	// Build scopes from agent integrations.
	rotateScopes := buildScopesFromIntegrations(agent.Integrations)
	var rotateScopesJSON model.JSON
	if len(rotateScopes) > 0 {
		rotateScopesJSON = model.JSON{"scopes": rotateScopes}
	}

	// Store in DB
	now := time.Now()
	expiresAt := now.Add(agentTokenTTL)
	dbToken := model.Token{
		OrgID:        *agent.OrgID,
		CredentialID: cred.ID,
		JTI:          jti,
		ExpiresAt:    expiresAt,
		Scopes:       rotateScopesJSON,
		Meta:         model.JSON{"agent_id": agent.ID.String(), "type": "agent_proxy"},
	}
	if err := p.db.Create(&dbToken).Error; err != nil {
		return fmt.Errorf("storing new token: %w", err)
	}

	// Push to Bridge
	client, err := p.orchestrator.GetBridgeClient(ctx, sb)
	if err != nil {
		return fmt.Errorf("getting bridge client: %w", err)
	}
	if err := client.RotateAPIKey(ctx, agent.ID.String(), proxyToken); err != nil {
		return fmt.Errorf("rotating key in bridge: %w", err)
	}

	// Revoke old tokens for this agent (keep the new one)
	p.db.Model(&model.Token{}).
		Where("meta->>'agent_id' = ? AND meta->>'type' = 'agent_proxy' AND jti != ?",
			agent.ID.String(), jti).
		Update("revoked_at", now)

	slog.Info("agent token rotated",
		"agent_id", agent.ID,
		"new_jti", jti,
		"expires_at", expiresAt.Format(time.RFC3339),
	)

	return nil
}

// NeedsTokenRotation checks if the agent's proxy token is within the rotation window.
func (p *Pusher) NeedsTokenRotation(agentID string) bool {
	var tok model.Token
	err := p.db.Where("meta->>'agent_id' = ? AND meta->>'type' = 'agent_proxy' AND revoked_at IS NULL",
		agentID).Order("created_at DESC").First(&tok).Error
	if err != nil {
		return true // no token found, needs one
	}
	return time.Until(tok.ExpiresAt) < tokenRotationWindow
}

func (p *Pusher) pushAgentToSandbox(ctx context.Context, agent *model.Agent, sb *model.Sandbox) error {
	if agent.CredentialID == nil || agent.OrgID == nil {
		return fmt.Errorf("cannot push agent without credential and org")
	}

	// Load credential for provider info
	var cred model.Credential
	if err := p.db.Where("id = ?", *agent.CredentialID).First(&cred).Error; err != nil {
		return fmt.Errorf("loading credential: %w", err)
	}

	// Mint a proxy token for this agent
	proxyToken, jti, err := p.mintAgentToken(agent, &cred)
	if err != nil {
		return fmt.Errorf("minting proxy token: %w", err)
	}

	// Build scopes from agent integrations so the MCP server exposes tools.
	scopes := buildScopesFromIntegrations(agent.Integrations)
	var scopesJSON model.JSON
	if len(scopes) > 0 {
		scopesJSON = model.JSON{"scopes": scopes}
	}

	// Store the token in DB
	now := time.Now()
	expiresAt := now.Add(agentTokenTTL)
	dbToken := model.Token{
		OrgID:        *agent.OrgID,
		CredentialID: cred.ID,
		JTI:          jti,
		ExpiresAt:    expiresAt,
		Scopes:       scopesJSON,
		Meta:         model.JSON{"agent_id": agent.ID.String(), "type": "agent_proxy"},
	}
	if err := p.db.Create(&dbToken).Error; err != nil {
		return fmt.Errorf("storing proxy token: %w", err)
	}

	// Build the Bridge AgentDefinition
	def := p.buildAgentDefinition(agent, &cred, proxyToken, jti)

	// Load and attach subagents — each inherits the parent's credential and model.
	subagentDefs, err := p.buildSubagentDefinitions(agent, &cred)
	if err != nil {
		return fmt.Errorf("building subagent definitions: %w", err)
	}
	if len(subagentDefs) > 0 {
		def.Subagents = &subagentDefs
	}

	// Push to Bridge
	client, err := p.orchestrator.GetBridgeClient(ctx, sb)
	if err != nil {
		return fmt.Errorf("getting bridge client: %w", err)
	}

	if err := client.UpsertAgent(ctx, agent.ID.String(), def); err != nil {
		return fmt.Errorf("pushing agent to bridge: %w", err)
	}

	slog.Info("agent pushed to bridge",
		"agent_id", agent.ID,
		"agent_name", agent.Name,
		"sandbox_id", sb.ID,
		"sandbox_type", sb.SandboxType,
	)

	return nil
}

// pushSystemAgentToSandbox builds and pushes a system agent definition to Bridge
// without a credential. Uses agent.ProviderGroup for the Bridge ProviderType and
// sets an empty API key — per-conversation auth token override will supply the real one.
// BuildSystemAgentDef builds a Bridge agent definition for a system agent.
// Exported so the forge controller can add MCP servers before upserting.
func (p *Pusher) BuildSystemAgentDef(agent *model.Agent) bridgepkg.AgentDefinition {
	providerType := bridgepkg.Custom
	if pt, ok := providerTypeMap[agent.ProviderGroup]; ok {
		providerType = pt
	}

	proxyBaseURL := fmt.Sprintf("https://%s", p.cfg.ProxyHost)

	def := bridgepkg.AgentDefinition{
		Id:           agent.ID.String(),
		Name:         agent.Name,
		Description:  agent.Description,
		SystemPrompt: agent.SystemPrompt,
		Provider: bridgepkg.ProviderConfig{
			ProviderType: providerType,
			Model:        agent.Model,
			ApiKey:       "", // per-conversation override will supply this
			BaseUrl:      &proxyBaseURL,
		},
	}

	systemPermissions := decodeJSONAs[map[string]bridgepkg.ToolPermission](agent.Permissions)
	def.Config = applyAgentConfigDefaults(decodeJSONAs[bridgepkg.AgentConfig](agent.AgentConfig), agent.ProviderGroup, agent.Model)
	applyImmortalDefault(def.Config, def.Provider, agent.ProviderGroup, agent.Model)
	applyHistoryStripDefault(def.Config)
	applyToolRequirementsDefault(def.Config, systemPermissions)

	tools := decodeJSONAs[[]bridgepkg.ToolDefinition](agent.Tools)
	if tools != nil && len(*tools) > 0 {
		def.Tools = tools
	}

	mcpServers := decodeJSONAs[[]bridgepkg.McpServerDefinition](agent.McpServers)
	if mcpServers != nil && len(*mcpServers) > 0 {
		def.McpServers = mcpServers
	}

	if bridgeSkills := p.loadBridgeSkills(agent.ID); len(bridgeSkills) > 0 {
		def.Skills = &bridgeSkills
	}

	if systemPermissions != nil && len(*systemPermissions) > 0 {
		def.Permissions = systemPermissions
	}

	return def
}

func (p *Pusher) pushSystemAgentToSandbox(ctx context.Context, agent *model.Agent, sb *model.Sandbox) error {
	def := p.BuildSystemAgentDef(agent)

	client, err := p.orchestrator.GetBridgeClient(ctx, sb)
	if err != nil {
		return fmt.Errorf("getting bridge client: %w", err)
	}

	if err := client.UpsertAgent(ctx, agent.ID.String(), def); err != nil {
		return fmt.Errorf("pushing system agent to bridge: %w", err)
	}

	slog.Info("system agent pushed to bridge",
		"agent_id", agent.ID,
		"agent_name", agent.Name,
		"sandbox_id", sb.ID,
	)

	return nil
}

// PushAllSystemAgents loads every is_system=true active agent and upserts its
// definition into the given sandbox's Bridge. Idempotent — UpsertAgent
// overwrites existing definitions, so this safely propagates YAML edits and
// recovers from a Bridge restart that lost in-memory agent state.
//
// Called from worker startup (after the seeder) and from the periodic
// SystemAgentSync Asynq task. A failure on one agent is logged and skipped;
// the function returns an aggregated error only if at least one push failed.
func (p *Pusher) PushAllSystemAgents(ctx context.Context, sb *model.Sandbox) error {
	var agents []model.Agent
	if err := p.db.WithContext(ctx).
		Where("is_system = true AND status = ?", "active").
		Find(&agents).Error; err != nil {
		return fmt.Errorf("loading system agents: %w", err)
	}

	if len(agents) == 0 {
		slog.Info("no system agents to push", "sandbox_id", sb.ID)
		return nil
	}

	client, err := p.orchestrator.GetBridgeClient(ctx, sb)
	if err != nil {
		return fmt.Errorf("getting bridge client for system sandbox: %w", err)
	}

	var failed []string
	for i := range agents {
		agent := &agents[i]
		def := p.BuildSystemAgentDef(agent)
		if err := client.UpsertAgent(ctx, agent.ID.String(), def); err != nil {
			slog.Error("failed to push system agent",
				"agent_id", agent.ID, "agent_name", agent.Name, "error", err)
			failed = append(failed, agent.Name)
			continue
		}
		// Mark in the layer-1 cache so any stray code path that still calls
		// PushAgentToSandbox for a system agent (there shouldn't be any) is
		// also a fast no-op.
		p.markPushed(sb.ID.String(), agent.ID.String())
	}

	slog.Info("system agents synced to bridge",
		"sandbox_id", sb.ID,
		"total", len(agents),
		"succeeded", len(agents)-len(failed),
		"failed", len(failed),
	)

	if len(failed) > 0 {
		return fmt.Errorf("failed to push %d/%d system agents: %s",
			len(failed), len(agents), strings.Join(failed, ", "))
	}
	return nil
}

func (p *Pusher) mintAgentToken(agent *model.Agent, cred *model.Credential) (tokenStr, jti string, err error) {
	if agent.OrgID == nil {
		return "", "", fmt.Errorf("cannot mint token for agent without org_id")
	}
	tokenStr, jti, err = token.Mint(
		p.signingKey,
		(*agent.OrgID).String(),
		cred.ID.String(),
		agentTokenTTL,
	)
	if err != nil {
		return "", "", err
	}
	// Add ptok_ prefix
	tokenStr = "ptok_" + tokenStr
	return tokenStr, jti, nil
}

func (p *Pusher) buildAgentDefinition(agent *model.Agent, cred *model.Credential, proxyToken, jti string) bridgepkg.AgentDefinition {
	// Always use the real provider type so Bridge formats requests correctly
	// for the upstream LLM provider. Our proxy transparently forwards these.
	providerType := bridgepkg.Custom
	if pt, ok := providerTypeMap[cred.ProviderID]; ok {
		providerType = pt
	}

	// Build proxy base URL — Bridge will call our proxy for LLM requests
	// For providers that use non-Bearer auth (e.g. Anthropic uses x-api-key),
	// we strip the /v1/proxy prefix so the full upstream path is preserved.
	proxyBaseURL := fmt.Sprintf("https://%s", p.cfg.ProxyHost)

	// Resolve the system prompt for the credential's provider group.
	providerGroup := subagents.MapProviderToGroup(cred.ProviderID, agent.Model)
	systemPrompt, _ := agent.ResolveProviderConfig(providerGroup)

	// Append cloned repository context to the system prompt
	if repoContext := buildRepoContext(agent.Resources); repoContext != "" {
		systemPrompt += "\n\n" + repoContext
	}

	def := bridgepkg.AgentDefinition{
		Id:           agent.ID.String(),
		Name:         agent.Name,
		Description:  agent.Description,
		SystemPrompt: systemPrompt,
		Provider: bridgepkg.ProviderConfig{
			ProviderType: providerType,
			Model:        agent.Model,
			ApiKey:       proxyToken,
			BaseUrl:      &proxyBaseURL,
		},
	}

	permissions := decodeJSONAs[map[string]bridgepkg.ToolPermission](agent.Permissions)

	// Set config with defaults for any unspecified fields. Permissions are
	// passed into the tool-requirements helper so deny'd tools (e.g.
	// journal_write disabled on Stallone) aren't auto-required — Bridge
	// rejects a push where a requirement overlaps with disabled_tools.
	def.Config = applyAgentConfigDefaults(decodeJSONAs[bridgepkg.AgentConfig](agent.AgentConfig), cred.ProviderID, agent.Model)
	applyImmortalDefault(def.Config, def.Provider, cred.ProviderID, agent.Model)
	applyHistoryStripDefault(def.Config)
	applyToolRequirementsDefault(def.Config, permissions)

	// Set permissions if present. Tools with "deny" are removed from
	// permissions and added to DisabledTools so Bridge never presents
	// them to the LLM — the agent won't waste turns calling denied tools.
	if permissions != nil && len(*permissions) > 0 {
		var disabledTools []string
		allowed := make(map[string]bridgepkg.ToolPermission)
		for tool, perm := range *permissions {
			if perm == bridgepkg.ToolPermissionDeny {
				disabledTools = append(disabledTools, tool)
			} else {
				allowed[tool] = perm
			}
		}
		if len(allowed) > 0 {
			def.Permissions = &allowed
		}
		if len(disabledTools) > 0 {
			def.Config.DisabledTools = &disabledTools
		}
	}

	// Set tools if present.
	tools := decodeJSONAs[[]bridgepkg.ToolDefinition](agent.Tools)
	if tools != nil && len(*tools) > 0 {
		def.Tools = tools
	}

	// Set MCP servers — start with user-configured ones
	mcpServers := decodeJSONAs[[]bridgepkg.McpServerDefinition](agent.McpServers)

	// Add our MCP server only if agent has integrations configured
	hasIntegrations := agent.Integrations != nil && len(agent.Integrations) > 0
	if hasIntegrations && p.cfg.MCPBaseURL != "" && jti != "" {
		ourMCP := buildZiraLoopMCPServer(p.cfg.MCPBaseURL, jti, proxyToken)
		if mcpServers == nil {
			servers := []bridgepkg.McpServerDefinition{ourMCP}
			mcpServers = &servers
		} else {
			*mcpServers = append(*mcpServers, ourMCP)
		}
	}
	if mcpServers != nil && len(*mcpServers) > 0 {
		def.McpServers = mcpServers
	}

	// Load skills from agent_skills join table → skill_versions bundle
	if bridgeSkills := p.loadBridgeSkills(agent.ID); len(bridgeSkills) > 0 {
		def.Skills = &bridgeSkills
	}

	return def
}

// buildSubagentDefinitions loads all subagents attached to the parent agent
// and builds a Bridge AgentDefinition for each one. Each subagent inherits
// the parent's credential and model, and gets its own proxy token.
func (p *Pusher) buildSubagentDefinitions(parent *model.Agent, parentCred *model.Credential) ([]bridgepkg.AgentDefinition, error) {
	var links []model.AgentSubagent
	if err := p.db.Where("agent_id = ?", parent.ID).Find(&links).Error; err != nil {
		return nil, fmt.Errorf("querying agent_subagents: %w", err)
	}
	if len(links) == 0 {
		return nil, nil
	}

	subagentIDs := make([]uuid.UUID, len(links))
	for index, link := range links {
		subagentIDs[index] = link.SubagentID
	}

	var subagents []model.Agent
	if err := p.db.Where("id IN ?", subagentIDs).Find(&subagents).Error; err != nil {
		return nil, fmt.Errorf("loading subagents: %w", err)
	}

	defs := make([]bridgepkg.AgentDefinition, 0, len(subagents))
	for _, sub := range subagents {
		// Override subagent's model with parent's model.
		sub.Model = parent.Model

		// Mint and persist a proxy token for this subagent using the parent's credential.
		proxyTok, err := token.MintAndPersist(p.db, p.signingKey, *parent.OrgID, parentCred.ID, agentTokenTTL, map[string]any{
			"agent_id":        sub.ID.String(),
			"parent_agent_id": parent.ID.String(),
			"type":            "subagent_proxy",
		})
		if err != nil {
			return nil, fmt.Errorf("minting proxy token for subagent %s: %w", sub.ID, err)
		}

		defs = append(defs, p.buildAgentDefinition(&sub, parentCred, proxyTok.TokenString, proxyTok.JTI))
	}

	return defs, nil
}

func buildZiraLoopMCPServer(mcpBaseURL, jti, token string) bridgepkg.McpServerDefinition {
	// Our MCP server uses the JTI as the path and the proxy token for auth
	url := fmt.Sprintf("%s/%s", mcpBaseURL, jti)

	var transport bridgepkg.McpTransport
	httpTransport := bridgepkg.McpTransport1{
		Type: bridgepkg.StreamableHttp,
		Url:  url,
	}
	if token != "" {
		headers := map[string]string{"Authorization": "Bearer " + token}
		httpTransport.Headers = &headers
	}
	transport.FromMcpTransport1(httpTransport)

	return bridgepkg.McpServerDefinition{
		Name:      "ziraloop",
		Transport: transport,
	}
}

// buildScopesFromIntegrations converts the agent's Integrations JSON
// (map[connectionID] → {actions: [...]}) into MCP TokenScope format.
func buildScopesFromIntegrations(integrations model.JSON) []map[string]any {
	if len(integrations) == 0 {
		return nil
	}

	var scopes []map[string]any
	for connectionID, config := range integrations {
		configMap, ok := config.(map[string]any)
		if !ok {
			continue
		}
		actionsRaw, ok := configMap["actions"]
		if !ok {
			continue
		}
		actionsSlice, ok := actionsRaw.([]any)
		if !ok {
			continue
		}
		actions := make([]string, 0, len(actionsSlice))
		for _, action := range actionsSlice {
			if actionStr, ok := action.(string); ok {
				actions = append(actions, actionStr)
			}
		}
		if len(actions) > 0 {
			scopes = append(scopes, map[string]any{
				"connection_id": connectionID,
				"actions":       actions,
			})
		}
	}

	return scopes
}

func ptrToString(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

// decodeJSONAs converts a model.JSON (map[string]any) to a typed struct via JSON round-trip.
// Returns nil if the input is nil or empty.
func decodeJSONAs[T any](j model.JSON) *T {
	if j == nil || len(j) == 0 {
		return nil
	}
	b, err := json.Marshal(j)
	if err != nil {
		return nil
	}
	var result T
	if err := json.Unmarshal(b, &result); err != nil {
		return nil
	}
	return &result
}

// loadBridgeSkills loads an agent's attached skills from the agent_skills join
// table and converts each skill's latest version bundle into a Bridge SkillDefinition.
func (p *Pusher) loadBridgeSkills(agentID uuid.UUID) []bridgepkg.SkillDefinition {
	var links []model.AgentSkill
	if err := p.db.Where("agent_id = ?", agentID).Find(&links).Error; err != nil || len(links) == 0 {
		return nil
	}

	skillIDs := make([]uuid.UUID, len(links))
	for index, link := range links {
		skillIDs[index] = link.SkillID
	}

	// Load skills with their latest version
	var skills []model.Skill
	if err := p.db.Where("id IN ?", skillIDs).Find(&skills).Error; err != nil {
		return nil
	}

	// Collect version IDs
	var versionIDs []uuid.UUID
	for _, skill := range skills {
		if skill.LatestVersionID != nil {
			versionIDs = append(versionIDs, *skill.LatestVersionID)
		}
	}
	if len(versionIDs) == 0 {
		return nil
	}

	// Load version bundles
	var versions []model.SkillVersion
	if err := p.db.Where("id IN ?", versionIDs).Find(&versions).Error; err != nil {
		return nil
	}
	versionByID := make(map[uuid.UUID]model.SkillVersion, len(versions))
	for _, version := range versions {
		versionByID[version.ID] = version
	}

	// Convert bundles to Bridge SkillDefinitions
	var result []bridgepkg.SkillDefinition
	for _, skill := range skills {
		if skill.LatestVersionID == nil {
			continue
		}
		version, ok := versionByID[*skill.LatestVersionID]
		if !ok {
			continue
		}
		var def bridgepkg.SkillDefinition
		if err := json.Unmarshal(version.Bundle, &def); err != nil {
			slog.Warn("failed to unmarshal skill bundle", "skill_id", skill.ID, "error", err)
			continue
		}
		result = append(result, def)
	}

	return result
}

// applyAgentConfigDefaults fills in sensible defaults for any AgentConfig fields
// the user did not explicitly set. The providerID and model are used to pick
// the best defaults for the specific LLM.
func applyAgentConfigDefaults(cfg *bridgepkg.AgentConfig, providerID, modelName string) *bridgepkg.AgentConfig {
	if cfg == nil {
		cfg = &bridgepkg.AgentConfig{}
	}

	setDefault := func(ptr **int32, val int32) {
		if *ptr == nil {
			*ptr = &val
		}
	}

	setDefault(&cfg.MaxTokens, defaultMaxTokens(providerID, modelName))
	setDefault(&cfg.MaxTurns, 250)
	setDefault(&cfg.MaxTasksPerConversation, 50)
	setDefault(&cfg.MaxConcurrentConversations, 100)

	if cfg.Temperature == nil {
		temp := defaultTemperature(providerID, modelName)
		cfg.Temperature = &temp
	}

	return cfg
}

// providerCheckpointModel maps a credential provider ID to the model we use
// for ImmortalConfig checkpoint extraction. The choice prioritizes speed
// and cost, since the checkpoint call runs every time the primary chain
// hits the token budget — a pure summarization job that a small fast model
// handles well. Providers missing from this map get no auto-generated
// immortal config; authors can still opt in by setting AgentConfig.Immortal
// explicitly in the agent definition.
var providerCheckpointModel = map[string]string{
	"anthropic":    "claude-3-5-haiku-latest",
	"google":       "gemini-2.5-flash",
	"openrouter":   "google/gemini-2.5-flash",
	"openai":       "gpt-5.1-codex-mini",
	"groq":         "openai/gpt-oss-20b",
	"fireworks":    "accounts/fireworks/models/minimax-m2p1",
	"fireworks-ai": "accounts/fireworks/models/minimax-m2p1",
	"moonshotai":   "kimi-k2-thinking",
	"zai":          "glm-4.7-flash",
	"zhipuai":      "glm-4.7-flash",
}

// immortalTokenBudgetFraction is the fraction of the parent model's context
// window at which Bridge should cut a new chain. Set at 50%: earlier resets
// cap history growth (the dominant cost driver in long conversations) even
// though each reset pays a checkpoint-extraction LLM call. In practice the
// checkpoint cost is dwarfed by the savings from running subsequent turns
// against a small carry-forward instead of a 100k+ token history. Raise if
// you see conversations bouncing off the budget (frequent chain handoffs
// with low token pressure between them).
const immortalTokenBudgetFraction = 0.50

// fallbackParentContextWindow is used when the parent model is missing from
// the curated registry (e.g. a freshly-added provider model the agent picked
// before we curated it). 128k covers the vast majority of modern models
// without overshooting smaller ones too aggressively.
const fallbackParentContextWindow = 128_000

// applyImmortalDefault populates cfg.Immortal with a checkpoint provider and
// token budget when the agent author hasn't set one explicitly. Call this
// AFTER applyAgentConfigDefaults. No-op when:
//
//   - cfg.Immortal is already set (author override wins)
//   - the credential provider has no entry in providerCheckpointModel
//     (we refuse to guess a checkpoint model for unsupported providers)
//
// The checkpoint ProviderConfig mirrors the primary: same ProviderType,
// ApiKey (proxy token), and BaseUrl. Only Model is swapped for the cheap
// fast variant. This lets our proxy route checkpoint calls through the
// same credential-backed tunnel as the primary conversation.
func applyImmortalDefault(
	cfg *bridgepkg.AgentConfig,
	primary bridgepkg.ProviderConfig,
	providerID, primaryModel string,
) {
	if cfg == nil || cfg.Immortal != nil {
		return
	}
	checkpointModel, ok := providerCheckpointModel[providerID]
	if !ok {
		return
	}

	tokenBudget := int32(float64(parentContextWindow(providerID, primaryModel)) * immortalTokenBudgetFraction)

	cfg.Immortal = &bridgepkg.ImmortalConfig{
		CheckpointProvider: bridgepkg.ProviderConfig{
			ProviderType: primary.ProviderType,
			Model:        checkpointModel,
			ApiKey:       primary.ApiKey,
			BaseUrl:      primary.BaseUrl,
		},
		TokenBudget: &tokenBudget,
	}
}

// parentContextWindow returns the context window (in tokens) for the
// primary model from the curated registry, falling back to a safe default
// when the model isn't curated yet.
func parentContextWindow(providerID, modelID string) int64 {
	if prov, ok := registry.Global().GetProvider(providerID); ok {
		if m, ok := prov.Models[modelID]; ok && m.Limit != nil && m.Limit.Context > 0 {
			return m.Limit.Context
		}
	}
	return fallbackParentContextWindow
}

// defaultRequirementCadenceTurns is how often the memory + journal loop
// runs. Every 5 turns strikes a balance between keeping the agent's
// recall sharp and not burning an LLM call on bookkeeping every turn.
// defaultRequirementCadenceTurns is how often Bridge checks whether a
// required tool has been called. 10 is a compromise: strict enough to
// still nudge agents into the memory/journal loop, loose enough that
// naturally exploration-heavy turns (sub-agent calls, long tool chains)
// don't trip the check on every pass.
//
// Was 5; relaxed after observing that every violation fires a
// `tool_requirement_violated` event and (under NextTurnReminder
// enforcement) writes a `<system-reminder>` into the next user message
// — which then lives forever in history and breaks the provider prompt
// cache at that point. Each fragmentation pushes subsequent-turn cache
// match back. Cutting the violation rate cuts the fragmentation.
const defaultRequirementCadenceTurns = 10

// applyToolRequirementsDefault populates cfg.ToolRequirements with the
// memory + journal loop every agent is expected to run: recall at the
// start of the window, retain + journal anywhere in it. The cadence
// nudges the loop even if the system prompt doesn't explicitly ask.
//
// Semantics (all three share cadence=every_n_turns, n=10):
//   - memory_recall — pulled to turn_start so the agent reads memory
//     BEFORE it does work that would benefit from it.
//   - memory_retain — anywhere in the turn; the agent writes out what
//     it learned at a natural point in its reasoning.
//   - journal_write — anywhere; separate from memory_retain because
//     journal captures narrative/observations, memory captures durable
//     facts. Both matter but fill different roles.
//
// Enforcement is set to Warn (not the Bridge default of
// NextTurnReminder). Reasoning: NextTurnReminder prepends a
// `<system-reminder>` block to the next user message, which gets
// committed to history and fragments the cacheable prefix on every
// subsequent turn. Warn logs the miss and fires the
// `tool_requirement_violated` event for observability without touching
// the prompt. If a specific agent needs the harder guarantee, override
// `ToolRequirements` explicitly on its agent_config with
// `enforcement: next_turn_reminder` per requirement.
//
// Any candidate tool that is also disabled (either in cfg.DisabledTools
// directly or via a "deny" entry in the agent's permissions map) is
// skipped — Bridge rejects a push where tool_requirements overlaps with
// disabled_tools.
//
// Opt-out: authors can disable the entire default list by setting
// `ToolRequirements` to an empty slice `[]` on the agent's AgentConfig.
// Only a nil (unset) list triggers auto-injection; a non-nil slice of
// any length — including empty — wins.
func applyToolRequirementsDefault(
	cfg *bridgepkg.AgentConfig,
	permissions *map[string]bridgepkg.ToolPermission,
) {
	if cfg == nil || cfg.ToolRequirements != nil {
		return
	}

	disabled := make(map[string]bool)
	if cfg.DisabledTools != nil {
		for _, tool := range *cfg.DisabledTools {
			disabled[tool] = true
		}
	}
	if permissions != nil {
		for tool, perm := range *permissions {
			if perm == bridgepkg.ToolPermissionDeny {
				disabled[tool] = true
			}
		}
	}

	turnStart := bridgepkg.TurnStart
	warn := bridgepkg.Warn
	candidates := []struct {
		tool     string
		position *bridgepkg.RequirementPosition
	}{
		{"memory_recall", &turnStart},
		{"memory_retain", nil},
		{"journal_write", nil},
	}

	var reqs []bridgepkg.ToolRequirement
	for _, candidate := range candidates {
		if disabled[candidate.tool] {
			continue
		}
		reqs = append(reqs, bridgepkg.ToolRequirement{
			Tool:        candidate.tool,
			Cadence:     newEveryNTurnsCadence(defaultRequirementCadenceTurns),
			Position:    candidate.position,
			Enforcement: &warn,
		})
	}
	if len(reqs) > 0 {
		cfg.ToolRequirements = &reqs
	}
}

// newEveryNTurnsCadence builds a RequirementCadence union value for the
// "every N turns" variant. Each call returns a fresh pointer so callers
// don't accidentally share union state between requirements.
func newEveryNTurnsCadence(n int32) *bridgepkg.RequirementCadence {
	var cadence bridgepkg.RequirementCadence
	_ = cadence.FromRequirementCadence2(bridgepkg.RequirementCadence2{
		Type: bridgepkg.EveryNTurns,
		N:    n,
	})
	return &cadence
}

// historyStripPinRecent is the number of most-recent tool results bridge keeps
// verbatim regardless of age. The agent is actively reasoning over these; we
// don't want to strip what it just looked at.
const historyStripPinRecent = 5

// historyStripAgeThreshold is the number of assistant messages that must
// follow a tool result before it becomes strippable. 3 means "a result stays
// verbatim for 3 more turns after it lands, then gets compressed to a
// spill-file pointer". Conservative enough that the agent almost always has
// the recent tool output it's actively citing, while aggressive enough to cap
// history growth.
const historyStripAgeThreshold = 3

// applyHistoryStripDefault populates cfg.HistoryStrip with bridge's
// tool-result-stripping contract when the author hasn't set one. Stripping
// rewrites old tool-result bodies in the LLM-visible prompt to tiny "use
// RipGrep on the spill file if you need this" markers. The replacement text
// is byte-stable across turns, so once a result transitions to stripped the
// provider's prompt cache only breaks once (at first-strip) rather than
// continuously paying the full body on every turn. Persistence is untouched
// — the agent's full conversation record stays intact on disk.
//
// Without this, a long conversation accumulates every tool result verbatim
// forever. On Kira's incident-handling runs this added ~100k+ tokens of
// stale tool output to each LLM call by turn 20, dominating input cost.
//
// Respects author override: a non-nil HistoryStrip stays as-is.
func applyHistoryStripDefault(cfg *bridgepkg.AgentConfig) {
	if cfg == nil || cfg.HistoryStrip != nil {
		return
	}
	enabled := true
	pinErrors := true
	pinRecent := historyStripPinRecent
	ageThreshold := historyStripAgeThreshold
	cfg.HistoryStrip = &bridgepkg.HistoryStripConfig{
		Enabled:        &enabled,
		PinErrors:      &pinErrors,
		PinRecentCount: &pinRecent,
		AgeThreshold:   &ageThreshold,
	}
}

// defaultMaxTokens returns a sensible max_tokens default at ~80% of the model's
// output limit. Model-specific overrides are checked first, then provider-level
// defaults. Values are derived from internal/registry/models.json.
func defaultMaxTokens(providerID, modelName string) int32 {
	// Model-specific overrides — covers models whose output limits differ
	// significantly from the provider median.
	switch {
	// OpenAI: gpt-5-pro has 272k output, gpt-5.x codex/pro have 128k,
	// but chat models and older ones are 16k.
	case strings.Contains(modelName, "gpt-5-pro"):
		return 217600 // 80% of 272,000
	case strings.Contains(modelName, "gpt-5") && !strings.Contains(modelName, "chat"):
		return 102400 // 80% of 128,000
	case strings.Contains(modelName, "o3") || strings.Contains(modelName, "o4") || strings.Contains(modelName, "o1"):
		return 80000 // 80% of 100,000

	// Kimi: k2.5/thinking models have 262k output, instruct models have 16k.
	case strings.Contains(modelName, "kimi") && strings.Contains(modelName, "instruct"):
		return 13107 // 80% of 16,384
	case strings.Contains(modelName, "kimi"):
		return 209715 // 80% of 262,144

	// MiniMax: M2+ models have 131k output, M2 has 128k.
	case strings.Contains(modelName, "minimax") || strings.Contains(modelName, "MiniMax"):
		return 104857 // 80% of 131,072

	// GLM: 4.7+ have 131k, 4.5 has 98k, older have 32k.
	case strings.Contains(modelName, "glm-5") || strings.Contains(modelName, "glm-4.7") || strings.Contains(modelName, "glm-4.6"):
		return 104857 // 80% of 131,072
	case strings.Contains(modelName, "glm-4.5"):
		return 78643 // 80% of 98,304
	case strings.Contains(modelName, "glm"):
		return 26214 // 80% of 32,768
	}

	// Provider-level defaults — 80% of the most common output limit.
	switch providerID {
	case "anthropic":
		return 51200 // 80% of 64,000 (most models; opus-4-6 is 128k but is the exception)
	case "openai":
		return 102400 // 80% of 128,000
	case "google":
		return 52428 // 80% of 65,536
	case "moonshotai":
		return 209715 // 80% of 262,144
	case "zai", "zhipuai":
		return 104857 // 80% of 131,072
	case "minimax":
		return 104857 // 80% of 131,072
	default:
		return 13107 // 80% of 16,384 — safe fallback for unknown providers
	}
}

// defaultTemperature returns the recommended default temperature for a given
// provider/model combination based on each provider's official guidance.
func defaultTemperature(providerID, modelName string) float64 {
	// Check model-specific overrides first (reasoning/thinking models).
	// We always default to thinking-mode temperatures for best reasoning output.
	if strings.Contains(modelName, "kimi") {
		// Kimi K2 Thinking mode recommends 1.0.
		return 1.0
	}
	if strings.Contains(modelName, "deepseek-r1") || strings.Contains(modelName, "deepseek-reasoner") {
		// DeepSeek R1 recommends 0.6 for thinking mode.
		return 0.6
	}
	if strings.Contains(modelName, "o1") || strings.Contains(modelName, "o3") || strings.Contains(modelName, "o4") {
		// OpenAI reasoning models ignore temperature; pass 1.0 (their default).
		return 1.0
	}

	// Provider-level defaults based on official documentation.
	switch providerID {
	case "anthropic":
		// Anthropic defaults to 1.0; range 0-1.
		return 1.0
	case "google":
		// Google recommends keeping Gemini at 1.0.
		return 1.0
	case "openai":
		// OpenAI defaults to 1.0.
		return 1.0
	case "deepseek":
		// DeepSeek V3 API maps 1.0 → internal 0.3. Sending 1.0 is correct.
		return 1.0
	case "cohere":
		// Cohere defaults to 0.3.
		return 0.3
	case "xai":
		// xAI Grok defaults to 0.7 in most integrations.
		return 0.7
	case "mistral":
		// Mistral recommends 0.7 for general use.
		return 0.7
	default:
		return 0.7
	}
}

// buildRepoContext generates a system prompt section listing cloned repositories.
func buildRepoContext(resources model.JSON) string {
	if resources == nil || len(resources) == 0 {
		return ""
	}

	type repo struct {
		id   string
		name string
	}
	var repos []repo

	for _, resourceTypes := range resources {
		typesMap, ok := resourceTypes.(map[string]any)
		if !ok {
			continue
		}
		repoList, ok := typesMap["repository"]
		if !ok {
			continue
		}
		repoSlice, ok := repoList.([]any)
		if !ok {
			continue
		}
		for _, item := range repoSlice {
			itemMap, ok := item.(map[string]any)
			if !ok {
				continue
			}
			repoID, _ := itemMap["id"].(string)
			repoName, _ := itemMap["name"].(string)
			if repoID != "" && repoName != "" {
				repos = append(repos, repo{id: repoID, name: repoName})
			}
		}
	}

	if len(repos) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("── CLONED REPOSITORIES ──\n\n")
	builder.WriteString("The following GitHub repositories have been cloned into your workspace:\n\n")
	for _, repo := range repos {
		builder.WriteString(fmt.Sprintf("  - %s → /home/daytona/repos/%s\n", repo.id, repo.name))
	}
	builder.WriteString("\nYou can read, search, and modify files in these directories directly.")
	return builder.String()
}

