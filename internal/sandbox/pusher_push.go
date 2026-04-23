package sandbox

import (
	"context"
	"fmt"
	"log/slog"

	bridgepkg "github.com/usehiveloop/hiveloop/internal/bridge"
	"github.com/usehiveloop/hiveloop/internal/model"
	subagents "github.com/usehiveloop/hiveloop/internal/sub-agents"
	"time"
)

func (p *Pusher) pushAgentToSandbox(ctx context.Context, agent *model.Agent, sb *model.Sandbox) error {
	if agent.CredentialID == nil || agent.OrgID == nil {
		return fmt.Errorf("cannot push agent without credential and org")
	}

	var cred model.Credential
	if err := p.db.Where("id = ?", *agent.CredentialID).First(&cred).Error; err != nil {
		return fmt.Errorf("loading credential: %w", err)
	}

	proxyToken, jti, err := p.mintAgentToken(agent, &cred)
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

	def := p.buildAgentDefinition(agent, &cred, proxyToken, jti)

	subagentDefs, err := p.buildSubagentDefinitions(agent, &cred)
	if err != nil {
		return fmt.Errorf("building subagent definitions: %w", err)
	}
	if len(subagentDefs) > 0 {
		def.Subagents = &subagentDefs
	}

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

func (p *Pusher) buildAgentDefinition(agent *model.Agent, cred *model.Credential, proxyToken, jti string) bridgepkg.AgentDefinition {
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
		Provider: bridgepkg.ProviderConfig{
			ProviderType: providerType,
			Model:        agent.Model,
			ApiKey:       proxyToken,
			BaseUrl:      &proxyBaseURL,
		},
	}

	permissions := decodeJSONAs[map[string]bridgepkg.ToolPermission](agent.Permissions)

	def.Config = applyAgentConfigDefaults(decodeJSONAs[bridgepkg.AgentConfig](agent.AgentConfig), cred.ProviderID, agent.Model)
	applyImmortalDefault(def.Config, def.Provider, cred.ProviderID, agent.Model)
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

	tools := decodeJSONAs[[]bridgepkg.ToolDefinition](agent.Tools)
	if tools != nil && len(*tools) > 0 {
		def.Tools = tools
	}

	mcpServers := decodeJSONAs[[]bridgepkg.McpServerDefinition](agent.McpServers)

	hasIntegrations := agent.Integrations != nil && len(agent.Integrations) > 0
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

	if bridgeSkills := p.loadBridgeSkills(agent.ID); len(bridgeSkills) > 0 {
		def.Skills = &bridgeSkills
	}

	return def
}
