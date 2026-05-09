use dashmap::DashSet;
use std::collections::HashMap;
use std::sync::Arc;

use adk_rust::prelude::{Content, Tool, ToolContext};
use adk_rust::tool::McpToolset;
use adk_rust::{ReadonlyContext, Toolset};
use tokio::sync::OnceCell;
use domain::McpSpec;
use http::{HeaderName, HeaderValue};
use rmcp::{
    ServiceExt,
    transport::{
        TokioChildProcess,
        streamable_http_client::{StreamableHttpClientTransport, StreamableHttpClientTransportConfig},
    },
};
use tokio::process::Command;
use tracing::{error, info, warn};

struct McpEntry {
    _toolset: Arc<dyn adk_rust::Toolset>,
    server_name: String,
    tool_names: Vec<(String, String)>,
}

/// Wrapper that prefixes an MCP tool's name (e.g., `list_initiatives` →
/// `linear_list_initiatives`) so it matches the system prompt, and gates
/// `execute()` on the `loaded` DashSet.
struct PrefixedGuardTool {
    inner: Arc<dyn Tool>,
    loaded: Arc<DashSet<String>>,
    prefixed_name: String,
}

#[async_trait::async_trait]
impl Tool for PrefixedGuardTool {
    fn name(&self) -> &str { &self.prefixed_name }
    fn description(&self) -> &str { self.inner.description() }
    fn declaration(&self) -> serde_json::Value { self.inner.declaration() }
    fn parameters_schema(&self) -> Option<serde_json::Value> { self.inner.parameters_schema() }
    fn response_schema(&self) -> Option<serde_json::Value> { self.inner.response_schema() }
    fn is_long_running(&self) -> bool { self.inner.is_long_running() }

    async fn execute(&self, ctx: Arc<dyn ToolContext>, args: serde_json::Value) -> adk_rust::Result<serde_json::Value> {
        if self.loaded.contains(self.inner.name()) {
            self.inner.execute(ctx, args).await
        } else {
            Err(adk_rust::AdkError::tool(format!(
                "Tool '{}' is not loaded yet. Call load_tools(tool_names=[\"{}\"]) first.",
                self.prefixed_name, self.prefixed_name
            )))
        }
    }
}

/// Caches all discovered tools from the inner toolset on first call, then
/// filters by the `loaded` DashSet and prefixes names on each `tools()` call.
struct LoadedCachingToolset {
    inner: Arc<dyn Toolset>,
    loaded: Arc<DashSet<String>>,
    prefix: String,
    name: String,
    cache: OnceCell<Vec<Arc<dyn Tool>>>,
}

impl LoadedCachingToolset {
    fn new(inner: Arc<dyn Toolset>, loaded: Arc<DashSet<String>>, prefix: impl Into<String>) -> Self {
        let prefix = prefix.into();
        let name = format!("{}_caching", inner.name());
        Self { inner, loaded, prefix, name, cache: OnceCell::new() }
    }
}

#[async_trait::async_trait]
impl Toolset for LoadedCachingToolset {
    fn name(&self) -> &str {
        &self.name
    }

    async fn tools(&self, ctx: Arc<dyn ReadonlyContext>) -> adk_rust::Result<Vec<Arc<dyn Tool>>> {
        let all = self.cache.get_or_try_init(|| {
            let inner = self.inner.clone();
            let ctx = ctx.clone();
            Box::pin(async move { inner.tools(ctx).await })
        }).await?;
        Ok(all
            .iter()
            .filter(|t| self.loaded.contains(t.name()))
            .map(|t| {
                Arc::new(PrefixedGuardTool {
                    inner: t.clone(),
                    loaded: self.loaded.clone(),
                    prefixed_name: format!("{}_{}", self.prefix, t.name()),
                }) as Arc<dyn Tool>
            })
            .collect())
    }
}

pub struct McpRegistry {
    entries: Vec<McpEntry>,
    loaded: Arc<DashSet<String>>,
}

