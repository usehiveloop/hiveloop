use domain::InboundEvent;
use slack_morphism::prelude::*;

use super::helpers::{
    replace_bot_mention, serialize_blocks, slack_files_to_attachments, subtype_should_be_dropped,
};
use crate::slack::content::{extract_link_previews_from_attachments, extract_message_text};
use crate::slack::context::SlackContext;
use crate::slack::filters::{
    channel_is_allowed, classify_bot_message, ignore_users_blocks, mention_rules_allow,
    BotMessageDecision, MentionDecision,
};
use crate::slack::session_keys::{build_session_id, engaged_thread_key, message_handle};
use crate::slack::users::resolve_display_name;

pub async fn handle_message(
    context: &SlackContext,
    envelope_id: String,
    payload: SlackMessageEvent,
) {
    let bot_id = context.bot_user_id_str();
    let snapshot = context.config.snapshot();
    let slack_cfg = &snapshot.slack;

    if let Some(user) = payload.sender.user.as_ref() {
        if user.0 == bot_id {
            return;
        }
        if ignore_users_blocks(slack_cfg, &user.0) {
            return;
        }
    }

    let raw_text_initial = payload
        .content
        .as_ref()
        .and_then(|c| c.text.clone())
        .unwrap_or_default();
    let mention_present = raw_text_initial.contains(&format!("<@{bot_id}>"));

    match classify_bot_message(slack_cfg, payload.sender.bot_id.is_some()) {
        BotMessageDecision::AllowAlways => {}
        BotMessageDecision::Drop => return,
        BotMessageDecision::AllowOnlyIfMentioned => {
            if !mention_present {
                return;
            }
        }
    }
    if subtype_should_be_dropped(&payload) {
        return;
    }

    let Some(channel_id) = payload.origin.channel.as_ref().map(|c| c.0.clone()) else {
        return;
    };
    if !channel_is_allowed(slack_cfg, &channel_id) {
        return;
    }

    let message_ts = payload.origin.ts.0.clone();
    let original_thread_ts = payload.origin.thread_ts.as_ref().map(|t| t.0.clone());
    let thread_ts = original_thread_ts
        .clone()
        .unwrap_or_else(|| message_ts.clone());
    let is_synthetic_thread = original_thread_ts.is_none();

    let raw_text = raw_text_initial;
    let blocks_for_extraction = payload.content.as_ref().and_then(|c| c.blocks.clone());

    let is_direct = matches!(
        payload.origin.channel_type,
        Some(SlackChannelType(ref kind)) if kind == "im"
    );
    let engaged_key = engaged_thread_key(&channel_id, &thread_ts);
    let already_engaged = context.engaged_threads.contains(&engaged_key);
    let bot_has_replied_in_thread = original_thread_ts
        .as_ref()
        .map(|ts| context.bot_message_timestamps.contains(ts))
        .unwrap_or(false);

    match mention_rules_allow(
        slack_cfg,
        is_direct,
        &channel_id,
        mention_present,
        already_engaged,
        bot_has_replied_in_thread,
    ) {
        MentionDecision::Allow => {}
        MentionDecision::Block => return,
    }

    if mention_present {
        context.engaged_threads.insert(engaged_key);
    }

    let session_id = build_session_id(&channel_id, Some(&thread_ts), &message_ts);
    if is_synthetic_thread {
        context
            .synthetic_thread_sessions
            .insert(session_id.as_str().to_string());
    }
    let blocks_as_json = serialize_blocks(&blocks_for_extraction);
    let text = extract_message_text(&raw_text, &blocks_as_json, slack_cfg);
    let files_for_extraction: Vec<_> = payload
        .content
        .as_ref()
        .and_then(|c| c.files.clone())
        .unwrap_or_default();
    let attachments = if slack_cfg.download_attachments {
        slack_files_to_attachments(&files_for_extraction)
    } else {
        Vec::new()
    };
    let payload_attachments = payload.content.as_ref().and_then(|c| c.attachments.clone());
    let link_previews = if slack_cfg.extract_link_unfurls {
        extract_link_previews_from_attachments(payload_attachments.as_deref().unwrap_or(&[]))
    } else {
        Vec::new()
    };

    let user = payload
        .sender
        .user
        .as_ref()
        .map(|u| u.0.clone())
        .unwrap_or_default();
    let user_display_name = if slack_cfg.fetch_user_names && !user.is_empty() {
        resolve_display_name(context, &user).await.ok()
    } else {
        None
    };

    let is_directly_addressed =
        is_direct || mention_present || already_engaged || bot_has_replied_in_thread;
    let inbound = InboundEvent {
        envelope_id,
        session_id,
        user,
        user_display_name,
        text: replace_bot_mention(&text, bot_id, &snapshot.agent.name),
        attachments,
        raw: serde_json::json!({"source": "slack"}),
        inbound_handle: message_handle(&channel_id, &message_ts),
        is_direct_message: is_direct,
        is_directly_addressed,
        link_previews,
    };
    super::dispatch_inbound(context, inbound).await;
}
