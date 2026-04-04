package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	bridgepkg "github.com/ziraloop/ziraloop/internal/bridge"
	"github.com/ziraloop/ziraloop/internal/config"
	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/token"
)

// providerTypeMap maps our credential provider IDs to Bridge ProviderType values.
var providerTypeMap = map[string]bridgepkg.ProviderType{
	"openai":    bridgepkg.OpenAi,
	"anthropic": bridgepkg.Anthropic,
	"google":    bridgepkg.Google,
	"cohere":    bridgepkg.Cohere,
	"groq":      bridgepkg.Groq,
	"deepseek":  bridgepkg.DeepSeek,
	"mistral":   bridgepkg.Mistral,
	"fireworks": bridgepkg.Fireworks,
	"together":  bridgepkg.Together,
	"xai":       bridgepkg.XAi,
	"ollama":    bridgepkg.Ollama,
}

const (
	agentTokenTTL      = 24 * time.Hour
	tokenRotationWindow = 3 * time.Hour // rotate when within 3h of expiry
)

// Pusher constructs Bridge AgentDefinitions from our Agent model
// and pushes them to Bridge instances running in sandboxes.
type Pusher struct {
	db              *gorm.DB
	orchestrator    *Orchestrator
	signingKey      []byte // JWT signing key for minting proxy tokens
	cfg             *config.Config
	pushed          sync.Map // key: "{sandboxID}:{agentID}" → true
	hindsightMCPURL func(uuid.UUID) string // nil = no memory; returns MCP URL for an agent
}

