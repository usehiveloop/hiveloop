package sandbox

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

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

	owningEmployee, err := p.loadOwningEmployee(ctx, agent)
	if err != nil {
		return fmt.Errorf("loading owning employee: %w", err)
	}

	scopes := buildScopesFromIntegrations(mergeAgentIntegrationsForAccess(agent, owningEmployee))
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

	def := p.buildAgentDefinition(ctx, agent, owningEmployee, cred, proxyToken, jti)

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

func (p *Pusher) buildAgentDefinition(ctx context.Context, agent *model.Agent, owningEmployee *model.Agent, cred *model.Credential, proxyToken, jti string) bridgepkg.AgentDefinition {
	providerType := bridgepkg.Custom
	if pt, ok := providerTypeMap[cred.ProviderID]; ok {
		providerType = pt
	}

	proxyBaseURL := fmt.Sprintf("https://%s", p.cfg.ProxyHost)

	providerGroup := subagents.MapProviderToGroup(cred.ProviderID, agent.Model)
	systemPrompt, modelName := agent.ResolveProviderConfig(providerGroup)

	if repoContext := buildRepoContext(mergeAgentResourcesForContext(agent, owningEmployee)); repoContext != "" {
		systemPrompt += "\n\n" + repoContext
	}

	def := bridgepkg.AgentDefinition{
		Id:           agent.ID.String(),
		Name:         agent.Name,
		Description:  agent.Description,
		SystemPrompt: systemPrompt,
		Harness:      harnessFromAgent(agent.Harness),
		Provider: bridgepkg.ProviderConfig{
			ProviderType: providerType,
			Model:        modelName,
			ApiKey:       proxyToken,
			BaseUrl:      &proxyBaseURL,
		},
	}

	authorCfg := decodeJSONAs[bridgepkg.AgentConfig](agent.AgentConfig)
	def.Config = applyAgentConfigDefaults(authorCfg, cred.ProviderID, modelName)
	applyHarnessOptionalFields(def.Config, authorCfg)

	mcpServers := decodeJSONAs[[]bridgepkg.McpServerDefinition](agent.McpServers)

	effectiveIntegrations := mergeAgentIntegrationsForAccess(agent, owningEmployee)
	hasIntegrations := len(effectiveIntegrations) > 0
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

	var inheritedSkillAgentIDs []uuid.UUID
	if owningEmployee != nil {
		inheritedSkillAgentIDs = append(inheritedSkillAgentIDs, owningEmployee.ID)
	}
	if bridgeSkills := p.loadBridgeSkills(ctx, agent.ID, inheritedSkillAgentIDs...); len(bridgeSkills) > 0 {
		def.Skills = &bridgeSkills
	}

	return def
}
