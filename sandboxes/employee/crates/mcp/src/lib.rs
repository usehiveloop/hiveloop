use std::collections::HashMap;
use std::sync::Arc;

use adk_rust::tool::McpToolset;
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
use tracing::{error, info};

pub struct McpRegistry {
    toolsets: Vec<Arc<dyn adk_rust::Toolset>>,
}

impl McpRegistry {
    pub async fn from_specs(specs: &[McpSpec]) -> Self {
        let mut toolsets: Vec<Arc<dyn adk_rust::Toolset>> = Vec::new();
        for spec in specs {
            match connect_spec(spec).await {
                Ok(toolset) => toolsets.push(toolset),
                Err(e) => error!(name = %spec.name(), error = %e, "failed to connect MCP server"),
            }
        }
        Self { toolsets }
    }

    pub fn toolsets(&self) -> &[Arc<dyn adk_rust::Toolset>] {
        &self.toolsets
    }
}

async fn connect_spec(spec: &McpSpec) -> Result<Arc<dyn adk_rust::Toolset>, anyhow::Error> {
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
            info!(%name, %command, "connecting MCP stdio server");
            let client = ()
                .serve(TokioChildProcess::new(cmd)?)
                .await?;
            let mut toolset = McpToolset::new(client).with_name(name.clone());
            if let Some(filter) = tool_filter {
                toolset = apply_filter(toolset, filter);
            }
            Ok(Arc::new(toolset))
        }
        McpSpec::Http {
            name,
            url,
            headers,
            tool_filter,
        }
        | McpSpec::StreamableHttp {
            name,
            url,
            headers,
            tool_filter,
        } => {
            let mut config = StreamableHttpClientTransportConfig::with_uri(url.clone());
            let custom_headers = build_headers(headers)?;
            if !custom_headers.is_empty() {
                config.custom_headers = custom_headers;
            }
            info!(%name, %url, "connecting MCP streamable HTTP server");
            let client = ()
                .serve(StreamableHttpClientTransport::from_config(config))
                .await?;
            let mut toolset = McpToolset::new(client).with_name(name.clone());
            if let Some(filter) = tool_filter {
                toolset = apply_filter(toolset, filter);
            }
            Ok(Arc::new(toolset))
        }
    }
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

fn apply_filter(
    toolset: McpToolset,
    filter: &domain::ToolFilter,
) -> McpToolset {
    let allow: Option<Vec<String>> = filter.allow.clone();
    let deny: Option<Vec<String>> = filter.deny.clone();
    toolset.with_filter(move |name| {
        if let Some(ref deny_list) = deny {
            if deny_list.iter().any(|d| d == name) {
                return false;
            }
        }
        if let Some(ref allow_list) = allow {
            return allow_list.iter().any(|a| a == name);
        }
        true
    })
}
