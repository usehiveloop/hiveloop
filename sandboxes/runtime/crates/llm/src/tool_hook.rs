use crate::permission_manager::PermissionManager;
use crate::streaming::TodoItem;
use crate::SseEvent;
use bridge_core::permission::{ApprovalDecision, ToolPermission};
use rig::agent::{HookAction, PromptHook, ToolCallHookAction};
use rig::completion::CompletionModel;
use serde_json::json;
use std::collections::{HashMap, HashSet};
use std::sync::Arc;
use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;
use tools::agent::{AgentTaskNotification, AgentToolParams, AGENT_CONTEXT};
use tools::bash::{run_command, BashArgs};
use tools::todo::TodoWriteResult;
use tools::ToolExecutor;
use tracing::{debug, warn};
use webhooks::WebhookContext;

/// A [`PromptHook`] that emits [`SseEvent::ToolCallStart`] and
/// [`SseEvent::ToolCallResult`] events through an SSE channel whenever the
/// agent loop invokes a tool.
///
/// Also intercepts bash tool calls with `background: true` to spawn them
/// asynchronously and send a notification when they complete. This is handled
/// here (rather than in the bash tool's execute method) because rig-core's
/// tool server dispatches tool calls in separate `tokio::spawn` tasks, which
/// lose the `AGENT_CONTEXT` task_local. The hook runs in the original task
/// scope where `AGENT_CONTEXT` is available.
///
/// Additionally intercepts unknown tool names and returns helpful error
/// messages with suggestions (case-insensitive match or Levenshtein distance).
#[derive(Clone)]
pub struct ToolCallEmitter {
    pub sse_tx: mpsc::Sender<SseEvent>,
    pub cancel: CancellationToken,
    /// Known tool names for tool repair. When populated, unknown tool names
    /// are intercepted and a helpful suggestion is returned instead of letting
    /// rig-core return a generic error.
    pub tool_names: HashSet<String>,
    /// Tool executors keyed by canonical name. Used to execute tools directly
    /// when the LLM-provided name was auto-repaired (trimmed, case-fixed, etc.)
    /// and rig-core would not find the tool under the original name.
    pub tool_executors: HashMap<String, Arc<dyn ToolExecutor>>,
    /// Optional webhook context for dispatching webhook events alongside SSE.
    pub webhook_ctx: Option<WebhookContext>,
    /// Agent ID for webhook payloads.
    pub agent_id: String,
    /// Conversation ID for webhook payloads.
    pub conversation_id: String,
    /// Permission manager for handling tool approval requests.
    pub permission_manager: Arc<PermissionManager>,
    /// Per-tool permission overrides for this agent.
    pub agent_permissions: HashMap<String, ToolPermission>,
}

impl<M: CompletionModel> PromptHook<M> for ToolCallEmitter {
    async fn on_tool_call(
        &self,
        tool_name: &str,
        tool_call_id: Option<String>,
        internal_call_id: &str,
        args: &str,
    ) -> ToolCallHookAction {
        let id = tool_call_id.unwrap_or_else(|| internal_call_id.to_string());
        let arguments = serde_json::from_str(args)
            .unwrap_or_else(|_| serde_json::Value::String(args.to_string()));

        debug!(tool_name = tool_name, id = %id, "tool call start");

        let id_for_bg = id.clone();
        let _ = self
            .sse_tx
            .send(SseEvent::ToolCallStart {
                id,
                name: tool_name.to_string(),
                arguments: arguments.clone(),
            })
            .await;
        if let Some(ref wh) = self.webhook_ctx {
            wh.dispatcher.dispatch(webhooks::events::tool_call_started(
                &self.agent_id,
                &self.conversation_id,
                json!({"tool_name": tool_name, "arguments": &arguments}),
                &wh.url,
                &wh.secret,
            ));
        }

        // Resolve the effective tool name: normalize, case-insensitive, fuzzy.
        let (effective_name, name_was_repaired) =
            if !self.tool_names.is_empty() {
                match self.resolve_tool_name(tool_name) {
                    Some(resolved) => {
                        let repaired = resolved != tool_name;
                        if repaired {
                            debug!(
                                original = tool_name,
                                resolved = %resolved,
                                "auto-repaired tool name"
                            );
                        }
                        (resolved, repaired)
                    }
                    None => {
                        // Unresolvable — return error with suggestion.
                        let error = self.unknown_tool_error(tool_name);
                        let _ = self
                            .sse_tx
                            .send(SseEvent::ToolCallResult {
                                id: id_for_bg.clone(),
                                result: error.clone(),
                                is_error: true,
                            })
                            .await;
                        if let Some(ref wh) = self.webhook_ctx {
                            wh.dispatcher.dispatch(webhooks::events::tool_call_completed(
                            &self.agent_id, &self.conversation_id,
                            json!({"tool_name": tool_name, "result": &error, "is_error": true}),
                            &wh.url, &wh.secret,
                        ));
                        }
                        return ToolCallHookAction::Skip { reason: error };
                    }
                }
            } else {
                (tool_name.to_string(), false)
            };

        // Check permission for this tool
        match self.agent_permissions.get(&effective_name) {
            Some(ToolPermission::Deny) => {
                let error = json!({
                    "error": format!("Tool '{}' is denied by agent permissions", effective_name)
                })
                .to_string();
                let _ = self
                    .sse_tx
                    .send(SseEvent::ToolCallResult {
                        id: id_for_bg.clone(),
                        result: error.clone(),
                        is_error: true,
                    })
                    .await;
                if let Some(ref wh) = self.webhook_ctx {
                    wh.dispatcher.dispatch(webhooks::events::tool_call_completed(
                        &self.agent_id,
                        &self.conversation_id,
                        json!({"tool_name": &effective_name, "result": &error, "is_error": true}),
                        &wh.url,
                        &wh.secret,
                    ));
                }
                return ToolCallHookAction::Skip { reason: error };
            }
            Some(ToolPermission::RequireApproval) => {
                // Extract integration metadata from tool name (format: "integration__action")
                let (int_name, int_action) =
                    tools::integration::parse_integration_tool_name(&effective_name)
                        .map(|(n, a)| (Some(n.to_string()), Some(a.to_string())))
                        .unwrap_or((None, None));

                let decision = self
                    .permission_manager
                    .request_approval(
                        &self.agent_id,
                        &self.conversation_id,
                        &effective_name,
                        &id_for_bg,
                        &arguments,
                        &self.sse_tx,
                        &self.webhook_ctx,
                        int_name,
                        int_action,
                    )
                    .await;
                match decision {
                    Ok(ApprovalDecision::Deny) => {
                        let error = json!({"error": "Tool call denied by user"}).to_string();
                        let _ = self
                            .sse_tx
                            .send(SseEvent::ToolCallResult {
                                id: id_for_bg.clone(),
                                result: error.clone(),
                                is_error: true,
                            })
                            .await;
                        if let Some(ref wh) = self.webhook_ctx {
                            wh.dispatcher.dispatch(webhooks::events::tool_call_completed(
                                &self.agent_id,
                                &self.conversation_id,
                                json!({"tool_name": &effective_name, "result": &error, "is_error": true}),
                                &wh.url,
                                &wh.secret,
                            ));
                        }
                        return ToolCallHookAction::Skip { reason: error };
                    }
                    Ok(ApprovalDecision::Approve) => {
                        // Fall through to normal execution
                    }
                    Err(()) => {
                        warn!(
                            tool_name = %effective_name,
                            "approval channel dropped (conversation ended), skipping tool call"
                        );
                        let error = json!({
                            "error": "Tool approval cancelled — conversation ended"
                        })
                        .to_string();
                        return ToolCallHookAction::Skip { reason: error };
                    }
                }
            }
            Some(ToolPermission::Allow) | None => {
                // Fall through to normal execution
            }
        }

        // Intercept bash calls with background: true.
        if effective_name == "bash" {
            if let Ok(bash_args) = serde_json::from_str::<BashArgs>(args) {
                if bash_args.background {
                    return self.handle_background_bash(bash_args, id_for_bg).await;
                }
            }
        }

        // Intercept ALL agent tool calls (AGENT_CONTEXT is only available here).
        if effective_name == "agent" {
            if let Ok(agent_params) = serde_json::from_str::<AgentToolParams>(args) {
                return self.handle_agent_tool(agent_params, id_for_bg).await;
            }
        }

        // If the name was repaired, rig-core won't find the tool under the
        // original name. Execute the tool ourselves and return Skip.
        if name_was_repaired {
            return self
                .execute_repaired_tool(&effective_name, args, id_for_bg)
                .await;
        }

        ToolCallHookAction::Continue
    }

