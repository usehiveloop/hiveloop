package sandbox

import (
	bridgepkg "github.com/usehiveloop/hiveloop/internal/bridge"
)

// TODO(wave-2): The Immortal config + journal-tools logic was tied to the
// old bridge's AgentConfig.Immortal field, which the new ACP-harness OpenAPI
// dropped. The whole feature (token-budget retention, journal_read/journal_write
// gating, parent-context-window math) needs to be reimplemented on top of the
// new harness-driven config — most likely as a server-side preprocessor that
// owns its own state instead of riding on the bridge agent definition. Until
// then, this is a no-op so the rest of the pusher pipeline keeps compiling.
// Wave 2 will delete this file or rebuild the feature.
func applyImmortalDefault(
	_ *bridgepkg.AgentConfig,
	_ bridgepkg.ProviderConfig,
	_, _ string,
	_ *map[string]bridgepkg.ToolPermission,
) {
}
