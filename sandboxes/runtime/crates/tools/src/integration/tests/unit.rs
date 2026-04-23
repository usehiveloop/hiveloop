use crate::integration::*;
use bridge_core::integration::{IntegrationAction, IntegrationDefinition};
use bridge_core::permission::ToolPermission;

use crate::ToolExecutor;

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
