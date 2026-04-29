//! Direct HTTP client for the verifier agent.
//!
//! Bypasses [`crate::providers`] / rig because the verifier needs no tools, no
//! streaming, no agent state, and must pin OpenAI Structured Outputs
//! `strict: true` for grammar-constrained schema enforcement. The body order
//! is locked so the stable prefix (verifier system prompt + JSON schema) is
//! re-used by OpenAI's automatic prompt cache call after call.

use std::time::{Duration, Instant};

use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};

#[derive(Debug, thiserror::Error)]
pub enum VerifierError {
    #[error("verifier transport error: {0}")]
    Transport(String),
    #[error("verifier non-success status {status}: {body}")]
    Status { status: u16, body: String },
    #[error("verifier response missing content")]
    MissingContent,
    #[error("verifier response could not be parsed: {0}")]
    Parse(String),
    #[error("verifier call timed out after {0}ms")]
    Timeout(u64),
}

/// What the verifier was asked. The `system` and `schema` are byte-stable
/// across the lifetime of one process; only `user` varies per call.
pub struct VerifierRequest<'a> {
    pub system: &'a str,
    pub schema: &'a serde_json::Value,
    pub user: &'a str,
}

/// Raw response from the verifier model. Caller is responsible for parsing
/// `raw_json` against the verdict schema.
#[derive(Debug, Clone)]
pub struct VerifierRawResponse {
    pub raw_json: String,
    pub model_used: String,
    pub input_tokens: u64,
    pub cached_input_tokens: u64,
    pub output_tokens: u64,
    pub latency_ms: u64,
}

#[derive(Debug, Clone)]
pub enum VerifierBackend {
    OpenAI {
        api_key: String,
        base_url: String,
        model: String,
    },
}

#[derive(Debug, Clone)]
pub struct VerifierClient {
    primary: VerifierBackend,
    timeout: Duration,
    prefix_hash: String,
    http: reqwest::Client,
}

impl VerifierClient {
    /// Build a client with the given backend, request timeout, and stable
    /// prefix bytes (system prompt + schema bytes). The hash is logged on
    /// every call so drift in the supposedly-stable prefix shows up in
    /// telemetry — same diagnostic shape as `BridgeAgent::prefix_hash`.
    pub fn new(
        primary: VerifierBackend,
        timeout: Duration,
        verifier_system_prompt: &str,
        schema_bytes: &str,
    ) -> Result<Self, VerifierError> {
        let http = reqwest::Client::builder()
            .timeout(timeout)
            .build()
            .map_err(|e| VerifierError::Transport(e.to_string()))?;

        let mut h = Sha256::new();
        h.update(verifier_system_prompt.as_bytes());
        h.update(schema_bytes.as_bytes());
        let prefix_hash = format!("{:x}", h.finalize());

        Ok(Self {
            primary,
            timeout,
            prefix_hash,
            http,
        })
    }

    pub fn prefix_hash(&self) -> &str {
        &self.prefix_hash
    }

    pub async fn verify(
        &self,
        req: VerifierRequest<'_>,
    ) -> Result<VerifierRawResponse, VerifierError> {
        match &self.primary {
            VerifierBackend::OpenAI {
                api_key,
                base_url,
                model,
            } => self.call_openai(api_key, base_url, model, req).await,
        }
    }

    async fn call_openai(
        &self,
        api_key: &str,
        base_url: &str,
        model: &str,
        req: VerifierRequest<'_>,
    ) -> Result<VerifierRawResponse, VerifierError> {
        let url = format!("{}/chat/completions", base_url.trim_end_matches('/'));

        let body = serde_json::json!({
            "model": model,
            "messages": [
                { "role": "system", "content": req.system },
                { "role": "user",   "content": req.user },
            ],
            "response_format": {
                "type": "json_schema",
                "json_schema": {
                    "name": "verifier_verdict",
                    "schema": req.schema,
                    "strict": true,
                }
            },
            "temperature": 0,
            "max_tokens": 200,
        });

        let start = Instant::now();
        let resp = tokio::time::timeout(
            self.timeout,
            self.http.post(&url).bearer_auth(api_key).json(&body).send(),
        )
        .await
        .map_err(|_| VerifierError::Timeout(self.timeout.as_millis() as u64))?
        .map_err(|e| VerifierError::Transport(e.to_string()))?;

        let status = resp.status();
        if !status.is_success() {
            let body = resp.text().await.unwrap_or_default();
            return Err(VerifierError::Status {
                status: status.as_u16(),
                body,
            });
        }

        let parsed: OpenAIChatCompletion = resp
            .json()
            .await
            .map_err(|e| VerifierError::Parse(e.to_string()))?;
        let latency_ms = start.elapsed().as_millis() as u64;

        let raw_json = parsed
            .choices
            .into_iter()
            .next()
            .and_then(|c| c.message.content)
            .ok_or(VerifierError::MissingContent)?;

        let usage = parsed.usage.unwrap_or_default();
        let cached_input_tokens = usage
            .prompt_tokens_details
            .as_ref()
            .map(|d| d.cached_tokens)
            .unwrap_or(0);

        Ok(VerifierRawResponse {
            raw_json,
            model_used: model.to_string(),
            input_tokens: usage.prompt_tokens,
            cached_input_tokens,
            output_tokens: usage.completion_tokens,
            latency_ms,
        })
    }
}

