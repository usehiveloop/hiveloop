mod composition;
mod media;
mod session;

use std::sync::Arc;
use std::time::Duration;

use agent::{AgentEvent, AgentRunner, HistoryEntry, HistoryRole, TurnInput};
use anyhow::Result;
use chrono::Utc;
use domain::{
    event_types, ConfigStore, HistoryMessage, InboundEvent, OutboundEvent, Reply, SessionId,
    SessionStatus,
};
use futures::StreamExt;
use gateway::ChannelGateway;
use outbound::OutboundEmitter;
use storage::SessionRepo;
use tokio::sync::oneshot;
use tracing::{info, warn};

use composition::compose_annotated_text;
use media::{collect_media_for_turn, DownloadResults};
use session::{
    derive_channel_from_session, ensure_session_persisted, is_cron_message, is_wake_cron,
};

use crate::session_coordinator::{SessionCoordinator, Submission};

const STATUS_REFRESH_SECONDS: u64 = 2;
const GENERIC_ERROR_REPLY: &str =
    "Sorry, something went wrong while generating that response. Please try again.";

pub async fn handle_inbound(
    runner: Arc<dyn AgentRunner>,
    gateway: Arc<dyn ChannelGateway>,
    config: ConfigStore,
    emitter: Arc<OutboundEmitter>,
    session_repo: Arc<dyn SessionRepo>,
    coordinator: Arc<SessionCoordinator>,
    inbound: InboundEvent,
) -> Result<()> {
    let submission =
        coordinator.submit_or_queue(&inbound.session_id, inbound.text.clone());
    if matches!(submission, Submission::Queued) {
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
        )
        .await?;

        let follow_ups = coordinator.finish_turn(&current_inbound.session_id);
        if follow_ups.is_empty() {
            break;
        }

        let merged = format!(
            "[Additional request(s) received while working on previous task:]\n{}",
            follow_ups.join("\n")
        );
        current_inbound.text = merged;
        coordinator.submit_or_queue(&current_inbound.session_id, String::new());
    }

    Ok(())
}

async fn process_single_turn(
    runner: Arc<dyn AgentRunner>,
    gateway: Arc<dyn ChannelGateway>,
    config: ConfigStore,
    emitter: Arc<OutboundEmitter>,
    session_repo: Arc<dyn SessionRepo>,
    inbound: &InboundEvent,
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
    let slack = snapshot.slack.clone();
    let session_id = inbound.session_id.clone();

    let was_new_session =
        ensure_session_persisted(session_repo.as_ref(), &inbound, &emitter).await;
    if !was_new_session {
        let _ = session_repo.touch(&session_id, Utc::now()).await;
    }

    let skip_typing = is_cron_message(inbound) || !slack.typing_indicator;
    let typing_loop = if skip_typing {
        None
    } else {
        Some(spawn_thinking_status_loop(gateway.clone(), session_id.clone()))
    };

    let multimodal_available = snapshot.multimodal_model.is_some();
    let media = collect_media_for_turn(
        gateway.as_ref(),
        &inbound.attachments,
        &slack,
        multimodal_available,
    )
    .await;
    let annotated_text = compose_annotated_text(&inbound, &slack, &session_id, &media);

    let prior_history = fetch_prior_history_for_session(
        gateway.as_ref(),
        &session_id,
        &slack,
    )
    .await;

    let DownloadResults { images, .. } = media;
    let mut turn_input = TurnInput::text(annotated_text).with_history(prior_history);
    for (mime, bytes) in images {
        turn_input = turn_input.with_image(mime, bytes);
    }

    let stream_result = runner.run_turn(&session_id, turn_input).await;
    let outcome = match stream_result {
        Ok(stream) => consume_agent_stream(stream).await,
        Err(e) => StreamOutcome {
            text: String::new(),
            error: Some(e.to_string()),
        },
    };

    if let Some((task, cancel_signal)) = typing_loop {
        let _ = cancel_signal.send(());
        let _ = task.await;
    }
    if slack.typing_indicator && !is_cron_message(inbound) {
        if let Err(e) = gateway.stop_typing(&session_id).await {
            warn!(error = %e, "stop_typing failed");
        }
    }

    let final_text = format_final_message(&outcome);
    let reply_text_for_event = final_text.clone();
    if is_cron_message(inbound) && !is_wake_cron(inbound) {
        let channel = derive_channel_from_session(&session_id);
        if let Err(e) = gateway.post_to_channel(&channel, Reply::Text(final_text)).await {
            warn!(error = %e, "post_to_channel (cron) failed");
        }
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
                    "error": error_message,
                }),
            ))
            .await;
        let _ = session_repo
            .set_status(&session_id, SessionStatus::Errored)
            .await;
    } else {
        emitter
            .emit(OutboundEvent::new(
                event_types::AGENT_MESSAGE_SENT,
                serde_json::json!({
                    "session_id": session_id.as_str(),
                    "text": reply_text_for_event,
                }),
            ))
            .await;
    }

    info!(session = %session_id, len = outcome.text.len(), "turn complete");
    Ok(())
}

pub(crate) struct StreamOutcome {
    pub text: String,
    pub error: Option<String>,
}

fn spawn_thinking_status_loop(
    gateway: Arc<dyn ChannelGateway>,
    session_id: SessionId,
) -> (tokio::task::JoinHandle<()>, oneshot::Sender<()>) {
    let (cancel_signal, mut cancel_receiver) = oneshot::channel();
    let handle = tokio::spawn(async move {
        loop {
            if let Err(e) = gateway.typing(&session_id).await {
                warn!(error = %e, "typing(status) failed");
            }
            tokio::select! {
                _ = tokio::time::sleep(Duration::from_secs(STATUS_REFRESH_SECONDS)) => continue,
                _ = &mut cancel_receiver => break,
            }
        }
    });
    (handle, cancel_signal)
}

async fn consume_agent_stream(
    mut stream: futures::stream::BoxStream<'static, AgentEvent>,
) -> StreamOutcome {
    let mut accumulated = String::new();
    let mut error_message: Option<String> = None;
    while let Some(event) = stream.next().await {
        match event {
            AgentEvent::TokenChunk { text } => accumulated.push_str(&text),
            AgentEvent::FinalMessage { text } => accumulated = text,
            AgentEvent::Error { message } => error_message = Some(message),
            _ => {}
        }
    }
    StreamOutcome {
        text: accumulated,
        error: error_message,
    }
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

async fn fetch_prior_history_for_session(
    gateway: &dyn ChannelGateway,
    session_id: &SessionId,
    slack_config: &domain::SlackConfig,
) -> Vec<HistoryEntry> {
    let limit = slack_config.thread_context.max_messages.max(1);
    let history_result = gateway.fetch_thread_history(session_id, limit).await;
    let history = match history_result {
        Ok(messages) => messages,
        Err(e) => {
            warn!(session = %session_id, error = %e, "fetch_thread_history failed");
            return Vec::new();
        }
    };
    history
        .into_iter()
        .map(history_message_into_entry)
        .collect()
}

fn history_message_into_entry(message: HistoryMessage) -> HistoryEntry {
    HistoryEntry {
        role: if message.is_bot {
            HistoryRole::Assistant
        } else {
            HistoryRole::User
        },
        speaker_id: message.user,
        speaker_display_name: message.user_display_name,
        text: message.text,
    }
}
