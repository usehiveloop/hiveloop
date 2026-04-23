use std::sync::Arc;

use crate::boundary::ProjectBoundary;
use crate::file_tracker::FileTracker;
use crate::todo::TodoState;
use crate::ToolRegistry;
use lsp::LspManager;

/// Helper: register a tool only if its name appears in the allowed list.
pub(super) fn maybe_register(
    registry: &mut ToolRegistry,
    tool: Arc<dyn crate::ToolExecutor>,
    allowed: Option<&[String]>,
) {
    if let Some(names) = allowed {
        if !names.iter().any(|n| n == tool.name()) {
            return;
        }
    }
    registry.register(tool);
}

/// Register only the built-in tools whose names appear in `allowed_tools`.
///
/// If `allowed_tools` is empty, NO tools are registered — an empty list means
/// the agent intentionally has no built-in tools.
/// Unknown tool names in the list are silently ignored.
pub fn register_filtered_builtin_tools(registry: &mut ToolRegistry, allowed_tools: &[String]) {
    register_filtered_builtin_tools_with_lsp(registry, allowed_tools, None);
}

/// Register filtered built-in tools, optionally including the LSP tool.
pub fn register_filtered_builtin_tools_with_lsp(
    registry: &mut ToolRegistry,
    allowed_tools: &[String],
    lsp_manager: Option<Arc<LspManager>>,
) {
    if allowed_tools.is_empty() {
        return;
    }

    let filter = Some(allowed_tools);
    let tracker = FileTracker::new();
    let boundary = ProjectBoundary::new(std::env::current_dir().unwrap_or_default());

    // Filesystem search tools
    maybe_register(
        registry,
        Arc::new(
            crate::read::ReadTool::new()
                .with_file_tracker(tracker.clone())
                .with_boundary(boundary.clone()),
        ),
        filter,
    );
    maybe_register(
        registry,
        Arc::new(crate::glob::GlobTool::new().with_boundary(boundary.clone())),
        filter,
    );
    maybe_register(registry, Arc::new(crate::ls::LsTool::new()), filter);
    maybe_register(
        registry,
        Arc::new(crate::ast_grep::AstGrepTool::new().with_boundary(boundary.clone())),
        filter,
    );
    maybe_register(
        registry,
        Arc::new(crate::rip_grep::RipGrepTool::new().with_boundary(boundary.clone())),
        filter,
    );

    // Write-side tools (with LSP manager for diagnostics)
    maybe_register(registry, Arc::new(crate::bash::BashTool::new()), filter);
    maybe_register(
        registry,
        Arc::new(
            crate::edit::EditTool::new()
                .with_file_tracker(tracker.clone())
                .with_boundary(boundary.clone())
                .with_lsp_manager_opt(lsp_manager.clone()),
        ),
        filter,
    );
    maybe_register(
        registry,
        Arc::new(
            crate::write::WriteTool::new()
                .with_file_tracker(tracker.clone())
                .with_boundary(boundary.clone())
                .with_lsp_manager_opt(lsp_manager.clone()),
        ),
        filter,
    );
    maybe_register(
        registry,
        Arc::new(
            crate::apply_patch::ApplyPatchTool::new().with_lsp_manager_opt(lsp_manager.clone()),
        ),
        filter,
    );
    maybe_register(
        registry,
        Arc::new(
            crate::multiedit::MultiEditTool::new()
                .with_file_tracker(tracker)
                .with_boundary(boundary)
                .with_lsp_manager_opt(lsp_manager.clone()),
        ),
        filter,
    );

    // Web fetch (with optional fallback service)
    let web_fetch_filtered = if let Ok(url) = std::env::var("BRIDGE_WEB_URL") {
        crate::web_fetch::WebFetchTool::with_fallback(url)
    } else {
        crate::web_fetch::WebFetchTool::with_defaults()
    };
    maybe_register(registry, Arc::new(web_fetch_filtered), filter);

    // Spider-backed web tools (search, crawl, links, screenshot, transform)
    if let Ok(base_url) = std::env::var("BRIDGE_WEB_URL") {
        let spider = Arc::new(crate::spider_tools::SpiderClient::new(base_url));
        maybe_register(
            registry,
            Arc::new(crate::spider_tools::WebSearchTool::new(spider.clone())),
            filter,
        );
        maybe_register(
            registry,
            Arc::new(crate::spider_tools::WebCrawlTool::new(spider.clone())),
            filter,
        );
        maybe_register(
            registry,
            Arc::new(crate::spider_tools::WebGetLinksTool::new(spider.clone())),
            filter,
        );
        maybe_register(
            registry,
            Arc::new(crate::spider_tools::WebScreenshotTool::new(spider.clone())),
            filter,
        );
        maybe_register(
            registry,
            Arc::new(crate::spider_tools::WebTransformTool::new(spider)),
            filter,
        );
    }

    // Todo tools
    let todo_state = TodoState::new();
    maybe_register(
        registry,
        Arc::new(crate::todo::TodoWriteTool::with_state(todo_state.clone())),
        filter,
    );
    maybe_register(
        registry,
        Arc::new(crate::todo::TodoReadTool::with_state(todo_state)),
        filter,
    );

    // LSP tool — code intelligence (only if manager provided and allowed)
    if let Some(manager) = lsp_manager {
        maybe_register(
            registry,
            Arc::new(crate::lsp_tool::LspTool::new(manager)),
            filter,
        );
    }

    // Self-delegation agent tool
    maybe_register(
        registry,
        Arc::new(crate::self_agent::AgentTool::new()),
        filter,
    );

    // Sub-agent tool
    maybe_register(
        registry,
        Arc::new(crate::agent::SubAgentTool::new()),
        filter,
    );

    // Batch tool — registered last with a snapshot of all other tools
    if allowed_tools.iter().any(|n| n == "batch") {
        let tool_snapshot = registry.snapshot();
        registry.register(Arc::new(crate::batch::BatchTool::new(tool_snapshot)));
    }
}
