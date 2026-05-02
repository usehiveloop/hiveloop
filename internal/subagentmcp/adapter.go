package subagentmcp

import (
	"context"
	"io"

	"github.com/google/uuid"

	bridgepkg "github.com/usehiveloop/hiveloop/internal/bridge"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/sandbox"
)

// OrchestratorAdapter wraps a concrete *sandbox.Orchestrator so it satisfies
// the local Orchestrator interface (which keeps internal/subagentmcp testable
// without dragging in the full orchestrator graph).
type OrchestratorAdapter struct {
	Inner *sandbox.Orchestrator
}

// NewOrchestratorAdapter returns an Orchestrator-shaped wrapper.
func NewOrchestratorAdapter(o *sandbox.Orchestrator) *OrchestratorAdapter {
	return &OrchestratorAdapter{Inner: o}
}

func (a *OrchestratorAdapter) EnsureSubagentSandbox(ctx context.Context, orgID, parentAgentID, subagentID uuid.UUID) (*model.Sandbox, error) {
	return a.Inner.EnsureSubagentSandbox(ctx, orgID, parentAgentID, subagentID)
}

func (a *OrchestratorAdapter) GetBridgeClient(ctx context.Context, sb *model.Sandbox) (BridgeClient, error) {
	c, err := a.Inner.GetBridgeClient(ctx, sb)
	if err != nil {
		return nil, err
	}
	return &bridgeClientAdapter{inner: c}, nil
}

type bridgeClientAdapter struct {
	inner *bridgepkg.BridgeClient
}

func (b *bridgeClientAdapter) CreateConversation(ctx context.Context, agentID string) (*CreateConversationResponse, error) {
	resp, err := b.inner.CreateConversation(ctx, agentID)
	if err != nil {
		return nil, err
	}
	return &CreateConversationResponse{ConversationID: resp.ConversationId}, nil
}

func (b *bridgeClientAdapter) SendMessage(ctx context.Context, convID, content string) error {
	return b.inner.SendMessage(ctx, convID, content)
}

func (b *bridgeClientAdapter) SSEStream(ctx context.Context, convID string) (io.ReadCloser, error) {
	return b.inner.SSEStream(ctx, convID)
}

// PusherAdapter wraps *sandbox.Pusher into the local Pusher interface.
type PusherAdapter struct {
	Inner *sandbox.Pusher
}

func NewPusherAdapter(p *sandbox.Pusher) *PusherAdapter {
	return &PusherAdapter{Inner: p}
}

func (a *PusherAdapter) PushAgentToSandbox(ctx context.Context, agent *model.Agent, sb *model.Sandbox) error {
	return a.Inner.PushAgentToSandbox(ctx, agent, sb)
}
