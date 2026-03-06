use bridge_core::webhook::{WebhookEventType, WebhookPayload};
use serde_json::json;

/// Create a webhook payload for a conversation_created event.
pub fn conversation_created(
    agent_id: &str,
    conv_id: &str,
    webhook_url: &str,
    webhook_secret: &str,
) -> WebhookPayload {
    WebhookPayload::new(
        WebhookEventType::ConversationCreated,
        agent_id,
        conv_id,
        json!({}),
        webhook_url,
        webhook_secret,
    )
}

/// Create a webhook payload for a message_received event.
pub fn message_received(
    agent_id: &str,
    conv_id: &str,
    data: serde_json::Value,
    webhook_url: &str,
    webhook_secret: &str,
) -> WebhookPayload {
    WebhookPayload::new(
        WebhookEventType::MessageReceived,
        agent_id,
        conv_id,
        data,
        webhook_url,
        webhook_secret,
    )
}

/// Create a webhook payload for a response_started event.
pub fn response_started(
    agent_id: &str,
    conv_id: &str,
    webhook_url: &str,
    webhook_secret: &str,
) -> WebhookPayload {
    WebhookPayload::new(
        WebhookEventType::ResponseStarted,
        agent_id,
        conv_id,
        json!({}),
        webhook_url,
        webhook_secret,
    )
}

/// Create a webhook payload for a response_chunk event.
pub fn response_chunk(
    agent_id: &str,
    conv_id: &str,
    data: serde_json::Value,
    webhook_url: &str,
    webhook_secret: &str,
) -> WebhookPayload {
    WebhookPayload::new(
        WebhookEventType::ResponseChunk,
        agent_id,
        conv_id,
        data,
        webhook_url,
        webhook_secret,
    )
}

/// Create a webhook payload for a response_completed event.
pub fn response_completed(
    agent_id: &str,
    conv_id: &str,
    data: serde_json::Value,
    webhook_url: &str,
    webhook_secret: &str,
) -> WebhookPayload {
    WebhookPayload::new(
        WebhookEventType::ResponseCompleted,
        agent_id,
        conv_id,
        data,
        webhook_url,
        webhook_secret,
    )
}

/// Create a webhook payload for a tool_call_started event.
pub fn tool_call_started(
    agent_id: &str,
    conv_id: &str,
    data: serde_json::Value,
    webhook_url: &str,
    webhook_secret: &str,
) -> WebhookPayload {
    WebhookPayload::new(
        WebhookEventType::ToolCallStarted,
        agent_id,
        conv_id,
        data,
        webhook_url,
        webhook_secret,
    )
}

/// Create a webhook payload for a tool_call_completed event.
pub fn tool_call_completed(
    agent_id: &str,
    conv_id: &str,
    data: serde_json::Value,
    webhook_url: &str,
    webhook_secret: &str,
) -> WebhookPayload {
    WebhookPayload::new(
        WebhookEventType::ToolCallCompleted,
        agent_id,
        conv_id,
        data,
        webhook_url,
        webhook_secret,
    )
}

/// Create a webhook payload for a conversation_ended event.
pub fn conversation_ended(
    agent_id: &str,
    conv_id: &str,
    webhook_url: &str,
    webhook_secret: &str,
) -> WebhookPayload {
    WebhookPayload::new(
        WebhookEventType::ConversationEnded,
        agent_id,
        conv_id,
        json!({}),
        webhook_url,
        webhook_secret,
    )
}

/// Create a webhook payload for a todo_updated event.
pub fn todo_updated(
    agent_id: &str,
    conv_id: &str,
    data: serde_json::Value,
    webhook_url: &str,
    webhook_secret: &str,
) -> WebhookPayload {
    WebhookPayload::new(
        WebhookEventType::TodoUpdated,
        agent_id,
        conv_id,
        data,
        webhook_url,
        webhook_secret,
    )
}

/// Create a webhook payload for a turn_completed event.
pub fn turn_completed(
    agent_id: &str,
    conv_id: &str,
    webhook_url: &str,
    webhook_secret: &str,
) -> WebhookPayload {
    WebhookPayload::new(
        WebhookEventType::TurnCompleted,
        agent_id,
        conv_id,
        json!({}),
        webhook_url,
        webhook_secret,
    )
}

/// Create a webhook payload for a tool_approval_required event.
pub fn tool_approval_required(
    agent_id: &str,
    conv_id: &str,
    data: serde_json::Value,
    webhook_url: &str,
    webhook_secret: &str,
) -> WebhookPayload {
    WebhookPayload::new(
        WebhookEventType::ToolApprovalRequired,
        agent_id,
        conv_id,
        data,
        webhook_url,
        webhook_secret,
    )
}

