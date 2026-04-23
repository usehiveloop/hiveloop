use anyhow::{anyhow, Context, Result};

use super::TestHarness;

impl TestHarness {
    /// Fetch agents from mock CP via GET /agents, then push them to bridge via POST /push/agents.
    pub async fn push_agents_from_cp(&self) -> Result<()> {
        // Fetch agent definitions from mock control plane
        let resp = self
            .client
            .get(format!("{}/agents", self.cp_base_url))
            .send()
            .await
            .context("GET /agents from CP failed")?;

        let agents: Vec<serde_json::Value> = resp
            .json()
            .await
            .context("failed to parse CP /agents response")?;

        tracing::info!(
            count = agents.len(),
            "fetched agents from mock CP, pushing to bridge"
        );

        // Push to bridge
        let push_resp = self
            .client
            .post(format!("{}/push/agents", self.bridge_base_url))
            .header("authorization", "Bearer e2e-test-key")
            .json(&serde_json::json!({"agents": agents}))
            .send()
            .await
            .context("POST /push/agents to bridge failed")?;

        if !push_resp.status().is_success() {
            let status = push_resp.status();
            let body = push_resp.text().await.unwrap_or_default();
            return Err(anyhow!(
                "failed to push agents to bridge: status={}, body={}",
                status,
                body
            ));
        }

        Ok(())
    }

    /// Push a diff to the bridge via POST /push/diff.
    pub async fn push_diff_to_bridge(
        &self,
        added: &[serde_json::Value],
        updated: &[serde_json::Value],
        removed: &[&str],
    ) -> Result<()> {
        let resp = self
            .client
            .post(format!("{}/push/diff", self.bridge_base_url))
            .header("authorization", "Bearer e2e-test-key")
            .json(&serde_json::json!({
                "added": added,
                "updated": updated,
                "removed": removed,
            }))
            .send()
            .await
            .context("POST /push/diff to bridge failed")?;

        if !resp.status().is_success() {
            let status = resp.status();
            let body = resp.text().await.unwrap_or_default();
            return Err(anyhow!(
                "failed to push diff to bridge: status={}, body={}",
                status,
                body
            ));
        }

        Ok(())
    }

    /// Push a single agent to the bridge via PUT /push/agents/{agent_id}.
    pub async fn push_agent_to_bridge(
        &self,
        agent: &serde_json::Value,
    ) -> Result<reqwest::Response> {
        let agent_id = agent
            .get("id")
            .and_then(|v| v.as_str())
            .ok_or_else(|| anyhow!("agent has no id field"))?;

        let resp = self
            .client
            .put(format!("{}/push/agents/{}", self.bridge_base_url, agent_id))
            .header("authorization", "Bearer e2e-test-key")
            .json(agent)
            .send()
            .await
            .context("PUT /push/agents/{id} to bridge failed")?;

        Ok(resp)
    }

    /// PATCH /push/agents/{agent_id}/api-key — rotate an agent's API key at runtime.
    pub async fn patch_agent_api_key(
        &self,
        agent_id: &str,
        api_key: &str,
    ) -> Result<reqwest::Response> {
        self.client
            .patch(format!(
                "{}/push/agents/{}/api-key",
                self.bridge_base_url, agent_id
            ))
            .header("authorization", "Bearer e2e-test-key")
            .json(&serde_json::json!({"api_key": api_key}))
            .send()
            .await
            .context("PATCH api-key request failed")
    }

    /// POST /agents on the mock control plane — add a new agent definition.
    pub async fn add_agent_to_cp(&self, def: &bridge_core::AgentDefinition) -> Result<()> {
        let resp = self
            .client
            .post(format!("{}/agents", self.cp_base_url))
            .json(def)
            .send()
            .await
            .context("POST agent to CP failed")?;

        if !resp.status().is_success() {
            let status = resp.status();
            let body = resp.text().await.unwrap_or_default();
            return Err(anyhow!(
                "failed to add agent to CP: status={}, body={}",
                status,
                body
            ));
        }

        Ok(())
    }

    /// PUT /agents/{id} on the mock control plane — update an agent definition.
    pub async fn update_agent_in_cp(
        &self,
        id: &str,
        def: &bridge_core::AgentDefinition,
    ) -> Result<()> {
        let resp = self
            .client
            .put(format!("{}/agents/{}", self.cp_base_url, id))
            .json(def)
            .send()
            .await
            .context("PUT agent in CP failed")?;

        if !resp.status().is_success() {
            let status = resp.status();
            let body = resp.text().await.unwrap_or_default();
            return Err(anyhow!(
                "failed to update agent in CP: status={}, body={}",
                status,
                body
            ));
        }

        Ok(())
    }

    /// DELETE /agents/{id} on the mock control plane — remove an agent definition.
    pub async fn delete_agent_from_cp(&self, id: &str) -> Result<()> {
        let resp = self
            .client
            .delete(format!("{}/agents/{}", self.cp_base_url, id))
            .send()
            .await
            .context("DELETE agent from CP failed")?;

        if !resp.status().is_success() {
            let status = resp.status();
            let body = resp.text().await.unwrap_or_default();
            return Err(anyhow!(
                "failed to delete agent from CP: status={}, body={}",
                status,
                body
            ));
        }

        Ok(())
    }
}
