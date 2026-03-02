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

    // Web fetch (no config needed)
    registry.register(Arc::new(crate::web_fetch::WebFetchTool::with_defaults()));

    // Web search (needs endpoint URL from control plane)
    if let Ok(endpoint) = std::env::var("SEARCH_ENDPOINT") {
        registry.register(Arc::new(crate::web_search::WebSearchTool::new(endpoint)));
    }
}
