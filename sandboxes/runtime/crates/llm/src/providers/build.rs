//! Provider client construction and agent building.

use std::sync::{Arc, OnceLock};
use std::time::Duration;

use bridge_core::provider::{ProviderConfig, ProviderType};
use bridge_core::BridgeError;
use bytes::Bytes;
use reqwest_middleware::{ClientBuilder as MwClientBuilder, ClientWithMiddleware};
use reqwest_retry::policies::ExponentialBackoff;
use reqwest_retry::RetryTransientMiddleware;
use rig::agent::Agent;
use rig::completion::CompletionModel;
use rig::http_client::{
    HttpClientExt, LazyBody, MultipartForm, Result as HttpResult, StreamingResponse,
};
use rig::prelude::CompletionClient;
use rig::wasm_compat::WasmCompatSend;
use tracing::{info, warn};

/// One process-global `ClientWithMiddleware` carrying the retry middleware.
/// Cached so each agent build doesn't re-create the connection pool.
fn shared_retrying_client() -> ClientWithMiddleware {
    static CELL: OnceLock<ClientWithMiddleware> = OnceLock::new();
    CELL.get_or_init(|| {
        // Exponential backoff with jitter, capped at 2 minutes per attempt.
        // 8 retries with a 500ms floor and 120s ceiling gives the policy room
        // to actually use the upper bound (with only 5 retries the doubling
        // sequence tops out around 16s and never reaches the cap). Total
        // worst-case wait across all retries: ~4 minutes.
        let policy = ExponentialBackoff::builder()
            .retry_bounds(Duration::from_millis(500), Duration::from_secs(120))
            .build_with_max_retries(8);
        // `read_timeout` is the maximum gap between bytes on an open response.
        // Some providers (seen in the wild on OpenRouter routing certain
        // reasoning models) accept a streaming request, return headers, then
        // never produce a single SSE chunk. Without this guard the connection
        // sits open until something else times out — eating the conversation's
        // wall budget on one stuck turn. Set higher than the application-level
        // guard in `stream_loop.rs` (5 min) so the friendly app-layer timeout
        // fires first; this is the transport-layer backstop in case the
        // streaming future itself wedges below the polling layer.
        let http = reqwest::Client::builder()
            .read_timeout(Duration::from_secs(360))
            .pool_idle_timeout(Duration::from_secs(90))
            .build()
            .expect("reqwest client builder must not fail with default tls");
        let chain =
            MwClientBuilder::new(http).with(RetryTransientMiddleware::new_with_policy(policy));
        // Cache-control middleware is opt-OUT-able via env so we can
        // bisect issues during benchmarks. Set BRIDGE_DISABLE_CACHE_CONTROL=1
        // to skip injecting cache_control markers entirely.
        let chain = if std::env::var("BRIDGE_DISABLE_CACHE_CONTROL")
            .map(|v| v == "1" || v.eq_ignore_ascii_case("true"))
            .unwrap_or(false)
        {
            tracing::info!("cache_control middleware DISABLED (BRIDGE_DISABLE_CACHE_CONTROL=1)");
            chain
        } else {
            chain.with(super::cache_control_middleware::CacheControlMiddleware)
        };
        // tool_choice middleware: env-controlled, off unless BRIDGE_TOOL_CHOICE
        // is set to "required" / "auto" / "none". See module docs.
        let chain = chain.with(super::tool_choice_middleware::ToolChoiceMiddleware::from_env());
        chain.build()
    })
    .clone()
}

/// Newtype wrapper so we can give the rig client builder a type that
/// satisfies its `H: Default + HttpClientExt` bound. `Default::default()`
/// returns a clone of the process-wide retrying middleware client.
///
/// This is needed because `reqwest_middleware::ClientWithMiddleware` itself
/// does not implement `Default`, but rig 0.35's generic `ClientBuilder::build`
/// requires it. We delegate every `HttpClientExt` method to the inner client
/// so retry behavior is preserved.
#[derive(Debug, Clone)]
pub struct RetryingHttp(ClientWithMiddleware);

impl Default for RetryingHttp {
    fn default() -> Self {
        Self(shared_retrying_client())
    }
}

impl HttpClientExt for RetryingHttp {
    fn send<T, U>(
        &self,
        req: http::Request<T>,
    ) -> impl std::future::Future<Output = HttpResult<http::Response<LazyBody<U>>>>
           + WasmCompatSend
           + 'static
    where
        T: Into<Bytes> + WasmCompatSend,
        U: From<Bytes> + WasmCompatSend + 'static,
    {
        self.0.send(req)
    }

    fn send_multipart<U>(
        &self,
        req: http::Request<MultipartForm>,
    ) -> impl std::future::Future<Output = HttpResult<http::Response<LazyBody<U>>>>
           + WasmCompatSend
           + 'static
    where
        U: From<Bytes> + WasmCompatSend + 'static,
    {
        self.0.send_multipart(req)
    }

