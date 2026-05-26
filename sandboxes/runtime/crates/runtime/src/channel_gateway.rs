use std::sync::Arc;

use async_trait::async_trait;
use domain::{Attachment, MessageHandle, Reply, SessionId};
use gateway::{ChannelGateway, GatewayError, Result};

pub struct RuntimeGateway {
    http_streams: Arc<api::HttpStreamBroker>,
    client: reqwest::Client,
}

impl RuntimeGateway {
    pub fn new(http_streams: Arc<api::HttpStreamBroker>) -> Self {
        Self {
            http_streams,
            client: reqwest::Client::new(),
        }
    }

    fn stream_id_from_handle(handle: &MessageHandle) -> Option<&str> {
        (handle.channel == "http").then_some(handle.ts.as_str())
    }
}

#[async_trait]
impl ChannelGateway for RuntimeGateway {
    async fn reply(&self, session_id: &SessionId, body: Reply) -> Result<MessageHandle> {
        let stream_id = self
            .http_streams
            .stream_id_for_session(session_id.as_str())
            .await
            .unwrap_or_else(|| session_id.as_str().trim_start_matches("http-").to_string());
        let text = reply_to_text(body);
        self.http_streams
            .publish(
                &stream_id,
                "status",
                serde_json::json!({
                    "session_id": session_id.as_str(),
                    "text": text,
                }),
            )
            .await;
        Ok(MessageHandle {
            channel: "http".to_string(),
            ts: stream_id,
        })
    }

    async fn edit(&self, handle: &MessageHandle, body: Reply) -> Result<()> {
        if let Some(stream_id) = Self::stream_id_from_handle(handle) {
            self.http_streams
                .publish(
                    stream_id,
                    "edit",
                    serde_json::json!({
                        "text": reply_to_text(body),
                    }),
                )
                .await;
            return Ok(());
        }
        Err(GatewayError::Unsupported("non-http edit handle"))
    }

    async fn download_attachment(&self, attachment: &Attachment) -> Result<Vec<u8>> {
        let url = reqwest::Url::parse(&attachment.url).map_err(|error| {
            GatewayError::Other(anyhow::anyhow!("invalid attachment url: {error}"))
        })?;
        match url.scheme() {
            "http" | "https" => {}
            _ => return Err(GatewayError::Unsupported("attachment url scheme")),
        }
        let response = self
            .client
            .get(url)
            .send()
            .await
            .map_err(|error| GatewayError::Transport(error.to_string()))?
            .error_for_status()
            .map_err(|error| GatewayError::Transport(error.to_string()))?;
        let bytes = response
            .bytes()
            .await
            .map_err(|error| GatewayError::Transport(error.to_string()))?;
        Ok(bytes.to_vec())
    }
}

fn reply_to_text(body: Reply) -> String {
    match body {
        Reply::Text(text) => text,
        Reply::Rich(value) => value.to_string(),
    }
}
