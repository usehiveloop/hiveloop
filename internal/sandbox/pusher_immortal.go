package sandbox

import (
	bridgepkg "github.com/usehiveloop/hiveloop/internal/bridge"
	"github.com/usehiveloop/hiveloop/internal/registry"
)

const (
	immortalTokenBudgetFraction = 0.50
	fallbackParentContextWindow = 128_000
	immortalRetentionWindow     = 20
	immortalEvictionWindow      = 1.0
)

func applyImmortalDefault(
	cfg *bridgepkg.AgentConfig,
	_ bridgepkg.ProviderConfig,
	providerID, primaryModel string,
	permissions *map[string]bridgepkg.ToolPermission,
) {
	if cfg == nil || cfg.Immortal != nil {
		return
	}

	tokenBudget := int32(float64(parentContextWindow(providerID, primaryModel)) * immortalTokenBudgetFraction)
	retention := int32(immortalRetentionWindow)
	eviction := float64(immortalEvictionWindow)
	expose := journalToolsAllowed(cfg, permissions)

	imm := bridgepkg.ImmortalConfig{
		TokenBudget:        &tokenBudget,
		RetentionWindow:    &retention,
		EvictionWindow:     &eviction,
		ExposeJournalTools: &expose,
	}

	var wrapped bridgepkg.AgentConfig_Immortal
	if err := wrapped.FromImmortalConfig(imm); err != nil {
		return
	}
	cfg.Immortal = &wrapped
}

// journalToolsAllowed returns true when the agent has any non-Deny permission
// for journal_read or journal_write (and they're not in disabled_tools). Mirrors
// applyToolRequirementsDefault's allow/deny sourcing.
func journalToolsAllowed(cfg *bridgepkg.AgentConfig, permissions *map[string]bridgepkg.ToolPermission) bool {
	disabled := map[string]bool{}
	if cfg != nil && cfg.DisabledTools != nil {
		for _, t := range *cfg.DisabledTools {
			disabled[t] = true
		}
	}
	if permissions != nil {
		for tool, perm := range *permissions {
			if perm == bridgepkg.ToolPermissionDeny {
				disabled[tool] = true
			}
		}
	}
	for _, t := range []string{"journal_read", "journal_write"} {
		if !disabled[t] {
			return true
		}
	}
	return false
}

func parentContextWindow(providerID, modelID string) int64 {
	if prov, ok := registry.Global().GetProvider(providerID); ok {
		if m, ok := prov.Models[modelID]; ok && m.Limit != nil && m.Limit.Context > 0 {
			return m.Limit.Context
		}
	}
	return fallbackParentContextWindow
}
