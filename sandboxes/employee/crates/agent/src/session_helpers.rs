use std::sync::Arc;

use adk_rust::prelude::*;
use adk_rust::session::{CreateRequest, GetRequest, SessionService};
use adk_rust::Event;

use crate::{AgentError, HistoryEntry, HistoryRole, Result};

pub async fn session_contains_image_parts(
    svc: &Arc<dyn SessionService>,
    app_name: &str,
    user_id: &str,
    session_id: &str,
) -> bool {
    let req = GetRequest {
        app_name: app_name.into(),
        user_id: user_id.into(),
        session_id: session_id.into(),
        after: None,
        num_recent_events: None,
    };
    let Ok(session) = svc.get(req).await else {
        return false;
    };
    session.events().all().iter().any(|event| {
        event
            .llm_response
            .content
            .as_ref()
            .map(|content| {
                content.parts.iter().any(|part| {
                    matches!(part, Part::InlineData { .. } | Part::FileData { .. })
                })
            })
            .unwrap_or(false)
    })
}

pub async fn ensure_session(
    svc: &Arc<dyn SessionService>,
    app_name: &str,
    user_id: &str,
    session_id: &str,
) -> Result<bool> {
    let get_req = GetRequest {
        app_name: app_name.into(),
        user_id: user_id.into(),
        session_id: session_id.into(),
        after: None,
        num_recent_events: None,
    };
    if svc.get(get_req).await.is_ok() {
        return Ok(false);
    }
    let create_req = CreateRequest {
        app_name: app_name.into(),
        user_id: user_id.into(),
        session_id: Some(session_id.into()),
        state: Default::default(),
    };
    svc.create(create_req)
        .await
        .map_err(|e| AgentError::Other(anyhow::anyhow!("session create: {e}")))?;
    Ok(true)
}

pub async fn session_event_count(
    svc: &Arc<dyn SessionService>,
    app_name: &str,
    user_id: &str,
    session_id: &str,
) -> Result<usize> {
    let get_req = GetRequest {
        app_name: app_name.into(),
        user_id: user_id.into(),
        session_id: session_id.into(),
        after: None,
        num_recent_events: None,
    };
    let session = svc
        .get(get_req)
        .await
        .map_err(|e| AgentError::Other(anyhow::anyhow!("session get: {e}")))?;
    Ok(session.events().all().len())
}

pub async fn seed_history_into_session(
    svc: &Arc<dyn SessionService>,
    session_id: &str,
    agent_name: &str,
    history: &[HistoryEntry],
) -> Result<()> {
    for (index, entry) in history.iter().enumerate() {
        let invocation_id = format!("replay-{index}");
        let mut event = Event::new(invocation_id);
        match entry.role {
            HistoryRole::User => {
                event.author = "user".to_string();
                let speaker_label = match entry.speaker_display_name.as_deref() {
                    Some(name) if !name.is_empty() => format!("{}/{}", entry.speaker_id, name),
                    _ => entry.speaker_id.clone(),
                };
                let formatted = format!("[{speaker_label}]: {}", entry.text);
                event.llm_response.content = Some(Content::new("user").with_text(formatted));
            }
            HistoryRole::Assistant => {
                event.author = agent_name.to_string();
                event.llm_response.content =
                    Some(Content::new("model").with_text(entry.text.clone()));
            }
        }
        event.llm_response.partial = false;
        svc.append_event(session_id, event)
            .await
            .map_err(|e| AgentError::Other(anyhow::anyhow!("append_event during seed: {e}")))?;
    }
    Ok(())
}

fn extract_text_from_event(event: &Event) -> String {
    let Some(content) = event.llm_response.content.as_ref() else {
        return String::new();
    };
    content
        .parts
        .iter()
        .filter_map(|part| match part {
            Part::Text { text } => Some(text.clone()),
            _ => None,
        })
        .collect()
}

fn role_label_for_event(event: &Event, agent_name: &str) -> &'static str {
    if event.author == "user" {
        "user"
    } else if event.author == agent_name {
        "assistant"
    } else {
        "assistant"
    }
}

pub async fn log_full_conversation(
    svc: &Arc<dyn SessionService>,
    app_name: &str,
    user_id: &str,
    session_id: &str,
    system_prompt: &str,
    agent_name: &str,
    upcoming_user_text: &str,
    upcoming_image_count: usize,
) {
    let req = GetRequest {
        app_name: app_name.into(),
        user_id: user_id.into(),
        session_id: session_id.into(),
        after: None,
        num_recent_events: None,
    };
    let session = match svc.get(req).await {
        Ok(s) => s,
        Err(e) => {
            tracing::warn!(error = %e, "could not load session for conversation log");
            return;
        }
    };
    let events = session.events().all();

    let mut transcript = String::new();
    transcript.push_str("\n========== CONVERSATION SENT TO MODEL ==========\n");
    transcript.push_str(&format!("[system] {system_prompt}\n"));
    for event in events.iter() {
        let role = role_label_for_event(event, agent_name);
        let text = extract_text_from_event(event);
        if text.is_empty() {
            continue;
        }
        transcript.push_str(&format!("[{role}] {text}\n"));
    }
    let upcoming_image_note = if upcoming_image_count > 0 {
        format!(" (+ {upcoming_image_count} image attachment(s))")
    } else {
        String::new()
    };
    transcript.push_str(&format!(
        "[user] {upcoming_user_text}{upcoming_image_note}\n"
    ));
    transcript.push_str("================================================");

    tracing::info!(session_id = %session_id, "{}", transcript);
}
