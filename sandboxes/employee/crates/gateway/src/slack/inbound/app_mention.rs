use domain::InboundEvent;
use slack_morphism::prelude::*;

use super::helpers::{replace_bot_mention, serialize_blocks, slack_files_to_attachments};
use crate::slack::content::{extract_link_previews_from_attachments, extract_message_text};
use crate::slack::context::SlackContext;
use crate::slack::filters::{channel_is_allowed, ignore_users_blocks};
use crate::slack::session_keys::{build_session_id, engaged_thread_key, message_handle};
use crate::slack::users::resolve_display_name;

pub async fn handle_app_mention(
    context: &SlackContext,
    envelope_id: String,
    payload: SlackAppMentionEvent,
) {
    let bot_id = context.bot_user_id_str();
    if payload.user.0 == bot_id {
        return;
    }

    let snapshot = context.config.snapshot();
    let slack_cfg = &snapshot.slack;
    if ignore_users_blocks(slack_cfg, &payload.user.0) {
        return;
    }
    if !channel_is_allowed(slack_cfg, &payload.channel.0) {
        return;
    }

    let channel = payload.channel.0;
    let message_ts = payload.origin.ts.0;
    let original_thread_ts = payload.origin.thread_ts.as_ref().map(|t| t.0.clone());
    let thread_ts = original_thread_ts.clone().unwrap_or_else(|| message_ts.clone());
    let is_synthetic_thread = original_thread_ts.is_none();

    context
        .engaged_threads
        .insert(engaged_thread_key(&channel, &thread_ts));

    let session_id = build_session_id(&channel, Some(&thread_ts), &message_ts);
    if is_synthetic_thread {
        context
            .synthetic_thread_sessions
            .insert(session_id.as_str().to_string());
    }
    let raw_text = payload.content.text.clone().unwrap_or_default();
    let blocks_as_json = serialize_blocks(&payload.content.blocks);
    let text = extract_message_text(&raw_text, &blocks_as_json, slack_cfg);
    let attachments = if slack_cfg.download_attachments {
        slack_files_to_attachments(payload.content.files.as_deref().unwrap_or(&[]))
    } else {
        Vec::new()
    };
    let link_previews = if slack_cfg.extract_link_unfurls {
        extract_link_previews_from_attachments(payload.content.attachments.as_deref().unwrap_or(&[]))
    } else {
        Vec::new()
    };

    let user_display_name = if slack_cfg.fetch_user_names {
        resolve_display_name(context, &payload.user.0).await.ok()
    } else {
        None
    };

    let inbound = InboundEvent {
        envelope_id,
        session_id,
        user: payload.user.0.clone(),
        user_display_name,
        text: replace_bot_mention(&text, bot_id, &snapshot.agent.name),
        attachments,
        raw: serde_json::Value::Null,
        inbound_handle: message_handle(&channel, &message_ts),
        is_direct_message: matches!(
            payload.origin.channel_type,
            Some(SlackChannelType(ref kind)) if kind == "im"
        ),
        is_directly_addressed: true,
        link_previews,
    };
    super::dispatch_inbound(context, inbound).await;
}
