use bridge_core::{AgentDefinition, BridgeError, RuntimeConfig};
use tokio_util::sync::CancellationToken;
use tracing::{error, info};

use crate::diff::compute_diff;
use crate::updater::apply_diff;

/// Fetch the current list of agent definitions from the control plane.
///
/// Issues a `GET {control_plane_url}/agents` request with a Bearer token
/// and deserialises the JSON response body into a `Vec<AgentDefinition>`.
pub async fn fetch_agents(
    client: &reqwest::Client,
    config: &RuntimeConfig,
) -> Result<Vec<AgentDefinition>, BridgeError> {
    let url = format!("{}/agents", config.control_plane_url.trim_end_matches('/'));

    let response = client
        .get(&url)
        .header(
            "Authorization",
            format!("Bearer {}", config.control_plane_api_key),
        )
        .send()
        .await
        .map_err(|e| BridgeError::Internal(format!("failed to fetch agents: {e}")))?;

    if !response.status().is_success() {
        return Err(BridgeError::Internal(format!(
            "control plane returned status {}",
            response.status()
        )));
    }

    let agents: Vec<AgentDefinition> = response
        .json()
        .await
        .map_err(|e| BridgeError::Internal(format!("failed to parse agents response: {e}")))?;

    Ok(agents)
}

/// Run the continuous sync loop.
///
/// On each tick the loop fetches the latest agent definitions from the
/// control plane, computes a diff against the currently loaded agents,
/// and applies any changes via the supervisor.
///
/// The loop exits when the `cancel` token is cancelled.
pub async fn run_sync_loop(
    supervisor: &runtime::AgentSupervisor,
    client: &reqwest::Client,
    config: &RuntimeConfig,
    cancel: CancellationToken,
) {
    let interval = std::time::Duration::from_secs(config.sync_interval_secs);

    loop {
        tokio::select! {
            _ = cancel.cancelled() => {
                info!("sync loop cancelled");
                return;
            }
            _ = tokio::time::sleep(interval) => {}
        }

        // Fetch latest agents from the control plane
        let fetched = match fetch_agents(client, config).await {
            Ok(agents) => agents,
            Err(e) => {
                error!(error = %e, "sync: failed to fetch agents");
                continue;
            }
        };

        // Compute the diff against currently loaded agents
        let current = supervisor.list_agents();
        let diff = compute_diff(&current, &fetched);

        if diff.is_empty() {
            continue;
        }

        info!(
            added = diff.added.len(),
            updated = diff.updated.len(),
            removed = diff.removed.len(),
            "sync: applying diff"
        );

        if let Err(e) = apply_diff(supervisor, diff).await {
            error!(error = %e, "sync: failed to apply diff");
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use bridge_core::agent::AgentConfig;
    use bridge_core::provider::{ProviderConfig, ProviderType};
    use wiremock::matchers::{header, method, path};
    use wiremock::{Mock, MockServer, ResponseTemplate};

    fn make_definition(id: &str, version: &str) -> AgentDefinition {
        AgentDefinition {
            id: id.to_string(),
            name: format!("Agent {id}"),
            system_prompt: "test".to_string(),
            provider: ProviderConfig {
                provider_type: ProviderType::OpenAI,
                model: "gpt-4o".to_string(),
                api_key: "test-key".to_string(),
                base_url: None,
            },
            tools: vec![],
            mcp_servers: vec![],
            skills: vec![],
            config: AgentConfig::default(),
            subagents: vec![],
            webhook_url: None,
            webhook_secret: None,
            version: Some(version.to_string()),
            updated_at: None,
        }
    }

    fn make_config(url: &str) -> RuntimeConfig {
        RuntimeConfig {
            control_plane_url: url.to_string(),
            control_plane_api_key: "valid-control-plane-token".to_string(),
            ..RuntimeConfig::default()
        }
    }

    #[tokio::test]
    async fn test_fetch_agents_correct_request_format() {
        let mock_server = MockServer::start().await;
        let agents = vec![make_definition("agent1", "1")];
        let body = serde_json::to_string(&agents).unwrap();

        Mock::given(method("GET"))
            .and(path("/agents"))
            .and(header("Authorization", "Bearer valid-control-plane-token"))
            .respond_with(ResponseTemplate::new(200).set_body_string(body))
            .expect(1)
            .mount(&mock_server)
            .await;

        let client = reqwest::Client::new();
        let config = make_config(&mock_server.uri());

        let result = fetch_agents(&client, &config).await;
        assert!(result.is_ok());

        let fetched = result.unwrap();
        assert_eq!(fetched.len(), 1);
        assert_eq!(fetched[0].id, "agent1");
        assert_eq!(fetched[0].version, Some("1".to_string()));
    }

    #[tokio::test]
    async fn test_fetch_agents_server_error() {
        let mock_server = MockServer::start().await;

        Mock::given(method("GET"))
            .and(path("/agents"))
            .respond_with(ResponseTemplate::new(500))
            .expect(1)
            .mount(&mock_server)
            .await;

        let client = reqwest::Client::new();
        let config = make_config(&mock_server.uri());

        let result = fetch_agents(&client, &config).await;
        assert!(result.is_err());
    }

    #[tokio::test]
    async fn test_fetch_agents_empty_list() {
        let mock_server = MockServer::start().await;
        let agents: Vec<AgentDefinition> = vec![];
        let body = serde_json::to_string(&agents).unwrap();

        Mock::given(method("GET"))
            .and(path("/agents"))
            .and(header("Authorization", "Bearer valid-control-plane-token"))
            .respond_with(ResponseTemplate::new(200).set_body_string(body))
            .expect(1)
            .mount(&mock_server)
            .await;

        let client = reqwest::Client::new();
        let config = make_config(&mock_server.uri());

        let result = fetch_agents(&client, &config).await;
        assert!(result.is_ok());
        assert!(result.unwrap().is_empty());
    }

    #[tokio::test]
    async fn test_fetch_agents_trailing_slash_in_url() {
        let mock_server = MockServer::start().await;
        let agents = vec![make_definition("a1", "1")];
        let body = serde_json::to_string(&agents).unwrap();

        Mock::given(method("GET"))
            .and(path("/agents"))
            .and(header("Authorization", "Bearer valid-control-plane-token"))
            .respond_with(ResponseTemplate::new(200).set_body_string(body))
            .expect(1)
            .mount(&mock_server)
            .await;

        let client = reqwest::Client::new();
        // URL with trailing slash
        let config = make_config(&format!("{}/", mock_server.uri()));

        let result = fetch_agents(&client, &config).await;
        assert!(result.is_ok());
        assert_eq!(result.unwrap().len(), 1);
    }
}
