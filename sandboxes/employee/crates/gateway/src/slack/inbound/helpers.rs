use slack_morphism::prelude::*;

pub fn replace_bot_mention(text: &str, bot_user_id: &str, agent_name: &str) -> String {
    if bot_user_id.is_empty() {
        return text.trim().to_string();
    }
    let needle = format!("<@{bot_user_id}>");
    let replacement = format!("@{agent_name}");
    text.replace(&needle, &replacement).trim().to_string()
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
                mime_type: file.mimetype.as_ref().map(|m| m.0.clone()).unwrap_or_default(),
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
