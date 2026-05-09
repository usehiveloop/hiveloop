use domain::{LinkPreview, SlackConfig};
use serde_json::Value;
use slack_morphism::prelude::SlackMessageAttachment;

pub fn extract_message_text(
    raw_text: &str,
    blocks: &Option<Vec<Value>>,
    config: &SlackConfig,
) -> String {
    let mut combined = raw_text.trim().to_string();
    if config.extract_blocks_text {
        if let Some(block_list) = blocks {
            let block_text = collect_text_from_blocks(block_list);
            if !block_text.trim().is_empty() && !combined.contains(&block_text) {
                if !combined.is_empty() {
                    combined.push_str("\n\n");
                }
                combined.push_str(&block_text);
            }
        }
    }
    combined
}

fn collect_text_from_blocks(blocks: &[Value]) -> String {
    let mut out = String::new();
    for block in blocks {
        walk_block_for_text(block, &mut out);
    }
    out.trim().to_string()
}

fn walk_block_for_text(node: &Value, out: &mut String) {
    let Value::Object(map) = node else {
        if let Value::Array(items) = node {
            for item in items {
                walk_block_for_text(item, out);
            }
        }
        return;
    };
    let node_type = map.get("type").and_then(|v| v.as_str()).unwrap_or("");
    match node_type {
        "rich_text_quote" => {
            let mut quote_text = String::new();
            walk_elements(map, &mut quote_text);
            if !quote_text.trim().is_empty() {
                push_with_separator(out);
                out.push_str("> ");
                out.push_str(quote_text.trim());
            }
        }
        "rich_text_preformatted" => {
            let mut code_text = String::new();
            walk_elements(map, &mut code_text);
            if !code_text.trim().is_empty() {
                push_with_separator(out);
                out.push_str("```");
                out.push_str(code_text.trim());
                out.push_str("```");
            }
        }
        "rich_text_list" => render_list(map, out),
        "text" => {
            if let Some(literal) = map.get("text").and_then(|v| v.as_str()) {
                out.push_str(literal);
            }
        }
        "link" => {
            if let Some(url) = map.get("url").and_then(|v| v.as_str()) {
                let label = map.get("text").and_then(|v| v.as_str()).unwrap_or(url);
                out.push('<');
                out.push_str(url);
                out.push('|');
                out.push_str(label);
                out.push('>');
            }
        }
        "user" => {
            if let Some(user_id) = map.get("user_id").and_then(|v| v.as_str()) {
                out.push_str(&format!("<@{user_id}>"));
            }
        }
        "channel" => {
            if let Some(channel_id) = map.get("channel_id").and_then(|v| v.as_str()) {
                out.push_str(&format!("<#{channel_id}>"));
            }
        }
        "emoji" => {
            if let Some(name) = map.get("name").and_then(|v| v.as_str()) {
                out.push_str(&format!(":{name}:"));
            }
        }
        _ => {
            for child in map.values() {
                walk_block_for_text(child, out);
            }
        }
    }
}

fn walk_elements(map: &serde_json::Map<String, Value>, out: &mut String) {
    if let Some(Value::Array(elements)) = map.get("elements") {
        for element in elements {
            walk_block_for_text(element, out);
        }
    }
}

fn render_list(map: &serde_json::Map<String, Value>, out: &mut String) {
    let style = map
        .get("style")
        .and_then(|v| v.as_str())
        .unwrap_or("bullet");
    let indent = map.get("indent").and_then(|v| v.as_u64()).unwrap_or(0) as usize;
    let elements = match map.get("elements") {
        Some(Value::Array(items)) => items,
        _ => return,
    };
    for (item_index, element) in elements.iter().enumerate() {
        let mut item_text = String::new();
        walk_block_for_text(element, &mut item_text);
        let trimmed = item_text.trim();
        if trimmed.is_empty() {
            continue;
        }
        push_with_separator(out);
        for _ in 0..indent {
            out.push_str("  ");
        }
        if style == "ordered" {
            out.push_str(&format!("{}. ", item_index + 1));
        } else {
            out.push_str("• ");
        }
        out.push_str(trimmed);
    }
}

fn push_with_separator(out: &mut String) {
    if !out.is_empty() && !out.ends_with('\n') {
        out.push('\n');
    }
}

pub fn extract_link_previews_from_attachments(
    attachments: &[SlackMessageAttachment],
) -> Vec<LinkPreview> {
    attachments
        .iter()
        .filter_map(|attachment| {
            let value = serde_json::to_value(attachment).ok()?;
            let map = value.as_object()?;
            if matches!(map.get("is_msg_unfurl"), Some(Value::Bool(true))) {
                return None;
            }
            let url = map
                .get("original_url")
                .or_else(|| map.get("from_url"))
                .or_else(|| map.get("title_link"))
                .and_then(|v| v.as_str())?
                .to_string();
            let title = map
                .get("title")
                .and_then(|v| v.as_str())
                .map(str::to_string);
            let description = map
                .get("text")
                .or_else(|| map.get("fallback"))
                .and_then(|v| v.as_str())
                .map(str::to_string);
            Some(LinkPreview {
                url,
                title,
                description,
            })
        })
        .collect()
}
