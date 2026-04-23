use anyhow::{anyhow, Context, Result};

use super::TestHarness;

impl TestHarness {
    /// GET /agents/{agent_id}/conversations/{conv_id}/approvals — list pending approvals.
    pub async fn list_approvals(
        &self,
        agent_id: &str,
        conv_id: &str,
    ) -> Result<Vec<serde_json::Value>> {
        let resp = self
            .client
            .get(format!(
                "{}/agents/{}/conversations/{}/approvals",
                self.bridge_base_url, agent_id, conv_id
            ))
            .send()
            .await
            .context("GET approvals request failed")?;

        let status = resp.status();
        if !status.is_success() {
            let body = resp.text().await.unwrap_or_default();
            return Err(anyhow!(
                "list approvals failed: status={}, body={}",
                status,
                body
            ));
        }

        let approvals: Vec<serde_json::Value> = resp
            .json()
            .await
            .context("failed to parse approvals response")?;
        Ok(approvals)
    }

    /// POST /agents/{agent_id}/conversations/{conv_id}/approvals/{request_id}
    /// — resolve a single approval request.
    pub async fn resolve_approval(
        &self,
        agent_id: &str,
        conv_id: &str,
        request_id: &str,
        decision: &str,
    ) -> Result<reqwest::Response> {
        let resp = self
            .client
            .post(format!(
                "{}/agents/{}/conversations/{}/approvals/{}",
                self.bridge_base_url, agent_id, conv_id, request_id
            ))
            .json(&serde_json::json!({"decision": decision}))
            .send()
            .await
            .context("POST resolve approval request failed")?;

        Ok(resp)
    }

    /// POST /agents/{agent_id}/conversations/{conv_id}/approvals
    /// — bulk resolve multiple approval requests.
    pub async fn bulk_resolve_approvals(
        &self,
        agent_id: &str,
        conv_id: &str,
        request_ids: &[String],
        decision: &str,
    ) -> Result<serde_json::Value> {
        let resp = self
            .client
            .post(format!(
                "{}/agents/{}/conversations/{}/approvals",
                self.bridge_base_url, agent_id, conv_id
            ))
            .json(&serde_json::json!({
                "request_ids": request_ids,
                "decision": decision,
            }))
            .send()
            .await
            .context("POST bulk resolve approvals request failed")?;

        let status = resp.status();
        if !status.is_success() {
            let body = resp.text().await.unwrap_or_default();
            return Err(anyhow!(
                "bulk resolve failed: status={}, body={}",
                status,
                body
            ));
        }

        resp.json()
            .await
            .context("failed to parse bulk resolve response")
    }
}
