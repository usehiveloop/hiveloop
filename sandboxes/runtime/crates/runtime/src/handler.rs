#![allow(clippy::items_after_test_module)]

mod composition;
mod media;
mod session;

use std::sync::Arc;

use agent::{AgentEvent, AgentRunner, TurnInput};
use anyhow::Result;
use async_trait::async_trait;
use chrono::Utc;
use domain::{
    event_types, ConfigStore, InboundEvent, OutboundEvent, Reply, SessionId, SessionStatus,
};
use futures::StreamExt;
use gateway::ChannelGateway;
use outbound::OutboundEmitter;
use serde_json::Value;
use storage::SessionRepo;
use tracing::{info, warn};

use composition::compose_annotated_text;
use media::{collect_media_for_turn, DownloadResults};
use session::ensure_session_persisted;

use crate::session_coordinator::{SessionCoordinator, Submission};

const GENERIC_ERROR_REPLY: &str =
    "Sorry, something went wrong while generating that response. Please try again.";

#[async_trait]
pub trait TurnEventSink: Send + Sync + 'static {
    async fn activate_session_stream(&self, _session_id: &SessionId, _stream_id: &str) {}

    async fn clear_active_session_stream(&self, _session_id: &SessionId, _stream_id: &str) {}

    async fn publish_final(&self, _stream_id: &str, _session_id: &SessionId, _text: &str) {}

    async fn publish_agent_event(
        &self,
        stream_id: &str,
        session_id: &SessionId,
        event: &AgentEvent,
    );
}

#[async_trait]
impl TurnEventSink for api::HttpStreamBroker {
    async fn activate_session_stream(&self, session_id: &SessionId, stream_id: &str) {
        self.activate_session_stream(session_id.as_str(), stream_id)
            .await;
    }

    async fn clear_active_session_stream(&self, session_id: &SessionId, stream_id: &str) {
        self.clear_active_session_stream(session_id.as_str(), stream_id)
            .await;
    }

    async fn publish_final(&self, stream_id: &str, session_id: &SessionId, text: &str) {
        self.publish(
            stream_id,
            "final",
            serde_json::json!({
                "session_id": session_id.as_str(),
                "text": text,
            }),
        )
        .await;
        self.publish(
            stream_id,
            "done",
            serde_json::json!({
                "session_id": session_id.as_str(),
            }),
        )
        .await;
    }

    async fn publish_agent_event(
        &self,
        stream_id: &str,
        session_id: &SessionId,
        event: &AgentEvent,
    ) {
        match event {
            AgentEvent::ThinkingChunk { text } => {
                self.publish(
                    stream_id,
                    "thinking",
                    serde_json::json!({
                        "session_id": session_id.as_str(),
                        "text": text,
                    }),
                )
                .await;
            }
            AgentEvent::TokenChunk { text } => {
                self.publish(
                    stream_id,
                    "token",
                    serde_json::json!({
                        "session_id": session_id.as_str(),
                        "text": text,
                    }),
                )
                .await;
            }
            AgentEvent::ToolCall { id, tool, args } => {
                self.publish(
                    stream_id,
                    "tool_call",
                    serde_json::json!({
                        "session_id": session_id.as_str(),
                        "id": id,
                        "tool": tool,
                        "args": args,
                    }),
                )
                .await;
            }
            AgentEvent::ToolResult { id, result } => {
                self.publish(
                    stream_id,
                    "tool_result",
                    serde_json::json!({
                        "session_id": session_id.as_str(),
                        "id": id,
                        "result": result,
                    }),
                )
                .await;
            }
            AgentEvent::RunEvent { event, payload } => {
                self.publish(stream_id, event, payload.clone()).await;
            }
            AgentEvent::Error { message } => {
                self.publish(
                    stream_id,
                    "error",
                    serde_json::json!({
                        "session_id": session_id.as_str(),
                        "message": message,
                    }),
                )
                .await;
            }
            AgentEvent::FinalMessage { .. } => {}
        }
    }
}

