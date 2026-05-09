use chrono::Utc;
use domain::{HistoryMessage, SessionId};
use slack_morphism::prelude::*;

use super::context::{CachedThreadHistory, SlackContext};
use super::retry::with_retry;
use super::session_keys::split_session_id;
use super::users::resolve_display_name;
use crate::{GatewayError, Result};

const DEFAULT_RETRY_ATTEMPTS: u32 = 3;

pub async fn fetch_recent(
    context: &SlackContext,
    session_id: &SessionId,
    limit: u32,
) -> Result<Vec<HistoryMessage>> {
    let snapshot = context.config.snapshot();
    let cache_ttl_seconds = snapshot.slack.thread_context.cache_ttl_seconds as i64;
    let cache_key = session_id.as_str().to_string();

    if let Some(cached) = context.thread_history_cache.get(&cache_key) {
        let age = Utc::now() - cached.fetched_at;
        if age.num_seconds() < cache_ttl_seconds {
            return Ok(cached.messages.clone());
        }
    }

    let max_attempts = snapshot
        .slack
        .retry_max_attempts
        .unwrap_or(DEFAULT_RETRY_ATTEMPTS);
    let messages = with_retry(max_attempts, || {
        fetch_replies_from_slack(context, session_id, limit)
    })
    .await?;
    let with_names =
        enrich_with_display_names(context, messages, snapshot.slack.fetch_user_names).await;

    context.thread_history_cache.insert(
        cache_key,
        CachedThreadHistory {
            messages: with_names.clone(),
            fetched_at: Utc::now(),
        },
    );
    Ok(with_names)
}

async fn fetch_replies_from_slack(
    context: &SlackContext,
    session_id: &SessionId,
    limit: u32,
) -> Result<Vec<HistoryMessage>> {
    let (channel, thread_ts) = split_session_id(session_id)?;
    let session = context.open_api_session();
    let request = SlackApiConversationsRepliesRequest::new(channel, thread_ts.clone())
        .with_limit(limit as u16);
    let response = session
        .conversations_replies(&request)
        .await
        .map_err(|e| GatewayError::Other(anyhow::anyhow!("conversations.replies: {e}")))?;

    let bot_user_id = context.bot_user_id_str().to_string();
    let agent_name = context.config.snapshot().agent.name.clone();
    let mut messages = Vec::new();
    for raw in response.messages.into_iter() {
        if raw.subtype.is_some() {
            continue;
        }
        let user = raw
            .sender
            .user
            .as_ref()
            .map(|u| u.0.clone())
            .unwrap_or_default();
        let is_bot_authored =
            !bot_user_id.is_empty() && (user == bot_user_id || raw.sender.bot_id.is_some());
        let raw_text = raw.content.text.clone().unwrap_or_default();
        if raw_text.trim().is_empty() {
            continue;
        }
        let text = if bot_user_id.is_empty() {
            raw_text
        } else {
            raw_text.replace(&format!("<@{bot_user_id}>"), &format!("@{agent_name}"))
        };
        messages.push(HistoryMessage {
            user,
            user_display_name: None,
            text,
            ts: raw.origin.ts.0.clone(),
            is_bot: is_bot_authored,
        });
    }
    if !messages.is_empty() {
        messages.pop();
    }
    Ok(messages)
}

async fn enrich_with_display_names(
    context: &SlackContext,
    mut messages: Vec<HistoryMessage>,
    fetch_names: bool,
) -> Vec<HistoryMessage> {
    if !fetch_names {
        return messages;
    }
    for message in messages.iter_mut() {
        if message.is_bot || message.user.is_empty() {
            continue;
        }
        if let Ok(name) = resolve_display_name(context, &message.user).await {
            message.user_display_name = Some(name);
        }
    }
    messages
}
