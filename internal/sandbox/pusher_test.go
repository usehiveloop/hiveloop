package sandbox

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/ziraloop/ziraloop/internal/model"
)

// TestPushAgent_NoOpForSystemAgent verifies that PushAgent returns nil
// immediately for is_system=true agents, without touching the orchestrator,
// provider, or DB. System agents live in the singleton system sandbox which
// is provisioned at worker startup and refreshed by the periodic
// SystemAgentSync task.
//
// We construct the Pusher with all nil dependencies — if the IsSystem
// shortcut isn't the very first thing in PushAgent, this test will panic
// with a nil pointer dereference, catching the regression immediately.
func TestPushAgent_NoOpForSystemAgent(t *testing.T) {
	pusher := &Pusher{} // all deps nil — only IsSystem branch is safe to hit

	agent := &model.Agent{
		ID:          uuid.New(),
		Name:        "forge-architect-anthropic",
		IsSystem:    true,
		SandboxType: "shared",
	}

	if err := pusher.PushAgent(context.Background(), agent); err != nil {
		t.Errorf("PushAgent for system agent: got %v, want nil", err)
	}
}

// TestPushAgentToSandbox_NoOpForSystemAgent — same contract for the
// per-sandbox push entrypoint. The handler call sites in
// system_conversations.go / conversations.go / forge/controller.go all hit
// this method even when the agent is system, so the no-op must apply here
// too or we'd be doing a Bridge round-trip on every system-agent request
// (defeating the periodic-sync strategy).
func TestPushAgentToSandbox_NoOpForSystemAgent(t *testing.T) {
	pusher := &Pusher{}

	agent := &model.Agent{
		ID:          uuid.New(),
		Name:        "forge-judge-openai",
		IsSystem:    true,
		SandboxType: "shared",
	}
	sb := &model.Sandbox{
		ID:          uuid.New(),
		SandboxType: "system",
	}

	if err := pusher.PushAgentToSandbox(context.Background(), agent, sb); err != nil {
		t.Errorf("PushAgentToSandbox for system agent: got %v, want nil", err)
	}
}