#[allow(clippy::too_many_arguments)]
pub async fn handle_inbound(
    runner: Arc<dyn AgentRunner>,
    gateway: Arc<dyn ChannelGateway>,
    config: ConfigStore,
    emitter: Arc<OutboundEmitter>,
    session_repo: Arc<dyn SessionRepo>,
    coordinator: Arc<SessionCoordinator>,
    turn_event_sink: Arc<dyn TurnEventSink>,
    inbound: InboundEvent,
) -> Result<()> {
    let submission = coordinator.submit_or_queue(inbound.clone());
    if matches!(submission, Submission::Queued) {
        let event_source = inbound_event_source(&inbound);
        emit_user_message_received(&emitter, &inbound, event_source, true).await;
        if let Some(stream_id) = session_stream_id(&inbound) {
            turn_event_sink
                .publish_final(
                    &stream_id,
                    &inbound.session_id,
                    "Queued. I will process this after the current turn finishes.",
                )
                .await;
        }
        return Ok(());
    }

    let mut current_inbound = inbound;
    loop {
        process_single_turn(
            runner.clone(),
            gateway.clone(),
            config.clone(),
            emitter.clone(),
            session_repo.clone(),
            &current_inbound,
            turn_event_sink.clone(),
        )
        .await?;

        let follow_ups = coordinator.finish_turn(&current_inbound.session_id);
        if follow_ups.is_empty() {
            break;
        }

        current_inbound = merge_queued_inbound(&current_inbound, follow_ups);
        coordinator.reserve(&current_inbound.session_id);
    }

    Ok(())
}

fn merge_queued_inbound(current: &InboundEvent, queued: Vec<InboundEvent>) -> InboundEvent {
    let mut merged = current.clone();
    let mut text =
        String::from("[Additional request(s) received while working on the previous task]\n");
    let mut attachments = Vec::new();
    let mut raw_events = Vec::new();

    for (index, event) in queued.into_iter().enumerate() {
        let number = index + 1;
        let source = inbound_event_source(&event);
        let display_name = event
            .user_display_name
            .clone()
            .unwrap_or_else(|| event.user.clone());
        text.push_str(&format!(
            "\n{number}. Source: {source}; User: {display_name} ({})\n",
            event.user
        ));
        if !event.text.trim().is_empty() {
            text.push_str("   Text:\n");
            for line in event.text.lines() {
                text.push_str("   ");
                text.push_str(line);
                text.push('\n');
            }
        }
        if !event.attachments.is_empty() {
            text.push_str("   Attachments:\n");
            for attachment in &event.attachments {
                text.push_str(&format!(
                    "   - {} ({}, {} bytes)\n",
                    attachment.name,
                    attachment.mime_type,
                    attachment.size_bytes.unwrap_or_default()
                ));
            }
        }
        attachments.extend(event.attachments.clone());
        raw_events.push(serde_json::json!({
            "envelope_id": event.envelope_id,
            "source": source,
            "user": event.user,
            "user_display_name": event.user_display_name,
            "raw": event.raw,
        }));
    }

    merged.envelope_id = format!("{}-queued", current.envelope_id);
    merged.user = "queued-inbound".to_string();
    merged.user_display_name = Some("Queued inbound messages".to_string());
    merged.text = text;
    merged.attachments = attachments;
    merged.raw = serde_json::json!({
        "source": "queued_batch",
        "events": raw_events,
    });
    merged.inbound_handle.ts = merged.envelope_id.clone();
    merged
}

#[cfg(test)]
mod queue_tests {
    use super::*;
    use domain::{reply::MessageHandle, Attachment};

    fn inbound(session: &str, envelope: &str, text: &str, raw: serde_json::Value) -> InboundEvent {
        InboundEvent {
            envelope_id: envelope.to_string(),
            session_id: SessionId::from(session.to_string()),
            user: "U123".to_string(),
            user_display_name: Some("Ada".to_string()),
            text: text.to_string(),
            attachments: vec![Attachment {
                url: "https://example.test/evidence.txt".to_string(),
                mime_type: "text/plain".to_string(),
                name: "evidence.txt".to_string(),
                size_bytes: Some(128),
            }],
            raw,
            inbound_handle: MessageHandle {
                channel: "C123".to_string(),
                ts: envelope.to_string(),
            },
            is_direct_message: false,
            is_directly_addressed: true,
            link_previews: Vec::new(),
        }
    }

