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
	systemagents "github.com/ziraloop/ziraloop/internal/system-agents"
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
		Meta:         model.JSON{"agent_id": agent.ID.String(), "identity_id": ptrToString(agent.IdentityID), "type": "agent_proxy"},
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
		Meta:         model.JSON{"agent_id": agent.ID.String(), "identity_id": ptrToString(agent.IdentityID), "type": "agent_proxy"},
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

	def.Config = applyAgentConfigDefaults(decodeJSONAs[bridgepkg.AgentConfig](agent.AgentConfig), agent.ProviderGroup, agent.Model)

	tools := decodeJSONAs[[]bridgepkg.ToolDefinition](agent.Tools)
	if tools != nil && len(*tools) > 0 {
		def.Tools = tools
	}

	mcpServers := decodeJSONAs[[]bridgepkg.McpServerDefinition](agent.McpServers)
	if mcpServers != nil && len(*mcpServers) > 0 {
		def.McpServers = mcpServers
	}

	skills := decodeJSONAs[[]bridgepkg.SkillDefinition](agent.Skills)
	if skills != nil && len(*skills) > 0 {
		def.Skills = skills
	}

	permissions := decodeJSONAs[map[string]bridgepkg.ToolPermission](agent.Permissions)
	if permissions != nil && len(*permissions) > 0 {
		def.Permissions = permissions
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
	providerGroup := systemagents.MapProviderToGroup(cred.ProviderID, agent.Model)
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

	// Set config with defaults for any unspecified fields
	def.Config = applyAgentConfigDefaults(decodeJSONAs[bridgepkg.AgentConfig](agent.AgentConfig), cred.ProviderID, agent.Model)

	// Set permissions if present
	permissions := decodeJSONAs[map[string]bridgepkg.ToolPermission](agent.Permissions)
	if permissions != nil && len(*permissions) > 0 {
		def.Permissions = permissions
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

	// Set skills if present
	skills := decodeJSONAs[[]bridgepkg.SkillDefinition](agent.Skills)
	if skills != nil && len(*skills) > 0 {
		def.Skills = skills
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

