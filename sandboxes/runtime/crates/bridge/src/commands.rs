use tracing::{info, warn};

use crate::cli::ToolCommands;

/// Tool information for JSON output
#[derive(serde::Serialize)]
pub(crate) struct ToolInfo {
    name: String,
    description: String,
    category: String,
    #[serde(skip)]
    is_read_only: bool,
    parameters: serde_json::Value,
}

pub(crate) async fn handle_install_lsp_command(servers: String) -> anyhow::Result<()> {
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| tracing_subscriber::EnvFilter::new("info")),
        )
        .init();

    let server_ids: Vec<String> = servers
        .split(',')
        .map(|s| s.trim().to_string())
        .filter(|s| !s.is_empty())
        .collect();

    if server_ids.is_empty() {
        anyhow::bail!("no servers specified; pass a comma-separated list or \"all\"");
    }

    info!(servers = ?server_ids, "starting LSP server installation");
    let installer = lsp::LspInstaller::new();
    let results = installer.install(&server_ids).await;

    let mut succeeded = 0;
    let mut failed: Vec<(String, String)> = Vec::new();
    for (id, result) in &results {
        match result {
            Ok(_) => {
                info!(server = %id, "installed successfully");
                succeeded += 1;
            }
            Err(e) => {
                warn!(server = %id, error = %e, "installation failed");
                failed.push((id.clone(), e.clone()));
            }
        }
    }

    info!(
        succeeded,
        failed = failed.len(),
        "LSP server installation complete"
    );

    // Per-server failures are non-fatal: a missing toolchain (opam, gem,
    // dotnet, ...) on the host should not stop the rest of `install-lsp all`
    // from succeeding, nor should it make this command exit non-zero — the
    // operator can install the underlying toolchain and re-run the specific
    // id. We surface a single summary warning so the failure is visible.
    if !failed.is_empty() {
        let summary: String = failed
            .iter()
            .map(|(id, err)| format!("{} ({})", id, err))
            .collect::<Vec<_>>()
            .join(", ");
        warn!(
            count = failed.len(),
            details = %summary,
            "some LSP servers were skipped"
        );
    }
    Ok(())
}

pub(crate) async fn handle_tools_command(action: Option<ToolCommands>) -> anyhow::Result<()> {
    let action = action.unwrap_or(ToolCommands::List {
        json: true,
        read_only: false,
    });

    match action {
        ToolCommands::List { json: _, read_only } => {
            let tools = get_tools_info(read_only)?;
            println!("{}", serde_json::to_string_pretty(&tools)?);
            Ok(())
        }
    }
}

fn get_tools_info(filter_read_only: bool) -> anyhow::Result<Vec<ToolInfo>> {
    use tools::{register_builtin_tools, ToolRegistry};

    let mut registry = ToolRegistry::new();
    register_builtin_tools(&mut registry);

    let mut tools: Vec<ToolInfo> = registry
        .snapshot()
        .values()
        .map(|tool| {
            let name = tool.name();
            let category = categorize_tool(name);
            let is_read_only = is_read_only_tool(name);

            ToolInfo {
                name: name.to_string(),
                description: tool.description().to_string(),
                category,
                is_read_only,
                parameters: tool.parameters_schema(),
            }
        })
        .collect();

    // Filter to read-only tools if requested
    if filter_read_only {
        tools.retain(|t| t.is_read_only);
    }

    Ok(tools)
}

fn categorize_tool(name: &str) -> String {
    match name {
        "bash" | "agent" | "sub_agent" | "Batch" => "action".to_string(),
        "Read" | "write" | "edit" | "apply_patch" | "LS" | "Glob" | "RipGrep" | "AstGrep" => {
            "filesystem".to_string()
        }
        "web_fetch" | "WebSearch" => "web".to_string(),
        "TodoWrite" | "TodoRead" => "task".to_string(),
        "lsp" => "code".to_string(),
        "skill" => "skill".to_string(),
        _ => "other".to_string(),
    }
}

/// Check if a tool is read-only (doesn't modify state)
fn is_read_only_tool(name: &str) -> bool {
    matches!(
        name,
        "Read" | "RipGrep" | "AstGrep" | "Glob" | "LS" | "web_fetch" | "todoread"
    )
}
