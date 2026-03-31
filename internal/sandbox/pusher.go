package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"

	bridgepkg "github.com/llmvault/llmvault/internal/bridge"
	"github.com/llmvault/llmvault/internal/config"
	"github.com/llmvault/llmvault/internal/model"
	"github.com/llmvault/llmvault/internal/token"
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

const agentTokenTTL = 24 * time.Hour

// Pusher constructs Bridge AgentDefinitions from our Agent model
// and pushes them to Bridge instances running in sandboxes.
type Pusher struct {
	db           *gorm.DB
	orchestrator *Orchestrator
	signingKey   []byte // JWT signing key for minting proxy tokens
	cfg          *config.Config
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

// PushAgent ensures the sandbox is running and pushes the agent definition to Bridge.
// For shared agents only — called on agent create/update.
func (p *Pusher) PushAgent(ctx context.Context, agent *model.Agent) error {
	if agent.SandboxType != "shared" {
		return nil // dedicated agents are pushed lazily on conversation create
	}

	// Load associations we need
	var org model.Org
	if err := p.db.Where("id = ?", agent.OrgID).First(&org).Error; err != nil {
		return fmt.Errorf("loading org: %w", err)
	}
	var identity model.Identity
	if err := p.db.Where("id = ?", agent.IdentityID).First(&identity).Error; err != nil {
		return fmt.Errorf("loading identity: %w", err)
	}

	// Ensure shared sandbox is running
	sb, err := p.orchestrator.EnsureSharedSandbox(ctx, &org, &identity)
	if err != nil {
		return fmt.Errorf("ensuring shared sandbox: %w", err)
	}

	// Build and push
	return p.pushAgentToSandbox(ctx, agent, sb)
}

// PushAgentToSandbox pushes an agent definition to a specific sandbox.
// Used by both shared (via PushAgent) and dedicated (via conversation create in Phase 7).
func (p *Pusher) PushAgentToSandbox(ctx context.Context, agent *model.Agent, sb *model.Sandbox) error {
	return p.pushAgentToSandbox(ctx, agent, sb)
}

// RemoveAgent removes an agent from Bridge.
// For shared agents only — dedicated sandboxes are deleted entirely.
func (p *Pusher) RemoveAgent(ctx context.Context, agent *model.Agent) error {
	if agent.SandboxType != "shared" {
		return nil
	}

	// Find the shared sandbox for this identity
	var sb model.Sandbox
	if err := p.db.Where("identity_id = ? AND sandbox_type = 'shared' AND status = 'running'",
		agent.IdentityID).First(&sb).Error; err != nil {
		return nil // no running sandbox — nothing to remove from
	}

	client, err := p.orchestrator.GetBridgeClient(ctx, &sb)
	if err != nil {
		return fmt.Errorf("getting bridge client: %w", err)
	}

	if err := client.RemoveAgentDefinition(ctx, agent.ID.String()); err != nil {
		slog.Warn("failed to remove agent from bridge", "agent_id", agent.ID, "sandbox_id", sb.ID, "error", err)
		// Non-fatal — agent is deleted from our DB regardless
	}
	return nil
}

func (p *Pusher) pushAgentToSandbox(ctx context.Context, agent *model.Agent, sb *model.Sandbox) error {
	// Load credential for provider info
	var cred model.Credential
	if err := p.db.Where("id = ?", agent.CredentialID).First(&cred).Error; err != nil {
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
		OrgID:        agent.OrgID,
		CredentialID: cred.ID,
		JTI:          jti,
		ExpiresAt:    expiresAt,
		Meta:         model.JSON{"agent_id": agent.ID.String(), "type": "agent_proxy"},
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

func (p *Pusher) mintAgentToken(agent *model.Agent, cred *model.Credential) (tokenStr, jti string, err error) {
	tokenStr, jti, err = token.Mint(
		p.signingKey,
		agent.OrgID.String(),
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
		ourMCP := buildLLMVaultMCPServer(p.cfg.MCPBaseURL, jti, proxyToken)
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

	// Set subagents if present
	subagents := decodeJSONAs[[]bridgepkg.AgentDefinition](agent.Subagents)
	if subagents != nil && len(*subagents) > 0 {
		def.Subagents = subagents
	}

	return def
}

func buildLLMVaultMCPServer(mcpBaseURL, jti, token string) bridgepkg.McpServerDefinition {
	// Our MCP server uses the JTI as the path and the proxy token for auth
	url := fmt.Sprintf("%s/%s", mcpBaseURL, jti)

	var transport bridgepkg.McpTransport
	httpTransport := bridgepkg.McpTransport1{
		Type: bridgepkg.StreamableHttp,
		Url:  url,
	}
	transport.FromMcpTransport1(httpTransport)

	return bridgepkg.McpServerDefinition{
		Name:      "llmvault",
		Transport: transport,
	}
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
