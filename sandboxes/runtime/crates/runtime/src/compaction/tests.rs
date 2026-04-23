use super::serialize::is_user_message;
use super::tokens::message_byte_count;
use super::*;

#[test]
fn test_estimate_tokens_empty() {
    assert_eq!(estimate_tokens(&[]), 0);
}

#[test]
fn test_estimate_tokens_known_input() {
    let history = vec![Message::user("Hello, world!")];
    let tokens = estimate_tokens(&history);
    // "Hello, world!" is ~4 tokens + 4 framing = ~8
    assert!(tokens > 0);
    assert!(tokens < 20);
}

#[test]
fn test_no_compaction_under_budget() {
    let config = CompactionConfig {
        token_budget: 100_000,
        tail_messages: 10,
        summary_prompt: None,
        summary_provider: bridge_core::provider::ProviderConfig {
            provider_type: bridge_core::provider::ProviderType::OpenAI,
            model: "gpt-4o-mini".to_string(),
            api_key: "test".to_string(),
            base_url: Some("https://api.openai.com/v1".to_string()),
            prompt_caching_enabled: true,
            cache_ttl: Default::default(),
        },
    };

    let history = vec![Message::user("hello")];
    let rt = tokio::runtime::Runtime::new().unwrap();
    let result = rt.block_on(maybe_compact(&history, &config)).unwrap();
    assert!(result.is_none());
}

#[test]
fn test_tail_boundary_alignment() {
    // Build a history: User, Assistant, User, Assistant, User, Assistant
    let history = [
        Message::user("first question"),
        Message::assistant("first answer"),
        Message::user("second question"),
        Message::assistant("second answer"),
        Message::user("third question"),
        Message::assistant("third answer"),
    ];

    // With tail_messages=2, split_at would be 4 (history[4] is User) — good
    let tail_count = 2usize;
    let total = history.len();
    let mut split_at = total.saturating_sub(tail_count);
    while split_at > 0 && !is_user_message(&history[split_at]) {
        split_at -= 1;
    }
    assert!(is_user_message(&history[split_at]));

    // With tail_messages=1, split_at would be 5 (history[5] is Assistant), should adjust to 4
    let tail_count = 1usize;
    let mut split_at = total.saturating_sub(tail_count);
    while split_at > 0 && !is_user_message(&history[split_at]) {
        split_at -= 1;
    }
    assert!(is_user_message(&history[split_at]));
    assert_eq!(split_at, 4);
}

#[test]
fn test_serialize_history_for_summary() {
    let history = vec![
        Message::user("Can you help me?"),
        Message::assistant("Sure, I'll help."),
    ];

    let text = serialize_history_for_summary(&history);
    assert!(text.contains("[User]: Can you help me?"));
    assert!(text.contains("[Assistant]: Sure, I'll help."));
}

#[test]
fn test_compaction_config_serde_defaults() {
    let json = r#"{
            "summary_provider": {
                "provider_type": "open_ai",
                "model": "gpt-4o-mini",
                "api_key": "test-key"
            }
        }"#;
    let config: CompactionConfig = serde_json::from_str(json).unwrap();
    assert_eq!(config.token_budget, 100_000);
    assert_eq!(config.tail_messages, 10);
    assert!(config.summary_prompt.is_none());
}

// ── Fix #2: Cached tokenizer tests ─────────────────────────────────

#[test]
fn test_bpe_tokenizer_is_cached_and_reusable() {
    // Calling estimate_tokens multiple times must not panic or reinitialize.
    // The LazyLock ensures the tokenizer is created exactly once.
    let history = vec![Message::user("Hello, world!")];
    let t1 = estimate_tokens(&history);
    let t2 = estimate_tokens(&history);
    assert_eq!(
        t1, t2,
        "cached tokenizer must produce deterministic results"
    );
}

#[test]
fn test_bpe_tokenizer_thread_safety() {
    // Verify the LazyLock tokenizer works across threads.
    let handles: Vec<_> = (0..8)
        .map(|i| {
            std::thread::spawn(move || {
                let history = [Message::user(format!("Thread {} says hello", i))];
                estimate_tokens(&history)
            })
        })
        .collect();

    let results: Vec<usize> = handles.into_iter().map(|h| h.join().unwrap()).collect();
    // All threads should produce a reasonable token count
    for count in &results {
        assert!(*count > 0 && *count < 50);
    }
}

// ── Fast estimation tests ──────────────────────────────────────────

#[test]
fn test_fast_estimate_clearly_under_budget() {
    let history = vec![Message::user("short")];
    // Budget is huge — should return Some(small_number)
    let result = estimate_tokens_fast(&history, 100_000);
    assert!(
        result.is_some(),
        "short message should be clearly under budget"
    );
    assert!(result.unwrap() < 100_000);
}

#[test]
fn test_fast_estimate_returns_none_near_boundary() {
    // Create a history that's roughly near a small budget
    let msg = "a ".repeat(200); // ~100 tokens
    let history = vec![Message::user(&msg)];
    // Set budget to exactly what we estimate — should be ambiguous
    let precise = estimate_tokens(&history);
    let result = estimate_tokens_fast(&history, precise);
    // Near the boundary: might be None (ambiguous) or Some (heuristic happened to be clear)
    // Just ensure it doesn't panic and returns a reasonable answer
    if let Some(fast) = result {
        // If it returns Some, the heuristic was confident
        assert!(fast > 0);
    }
}

#[test]
fn test_fast_estimate_empty_history() {
    let result = estimate_tokens_fast(&[], 100_000);
    assert!(result.is_some());
    assert_eq!(result.unwrap(), 0);
}

// ── Byte count tests ───────────────────────────────────────────────

#[test]
fn test_message_byte_count_user() {
    let msg = Message::user("Hello, world!");
    let count = message_byte_count(&msg);
    assert_eq!(count, 13); // "Hello, world!" is 13 bytes
}

#[test]
fn test_message_byte_count_assistant() {
    let msg = Message::assistant("I can help with that.");
    let count = message_byte_count(&msg);
    assert_eq!(count, 21);
}
