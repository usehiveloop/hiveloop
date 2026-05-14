use dashmap::{DashMap, DashSet};
use std::collections::HashMap;
use std::sync::Arc;

use arc_swap::ArcSwap;
use domain::McpSpec;
use http::{HeaderName, HeaderValue};
use rmcp::{
    model::{CallToolRequestParams, JsonObject},
    service::RunningService,
    transport::{
        streamable_http_client::{
            StreamableHttpClientTransport, StreamableHttpClientTransportConfig,
        },
        TokioChildProcess,
    },
    Peer, RoleClient, ServiceExt,
};
use tokio::process::Command;
use tracing::{error, info, warn};

struct McpEntry {
    _service: RunningService<RoleClient, ()>,
    peer: Peer<RoleClient>,
    tool_names: Vec<(String, String)>,
    tools: Vec<McpToolDefinition>,
}

#[derive(Debug, Clone)]
pub struct McpToolDefinition {
    pub prefixed_name: String,
    pub raw_name: String,
    pub description: String,
    pub parameters: serde_json::Value,
}

pub struct McpRegistry {
    entries: ArcSwap<Vec<McpEntry>>,
    default_loaded: ArcSwap<DashSet<String>>,
    session_loaded: DashMap<String, Arc<DashSet<String>>>,
}

impl McpRegistry {
    pub async fn from_specs(specs: &[McpSpec]) -> Self {
        let (entries, default_loaded) = connect_specs(specs).await;
        Self {
            entries: ArcSwap::from_pointee(entries),
            default_loaded: ArcSwap::from(default_loaded),
            session_loaded: DashMap::new(),
        }
    }

    pub async fn reload_from_specs(&self, specs: &[McpSpec]) {
        let (entries, default_loaded) = connect_specs(specs).await;
        self.entries.store(Arc::new(entries));
        self.default_loaded.store(default_loaded);
        self.session_loaded.clear();
    }

    pub fn available_tool_names(&self) -> Vec<String> {
        let entries = self.entries.load();
        let mut names: Vec<String> = entries
            .iter()
            .flat_map(|e| e.tool_names.iter().map(|(pfx, _)| pfx.clone()))
            .collect();
        names.sort();
        names.dedup();
        names
    }

    pub fn loaded_tool_names(&self) -> Vec<String> {
        self.loaded_tool_names_for_session("")
    }

    pub fn loaded_tool_names_for_session(&self, session_id: &str) -> Vec<String> {
        let entries = self.entries.load();
        let mut names: Vec<String> = Vec::new();
        for entry in entries.iter() {
            for (prefixed, raw) in &entry.tool_names {
                if self.is_raw_loaded_for_session(raw, session_id) {
                    names.push(prefixed.clone());
                }
            }
        }
        names.sort();
        names
    }

    pub fn unloaded_tool_names(&self) -> Vec<String> {
        self.unloaded_tool_names_for_session("")
    }

    pub fn unloaded_tool_names_for_session(&self, session_id: &str) -> Vec<String> {
        let entries = self.entries.load();
        let mut names: Vec<String> = Vec::new();
        for entry in entries.iter() {
            for (prefixed, raw) in &entry.tool_names {
                if !self.is_raw_loaded_for_session(raw, session_id) {
                    names.push(prefixed.clone());
                }
            }
        }
        names.sort();
        names
    }

    pub fn is_loaded(&self, tool_name: &str) -> bool {
        self.default_loaded.load().contains(tool_name)
    }

    pub fn load_tools(&self, names: &[String]) -> Vec<String> {
        self.load_tools_for_session("", names)
    }