    async fn on_tool_result(
        &self,
        tool_name: &str,
        tool_call_id: Option<String>,
        internal_call_id: &str,
        _args: &str,
        result: &str,
    ) -> HookAction {
        let id = tool_call_id.unwrap_or_else(|| internal_call_id.to_string());

        debug!(tool_name = tool_name, id = %id, "tool call result");

        let _ = self
            .sse_tx
            .send(SseEvent::ToolCallResult {
                id,
                result: result.to_string(),
                is_error: false,
            })
            .await;
        if let Some(ref wh) = self.webhook_ctx {
            wh.dispatcher
                .dispatch(webhooks::events::tool_call_completed(
                    &self.agent_id,
                    &self.conversation_id,
                    json!({"tool_name": tool_name, "result": result, "is_error": false}),
                    &wh.url,
                    &wh.secret,
                ));
        }

        // Emit a structured TodoUpdated event when the todowrite tool completes.
        if tool_name == "todowrite" {
            if let Ok(parsed) = serde_json::from_str::<TodoWriteResult>(result) {
                let todos: Vec<TodoItem> = parsed
                    .todos
                    .into_iter()
                    .map(|t| TodoItem {
                        content: t.content,
                        status: t.status,
                        priority: t.priority,
                    })
                    .collect();
                if let Some(ref wh) = self.webhook_ctx {
                    wh.dispatcher.dispatch(webhooks::events::todo_updated(
                        &self.agent_id,
                        &self.conversation_id,
                        json!({"todos": &todos}),
                        &wh.url,
                        &wh.secret,
                    ));
                }
                let _ = self.sse_tx.send(SseEvent::TodoUpdated { todos }).await;
            }
        }

        HookAction::cont()
    }
}

/// Normalize a tool name by stripping common LLM artifacts.
fn normalize_tool_name(name: &str) -> String {
    let mut s = name.trim().to_string();

    // Strip wrapping double quotes: "bash" → bash
    if s.len() >= 2 && s.starts_with('"') && s.ends_with('"') {
        s = s[1..s.len() - 1].to_string();
    }
    // Strip wrapping single quotes: 'bash' → bash
    if s.len() >= 2 && s.starts_with('\'') && s.ends_with('\'') {
        s = s[1..s.len() - 1].to_string();
    }
    // Strip wrapping backticks: `bash` → bash
    if s.len() >= 2 && s.starts_with('`') && s.ends_with('`') {
        s = s[1..s.len() - 1].to_string();
    }

    // Trim again in case of nested whitespace
    s.trim().to_string()
}

