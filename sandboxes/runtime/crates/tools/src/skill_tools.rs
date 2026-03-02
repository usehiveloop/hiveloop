use async_trait::async_trait;
use bridge_core::SkillDefinition;
use schemars::JsonSchema;
use serde::Deserialize;

use crate::ToolExecutor;

// ---------------------------------------------------------------------------
// FetchSkillsTool
// ---------------------------------------------------------------------------

/// Arguments for the FetchSkills tool.
#[derive(Debug, Deserialize, JsonSchema)]
pub struct FetchSkillsArgs {
    /// A search query to filter available skills by title or description.
    pub query: String,
}

/// A tool that searches the agent's available skills using fuzzy matching.
pub struct FetchSkillsTool {
    skills: Vec<SkillDefinition>,
}

impl FetchSkillsTool {
    pub fn new(skills: Vec<SkillDefinition>) -> Self {
        Self { skills }
    }
}

#[async_trait]
impl ToolExecutor for FetchSkillsTool {
    fn name(&self) -> &str {
        "fetch_skills"
    }

    fn description(&self) -> &str {
        "Search available skills by query. Returns matching skills ranked by relevance."
    }

    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::to_value(schemars::schema_for!(FetchSkillsArgs))
            .unwrap_or_else(|_| serde_json::json!({}))
    }

    async fn execute(&self, args: serde_json::Value) -> Result<String, String> {
        let args: FetchSkillsArgs =
            serde_json::from_value(args).map_err(|e| format!("Invalid arguments: {e}"))?;

        let query_lower = args.query.to_lowercase();

        // Score each skill: 2 for title match, 1 for description match, 0 for no match.
        let mut scored: Vec<(u8, &SkillDefinition)> = self
            .skills
            .iter()
            .filter_map(|skill| {
                let title_match = skill.title.to_lowercase().contains(&query_lower);
                let desc_match = skill.description.to_lowercase().contains(&query_lower);

                if title_match {
                    Some((2, skill))
                } else if desc_match {
                    Some((1, skill))
                } else {
                    None
                }
            })
            .collect();

        // Sort by score descending (title matches first).
        scored.sort_by(|a, b| b.0.cmp(&a.0));

        let results: Vec<&SkillDefinition> = scored.into_iter().map(|(_, skill)| skill).collect();

        serde_json::to_string(&results).map_err(|e| format!("Failed to serialize results: {e}"))
    }
}

// ---------------------------------------------------------------------------
// ActivateSkillTool
// ---------------------------------------------------------------------------

/// Arguments for the ActivateSkill tool.
#[derive(Debug, Deserialize, JsonSchema)]
pub struct ActivateSkillArgs {
    /// The unique identifier of the skill to activate.
    pub skill_id: String,
}

/// A tool that activates a skill by fetching its definition from the control plane.
pub struct ActivateSkillTool {
    client: reqwest::Client,
    control_plane_url: String,
    agent_id: String,
}

impl ActivateSkillTool {
    pub fn new(client: reqwest::Client, control_plane_url: String, agent_id: String) -> Self {
        Self {
            client,
            control_plane_url,
            agent_id,
        }
    }
}

#[async_trait]
impl ToolExecutor for ActivateSkillTool {
    fn name(&self) -> &str {
        "activate_skill"
    }

    fn description(&self) -> &str {
        "Activate a skill by its ID. Fetches the full skill definition from the control plane."
    }

    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::to_value(schemars::schema_for!(ActivateSkillArgs))
            .unwrap_or_else(|_| serde_json::json!({}))
    }

    async fn execute(&self, args: serde_json::Value) -> Result<String, String> {
        let args: ActivateSkillArgs =
            serde_json::from_value(args).map_err(|e| format!("Invalid arguments: {e}"))?;

        let url = format!(
            "{}/agents/{}/skills/{}",
            self.control_plane_url.trim_end_matches('/'),
            self.agent_id,
            args.skill_id
        );

        let response = self
            .client
            .get(&url)
            .send()
            .await
            .map_err(|e| format!("Failed to fetch skill: {e}"))?;

        let status = response.status();
        let body = response
            .text()
            .await
            .map_err(|e| format!("Failed to read response body: {e}"))?;

        if !status.is_success() {
            return Err(format!("Control plane returned status {status}: {body}"));
        }

        Ok(body)
    }
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

#[cfg(test)]
mod tests {
    use super::*;
    use wiremock::matchers::{method, path};
    use wiremock::{Mock, MockServer, ResponseTemplate};