// NewPusher creates an agent pusher. hindsightMCPURL is optional (nil disables memory MCP injection).
func NewPusher(db *gorm.DB, orchestrator *Orchestrator, signingKey []byte, cfg *config.Config, hindsightMCPURL func(uuid.UUID) string) *Pusher {
	return &Pusher{
		db:              db,
		orchestrator:    orchestrator,
		signingKey:      signingKey,
		cfg:             cfg,
		hindsightMCPURL: hindsightMCPURL,
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
func (p *Pusher) PushAgent(ctx context.Context, agent *model.Agent) error {
	if agent.SandboxType != "shared" {
		return nil // dedicated agents are pushed lazily on conversation create
	}

	// Assign a pool sandbox (reuses existing if already assigned)
	sb, err := p.orchestrator.AssignPoolSandbox(ctx, agent)
	if err != nil {
		return fmt.Errorf("assigning pool sandbox: %w", err)
	}

	// System agents don't have credentials — push without a proxy token.
	// Per-conversation auth token override will supply the real token.
	if agent.IsSystem {
		return p.pushSystemAgentToSandbox(ctx, agent, sb)
	}

	// Build and push
	return p.pushAgentToSandbox(ctx, agent, sb)
}

// PushAgentToSandbox pushes an agent definition to a specific sandbox.
// Uses a two-layer check to avoid redundant pushes that would cause Bridge
// to reload the agent and wipe active conversations:
//  1. In-memory cache (instant, survives within process lifetime)
//  2. Bridge API check (survives server restarts)
func (p *Pusher) PushAgentToSandbox(ctx context.Context, agent *model.Agent, sb *model.Sandbox) error {
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

	// Not found in either layer — do the full push
	if agent.IsSystem {
		if err := p.pushSystemAgentToSandbox(ctx, agent, sb); err != nil {
			return err
		}
	} else {
		if err := p.pushAgentToSandbox(ctx, agent, sb); err != nil {
			return err
		}
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

	// Store in DB
	now := time.Now()
	expiresAt := now.Add(agentTokenTTL)
	dbToken := model.Token{
		OrgID:        *agent.OrgID,
		CredentialID: cred.ID,
		JTI:          jti,
		ExpiresAt:    expiresAt,
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

	// Store the token in DB
	now := time.Now()
	expiresAt := now.Add(agentTokenTTL)
	dbToken := model.Token{
		OrgID:        *agent.OrgID,
		CredentialID: cred.ID,
		JTI:          jti,
		ExpiresAt:    expiresAt,
		Meta:         model.JSON{"agent_id": agent.ID.String(), "identity_id": ptrToString(agent.IdentityID), "type": "agent_proxy"},
	}
	if err := p.db.Create(&dbToken).Error; err != nil {
		return fmt.Errorf("storing proxy token: %w", err)
	}

	// Build the Bridge AgentDefinition
	def := p.buildAgentDefinition(agent, &cred, proxyToken, jti)

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
func (p *Pusher) pushSystemAgentToSandbox(ctx context.Context, agent *model.Agent, sb *model.Sandbox) error {
	providerType := bridgepkg.Custom
	if pt, ok := providerTypeMap[agent.ProviderGroup]; ok {
		providerType = pt
	}

	proxyBaseURL := fmt.Sprintf("https://%s/v1/proxy", p.cfg.BridgeHost)

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

	// Set config if present
	agentConfig := decodeJSONAs[bridgepkg.AgentConfig](agent.AgentConfig)
	if agentConfig != nil {
		def.Config = agentConfig
	}

	// Set tools if present
	tools := decodeJSONAs[[]bridgepkg.ToolDefinition](agent.Tools)
	if tools != nil && len(*tools) > 0 {
		def.Tools = tools
	}

	// Set MCP servers if present
	mcpServers := decodeJSONAs[[]bridgepkg.McpServerDefinition](agent.McpServers)
	if mcpServers != nil && len(*mcpServers) > 0 {
		def.McpServers = mcpServers
	}

	// Set skills if present
	skills := decodeJSONAs[[]bridgepkg.SkillDefinition](agent.Skills)
	if skills != nil && len(*skills) > 0 {
		def.Skills = skills
	}

	// Set subagents if present
	subagents := decodeJSONAs[[]bridgepkg.AgentDefinition](agent.Subagents)
	if subagents != nil && len(*subagents) > 0 {
		def.Subagents = subagents
	}

	// Set permissions if present
	permissions := decodeJSONAs[map[string]bridgepkg.ToolPermission](agent.Permissions)
	if permissions != nil && len(*permissions) > 0 {
		def.Permissions = permissions
	}

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
	proxyBaseURL := fmt.Sprintf("https://%s/v1/proxy", p.cfg.BridgeHost)

	def := bridgepkg.AgentDefinition{
		Id:           agent.ID.String(),
		Name:         agent.Name,
		Description:  agent.Description,
		SystemPrompt: agent.SystemPrompt,
		Provider: bridgepkg.ProviderConfig{
			ProviderType: providerType,
			Model:        agent.Model,
			ApiKey:       proxyToken,
			BaseUrl:      &proxyBaseURL,
		},
	}

	// Set config if present
	agentConfig := decodeJSONAs[bridgepkg.AgentConfig](agent.AgentConfig)
	if agentConfig != nil {
		def.Config = agentConfig
	}

	// Set permissions if present
	permissions := decodeJSONAs[map[string]bridgepkg.ToolPermission](agent.Permissions)
	if permissions != nil && len(*permissions) > 0 {
		def.Permissions = permissions
	}

	// Set tools if present
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
	// Add Hindsight memory MCP server (if configured and agent has a team)
	if p.hindsightMCPURL != nil && agent.Team != "" {
		hsMCP := buildHindsightMCPServer(p.hindsightMCPURL(agent.ID))
		if mcpServers == nil {
			servers := []bridgepkg.McpServerDefinition{hsMCP}
			mcpServers = &servers
		} else {
			*mcpServers = append(*mcpServers, hsMCP)
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

	// Set subagents if present
	subagents := decodeJSONAs[[]bridgepkg.AgentDefinition](agent.Subagents)
	if subagents != nil && len(*subagents) > 0 {
		def.Subagents = subagents
	}

	return def
}

func buildHindsightMCPServer(mcpURL string) bridgepkg.McpServerDefinition {
	var transport bridgepkg.McpTransport
	httpTransport := bridgepkg.McpTransport1{
		Type: bridgepkg.StreamableHttp,
		Url:  mcpURL,
	}
	transport.FromMcpTransport1(httpTransport)

	return bridgepkg.McpServerDefinition{
		Name:      "memory",
		Transport: transport,
	}
}

func buildZiraLoopMCPServer(mcpBaseURL, jti, token string) bridgepkg.McpServerDefinition {
	// Our MCP server uses the JTI as the path and the proxy token for auth
	url := fmt.Sprintf("%s/%s", mcpBaseURL, jti)

	var transport bridgepkg.McpTransport
	httpTransport := bridgepkg.McpTransport1{
		Type: bridgepkg.StreamableHttp,
		Url:  url,
	}
	transport.FromMcpTransport1(httpTransport)

	return bridgepkg.McpServerDefinition{
		Name:      "ziraloop",
		Transport: transport,
	}
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
