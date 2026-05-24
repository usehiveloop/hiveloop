package sandbox

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	bridgepkg "github.com/usehivy/hivy/internal/bridge"
	"github.com/usehivy/hivy/internal/credentials"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
)

func (p *Pusher) pushSpecialistToSandbox(ctx context.Context, agent *model.Employee, sb *model.Sandbox) error {
	if agent.OrgID == nil {
		return fmt.Errorf("cannot push agent without org")
	}

	cred, err := credentials.Resolve(ctx, p.db, p.picker, agent)
	if err != nil {
		return fmt.Errorf("resolving agent credential: %w", err)
	}

	proxyToken, jti, err := p.mintEmployeeProxyToken(agent, cred)
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
		Meta:         model.JSON{"employee_id": agent.ID.String(), "type": "employee_proxy"},
	}
	if err := p.db.Create(&dbToken).Error; err != nil {
		return fmt.Errorf("storing proxy token: %w", err)
	}

	def := p.buildSpecialistDefinition(ctx, agent, owningEmployee, cred, proxyToken, jti)

	client, err := p.orchestrator.GetBridgeClient(ctx, sb)
	if err != nil {
		return fmt.Errorf("getting bridge client: %w", err)
	}

	if err := client.UpsertAgent(ctx, agent.ID.String(), def); err != nil {
		return fmt.Errorf("pushing agent to bridge: %w", err)
	}

	logging.FromContext(ctx).DebugContext(ctx, "agent pushed to bridge", "employee_id", agent.ID, "sandbox_id", sb.ID)

	return nil
}

func (p *Pusher) buildSpecialistDefinition(ctx context.Context, agent *model.Employee, owningEmployee *model.Employee, cred *model.Credential, proxyToken, jti string) bridgepkg.AgentDefinition {
	providerType := bridgepkg.Custom
	if pt, ok := providerTypeMap[cred.ProviderID]; ok {
		providerType = pt
	}

	proxyBaseURL := p.cfg.ProxyOriginURL()

	systemPrompt := agent.SystemPrompt
	modelName := agent.Model
	if route, ok := registry.Global().ResolveModel(cred.ProviderID, agent.Model); ok {
		modelName = route.UpstreamID
	}

	layout := p.orchestrator.runtimeLayout()
	if repoContext := buildRepoContext(mergeAgentResourcesForContext(agent, owningEmployee), layout.AgentRepoDir); repoContext != "" {
		systemPrompt += "\n\n" + repoContext
	}
	if owningEmployee != nil {
		selectedRepos, err := loadSelectedGitHubRepositoriesForAgent(ctx, p.db, owningEmployee.ID)
		if err != nil {
			logging.Capture(ctx, fmt.Errorf("load employee selected GitHub repositories for agent definition: %w", err))
		} else if selectedRepoContext := buildSelectedGitHubRepoContext(selectedRepos, layout.EmployeeRepoDir); selectedRepoContext != "" {
			systemPrompt += "\n\n" + selectedRepoContext
		}
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

	authorCfg := decodeJSONAs[bridgepkg.AgentConfig](agent.RuntimeConfig)
	def.Config = applyAgentConfigDefaults(authorCfg, cred.ProviderID, modelName)
	applyHarnessOptionalFields(def.Config, authorCfg)

	mcpServers := decodeJSONAs[[]bridgepkg.McpServerDefinition](agent.McpServers)

	effectiveIntegrations := mergeAgentIntegrationsForAccess(agent, owningEmployee)
	hasIntegrations := len(effectiveIntegrations) > 0
	if hasIntegrations && p.cfg.MCPBaseURL != "" && jti != "" {
		ourMCP := buildHivyMCPServer(p.cfg.MCPBaseURL, jti, proxyToken)
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
