use std::collections::HashMap;
use std::path::PathBuf;
use std::sync::Arc;

use async_stream::stream;
use domain::{ConfigStore, ModelConfig, ReasoningEffort, SessionId};
use futures::{stream::BoxStream, StreamExt};
use gateway::ChannelGateway;
use mcp::McpRegistry;
use outbound::OutboundEmitter;
use storage::CronJobRepo;
use tools::{JsonTool, LocalBashOperations, LocalFsOperations, ProcessRegistry, ToolBuildContext};

use crate::history::{append_model_message, load_model_history, seed_model_history_from_gateway};
use crate::model_client::ChatModelClient;
use crate::primitives::{
    AgentMessage, CacheControlPolicy, MessagePart, ModelRequest, ModelStreamEvent, ToolCall,
};
use crate::rig_tool_registry::{
    build_agent_tools, emit_tool_error, emit_tool_invoked, DynamicTool, ToolContext,
};
use crate::{AgentError, AgentEvent, AgentRunner, Result, TurnInput};

pub struct RigAgentRunner {
    config: ConfigStore,
    tool_context: ToolBuildContext,
    outbound_emitter: Option<Arc<OutboundEmitter>>,
    gateway: Option<Arc<dyn ChannelGateway>>,
    cron_repo: Option<Arc<dyn CronJobRepo>>,
    event_repo: Option<Arc<dyn storage::EventRepo>>,
    mcp_registry: Option<Arc<McpRegistry>>,
}

impl RigAgentRunner {
    pub fn new(config: ConfigStore, workspace_root: PathBuf) -> Self {
        Self {
            config,
            tool_context: ToolBuildContext {
                workspace_root,
                fs: Arc::new(LocalFsOperations::default()),
                bash: Arc::new(LocalBashOperations::default()),
                process_registry: Arc::new(ProcessRegistry::new()),
            },
            outbound_emitter: None,
            gateway: None,
            cron_repo: None,
            event_repo: None,
            mcp_registry: None,
        }
    }

    pub fn with_outbound_emitter(mut self, emitter: Arc<OutboundEmitter>) -> Self {
        self.outbound_emitter = Some(emitter);
        self
    }

    pub fn with_gateway(mut self, gateway: Arc<dyn ChannelGateway>) -> Self {
        self.gateway = Some(gateway);
        self
    }

    pub fn with_cron_repo(mut self, cron_repo: Arc<dyn CronJobRepo>) -> Self {
        self.cron_repo = Some(cron_repo);
        self
    }

    pub fn with_event_repo(mut self, event_repo: Arc<dyn storage::EventRepo>) -> Self {
        self.event_repo = Some(event_repo);
        self
    }

    pub fn with_mcp_registry(mut self, registry: Arc<McpRegistry>) -> Self {
        self.mcp_registry = Some(registry);
        self
    }
}

#[async_trait::async_trait]
impl AgentRunner for RigAgentRunner {
    async fn run_turn(
        &self,
        session_id: &SessionId,
        user_input: TurnInput,
    ) -> Result<BoxStream<'static, AgentEvent>> {
        let snapshot = self.config.snapshot();
        let model_config = pick_model_for_turn(&snapshot, &user_input);
        let (client, model_id, cache_policy, reasoning_effort, temperature, max_output_tokens) =
            build_model_client(model_config)?;

        let mut messages = build_initial_messages(
            &snapshot.agent.system_prompt,
            session_id,
            user_input,
            self.event_repo.as_deref(),
        )
        .await?;
        if let Some(compaction) = snapshot.context.compaction.as_ref().filter(|c| c.enabled) {
            messages = compact_messages_if_needed(messages, compaction).await?;
        }
        let mut available_tools = build_all_tools(
            &snapshot.tools,
            session_id,
            &self.tool_context,
            &ToolContext {
                gateway: self.gateway.clone(),
                cron_repo: self.cron_repo.clone(),
                process_registry: Some(self.tool_context.process_registry.clone()),
                mcp_registry: self.mcp_registry.clone(),
            },
            self.mcp_registry.clone(),
        );
        available_tools.sort_by(|a, b| a.definition().name.cmp(&b.definition().name));

