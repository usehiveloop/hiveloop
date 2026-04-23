use bridge_core::conversation::{Message, Role};
use bridge_core::event::{BridgeEvent, BridgeEventType};
use serde_json::json;
use std::sync::Arc;
use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;
use tools::agent::AgentTaskNotification;
use tracing::debug;
use webhooks::EventBus;

use super::convert::{extract_text_content, normalize_messages_for_persistence};
use super::params::IncomingMessage;

/// Push the final user message onto `history`, append the persisted-side
/// message to `persisted_messages`, replay the snapshot to storage (if any),
/// and emit the `ResponseStarted` event. Returns the `pre_turn_len` of the
/// persisted-messages guard before the push (needed for rollback on failure).
#[allow(clippy::too_many_arguments)]
pub(super) fn commit_user_turn(
    history: &mut Vec<rig::message::Message>,
    persisted_messages: &std::sync::Arc<std::sync::Mutex<Vec<Message>>>,
    final_user_text: &str,
    persisted_user_message: Message,
    storage: &Option<storage::StorageHandle>,
    conversation_id: &str,
    event_bus: &std::sync::Arc<webhooks::EventBus>,
    agent_id: &str,
    msg_id: &str,
) -> usize {
    history.push(rig::message::Message::user(final_user_text));
    let pre_turn_len = {
        let mut guard = persisted_messages.lock().unwrap();
        let len = guard.len();
        guard.push(persisted_user_message);
        len
    };

    // Persist immediately so the user message survives a crash
    if let Some(storage) = storage {
        storage.replace_messages(
            conversation_id.to_string(),
            persisted_messages.lock().unwrap().clone(),
        );
    }

    // Signal response starting
    event_bus.emit(bridge_core::event::BridgeEvent::new(
        bridge_core::event::BridgeEventType::ResponseStarted,
        agent_id,
        conversation_id,
        serde_json::json!({
            "conversation_id": conversation_id,
            "message_id": msg_id,
        }),
    ));

    pre_turn_len
}

/// Build the raw user text for this turn, optionally prepending a
/// pending tool-requirement reminder that was stashed on the previous turn.
/// The reminder is wrapped in `<system-reminder>` tags so the model treats
/// it as system-level guidance rather than user intent.
pub(super) fn build_user_text_with_pending(
    incoming: &IncomingMessage,
    pending_reminder: Option<String>,
    event_bus: &std::sync::Arc<webhooks::EventBus>,
    agent_id: &str,
    conversation_id: &str,
) -> String {
    let user_text = build_user_text_from_incoming(incoming, event_bus, agent_id, conversation_id);
    if let Some(reminder) = pending_reminder {
        format!("<system-reminder>\n{reminder}\n</system-reminder>\n\n{user_text}")
    } else {
        user_text
    }
}

/// Outcome of a single iteration of the channel-select.
pub(super) enum ReceiveOutcome {
    /// Normal incoming message — proceed with turn.
    Got(IncomingMessage),
    /// Loop should break (cancellation or channel closed).
    Break,
}

/// Wait for the next incoming signal (user message, background task
/// completion, ping timer, or cancellation). Mirrors the original
/// `tokio::select!` inside the loop verbatim.
pub(super) async fn receive_incoming(
    cancel: &CancellationToken,
    conversation_id: &str,
    message_rx: &mut mpsc::Receiver<Message>,
    notification_rx: &mut Option<mpsc::Receiver<AgentTaskNotification>>,
    ping_state: &Option<tools::ping_me_back::PingState>,
) -> ReceiveOutcome {
    tokio::select! {
        _ = cancel.cancelled() => {
            debug!(conversation_id = conversation_id, "conversation cancelled");
            ReceiveOutcome::Break
        }
        msg = message_rx.recv() => {
            match msg {
                Some(m) => ReceiveOutcome::Got(IncomingMessage::User(m)),
                None => {
                    debug!(conversation_id = conversation_id, "message channel closed");
                    ReceiveOutcome::Break
                }
            }
        }
        Some(notif) = async {
            match notification_rx.as_mut() {
                Some(rx) => rx.recv().await,
                None => std::future::pending().await,
            }
        } => {
            ReceiveOutcome::Got(IncomingMessage::BackgroundComplete(notif))
        }
        _ = async {
            match ping_state {
                Some(ps) => match ps.next_fire_time().await {
                    Some(instant) => tokio::time::sleep_until(instant).await,
                    None => std::future::pending().await,
                },
                None => std::future::pending().await,
            }
        } => {
            let fired = match ping_state {
                Some(ps) => ps.pop_fired().await,
                None => vec![],
            };
            ReceiveOutcome::Got(IncomingMessage::PingFired(fired))
        }
    }
}

fn text_message(role: Role, text: String) -> Message {
    Message {
        role,
        content: vec![bridge_core::conversation::ContentBlock::Text { text }],
        timestamp: chrono::Utc::now(),
        system_reminder: None,
    }
}

/// Build the raw user text (without system reminders) from the incoming
/// signal. Emits `BackgroundTaskCompleted` events when the signal is a
/// background subagent finishing.
pub(super) fn build_user_text_from_incoming(
    incoming: &IncomingMessage,
    event_bus: &Arc<EventBus>,
    agent_id: &str,
    conversation_id: &str,
) -> String {
    match incoming {
        IncomingMessage::User(msg) => extract_text_content(msg),
        IncomingMessage::BackgroundComplete(notif) => {
            let task_id = notif.task_id.clone();
            let description = notif.description.clone();
            let is_error = notif.output.is_err();
            let output_text = match &notif.output {
                Ok(output) => output.clone(),
                Err(error) => format!("[ERROR] {}", error),
            };

            // Emit background task completion event
            event_bus.emit(BridgeEvent::new(
                BridgeEventType::BackgroundTaskCompleted,
                agent_id,
                conversation_id,
                json!({
                    "task_id": task_id,
                    "description": description,
                    "output": output_text,
                    "is_error": is_error,
                }),
            ));

            format!(
                "[Background Agent Task Completed]\ntask_id: {}\ndescription: {}\n\n<task_result>\n{}\n</task_result>",
                task_id,
                description,
                output_text,
            )
        }
        IncomingMessage::PingFired(pings) => {
            let mut parts = Vec::new();
            for ping in pings {
                parts.push(format!(
                    "[Ping-Me-Back Fired]\nYou are being pinged back because you asked to be pinged back. It has been {} seconds since then.\n\nYour message: {}",
                    ping.delay_secs, ping.message
                ));
            }
            parts.join("\n\n")
        }
    }
}

/// Given the raw `user_text` and incoming signal, build the bridge_core
/// `Message` that represents the user's side of this turn for persistence.
pub(super) fn build_persisted_user_message(incoming: &IncomingMessage, user_text: &str) -> Message {
    match incoming {
        IncomingMessage::User(msg) => normalize_messages_for_persistence(std::slice::from_ref(msg))
            .into_iter()
            .next()
            .unwrap_or_else(|| msg.clone()),
        IncomingMessage::BackgroundComplete(_) | IncomingMessage::PingFired(_) => {
            text_message(Role::User, user_text.to_string())
        }
    }
}