    fn send_streaming<T>(
        &self,
        req: http::Request<T>,
    ) -> impl std::future::Future<Output = HttpResult<StreamingResponse>> + WasmCompatSend
    where
        T: Into<Bytes>,
    {
        self.0.send_streaming(req)
    }
}

use crate::prefix_hash::{
    prefix_hash_from_definitions, split_hashes_from_definitions, suspected_volatile_markers,
};

use super::{BridgeAgent, BridgeAgentInner};

/// Build a `BridgeAgent` for the given provider configuration and tools.
///
/// Dispatches on `provider_type` to instantiate the correct native rig client
/// (OpenAI, Anthropic, Gemini, Cohere) and wraps the resulting agent in the
/// corresponding enum variant. OpenAI-compatible providers (Groq, DeepSeek,
/// Mistral, xAI, Together, Fireworks, Ollama, Custom) all use the OpenAI
/// client with a custom base_url.
pub fn create_agent(
    config: &ProviderConfig,
    tools: Vec<crate::tool_adapter::DynamicTool>,
    preamble: &str,
    definition: &bridge_core::agent::AgentDefinition,
) -> Result<BridgeAgent, BridgeError> {
    // Compute prefix hash BEFORE moving `tools` into the builder. The hash
    // fingerprints the exact (preamble || tool_defs) bytes the provider
    // will see — any drift between two calls with identical agent config
    // means our prefix is non-deterministic and cache hits will suffer.
    let tool_defs: Vec<rig::completion::ToolDefinition> =
        tools.iter().map(|t| t.definition_sync()).collect();
    let prefix_hash: Arc<str> = prefix_hash_from_definitions(preamble, &tool_defs).into();
    let (preamble_hash, tools_hash) = split_hashes_from_definitions(preamble, &tool_defs);

    // Hygiene warning: if the preamble looks like it interpolates dynamic
    // content, cache hits will thrash. We only log — never fail — because
    // false positives on static text that happens to mention a year are
    // possible. Grep the logs for `preamble_volatile_markers` if hit rate
    // suddenly drops.
    let markers = suspected_volatile_markers(preamble);
    if !markers.is_empty() {
        warn!(
            provider = %config.provider_type,
            model = %config.model,
            preamble_hash = %preamble_hash,
            markers = ?markers,
            "preamble_volatile_markers_detected"
        );
    }

    info!(
        provider = %config.provider_type,
        model = %config.model,
        prefix_hash = %prefix_hash,
        preamble_hash = %preamble_hash,
        tools_hash = %tools_hash,
        tool_count = tool_defs.len(),
        preamble_bytes = preamble.len(),
        "bridge_agent_built"
    );

    let inner = match config.provider_type {
        // Native Anthropic client
        ProviderType::Anthropic => {
            let client = build_anthropic_client(config)?;
            // P2: enable explicit prompt-cache breakpoints on Anthropic when
            // caching is permitted for this agent. `with_prompt_caching` is
            // on the CompletionModel, not the AgentBuilder — hence the
            // detour through `completion_model(...)`. rig 0.31 places the
            // breakpoints on the last system block and the last message,
            // which is the minimum viable "automatic" layout.
            let mut model = client.completion_model(&config.model);
            if config.prompt_caching_enabled {
                info!(
                    provider = "anthropic",
                    model = %config.model,
                    cache_ttl = ?config.cache_ttl,
                    "anthropic_prompt_caching_enabled"
                );
                model = model.with_prompt_caching();
            }
            let builder = rig::agent::AgentBuilder::new(model);
            let agent = configure_and_build(builder, preamble, definition, tools);
            BridgeAgentInner::Anthropic(agent)
        }
        // Native Gemini client
        ProviderType::Google => {
            let client = build_gemini_client(config)?;
            let builder = client.agent(&config.model);
            let agent = configure_and_build(builder, preamble, definition, tools);
            BridgeAgentInner::Gemini(agent)
        }
        // Native Cohere client
        ProviderType::Cohere => {
            let client = build_cohere_client(config)?;
            let builder = client.agent(&config.model);
            let agent = configure_and_build(builder, preamble, definition, tools);
            BridgeAgentInner::Cohere(agent)
        }
        // OpenAI + all OpenAI-compatible providers
        ProviderType::OpenAI
        | ProviderType::Groq
        | ProviderType::DeepSeek
        | ProviderType::Mistral
        | ProviderType::XAi
        | ProviderType::Together
        | ProviderType::Fireworks
        | ProviderType::Ollama
        | ProviderType::Custom => {
            let client = build_openai_client(config)?;
            let builder = client.agent(&config.model);
            let agent = configure_and_build(builder, preamble, definition, tools);
            BridgeAgentInner::OpenAI(agent)
        }
    };

    Ok(BridgeAgent::from_parts(inner, prefix_hash))
}

