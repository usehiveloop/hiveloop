use std::sync::Arc;
use std::time::Duration;

use async_trait::async_trait;
use backon::{ExponentialBuilder, Retryable};
use bridge_core::integration::IntegrationDefinition;
use bridge_core::permission::ToolPermission;

use crate::ToolExecutor;

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

#[cfg(test)]
mod tests {
    use super::*;
    use bridge_core::integration::{IntegrationAction, IntegrationDefinition};

    #[test]
    fn test_integration_tool_name_format() {
        let executor = IntegrationToolExecutor::new(
            "github".to_string(),
            "create_pull_request".to_string(),
            "Create a PR".to_string(),
            serde_json::json!({}),
            "http://localhost:3000".to_string(),
        );
        assert_eq!(executor.name(), "github__create_pull_request");
    }

    #[test]
    fn test_integration_tool_description_has_prefix() {
        let executor = IntegrationToolExecutor::new(
            "github".to_string(),
            "create_pull_request".to_string(),
            "[github] Create a new pull request".to_string(),
            serde_json::json!({}),
            "http://localhost:3000".to_string(),
        );
        assert!(executor.description().contains("[github]"));
    }

    #[test]
    fn test_integration_tool_schema_matches_action() {
        let schema = serde_json::json!({
            "type": "object",
            "properties": {
                "title": { "type": "string" }
            },
            "required": ["title"]
        });
        let executor = IntegrationToolExecutor::new(
            "github".to_string(),
            "create_pull_request".to_string(),
            "Create a PR".to_string(),
            schema.clone(),
            "http://localhost:3000".to_string(),
        );
        assert_eq!(executor.parameters_schema(), schema);
    }

    #[test]
    fn test_create_integration_tools_filters_deny() {
        let integrations = vec![IntegrationDefinition {
            name: "github".to_string(),
            description: "GitHub".to_string(),
            actions: vec![
                IntegrationAction {
                    name: "list_issues".to_string(),
                    description: "List issues".to_string(),
                    parameters_schema: serde_json::json!({}),
                    permission: ToolPermission::Allow,
                },
                IntegrationAction {
                    name: "delete_repo".to_string(),
                    description: "Delete repo".to_string(),
                    parameters_schema: serde_json::json!({}),
                    permission: ToolPermission::Deny,
                },
                IntegrationAction {
                    name: "create_pr".to_string(),
                    description: "Create PR".to_string(),
                    parameters_schema: serde_json::json!({}),
                    permission: ToolPermission::RequireApproval,
                },
            ],
        }];

        let tools = create_integration_tools(&integrations, "http://localhost:3000");
        assert_eq!(tools.len(), 2, "deny action should be filtered out");

        let names: Vec<&str> = tools.iter().map(|(t, _)| t.name()).collect();
        assert!(names.contains(&"github__list_issues"));
        assert!(names.contains(&"github__create_pr"));
        assert!(!names.contains(&"github__delete_repo"));
    }

    #[test]
    fn test_create_integration_tools_returns_permissions() {
        let integrations = vec![IntegrationDefinition {
            name: "slack".to_string(),
            description: "Slack".to_string(),
            actions: vec![
                IntegrationAction {
                    name: "send_message".to_string(),
                    description: "Send".to_string(),
                    parameters_schema: serde_json::json!({}),
                    permission: ToolPermission::RequireApproval,
                },
                IntegrationAction {
                    name: "list_channels".to_string(),
                    description: "List".to_string(),
                    parameters_schema: serde_json::json!({}),
                    permission: ToolPermission::Allow,
                },
            ],
        }];

        let tools = create_integration_tools(&integrations, "http://localhost:3000");
        assert_eq!(tools.len(), 2);

        let send = tools
            .iter()
            .find(|(t, _)| t.name() == "slack__send_message");
        assert_eq!(send.unwrap().1, ToolPermission::RequireApproval);

        let list = tools
            .iter()
            .find(|(t, _)| t.name() == "slack__list_channels");
        assert_eq!(list.unwrap().1, ToolPermission::Allow);
    }

    #[test]
    fn test_create_integration_tools_empty_integrations() {
        let tools = create_integration_tools(&[], "http://localhost:3000");
        assert!(tools.is_empty());
    }

    #[test]
    fn test_parse_integration_tool_name() {
        assert_eq!(
            parse_integration_tool_name("github__create_pull_request"),
            Some(("github", "create_pull_request"))
        );
        assert_eq!(parse_integration_tool_name("bash"), None);
        assert_eq!(parse_integration_tool_name("__bad"), None);
        assert_eq!(parse_integration_tool_name("bad__"), None);
    }

    #[test]
    fn test_integration_tool_name_helper() {
        assert_eq!(
            integration_tool_name("github", "create_pull_request"),
            "github__create_pull_request"
        );
    }

