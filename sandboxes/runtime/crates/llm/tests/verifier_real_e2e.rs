//! Real OpenAI verifier round-trip — Test A from the rollout plan.
//!
//! Gated by `BRIDGE_VERIFIER_OPENAI_API_KEY`. Run with:
//!
//! ```sh
//! BRIDGE_VERIFIER_OPENAI_API_KEY=<provider-api-key> \
//!   cargo test -p llm --test verifier_real_e2e -- --ignored
//! ```
//!
//! Sends a tiny "clean" projection (a one-shot factual answer) and asserts:
//! 1. The call succeeds against the real GPT-5-nano endpoint.
//! 2. The verdict parses against the frozen JSON schema.
//! 3. The verdict is one of `users_turn` / `completed` (not `needs_work`),
//!    because there is genuinely nothing more for the agent to do.

use std::time::Duration;

use llm::{
    ParsedVerdict, Verdict, VerifierBackend, VerifierClient, VerifierRequest,
    VERIFIER_SYSTEM_PROMPT, VERIFIER_VERDICT_SCHEMA,
};
use serde_json::json;

fn key() -> Option<String> {
    std::env::var("BRIDGE_VERIFIER_OPENAI_API_KEY").ok()
}

fn model() -> String {
    std::env::var("BRIDGE_VERIFIER_OPENAI_MODEL").unwrap_or_else(|_| "gpt-5-nano".to_string())
}

fn base_url() -> String {
    std::env::var("BRIDGE_VERIFIER_OPENAI_BASE_URL")
        .unwrap_or_else(|_| "https://api.openai.com/v1".to_string())
}

#[tokio::test]
#[ignore = "requires BRIDGE_VERIFIER_OPENAI_API_KEY"]
async fn real_openai_clean_factual_turn_is_users_turn_or_completed() {
    let Some(api_key) = key() else {
        eprintln!("BRIDGE_VERIFIER_OPENAI_API_KEY not set — skipping");
        return;
    };

    let client = VerifierClient::new(
        VerifierBackend::OpenAI {
            api_key,
            base_url: base_url(),
            model: model(),
        },
        Duration::from_secs(30),
        VERIFIER_SYSTEM_PROMPT,
        VERIFIER_VERDICT_SCHEMA,
    )
    .expect("verifier client builds");

    // Project a clean factual exchange. The agent gave a complete answer
    // and made no tool calls. The verifier should not ask for more work.
    let projection = json!({
        "system_prompt_excerpt": "You are a helpful assistant.",
        "messages": [
            { "role": "user", "text": "What is the capital of France?" },
            { "role": "assistant", "text": "The capital of France is Paris.", "tool_intents": [] }
        ]
    });
    let user_payload = format!(
        "## Agent system prompt\nYou are a helpful assistant.\n\n## Conversation\n{}",
        serde_json::to_string(&projection).unwrap()
    );

    let schema: serde_json::Value = serde_json::from_str(VERIFIER_VERDICT_SCHEMA).unwrap();

    let raw = client
        .verify(VerifierRequest {
            system: VERIFIER_SYSTEM_PROMPT,
            schema: &schema,
            user: &user_payload,
        })
        .await
        .expect("real OpenAI call succeeds");

    eprintln!(
        "verifier_call ok model={} prefix_hash={} input_tokens={} cached={} output_tokens={} latency_ms={} raw={}",
        raw.model_used,
        client.prefix_hash(),
        raw.input_tokens,
        raw.cached_input_tokens,
        raw.output_tokens,
        raw.latency_ms,
        raw.raw_json,
    );

    let parsed = ParsedVerdict::parse(&raw.raw_json).expect("verdict parses against schema");
    assert!(
        matches!(parsed.verdict, Verdict::UsersTurn | Verdict::Completed),
        "expected users_turn or completed for clean factual answer; got {:?} ({})",
        parsed.verdict,
        parsed.instruction
    );
}
