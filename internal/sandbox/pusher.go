package sandbox

import (
	"context"
	"sync"
	"time"

	"gorm.io/gorm"

	bridgepkg "github.com/usehiveloop/hiveloop/internal/bridge"
	"github.com/usehiveloop/hiveloop/internal/config"
	"github.com/usehiveloop/hiveloop/internal/credentials"
	"github.com/usehiveloop/hiveloop/internal/model"
)

var providerTypeMap = map[string]bridgepkg.ProviderType{
	"openai":       bridgepkg.ProviderTypeOpenAi,
	"anthropic":    bridgepkg.ProviderTypeAnthropic,
	"google":       bridgepkg.ProviderTypeGoogle,
	"groq":         bridgepkg.ProviderTypeGroq,
	"fireworks":    bridgepkg.ProviderTypeFireworks,
	"openrouter":   bridgepkg.ProviderTypeOpenAi,
	"moonshotai":   bridgepkg.ProviderTypeOpenAi,
	"zai":          bridgepkg.ProviderTypeOpenAi,
	"zhipuai":      bridgepkg.ProviderTypeOpenAi,
	"fireworks-ai": bridgepkg.ProviderTypeFireworks,
	"ollama":       bridgepkg.ProviderTypeOllama,
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
	// picker resolves a system credential for any agent whose credential_id
	// is nil (platform-keys mode). Only used via credentials.Resolve, never
	// called directly.
	picker credentials.Picker
	pushed sync.Map
}

func NewPusher(db *gorm.DB, orchestrator *Orchestrator, signingKey []byte, cfg *config.Config, picker credentials.Picker) *Pusher {
	return &Pusher{
		db:           db,
		orchestrator: orchestrator,
		signingKey:   signingKey,
		cfg:          cfg,
		picker:       picker,
	}
}

func (p *Pusher) isPushed(sandboxID, agentID string) bool {
	_, ok := p.pushed.Load(sandboxID + ":" + agentID)
	return ok
}

func (p *Pusher) markPushed(sandboxID, agentID string) {
	p.pushed.Store(sandboxID+":"+agentID, true)
}

func (p *Pusher) PushAgentToSandbox(ctx context.Context, agent *model.Agent, sb *model.Sandbox) error {
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
