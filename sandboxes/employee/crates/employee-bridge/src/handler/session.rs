use chrono::Utc;
use domain::{
    event_types, InboundEvent, OutboundEvent, Session, SessionId, SessionStatus,
};
use outbound::OutboundEmitter;
use storage::SessionRepo;
use tracing::warn;

pub async fn ensure_session_persisted(
    session_repo: &dyn SessionRepo,
    inbound: &InboundEvent,
    emitter: &OutboundEmitter,
) -> bool {
    match session_repo.get(&inbound.session_id).await {
        Ok(Some(_)) => false,
        Ok(None) => {
            let (channel, thread_ts) = derive_channel_and_thread(&inbound.session_id);
            let now = Utc::now();
            let session = Session {
                id: inbound.session_id.clone(),
                channel: channel.clone(),
                thread_ts: thread_ts.clone(),
                adk_session_id: inbound.session_id.as_str().to_string(),
                status: SessionStatus::Active,
                created_at: now,
                last_activity_at: now,
            };
            if let Err(error) = session_repo.create(&session).await {
                warn!(session = %inbound.session_id, %error, "session_repo create failed");
                return false;
            }
            emitter
                .emit(OutboundEvent::new(
                    event_types::SESSION_CREATED,
                    serde_json::json!({
                        "session_id": inbound.session_id.as_str(),
                        "channel": channel,
                        "thread_ts": thread_ts,
                        "is_direct_message": inbound.is_direct_message,
                    }),
                ))
                .await;
            true
        }
        Err(error) => {
            warn!(session = %inbound.session_id, %error, "session_repo get failed");
            false
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

pub fn is_cron_message(inbound: &InboundEvent) -> bool {
    inbound.user == "cron"
}

pub fn derive_channel_from_session(session_id: &SessionId) -> String {
    session_id
        .as_str()
        .split_once('-')
        .map(|(c, _)| c.to_string())
        .unwrap_or_else(|| session_id.as_str().to_string())
}
