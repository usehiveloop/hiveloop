package sandbox

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	bridgepkg "github.com/usehiveloop/hiveloop/internal/bridge"
	"github.com/usehiveloop/hiveloop/internal/model"
)

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
			ApiKey:       "",
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
