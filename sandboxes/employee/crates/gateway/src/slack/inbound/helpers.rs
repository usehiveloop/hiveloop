use slack_morphism::prelude::*;

pub fn slack_raw_metadata(
    channel: &str,
    thread_ts: &str,
    user_id: &str,
    user_display_name: Option<&str>,
    text: &str,
) -> serde_json::Value {
    serde_json::json!({
        "source": "slack",
        "channel": channel,
        "thread_ts": thread_ts,
        "user": user_id,
        "user_display_name": user_display_name,
        "user_mention": slack_user_mention(user_id),
        "mentioned_users": mentioned_slack_user_ids(text),
    })
}

pub fn replace_bot_mention(text: &str, bot_user_id: &str, agent_name: &str) -> String {
    if bot_user_id.is_empty() {
        return text.trim().to_string();
    }
    let needle = format!("<@{bot_user_id}>");
    let replacement = format!("@{agent_name}");
    text.replace(&needle, &replacement).trim().to_string()
}

fn slack_user_mention(user_id: &str) -> Option<String> {
    if user_id.starts_with('U') || user_id.starts_with('W') {
        Some(format!("<@{user_id}>"))
    } else {
        None
    }
}

fn mentioned_slack_user_ids(text: &str) -> Vec<String> {
    let bytes = text.as_bytes();
    let mut users = std::collections::BTreeSet::new();
    let mut index = 0usize;
    while index + 3 < bytes.len() {
        if bytes[index] == b'<' && bytes[index + 1] == b'@' {
            if let Some(end) = text[index + 2..].find('>') {
                let user_id = text[index + 2..index + 2 + end]
                    .split('|')
                    .next()
                    .unwrap_or("");
                if slack_user_mention(user_id).is_some() {
                    users.insert(user_id.to_string());
                }
                index += end + 3;
                continue;
            }
        }
        index += 1;
    }
    users.into_iter().collect()
}

pub fn subtype_should_be_dropped(payload: &SlackMessageEvent) -> bool {
    let Some(subtype) = payload.subtype.as_ref() else {
        return false;
    };
    let raw = serde_json::to_value(subtype)
        .ok()
        .and_then(|v| v.as_str().map(str::to_string))
        .unwrap_or_default();
    matches!(raw.as_str(), "message_changed" | "message_deleted")
}

pub fn serialize_blocks<T: serde::Serialize>(
    blocks: &Option<Vec<T>>,
) -> Option<Vec<serde_json::Value>> {
    blocks.as_ref().map(|list| {
        list.iter()
            .filter_map(|item| serde_json::to_value(item).ok())
            .collect()
    })
}

pub fn slack_files_to_attachments(files: &[SlackFile]) -> Vec<domain::Attachment> {
    files
        .iter()
        .filter_map(|file| {
            let url = file
                .url_private_download
                .as_ref()
                .or(file.url_private.as_ref())
                .map(|u| u.to_string())?;
            Some(domain::Attachment {
                url,
                mime_type: file
                    .mimetype
                    .as_ref()
                    .map(|m| m.0.clone())
                    .unwrap_or_default(),
                name: file
                    .name
                    .clone()
                    .or_else(|| file.title.clone())
                    .unwrap_or_else(|| "file".to_string()),
                size_bytes: None,
            })
        })
        .collect()
}
