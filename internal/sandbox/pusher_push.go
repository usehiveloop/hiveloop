package sandbox

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

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

	// Persist the resolved harness on the Agent row so subsequent pushes
	// short-circuit the (provider, model) computation. We only stamp on
	// the first push (Harness column empty) and we ignore the not-found
	// case (e.g. unit tests using an in-memory agent that wasn't persisted).
	if agent.Harness == "" {
		harnessStr := string(def.Harness)
		if err := p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return tx.Model(&model.Agent{}).
				Where("id = ? AND (harness IS NULL OR harness = '')", agent.ID).
				Update("harness", harnessStr).Error
		}); err != nil {
			logging.FromContext(ctx).WarnContext(ctx, "failed to stamp agent harness", "agent_id", agent.ID, "err", err)
		} else {
			agent.Harness = harnessStr
		}
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
		Harness:      resolveHarness(agent.Harness, providerType, agent.Model),
		Provider: bridgepkg.ProviderConfig{
			ProviderType: providerType,
			Model:        agent.Model,
			ApiKey:       proxyToken,
			BaseUrl:      &proxyBaseURL,
		},
	}

	permissions := decodeJSONAs[map[string]bridgepkg.ToolPermission](agent.Permissions)

	authorCfg := decodeJSONAs[bridgepkg.AgentConfig](agent.AgentConfig)
	def.Config = applyAgentConfigDefaults(authorCfg, cred.ProviderID, agent.Model)
	applyHarnessOptionalFields(def.Config, authorCfg)

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
			// Permissions-derived denylist wins over author-supplied
			// disabled_tools; merge the two so we don't silently drop
			// either source.
			if def.Config.DisabledTools != nil {
				existing := *def.Config.DisabledTools
				seen := make(map[string]struct{}, len(existing)+len(disabledTools))
				for _, t := range existing {
					seen[t] = struct{}{}
				}
				for _, t := range disabledTools {
					if _, ok := seen[t]; !ok {
						existing = append(existing, t)
						seen[t] = struct{}{}
					}
				}
				def.Config.DisabledTools = &existing
			} else {
				def.Config.DisabledTools = &disabledTools
			}
		}
	}

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
