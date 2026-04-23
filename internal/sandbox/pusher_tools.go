package sandbox

import bridgepkg "github.com/usehiveloop/hiveloop/internal/bridge"

const defaultRequirementCadenceTurns = 10

func applyToolRequirementsDefault(
	cfg *bridgepkg.AgentConfig,
	permissions *map[string]bridgepkg.ToolPermission,
) {
	if cfg == nil || cfg.ToolRequirements != nil {
		return
	}

	disabled := make(map[string]bool)
	if cfg.DisabledTools != nil {
		for _, tool := range *cfg.DisabledTools {
			disabled[tool] = true
		}
	}
	if permissions != nil {
		for tool, perm := range *permissions {
			if perm == bridgepkg.ToolPermissionDeny {
				disabled[tool] = true
			}
		}
	}

	turnStart := bridgepkg.TurnStart
	warn := bridgepkg.Warn
	candidates := []struct {
		tool     string
		position *bridgepkg.RequirementPosition
	}{
		{"memory_recall", &turnStart},
		{"memory_retain", nil},
		{"journal_write", nil},
	}

	var reqs []bridgepkg.ToolRequirement
	for _, candidate := range candidates {
		if disabled[candidate.tool] {
			continue
		}
		reqs = append(reqs, bridgepkg.ToolRequirement{
			Tool:        candidate.tool,
			Cadence:     newEveryNTurnsCadence(defaultRequirementCadenceTurns),
			Position:    candidate.position,
			Enforcement: &warn,
		})
	}
	if len(reqs) > 0 {
		cfg.ToolRequirements = &reqs
	}
}

func newEveryNTurnsCadence(n int32) *bridgepkg.RequirementCadence {
	var cadence bridgepkg.RequirementCadence
	_ = cadence.FromRequirementCadence2(bridgepkg.RequirementCadence2{
		Type: bridgepkg.EveryNTurns,
		N:    n,
	})
	return &cadence
}