impl ToolCallEmitter {
    /// Try to resolve a raw tool name to a known canonical tool name.
    ///
    /// Resolution order:
    /// 1. Exact match (fast path)
    /// 2. Exact match after normalization (trim, strip quotes/backticks)
    /// 3. Case-insensitive match
    /// 4. High-confidence Levenshtein match (score > 0.8)
    ///
    /// Returns `None` if the name cannot be confidently resolved.
    fn resolve_tool_name(&self, raw_name: &str) -> Option<String> {
        // 1. Exact match (most common case)
        if self.tool_names.contains(raw_name) {
            return Some(raw_name.to_string());
        }

        // 2. Normalize and try exact match
        let normalized = normalize_tool_name(raw_name);
        if normalized != raw_name && self.tool_names.contains(&normalized) {
            return Some(normalized);
        }

        // 3. Case-insensitive match on the normalized name
        let lower = normalized.to_lowercase();
        for known in &self.tool_names {
            if known.to_lowercase() == lower {
                return Some(known.clone());
            }
        }

        // 4. High-confidence Levenshtein match (>0.8)
        let mut best: Option<(String, f64)> = None;
        for known in &self.tool_names {
            let score = strsim::normalized_levenshtein(&lower, &known.to_lowercase());
            if score > best.as_ref().map_or(0.0, |(_, d)| *d) {
                best = Some((known.clone(), score));
            }
        }
        if let Some((name, score)) = best {
            if score > 0.8 {
                return Some(name);
            }
        }

        None
    }

    /// Build an error message for a tool name that could not be resolved.
    ///
    /// Includes a lower-confidence Levenshtein suggestion (>0.4) to help the
    /// model self-correct on the next attempt.
    fn unknown_tool_error(&self, name: &str) -> String {
        let normalized = normalize_tool_name(name);
        let lower = normalized.to_lowercase();

        // Levenshtein distance suggestion
        let mut best: Option<(&str, f64)> = None;
        for known in &self.tool_names {
            let score = strsim::normalized_levenshtein(&lower, &known.to_lowercase());
            if score > best.as_ref().map_or(0.0, |(_, d)| *d) {
                best = Some((known, score));
            }
        }

        let names: Vec<&str> = self.tool_names.iter().map(|s| s.as_str()).collect();
        if let Some((suggestion, score)) = best {
            if score > 0.4 {
                return format!(
                    "Unknown tool '{}'. Did you mean '{}'? Available tools: [{}]",
                    name,
                    suggestion,
                    names.join(", ")
                );
            }
        }

        format!(
            "Unknown tool '{}'. Available tools: [{}]",
            name,
            names.join(", ")
        )
    }

    /// Execute a tool directly after its name was auto-repaired.
    ///
    /// Because rig-core dispatches tools by exact name match, a repaired name
    /// (e.g. `" bash"` → `"bash"`) would not be found by rig-core. We execute
    /// the tool ourselves and return `Skip` with the result.
    async fn execute_repaired_tool(
        &self,
        tool_name: &str,
        args: &str,
        sse_id: String,
    ) -> ToolCallHookAction {
        let executor = match self.tool_executors.get(tool_name) {
            Some(executor) => executor.clone(),
            None => {
                let error = format!(
                    "Tool '{}' resolved but executor not found (internal error)",
                    tool_name
                );
                let _ = self
                    .sse_tx
                    .send(SseEvent::ToolCallResult {
                        id: sse_id,
                        result: error.clone(),
                        is_error: true,
                    })
                    .await;
                if let Some(ref wh) = self.webhook_ctx {
                    wh.dispatcher
                        .dispatch(webhooks::events::tool_call_completed(
                            &self.agent_id,
                            &self.conversation_id,
                            json!({"tool_name": tool_name, "result": &error, "is_error": true}),
                            &wh.url,
                            &wh.secret,
                        ));
                }
                return ToolCallHookAction::Skip { reason: error };
            }
        };

        let args_value: serde_json::Value =
            serde_json::from_str(args).unwrap_or(serde_json::Value::Object(serde_json::Map::new()));

        let (result_str, is_error) = match executor.execute(args_value).await {
            Ok(output) => (output, false),
            Err(e) => (format!("Toolset error: {}", e), true),
        };

        let _ = self
            .sse_tx
            .send(SseEvent::ToolCallResult {
                id: sse_id,
                result: result_str.clone(),
                is_error,
            })
            .await;
        if let Some(ref wh) = self.webhook_ctx {
            wh.dispatcher
                .dispatch(webhooks::events::tool_call_completed(
                    &self.agent_id,
                    &self.conversation_id,
                    json!({"tool_name": tool_name, "result": &result_str, "is_error": is_error}),
                    &wh.url,
                    &wh.secret,
                ));
        }

        ToolCallHookAction::Skip { reason: result_str }
    }

