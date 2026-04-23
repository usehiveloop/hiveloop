package sandbox

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"gorm.io/gorm"

	bridgepkg "github.com/usehiveloop/hiveloop/internal/bridge"
	"github.com/usehiveloop/hiveloop/internal/config"
	"github.com/usehiveloop/hiveloop/internal/model"
)

var providerTypeMap = map[string]bridgepkg.ProviderType{
	"openai":       bridgepkg.OpenAi,
	"anthropic":    bridgepkg.Anthropic,
	"google":       bridgepkg.Google,
	"groq":         bridgepkg.Groq,
	"fireworks":    bridgepkg.Fireworks,
	"openrouter":   bridgepkg.OpenAi,
	"moonshotai":   bridgepkg.OpenAi,
	"zai":          bridgepkg.OpenAi,
	"zhipuai":      bridgepkg.OpenAi,
	"fireworks-ai": bridgepkg.Fireworks,
	"ollama":       bridgepkg.Ollama,
}

const (
	agentTokenTTL       = 24 * time.Hour
	tokenRotationWindow = 3 * time.Hour
)

type Pusher struct {
	db           *gorm.DB
	orchestrator *Orchestrator
	signingKey   []byte
	cfg          *config.Config
	pushed       sync.Map
}

func NewPusher(db *gorm.DB, orchestrator *Orchestrator, signingKey []byte, cfg *config.Config) *Pusher {
	return &Pusher{
		db:           db,
		orchestrator: orchestrator,
		signingKey:   signingKey,
		cfg:          cfg,
	}
}

func (p *Pusher) isPushed(sandboxID, agentID string) bool {
	_, ok := p.pushed.Load(sandboxID + ":" + agentID)
	return ok
}

func (p *Pusher) markPushed(sandboxID, agentID string) {
	p.pushed.Store(sandboxID+":"+agentID, true)
}

func (p *Pusher) PushAgent(ctx context.Context, agent *model.Agent) error {
	if agent.IsSystem {
		return nil
	}
	if agent.SandboxType != "shared" {
		return nil
	}

	sb, err := p.orchestrator.AssignPoolSandbox(ctx, agent)
	if err != nil {
		return fmt.Errorf("assigning pool sandbox: %w", err)
	}

	return p.pushAgentToSandbox(ctx, agent, sb)
}

func (p *Pusher) PushAgentToSandbox(ctx context.Context, agent *model.Agent, sb *model.Sandbox) error {
	if agent.IsSystem {
		return nil
	}

	sandboxID := sb.ID.String()
	agentID := agent.ID.String()

	if p.isPushed(sandboxID, agentID) {
		return nil
	}

	client, err := p.orchestrator.GetBridgeClient(ctx, sb)
	if err == nil {
		if exists, checkErr := client.HasAgent(ctx, agentID); checkErr == nil && exists {
			p.markPushed(sandboxID, agentID)
			return nil
		}
	}

	if err := p.pushAgentToSandbox(ctx, agent, sb); err != nil {
		return err
	}
	p.markPushed(sandboxID, agentID)
	return nil
}

func (p *Pusher) RemoveAgent(ctx context.Context, agent *model.Agent) error {
	if agent.SandboxType != "shared" {
		return nil
	}

	if agent.SandboxID == nil {
		return nil
	}

	var sb model.Sandbox
	if err := p.db.Where("id = ? AND status = 'running'", *agent.SandboxID).First(&sb).Error; err != nil {
		_ = p.orchestrator.ReleasePoolSandbox(ctx, agent)
		return nil
	}

	client, err := p.orchestrator.GetBridgeClient(ctx, &sb)
	if err != nil {
		slog.Warn("failed to get bridge client for agent removal", "agent_id", agent.ID, "sandbox_id", sb.ID, "error", err)
	} else {
		if err := client.RemoveAgentDefinition(ctx, agent.ID.String()); err != nil {
			slog.Warn("failed to remove agent from bridge", "agent_id", agent.ID, "sandbox_id", sb.ID, "error", err)
		}
	}

	p.pushed.Delete(sb.ID.String() + ":" + agent.ID.String())

	return p.orchestrator.ReleasePoolSandbox(ctx, agent)
}
