use domain::{MessageHandle, Reply, SessionId};
use slack_morphism::prelude::*;

use super::context::SlackContext;
use super::diagnostics::user_facing_explanation;
use super::format::{outgoing_text, split_for_slack};
use super::session_keys::{message_handle, split_session_id};
use crate::{GatewayError, Result};

pub async fn post_text_reply(
    context: &SlackContext,
    session_id: &SessionId,
    body: Reply,
) -> Result<MessageHandle> {
    let (channel, thread_ts) = split_session_id(session_id)?;
    let raw = match body {
        Reply::Text(text) => text,
        Reply::Rich(_) => return Err(GatewayError::Unsupported("Reply::Rich")),
    };

    let snapshot = context.config.snapshot();
    let formatted = outgoing_text(&raw, &snapshot.slack);
    let chunks = if snapshot.slack.split_long_replies {
        split_for_slack(&formatted, snapshot.slack.max_message_length)
    } else {
        vec![formatted]
    };

    let thread_ts_for_post = pick_thread_ts_for_reply(context, session_id, &thread_ts, &snapshot.slack);

    let session = context.open_api_session();
    let mut last_handle: Option<MessageHandle> = None;
    let total_chunks = chunks.len();
    for (index, chunk) in chunks.into_iter().enumerate() {
        let mut request = SlackApiChatPostMessageRequest::new(
            channel.clone(),
            SlackMessageContent::new().with_text(chunk),
        );
        if let Some(ts) = &thread_ts_for_post {
            request = request.with_thread_ts(ts.clone());
        }
        if snapshot.slack.reply_broadcast && index == 0 && total_chunks > 0 {
            request = request.with_reply_broadcast(true);
        }
        let response = session.chat_post_message(&request).await.map_err(|e| {
            let detail = user_facing_explanation(&e).unwrap_or_else(|| e.to_string());
            GatewayError::Other(anyhow::anyhow!("chat.postMessage: {detail}"))
        })?;
        context
            .bot_message_timestamps
            .insert(response.ts.0.clone());
        last_handle = Some(message_handle(&response.channel.0, &response.ts.0));
    }
    last_handle.ok_or_else(|| GatewayError::Other(anyhow::anyhow!("no chunks posted")))
}

pub async fn post_text_to_channel(
    context: &SlackContext,
    channel: &str,
    body: Reply,
) -> Result<MessageHandle> {
    let raw = match body {
        Reply::Text(text) => text,
        Reply::Rich(_) => return Err(GatewayError::Unsupported("Reply::Rich")),
    };
    let snapshot = context.config.snapshot();
    let formatted = outgoing_text(&raw, &snapshot.slack);
    let session = context.open_api_session();
    let request = SlackApiChatPostMessageRequest::new(
        SlackChannelId(channel.into()),
        SlackMessageContent::new().with_text(formatted),
    );
    let response = session.chat_post_message(&request).await.map_err(|e| {
        let detail = user_facing_explanation(&e).unwrap_or_else(|| e.to_string());
        GatewayError::Other(anyhow::anyhow!("chat.postMessage: {detail}"))
    })?;
    context
        .bot_message_timestamps
        .insert(response.ts.0.clone());
    Ok(message_handle(&response.channel.0, &response.ts.0))
}

fn pick_thread_ts_for_reply(
    context: &SlackContext,
    session_id: &SessionId,
    thread_ts: &SlackTs,
    slack_config: &domain::SlackConfig,
) -> Option<SlackTs> {
    if slack_config.reply_in_thread {
        return Some(thread_ts.clone());
    }
    let is_synthetic = context
        .synthetic_thread_sessions
        .contains(session_id.as_str());
    if is_synthetic {
        None
    } else {
        Some(thread_ts.clone())
    }
}

pub async fn edit_text_reply(
    context: &SlackContext,
    handle: &MessageHandle,
    body: Reply,
) -> Result<()> {
    let raw = match body {
        Reply::Text(text) => text,
        Reply::Rich(_) => return Err(GatewayError::Unsupported("Reply::Rich")),
    };
    let snapshot = context.config.snapshot();
    let formatted = outgoing_text(&raw, &snapshot.slack);
    let truncated = truncate_for_edit(&formatted, snapshot.slack.max_message_length);

    let session = context.open_api_session();
    let request = SlackApiChatUpdateRequest::new(
        SlackChannelId(handle.channel.clone()),
        SlackMessageContent::new().with_text(truncated),
        SlackTs(handle.ts.clone()),
    );
    session.chat_update(&request).await.map_err(|e| {
        let detail = user_facing_explanation(&e).unwrap_or_else(|| e.to_string());
        GatewayError::Other(anyhow::anyhow!("chat.update: {detail}"))
    })?;
    Ok(())
}

fn truncate_for_edit(text: &str, max_length: u32) -> String {
    let limit = max_length as usize;
    if text.len() <= limit {
        return text.to_string();
    }
    let mut shortened = text[..limit.saturating_sub(20)].to_string();
    shortened.push_str("\n…(truncated)");
    shortened
}
