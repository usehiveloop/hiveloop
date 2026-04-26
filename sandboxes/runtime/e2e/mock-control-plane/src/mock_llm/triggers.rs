/// Extract an integration tool trigger from the user message.
/// Pattern: "use_integration:INTEGRATION:ACTION" in the message text.
/// Returns Some((tool_name, cleaned_prompt)) if found.
pub(super) fn extract_integration_trigger(message: &str) -> Option<(String, String)> {
    let prefix = "use_integration:";
    if let Some(start) = message.find(prefix) {
        let after = &message[start + prefix.len()..];
        let parts: Vec<&str> = after.splitn(2, ':').collect();
        if parts.len() == 2 {
            let integration: String = parts[0]
                .chars()
                .take_while(|c| c.is_alphanumeric() || *c == '_' || *c == '-')
                .collect();
            let action: String = parts[1]
                .chars()
                .take_while(|c| c.is_alphanumeric() || *c == '_' || *c == '-')
                .collect();
            if !integration.is_empty() && !action.is_empty() {
                let tool_name = format!("{}__{}", integration, action);
                let trigger = format!("{}{}:{}", prefix, integration, action);
                let prompt = message.replace(&trigger, "").trim().to_string();
                return Some((tool_name, prompt));
            }
        }
    }
    None
}

/// Return mock arguments for a given integration tool call.
pub(super) fn mock_integration_args(tool_name: &str) -> serde_json::Value {
    match tool_name {
        "github__create_pull_request" => serde_json::json!({
            "title": "Add feature X",
            "body": "This PR adds feature X to the project",
            "head": "feature-x",
            "base": "main"
        }),
        "github__list_issues" => serde_json::json!({}),
        "github__get_repository" => serde_json::json!({}),
        "mailchimp__create_campaign" => serde_json::json!({
            "list_id": "list_default",
            "subject": "March Newsletter"
        }),
        "mailchimp__list_subscribers" => serde_json::json!({}),
        "slack__send_message" => serde_json::json!({
            "channel": "C01234567",
            "text": "Hello from the agent"
        }),
        "slack__list_channels" => serde_json::json!({}),
        _ => serde_json::json!({}),
    }
}

/// Extract an agent tool trigger from the user message.
/// Pattern: "use_agent:SUBAGENT_NAME" in the message text.
/// Returns Some((subagent_name, cleaned_prompt)) if found.
pub(super) fn extract_agent_trigger(message: &str) -> Option<(String, String)> {
    // Look for use_agent:NAME pattern
    let prefix = "use_agent:";
    if let Some(start) = message.find(prefix) {
        let after = &message[start + prefix.len()..];
        // Name is the next word (alphanumeric + underscore + hyphen)
        let name: String = after
            .chars()
            .take_while(|c| c.is_alphanumeric() || *c == '_' || *c == '-')
            .collect();
        if !name.is_empty() {
            // Build prompt from the message with the trigger removed
            let trigger = format!("{}{}", prefix, name);
            let prompt = message.replace(&trigger, "").trim().to_string();
            return Some((name, prompt));
        }
    }
    None
}