    #[tokio::test]
    async fn test_integration_tool_execute_success() {
        use wiremock::matchers::{method, path};
        use wiremock::{Mock, MockServer, ResponseTemplate};

        let server = MockServer::start().await;
        let response_body = serde_json::json!({
            "id": 42,
            "number": 123,
            "title": "Test PR",
            "state": "open"
        });

        Mock::given(method("POST"))
            .and(path("/integrations/github/actions/create_pull_request"))
            .respond_with(ResponseTemplate::new(200).set_body_json(&response_body))
            .expect(1)
            .mount(&server)
            .await;

        let executor = IntegrationToolExecutor::new(
            "github".to_string(),
            "create_pull_request".to_string(),
            "Create a PR".to_string(),
            serde_json::json!({}),
            server.uri(),
        );

        let result = executor
            .execute(serde_json::json!({"title": "Test PR"}))
            .await
            .expect("should succeed");

        let parsed: serde_json::Value =
            serde_json::from_str(&result).expect("result should be valid JSON");
        assert_eq!(parsed["number"], 123);
        assert_eq!(parsed["state"], "open");
    }

    #[tokio::test]
    async fn test_integration_tool_execute_error_response() {
        use wiremock::matchers::{method, path};
        use wiremock::{Mock, MockServer, ResponseTemplate};

        let server = MockServer::start().await;
        let error_body = serde_json::json!({
            "error": "Unknown action 'bad_action' for integration 'github'",
            "available_actions": ["create_pull_request", "list_issues"]
        });

        Mock::given(method("POST"))
            .and(path("/integrations/github/actions/bad_action"))
            .respond_with(ResponseTemplate::new(404).set_body_json(&error_body))
            .expect(1)
            .mount(&server)
            .await;

        let executor = IntegrationToolExecutor::new(
            "github".to_string(),
            "bad_action".to_string(),
            "Bad action".to_string(),
            serde_json::json!({}),
            server.uri(),
        );

        // 404 is a client error — should be returned as-is (not retried)
        let result = executor.execute(serde_json::json!({})).await;
        assert!(
            result.is_ok(),
            "client errors should be returned as Ok (passthrough)"
        );
        let body = result.unwrap();
        assert!(body.contains("Unknown action"));
    }

    #[tokio::test]
    async fn test_integration_tool_execute_retry_on_server_error() {
        use wiremock::matchers::{method, path};
        use wiremock::{Mock, MockServer, ResponseTemplate};

        let server = MockServer::start().await;

        // First two calls return 500, third returns 200
        Mock::given(method("POST"))
            .and(path("/integrations/github/actions/list_issues"))
            .respond_with(ResponseTemplate::new(500).set_body_string("internal error"))
            .up_to_n_times(2)
            .mount(&server)
            .await;

        Mock::given(method("POST"))
            .and(path("/integrations/github/actions/list_issues"))
            .respond_with(ResponseTemplate::new(200).set_body_json(serde_json::json!([{"id": 1}])))
            .mount(&server)
            .await;

        let executor = IntegrationToolExecutor::new(
            "github".to_string(),
            "list_issues".to_string(),
            "List issues".to_string(),
            serde_json::json!({}),
            server.uri(),
        );

        let result = executor.execute(serde_json::json!({})).await;
        assert!(result.is_ok(), "should succeed after retries");
    }

    #[tokio::test]
    async fn test_integration_tool_execute_retry_exhausted() {
        use wiremock::matchers::{method, path};
        use wiremock::{Mock, MockServer, ResponseTemplate};

        let server = MockServer::start().await;

        Mock::given(method("POST"))
            .and(path("/integrations/github/actions/list_issues"))
            .respond_with(ResponseTemplate::new(500).set_body_string("internal error"))
            .mount(&server)
            .await;

        let executor = IntegrationToolExecutor::new(
            "github".to_string(),
            "list_issues".to_string(),
            "List issues".to_string(),
            serde_json::json!({}),
            server.uri(),
        );

        let result = executor.execute(serde_json::json!({})).await;
        assert!(result.is_err(), "should fail after retries exhausted");
        assert!(result.unwrap_err().contains("Server error"));
    }

    #[tokio::test]
    async fn test_integration_tool_sends_correct_request_body() {
        use wiremock::matchers::{body_partial_json, method, path};
        use wiremock::{Mock, MockServer, ResponseTemplate};

        let server = MockServer::start().await;

        Mock::given(method("POST"))
            .and(path("/integrations/slack/actions/send_message"))
            .and(body_partial_json(serde_json::json!({
                "params": {
                    "channel": "#general",
                    "text": "hello"
                }
            })))
            .respond_with(ResponseTemplate::new(200).set_body_json(serde_json::json!({"ok": true})))
            .expect(1)
            .mount(&server)
            .await;

        let executor = IntegrationToolExecutor::new(
            "slack".to_string(),
            "send_message".to_string(),
            "Send message".to_string(),
            serde_json::json!({}),
            server.uri(),
        );

        let result = executor
            .execute(serde_json::json!({"channel": "#general", "text": "hello"}))
            .await;
        assert!(result.is_ok());
    }
}