    fn make_skills() -> Vec<SkillDefinition> {
        vec![
            SkillDefinition {
                id: "skill-1".to_string(),
                title: "Code Review".to_string(),
                description: "Analyzes pull requests and provides feedback".to_string(),
            },
            SkillDefinition {
                id: "skill-2".to_string(),
                title: "Summarizer".to_string(),
                description: "Summarizes long documents into concise text".to_string(),
            },
            SkillDefinition {
                id: "skill-3".to_string(),
                title: "Translator".to_string(),
                description: "Translates text between languages using code mappings".to_string(),
            },
        ]
    }

    // --- FetchSkillsTool tests ---

    #[tokio::test]
    async fn test_fetch_skills_matches_by_title_substring() {
        let tool = FetchSkillsTool::new(make_skills());
        let args = serde_json::json!({ "query": "Review" });
        let result = tool.execute(args).await.expect("execute");
        let skills: Vec<SkillDefinition> = serde_json::from_str(&result).expect("parse");

        assert_eq!(skills.len(), 1);
        assert_eq!(skills[0].id, "skill-1");
        assert_eq!(skills[0].title, "Code Review");
    }

    #[tokio::test]
    async fn test_fetch_skills_matches_by_description_substring() {
        let tool = FetchSkillsTool::new(make_skills());
        let args = serde_json::json!({ "query": "pull requests" });
        let result = tool.execute(args).await.expect("execute");
        let skills: Vec<SkillDefinition> = serde_json::from_str(&result).expect("parse");

        assert_eq!(skills.len(), 1);
        assert_eq!(skills[0].id, "skill-1");
    }

    #[tokio::test]
    async fn test_fetch_skills_case_insensitive_matching() {
        let tool = FetchSkillsTool::new(make_skills());
        let args = serde_json::json!({ "query": "code review" });
        let result = tool.execute(args).await.expect("execute");
        let skills: Vec<SkillDefinition> = serde_json::from_str(&result).expect("parse");

        assert_eq!(skills.len(), 1);
        assert_eq!(skills[0].id, "skill-1");
    }

    #[tokio::test]
    async fn test_fetch_skills_no_matches_returns_empty_array() {
        let tool = FetchSkillsTool::new(make_skills());
        let args = serde_json::json!({ "query": "nonexistent" });
        let result = tool.execute(args).await.expect("execute");
        let skills: Vec<SkillDefinition> = serde_json::from_str(&result).expect("parse");

        assert!(skills.is_empty());
    }

    #[tokio::test]
    async fn test_fetch_skills_title_match_ranked_above_description_match() {
        let tool = FetchSkillsTool::new(make_skills());
        // "code" appears in skill-1 title ("Code Review") and skill-3 description ("code mappings")
        let args = serde_json::json!({ "query": "code" });
        let result = tool.execute(args).await.expect("execute");
        let skills: Vec<SkillDefinition> = serde_json::from_str(&result).expect("parse");

        assert_eq!(skills.len(), 2);
        // Title match should come first
        assert_eq!(skills[0].id, "skill-1");
        assert_eq!(skills[1].id, "skill-3");
    }

    // --- ActivateSkillTool tests ---

    #[tokio::test]
    async fn test_activate_skill_constructs_correct_url() {
        let server = MockServer::start().await;
        let agent_id = "agent-42";
        let skill_id = "skill-99";
        let response_body = serde_json::json!({
            "id": skill_id,
            "title": "Test Skill",
            "prompt": "You are a test skill."
        });

        Mock::given(method("GET"))
            .and(path(format!("/agents/{agent_id}/skills/{skill_id}")))
            .respond_with(ResponseTemplate::new(200).set_body_json(&response_body))
            .expect(1)
            .mount(&server)
            .await;

        let tool =
            ActivateSkillTool::new(reqwest::Client::new(), server.uri(), agent_id.to_string());

        let args = serde_json::json!({ "skill_id": skill_id });
        let result = tool.execute(args).await.expect("execute");
        let parsed: serde_json::Value = serde_json::from_str(&result).expect("parse");

        assert_eq!(parsed["id"], skill_id);
        assert_eq!(parsed["title"], "Test Skill");
        assert_eq!(parsed["prompt"], "You are a test skill.");
    }

    #[tokio::test]
    async fn test_activate_skill_returns_error_on_non_success_status() {
        let server = MockServer::start().await;
        let agent_id = "agent-1";
        let skill_id = "missing-skill";

        Mock::given(method("GET"))
            .and(path(format!("/agents/{agent_id}/skills/{skill_id}")))
            .respond_with(ResponseTemplate::new(404).set_body_string("Not Found"))
            .expect(1)
            .mount(&server)
            .await;

        let tool =
            ActivateSkillTool::new(reqwest::Client::new(), server.uri(), agent_id.to_string());

        let args = serde_json::json!({ "skill_id": skill_id });
        let err = tool.execute(args).await.unwrap_err();

        assert!(err.contains("404"));
        assert!(err.contains("Not Found"));
    }
}
