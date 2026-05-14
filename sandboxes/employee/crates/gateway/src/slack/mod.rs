mod content;
mod context;
mod diagnostics;
mod files;
mod filters;
mod format;
mod inbound;
mod reactions;
mod retry;
mod sender;
mod session_keys;
mod status;
mod thread_history;
mod users;

use std::sync::Arc;

use async_trait::async_trait;
use domain::{
    Attachment, ConfigStore, HistoryMessage, InboundEvent, MessageHandle, Reply, SessionId,
};
use slack_morphism::prelude::*;
use tokio::sync::mpsc;
use tracing::{error, info};

use crate::{ChannelGateway, GatewayError, Result};
use context::SlackContext;

pub struct SlackGateway {
    client: Arc<SlackHyperClient>,
    bot_token: SlackApiToken,
    app_token: SlackApiToken,
    context: Arc<SlackContext>,
}

impl SlackGateway {
    pub fn new(
        bot_token: impl Into<String>,
        app_token: impl Into<String>,
        config: ConfigStore,
    ) -> anyhow::Result<Self> {
        let connector = SlackClientHyperConnector::new()?;
        let client = Arc::new(SlackClient::new(connector));
        let bot_token = SlackApiToken::new(SlackApiTokenValue::from(bot_token.into()));
        let app_token = SlackApiToken::new(SlackApiTokenValue::from(app_token.into()));
        let context = Arc::new(SlackContext::new(client.clone(), bot_token.clone(), config));
        Ok(Self {
            client,
            bot_token,
            app_token,
            context,
        })
    }

    async fn auth_test_for_bot_user_id(&self) -> anyhow::Result<String> {
        let session = self.client.open_session(&self.bot_token);
        let response = session.auth_test().await?;
        Ok(response.user_id.0)
    }
}

#[async_trait]
impl ChannelGateway for SlackGateway {
    fn platform(&self) -> &'static str {
        "slack"
    }

    async fn run(&self, sink: mpsc::Sender<InboundEvent>) -> Result<()> {
        let bot_user_id = self
            .auth_test_for_bot_user_id()
            .await
            .map_err(GatewayError::Other)?;
        info!(%bot_user_id, "slack: authed; starting socket mode listener");

        self.context.set_runtime_state(sink, bot_user_id);
        context::install_global(self.context.clone())?;

        let callbacks =
            SlackSocketModeListenerCallbacks::new().with_push_events(inbound::handle_push_event);

        let listener_environment = Arc::new(
            SlackClientEventsListenerEnvironment::new(self.client.clone())
                .with_error_handler(slack_listener_error_handler),
        );

        let listener = SlackClientSocketModeListener::new(
            &SlackClientSocketModeConfig::new(),
            listener_environment,
            callbacks,
        );

        listener
            .listen_for(&self.app_token)
            .await
            .map_err(|e| GatewayError::Transport(e.to_string()))?;
        listener.serve().await;
        Ok(())
    }

    async fn reply(&self, session_id: &SessionId, body: Reply) -> Result<MessageHandle> {
        sender::post_text_reply(&self.context, session_id, body).await
    }

    async fn post_to_channel(&self, channel: &str, body: Reply) -> Result<MessageHandle> {
        sender::post_text_to_channel(&self.context, channel, body).await
    }

    async fn edit(&self, handle: &MessageHandle, body: Reply) -> Result<()> {
        sender::edit_text_reply(&self.context, handle, body).await
    }

    async fn typing(&self, session_id: &SessionId) -> Result<()> {
        status::set_thinking(&self.context, session_id).await
    }

    async fn stop_typing(&self, session_id: &SessionId) -> Result<()> {
        status::clear_status(&self.context, session_id).await
    }

    async fn upload(
        &self,
        session_id: &SessionId,
        bytes: Vec<u8>,
        filename: &str,
        caption: Option<&str>,
    ) -> Result<MessageHandle> {
        files::upload_file(&self.context, session_id, bytes, filename, caption).await
    }

    async fn react(&self, handle: &MessageHandle, emoji: &str) -> Result<()> {
        reactions::add(&self.context, handle, emoji).await
    }

    async fn unreact(&self, handle: &MessageHandle, emoji: &str) -> Result<()> {
        reactions::remove(&self.context, handle, emoji).await
    }

    async fn fetch_thread_history(
        &self,
        session_id: &SessionId,
        limit: u32,
    ) -> Result<Vec<HistoryMessage>> {
        thread_history::fetch_recent(&self.context, session_id, limit).await
    }

    async fn download_attachment(&self, attachment: &Attachment) -> Result<Vec<u8>> {
        files::download_attachment(&self.context, attachment).await
    }
}

fn slack_listener_error_handler(
    err: Box<dyn std::error::Error + Send + Sync>,
    _client: Arc<SlackHyperClient>,
    _states: SlackClientEventsUserState,
) -> http::StatusCode {
    error!(error = %err, "slack listener error");
    http::StatusCode::OK
}
