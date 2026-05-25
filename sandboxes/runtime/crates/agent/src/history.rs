use domain::{EventKind, SessionEvent, SessionId};
use serde::{Deserialize, Serialize};
use storage::EventRepo;

use crate::primitives::{AgentMessage, AgentMessageRole};
use crate::{HistoryEntry, HistoryRole, Result};

const HISTORY_PAYLOAD_VERSION: u32 = 1;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ModelHistoryPayload {
    pub version: u32,
    pub message: AgentMessage,
}

pub async fn load_model_history(
    repo: Option<&dyn EventRepo>,
    session_id: &SessionId,
    limit: u32,
) -> Result<Vec<AgentMessage>> {
    let Some(repo) = repo else {
        return Ok(Vec::new());
    };
    let events = repo
        .list_chronological(session_id, limit)
        .await
        .map_err(|e| crate::AgentError::Other(anyhow::anyhow!(e)))?;
    Ok(events.into_iter().filter_map(message_from_event).collect())
}

pub async fn append_model_message(
    repo: Option<&dyn EventRepo>,
    session_id: &SessionId,
    message: &AgentMessage,
) -> Result<()> {
    let Some(repo) = repo else {
        return Ok(());
    };
    let payload = serde_json::to_value(ModelHistoryPayload {
        version: HISTORY_PAYLOAD_VERSION,
        message: message.clone(),
    })
    .map_err(|e| crate::AgentError::Other(anyhow::anyhow!(e)))?;
    repo.append(session_id, event_kind_for_message(message), payload)
        .await
        .map_err(|e| crate::AgentError::Other(anyhow::anyhow!(e)))?;
    Ok(())
}

pub async fn seed_model_history_from_gateway(
    repo: Option<&dyn EventRepo>,
    session_id: &SessionId,
    history: &[HistoryEntry],
) -> Result<Vec<AgentMessage>> {
    let mut messages = Vec::new();
    for entry in history {
        let message = match entry.role {
            HistoryRole::User => AgentMessage::user(format_history_user(
                entry.speaker_id.clone(),
                entry.speaker_display_name.clone(),
                entry.text.clone(),
            )),
            HistoryRole::Assistant => AgentMessage::assistant(entry.text.clone()),
        };
        append_model_message(repo, session_id, &message).await?;
        messages.push(message);
    }
    Ok(messages)
}

pub fn visible_messages_from_gateway(history: Vec<HistoryEntry>) -> Vec<AgentMessage> {
    history
        .into_iter()
        .map(|entry| match entry.role {
            HistoryRole::User => AgentMessage::user(format_history_user(
                entry.speaker_id,
                entry.speaker_display_name,
                entry.text,
            )),
            HistoryRole::Assistant => AgentMessage::assistant(entry.text),
        })
        .collect()
}

fn message_from_event(event: SessionEvent) -> Option<AgentMessage> {
    let payload: ModelHistoryPayload = serde_json::from_value(event.payload).ok()?;
    if payload.version != HISTORY_PAYLOAD_VERSION {
        return None;
    }
    Some(payload.message)
}

fn event_kind_for_message(message: &AgentMessage) -> EventKind {
    match message.role {
        AgentMessageRole::User => EventKind::UserMessage,
        AgentMessageRole::Assistant if !message.tool_calls.is_empty() => EventKind::ToolCall,
        AgentMessageRole::Assistant => EventKind::AssistantMessage,
        AgentMessageRole::Tool => EventKind::ToolResult,
        AgentMessageRole::System => EventKind::AssistantMessage,
    }
}

