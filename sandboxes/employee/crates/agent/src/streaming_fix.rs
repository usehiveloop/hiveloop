use std::sync::{Arc, Mutex};

use adk_rust::prelude::{Content, Part};
use adk_rust::{AdkError, Llm, LlmRequest, LlmResponse, LlmResponseStream};
use async_trait::async_trait;
use futures::StreamExt;

pub struct AccumulatingStreamLlm<L: Llm + 'static> {
    inner: Arc<L>,
}

impl<L: Llm + 'static> AccumulatingStreamLlm<L> {
    pub fn wrap(inner: Arc<L>) -> Arc<Self> {
        Arc::new(Self { inner })
    }
}

#[async_trait]
impl<L: Llm + 'static> Llm for AccumulatingStreamLlm<L> {
    fn name(&self) -> &str {
        self.inner.name()
    }

    async fn generate_content(
        &self,
        request: LlmRequest,
        stream: bool,
    ) -> Result<LlmResponseStream, AdkError> {
        let inner_stream = self.inner.generate_content(request, stream).await?;
        let accumulator = Arc::new(Mutex::new(String::new()));
        let patched = inner_stream.map(move |result| {
            result.map(|mut response| {
                if response.partial {
                    capture_partial_text(&response, &accumulator);
                } else {
                    ensure_final_event_has_text(&mut response, &accumulator);
                }
                response
            })
        });
        Ok(Box::pin(patched))
    }
}

fn capture_partial_text(response: &LlmResponse, accumulator: &Arc<Mutex<String>>) {
    let Some(content) = response.content.as_ref() else {
        return;
    };
    let mut buffer = accumulator.lock().unwrap();
    for part in &content.parts {
        if let Part::Text { text } = part {
            buffer.push_str(text);
        }
    }
}

fn ensure_final_event_has_text(response: &mut LlmResponse, accumulator: &Arc<Mutex<String>>) {
    let accumulated_text = accumulator.lock().unwrap().clone();
    if accumulated_text.is_empty() {
        return;
    }
    let already_has_text = response
        .content
        .as_ref()
        .map(|content| {
            content
                .parts
                .iter()
                .any(|part| matches!(part, Part::Text { text } if !text.is_empty()))
        })
        .unwrap_or(false);
    if already_has_text {
        return;
    }
    let mut content = response
        .content
        .take()
        .unwrap_or_else(|| Content::new("model"));
    content.parts.push(Part::Text {
        text: accumulated_text,
    });
    if content.role.is_empty() {
        content.role = "model".to_string();
    }
    response.content = Some(content);
}
