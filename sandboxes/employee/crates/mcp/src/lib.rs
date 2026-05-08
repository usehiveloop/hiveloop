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
    server_name: String,
    tool_names: Vec<(String, String)>,
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
            let toolset = apply_loaded_filter(toolset, loaded);
            Ok((Arc::new(toolset), names, server_name))
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
            let toolset = apply_loaded_filter(toolset, loaded);
            Ok((Arc::new(toolset), names, server_name))
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

fn apply_loaded_filter(toolset: McpToolset, loaded: Arc<DashSet<String>>) -> McpToolset {
    toolset.with_filter(move |name| loaded.contains(name))
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
