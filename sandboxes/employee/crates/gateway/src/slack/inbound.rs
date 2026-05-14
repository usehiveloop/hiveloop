mod app_mention;
mod helpers;
mod message;

use std::sync::Arc;

use chrono::Utc;
use slack_morphism::prelude::*;
use tracing::{debug, error, warn};

use super::context::{global, AssistantThreadMetadata, SlackContext};
use domain::{InboundEvent, SessionId};

pub async fn handle_push_event(
    event: SlackPushEventCallback,
    _client: Arc<SlackHyperClient>,
    _states: SlackClientEventsUserState,
) -> std::result::Result<(), Box<dyn std::error::Error + Send + Sync>> {
    let Some(context) = global() else {
        warn!("push event before context installed");
        return Ok(());
    };

    let envelope_event_id = event.event_id.0.clone();
    let dedupe_key = match &event.event {
        SlackEventCallbackBody::AppMention(payload) => {
            Some(format!("{}-{}", payload.channel.0, payload.origin.ts.0))
        }
        SlackEventCallbackBody::Message(payload) => payload
            .origin
            .channel
            .as_ref()
            .map(|channel| format!("{}-{}", channel.0, payload.origin.ts.0)),
        _ => None,
    };

    if let Some(key) = dedupe_key {
        if !record_message_or_drop_duplicate(context, &key).await {
            debug!(%key, "duplicate slack message; dropping");
            return Ok(());
        }
    }

    match event.event {
        SlackEventCallbackBody::AppMention(payload) => {
            app_mention::handle_app_mention(context, envelope_event_id, payload).await;
        }
        SlackEventCallbackBody::Message(payload) => {
            message::handle_message(context, envelope_event_id, payload).await;
        }
        SlackEventCallbackBody::AssistantThreadStarted(_)
        | SlackEventCallbackBody::AssistantThreadContextChanged(_) => {
            try_cache_assistant_thread_metadata(context, &serde_json::to_value(&event).ok());
        }
        _ => {}
    }
    Ok(())
}

async fn record_message_or_drop_duplicate(context: &SlackContext, dedupe_key: &str) -> bool {
    let mut cache = context.seen_event_ids.lock().await;
    cache.put(dedupe_key.to_string(), ()).is_none()
}

fn try_cache_assistant_thread_metadata(
    context: &SlackContext,
    payload: &Option<serde_json::Value>,
) {
    let Some(value) = payload else { return };
    let event_obj = value.get("event").and_then(|v| v.as_object());
    let Some(event_obj) = event_obj else { return };
    let context_obj = event_obj
        .get("assistant_thread")
        .and_then(|v| v.as_object())
        .or_else(|| event_obj.get("context").and_then(|v| v.as_object()));
    let Some(thread_obj) = context_obj else {
        return;
    };
    let user_id = thread_obj
        .get("user_id")
        .and_then(|v| v.as_str())
        .map(str::to_string)
        .unwrap_or_default();
    let channel_id = thread_obj
        .get("channel_id")
        .and_then(|v| v.as_str())
        .map(str::to_string)
        .unwrap_or_default();
    let thread_ts = thread_obj
        .get("thread_ts")
        .and_then(|v| v.as_str())
        .map(str::to_string)
        .unwrap_or_default();
    if channel_id.is_empty() || thread_ts.is_empty() {
        return;
    }
    let key = format!("{channel_id}-{thread_ts}");
    context.assistant_thread_metadata.insert(
        key,
        AssistantThreadMetadata {
            user_id,
            channel_id,
            thread_ts,
            fetched_at: Utc::now(),
        },
    );
}

pub(super) async fn dispatch_inbound(context: &SlackContext, inbound: InboundEvent) {
    let session_id_for_log: SessionId = inbound.session_id.clone();
    let Some(sink) = context.inbound_sink.get() else {
        error!(session = %session_id_for_log, "no inbound sink installed; dropping event");
        return;
    };
    if sink.send(inbound).await.is_err() {
        error!(session = %session_id_for_log, "inbound sink closed; dropping event");
    }
}
