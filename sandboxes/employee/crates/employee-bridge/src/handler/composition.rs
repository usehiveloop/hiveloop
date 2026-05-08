use domain::{InboundEvent, SessionId, SlackConfig};

use super::media::DownloadResults;

pub fn compose_annotated_text(
    inbound: &InboundEvent,
    slack: &SlackConfig,
    session_id: &SessionId,
    media: &DownloadResults,
) -> String {
    let mut composed = inbound.text.clone();

    if let Some(addendum) = lookup_channel_prompt(slack, session_id) {
        composed.push_str("\n\n[Channel-specific instruction]\n");
        composed.push_str(&addendum);
    }

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

fn lookup_channel_prompt(slack: &SlackConfig, session_id: &SessionId) -> Option<String> {
    let session_str = session_id.as_str();
    let channel = session_str
        .split_once('-')
        .map(|(c, _)| c)
        .unwrap_or(session_str);
    slack.channel_prompts.get(channel).cloned()
}
