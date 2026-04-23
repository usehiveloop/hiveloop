use anyhow::{anyhow, Context, Result};
use std::path::PathBuf;
use std::process::{Command, Stdio};
use std::time::Duration;

use super::types::now_str;
use super::TestHarness;

impl TestHarness {
    /// GET /health
    pub async fn health(&self) -> Result<serde_json::Value> {
        let resp = self
            .client
            .get(format!("{}/health", self.bridge_base_url))
            .send()
            .await
            .context("GET /health request failed")?;

        let body = resp.json().await.context("failed to parse /health body")?;
        Ok(body)
    }

    /// GET /agents — returns list of agents.
    pub async fn get_agents(&self) -> Result<Vec<serde_json::Value>> {
        let resp = self
            .client
            .get(format!("{}/agents", self.bridge_base_url))
            .send()
            .await
            .context("GET /agents request failed")?;

        let body = resp.json().await.context("failed to parse /agents body")?;
        Ok(body)
    }

    /// GET /agents/{id} — returns agent details or error.
    pub async fn get_agent(&self, id: &str) -> Result<reqwest::Response> {
        let resp = self
            .client
            .get(format!("{}/agents/{}", self.bridge_base_url, id))
            .send()
            .await
            .context("GET /agents/{id} request failed")?;

        Ok(resp)
    }

    /// POST /agents/{agent_id}/conversations — create a new conversation.
    pub async fn create_conversation(&self, agent_id: &str) -> Result<reqwest::Response> {
        let resp = self
            .client
            .post(format!(
                "{}/agents/{}/conversations",
                self.bridge_base_url, agent_id
            ))
            .send()
            .await
            .context("POST create conversation request failed")?;

        Ok(resp)
    }

    /// POST /agents/{agent_id}/conversations with a JSON body for tool/MCP scoping,
    /// provider override, or per-conversation MCP servers.
    pub async fn create_conversation_with_body(
        &self,
        agent_id: &str,
        body: serde_json::Value,
    ) -> Result<reqwest::Response> {
        let resp = self
            .client
            .post(format!(
                "{}/agents/{}/conversations",
                self.bridge_base_url, agent_id
            ))
            .json(&body)
            .send()
            .await
            .context("POST create conversation request (with body) failed")?;

        Ok(resp)
    }

    /// Build the mock-portal-mcp binary (if not already built) and return its path.
    /// Used by per-conversation MCP tests that need a spawnable stdio MCP server.
    pub fn ensure_mock_portal_mcp(&self) -> Result<PathBuf> {
        let target_dir = self.workspace_root.join("target").join("debug");
        let binary = target_dir.join("mock-portal-mcp");
        if !binary.exists() {
            let status = Command::new("cargo")
                .args(["build", "-p", "mock-portal-mcp"])
                .current_dir(&self.workspace_root)
                .stdout(Stdio::null())
                .stderr(Stdio::inherit())
                .status()
                .context("failed to run cargo build for mock-portal-mcp")?;
            if !status.success() {
                return Err(anyhow!(
                    "cargo build -p mock-portal-mcp failed with status {}",
                    status
                ));
            }
        }
        if !binary.exists() {
            return Err(anyhow!(
                "mock-portal-mcp binary still missing at {}",
                binary.display()
            ));
        }
        Ok(binary)
    }

    /// POST /conversations/{conv_id}/messages — send a message.
    pub async fn send_message(&self, conv_id: &str, content: &str) -> Result<reqwest::Response> {
        let label = self.log_label(conv_id);
        self.append_log(
            &label,
            &format!(
                "[{}] ================================================================================\n\
                 USER MESSAGE\n\
                 ================================================================================\n\
                 {}\n\n",
                now_str(),
                content
            ),
        );

        let resp = self
            .client
            .post(format!(
                "{}/conversations/{}/messages",
                self.bridge_base_url, conv_id
            ))
            .json(&serde_json::json!({"content": content}))
            .send()
            .await
            .context("POST send message request failed")?;

        Ok(resp)
    }

    /// DELETE /conversations/{conv_id} — end a conversation.
    pub async fn end_conversation(&self, conv_id: &str) -> Result<reqwest::Response> {
        let label = self.log_label(conv_id);
        self.append_log(
            &label,
            &format!(
                "[{}] ================================================================================\n\
                 CONVERSATION ENDED\n\
                 ================================================================================\n\n",
                now_str()
            ),
        );

        let resp = self
            .client
            .delete(format!(
                "{}/conversations/{}",
                self.bridge_base_url, conv_id
            ))
            .send()
            .await
            .context("DELETE end conversation request failed")?;

        Ok(resp)
    }

    /// POST /conversations/{conv_id}/abort — abort the current in-flight turn.
    pub async fn abort_conversation(&self, conv_id: &str) -> Result<reqwest::Response> {
        let label = self.log_label(conv_id);
        self.append_log(
            &label,
            &format!(
                "[{}] ================================================================================\n\
                 CONVERSATION ABORTED\n\
                 ================================================================================\n\n",
                now_str()
            ),
        );

        let resp = self
            .client
            .post(format!(
                "{}/conversations/{}/abort",
                self.bridge_base_url, conv_id
            ))
            .send()
            .await
            .context("POST abort conversation request failed")?;

        Ok(resp)
    }

    /// GET /metrics
    pub async fn get_metrics(&self) -> Result<serde_json::Value> {
        let resp = self
            .client
            .get(format!("{}/metrics", self.bridge_base_url))
            .send()
            .await
            .context("GET /metrics request failed")?;

        let body = resp.json().await.context("failed to parse /metrics body")?;
        Ok(body)
    }

    /// Connect to the SSE stream for a conversation and collect events
    /// until the stream ends or a timeout is reached.
    pub async fn stream_events(&self, conv_id: &str, timeout: Duration) -> Result<Vec<String>> {
        let stream_client = reqwest::Client::builder()
            .timeout(timeout)
            .build()
            .context("failed to build stream client")?;

        let resp = stream_client
            .get(format!(
                "{}/conversations/{}/stream",
                self.bridge_base_url, conv_id
            ))
            .send()
            .await
            .context("GET stream request failed")?;

        let mut events = Vec::new();
        let body = resp.text().await.context("failed to read stream body")?;

        // Parse SSE format: lines starting with "data:" contain event data
        for line in body.lines() {
            if let Some(data) = line.strip_prefix("data:") {
                let data = data.trim();
                if !data.is_empty() {
                    events.push(data.to_string());
                }
            }
        }

        Ok(events)
    }
}
