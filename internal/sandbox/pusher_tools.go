package sandbox

import bridgepkg "github.com/usehiveloop/hiveloop/internal/bridge"

// TODO(wave-2): The tool-requirements feature (memory_recall / memory_retain /
// journal_write cadence enforcement) was tied to the old bridge's
// AgentConfig.ToolRequirements field, which the new ACP-harness OpenAPI no
// longer carries. The whole memory/journal loop needs to be reimplemented on
// top of the new harness model — likely as an integration MCP tool plus a
// bridge-side enforcement hook. Until then, this is a no-op so the rest of
// the pusher pipeline keeps compiling. Wave 2 will delete this file or
// rebuild the feature.
func applyToolRequirementsDefault(
	_ *bridgepkg.AgentConfig,
	_ *map[string]bridgepkg.ToolPermission,
) {
}
