package sandbox

import (
	"context"

	bridgepkg "github.com/usehiveloop/hiveloop/internal/bridge"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// TODO(wave-2): The new ACP-harness AgentDefinition no longer carries a
// nested `subagents` field — the harness adapter is the one that resolves
// subagent IDs into runnable agents at session start. Wave 2 reworks the
// subagent pipeline so that the parent agent's harness pulls subagent
// definitions on-demand (or via a separate registration call), rather than
// us precomputing and embedding them here. For Wave 1 this returns an empty
// slice so pushAgentToSandbox compiles and the bridge gets the parent agent
// only.
func (p *Pusher) buildSubagentDefinitions(_ context.Context, _ *model.Agent, _ *model.Credential) ([]bridgepkg.AgentDefinition, error) {
	return nil, nil
}