    #[test]
    fn merged_queued_turn_preserves_event_metadata_and_attachments() {
        let current = inbound(
            "C123-T1",
            "E1",
            "working",
            serde_json::json!({"source": "http"}),
        );
        let queued = vec![
            inbound(
                "C123-T1",
                "E2",
                "first follow-up",
                serde_json::json!({
                    "source": "trigger",
                    "refs": {"issue": "123"},
                    "summary_refs": {"title": "Bug"}
                }),
            ),
            inbound(
                "C123-T1",
                "E3",
                "second follow-up",
                serde_json::json!({"source": "http"}),
            ),
        ];

        let merged = merge_queued_inbound(&current, queued);

        assert_eq!(merged.session_id.as_str(), "C123-T1");
        assert_eq!(merged.user, "queued-inbound");
        assert_eq!(merged.attachments.len(), 2);
        assert!(merged.text.contains("1. Source: trigger; User: Ada (U123)"));
        assert!(merged.text.contains("first follow-up"));
        assert!(merged.text.contains("evidence.txt (text/plain, 128 bytes)"));
        assert_eq!(merged.raw["source"], "queued_batch");
        assert_eq!(merged.raw["events"][0]["raw"]["refs"]["issue"], "123");
        assert_eq!(
            merged.raw["events"][0]["raw"]["summary_refs"]["title"],
            "Bug"
        );
        assert_eq!(merged.inbound_handle.ts, "E1-queued");
    }

    #[test]
    fn gateway_inbound_source_and_final_metadata_are_preserved() {
        let inbound = inbound(
            "http-gateway-conversation",
            "E1",
            "hello",
            serde_json::json!({
                "source": "gateway",
                "http_stream_id": "stream-1",
                "trace_id": "trace-1",
                "turn_id": "turn-1",
                "raw": {
                    "provider": "fake-slack",
                    "route_id": "route-1",
                    "thread_key": "fake-slack:T123:C123:100.000",
                    "channel_id": "C123",
                    "thread_id": "100.000"
                }
            }),
        );
        let mut payload = serde_json::json!({
            "session_id": inbound.session_id.as_str(),
            "source": inbound_event_source(&inbound),
            "text": "done"
        });

        copy_inbound_metadata(&mut payload, &inbound);

        assert_eq!(payload["source"], "gateway");
        assert_eq!(payload["trace_id"], "trace-1");
        assert_eq!(payload["turn_id"], "turn-1");
        assert_eq!(payload["provider"], "fake-slack");
        assert_eq!(payload["channel_id"], "C123");
        assert_eq!(payload["thread_id"], "100.000");
    }
}

