use domain::SessionId;
use slack_morphism::prelude::*;

use super::context::SlackContext;
use super::session_keys::split_session_id;
use crate::{GatewayError, Result};

const THINKING_STATUS_TEXT: &str = "is thinking...";

pub async fn set_thinking(context: &SlackContext, session_id: &SessionId) -> Result<()> {
    set_thread_status_text(context, session_id, THINKING_STATUS_TEXT).await
}

pub async fn clear_status(context: &SlackContext, session_id: &SessionId) -> Result<()> {
    set_thread_status_text(context, session_id, "").await
}

async fn set_thread_status_text(
    context: &SlackContext,
    session_id: &SessionId,
    status_text: &str,
) -> Result<()> {
    let (channel, thread_ts) = split_session_id(session_id)?;
    let session = context.open_api_session();
    let request = SlackApiAssistantThreadsSetStatusRequest::new(
        channel,
        status_text.to_string(),
        thread_ts,
    );
    session
        .assistant_threads_set_status(&request)
        .await
        .map_err(|e| {
            GatewayError::Other(anyhow::anyhow!("assistant.threads.setStatus: {e}"))
        })?;
    Ok(())
}
