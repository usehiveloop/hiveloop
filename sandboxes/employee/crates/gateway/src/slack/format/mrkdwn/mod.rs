mod protect;
mod transform;

use protect::{
    extract_blockquote_prefixes, extract_code_blocks, extract_inline_code, extract_slack_entities,
    html_entity_escape, BLOCKQUOTE_PLACEHOLDER_PREFIX, CODE_BLOCK_PLACEHOLDER_PREFIX,
    INLINE_CODE_PLACEHOLDER_PREFIX, SLACK_ENTITY_PLACEHOLDER_PREFIX,
};
use transform::{
    translate_bold_italic_runs, translate_bold_runs, translate_headers, translate_italic_runs,
    translate_link_runs, translate_strikethrough,
};

pub fn markdown_to_mrkdwn(input: &str) -> String {
    let (without_code_blocks, code_blocks) = extract_code_blocks(input);
    let (without_inline_code, inline_codes) = extract_inline_code(&without_code_blocks);
    let with_links = translate_link_runs(&without_inline_code);
    let (without_slack_entities, slack_entities) = extract_slack_entities(&with_links);
    let (without_blockquotes, blockquote_prefixes) =
        extract_blockquote_prefixes(&without_slack_entities);
    let escaped = html_entity_escape(&without_blockquotes);
    let with_headers = translate_headers(&escaped);
    let with_bold_italic = translate_bold_italic_runs(&with_headers);
    let with_bold = translate_bold_runs(&with_bold_italic);
    let with_italic = translate_italic_runs(&with_bold);
    let with_strike = translate_strikethrough(&with_italic);

    let restored_blockquotes = restore_placeholders(
        &with_strike,
        &blockquote_prefixes,
        BLOCKQUOTE_PLACEHOLDER_PREFIX,
    );
    let restored_entities = restore_placeholders(
        &restored_blockquotes,
        &slack_entities,
        SLACK_ENTITY_PLACEHOLDER_PREFIX,
    );
    let restored_inline = restore_placeholders(
        &restored_entities,
        &inline_codes,
        INLINE_CODE_PLACEHOLDER_PREFIX,
    );
    restore_placeholders(
        &restored_inline,
        &code_blocks,
        CODE_BLOCK_PLACEHOLDER_PREFIX,
    )
}

fn restore_placeholders(text: &str, blocks: &[String], placeholder_prefix: &str) -> String {
    let mut result = text.to_string();
    for (index, block) in blocks.iter().enumerate() {
        let needle = format!("{}{}\x00", placeholder_prefix, index);
        result = result.replace(&needle, block);
    }
    result
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn preserves_slack_user_mentions_during_markdown_translation() {
        let rendered = markdown_to_mrkdwn("Please ask **<@U123ABC>** to confirm.");
        assert!(rendered.contains("<@U123ABC>"));
        assert!(!rendered.contains("&lt;@U123ABC&gt;"));
    }
}