impl McpRegistry {
    pub async fn from_specs(specs: &[McpSpec]) -> Self {
        let mut entries: Vec<McpEntry> = Vec::new();
        let loaded = Arc::new(DashSet::new());

        for spec in specs {
            match connect_and_discover(spec, loaded.clone()).await {
                Ok((toolset, tool_names, server_name)) => {
                    for default_name in spec.default_enabled_tools() {
                        let found = tool_names.iter().find(|(pfx, raw)| {
                            raw == default_name || pfx.as_str() == default_name.as_str()
                        });
                        if let Some((_pfx, raw_name)) = found {
                            loaded.insert(raw_name.clone());
                            info!(name = %default_name, server = %server_name, "MCP tool auto-loaded");
                        } else {
                            warn!(name = %default_name, server = %server_name, "default_enabled_tool not found");
                        }
                    }
                    info!(server = %server_name, tool_count = tool_names.len(), loaded = loaded.len(), "MCP server connected");
                    entries.push(McpEntry { _toolset: toolset, server_name, tool_names });
                }
                Err(e) => error!(name = %spec.name(), error = %e, "failed to connect MCP server"),
            }
        }

        Self { entries, loaded }
    }

    pub fn toolsets(&self) -> Vec<Arc<dyn adk_rust::Toolset>> {
        self.entries.iter().map(|e| e._toolset.clone()).collect()
    }

    pub fn available_tool_names(&self) -> Vec<String> {
        let mut names: Vec<String> = self.entries.iter()
            .flat_map(|e| e.tool_names.iter().map(|(pfx, _)| pfx.clone()))
            .collect();
        names.sort();
        names.dedup();
        names
    }

    pub fn loaded_tool_names(&self) -> Vec<String> {
        let mut names: Vec<String> = Vec::new();
        for entry in &self.entries {
            for (prefixed, raw) in &entry.tool_names {
                if self.loaded.contains(raw) {
                    names.push(prefixed.clone());
                }
            }
        }
        names.sort();
        names
    }

    pub fn unloaded_tool_names(&self) -> Vec<String> {
        let mut names: Vec<String> = Vec::new();
        for entry in &self.entries {
            for (prefixed, raw) in &entry.tool_names {
                if !self.loaded.contains(raw) {
                    names.push(prefixed.clone());
                }
            }
        }
        names.sort();
        names
    }

    pub fn is_loaded(&self, tool_name: &str) -> bool {
        self.loaded.contains(tool_name)
    }

    pub fn load_tools(&self, names: &[String]) -> Vec<String> {
        let all: Vec<(&str, &str)> = self.entries.iter()
            .flat_map(|e| e.tool_names.iter().map(|(pfx, raw)| (pfx.as_str(), raw.as_str())))
            .collect();

        let mut loaded = Vec::new();
        let mut not_found = Vec::new();

        for name in names {
            if let Some((_, raw)) = all.iter().find(|(pfx, r)| *pfx == name.as_str() || *r == name.as_str()) {
                self.loaded.insert(raw.to_string());
                loaded.push(name.clone());
            } else {
                not_found.push(name.clone());
            }
        }

        if !not_found.is_empty() {
            info!(?not_found, "requested tools not found in any MCP server");
        }

        loaded
    }
}

struct DiscoveryContext {
    pub invocation_id: String,
    pub agent_name: String,
    pub user_id: String,
    pub app_name: String,
    pub session_id: String,
    pub branch: String,
    pub user_content: Content,
}

impl ReadonlyContext for DiscoveryContext {
    fn invocation_id(&self) -> &str { &self.invocation_id }
    fn agent_name(&self) -> &str { &self.agent_name }
    fn user_id(&self) -> &str { &self.user_id }
    fn app_name(&self) -> &str { &self.app_name }
    fn session_id(&self) -> &str { &self.session_id }
    fn branch(&self) -> &str { &self.branch }
    fn user_content(&self) -> &Content { &self.user_content }
}

