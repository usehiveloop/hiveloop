mod mrkdwn;

use domain::SlackConfig;

pub use mrkdwn::markdown_to_mrkdwn;

pub fn outgoing_text(text: &str, config: &SlackConfig) -> String {
    let translated = if config.mrkdwn_translation {
        markdown_to_mrkdwn(text)
    } else {
        text.to_string()
    };
    apply_reply_prefix(&translated, &config.reply_prefix)
}

pub fn split_for_slack(text: &str, max_length: u32) -> Vec<String> {
    let max = max_length as usize;
    if text.len() <= max {
        return vec![text.to_string()];
    }
    let mut chunks = Vec::new();
    let mut remaining = text;
    while remaining.len() > max {
        let split_index = best_split_index(remaining, max);
        let (head, tail) = remaining.split_at(split_index);
        chunks.push(head.to_string());
        remaining = tail.trim_start();
    }
    if !remaining.is_empty() {
        chunks.push(remaining.to_string());
    }
    chunks
}

fn best_split_index(text: &str, max: usize) -> usize {
    let mut limit = max.min(text.len());
    while limit > 0 && !text.is_char_boundary(limit) {
        limit -= 1;
    }
    if let Some(idx) = text[..limit].rfind("\n\n") {
        return idx;
    }
    if let Some(idx) = text[..limit].rfind('\n') {
        return idx;
    }
    if let Some(idx) = text[..limit].rfind(' ') {
        return idx;
    }
    limit
}

fn apply_reply_prefix(text: &str, prefix: &str) -> String {
    if prefix.is_empty() {
        text.to_string()
    } else {
        format!("{prefix}{text}")
    }
}
