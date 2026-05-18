use std::collections::BTreeSet;

use domain::{InboundEvent, SessionId, SlackConfig};

use super::media::DownloadResults;

pub fn compose_annotated_text(
    inbound: &InboundEvent,
    _slack: &SlackConfig,
    _session_id: &SessionId,
    media: &DownloadResults,
) -> String {
    let mut composed = if is_slack_inbound(inbound, _session_id) {
        compose_slack_annotated_text(inbound, _session_id)
    } else {
        inbound.text.clone()
    };

    if !inbound.attachments.is_empty() {
        composed.push_str("\n\n[Attached files]");
        for attachment in &inbound.attachments {
            composed.push_str(&format!(
                "\n- {} ({})",
                attachment.name, attachment.mime_type
            ));
        }
    }
    for inlined in &media.text_files {
        composed.push_str(&format!("\n\n[Content of {}]\n", inlined.name));
        composed.push_str(&inlined.contents);
    }
    if !media.audio_summaries.is_empty() {
        composed.push_str("\n\n[Audio attachments]");
        for summary in &media.audio_summaries {
            composed.push_str(&format!("\n- {summary}"));
        }
    }
    if !media.document_summaries.is_empty() {
        composed.push_str("\n\n[Document attachments]");
        for summary in &media.document_summaries {
            composed.push_str(&format!("\n- {summary}"));
        }
    }
    if !inbound.link_previews.is_empty() {
        composed.push_str("\n\n[Link previews]");
        for preview in &inbound.link_previews {
            let title = preview.title.as_deref().unwrap_or("");
            let description = preview.description.as_deref().unwrap_or("");
            composed.push_str(&format!("\n- {title} ({}) — {description}", preview.url));
        }
    }
    if !media.failure_notices.is_empty() {
        composed.push_str("\n\n[Slack attachment notice]");
        for notice in &media.failure_notices {
            composed.push_str(&format!("\n- {notice}"));
        }
    }
    composed
}

pub fn slack_communication_context() -> &'static str {
    r#"## Slack communication
- Be brief by default. Use one sentence when possible. Use at most three tight bullets when detail is genuinely needed.
- Post thread status only for longer work, blockers, material plan changes, or verified completion. Skip play-by-play for quick checks.
- Do not mention internal tools, cloud agents, task ids, proxy URLs, schema probing, or execution mechanics unless the user asked how the system works.
- Speak to the teammate in the thread. Use teammate names naturally; do not describe the sender in the third person.
- Mention teammates with exact Slack mention tokens such as `<@U123ABC>`. Do not write `@Name` when a Slack user ID is known, and never invent a user ID.
- Good brief reply: `Done. The deploy check is green.`
- Good status update: `I am checking the deploy logs now. I will only post again if I find a blocker or finish.`
- Good mention: `<@U123ABC> can you confirm the deploy window?`
- Bad reply: `Absolutely, I would be happy to dive into this and provide a comprehensive overview.`
- Bad status update: `Cloud agent is running the check now.`
- Bad status update: `Checking repos for PostHog references because <Name> asked.`"#
}

pub fn should_include_slack_communication_context(inbound: &InboundEvent) -> bool {
    if inbound.session_id.as_str().starts_with("http-") {
        return false;
    }
    if inbound
        .raw
        .get("source")
        .and_then(serde_json::Value::as_str)
        .is_some_and(|source| source == "http" || source == "cloud_agent_callback")
    {
        return false;
    }
    inbound.session_id.as_str().split_once('-').is_some()
}

fn is_slack_inbound(inbound: &InboundEvent, _session_id: &SessionId) -> bool {
    inbound
        .raw
        .get("source")
        .and_then(serde_json::Value::as_str)
        .is_some_and(|source| source == "slack")
}

fn compose_slack_annotated_text(inbound: &InboundEvent, session_id: &SessionId) -> String {
    let (channel, thread_ts) = slack_channel_and_thread(session_id);
    let sender = slack_user_label(&inbound.user, inbound.user_display_name.as_deref());
    let mut composed =
        format!("Slack message from {sender} in {channel} / thread_ts: {thread_ts}\n\n");

    let known_users = known_slack_users(inbound);
    if !known_users.is_empty() {
        composed.push_str("Known Slack users:\n");
        for user in known_users {
            composed.push_str("- ");
            composed.push_str(&user);
            if user == sender {
                composed.push_str(" (sender)");
            }
            composed.push('\n');
        }
        composed.push('\n');
    }

    composed.push_str("Message:\n");
    composed.push_str(&inbound.text);
    composed
}

