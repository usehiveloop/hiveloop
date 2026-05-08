use std::sync::Arc;

use adk_rust::prelude::*;
use adk_rust::model::ReasoningEffort as AdkReasoningEffort;
use domain::ModelConfig;

use crate::streaming_fix::AccumulatingStreamLlm;
use crate::{AgentError, ImageInput, Result, TurnInput};

pub fn build_model(model: &ModelConfig) -> Result<Arc<AccumulatingStreamLlm<OpenAIClient>>> {
    match model {
        ModelConfig::OpenaiCompatible {
            base_url,
            model_id,
            api_key_env,
            reasoning_effort,
            ..
        } => {
            let api_key = std::env::var(api_key_env)
                .map_err(|_| AgentError::Model(format!("env var `{api_key_env}` not set")))?;
            let mut cfg = OpenAIConfig::compatible(api_key, base_url.clone(), model_id.clone());
            if let Some(effort) = reasoning_effort {
                cfg = cfg.with_reasoning_effort(map_reasoning_effort(*effort));
            }
            let client = OpenAIClient::new(cfg)
                .map_err(|e| AgentError::Model(format!("OpenAIClient init: {e}")))?;
            Ok(AccumulatingStreamLlm::wrap(Arc::new(client)))
        }
    }
}

fn map_reasoning_effort(effort: domain::ReasoningEffort) -> AdkReasoningEffort {
    match effort {
        domain::ReasoningEffort::Low => AdkReasoningEffort::Low,
        domain::ReasoningEffort::Medium => AdkReasoningEffort::Medium,
        domain::ReasoningEffort::High => AdkReasoningEffort::High,
    }
}

pub fn pick_model_for_turn<'a>(
    snapshot: &'a domain::AgentDefinition,
    user_input: &TurnInput,
    session_has_image_history: bool,
) -> &'a ModelConfig {
    let needs_vision = !user_input.images.is_empty() || session_has_image_history;
    if needs_vision {
        if let Some(multimodal) = snapshot.multimodal_model.as_ref() {
            tracing::info!(
                model = %describe_model(multimodal),
                session_has_image_history,
                inbound_has_images = !user_input.images.is_empty(),
                "routing turn to multimodal model"
            );
            return multimodal;
        }
    }
    tracing::info!(model = %describe_model(&snapshot.model), "routing turn to text model");
    &snapshot.model
}

pub fn describe_model(config: &ModelConfig) -> String {
    match config {
        ModelConfig::OpenaiCompatible {
            base_url, model_id, ..
        } => format!("{model_id} @ {base_url}"),
    }
}

pub fn build_user_content(text: String, images: Vec<ImageInput>) -> Content {
    let mut content = Content::new("user").with_text(text);
    for image in images {
        content = content.with_inline_data(image.mime_type, image.data);
    }
    content
}

pub fn build_summarizer_llm(model: &ModelConfig) -> Result<Arc<dyn Llm>> {
    match model {
        ModelConfig::OpenaiCompatible { base_url, model_id, api_key_env, .. } => {
            let api_key = std::env::var(api_key_env)
                .map_err(|_| AgentError::Model(format!("env var `{api_key_env}` not set")))?;
            let cfg = OpenAIConfig::compatible(api_key, base_url.clone(), model_id.clone());
            let client = OpenAIClient::new(cfg)
                .map_err(|e| AgentError::Model(format!("OpenAIClient init: {e}")))?;
            Ok(Arc::new(client))
        }
    }
}
