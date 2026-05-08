use dashmap::DashSet;
use std::collections::HashMap;
use std::sync::Arc;

use adk_rust::prelude::Content;
use adk_rust::tool::McpToolset;
use adk_rust::{ReadonlyContext, Toolset};
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
    tool_names: Vec<String>,
}

pub struct McpRegistry {
    entries: Vec<McpEntry>,
    loaded: DashSet<String>,
}

impl McpRegistry {
    pub async fn from_specs(specs: &[McpSpec]) -> Self {
        let mut entries: Vec<McpEntry> = Vec::new();
        let loaded = DashSet::new();

        for spec in specs {
            match connect_and_discover(spec).await {
                Ok((toolset, tool_names)) => {
                    for name in spec.default_enabled_tools() {
                        if tool_names.iter().any(|t| t == name) {
                            loaded.insert(name.clone());
                            info!(%name, server = %spec.name(), "MCP tool auto-loaded");
                        } else {
                            warn!(%name, server = %spec.name(), "default_enabled_tool not found");
                        }
                    }
                    info!(server = %spec.name(), tool_count = tool_names.len(), loaded = loaded.len(), "MCP server connected");
                    entries.push(McpEntry { _toolset: toolset, tool_names });
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
            .flat_map(|e| e.tool_names.clone())
            .collect();
        names.sort();
        names.dedup();
        names
    }

    pub fn loaded_tool_names(&self) -> Vec<String> {
        let mut names: Vec<String> = self.available_tool_names();
        names.retain(|n| self.loaded.contains(n));
        names
    }

    pub fn unloaded_tool_names(&self) -> Vec<String> {
        let mut names: Vec<String> = self.available_tool_names();
        names.retain(|n| !self.loaded.contains(n));
        names
    }

    pub fn is_loaded(&self, tool_name: &str) -> bool {
        self.loaded.contains(tool_name)
    }

    pub fn load_tools(&self, names: &[String]) -> Vec<String> {
        let all: Vec<String> = self.entries.iter()
            .flat_map(|e| e.tool_names.clone())
            .collect();

        let mut loaded = Vec::new();
        let mut not_found = Vec::new();

        for name in names {
            if all.iter().any(|t| t == name) {
                self.loaded.insert(name.clone());
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

impl adk_rust::ReadonlyContext for DiscoveryContext {
    fn invocation_id(&self) -> &str { &self.invocation_id }
    fn agent_name(&self) -> &str { &self.agent_name }
    fn user_id(&self) -> &str { &self.user_id }
    fn app_name(&self) -> &str { &self.app_name }
    fn session_id(&self) -> &str { &self.session_id }
    fn branch(&self) -> &str { &self.branch }
    fn user_content(&self) -> &Content { &self.user_content }
}

async fn connect_and_discover(spec: &McpSpec) -> anyhow::Result<(Arc<dyn adk_rust::Toolset>, Vec<String>)> {
    let (toolset, tool_names) = match spec {
        McpSpec::Stdio { name, command, args, env, tool_filter, .. } => {
            let mut cmd = Command::new(command);
            cmd.args(args.iter().map(|a| a.as_str()));
            for (key, value) in env { cmd.env(key, value); }
            info!(%name, "connecting MCP stdio server");
            let client = ().serve(TokioChildProcess::new(cmd)?).await?;
            let toolset = McpToolset::new(client).with_name(name.clone());
            let ctx = Arc::new(DiscoveryContext {
                invocation_id: "discovery".into(), agent_name: "discovery".into(),
                user_id: "runtime".into(), app_name: "employee-bridge".into(),
                session_id: "discovery".into(), branch: "main".into(),
                user_content: Content::new("user"),
            });
            let names = match toolset.tools(ctx).await {
                Ok(tools) => {
                    let mut names: Vec<String> = tools.iter().map(|t| t.name().to_string()).collect();
                    if let Some(f) = tool_filter {
                        if let Some(ref allow) = f.allow { names.retain(|n| allow.iter().any(|a| a == n)); }
                        if let Some(ref deny) = f.deny { names.retain(|n| !deny.iter().any(|d| d == n)); }
                    }
                    names
                }
                Err(e) => { warn!(error = %e, "failed to discover tools"); vec![] }
            };
            let toolset = apply_filter(toolset, tool_filter);
            (Arc::new(toolset) as Arc<dyn adk_rust::Toolset>, names)
        }
        McpSpec::Http { name, url, headers, tool_filter, .. }
        | McpSpec::StreamableHttp { name, url, headers, tool_filter, .. } => {
            let mut config = StreamableHttpClientTransportConfig::with_uri(url.clone());
            let custom_headers = build_headers(headers)?;
            if !custom_headers.is_empty() { config.custom_headers = custom_headers; }
            info!(%name, "connecting MCP HTTP server");
            let client = ().serve(StreamableHttpClientTransport::from_config(config)).await?;
            let toolset = McpToolset::new(client).with_name(name.clone());
            let ctx = Arc::new(DiscoveryContext {
                invocation_id: "discovery".into(), agent_name: "discovery".into(),
                user_id: "runtime".into(), app_name: "employee-bridge".into(),
                session_id: "discovery".into(), branch: "main".into(),
                user_content: Content::new("user"),
            });
            let names = match toolset.tools(ctx).await {
                Ok(tools) => {
                    let mut names: Vec<String> = tools.iter().map(|t| t.name().to_string()).collect();
                    if let Some(f) = tool_filter {
                        if let Some(ref allow) = f.allow { names.retain(|n| allow.iter().any(|a| a == n)); }
                        if let Some(ref deny) = f.deny { names.retain(|n| !deny.iter().any(|d| d == n)); }
                    }
                    names
                }
                Err(e) => { warn!(error = %e, "failed to discover tools"); vec![] }
            };
            let toolset = apply_filter(toolset, tool_filter);
            (Arc::new(toolset) as Arc<dyn adk_rust::Toolset>, names)
        }
    };
    Ok((toolset, tool_names))
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