    /// Handle a bash tool call with `background: true`.
    ///
    /// Spawns the command asynchronously and sends a notification via the
    /// conversation's `notification_tx` when complete. Returns `Skip` with
    /// a JSON result containing the task_id so the tool server does not
    /// execute the bash tool itself.
    async fn handle_background_bash(&self, args: BashArgs, sse_id: String) -> ToolCallHookAction {
        let ctx = match AGENT_CONTEXT.try_with(|c| c.clone()) {
            Ok(ctx) => ctx,
            Err(_) => {
                return ToolCallHookAction::Skip {
                    reason: "Background bash requires a conversation context".to_string(),
                };
            }
        };

        let task_id = uuid::Uuid::new_v4().to_string();
        let task_id_clone = task_id.clone();
        let notification_tx = ctx.notification_tx.clone();

        let command = args.command.clone();
        let timeout_ms = args.timeout.unwrap_or(120_000);
        let workdir = args.workdir.unwrap_or_else(|| ".".to_string());
        let description = args
            .description
            .unwrap_or_else(|| command.chars().take(80).collect());
        let description_clone = description.clone();

        let result_json = serde_json::json!({
            "task_id": task_id,
            "status": "running",
            "message": "Background command started. You will be notified when it completes."
        })
        .to_string();

        // Emit the tool result SSE event for the immediate response
        let result_json_clone = result_json.clone();
        let cancel = self.cancel.clone();
        tokio::spawn(async move {
            let result = tokio::select! {
                _ = cancel.cancelled() => {
                    Err("Background command cancelled".to_string())
                }
                result = run_command(&command, &workdir, timeout_ms) => result,
            };

            let output = match result {
                Ok(bash_result) => match serde_json::to_string(&bash_result) {
                    Ok(json) => Ok(json),
                    Err(e) => Err(format!("Failed to serialize result: {e}")),
                },
                Err(e) => Err(e),
            };

            let notification = AgentTaskNotification {
                task_id: task_id_clone,
                description: description_clone,
                output,
            };

            // If the receiver is dropped (conversation ended), silently discard
            let _ = notification_tx.send(notification).await;
        });

        // Emit tool_call_result SSE so the client sees the immediate response
        let _ = self
            .sse_tx
            .send(SseEvent::ToolCallResult {
                id: sse_id,
                result: result_json_clone.clone(),
                is_error: false,
            })
            .await;
        if let Some(ref wh) = self.webhook_ctx {
            wh.dispatcher
                .dispatch(webhooks::events::tool_call_completed(
                    &self.agent_id,
                    &self.conversation_id,
                    json!({"tool_name": "bash", "result": &result_json_clone, "is_error": false}),
                    &wh.url,
                    &wh.secret,
                ));
        }

        ToolCallHookAction::Skip {
            reason: result_json,
        }
    }

