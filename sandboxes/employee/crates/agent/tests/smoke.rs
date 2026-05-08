//! End-to-end smoke test for `AdkAgentRunner`.
//!
//! Skips silently unless an API key is configured. Run with:
//!
//! ```bash
//! OPENAI_API_KEY=... cargo test -p agent --test smoke -- --nocapture
//! ```
//!
//! Or against any OpenAI-compatible endpoint (e.g. Groq):
//!
//! ```bash
//! GROQ_API_KEY=... \
//! SMOKE_BASE_URL=https://api.groq.com/openai/v1 \
//! SMOKE_MODEL=llama-3.3-70b-versatile \
//! SMOKE_API_KEY_ENV=GROQ_API_KEY \
//! cargo test -p agent --test smoke -- --nocapture
//! ```

use agent::{AdkAgentRunner, AgentEvent, AgentRunner, TurnInput};
use domain::{
    AgentDefinition, AgentMeta, ConfigStore, ModelConfig, SessionId, WebhookConfig,
};
use futures::StreamExt;

fn definition_from_env() -> Option<AgentDefinition> {
    let api_key_env = std::env::var("SMOKE_API_KEY_ENV")
        .unwrap_or_else(|_| "OPENAI_API_KEY".to_string());
    if std::env::var(&api_key_env).is_err() {
        eprintln!("[smoke] env `{api_key_env}` not set — skipping");
        return None;
    }

    let base_url = std::env::var("SMOKE_BASE_URL")
        .unwrap_or_else(|_| "https://api.openai.com/v1".to_string());
    let model_id =
        std::env::var("SMOKE_MODEL").unwrap_or_else(|_| "gpt-4o-mini".to_string());

    Some(AgentDefinition {
        agent: AgentMeta {
            name: "smoke-agent".into(),
            description: "smoke test".into(),
            system_prompt:
                "You are a concise assistant. When asked to confirm liveness, reply with exactly the word OK and nothing else."
                    .into(),
        },
        model: ModelConfig::OpenaiCompatible {
            base_url,
            model_id,
            api_key_env,
            temperature: Some(0.0),
            max_output_tokens: Some(32),
            extra_headers: Default::default(),
            fallback: None,
        },
        limits: Default::default(),
        context: Default::default(),
        tools: Vec::new(),
        mcp_servers: Vec::new(),
        skills: Vec::new(),
        subagents: Vec::new(),
        slack: Default::default(),
        webhooks: WebhookConfig {
            url: "http://invalid.local/ignored".into(),
            secret_env: "UNSET".into(),
            events: Vec::new(),
        },
    })
}

#[tokio::test]
async fn end_to_end_openai_compatible() {
    let _ = tracing_subscriber::fmt::try_init();

    let Some(def) = definition_from_env() else {
        return;
    };

    let config = ConfigStore::new(def);
    let runner = AdkAgentRunner::with_in_memory(config, "smoke-app", std::env::temp_dir());
    let session_id = SessionId::from("smoke-session-1");

    let mut stream = runner
        .run_turn(&session_id, TurnInput::text("Confirm liveness."))
        .await
        .expect("run_turn should succeed");

    let mut chunk_count = 0usize;
    let mut final_text: Option<String> = None;
    let mut error: Option<String> = None;

    while let Some(event) = stream.next().await {
        match event {
            AgentEvent::TokenChunk { text } => {
                chunk_count += 1;
                eprint!("{text}");
            }
            AgentEvent::FinalMessage { text } => {
                eprintln!("\n[final] {text}");
                final_text = Some(text);
            }
            AgentEvent::Error { message } => {
                eprintln!("\n[error] {message}");
                error = Some(message);
            }
            _ => {}
        }
    }

    if let Some(err) = error {
        panic!("agent emitted error: {err}");
    }
    let final_text = final_text.expect("FinalMessage should be emitted");
    assert!(!final_text.trim().is_empty(), "final text should be non-empty");
    eprintln!(
        "[smoke] ok: {chunk_count} chunks, final={:?}",
        final_text.trim()
    );
}