    pub fn load_tools_for_session(&self, session_id: &str, names: &[String]) -> Vec<String> {
        let entries = self.entries.load();
        let all: Vec<(&str, &str)> = entries
            .iter()
            .flat_map(|e| {
                e.tool_names
                    .iter()
                    .map(|(pfx, raw)| (pfx.as_str(), raw.as_str()))
            })
            .collect();

        let mut loaded = Vec::new();
        let mut not_found = Vec::new();
        let session_loaded = self.session_loaded_set(session_id);
        let default_loaded = self.default_loaded.load();

        for name in names {
            if let Some((_, raw)) = all
                .iter()
                .find(|(pfx, r)| *pfx == name.as_str() || *r == name.as_str())
            {
                if !default_loaded.contains(*raw) {
                    session_loaded.insert(raw.to_string());
                }
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

    pub fn loaded_tools(&self) -> Vec<McpToolDefinition> {
        self.loaded_tools_for_session("")
    }

    pub fn loaded_tools_for_session(&self, session_id: &str) -> Vec<McpToolDefinition> {
        let entries = self.entries.load();
        let mut tools = Vec::new();
        for entry in entries.iter() {
            for tool in &entry.tools {
                if self.is_raw_loaded_for_session(&tool.raw_name, session_id) {
                    tools.push(tool.clone());
                }
            }
        }
        tools.sort_by(|a, b| a.prefixed_name.cmp(&b.prefixed_name));
        tools
    }

    pub async fn call_tool(
        &self,
        prefixed_name: &str,
        args: serde_json::Value,
    ) -> anyhow::Result<serde_json::Value> {
        self.call_tool_for_session("", prefixed_name, args).await
    }

    pub async fn call_tool_for_session(
        &self,
        session_id: &str,
        prefixed_name: &str,
        args: serde_json::Value,
    ) -> anyhow::Result<serde_json::Value> {
        let entries = self.entries.load();
        for entry in entries.iter() {
            if let Some((_, raw)) = entry
                .tool_names
                .iter()
                .find(|(pfx, _)| pfx == prefixed_name)
            {
                if !self.is_raw_loaded_for_session(raw, session_id) {
                    anyhow::bail!(
                        "Tool '{prefixed_name}' is not loaded yet. Call load_tools first."
                    );
                }
                let arguments = match args {
                    serde_json::Value::Object(map) => map,
                    serde_json::Value::Null => JsonObject::new(),
                    other => {
                        let mut map = JsonObject::new();
                        map.insert("value".to_string(), other);
                        map
                    }
                };
                let result = entry
                    .peer
                    .call_tool(CallToolRequestParams::new(raw.clone()).with_arguments(arguments))
                    .await?;
                return Ok(serde_json::to_value(result)?);
            }
        }
        anyhow::bail!("MCP tool '{prefixed_name}' not found")
    }

    fn is_raw_loaded_for_session(&self, raw_name: &str, session_id: &str) -> bool {
        if self.default_loaded.load().contains(raw_name) {
            return true;
        }
        self.session_loaded
            .get(session_id)
            .is_some_and(|loaded| loaded.contains(raw_name))
    }

    fn session_loaded_set(&self, session_id: &str) -> Arc<DashSet<String>> {
        if let Some(loaded) = self.session_loaded.get(session_id) {
            return loaded.clone();
        }
        let loaded = Arc::new(DashSet::new());
        let existing = self
            .session_loaded
            .insert(session_id.to_string(), loaded.clone());
        existing.unwrap_or(loaded)
    }
}

async fn connect_specs(specs: &[McpSpec]) -> (Vec<McpEntry>, Arc<DashSet<String>>) {
    let mut entries: Vec<McpEntry> = Vec::new();
    let default_loaded = Arc::new(DashSet::new());

    for spec in specs {
        match connect_and_discover(spec).await {
            Ok((service, peer, tool_names, tools, server_name)) => {
                if spec.default_enable_all_tools() {
                    for (_prefixed_name, raw_name) in &tool_names {
                        default_loaded.insert(raw_name.clone());
                    }
                    info!(server = %server_name, loaded = tool_names.len(), "all MCP tools auto-loaded");
                }
                for default_name in spec.default_enabled_tools() {
                    let found = tool_names.iter().find(|(pfx, raw)| {
                        raw == default_name || pfx.as_str() == default_name.as_str()
                    });
                    if let Some((_pfx, raw_name)) = found {
                        default_loaded.insert(raw_name.clone());
                        info!(name = %default_name, server = %server_name, "MCP tool auto-loaded");
                    } else {
                        warn!(name = %default_name, server = %server_name, "default_enabled_tool not found");
                    }
                }
                info!(server = %server_name, tool_count = tool_names.len(), loaded = default_loaded.len(), "MCP server connected");
                entries.push(McpEntry {
                    _service: service,
                    peer,
                    tool_names,
                    tools,
                });
            }
            Err(e) => error!(name = %spec.name(), error = %e, "failed to connect MCP server"),
        }
    }

    (entries, default_loaded)
}

async fn connect_and_discover(
    spec: &McpSpec,
) -> anyhow::Result<(
    RunningService<RoleClient, ()>,
    Peer<RoleClient>,
    Vec<(String, String)>,
    Vec<McpToolDefinition>,
    String,
)> {
    let server_name = spec.name().to_string();

    match spec {
        McpSpec::Stdio {
            name,
            command,
            args,
            env,
            tool_filter,
            ..
        } => {
            let mut cmd = Command::new(command);
            cmd.args(args.iter().map(|a| a.as_str()));
            for (key, value) in env {
                cmd.env(key, value);
            }
            info!(%name, "connecting MCP stdio server");
            let service = ().serve(TokioChildProcess::new(cmd)?).await?;
            let peer = service.peer().clone();
            let (names, tools) = discover_tools(&peer, &server_name, tool_filter).await;
            Ok((service, peer, names, tools, server_name))
        }
        McpSpec::Http {
            name,
            url,
            headers,
            tool_filter,
            ..
        }
        | McpSpec::StreamableHttp {
            name,
            url,
            headers,
            tool_filter,
            ..
        } => {
            let mut config = StreamableHttpClientTransportConfig::with_uri(url.clone());
            let custom_headers = build_headers(headers)?;
            if !custom_headers.is_empty() {
                config.custom_headers = custom_headers;
            }
            info!(%name, "connecting MCP HTTP server");
            let service = ().serve(StreamableHttpClientTransport::from_config(config)).await?;
            let peer = service.peer().clone();
            let (names, tools) = discover_tools(&peer, &server_name, tool_filter).await;
            Ok((service, peer, names, tools, server_name))
        }
    }
}

async fn discover_tools(
    peer: &Peer<RoleClient>,
    server_name: &str,
    tool_filter: &Option<domain::ToolFilter>,
) -> (Vec<(String, String)>, Vec<McpToolDefinition>) {
    let discovered = match peer.list_all_tools().await {
        Ok(t) => t,
        Err(e) => {
            warn!(error = %e, "failed to discover tools");
            return (vec![], vec![]);
        }
    };
    let mut names = Vec::new();
    let mut defs = Vec::new();
    for tool in discovered {
        let raw = tool.name.to_string();
        if let Some(f) = tool_filter {
            if let Some(ref allow) = f.allow {
                if !allow.iter().any(|a| a == &raw) {
                    continue;
                }
            }
            if let Some(ref deny) = f.deny {
                if deny.iter().any(|d| d == &raw) {
                    continue;
                }
            }
        }
        let prefixed = format!("{}_{}", server_name, raw);
        names.push((prefixed.clone(), raw.clone()));
        defs.push(McpToolDefinition {
            prefixed_name: prefixed,
            raw_name: raw,
            description: tool.description.map(|d| d.into_owned()).unwrap_or_default(),
            parameters: serde_json::Value::Object((*tool.input_schema).clone()),
        });
    }
    names.sort();
    defs.sort_by(|a, b| a.prefixed_name.cmp(&b.prefixed_name));
    (names, defs)
}

fn build_headers(
    headers: &HashMap<String, String>,
) -> Result<HashMap<HeaderName, HeaderValue>, anyhow::Error> {
    let mut map = HashMap::new();
    for (key, value) in headers {
        let name = HeaderName::from_bytes(key.as_bytes())?;
        let expanded = expand_env_placeholders(value);
        let val = HeaderValue::from_str(&expanded)?;
        map.insert(name, val);
    }
    Ok(map)
}

fn expand_env_placeholders(value: &str) -> String {
    let mut output = value.to_string();
    for (key, env_value) in std::env::vars() {
        output = output.replace(&format!("${{{key}}}"), &env_value);
    }
    output
}

#[cfg(test)]
mod tests {
    use super::expand_env_placeholders;

    #[test]
    fn expands_env_placeholders_in_header_values() {
        unsafe {
            std::env::set_var("HIVELOOP_PROXY_API_KEY", "proxy-test-token");
        }
        assert_eq!(
            expand_env_placeholders("Bearer ${HIVELOOP_PROXY_API_KEY}"),
            "Bearer proxy-test-token"
        );
    }
}