#[derive(Deserialize)]
struct OpenAIChatCompletion {
    choices: Vec<OpenAIChoice>,
    #[serde(default)]
    usage: Option<OpenAIUsage>,
}

#[derive(Deserialize)]
struct OpenAIChoice {
    message: OpenAIMessage,
}

#[derive(Deserialize)]
struct OpenAIMessage {
    #[serde(default)]
    content: Option<String>,
}

#[derive(Default, Deserialize)]
struct OpenAIUsage {
    #[serde(default)]
    prompt_tokens: u64,
    #[serde(default)]
    completion_tokens: u64,
    #[serde(default)]
    prompt_tokens_details: Option<OpenAIPromptDetails>,
}

#[derive(Deserialize)]
struct OpenAIPromptDetails {
    #[serde(default)]
    cached_tokens: u64,
}

/// JSON schema for a verifier verdict. Frozen bytes — used as the cache
/// prefix target. Do not edit casually.
pub const VERIFIER_VERDICT_SCHEMA: &str = r#"{
  "type": "object",
  "additionalProperties": false,
  "required": ["verdict", "confidence", "reason"],
  "properties": {
    "verdict": {
      "type": "string",
      "enum": ["users_turn", "completed", "needs_work"]
    },
    "confidence": {
      "type": "string",
      "enum": ["low", "high"]
    },
    "reason": {
      "type": "string",
      "description": "One short sentence. If needs_work, name the specific gap."
    }
  }
}"#;

/// Stable verifier system prompt. Same bytes every call → automatic prefix
/// cache hit on OpenAI from the second call onward in a session.
pub const VERIFIER_SYSTEM_PROMPT: &str = r#"You are a verifier. You judge whether an AI agent's most recent turn is genuinely complete or has stopped prematurely.

You receive:
- The agent's own system prompt.
- The conversation: user messages, the agent's text replies, and a high-level intent log of any tool actions (no tool names, arguments, or outputs).

Return one of three verdicts:
- "users_turn":  The agent has reasonably handed control back to the user (asked a clarifying question, responded to a non-task message, or fully completed the work and is awaiting next instructions).
- "completed":   The agent finished a task and stated that explicitly.
- "needs_work":  The agent stopped before genuinely finishing what was asked and should keep going.

Confidence:
- Use "high" only when the evidence is unambiguous.
- Use "low" when the situation is plausibly either way — terse-but-correct answers, ambiguous task boundaries, or any mid-flight uncertainty.

Bias toward "users_turn" / "completed" with "low" confidence on borderline cases. The runtime only re-prompts the agent on "needs_work" + "high" confidence.

Always return JSON conforming to the provided schema."#;

#[derive(Serialize, Deserialize, Debug, Clone, Copy, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum Verdict {
    UsersTurn,
    Completed,
    NeedsWork,
}

#[derive(Serialize, Deserialize, Debug, Clone, Copy, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum Confidence {
    Low,
    High,
}

#[derive(Serialize, Deserialize, Debug, Clone)]
pub struct ParsedVerdict {
    pub verdict: Verdict,
    pub confidence: Confidence,
    pub reason: String,
}

impl ParsedVerdict {
    pub fn parse(raw_json: &str) -> Result<Self, VerifierError> {
        serde_json::from_str(raw_json).map_err(|e| VerifierError::Parse(e.to_string()))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn schema_is_valid_json() {
        let v: serde_json::Value =
            serde_json::from_str(VERIFIER_VERDICT_SCHEMA).expect("schema parses");
        assert_eq!(v["type"], "object");
    }

    #[test]
    fn parse_verdict_users_turn() {
        let p =
            ParsedVerdict::parse(r#"{"verdict":"users_turn","confidence":"high","reason":"x"}"#)
                .unwrap();
        assert_eq!(p.verdict, Verdict::UsersTurn);
        assert_eq!(p.confidence, Confidence::High);
    }

    #[test]
    fn parse_verdict_needs_work_low() {
        let p = ParsedVerdict::parse(
            r#"{"verdict":"needs_work","confidence":"low","reason":"unsure"}"#,
        )
        .unwrap();
        assert_eq!(p.verdict, Verdict::NeedsWork);
        assert_eq!(p.confidence, Confidence::Low);
    }

    #[test]
    fn parse_verdict_rejects_garbage() {
        assert!(ParsedVerdict::parse("not json").is_err());
        assert!(ParsedVerdict::parse(r#"{"verdict":"yes"}"#).is_err());
    }

    #[test]
    fn prefix_hash_is_stable_for_same_bytes() {
        let c1 = VerifierClient::new(
            VerifierBackend::OpenAI {
                api_key: "k".into(),
                base_url: "http://x".into(),
                model: "m".into(),
            },
            Duration::from_secs(5),
            VERIFIER_SYSTEM_PROMPT,
            VERIFIER_VERDICT_SCHEMA,
        )
        .unwrap();
        let c2 = VerifierClient::new(
            VerifierBackend::OpenAI {
                api_key: "different".into(),
                base_url: "http://y".into(),
                model: "m2".into(),
            },
            Duration::from_secs(5),
            VERIFIER_SYSTEM_PROMPT,
            VERIFIER_VERDICT_SCHEMA,
        )
        .unwrap();
        assert_eq!(
            c1.prefix_hash(),
            c2.prefix_hash(),
            "prefix hash depends only on system prompt + schema bytes"
        );
    }
}
