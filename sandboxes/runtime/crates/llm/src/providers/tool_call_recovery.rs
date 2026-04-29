//! Detect leaked tool calls in assistant content.
//!
//! When an inference server's tool-call parser fails (vLLM hermes in streaming
//! mode, Crof's gemma endpoint under load, Xiaomi MiMo direct, OpenRouter's
//! tencent at small budget), the model's native tool-call chat template leaks
//! into the assistant message content as plain text instead of being
//! translated to OpenAI `tool_calls` JSON. The result: bridge sees a
//! non-empty text response with no structured tool_calls and treats the turn
//! as complete, when the model was actually trying to call a tool.
//!
//! This module exposes one function: [`detect_leak`]. Returns true when the
//! content begins with (or prominently contains) a recognizable leak marker.
//! The caller — `handle_got_result` in the runtime — uses the signal to
//! treat the turn as if it were an empty response, triggering the existing
//! recovery path so the model gets a chance to re-emit using the proper
//! structured `tool_calls` field.
//!
//! ## Known leak formats
//!
//! - **Hermes / Qwen / many fine-tunes**: `<tool_call>{json}</tool_call>`
//! - **Llama / MiMo / Mistral function-tag**: `<tool_call>\n<function=name>\n<parameter=key>val</parameter>\n</function>\n</tool_call>`
//! - **Gemma special-token format**: `<|tool_call>...<tool_call|>` (sometimes with `<|"|>` quote markers)
//! - **Tencent / Hunyuan template**: `<tool_calls>\n<tool_call>name<tool_sep>\n<arg_key>k</arg_key>\n<arg_value>v</arg_value>...`
//!
//! The detector only flags content where the marker appears in a position
//! consistent with the model trying to make a call (e.g. as the first
//! non-whitespace tokens, or directly after a `</think>` reasoning block).
//! It avoids false-positives on regular prose that happens to mention these
//! markers (for example, an article explaining the tool-call protocol).

/// Returns `true` when `content` looks like a model's native tool-call
/// template that leaked into the assistant message instead of being
/// translated by the inference server into structured `tool_calls`.
pub fn detect_leak(content: &str) -> bool {
    let trimmed = content.trim_start();
    let head = trimmed.get(..512).unwrap_or(trimmed);

    // Gemma's special-token format. The sequence `<|tool_call>` is unusual
    // enough that any occurrence in the response is a leak (it's never
    // legitimate prose).
    if head.contains("<|tool_call>") || head.contains("<tool_call|>") {
        return true;
    }

    // Tencent / Hunyuan template. Uses `<tool_sep>` and `<arg_key>` /
    // `<arg_value>` — these tags don't appear in legitimate prose. Require
    // both `<tool_sep>` and one of the wrapper tags to avoid matching a
    // single off-context occurrence.
    if head.contains("<tool_sep>") && (head.contains("<tool_calls>") || head.contains("<arg_key>"))
    {
        return true;
    }

    // Llama / MiMo / Mistral `<tool_call>` containing `<function=...>`. The
    // `<function=` opener is the unambiguous signal — there's no realistic
    // prose context for it.
    if head.contains("<function=") && (head.contains("<tool_call>") || head.contains("<parameter="))
    {
        return true;
    }

    // Hermes / Qwen XML. `<tool_call>` opener at the start of the response,
    // optionally after a `</think>` reasoning block. We require it to be at
    // the start (after any whitespace + optional `</think>...`) so that
    // prose discussion of the tag doesn't false-positive.
    let after_think = strip_think_prefix(trimmed);
    if after_think.starts_with("<tool_call>") {
        return true;
    }

    // Qwen2.5-Coder bare-JSON variant: assistant content that begins with
    // `{"name":"...","arguments":...}`. Conservative — only when the JSON
    // object is the FIRST thing in the response and contains both keys.
    if let Some(rest) = after_think.strip_prefix('{') {
        // Cheap check before parsing
        let head = rest.get(..256).unwrap_or(rest);
        if head.contains("\"name\"") && head.contains("\"arguments\"") {
            return true;
        }
    }

    false
}

/// Skip a leading `<think>...</think>` block if present, returning the rest
/// of the content. Used so that reasoning models that emit a thinking block
/// followed by a leaked tool call still trigger detection on the call.
fn strip_think_prefix(s: &str) -> &str {
    let s = s.trim_start();
    let Some(after_open) = s.strip_prefix("<think>") else {
        return s;
    };
    if let Some(close_idx) = after_open.find("</think>") {
        after_open[close_idx + "</think>".len()..].trim_start()
    } else {
        s
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn detects_hermes_qwen_xml() {
        assert!(detect_leak(
            r#"<tool_call>{"name":"bash","arguments":{"command":"ls"}}</tool_call>"#
        ));
    }

    #[test]
    fn detects_hermes_after_think_block() {
        assert!(detect_leak(
            "<think>I should list the directory.</think>\n<tool_call>{\"name\":\"bash\",\"arguments\":{}}</tool_call>"
        ));
    }

    #[test]
    fn detects_mimo_function_tag_leak() {
        assert!(detect_leak(
            "<tool_call>\n<function=bash>\n<parameter=command>which php</parameter>\n<parameter=description>Check PHP</parameter>\n</function>\n</tool_call>"
        ));
    }

    #[test]
    fn detects_gemma_special_tokens() {
        assert!(detect_leak(
            r#"<|tool_call>call:web_fetch{url:<|"|>https://example.com<|"|>}<tool_call|>"#
        ));
    }

    #[test]
    fn detects_tencent_hunyuan_template() {
        assert!(detect_leak(
            "<tool_calls>\n<tool_call>bash<tool_sep>\n<arg_key>command</arg_key>\n<arg_value>ls</arg_value>"
        ));
    }

    #[test]
    fn detects_qwen_bare_json() {
        assert!(detect_leak(
            r#"{"name":"bash","arguments":{"command":"ls -la"}}"#
        ));
    }

    #[test]
    fn detects_qwen_bare_json_after_think() {
        assert!(detect_leak(
            "<think>plan...</think>\n{\"name\":\"bash\",\"arguments\":{\"command\":\"pwd\"}}"
        ));
    }

    #[test]
    fn skips_normal_prose() {
        assert!(!detect_leak(
            "I'll help you debug the workflow. Let me check the YAML."
        ));
        assert!(!detect_leak(
            "Here's the error message: it says permission denied."
        ));
    }

    #[test]
    fn skips_prose_mentioning_tool_call_word() {
        // Discussing the tool_call concept in prose shouldn't false-positive
        assert!(!detect_leak(
            "To make a tool call, the model emits a structured tool_calls field."
        ));
    }

    #[test]
    fn skips_json_without_tool_call_shape() {
        // Just a JSON config, not a tool-call shape
        assert!(!detect_leak(r#"{"version": "1.0", "feature": "enabled"}"#));
    }

    #[test]
    fn skips_empty_or_whitespace() {
        assert!(!detect_leak(""));
        assert!(!detect_leak("   \n  \t  "));
    }

    #[test]
    fn skips_inline_code_block_containing_tool_call() {
        // A markdown code fence around `<tool_call>` is documentation, not a leak
        assert!(!detect_leak(
            "Sure, here's the format:\n```\n<tool_call>...</tool_call>\n```\nUse it like that."
        ));
    }
}