async fn connect_and_discover(spec: &McpSpec, loaded: Arc<DashSet<String>>) -> anyhow::Result<(Arc<dyn adk_rust::Toolset>, Vec<(String, String)>, String)> {
    let server_name = spec.name().to_string();

    match spec {
        McpSpec::Stdio { name, command, args, env, tool_filter, .. } => {
            let mut cmd = Command::new(command);
            cmd.args(args.iter().map(|a| a.as_str()));
            for (key, value) in env { cmd.env(key, value); }
            info!(%name, "connecting MCP stdio server");
            let client = ().serve(TokioChildProcess::new(cmd)?).await?;
            let toolset = McpToolset::new(client).with_name(name.clone());
            let names = discover_names(&toolset, &server_name, tool_filter).await;
            let toolset = apply_filter(toolset, tool_filter);
            let toolset: Arc<dyn adk_rust::Toolset> =
                Arc::new(LoadedCachingToolset::new(Arc::new(toolset), loaded, &server_name));
            Ok((toolset, names, server_name))
        }
        McpSpec::Http { name, url, headers, tool_filter, .. }
        | McpSpec::StreamableHttp { name, url, headers, tool_filter, .. } => {
            let mut config = StreamableHttpClientTransportConfig::with_uri(url.clone());
            let custom_headers = build_headers(headers)?;
            if !custom_headers.is_empty() { config.custom_headers = custom_headers; }
            info!(%name, "connecting MCP HTTP server");
            let client = ().serve(StreamableHttpClientTransport::from_config(config)).await?;
            let toolset = McpToolset::new(client).with_name(name.clone());
            let names = discover_names(&toolset, &server_name, tool_filter).await;
            let toolset = apply_filter(toolset, tool_filter);
            let toolset: Arc<dyn adk_rust::Toolset> =
                Arc::new(LoadedCachingToolset::new(Arc::new(toolset), loaded, &server_name));
            Ok((toolset, names, server_name))
        }
    }
}

async fn discover_names(
    toolset: &McpToolset,
    server_name: &str,
    tool_filter: &Option<domain::ToolFilter>,
) -> Vec<(String, String)> {
    let ctx = Arc::new(DiscoveryContext {
        invocation_id: "discovery".into(), agent_name: "discovery".into(),
        user_id: "runtime".into(), app_name: "employee-bridge".into(),
        session_id: "discovery".into(), branch: "main".into(),
        user_content: Content::new("user"),
    });
    let tools = match toolset.tools(ctx).await {
        Ok(t) => t,
        Err(e) => { warn!(error = %e, "failed to discover tools"); return vec![]; }
    };
    let mut names = Vec::new();
    for tool in &tools {
        let raw = tool.name().to_string();
        if let Some(f) = tool_filter {
            if let Some(ref allow) = f.allow {
                if !allow.iter().any(|a| a == &raw) { continue; }
            }
            if let Some(ref deny) = f.deny {
                if deny.iter().any(|d| d == &raw) { continue; }
            }
        }
        let prefixed = format!("{}_{}", server_name, raw);
        names.push((prefixed, raw));
    }
    names
}

fn apply_filter(toolset: McpToolset, filter: &Option<domain::ToolFilter>) -> McpToolset {
    let Some(filter) = filter else { return toolset; };
    let allow = filter.allow.clone();
    let deny = filter.deny.clone();
    toolset.with_filter(move |name| {
        if let Some(ref deny_list) = deny {
            if deny_list.iter().any(|d| d == name) { return false; }
        }
        if let Some(ref allow_list) = allow {
            return allow_list.iter().any(|a| a == name);
        }
        true
    })
}

fn build_headers(headers: &HashMap<String, String>) -> Result<HashMap<HeaderName, HeaderValue>, anyhow::Error> {
    let mut map = HashMap::new();
    for (key, value) in headers {
        let name = HeaderName::from_bytes(key.as_bytes())?;
        let val = HeaderValue::from_str(value)?;
        map.insert(name, val);
    }
    Ok(map)
}