/// Create a webhook payload for a tool_approval_resolved event.
pub fn tool_approval_resolved(
    agent_id: &str,
    conv_id: &str,
    data: serde_json::Value,
    webhook_url: &str,
    webhook_secret: &str,
) -> WebhookPayload {
    WebhookPayload::new(
        WebhookEventType::ToolApprovalResolved,
        agent_id,
        conv_id,
        data,
        webhook_url,
        webhook_secret,
    )
}

/// Create a webhook payload for an agent_error event.
pub fn agent_error(
    agent_id: &str,
    conv_id: &str,
    data: serde_json::Value,
    webhook_url: &str,
    webhook_secret: &str,
) -> WebhookPayload {
    WebhookPayload::new(
        WebhookEventType::AgentError,
        agent_id,
        conv_id,
        data,
        webhook_url,
        webhook_secret,
    )
}

#[cfg(test)]
mod tests {
    use super::*;

    const AGENT: &str = "agent-1";
    const CONV: &str = "conv-1";
    const URL: &str = "https://example.com/webhook";
    const SECRET: &str = "secret";

    #[test]
    fn test_conversation_created() {
        let payload = conversation_created(AGENT, CONV, URL, SECRET);
        assert_eq!(payload.event_type, WebhookEventType::ConversationCreated);
        assert_eq!(payload.agent_id, AGENT);
        assert_eq!(payload.conversation_id, CONV);
        assert_eq!(payload.webhook_url, URL);
        assert_eq!(payload.webhook_secret, SECRET);
        assert_eq!(payload.data, json!({}));
    }

    #[test]
    fn test_message_received() {
        let data = json!({"message": "hello"});
        let payload = message_received(AGENT, CONV, data.clone(), URL, SECRET);
        assert_eq!(payload.event_type, WebhookEventType::MessageReceived);
        assert_eq!(payload.agent_id, AGENT);
        assert_eq!(payload.conversation_id, CONV);
        assert_eq!(payload.data, data);
    }

    #[test]
    fn test_response_started() {
        let payload = response_started(AGENT, CONV, URL, SECRET);
        assert_eq!(payload.event_type, WebhookEventType::ResponseStarted);
        assert_eq!(payload.agent_id, AGENT);
        assert_eq!(payload.conversation_id, CONV);
        assert_eq!(payload.data, json!({}));
    }

    #[test]
    fn test_response_chunk() {
        let data = json!({"chunk": "partial"});
        let payload = response_chunk(AGENT, CONV, data.clone(), URL, SECRET);
        assert_eq!(payload.event_type, WebhookEventType::ResponseChunk);
        assert_eq!(payload.data, data);
    }

    #[test]
    fn test_response_completed() {
        let data = json!({"response": "done"});
        let payload = response_completed(AGENT, CONV, data.clone(), URL, SECRET);
        assert_eq!(payload.event_type, WebhookEventType::ResponseCompleted);
        assert_eq!(payload.data, data);
    }

    #[test]
    fn test_tool_call_started() {
        let data = json!({"tool": "search"});
        let payload = tool_call_started(AGENT, CONV, data.clone(), URL, SECRET);
        assert_eq!(payload.event_type, WebhookEventType::ToolCallStarted);
        assert_eq!(payload.data, data);
    }

    #[test]
    fn test_tool_call_completed() {
        let data = json!({"tool": "search", "result": "ok"});
        let payload = tool_call_completed(AGENT, CONV, data.clone(), URL, SECRET);
        assert_eq!(payload.event_type, WebhookEventType::ToolCallCompleted);
        assert_eq!(payload.data, data);
    }

    #[test]
    fn test_conversation_ended() {
        let payload = conversation_ended(AGENT, CONV, URL, SECRET);
        assert_eq!(payload.event_type, WebhookEventType::ConversationEnded);
        assert_eq!(payload.agent_id, AGENT);
        assert_eq!(payload.data, json!({}));
    }

    #[test]
    fn test_agent_error() {
        let data = json!({"error": "something went wrong"});
        let payload = agent_error(AGENT, CONV, data.clone(), URL, SECRET);
        assert_eq!(payload.event_type, WebhookEventType::AgentError);
        assert_eq!(payload.data, data);
    }

    #[test]
    fn test_todo_updated() {
        let data = json!({"todos": [{"content": "task 1", "status": "in_progress"}]});
        let payload = todo_updated(AGENT, CONV, data.clone(), URL, SECRET);
        assert_eq!(payload.event_type, WebhookEventType::TodoUpdated);
        assert_eq!(payload.data, data);
    }

    #[test]
    fn test_turn_completed() {
        let payload = turn_completed(AGENT, CONV, URL, SECRET);
        assert_eq!(payload.event_type, WebhookEventType::TurnCompleted);
        assert_eq!(payload.data, json!({}));
    }
}
