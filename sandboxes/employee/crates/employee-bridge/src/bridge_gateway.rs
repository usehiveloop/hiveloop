use std::sync::Arc;

use async_trait::async_trait;
use domain::{Attachment, HistoryMessage, InboundEvent, MessageHandle, Reply, SessionId};
use gateway::{ChannelGateway, GatewayError, Result};
use tokio::sync::mpsc;

pub struct BridgeGateway {
    slack: Arc<dyn ChannelGateway>,
    http_streams: Arc<api::HttpStreamBroker>,
}

impl BridgeGateway {
    pub fn new(slack: Arc<dyn ChannelGateway>, http_streams: Arc<api::HttpStreamBroker>) -> Self {
        Self {
            slack,
            http_streams,
        }
    }

    fn is_http_session(session_id: &SessionId) -> bool {
        session_id.as_str().starts_with("http-")
    }

    fn stream_id_from_handle(handle: &MessageHandle) -> Option<&str> {
        (handle.channel == "http").then_some(handle.ts.as_str())
    }
}

#[async_trait]
impl ChannelGateway for BridgeGateway {
    fn platform(&self) -> &'static str {
        "bridge"
    }

    async fn run(&self, sink: mpsc::Sender<InboundEvent>) -> Result<()> {
        self.slack.run(sink).await
    }

    async fn reply(&self, session_id: &SessionId, body: Reply) -> Result<MessageHandle> {
        if Self::is_http_session(session_id) {
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
            return Ok(MessageHandle {
                channel: "http".to_string(),
                ts: stream_id,
            });
        }
        self.slack.reply(session_id, body).await
    }

    async fn post_to_channel(&self, channel: &str, body: Reply) -> Result<MessageHandle> {
        self.slack.post_to_channel(channel, body).await
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
        self.slack.edit(handle, body).await
    }

    async fn typing(&self, session_id: &SessionId) -> Result<()> {
        if Self::is_http_session(session_id) {
            return Ok(());
        }
        self.slack.typing(session_id).await
    }

    async fn stop_typing(&self, session_id: &SessionId) -> Result<()> {
        if Self::is_http_session(session_id) {
            return Ok(());
        }
        self.slack.stop_typing(session_id).await
    }

    async fn upload(
        &self,
        session_id: &SessionId,
        bytes: Vec<u8>,
        filename: &str,
        caption: Option<&str>,
    ) -> Result<MessageHandle> {
        if Self::is_http_session(session_id) {
            return Err(GatewayError::Unsupported("http upload"));
        }
        self.slack
            .upload(session_id, bytes, filename, caption)
            .await
    }

    async fn react(&self, handle: &MessageHandle, emoji: &str) -> Result<()> {
        if Self::stream_id_from_handle(handle).is_some() {
            return Ok(());
        }
        self.slack.react(handle, emoji).await
    }

    async fn unreact(&self, handle: &MessageHandle, emoji: &str) -> Result<()> {
        if Self::stream_id_from_handle(handle).is_some() {
            return Ok(());
        }
        self.slack.unreact(handle, emoji).await
    }

    async fn fetch_thread_history(
        &self,
        session_id: &SessionId,
        limit: u32,
    ) -> Result<Vec<HistoryMessage>> {
        if Self::is_http_session(session_id) {
            return Ok(Vec::new());
        }
        self.slack.fetch_thread_history(session_id, limit).await
    }

    async fn download_attachment(&self, attachment: &Attachment) -> Result<Vec<u8>> {
        self.slack.download_attachment(attachment).await
    }
}

fn reply_to_text(body: Reply) -> String {
    match body {
        Reply::Text(text) => text,
        Reply::Rich(value) => value.to_string(),
    }
}
