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
    server_name: String,
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
}

impl McpRegistry {
    pub async fn from_specs(specs: &[McpSpec], runtime_env: &HashMap<String, String>) -> Self {
        let entries = connect_specs(specs, runtime_env).await;
        Self {
            entries: ArcSwap::from_pointee(entries),
        }
    }

    pub async fn reload_from_specs(
        &self,
        specs: &[McpSpec],
        runtime_env: &HashMap<String, String>,
    ) {
        let entries = connect_specs(specs, runtime_env).await;
        self.entries.store(Arc::new(entries));
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

    pub fn loaded_tools(&self) -> Vec<McpToolDefinition> {
        let entries = self.entries.load();
        let mut tools = Vec::new();
        for entry in entries.iter() {
            for tool in &entry.tools {
                tools.push(tool.clone());
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
                let mut arguments = match args {
                    serde_json::Value::Object(map) => map,
                    serde_json::Value::Null => JsonObject::new(),
                    other => {
                        let mut map = JsonObject::new();
                        map.insert("value".to_string(), other);
                        map
                    }
                };
                if entry.server_name == "hivy" {
                    arguments.insert(
                        "_hivy_session_id".to_string(),
                        serde_json::Value::String(session_id.to_string()),
                    );
                }
                let result = entry
                    .peer
                    .call_tool(CallToolRequestParams::new(raw.clone()).with_arguments(arguments))
                    .await?;
                return Ok(serde_json::to_value(result)?);
            }
        }
        anyhow::bail!("MCP tool '{prefixed_name}' not found")
    }
}

async fn connect_specs(specs: &[McpSpec], runtime_env: &HashMap<String, String>) -> Vec<McpEntry> {
    let mut entries: Vec<McpEntry> = Vec::new();

    for spec in specs {
        match connect_and_discover(spec, runtime_env).await {
            Ok((service, peer, tool_names, tools, server_name)) => {
                info!(server = %server_name, tool_count = tool_names.len(), "MCP server connected");
                entries.push(McpEntry {
                    server_name: server_name.clone(),
                    _service: service,
                    peer,
                    tool_names,
                    tools,
                });
            }
            Err(e) => error!(name = %spec.name(), error = %e, "failed to connect MCP server"),
        }
    }

    entries
}

async fn connect_and_discover(
    spec: &McpSpec,
    runtime_env: &HashMap<String, String>,
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
            let custom_headers = build_headers(headers, runtime_env)?;
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
    runtime_env: &HashMap<String, String>,
) -> Result<HashMap<HeaderName, HeaderValue>, anyhow::Error> {
    let mut map = HashMap::new();
    for (key, value) in headers {
        let name = HeaderName::from_bytes(key.as_bytes())?;
        let expanded = expand_env_placeholders(value, runtime_env);
        let val = HeaderValue::from_str(&expanded)?;
        map.insert(name, val);
    }
    Ok(map)
}

fn expand_env_placeholders(value: &str, runtime_env: &HashMap<String, String>) -> String {
    let mut output = value.to_string();
    for (key, env_value) in runtime_env {
        output = output.replace(&format!("${{{key}}}"), env_value);
    }
    output
}

#[cfg(test)]
mod tests {
    use super::expand_env_placeholders;
    use std::collections::HashMap;

    #[test]
    fn expands_env_placeholders_in_header_values() {
        let runtime_env = HashMap::from([(
            "HIVY_PROXY_API_KEY".to_string(),
            "proxy-test-token".to_string(),
        )]);
        assert_eq!(
            expand_env_placeholders("Bearer ${HIVY_PROXY_API_KEY}", &runtime_env),
            "Bearer proxy-test-token"
        );
    }
}