    /// Handle an agent tool call by executing it here where AGENT_CONTEXT is
    /// available, then returning `Skip` so rig-core does not dispatch to a
    /// spawned task (where the task_local would be lost).
    async fn handle_agent_tool(
        &self,
        params: AgentToolParams,
        sse_id: String,
    ) -> ToolCallHookAction {
        let ctx = match AGENT_CONTEXT.try_with(|c| c.clone()) {
            Ok(ctx) => ctx,
            Err(_) => {
                let error = "Agent tool requires a conversation context".to_string();
                let _ = self
                    .sse_tx
                    .send(SseEvent::ToolCallResult {
                        id: sse_id,
                        result: error.clone(),
                        is_error: true,
                    })
                    .await;
                if let Some(ref wh) = self.webhook_ctx {
                    wh.dispatcher
                        .dispatch(webhooks::events::tool_call_completed(
                            &self.agent_id,
                            &self.conversation_id,
                            json!({"tool_name": "agent", "result": &error, "is_error": true}),
                            &wh.url,
                            &wh.secret,
                        ));
                }
                return ToolCallHookAction::Skip { reason: error };
            }
        };

        // Check depth limit
        if ctx.depth >= ctx.max_depth {
            let error = format!("Maximum subagent depth ({}) reached", ctx.max_depth);
            let _ = self
                .sse_tx
                .send(SseEvent::ToolCallResult {
                    id: sse_id,
                    result: error.clone(),
                    is_error: true,
                })
                .await;
            if let Some(ref wh) = self.webhook_ctx {
                wh.dispatcher
                    .dispatch(webhooks::events::tool_call_completed(
                        &self.agent_id,
                        &self.conversation_id,
                        json!({"tool_name": "agent", "result": &error, "is_error": true}),
                        &wh.url,
                        &wh.secret,
                    ));
            }
            return ToolCallHookAction::Skip { reason: error };
        }

        // Validate subagent exists
        let available = ctx.runner.available_subagents();
        let subagent_exists = available.iter().any(|(name, _)| name == &params.subagent);
        if !subagent_exists {
            let error = if available.is_empty() {
                "No subagents available. This agent has no subagents configured.".to_string()
            } else {
                let names: Vec<&str> = available.iter().map(|(n, _)| n.as_str()).collect();
                format!(
                    "Unknown subagent '{}'. Available: [{}]",
                    params.subagent,
                    names.join(", ")
                )
            };
            let _ = self
                .sse_tx
                .send(SseEvent::ToolCallResult {
                    id: sse_id,
                    result: error.clone(),
                    is_error: true,
                })
                .await;
            if let Some(ref wh) = self.webhook_ctx {
                wh.dispatcher
                    .dispatch(webhooks::events::tool_call_completed(
                        &self.agent_id,
                        &self.conversation_id,
                        json!({"tool_name": "agent", "result": &error, "is_error": true}),
                        &wh.url,
                        &wh.secret,
                    ));
            }
            return ToolCallHookAction::Skip { reason: error };
        }

        if params.background {
            // Background execution
            let result = ctx
                .runner
                .run_background(&params.subagent, &params.prompt, &params.description)
                .await;

            let (result_str, is_error) = match result {
                Ok(handle) => {
                    let json = serde_json::json!({
                        "task_id": handle.task_id,
                        "status": "running",
                        "message": "Background task started. You will be notified when it completes."
                    })
                    .to_string();
                    (json, false)
                }
                Err(e) => (e, true),
            };

            let _ = self
                .sse_tx
                .send(SseEvent::ToolCallResult {
                    id: sse_id,
                    result: result_str.clone(),
                    is_error,
                })
                .await;
            if let Some(ref wh) = self.webhook_ctx {
                wh.dispatcher
                    .dispatch(webhooks::events::tool_call_completed(
                        &self.agent_id,
                        &self.conversation_id,
                        json!({"tool_name": "agent", "result": &result_str, "is_error": is_error}),
                        &wh.url,
                        &wh.secret,
                    ));
            }
            ToolCallHookAction::Skip { reason: result_str }
        } else {
            // Foreground execution
            let result = ctx
                .runner
                .run_foreground(&params.subagent, &params.prompt, params.task_id.as_deref())
                .await;

            let (result_str, is_error) = match result {
                Ok(task_result) => {
                    let output = format!(
                        "task_id: {} (for resuming)\n\n<task_result>\n{}\n</task_result>",
                        task_result.task_id, task_result.output
                    );
                    (output, false)
                }
                Err(e) => (e, true),
            };

            let _ = self
                .sse_tx
                .send(SseEvent::ToolCallResult {
                    id: sse_id,
                    result: result_str.clone(),
                    is_error,
                })
                .await;
            if let Some(ref wh) = self.webhook_ctx {
                wh.dispatcher
                    .dispatch(webhooks::events::tool_call_completed(
                        &self.agent_id,
                        &self.conversation_id,
                        json!({"tool_name": "agent", "result": &result_str, "is_error": is_error}),
                        &wh.url,
                        &wh.secret,
                    ));
            }
            ToolCallHookAction::Skip { reason: result_str }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use rig::prelude::CompletionClient;
    type TestModel =
        <rig::providers::openai::CompletionsClient as CompletionClient>::CompletionModel;

    #[tokio::test]
    async fn test_emitter_sends_tool_call_start() {
        let (tx, mut rx) = mpsc::channel(16);
        let emitter = ToolCallEmitter {
            sse_tx: tx,
            cancel: CancellationToken::new(),
            tool_names: HashSet::new(),
            tool_executors: HashMap::new(),
            webhook_ctx: None,
            agent_id: "test-agent".to_string(),
            conversation_id: "test-conv".to_string(),
            permission_manager: Arc::new(PermissionManager::new()),
            agent_permissions: HashMap::new(),
        };

        let action = PromptHook::<TestModel>::on_tool_call(
            &emitter,
            "web_search",
            Some("call_123".to_string()),
            "int_456",
            r#"{"query":"test"}"#,
        )
        .await;

        assert_eq!(action, ToolCallHookAction::Continue);

        let event = rx.try_recv().expect("should have received an event");
        match event {
            SseEvent::ToolCallStart {
                id,
                name,
                arguments,
            } => {
                assert_eq!(id, "call_123");
                assert_eq!(name, "web_search");
                assert_eq!(arguments, serde_json::json!({"query": "test"}));
            }
            other => panic!("expected ToolCallStart, got {:?}", other),
        }
    }

    #[tokio::test]
    async fn test_emitter_sends_tool_call_result() {
        let (tx, mut rx) = mpsc::channel(16);
        let emitter = ToolCallEmitter {
            sse_tx: tx,
            cancel: CancellationToken::new(),
            tool_names: HashSet::new(),
            tool_executors: HashMap::new(),
            webhook_ctx: None,
            agent_id: "test-agent".to_string(),
            conversation_id: "test-conv".to_string(),
            permission_manager: Arc::new(PermissionManager::new()),
            agent_permissions: HashMap::new(),
        };

        let action = PromptHook::<TestModel>::on_tool_result(
            &emitter,
            "web_search",
            Some("call_123".to_string()),
            "int_456",
            r#"{"query":"test"}"#,
            r#"{"results": ["page1"]}"#,
        )
        .await;

        assert_eq!(action, HookAction::cont());

        let event = rx.try_recv().expect("should have received an event");
        match event {
            SseEvent::ToolCallResult {
                id,
                result,
                is_error,
            } => {
                assert_eq!(id, "call_123");
                assert_eq!(result, r#"{"results": ["page1"]}"#);
                assert!(!is_error);
            }
            other => panic!("expected ToolCallResult, got {:?}", other),
        }
    }

    #[tokio::test]
    async fn test_emitter_returns_continue() {
        let (tx, _rx) = mpsc::channel(16);
        let emitter = ToolCallEmitter {
            sse_tx: tx,
            cancel: CancellationToken::new(),
            tool_names: HashSet::new(),
            tool_executors: HashMap::new(),
            webhook_ctx: None,
            agent_id: "test-agent".to_string(),
            conversation_id: "test-conv".to_string(),
            permission_manager: Arc::new(PermissionManager::new()),
            agent_permissions: HashMap::new(),
        };

        let tool_action =
            PromptHook::<TestModel>::on_tool_call(&emitter, "test_tool", None, "internal_1", "{}")
                .await;
        assert_eq!(tool_action, ToolCallHookAction::Continue);

        let result_action = PromptHook::<TestModel>::on_tool_result(
            &emitter,
            "test_tool",
            None,
            "internal_1",
            "{}",
            "ok",
        )
        .await;
        assert_eq!(result_action, HookAction::cont());
    }

    #[tokio::test]
    async fn test_emitter_uses_internal_call_id_when_no_tool_call_id() {
        let (tx, mut rx) = mpsc::channel(16);
        let emitter = ToolCallEmitter {
            sse_tx: tx,
            cancel: CancellationToken::new(),
            tool_names: HashSet::new(),
            tool_executors: HashMap::new(),
            webhook_ctx: None,
            agent_id: "test-agent".to_string(),
            conversation_id: "test-conv".to_string(),
            permission_manager: Arc::new(PermissionManager::new()),
            agent_permissions: HashMap::new(),
        };

        PromptHook::<TestModel>::on_tool_call(
            &emitter,
            "my_tool",
            None, // no tool_call_id
            "internal_99",
            "{}",
        )
        .await;

        let event = rx.try_recv().expect("should have received an event");
        match event {
            SseEvent::ToolCallStart { id, .. } => {
                assert_eq!(id, "internal_99");
            }
            other => panic!("expected ToolCallStart, got {:?}", other),
        }
    }

    #[tokio::test]
    async fn test_emitter_handles_invalid_json_args() {
        let (tx, mut rx) = mpsc::channel(16);
        let emitter = ToolCallEmitter {
            sse_tx: tx,
            cancel: CancellationToken::new(),
            tool_names: HashSet::new(),
            tool_executors: HashMap::new(),
            webhook_ctx: None,
            agent_id: "test-agent".to_string(),
            conversation_id: "test-conv".to_string(),
            permission_manager: Arc::new(PermissionManager::new()),
            agent_permissions: HashMap::new(),
        };

        PromptHook::<TestModel>::on_tool_call(
            &emitter,
            "my_tool",
            Some("call_1".to_string()),
            "int_1",
            "not valid json",
        )
        .await;

        let event = rx.try_recv().expect("should have received an event");
        match event {
            SseEvent::ToolCallStart { arguments, .. } => {
                assert_eq!(
                    arguments,
                    serde_json::Value::String("not valid json".to_string())
                );
            }
            other => panic!("expected ToolCallStart, got {:?}", other),
        }
    }

    #[tokio::test]
    async fn test_emitter_intercepts_bash_background() {
        use std::sync::Arc;
        use tools::agent::{
            AgentContext, AgentTaskHandle, AgentTaskResult, SubAgentRunner, AGENT_CONTEXT,
        };

        struct MockRunner;

        #[async_trait::async_trait]
        impl SubAgentRunner for MockRunner {
            fn available_subagents(&self) -> Vec<(String, String)> {
                vec![]
            }
            async fn run_foreground(
                &self,
                _: &str,
                _: &str,
                _: Option<&str>,
            ) -> Result<AgentTaskResult, String> {
                Err("not implemented".to_string())
            }
            async fn run_background(
                &self,
                _: &str,
                _: &str,
                _: &str,
            ) -> Result<AgentTaskHandle, String> {
                Err("not implemented".to_string())
            }
        }

        let (notif_tx, mut notif_rx) = tokio::sync::mpsc::channel::<AgentTaskNotification>(16);
        let ctx = AgentContext {
            runner: Arc::new(MockRunner),
            notification_tx: notif_tx,
            task_registry: None,
            depth: 0,
            max_depth: 3,
        };

        let (sse_tx, mut sse_rx) = mpsc::channel(16);
        let emitter = ToolCallEmitter {
            sse_tx,
            cancel: CancellationToken::new(),
            tool_names: HashSet::new(),
            tool_executors: HashMap::new(),
            webhook_ctx: None,
            agent_id: "test-agent".to_string(),
            conversation_id: "test-conv".to_string(),
            permission_manager: Arc::new(PermissionManager::new()),
            agent_permissions: HashMap::new(),
        };

        let action = AGENT_CONTEXT
            .scope(ctx, async {
                PromptHook::<TestModel>::on_tool_call(
                    &emitter,
                    "bash",
                    Some("call_bg".to_string()),
                    "int_bg",
                    r#"{"command":"echo hook_bg_test","background":true,"description":"bg test"}"#,
                )
                .await
            })
            .await;

        // Should return Skip with the immediate JSON result
        match action {
            ToolCallHookAction::Skip { reason } => {
                let parsed: serde_json::Value =
                    serde_json::from_str(&reason).expect("parse skip reason");
                assert!(parsed.get("task_id").is_some());
                assert_eq!(parsed["status"], "running");
            }
            other => panic!("expected Skip, got {:?}", other),
        }

        // Verify SSE events: tool_call_start + tool_call_result
        let start_event = sse_rx.try_recv().expect("should have tool_call_start");
        match &start_event {
            SseEvent::ToolCallStart { id, .. } => {
                assert_eq!(
                    id, "call_bg",
                    "ToolCallStart should use the original tool_call_id"
                );
            }
            other => panic!("expected ToolCallStart, got {:?}", other),
        }

        let result_event = sse_rx.try_recv().expect("should have tool_call_result");
        match &result_event {
            SseEvent::ToolCallResult { id, .. } => {
                assert_eq!(
                    id, "call_bg",
                    "ToolCallResult should use the same id as ToolCallStart"
                );
            }
            other => panic!("expected ToolCallResult, got {:?}", other),
        }

        // Wait for the background notification
        let notification = tokio::time::timeout(std::time::Duration::from_secs(5), notif_rx.recv())
            .await
            .expect("notification should arrive")
            .expect("channel should not be closed");

        assert_eq!(notification.description, "bg test");
        let output = notification.output.expect("should be Ok");
        assert!(output.contains("hook_bg_test"));
    }

    #[tokio::test]
    async fn test_emitter_does_not_intercept_foreground_bash() {
        let (tx, _rx) = mpsc::channel(16);
        let emitter = ToolCallEmitter {
            sse_tx: tx,
            cancel: CancellationToken::new(),
            tool_names: HashSet::new(),
            tool_executors: HashMap::new(),
            webhook_ctx: None,
            agent_id: "test-agent".to_string(),
            conversation_id: "test-conv".to_string(),
            permission_manager: Arc::new(PermissionManager::new()),
            agent_permissions: HashMap::new(),
        };

        // bash without background: true should Continue normally
        let action = PromptHook::<TestModel>::on_tool_call(
            &emitter,
            "bash",
            Some("call_fg".to_string()),
            "int_fg",
            r#"{"command":"echo hello"}"#,
        )
        .await;

        assert_eq!(action, ToolCallHookAction::Continue);
    }

    #[tokio::test]
    async fn test_emitter_intercepts_agent_tool() {
        use std::sync::Arc;
        use tools::agent::{
            AgentContext, AgentTaskHandle, AgentTaskResult, SubAgentRunner, AGENT_CONTEXT,
        };

        struct MockRunner;

        #[async_trait::async_trait]
        impl SubAgentRunner for MockRunner {
            fn available_subagents(&self) -> Vec<(String, String)> {
                vec![("coder".to_string(), "A coding agent".to_string())]
            }
            async fn run_foreground(
                &self,
                subagent: &str,
                prompt: &str,
                _task_id: Option<&str>,
            ) -> Result<AgentTaskResult, String> {
                Ok(AgentTaskResult {
                    task_id: "agent-task-789".to_string(),
                    output: format!("Result from {} for: {}", subagent, prompt),
                })
            }
            async fn run_background(
                &self,
                _: &str,
                _: &str,
                _: &str,
            ) -> Result<AgentTaskHandle, String> {
                Ok(AgentTaskHandle {
                    task_id: "bg-agent-456".to_string(),
                })
            }
        }

        let (notif_tx, _notif_rx) = tokio::sync::mpsc::channel::<AgentTaskNotification>(16);
        let ctx = AgentContext {
            runner: Arc::new(MockRunner),
            notification_tx: notif_tx,
            task_registry: None,
            depth: 0,
            max_depth: 3,
        };

        let (sse_tx, mut sse_rx) = mpsc::channel(16);
        let emitter = ToolCallEmitter {
            sse_tx,
            cancel: CancellationToken::new(),
            tool_names: HashSet::new(),
            tool_executors: HashMap::new(),
            webhook_ctx: None,
            agent_id: "test-agent".to_string(),
            conversation_id: "test-conv".to_string(),
            permission_manager: Arc::new(PermissionManager::new()),
            agent_permissions: HashMap::new(),
        };

        let action = AGENT_CONTEXT
            .scope(ctx, async {
                PromptHook::<TestModel>::on_tool_call(
                    &emitter,
                    "agent",
                    Some("call_agent".to_string()),
                    "int_agent",
                    r#"{"description":"test task","prompt":"write hello world","subagent":"coder"}"#,
                )
                .await
            })
            .await;

        // Should return Skip with the foreground result
        match &action {
            ToolCallHookAction::Skip { reason } => {
                assert!(reason.contains("agent-task-789"), "should contain task_id");
                assert!(
                    reason.contains("Result from coder"),
                    "should contain subagent output"
                );
                assert!(
                    reason.contains("<task_result>"),
                    "should contain task_result tags"
                );
            }
            other => panic!("expected Skip, got {:?}", other),
        }

        // Verify SSE events: tool_call_start + tool_call_result
        let start_event = sse_rx.try_recv().expect("should have tool_call_start");
        match &start_event {
            SseEvent::ToolCallStart { id, name, .. } => {
                assert_eq!(id, "call_agent");
                assert_eq!(name, "agent");
            }
            other => panic!("expected ToolCallStart, got {:?}", other),
        }

        let result_event = sse_rx.try_recv().expect("should have tool_call_result");
        match &result_event {
            SseEvent::ToolCallResult {
                id,
                is_error,
                result,
            } => {
                assert_eq!(id, "call_agent");
                assert!(!is_error, "should not be an error");
                assert!(result.contains("Result from coder"));
            }
            other => panic!("expected ToolCallResult, got {:?}", other),
        }
    }

    #[tokio::test]
    async fn test_emitter_intercepts_unknown_tool() {
        let (tx, mut rx) = mpsc::channel(16);
        let tool_names: HashSet<String> = ["bash", "read", "edit", "grep"]
            .iter()
            .map(|s| s.to_string())
            .collect();
        let emitter = ToolCallEmitter {
            sse_tx: tx,
            cancel: CancellationToken::new(),
            tool_names,
            tool_executors: HashMap::new(),
            webhook_ctx: None,
            agent_id: "test-agent".to_string(),
            conversation_id: "test-conv".to_string(),
            permission_manager: Arc::new(PermissionManager::new()),
            agent_permissions: HashMap::new(),
        };

        let action = PromptHook::<TestModel>::on_tool_call(
            &emitter,
            "bassh",
            Some("call_typo".to_string()),
            "int_typo",
            r#"{"command":"echo hello"}"#,
        )
        .await;

        // Should return Skip with an error suggesting "bash"
        match &action {
            ToolCallHookAction::Skip { reason } => {
                assert!(reason.contains("Unknown tool 'bassh'"));
                assert!(reason.contains("bash"));
            }
            other => panic!("expected Skip, got {:?}", other),
        }

        // Should emit ToolCallStart and ToolCallResult (error)
        let _start = rx.try_recv().expect("should have tool_call_start");
        let result_event = rx.try_recv().expect("should have tool_call_result");
        match &result_event {
            SseEvent::ToolCallResult {
                is_error, result, ..
            } => {
                assert!(is_error);
                assert!(result.contains("Unknown tool"));
            }
            other => panic!("expected ToolCallResult, got {:?}", other),
        }
    }

    #[tokio::test]
    async fn test_emitter_allows_known_tools() {
        let (tx, _rx) = mpsc::channel(16);
        let tool_names: HashSet<String> = ["bash", "read", "edit"]
            .iter()
            .map(|s| s.to_string())
            .collect();
        let emitter = ToolCallEmitter {
            sse_tx: tx,
            cancel: CancellationToken::new(),
            tool_names,
            tool_executors: HashMap::new(),
            webhook_ctx: None,
            agent_id: "test-agent".to_string(),
            conversation_id: "test-conv".to_string(),
            permission_manager: Arc::new(PermissionManager::new()),
            agent_permissions: HashMap::new(),
        };

        // Known tool should pass through
        let action = PromptHook::<TestModel>::on_tool_call(
            &emitter,
            "bash",
            Some("call_ok".to_string()),
            "int_ok",
            r#"{"command":"echo hello"}"#,
        )
        .await;

        assert_eq!(action, ToolCallHookAction::Continue);
    }

    #[tokio::test]
    async fn test_emitter_empty_tool_names_skips_check() {
        let (tx, _rx) = mpsc::channel(16);
        let emitter = ToolCallEmitter {
            sse_tx: tx,
            cancel: CancellationToken::new(),
            tool_names: HashSet::new(),
            tool_executors: HashMap::new(),
            webhook_ctx: None,
            agent_id: "test-agent".to_string(),
            conversation_id: "test-conv".to_string(),
            permission_manager: Arc::new(PermissionManager::new()),
            agent_permissions: HashMap::new(),
        };

        // With empty tool_names, all tools should pass through (backward compat)
        let action = PromptHook::<TestModel>::on_tool_call(
            &emitter,
            "anything_goes",
            Some("call_any".to_string()),
            "int_any",
            "{}",
        )
        .await;

        assert_eq!(action, ToolCallHookAction::Continue);
    }

    #[tokio::test]
    async fn test_emitter_auto_repairs_case_mismatch() {
        let (tx, mut rx) = mpsc::channel(16);
        let tool_names: HashSet<String> = ["bash", "Read", "edit"]
            .iter()
            .map(|s| s.to_string())
            .collect();

        // Create a stub executor for "bash" so auto-repair can execute it
        struct StubBash;
        #[async_trait::async_trait]
        impl ToolExecutor for StubBash {
            fn name(&self) -> &str {
                "bash"
            }
            fn description(&self) -> &str {
                "stub"
            }
            fn parameters_schema(&self) -> serde_json::Value {
                serde_json::json!({})
            }
            async fn execute(&self, _args: serde_json::Value) -> Result<String, String> {
                Ok("repaired_bash_output".to_string())
            }
            fn as_any(&self) -> &dyn std::any::Any {
                self
            }
        }
        let mut executors: HashMap<String, Arc<dyn ToolExecutor>> = HashMap::new();
        executors.insert("bash".to_string(), Arc::new(StubBash));

        let emitter = ToolCallEmitter {
            sse_tx: tx,
            cancel: CancellationToken::new(),
            tool_names,
            tool_executors: executors,
            webhook_ctx: None,
            agent_id: "test-agent".to_string(),
            conversation_id: "test-conv".to_string(),
            permission_manager: Arc::new(PermissionManager::new()),
            agent_permissions: HashMap::new(),
        };

        // "Bash" should auto-repair to "bash" and execute directly
        let action = PromptHook::<TestModel>::on_tool_call(
            &emitter,
            "Bash",
            Some("call_case".to_string()),
            "int_case",
            r#"{"command":"echo hello"}"#,
        )
        .await;

        match &action {
            ToolCallHookAction::Skip { reason } => {
                assert!(
                    reason.contains("repaired_bash_output"),
                    "should contain the tool output: {}",
                    reason
                );
            }
            other => panic!("expected Skip with repaired output, got {:?}", other),
        }

        // Should emit ToolCallStart + ToolCallResult
        let _start = rx.try_recv().expect("should have tool_call_start");
        let result_event = rx.try_recv().expect("should have tool_call_result");
        match &result_event {
            SseEvent::ToolCallResult {
                is_error, result, ..
            } => {
                assert!(!is_error, "should not be an error");
                assert!(result.contains("repaired_bash_output"));
            }
            other => panic!("expected ToolCallResult, got {:?}", other),
        }
    }

    #[tokio::test]
    async fn test_emitter_auto_repairs_whitespace() {
        let (tx, mut rx) = mpsc::channel(16);
        let tool_names: HashSet<String> = ["bash", "Read", "edit"]
            .iter()
            .map(|s| s.to_string())
            .collect();

        struct StubBash;
        #[async_trait::async_trait]
        impl ToolExecutor for StubBash {
            fn name(&self) -> &str {
                "bash"
            }
            fn description(&self) -> &str {
                "stub"
            }
            fn parameters_schema(&self) -> serde_json::Value {
                serde_json::json!({})
            }
            async fn execute(&self, _args: serde_json::Value) -> Result<String, String> {
                Ok("trimmed_output".to_string())
            }
            fn as_any(&self) -> &dyn std::any::Any {
                self
            }
        }
        let mut executors: HashMap<String, Arc<dyn ToolExecutor>> = HashMap::new();
        executors.insert("bash".to_string(), Arc::new(StubBash));

        let emitter = ToolCallEmitter {
            sse_tx: tx,
            cancel: CancellationToken::new(),
            tool_names,
            tool_executors: executors,
            webhook_ctx: None,
            agent_id: "test-agent".to_string(),
            conversation_id: "test-conv".to_string(),
            permission_manager: Arc::new(PermissionManager::new()),
            agent_permissions: HashMap::new(),
        };

        // " bash" (leading space) should auto-repair to "bash"
        let action = PromptHook::<TestModel>::on_tool_call(
            &emitter,
            " bash",
            Some("call_ws".to_string()),
            "int_ws",
            r#"{"command":"echo hello"}"#,
        )
        .await;

        match &action {
            ToolCallHookAction::Skip { reason } => {
                assert!(
                    reason.contains("trimmed_output"),
                    "should contain the tool output: {}",
                    reason
                );
            }
            other => panic!("expected Skip with repaired output, got {:?}", other),
        }

        let _start = rx.try_recv().expect("should have tool_call_start");
        let result_event = rx.try_recv().expect("should have tool_call_result");
        match &result_event {
            SseEvent::ToolCallResult { is_error, .. } => {
                assert!(!is_error, "should not be an error");
            }
            other => panic!("expected ToolCallResult, got {:?}", other),
        }
    }
}
