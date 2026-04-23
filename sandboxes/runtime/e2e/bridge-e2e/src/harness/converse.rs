use anyhow::{anyhow, Context, Result};
use std::time::{Duration, Instant};

use super::types::ConversationTurn;
use super::TestHarness;

impl TestHarness {
    /// Single-turn conversation helper.
    /// Creates a new conversation, sends one message, collects the full response.
    /// For multi-turn, use `converse_multi_turn`.
    pub async fn converse(
        &self,
        agent_id: &str,
        _conv_id: Option<&str>,
        message: &str,
        timeout: Duration,
    ) -> Result<ConversationTurn> {
        let start = Instant::now();

        // Always create a new conversation for simplicity
        // (the bridge SSE receiver is consumed once, so multi-turn on the same
        //  conversation requires keeping the stream open)
        let resp = self.create_conversation(agent_id).await?;
        let status = resp.status();
        let body: serde_json::Value = resp
            .json()
            .await
            .context("failed to parse create conversation response")?;

        if !status.is_success() {
            return Err(anyhow!(
                "failed to create conversation: status={}, body={}",
                status,
                body
            ));
        }

        let conversation_id = body
            .get("conversation_id")
            .and_then(|v| v.as_str())
            .ok_or_else(|| anyhow!("no conversation_id in response: {}", body))?
            .to_string();

        // Register conversation and log header + system prompt
        self.register_conversation(&conversation_id, agent_id).await;

        // Send message (logging happens inside send_message)
        let msg_resp = self.send_message(&conversation_id, message).await?;
        if !msg_resp.status().is_success() && msg_resp.status().as_u16() != 202 {
            let status = msg_resp.status();
            let body = msg_resp.text().await.unwrap_or_default();
            return Err(anyhow!(
                "failed to send message: status={}, body={}",
                status,
                body
            ));
        }

        // Connect to SSE stream and collect events (logging happens inside)
        let (events, response_text) = self
            .stream_sse_until_done(&conversation_id, timeout)
            .await?;

        // Log the assembled assistant response
        let label = self.log_label(&conversation_id);
        self.append_log(
            &label,
            &format!(
                "\n================================================================================\n\
                 ASSISTANT RESPONSE (complete)\n\
                 ================================================================================\n\
                 {}\n\n\
                 ================================================================================\n\
                 TURN COMPLETED ({:.1}s)\n\
                 ================================================================================\n\n",
                if response_text.is_empty() {
                    "[empty response]"
                } else {
                    &response_text
                },
                start.elapsed().as_secs_f64()
            ),
        );

        Ok(ConversationTurn {
            conversation_id,
            response_text,
            sse_events: events,
            duration: start.elapsed(),
        })
    }
}
