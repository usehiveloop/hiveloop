use std::sync::Arc;
use std::time::Duration;

use async_trait::async_trait;
use backon::{ExponentialBuilder, Retryable};
use bridge_core::integration::IntegrationDefinition;
use bridge_core::permission::ToolPermission;

use crate::ToolExecutor;

#[cfg(test)]
mod tests;

/// Tool executor for a single integration action.
///
/// Each instance represents one action (e.g., `github__create_pull_request`)
/// and forwards execution to the control plane, which proxies to the external service.
pub struct IntegrationToolExecutor {
    integration_name: String,
    action_name: String,
    tool_name: String,
    description: String,
    schema: serde_json::Value,
    client: reqwest::Client,
    control_plane_url: String,
}

impl IntegrationToolExecutor {
    pub fn new(
        integration_name: String,
        action_name: String,
        description: String,
        schema: serde_json::Value,
        control_plane_url: String,
    ) -> Self {
        let tool_name = format!("{}__{}", integration_name, action_name);
        Self {
            integration_name,
            action_name,
            tool_name,
            description,
            schema,
            client: reqwest::Client::builder()
                .timeout(Duration::from_secs(30))
                .build()
                .expect("failed to build HTTP client"),
            control_plane_url,
        }
    }

    async fn execute_with_retry(&self, params: serde_json::Value) -> Result<String, String> {
        let client = &self.client;
        let url = format!(
            "{}/integrations/{}/actions/{}",
            self.control_plane_url, self.integration_name, self.action_name
        );

        let do_request = || async {
            let body = serde_json::json!({
                "params": params,
            });

            let response = client
                .post(&url)
                .header("Content-Type", "application/json")
                .json(&body)
                .send()
                .await
                .map_err(|e| format!("Integration request failed: {e}"))?;

            let status = response.status();
            let response_body = response
                .text()
                .await
                .map_err(|e| format!("Failed to read integration response: {e}"))?;

            if status.is_server_error() {
                return Err(format!("Server error {status}: {response_body}"));
            }

            // For client errors (4xx), return the body as-is — the control plane
            // returns helpful error messages for unknown actions, permission issues, etc.
            Ok(response_body)
        };

        do_request
            .retry(
                ExponentialBuilder::default()
                    .with_min_delay(Duration::from_millis(500))
                    .with_max_delay(Duration::from_secs(5))
                    .with_max_times(3),
            )
            .when(|e: &String| {
                e.starts_with("Server error") || e.starts_with("Integration request failed")
            })
            .await
    }
}

#[async_trait]
impl ToolExecutor for IntegrationToolExecutor {
    fn name(&self) -> &str {
        &self.tool_name
    }

    fn description(&self) -> &str {
        &self.description
    }

    fn parameters_schema(&self) -> serde_json::Value {
        self.schema.clone()
    }

    async fn execute(&self, args: serde_json::Value) -> Result<String, String> {
        self.execute_with_retry(args).await
    }

    fn as_any(&self) -> &dyn std::any::Any {
        self
    }
}

/// Create tool executors for all non-denied integration actions.
///
/// Returns each executor paired with its permission level so the caller
/// can populate the agent's permissions map.
pub fn create_integration_tools(
    integrations: &[IntegrationDefinition],
    control_plane_url: &str,
) -> Vec<(Arc<dyn ToolExecutor>, ToolPermission)> {
    let mut tools = Vec::new();

    for integration in integrations {
        for action in &integration.actions {
            // Deny actions are never exposed to the LLM
            if action.permission == ToolPermission::Deny {
                continue;
            }

            let executor = Arc::new(IntegrationToolExecutor::new(
                integration.name.clone(),
                action.name.clone(),
                format!("[{}] {}", integration.name, action.description),
                action.parameters_schema.clone(),
                control_plane_url.to_string(),
            ));

            tools.push((executor as Arc<dyn ToolExecutor>, action.permission.clone()));
        }
    }

    tools
}

/// Format an integration tool name from integration + action names.
pub fn integration_tool_name(integration: &str, action: &str) -> String {
    format!("{}__{}", integration, action)
}

/// Check if a tool name matches the integration naming convention and
/// extract the integration and action names.
pub fn parse_integration_tool_name(tool_name: &str) -> Option<(&str, &str)> {
    tool_name
        .split_once("__")
        .filter(|(integration, action)| !integration.is_empty() && !action.is_empty())
}