        let max_turns = snapshot.limits.max_turns_per_session.max(1);
        let session_id = session_id.clone();
        let event_repo = self.event_repo.clone();
        let emitter = self.outbound_emitter.clone();

        Ok(Box::pin(stream! {
            let mut final_text = String::new();
            for _turn in 0..max_turns {
                let definitions = available_tools.iter().map(|tool| tool.definition()).collect();
                let request = ModelRequest {
                    model: model_id.clone(),
                    messages: messages.clone(),
                    tools: definitions,
                    temperature,
                    max_output_tokens,
                    reasoning_effort: reasoning_effort.clone(),
                    cache_policy,
                };

                let mut model_stream = match client.stream(request).await {
                    Ok(stream) => stream,
                    Err(error) => {
                        yield AgentEvent::Error { message: error.to_string() };
                        return;
                    }
                };

                let mut turn_text = String::new();
                let mut tool_calls: Vec<ToolCall> = Vec::new();
                while let Some(event) = model_stream.next().await {
                    match event {
                        Ok(ModelStreamEvent::TextDelta(text)) => {
                            turn_text.push_str(&text);
                            yield AgentEvent::TokenChunk { text };
                        }
                        Ok(ModelStreamEvent::ToolCalls(calls)) => tool_calls.extend(calls),
                        Ok(ModelStreamEvent::Usage(usage)) => {
                            tracing::info!(
                                prompt_tokens = usage.prompt_tokens,
                                completion_tokens = usage.completion_tokens,
                                total_tokens = usage.total_tokens,
                                cached_tokens = usage.cached_tokens,
                                cache_write_tokens = usage.cache_write_tokens,
                                cost = usage.cost,
                                "model usage"
                            );
                        }
                        Ok(ModelStreamEvent::Done) => {}
                        Err(error) => {
                            yield AgentEvent::Error { message: error.to_string() };
                            return;
                        }
                    }
                }

                if tool_calls.is_empty() {
                    final_text = turn_text.clone();
                    if !turn_text.is_empty() {
                        let assistant = AgentMessage::assistant(turn_text);
                        if let Err(error) = append_model_message(event_repo.as_deref(), &session_id, &assistant).await {
                            yield AgentEvent::Error { message: error.to_string() };
                            return;
                        }
                        messages.push(assistant);
                    }
                    break;
                }

                let assistant_tool_calls = AgentMessage::assistant_tool_calls(tool_calls.clone());
                if let Err(error) = append_model_message(event_repo.as_deref(), &session_id, &assistant_tool_calls).await {
                    yield AgentEvent::Error { message: error.to_string() };
                    return;
                }
                messages.push(assistant_tool_calls);
                for call in tool_calls {
                    yield AgentEvent::ToolCall { id: call.id.clone(), tool: call.name.clone(), args: call.arguments.clone() };
                    let Some(tool) = available_tools.iter().find(|tool| tool.definition().name == call.name).cloned() else {
                        let result = json_error(&format!("tool '{}' not found", call.name));
                        let message = AgentMessage::tool_result(call.id, result.to_string());
                        if let Err(error) = append_model_message(event_repo.as_deref(), &session_id, &message).await {
                            yield AgentEvent::Error { message: error.to_string() };
                            return;
                        }
                        messages.push(message);
                        continue;
                    };
                    match tool.call(call.arguments.clone()).await {
                        Ok(result) => {
                            emit_tool_invoked(emitter.clone(), &session_id, &call.name, &call.arguments, &result).await;
                            yield AgentEvent::ToolResult { id: call.id.clone(), result: result.clone() };
                            let message = AgentMessage::tool_result(call.id, result.to_string());
                            if let Err(error) = append_model_message(event_repo.as_deref(), &session_id, &message).await {
                                yield AgentEvent::Error { message: error.to_string() };
                                return;
                            }
                            messages.push(message);
                        }
                        Err(error) => {
                            emit_tool_error(emitter.clone(), &session_id, &call.name, &call.arguments, &error.to_string()).await;
                            let result = json_error(&error.to_string());
                            yield AgentEvent::ToolResult { id: call.id.clone(), result: result.clone() };
                            let message = AgentMessage::tool_result(call.id, result.to_string());
                            if let Err(error) = append_model_message(event_repo.as_deref(), &session_id, &message).await {
                                yield AgentEvent::Error { message: error.to_string() };
                                return;
                            }
                            messages.push(message);
                        }
                    }
                }
            }

            yield AgentEvent::FinalMessage { text: final_text };
        }))
    }
}