async fn process_single_turn(
    runner: Arc<dyn AgentRunner>,
    gateway: Arc<dyn ChannelGateway>,
    config: ConfigStore,
    emitter: Arc<OutboundEmitter>,
    session_repo: Arc<dyn SessionRepo>,
    inbound: &InboundEvent,
    turn_event_sink: Arc<dyn TurnEventSink>,
) -> Result<()> {
    if inbound.text.trim().is_empty() && inbound.attachments.is_empty() {
        return Ok(());
    }
    info!(
        session = %inbound.session_id,
        user = %inbound.user,
        text_len = inbound.text.len(),
        attachments = inbound.attachments.len(),
        "inbound message"
    );

    let snapshot = config.snapshot();
    let session_id = inbound.session_id.clone();

    let was_new_session = ensure_session_persisted(session_repo.as_ref(), inbound, &emitter).await;
    if !was_new_session {
        let _ = session_repo.touch(&session_id, Utc::now()).await;
    }
    let event_source = inbound_event_source(inbound);
    emit_user_message_received(&emitter, inbound, event_source, false).await;

    let multimodal_available = snapshot.multimodal_model.is_some();
    let media =
        collect_media_for_turn(gateway.as_ref(), &inbound.attachments, multimodal_available).await;
    let annotated_text = compose_annotated_text(inbound, &media);

    let DownloadResults { images, .. } = media;
    let mut turn_input = TurnInput::text(annotated_text);
    for (mime, bytes) in images {
        turn_input = turn_input.with_image(mime, bytes);
    }

    let http_stream_id = session_stream_id(inbound);
    if let Some(stream_id) = http_stream_id.as_deref() {
        turn_event_sink
            .activate_session_stream(&session_id, stream_id)
            .await;
    }

    let stream_result = runner.run_turn(&session_id, turn_input).await;
    let outcome = match stream_result {
        Ok(stream) => {
            consume_agent_stream(
                stream,
                http_stream_id.clone(),
                &session_id,
                &emitter,
                event_source,
                turn_event_sink.as_ref(),
            )
            .await
        }
        Err(e) => StreamOutcome {
            text: String::new(),
            error: Some(e.to_string()),
        },
    };

    let final_text = format_final_message(&outcome);
    let reply_text_for_event = final_text.clone();
    if let Some(stream_id) = http_stream_id.as_deref() {
        turn_event_sink
            .publish_final(stream_id, &session_id, &final_text)
            .await;
    } else {
        if let Err(e) = gateway.reply(&session_id, Reply::Text(final_text)).await {
            warn!(error = %e, "reply failed");
        }
    }

    if let Some(error_message) = outcome.error.as_ref() {
        emitter
            .emit(OutboundEvent::new(
                event_types::ERROR_MODEL,
                serde_json::json!({
                    "session_id": session_id.as_str(),
                    "source": event_source,
                    "error": error_message,
                }),
            ))
            .await;
        let _ = session_repo
            .set_status(&session_id, SessionStatus::Errored)
            .await;
    } else {
        let mut payload = serde_json::json!({
            "session_id": session_id.as_str(),
            "source": event_source,
            "text": reply_text_for_event,
        });
        copy_inbound_metadata(&mut payload, inbound);
        emitter
            .emit(OutboundEvent::new(event_types::AGENT_MESSAGE_SENT, payload))
            .await;
    }

    if let Some(stream_id) = http_stream_id.as_deref() {
        turn_event_sink
            .clear_active_session_stream(&session_id, stream_id)
            .await;
    }

    info!(session = %session_id, len = outcome.text.len(), "turn complete");
    Ok(())
}

async fn emit_user_message_received(
    emitter: &OutboundEmitter,
    inbound: &InboundEvent,
    source: &'static str,
    queued: bool,
) {
    let (channel, thread_ts) = derive_channel_and_thread(&inbound.session_id);
    emitter
        .emit(OutboundEvent::new(
            event_types::USER_MESSAGE_RECEIVED,
            serde_json::json!({
                "envelope_id": inbound.envelope_id,
                "session_id": inbound.session_id.as_str(),
                "source": source,
                "channel": channel,
                "thread_ts": thread_ts,
                "user": inbound.user,
                "user_display_name": inbound.user_display_name,
                "text": inbound.text,
                "attachments": inbound.attachments.iter().map(|attachment| {
                    serde_json::json!({
                        "name": attachment.name,
                        "mime_type": attachment.mime_type,
                        "size_bytes": attachment.size_bytes,
                    })
                }).collect::<Vec<_>>(),
                "link_previews": inbound.link_previews,
                "is_direct_message": inbound.is_direct_message,
                "is_directly_addressed": inbound.is_directly_addressed,
                "queued": queued,
            }),
        ))
        .await;
}

fn inbound_event_source(inbound: &InboundEvent) -> &'static str {
    match inbound
        .raw
        .get("source")
        .and_then(|value| value.as_str())
        .unwrap_or_default()
    {
        "http" => "http",
        "trigger" => "trigger",
        "cron" => "cron",
        "gateway" => "gateway",
        "specialist_callback" => "specialist_callback",
        _ if inbound.session_id.as_str().starts_with("http-") => "http",
        _ => "http",
    }
}

fn copy_inbound_metadata(payload: &mut Value, inbound: &InboundEvent) {
    let Some(map) = payload.as_object_mut() else {
        return;
    };
    for key in [
        "http_stream_id",
        "trace_id",
        "turn_id",
        "conversation_id",
        "provider",
        "route_id",
        "employee_session_id",
        "gateway_event_id",
        "dedupe_key",
        "thread_key",
        "channel_id",
        "thread_id",
        "external_message_id",
        "callback_url",
    ] {
        if let Some(value) = inbound.raw.get(key) {
            map.insert(key.to_string(), value.clone());
        }
        if let Some(value) = inbound
            .raw
            .get("raw")
            .and_then(Value::as_object)
            .and_then(|raw| raw.get(key))
        {
            map.insert(key.to_string(), value.clone());
        }
    }
}

