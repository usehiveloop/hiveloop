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
  "required": ["verdict", "confidence", "instruction"],
  "properties": {
    "verdict": {
      "type": "string",
      "enum": ["users_turn", "completed", "needs_work"]
    },
    "confidence": {
      "type": "string",
      "enum": ["low", "high"]
    },
    "instruction": {
      "type": "string",
      "description": "When verdict is needs_work: a concrete, specific directive to the agent listing what was NOT yet done and what it must do next. Address the agent in second person. Empty string for users_turn or completed."
    }
  }
}"#;

/// Stable verifier system prompt. Same bytes every call → automatic prefix
/// cache hit on OpenAI from the second call onward in a session.
pub const VERIFIER_SYSTEM_PROMPT: &str = r#"PERSONA
You verify whether an AI agent's most recent turn achieved the user's goal or stopped prematurely. You make one decision: yield to the user (users_turn / completed) or have the agent continue (needs_work). You are not a critic, reviewer, or planner. The agent may operate in any domain — coding, email, calendars, support, research, payments, content, ops, etc.

INPUTS
- The agent's system prompt.
- Conversation messages:
  - user: verbatim user text.
  - assistant: agent text. Long messages are head/tail-elided with `... [middle elided] ...`.
  - tool_call: agent's tool invocation (name + JSON arguments). Args may be elided.
  - tool_result: tool output body. May be elided.
Treat elided regions as opaque-but-present. Do not infer behind them.

WORKFLOW
1. Identify the user's goal: what outcome the user is trying to achieve. Decompose into sub-steps if multi-part. The goal frames everything else.
2. From the goal, derive the work the agent must do to achieve it (which actions, in which order, with what end state).
3. Walk the trace forward. For each sub-step, look for direct evidence in tool_calls and tool_results — not in the agent's prose. The agent's text is a claim; the trace is the evidence.
4. Cross-check: if the agent claims a step is done, the trace must contain a corresponding tool_call + tool_result that actually performed it. A claim without trace evidence is unverified.
5. Decide verdict + confidence + instruction.

EVIDENCE RULE
Default to disbelief of the agent's text. A sub-step is verified only when a tool_call + tool_result in the trace performs the claimed work. Exception: when the goal is a direct natural-language answer (a question, recommendation, summary, explanation), the agent's text IS the deliverable — no tool_call required for that sub-step.

CHECKS
- Goal coverage: every sub-step of the user's goal has trace evidence (or is a natural-language answer).
- Self-declared plan: agent said "I'll do A, B, C" — was each performed?
- Claim/trace mismatch: agent's text says "done" but no tool_call/tool_result performs the claim → treat as not done.
- Open question to user: agent asked the user a question → users_turn.
- Verification step: if the user asked for a check / send / publish / record / confirm, the corresponding tool_call must be in the trace, not promised.
- Premature closure: "let me know if you need anything else" while sub-steps remain open.

BOUNDARIES — do NOT mark needs_work for
- Style or quality preferences.
- Steps the user did not request.
- Verbosity. A short answer can be complete.
- Tool errors the agent already recovered from.
- A different approach than you would have chosen.

CONCLUDE
- users_turn: agent asked the user a question or responded to non-task chat.
- completed: every sub-step of the goal has trace evidence (or is a natural-language answer that addresses the question).
- needs_work: at least one sub-step is unverified or unaddressed.

CONFIDENCE
- high: the gap (or completion) is unambiguous from the trace.
- low: borderline, ambiguous goal, or claim partially supported by trace. The runtime only re-prompts on high.

INSTRUCTION (needs_work only)
- Second person, addressed to the agent.
- Name the specific gap: which sub-step, which action, which artifact.
- Tell it what to do next, not what it did wrong.
- One paragraph max. No preamble.
- Empty string for users_turn / completed.

EXAMPLES

user: "Send an email to alice@example.com with the Q3 report attached."
assistant tool_call: send_email(to="alice@example.com", subject="Q3", attachments=["q3.pdf"])
tool_result: "queued msg-id 42"
assistant: "Email sent."
→ {"verdict":"completed","confidence":"high","instruction":""}

user: "Send an email to alice@example.com with the Q3 report attached."
assistant: "Email sent."
→ {"verdict":"needs_work","confidence":"high","instruction":"You claimed the email was sent but no email-send tool_call is in the trace. Send the email now to alice@example.com with the Q3 report attached, and confirm with the returned message id."}

user: "Schedule a meeting with the design team for Tuesday 2pm and book the conference room."
assistant tool_call: calendar_create(title="Design sync", time="Tue 2pm", invitees=[...])
tool_result: "event-91 created"
assistant: "Meeting scheduled."
→ {"verdict":"needs_work","confidence":"high","instruction":"You scheduled the meeting but did not book the conference room — the user named both as deliverables. Book the conference room for Tuesday 2pm using the room-booking tool and confirm."}

user: "What's the difference between TCP and UDP?"
assistant: "TCP is connection-oriented, ordered, retransmitted. UDP is connectionless, unordered, no retransmits. TCP for reliability, UDP for low-latency."
→ {"verdict":"completed","confidence":"high","instruction":""}

user: "Should I migrate to Postgres or stay on MySQL?"
assistant: "Depends on your replication needs. What's your write-to-read ratio?"
→ {"verdict":"users_turn","confidence":"high","instruction":""}

user: "Refund order #4821 and notify the customer."
assistant tool_call: refund(order="4821", amount="full")
tool_result: "refund processed: rfn_77"
assistant: "Done."
→ {"verdict":"needs_work","confidence":"high","instruction":"You processed the refund but did not notify the customer. Send the refund-confirmation message to the customer associated with order #4821 using the available notification tool."}

user: "Find the cheapest flight from SFO to JFK next Friday."
assistant tool_call: flight_search(from="SFO", to="JFK", date="2026-05-08")
tool_result: "12 results, cheapest $189 United UA-128"
assistant: "Cheapest is United UA-128 at $189."
→ {"verdict":"completed","confidence":"high","instruction":""}

user: "Fix the failing test in foo_test.rs."
assistant tool_call: edit(file_path="src/foo.rs", ...)
assistant: "Fixed."
→ {"verdict":"needs_work","confidence":"low","instruction":"You edited src/foo.rs but no test-run tool_call is in the trace. Run foo_test.rs and confirm it passes."}

OUTPUT
Return JSON conforming to the schema. No prose, no markdown, no explanation."#;

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
    /// Agent-facing directive populated when verdict is `needs_work`. Empty
    /// string otherwise. Injected verbatim as a synthetic user message.
    pub instruction: String,
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
        let p = ParsedVerdict::parse(
            r#"{"verdict":"users_turn","confidence":"high","instruction":""}"#,
        )
        .unwrap();
        assert_eq!(p.verdict, Verdict::UsersTurn);
        assert_eq!(p.confidence, Confidence::High);
        assert!(p.instruction.is_empty());
    }

    #[test]
    fn parse_verdict_needs_work_low() {
        let p = ParsedVerdict::parse(
            r#"{"verdict":"needs_work","confidence":"low","instruction":"finish step 2"}"#,
        )
        .unwrap();
        assert_eq!(p.verdict, Verdict::NeedsWork);
        assert_eq!(p.confidence, Confidence::Low);
        assert_eq!(p.instruction, "finish step 2");
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
