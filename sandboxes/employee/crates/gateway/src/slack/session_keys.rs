use domain::{MessageHandle, SessionId};
use slack_morphism::prelude::{SlackChannelId, SlackTs};

use crate::{GatewayError, Result};

pub fn build_session_id(channel: &str, thread_ts: Option<&str>, message_ts: &str) -> SessionId {
    let resolved_thread_ts = thread_ts.unwrap_or(message_ts);
    SessionId::from_slack(channel, resolved_thread_ts)
}

pub fn engaged_thread_key(channel: &str, thread_ts: &str) -> String {
    format!("{channel}-{thread_ts}")
}

pub fn split_session_id(session_id: &SessionId) -> Result<(SlackChannelId, SlackTs)> {
    let raw = session_id.as_str();
    let (channel, ts) = raw.split_once('-').ok_or_else(|| {
        GatewayError::Other(anyhow::anyhow!("session id `{raw}` is not Slack-shaped"))
    })?;
    Ok((SlackChannelId(channel.into()), SlackTs(ts.into())))
}

pub fn message_handle(channel: &str, ts: &str) -> MessageHandle {
    MessageHandle {
        channel: channel.to_string(),
        ts: ts.to_string(),
    }
}