/// Apply preamble, temperature, max_tokens, max_turns, and tools to an agent
/// builder of any provider type.
fn configure_and_build<M: CompletionModel>(
    builder: rig::agent::AgentBuilder<M>,
    preamble: &str,
    definition: &bridge_core::agent::AgentDefinition,
    tools: Vec<crate::tool_adapter::DynamicTool>,
) -> Agent<M> {
    let builder = builder.preamble(preamble);

    let builder = if let Some(temp) = definition.config.temperature {
        builder.temperature(temp)
    } else {
        builder
    };

    let builder = if let Some(max_tokens) = definition.config.max_tokens {
        builder.max_tokens(max_tokens as u64)
    } else {
        builder
    };

    let builder = if let Some(max_turns) = definition.config.max_turns {
        builder.default_max_turns(max_turns as usize)
    } else {
        builder
    };

    // Wire json_schema for structured output
    let builder = if let Some(ref json_schema) = definition.config.json_schema {
        // Extract the inner "schema" field (OpenAI format: {name, schema})
        let schema_value = json_schema.get("schema").unwrap_or(json_schema);
        match serde_json::from_value::<schemars::Schema>(schema_value.clone()) {
            Ok(schema) => builder.output_schema_raw(schema),
            Err(e) => {
                tracing::warn!("invalid json_schema, skipping structured output: {}", e);
                builder
            }
        }
    } else {
        builder
    };

    if tools.is_empty() {
        builder.build()
    } else {
        let mut iter = tools.into_iter();
        let first = iter.next().expect("checked non-empty above");
        let mut builder = builder.tool(first);
        for tool in iter {
            builder = builder.tool(tool);
        }
        builder.build()
    }
}

fn require_base_url(config: &ProviderConfig) -> Result<&str, BridgeError> {
    config.base_url.as_deref().ok_or_else(|| {
        BridgeError::ConfigError(format!(
            "provider '{}' requires base_url to be set in the agent definition",
            config.provider_type
        ))
    })
}

pub(crate) fn build_openai_client(
    config: &ProviderConfig,
) -> Result<rig::providers::openai::CompletionsClient<RetryingHttp>, BridgeError> {
    let base_url = require_base_url(config)?;
    rig::providers::openai::CompletionsClient::builder()
        .api_key(&config.api_key)
        .base_url(base_url)
        .http_client(RetryingHttp::default())
        .build()
        .map_err(|e| BridgeError::ProviderError(format!("failed to create OpenAI client: {}", e)))
}

pub(crate) fn build_anthropic_client(
    config: &ProviderConfig,
) -> Result<rig::providers::anthropic::Client<RetryingHttp>, BridgeError> {
    let mut builder = rig::providers::anthropic::Client::builder()
        .api_key(&config.api_key)
        .http_client(RetryingHttp::default());
    if let Some(ref base_url) = config.base_url {
        builder = builder.base_url(base_url);
    }
    // 1-hour cache TTL ships behind a beta header. We set it whenever the
    // caller opts into OneHour so that the moment rig exposes `"ttl":"1h"`
    // on `CacheControl`, existing agents start getting 1-hour writes
    // without a config change. With rig 0.31 the effective TTL is still
    // 5-minute, but the header is a no-op otherwise and safe to send.
    if matches!(config.cache_ttl, bridge_core::provider::CacheTtl::OneHour) {
        builder = builder.anthropic_beta("extended-cache-ttl-2025-04-11");
    }
    builder.build().map_err(|e| {
        BridgeError::ProviderError(format!("failed to create Anthropic client: {}", e))
    })
}

pub(crate) fn build_gemini_client(
    config: &ProviderConfig,
) -> Result<rig::providers::gemini::Client, BridgeError> {
    // Gemini left on the default `reqwest::Client` (no retry middleware)
    // because of the rig 0.35 Capabilities-impl bug; see comment in mod.rs.
    let mut builder = rig::providers::gemini::Client::builder().api_key(&config.api_key);
    if let Some(ref base_url) = config.base_url {
        builder = builder.base_url(base_url);
    }
    builder
        .build()
        .map_err(|e| BridgeError::ProviderError(format!("failed to create Gemini client: {}", e)))
}

pub(crate) fn build_cohere_client(
    config: &ProviderConfig,
) -> Result<rig::providers::cohere::Client<RetryingHttp>, BridgeError> {
    let mut builder = rig::providers::cohere::Client::builder()
        .api_key(&config.api_key)
        .http_client(RetryingHttp::default());
    if let Some(ref base_url) = config.base_url {
        builder = builder.base_url(base_url);
    }
    builder
        .build()
        .map_err(|e| BridgeError::ProviderError(format!("failed to create Cohere client: {}", e)))
}
