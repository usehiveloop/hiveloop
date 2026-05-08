//! adk-rust API surface spike.
//!
//! Validates: OpenAI-compatible config, LlmAgentBuilder, Runner, streaming.
//! Run with:
//!
//! ```bash
//! OPENAI_API_KEY=... cargo run -p agent --example spike
//! ```
//!
//! Or against any OpenAI-compatible endpoint:
//!
//! ```bash
//! GROQ_API_KEY=... \
//! SPIKE_BASE_URL=https://api.groq.com/openai/v1 \
//! SPIKE_MODEL=llama-3.3-70b-versatile \
//! SPIKE_API_KEY_ENV=GROQ_API_KEY \
//! cargo run -p agent --example spike
//! ```

use std::sync::Arc;

use adk_rust::prelude::*;
use adk_rust::{SessionId, UserId};
use futures::StreamExt;

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| tracing_subscriber::EnvFilter::new("info")),
        )
        .init();

    let base_url = std::env::var("SPIKE_BASE_URL")
        .unwrap_or_else(|_| "https://api.openai.com/v1".to_string());
    let model_id = std::env::var("SPIKE_MODEL").unwrap_or_else(|_| "gpt-4o-mini".to_string());
    let api_key_env =
        std::env::var("SPIKE_API_KEY_ENV").unwrap_or_else(|_| "OPENAI_API_KEY".to_string());
    let api_key = std::env::var(&api_key_env)
        .map_err(|_| anyhow::anyhow!("env var {api_key_env} not set"))?;

    tracing::info!(%base_url, %model_id, "spike starting");

    let cfg = OpenAIConfig::compatible(api_key, base_url, model_id);
    let model = Arc::new(OpenAIClient::new(cfg)?);

    let agent: Arc<dyn Agent> = Arc::new(
        LlmAgentBuilder::new("spike-agent")
            .instruction("You are a concise assistant. Reply in one short sentence.")
            .model(model)
            .build()?,
    );

    let sessions: Arc<dyn adk_rust::session::SessionService> =
        Arc::new(InMemorySessionService::new());

    let app_name = "spike";
    let user_id = UserId::new("user-1")?;
    let session_id = SessionId::new("session-1")?;

    sessions
        .create(adk_rust::session::CreateRequest {
            app_name: app_name.into(),
            user_id: user_id.to_string(),
            session_id: Some(session_id.to_string()),
            state: Default::default(),
        })
        .await?;

    let runner = Runner::builder()
        .app_name(app_name)
        .agent(agent)
        .session_service(sessions)
        .build()?;

    let user_message = Content::new("user").with_text("Say hello and confirm you are alive.");
    let mut stream = runner.run(user_id, session_id, user_message).await?;

    while let Some(event) = stream.next().await {
        let event = event?;
        if let Some(content) = &event.llm_response.content {
            for part in &content.parts {
                if let Part::Text { text } = part {
                    print!("{text}");
                    use std::io::Write;
                    std::io::stdout().flush().ok();
                }
            }
        }
    }
    println!();

    Ok(())
}