fn derive_channel_and_thread(session_id: &SessionId) -> (String, String) {
    let raw = session_id.as_str();
    match raw.split_once('-') {
        Some((channel, thread_ts)) => (channel.to_string(), thread_ts.to_string()),
        None => (raw.to_string(), String::new()),
    }
}

pub(crate) struct StreamOutcome {
    pub text: String,
    pub error: Option<String>,
}

async fn consume_agent_stream(
    mut stream: futures::stream::BoxStream<'static, AgentEvent>,
    stream_id: Option<String>,
    session_id: &SessionId,
    emitter: &OutboundEmitter,
    source: &'static str,
    event_sink: &dyn TurnEventSink,
) -> StreamOutcome {
    let mut accumulated = String::new();
    let mut error_message: Option<String> = None;
    let mut sequence: u64 = 0;
    while let Some(event) = stream.next().await {
        sequence += 1;
        emit_agent_stream_event(emitter, session_id, source, sequence, &event).await;
        if let Some(stream_id) = stream_id.as_deref() {
            event_sink
                .publish_agent_event(stream_id, session_id, &event)
                .await;
        }
        match event {
            AgentEvent::ThinkingChunk { .. } => {}
            AgentEvent::TokenChunk { text } => accumulated.push_str(&text),
            AgentEvent::FinalMessage { text } => accumulated = text,
            AgentEvent::Error { message } => error_message = Some(message),
            AgentEvent::RunEvent { .. } => {}
            _ => {}
        }
    }
    emitter.flush_streams_for_session(session_id.as_str()).await;
    StreamOutcome {
        text: accumulated,
        error: error_message,
    }
}

async fn emit_agent_stream_event(
    emitter: &OutboundEmitter,
    session_id: &SessionId,
    source: &'static str,
    sequence: u64,
    event: &AgentEvent,
) {
    let agent_event = serde_json::to_value(event).unwrap_or_else(|_| {
        serde_json::json!({
            "kind": "serialization_error",
        })
    });
    let event_type = match event {
        AgentEvent::ThinkingChunk { .. } => "agent.stream.thinking",
        AgentEvent::TokenChunk { .. } => "agent.stream.token",
        AgentEvent::ToolCall { .. } => "agent.tool.call",
        AgentEvent::ToolResult { .. } => "agent.tool.result",
        AgentEvent::RunEvent { event: name, .. } => {
            let sanitized = name
                .chars()
                .map(|ch| match ch {
                    'a'..='z' | '0'..='9' | '_' | '-' => ch,
                    'A'..='Z' => ch.to_ascii_lowercase(),
                    _ => '.',
                })
                .collect::<String>()
                .replace('_', ".");
            let event_type = format!("agent.run.{sanitized}");
            let payload = serde_json::json!({
                "session_id": session_id.as_str(),
                "source": source,
                "sequence": sequence,
                "agent_event": agent_event,
            });
            emitter.emit(OutboundEvent::new(event_type, payload)).await;
            return;
        }
        AgentEvent::FinalMessage { .. } => "agent.final_message",
        AgentEvent::Error { .. } => "agent.error",
    };
    emitter
        .emit(OutboundEvent::new(
            event_type,
            serde_json::json!({
                "session_id": session_id.as_str(),
                "source": source,
                "sequence": sequence,
                "agent_event": agent_event,
            }),
        ))
        .await;
}

fn session_stream_id(inbound: &InboundEvent) -> Option<String> {
    inbound
        .raw
        .get("http_stream_id")
        .and_then(serde_json::Value::as_str)
        .map(ToString::to_string)
}

fn format_final_message(outcome: &StreamOutcome) -> String {
    if let Some(internal_error) = &outcome.error {
        warn!(error = %internal_error, "agent turn errored; replying with generic message");
        return GENERIC_ERROR_REPLY.to_string();
    }
    if outcome.text.trim().is_empty() {
        warn!("agent produced no response; replying with generic message");
        return GENERIC_ERROR_REPLY.to_string();
    }
    outcome.text.clone()
}