async fn build_initial_messages(
    system_prompt: &str,
    session_id: &SessionId,
    input: TurnInput,
    event_repo: Option<&dyn storage::EventRepo>,
) -> Result<Vec<AgentMessage>> {
    let mut messages = vec![AgentMessage::system(system_prompt.to_string())];
    let mut history = load_model_history(event_repo, session_id, 1000).await?;
    if history.is_empty() && !input.prior_history.is_empty() {
        history = seed_model_history_from_gateway(event_repo, session_id, &input.prior_history).await?;
    }
    messages.extend(history);
    let mut user = AgentMessage::user(input.text);
    for image in input.images {
        user.push_part(MessagePart::InlineData { mime_type: image.mime_type, data: image.data });
    }
    append_model_message(event_repo, session_id, &user).await?;
    messages.push(user);
    Ok(messages)
}

async fn compact_messages_if_needed(
    messages: Vec<AgentMessage>,
    config: &domain::CompactionConfig,
) -> Result<Vec<AgentMessage>> {
    let threshold = config.token_threshold;
    let estimated_tokens = estimate_tokens(&messages, config.chars_per_token.max(1));
    if estimated_tokens <= threshold || messages.len() <= config.overlap_event_count as usize + 2 {
        return Ok(messages);
    }

    let keep_count = config.overlap_event_count.max(1) as usize;
    let split_at = messages.len().saturating_sub(keep_count);
    let (older, recent) = messages.split_at(split_at);
    let transcript = older
        .iter()
        .map(message_to_transcript_line)
        .collect::<Vec<_>>()
        .join("\n");
    let summary = summarize_history(&config.summarizer_model, transcript).await?;

    let mut compacted = Vec::new();
    if let Some(system) = messages.first() {
        compacted.push(system.clone());
    }
    compacted.push(AgentMessage::system(format!(
        "Conversation summary so far:\n{summary}"
    )));
    compacted.extend(recent.iter().cloned());
    Ok(compacted)
}

async fn summarize_history(model: &ModelConfig, transcript: String) -> Result<String> {
    let (client, model_id, cache_policy, reasoning_effort, temperature, max_output_tokens) =
        build_model_client(model)?;
    let request = ModelRequest {
        model: model_id,
        messages: vec![
            AgentMessage::system("Summarize the conversation history compactly while preserving user goals, decisions, tool results, and unresolved tasks."),
            AgentMessage::user(transcript),
        ],
        tools: Vec::new(),
        temperature,
        max_output_tokens,
        reasoning_effort,
        cache_policy,
    };
    let mut stream = client.stream(request).await?;
    let mut summary = String::new();
    while let Some(event) = stream.next().await {
        match event? {
            ModelStreamEvent::TextDelta(text) => summary.push_str(&text),
            ModelStreamEvent::Done => break,
            _ => {}
        }
    }
    Ok(summary)
}

