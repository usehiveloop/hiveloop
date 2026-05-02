package sandbox

import (
	"context"
	"fmt"
	"time"

	bridgepkg "github.com/usehiveloop/hiveloop/internal/bridge"
	"github.com/usehiveloop/hiveloop/internal/credentials"
	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/model"
	subagents "github.com/usehiveloop/hiveloop/internal/sub-agents"
)

func (p *Pusher) pushAgentToSandbox(ctx context.Context, agent *model.Agent, sb *model.Sandbox) error {
	if agent.OrgID == nil {
		return fmt.Errorf("cannot push agent without org")
	}

	cred, err := credentials.Resolve(ctx, p.db, p.picker, agent)
	if err != nil {
		return fmt.Errorf("resolving agent credential: %w", err)
	}

	proxyToken, jti, err := p.mintAgentToken(agent, cred)
	if err != nil {
		return fmt.Errorf("minting proxy token: %w", err)
	}

	scopes := buildScopesFromIntegrations(agent.Integrations)
	var scopesJSON model.JSON
	if len(scopes) > 0 {
		scopesJSON = model.JSON{"scopes": scopes}
	}

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

	def := p.buildAgentDefinition(ctx, agent, cred, proxyToken, jti)

	// TODO(wave-2): The old bridge AgentDefinition had a `subagents` field that
	// embedded full child AgentDefinitions on the parent push. The new
	// ACP-harness OpenAPI removed it — subagent resolution now lives in the
	// harness adapter. We still call buildSubagentDefinitions to keep the
	// dependency graph wired (Wave 1 returns nil); Wave 2 either deletes this
	// call or replaces it with a separate /push/subagents registration step.
	if _, err := p.buildSubagentDefinitions(ctx, agent, cred); err != nil {
		return fmt.Errorf("building subagent definitions: %w", err)
	}

	client, err := p.orchestrator.GetBridgeClient(ctx, sb)
	if err != nil {
		return fmt.Errorf("getting bridge client: %w", err)
	}

	if err := client.UpsertAgent(ctx, agent.ID.String(), def); err != nil {
		return fmt.Errorf("pushing agent to bridge: %w", err)
	}

	logging.FromContext(ctx).DebugContext(ctx, "agent pushed to bridge", "agent_id", agent.ID, "sandbox_id", sb.ID)

	return nil
}

func (p *Pusher) buildAgentDefinition(ctx context.Context, agent *model.Agent, cred *model.Credential, proxyToken, jti string) bridgepkg.AgentDefinition {
	providerType := bridgepkg.Custom
	if pt, ok := providerTypeMap[cred.ProviderID]; ok {
		providerType = pt
	}

	proxyBaseURL := fmt.Sprintf("https://%s", p.cfg.ProxyHost)

	providerGroup := subagents.MapProviderToGroup(cred.ProviderID, agent.Model)
	systemPrompt, _ := agent.ResolveProviderConfig(providerGroup)

	if repoContext := buildRepoContext(agent.Resources); repoContext != "" {
		systemPrompt += "\n\n" + repoContext
	}

	def := bridgepkg.AgentDefinition{
		Id:           agent.ID.String(),
		Name:         agent.Name,
		Description:  agent.Description,
		SystemPrompt: systemPrompt,
		// TODO(wave-2): replace with deterministic harness(provider, model)
		// selection — for now every agent is forced onto the Claude Code
		// harness so the build goes through. OpenCode-targeted agents will
		// silently run on Claude Code until Wave 2 lands.
		Harness: bridgepkg.Claude,
		Provider: bridgepkg.ProviderConfig{
			ProviderType: providerType,
			Model:        agent.Model,
			ApiKey:       proxyToken,
			BaseUrl:      &proxyBaseURL,
		},
	}

	permissions := decodeJSONAs[map[string]bridgepkg.ToolPermission](agent.Permissions)

	def.Config = applyAgentConfigDefaults(decodeJSONAs[bridgepkg.AgentConfig](agent.AgentConfig), cred.ProviderID, agent.Model)
	applyImmortalDefault(def.Config, def.Provider, cred.ProviderID, agent.Model, permissions)
	applyHistoryStripDefault(def.Config)
	applyToolRequirementsDefault(def.Config, permissions)

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

	// TODO(wave-2): The new ACP-harness AgentDefinition removed the
	// agent-defined `tools` slice — tools are now exclusively driven by the
	// harness's built-in tool registry plus MCP servers. The legacy
	// agent.Tools JSONB column still exists in the DB; Wave 2 either
	// migrates surviving entries into MCP servers or drops the column.
	_ = decodeJSONAs[[]any](agent.Tools)

	mcpServers := decodeJSONAs[[]bridgepkg.McpServerDefinition](agent.McpServers)

	hasIntegrations := len(agent.Integrations) > 0
	if hasIntegrations && p.cfg.MCPBaseURL != "" && jti != "" {
		ourMCP := buildHiveLoopMCPServer(p.cfg.MCPBaseURL, jti, proxyToken)
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

	if bridgeSkills := p.loadBridgeSkills(ctx, agent.ID); len(bridgeSkills) > 0 {
		def.Skills = &bridgeSkills
	}

	return def
}