fn slack_channel_and_thread(session_id: &SessionId) -> (String, String) {
    let raw = session_id.as_str();
    match raw.split_once('-') {
        Some((channel, thread_ts)) => (channel.to_string(), thread_ts.to_string()),
        None => (raw.to_string(), "unknown".to_string()),
    }
}

fn known_slack_users(inbound: &InboundEvent) -> Vec<String> {
    let mut users = BTreeSet::new();
    let sender = slack_user_label(&inbound.user, inbound.user_display_name.as_deref());
    if !sender.is_empty() {
        users.insert(sender);
    }
    let sender_mention = slack_user_mention(&inbound.user);
    for mention in mentioned_slack_users(&inbound.text) {
        if sender_mention.as_deref() == Some(mention.as_str()) {
            continue;
        }
        users.insert(mention);
    }
    users.into_iter().collect()
}

fn slack_user_label(user_id: &str, display_name: Option<&str>) -> String {
    let user_id = user_id.trim();
    let name = display_name.unwrap_or("").trim();
    let mention = slack_user_mention(user_id);
    match (name.is_empty(), mention) {
        (false, Some(mention)) => format!("{name} ({mention})"),
        (false, None) => name.to_string(),
        (true, Some(mention)) => mention,
        (true, None) => "unknown teammate".to_string(),
    }
}

fn slack_user_mention(user_id: &str) -> Option<String> {
    if user_id.starts_with('U') || user_id.starts_with('W') {
        Some(format!("<@{user_id}>"))
    } else {
        None
    }
}

fn mentioned_slack_users(text: &str) -> Vec<String> {
    let bytes = text.as_bytes();
    let mut users = BTreeSet::new();
    let mut index = 0usize;
    while index + 3 < bytes.len() {
        if bytes[index] == b'<' && bytes[index + 1] == b'@' {
            if let Some(end) = text[index + 2..].find('>') {
                let user_id = text[index + 2..index + 2 + end]
                    .split('|')
                    .next()
                    .unwrap_or("");
                if let Some(mention) = slack_user_mention(user_id) {
                    users.insert(mention);
                }
                index += end + 3;
                continue;
            }
        }
        index += 1;
    }
    users.into_iter().collect()
}

pub fn lookup_channel_prompt(slack: &SlackConfig, session_id: &SessionId) -> Option<String> {
    let session_str = session_id.as_str();
    let channel = session_str
        .split_once('-')
        .map(|(c, _)| c)
        .unwrap_or(session_str);
    slack.channel_prompts.get(channel).cloned()
}

#[cfg(test)]
mod tests {
    use domain::{InboundEvent, MessageHandle, SessionId};

    use super::*;

    fn event() -> InboundEvent {
        InboundEvent {
            envelope_id: "env-1".into(),
            session_id: SessionId::from_slack("C123", "1770000000.000001"),
            user: "U123".into(),
            user_display_name: Some("Kim".into()),
            text: "Can <@U999> confirm the deploy window?".into(),
            attachments: Vec::new(),
            raw: serde_json::json!({"source": "slack"}),
            inbound_handle: MessageHandle {
                channel: "C123".into(),
                ts: "1770000000.000001".into(),
            },
            is_direct_message: false,
            is_directly_addressed: true,
            link_previews: Vec::new(),
        }
    }

    #[test]
    fn slack_turn_text_preserves_sender_name_and_user_id() {
        let inbound = event();
        let text = compose_annotated_text(
            &inbound,
            &SlackConfig::default(),
            &inbound.session_id,
            &DownloadResults::default(),
        );
        assert!(text
            .contains("Slack message from Kim (<@U123>) in C123 / thread_ts: 1770000000.000001"));
        assert!(text.contains("- Kim (<@U123>) (sender)"));
        assert!(text.contains("- <@U999>"));
        assert!(text.contains("Message:\nCan <@U999> confirm the deploy window?"));
    }

    #[test]
    fn slack_communication_context_is_only_for_slack_routed_sessions() {
        let inbound = event();
        assert!(should_include_slack_communication_context(&inbound));
        let context = slack_communication_context();
        assert!(context.contains("<@U123ABC>"));
        assert!(context.contains("Skip play-by-play"));
        assert!(context.contains("Do not mention internal tools, cloud agents"));
        assert!(context.contains("Bad status update: `Cloud agent is running the check now.`"));

        let mut http = inbound;
        http.session_id = SessionId::from("http-stream-1");
        http.raw = serde_json::json!({"source":"http"});
        assert!(!should_include_slack_communication_context(&http));
    }
}