fn estimate_tokens(messages: &[AgentMessage], chars_per_token: u32) -> u32 {
    let chars: usize = messages
        .iter()
        .flat_map(|message| message.parts.iter())
        .map(|part| match part {
            MessagePart::Text { text } => text.len(),
            MessagePart::InlineData { data, .. } => data.len(),
        })
        .sum();
    (chars as u32 / chars_per_token.max(1)).max(1)
}

fn message_to_transcript_line(message: &AgentMessage) -> String {
    let role = match message.role {
        crate::primitives::AgentMessageRole::System => "system",
        crate::primitives::AgentMessageRole::User => "user",
        crate::primitives::AgentMessageRole::Assistant => "assistant",
        crate::primitives::AgentMessageRole::Tool => "tool",
    };
    let text = message
        .parts
        .iter()
        .filter_map(|part| match part {
            MessagePart::Text { text } => Some(text.as_str()),
            MessagePart::InlineData { .. } => Some("[inline data]"),
        })
        .collect::<Vec<_>>()
        .join("\n");
    format!("[{role}] {text}")
}

fn pick_model_for_turn<'a>(snapshot: &'a domain::AgentDefinition, input: &TurnInput) -> &'a ModelConfig {
    if !input.images.is_empty() {
        if let Some(model) = snapshot.multimodal_model.as_ref() {
            return model;
        }
    }
    &snapshot.model
}

fn build_model_client(
    model: &ModelConfig,
) -> Result<(ChatModelClient, String, CacheControlPolicy, Option<String>, Option<f32>, Option<u32>)> {
    match model {
        ModelConfig::OpenaiCompatible {
            base_url,
            model_id,
            api_key_env,
            temperature,
            max_output_tokens,
            reasoning_effort,
            ..
        } => {
            let api_key = std::env::var(api_key_env)
                .map_err(|_| AgentError::Model(format!("env var `{api_key_env}` not set")))?;
            let cache_policy = if api_key_env == "OPENROUTER_API_KEY"
                || base_url.contains("openrouter")
                || base_url.contains("127.0.0.1")
            {
                CacheControlPolicy::OpenRouterGeminiEphemeral
            } else {
                CacheControlPolicy::Disabled
            };
            Ok((
                ChatModelClient::new(base_url.clone(), api_key),
                model_id.clone(),
                cache_policy,
                reasoning_effort.map(|effort| match effort {
                    ReasoningEffort::Low => "low".to_string(),
                    ReasoningEffort::Medium => "medium".to_string(),
                    ReasoningEffort::High => "high".to_string(),
                }),
                *temperature,
                *max_output_tokens,
            ))
        }
    }
}

fn build_all_tools(
    specs: &[domain::ToolSpec],
    session_id: &SessionId,
    context: &ToolBuildContext,
    tool_context: &ToolContext,
    mcp_registry: Option<Arc<McpRegistry>>,
) -> Vec<Arc<dyn JsonTool>> {
    let mut tools = tools::build_builtin_tools(specs, context);
    tools.extend(build_agent_tools(specs, session_id, tool_context));
    if let Some(registry) = mcp_registry {
        for def in registry.loaded_tools() {
            let registry = registry.clone();
            let prefixed = def.prefixed_name.clone();
            let definition = tools::ToolDefinition {
                name: prefixed.clone(),
                description: def.description,
                parameters: def.parameters,
            };
            tools.push(Arc::new(DynamicTool::new(definition, move |args| {
                let registry = registry.clone();
                let prefixed = prefixed.clone();
                Box::pin(async move { registry.call_tool(&prefixed, args).await })
            })));
        }
    }
    let mut by_name: HashMap<String, Arc<dyn JsonTool>> = HashMap::new();
    for tool in tools {
        by_name.insert(tool.definition().name, tool);
    }
    let mut tools: Vec<_> = by_name.into_values().collect();
    tools.sort_by(|a, b| a.definition().name.cmp(&b.definition().name));
    tools
}

fn json_error(message: &str) -> serde_json::Value {
    serde_json::json!({"error": message})
}
