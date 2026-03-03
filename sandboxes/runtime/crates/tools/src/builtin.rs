use std::sync::Arc;

use crate::ToolRegistry;

/// Register all built-in tools into the given registry.
/// Filesystem tools are always registered.
/// WebSearch is registered only when SEARCH_ENDPOINT env var is set.
pub fn register_builtin_tools(registry: &mut ToolRegistry) {
    // Filesystem tools (stateless, always available)
    registry.register(Arc::new(crate::grep::GrepTool::new()));
    registry.register(Arc::new(crate::read::ReadTool::new()));
    registry.register(Arc::new(crate::glob::GlobTool::new()));
    registry.register(Arc::new(crate::ls::LsTool::new()));

    // Write-side tools
    registry.register(Arc::new(crate::bash::BashTool::new()));
    registry.register(Arc::new(crate::edit::EditTool::new()));
    registry.register(Arc::new(crate::write::WriteTool::new()));
    registry.register(Arc::new(crate::apply_patch::ApplyPatchTool::new()));
    registry.register(Arc::new(crate::multiedit::MultiEditTool::new()));

    // Web fetch (no config needed)
    registry.register(Arc::new(crate::web_fetch::WebFetchTool::with_defaults()));

    // Web search (needs endpoint URL from control plane)
    if let Ok(endpoint) = std::env::var("SEARCH_ENDPOINT") {
        registry.register(Arc::new(crate::web_search::WebSearchTool::new(endpoint)));
    }

    // Agent tool — subagent invocation (uses task_local for context)
    registry.register(Arc::new(crate::agent::AgentTool::new()));

    // Batch tool — registered last with a snapshot of all other tools
    let tool_snapshot = registry.snapshot();
    registry.register(Arc::new(crate::batch::BatchTool::new(tool_snapshot)));
}

/// Register built-in tools for subagents (excludes the agent tool).
///
/// Subagents are leaf-level workers and should not be able to spawn
/// other subagents. This prevents unbounded recursion.
pub fn register_builtin_tools_for_subagent(registry: &mut ToolRegistry) {
    // Filesystem tools (stateless, always available)
    registry.register(Arc::new(crate::grep::GrepTool::new()));
    registry.register(Arc::new(crate::read::ReadTool::new()));
    registry.register(Arc::new(crate::glob::GlobTool::new()));
    registry.register(Arc::new(crate::ls::LsTool::new()));

    // Write-side tools
    registry.register(Arc::new(crate::bash::BashTool::new()));
    registry.register(Arc::new(crate::edit::EditTool::new()));
    registry.register(Arc::new(crate::write::WriteTool::new()));
    registry.register(Arc::new(crate::apply_patch::ApplyPatchTool::new()));
    registry.register(Arc::new(crate::multiedit::MultiEditTool::new()));

    // Web fetch (no config needed)
    registry.register(Arc::new(crate::web_fetch::WebFetchTool::with_defaults()));

    // Web search (needs endpoint URL from control plane)
    if let Ok(endpoint) = std::env::var("SEARCH_ENDPOINT") {
        registry.register(Arc::new(crate::web_search::WebSearchTool::new(endpoint)));
    }

    // No agent tool — subagents cannot spawn other subagents

    // Batch tool — registered last with a snapshot of all other tools
    let tool_snapshot = registry.snapshot();
    registry.register(Arc::new(crate::batch::BatchTool::new(tool_snapshot)));
}