fn format_history_user(user_id: String, name: Option<String>, text: String) -> String {
    let user_id = user_id.trim();
    match name.map(|name| name.trim().to_string()) {
        Some(name) if !name.is_empty() && !user_id.is_empty() => {
            format!("{name} ({user_id}): {text}")
        }
        Some(name) if !name.is_empty() => format!("{name}: {text}"),
        _ if !user_id.is_empty() && user_id != "cron" && user_id != "bot" => {
            format!("{user_id}: {text}")
        }
        _ => text,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::primitives::ToolCall;
    use chrono::Utc;
    use std::sync::Arc;
    use tokio::sync::Mutex;

    #[derive(Default)]
    struct MemoryEventRepo {
        events: Mutex<Vec<SessionEvent>>,
    }

    #[async_trait::async_trait]
    impl EventRepo for MemoryEventRepo {
        async fn append(
            &self,
            session_id: &SessionId,
            kind: EventKind,
            payload: serde_json::Value,
        ) -> storage::Result<i64> {
            let mut events = self.events.lock().await;
            let id = events.len() as i64 + 1;
            events.push(SessionEvent {
                id,
                session_id: session_id.clone(),
                seq: id,
                kind,
                payload,
                created_at: Utc::now(),
            });
            Ok(id)
        }

        async fn append_idempotent(
            &self,
            session_id: &SessionId,
            kind: EventKind,
            payload: serde_json::Value,
            _idempotency_key: &str,
        ) -> storage::Result<Option<i64>> {
            self.append(session_id, kind, payload).await.map(Some)
        }

        async fn list_recent(
            &self,
            session_id: &SessionId,
            _limit: u32,
        ) -> storage::Result<Vec<SessionEvent>> {
            let mut events: Vec<_> = self
                .events
                .lock()
                .await
                .iter()
                .filter(|event| &event.session_id == session_id)
                .cloned()
                .collect();
            events.reverse();
            Ok(events)
        }

        async fn list_chronological(
            &self,
            session_id: &SessionId,
            limit: u32,
        ) -> storage::Result<Vec<SessionEvent>> {
            let mut events = self.list_recent(session_id, limit).await?;
            events.reverse();
            Ok(events)
        }

        async fn search_sessions(
            &self,
            _query: &str,
            _session_id: Option<&SessionId>,
            _limit: u32,
        ) -> storage::Result<Vec<storage::SessionSearchResult>> {
            Ok(Vec::new())
        }
    }

    #[tokio::test]
    async fn history_round_trips_tool_messages() {
        let repo = Arc::new(MemoryEventRepo::default());
        let session_id = SessionId::from("s1");
        let user = AgentMessage::user("hello");
        let calls = AgentMessage::assistant_tool_calls(vec![ToolCall {
            id: "call_1".into(),
            name: "lookup".into(),
            arguments: serde_json::json!({"q":"x"}),
        }]);
        let result = AgentMessage::tool_result("call_1", "{\"ok\":true}");
        let final_message = AgentMessage::assistant("done");

        for message in [&user, &calls, &result, &final_message] {
            append_model_message(Some(repo.as_ref()), &session_id, message)
                .await
                .unwrap();
        }

        let loaded = load_model_history(Some(repo.as_ref()), &session_id, 100)
            .await
            .unwrap();
        assert_eq!(loaded.len(), 4);
        assert_eq!(loaded[0].role, AgentMessageRole::User);
        assert_eq!(loaded[1].role, AgentMessageRole::Assistant);
        assert_eq!(loaded[1].tool_calls[0].name, "lookup");
        assert_eq!(loaded[2].role, AgentMessageRole::Tool);
        assert_eq!(loaded[3].role, AgentMessageRole::Assistant);
    }

    #[tokio::test]
    async fn gateway_seed_only_contains_visible_roles() {
        let repo = Arc::new(MemoryEventRepo::default());
        let session_id = SessionId::from("s2");
        let seeded = seed_model_history_from_gateway(
            Some(repo.as_ref()),
            &session_id,
            &[
                HistoryEntry {
                    role: HistoryRole::User,
                    speaker_id: "U123".into(),
                    speaker_display_name: Some("Kim".into()),
                    text: "hi".into(),
                },
                HistoryEntry {
                    role: HistoryRole::Assistant,
                    speaker_id: "bot".into(),
                    speaker_display_name: None,
                    text: "hello".into(),
                },
            ],
        )
        .await
        .unwrap();

        assert_eq!(seeded.len(), 2);
        let seeded_text = match &seeded[0].parts[0] {
            crate::primitives::MessagePart::Text { text } => text,
            _ => panic!("expected text"),
        };
        assert_eq!(seeded_text, "Kim (U123): hi");
        assert_eq!(
            load_model_history(Some(repo.as_ref()), &session_id, 100)
                .await
                .unwrap()
                .len(),
            2
        );
    }

    #[tokio::test]
    async fn next_turn_preserves_tool_prefix_before_new_user_message() {
        let repo = Arc::new(MemoryEventRepo::default());
        let session_id = SessionId::from("s3");
        let first_user = AgentMessage::user("find issues");
        let calls = AgentMessage::assistant_tool_calls(vec![ToolCall {
            id: "tool_1".into(),
            name: "linear_list_issues".into(),
            arguments: serde_json::json!({"team":"PSL"}),
        }]);
        let tool_result = AgentMessage::tool_result("tool_1", "{\"issues\":[\"A\"]}");
        let final_reply = AgentMessage::assistant("Found one issue.");

        for message in [&first_user, &calls, &tool_result, &final_reply] {
            append_model_message(Some(repo.as_ref()), &session_id, message)
                .await
                .unwrap();
        }

        let mut second_turn = load_model_history(Some(repo.as_ref()), &session_id, 100)
            .await
            .unwrap();
        second_turn.push(AgentMessage::user("tell me more"));

        assert_eq!(second_turn[0].role, AgentMessageRole::User);
        assert_eq!(second_turn[1].role, AgentMessageRole::Assistant);
        assert_eq!(second_turn[1].tool_calls[0].name, "linear_list_issues");
        assert_eq!(second_turn[2].role, AgentMessageRole::Tool);
        assert_eq!(second_turn[4].role, AgentMessageRole::User);
    }

    #[test]
    fn visible_gateway_history_keeps_user_id_with_name() {
        let messages = visible_messages_from_gateway(vec![HistoryEntry {
            role: HistoryRole::User,
            speaker_id: "U08P1G9EDNG".into(),
            speaker_display_name: Some("Nora".into()),
            text: "Loop me in on invoice failures.".into(),
        }]);
        let text = match &messages[0].parts[0] {
            crate::primitives::MessagePart::Text { text } => text,
            _ => panic!("expected text"),
        };
        assert_eq!(text, "Nora (U08P1G9EDNG): Loop me in on invoice failures.");
    }
}
