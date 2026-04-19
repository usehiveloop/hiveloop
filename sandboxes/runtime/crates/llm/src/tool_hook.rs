use crate::permission_manager::PermissionManager;
use bridge_core::conversation::{
    ContentBlock, Message, Role, ToolCall, ToolResult as BridgeToolResult,
};
use bridge_core::event::{BridgeEvent, BridgeEventType};
use bridge_core::permission::{ApprovalDecision, ToolPermission};
use bridge_core::AgentMetrics;
use dashmap::DashMap;
use rig::agent::{HookAction, PromptHook, ToolCallHookAction};
use rig::completion::CompletionModel;
use serde_json::json;
use std::collections::{HashMap, HashSet};
use std::sync::{Arc, Mutex};
use std::time::Instant;
use storage::StorageHandle;
use tokio_util::sync::CancellationToken;
use tools::agent::{AgentTaskNotification, SubAgentToolParams, AGENT_CONTEXT};
use tools::bash::{run_command, BashArgs};
use tools::self_agent::AgentToolParams;
use tools::todo::TodoWriteResult;
use tools::ToolExecutor;
use tracing::{info, warn};
use webhooks::EventBus;

/// Maximum bytes for a single tool result entering conversation history.
/// Individual tools may use lower limits. This is a centralized safety net
/// that catches MCP tools, integration tools, and skill tools that don't
/// implement their own truncation.
const TOOL_RESULT_MAX_BYTES: usize = 50 * 1024; // 50KB

/// Validate tool arguments against a JSON schema.
/// Returns Ok(()) if valid, Err(message) with human-readable validation errors if not.
fn validate_tool_args(args: &serde_json::Value, schema: &serde_json::Value) -> Result<(), String> {
    // Skip validation for empty/trivial schemas
    if schema.is_null() || schema == &serde_json::json!({}) {
        return Ok(());
    }

    let validator = match jsonschema::validator_for(schema) {
        Ok(v) => v,
        Err(_) => return Ok(()), // If schema itself is invalid, skip validation
    };

    let errors: Vec<String> = validator
        .iter_errors(args)
        .map(|e| {
            let path = e.instance_path().to_string();
            if path.is_empty() {
                e.to_string()
            } else {
                format!("{}: {}", path, e)
            }
        })
        .collect();

    if errors.is_empty() {
        Ok(())
    } else {
        Err(errors.join("; "))
    }
}

/// Truncate a tool result string if it exceeds the safety net threshold.
/// Returns the original string if within limits.
fn truncate_if_needed(result: String) -> String {
    if result.len() <= TOOL_RESULT_MAX_BYTES {
        return result;
    }
    let truncated = tools::truncation::truncate_output(
        &result,
        tools::truncation::MAX_LINES,
        TOOL_RESULT_MAX_BYTES,
    );
    truncated.content
}

/// A [`PromptHook`] that emits [`BridgeEvent`]s through the [`EventBus`]
/// whenever the agent loop invokes a tool.
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
    pub event_bus: Arc<EventBus>,
    pub cancel: CancellationToken,
    /// Known tool names for tool repair. When populated, unknown tool names
    /// are intercepted and a helpful suggestion is returned instead of letting
    /// rig-core return a generic error.
    pub tool_names: HashSet<String>,
    /// Tool executors keyed by canonical name. Used to execute tools directly
    /// when the LLM-provided name was auto-repaired (trimmed, case-fixed, etc.)
    /// and rig-core would not find the tool under the original name.
    pub tool_executors: HashMap<String, Arc<dyn ToolExecutor>>,
    /// Agent ID for event payloads.
    pub agent_id: String,
    /// Conversation ID for event payloads.
    pub conversation_id: String,
    /// Permission manager for handling tool approval requests.
    pub permission_manager: Arc<PermissionManager>,
    /// Per-tool permission overrides for this agent.
    pub agent_permissions: HashMap<String, ToolPermission>,
    /// Shared metrics for recording per-tool stats.
    pub metrics: Arc<AgentMetrics>,
    /// Per-conversation metrics for token/tool tracking.
    pub conversation_metrics: Option<Arc<bridge_core::metrics::ConversationMetrics>>,
    /// Pending tool call start times, keyed by tool_call_id.
    /// Used to measure latency for rig-core dispatched tools where
    /// timing spans on_tool_call → on_tool_result.
    pub pending_tool_timings: Arc<DashMap<String, (Instant, String)>>,
    /// Optional storage handle for incremental persistence after each tool call.
    pub storage: Option<StorageHandle>,
    /// Shared persisted messages — updated incrementally after each tool interaction.
    pub persisted_messages: Option<Arc<Mutex<Vec<Message>>>>,
    /// Optional mid-turn context-pressure threshold (bytes). When cumulative
    /// tool-output bytes this turn exceed this value, a one-shot
    /// `ContextPressureWarning` event is emitted. `None` disables the check.
    pub pressure_threshold_bytes: Option<usize>,
    /// Cumulative tool-output bytes for the current turn. Owned by each
    /// turn's emitter clone (see `conversation.rs`) — reset implicitly when
    /// a fresh emitter is constructed next turn.
    pub pressure_counter: Arc<std::sync::atomic::AtomicUsize>,
    /// Flag so ContextPressureWarning is only emitted once per turn.
    pub pressure_warned: Arc<std::sync::atomic::AtomicBool>,
}

impl ToolCallEmitter {
    /// Record `bytes_added` of tool output this turn and emit a one-shot
    /// `ContextPressureWarning` if cumulative bytes cross the configured
    /// threshold. No-op if no threshold was configured.
    fn note_tool_output_bytes(&self, bytes_added: usize) {
        let Some(threshold) = self.pressure_threshold_bytes else {
            return;
        };
        use std::sync::atomic::Ordering;
        if self.pressure_warned.load(Ordering::Relaxed) {
            // Still count bytes, but don't re-warn.
            self.pressure_counter
                .fetch_add(bytes_added, Ordering::Relaxed);
            return;
        }
        let new_total = self
            .pressure_counter
            .fetch_add(bytes_added, Ordering::Relaxed)
            + bytes_added;
        if new_total >= threshold && !self.pressure_warned.swap(true, Ordering::Relaxed) {
            self.event_bus.emit(BridgeEvent::new(
                BridgeEventType::ContextPressureWarning,
                &self.agent_id,
                &self.conversation_id,
                json!({
                    "cumulative_tool_output_bytes": new_total,
                    "threshold_bytes": threshold,
                    "reason": "tool_output_accumulation",
                }),
            ));
        }
    }
}

impl<M: CompletionModel> PromptHook<M> for ToolCallEmitter {
    async fn on_tool_call(
        &self,
        tool_name: &str,
        tool_call_id: Option<String>,
        internal_call_id: &str,
        args: &str,
    ) -> ToolCallHookAction {
        let call_start = Instant::now();
        let id = tool_call_id.unwrap_or_else(|| internal_call_id.to_string());
        let arguments = serde_json::from_str(args)
            .unwrap_or_else(|_| serde_json::Value::String(args.to_string()));

        info!(
            agent_id = %self.agent_id,
            conversation_id = %self.conversation_id,
            tool_name = tool_name,
            tool_call_id = %id,
            arguments = %Truncated::new(args, 100),
            "tool_call_start"
        );

        let id_for_bg = id.clone();
        self.event_bus.emit(BridgeEvent::new(
            BridgeEventType::ToolCallStarted,
            &self.agent_id,
            &self.conversation_id,
            json!({"id": &id, "name": tool_name, "arguments": &arguments}),
        ));

        // Resolve the effective tool name: normalize, case-insensitive, fuzzy.
        let (effective_name, name_was_repaired) = if !self.tool_names.is_empty() {
            match self.resolve_tool_name(tool_name) {
                Some(resolved) => {
                    let repaired = resolved != tool_name;
                    if repaired {
                        info!(
                            agent_id = %self.agent_id,
                            conversation_id = %self.conversation_id,
                            original = tool_name,
                            resolved = %resolved,
                            "tool_name_repaired"
                        );
                    }
                    (resolved, repaired)
                }
                None => {
                    // Unresolvable — return error with suggestion.
                    let error = self.unknown_tool_error(tool_name);
                    let duration_ms = call_start.elapsed().as_millis() as u64;
                    self.metrics
                        .record_tool_call_detailed(tool_name, true, duration_ms);
                    warn!(
                        agent_id = %self.agent_id,
                        conversation_id = %self.conversation_id,
                        tool_name = tool_name,
                        duration_ms = duration_ms,
                        error = %error,
                        "tool_call_failed"
                    );
                    self.event_bus.emit(BridgeEvent::new(
                            BridgeEventType::ToolCallCompleted,
                            &self.agent_id,
                            &self.conversation_id,
                            json!({"id": &id_for_bg, "result": &error, "is_error": true, "duration_ms": duration_ms, "tool_name": tool_name}),
                        ));
                    self.persist_tool_interaction(tool_name, &id_for_bg, &arguments, &error, true);
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
                let duration_ms = call_start.elapsed().as_millis() as u64;
                self.metrics
                    .record_tool_call_detailed(&effective_name, true, duration_ms);
                warn!(
                    agent_id = %self.agent_id,
                    conversation_id = %self.conversation_id,
                    tool_name = %effective_name,
                    duration_ms = duration_ms,
                    error = %error,
                    "tool_call_failed"
                );
                self.event_bus.emit(BridgeEvent::new(
                    BridgeEventType::ToolCallCompleted,
                    &self.agent_id,
                    &self.conversation_id,
                    json!({"id": &id_for_bg, "result": &error, "is_error": true, "duration_ms": duration_ms, "tool_name": &effective_name}),
                ));
                self.persist_tool_interaction(
                    &effective_name,
                    &id_for_bg,
                    &arguments,
                    &error,
                    true,
                );
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
                        &self.event_bus,
                        int_name,
                        int_action,
                    )
                    .await;
                match decision {
                    Ok((ApprovalDecision::Deny, reason)) => {
                        info!(
                            agent_id = %self.agent_id,
                            conversation_id = %self.conversation_id,
                            tool_name = %effective_name,
                            decision = "denied",
                            "permission_decision"
                        );
                        let error_msg = match reason {
                            Some(r) => format!("Tool '{}' denied by user: {}", effective_name, r),
                            None => format!("Tool '{}' denied by user", effective_name),
                        };
                        let error = json!({"error": error_msg}).to_string();
                        let duration_ms = call_start.elapsed().as_millis() as u64;
                        self.metrics
                            .record_tool_call_detailed(&effective_name, true, duration_ms);
                        self.event_bus.emit(BridgeEvent::new(
                            BridgeEventType::ToolCallCompleted,
                            &self.agent_id,
                            &self.conversation_id,
                            json!({"id": &id_for_bg, "result": &error, "is_error": true, "duration_ms": duration_ms, "tool_name": &effective_name}),
                        ));
                        self.persist_tool_interaction(
                            &effective_name,
                            &id_for_bg,
                            &arguments,
                            &error,
                            true,
                        );
                        return ToolCallHookAction::Skip { reason: error };
                    }
                    Ok((ApprovalDecision::Approve, _)) => {
                        info!(
                            agent_id = %self.agent_id,
                            conversation_id = %self.conversation_id,
                            tool_name = %effective_name,
                            decision = "approved",
                            "permission_decision"
                        );
                    }
                    Err(()) => {
                        info!(
                            agent_id = %self.agent_id,
                            conversation_id = %self.conversation_id,
                            tool_name = %effective_name,
                            decision = "cancelled",
                            "permission_decision"
                        );
                        let error = json!({
                            "error": "Tool approval cancelled — conversation ended"
                        })
                        .to_string();
                        let duration_ms = call_start.elapsed().as_millis() as u64;
                        self.metrics
                            .record_tool_call_detailed(&effective_name, true, duration_ms);
                        self.persist_tool_interaction(
                            &effective_name,
                            &id_for_bg,
                            &arguments,
                            &error,
                            true,
                        );
                        return ToolCallHookAction::Skip { reason: error };
                    }
                }
            }
            Some(ToolPermission::Allow) | None => {
                // Fall through to normal execution
            }
        }

        // Validate tool arguments against the tool's JSON schema.
        // Catches malformed calls early so the agent can retry immediately
        // without a wasted round-trip to the tool executor.
        if let Some(executor) = self.tool_executors.get(&effective_name) {
            let schema = executor.parameters_schema();
            if let Err(validation_error) = validate_tool_args(&arguments, &schema) {
                let error = json!({
                    "error": format!("Invalid arguments for tool '{}': {}", effective_name, validation_error)
                })
                .to_string();
                let duration_ms = call_start.elapsed().as_millis() as u64;
                self.metrics
                    .record_tool_call_detailed(&effective_name, true, duration_ms);
                info!(
                    agent_id = %self.agent_id,
                    conversation_id = %self.conversation_id,
                    tool_name = %effective_name,
                    error = %error,
                    "tool_call_args_invalid"
                );
                self.event_bus.emit(BridgeEvent::new(
                    BridgeEventType::ToolCallCompleted,
                    &self.agent_id,
                    &self.conversation_id,
                    json!({"id": &id_for_bg, "result": &error, "is_error": true, "duration_ms": duration_ms, "tool_name": &effective_name}),
                ));
                self.persist_tool_interaction(
                    &effective_name,
                    &id_for_bg,
                    &arguments,
                    &error,
                    true,
                );
                return ToolCallHookAction::Skip {
                    reason: truncate_if_needed(error),
                };
            }
        }

        // Intercept bash calls with background: true.
        if effective_name == "bash" {
            if let Ok(bash_args) = serde_json::from_str::<BashArgs>(args) {
                if bash_args.background {
                    info!(
                        agent_id = %self.agent_id,
                        conversation_id = %self.conversation_id,
                        command = %Truncated::new(&bash_args.command, 100),
                        task_description = bash_args.description.as_deref().unwrap_or(""),
                        "background_task_spawn"
                    );
                    return self
                        .handle_background_bash(bash_args, id_for_bg, call_start)
                        .await;
                }
            }
        }

        // Intercept self-delegation agent tool calls (AGENT_CONTEXT is only available here).
        if effective_name == "agent" {
            if let Ok(agent_params) = serde_json::from_str::<AgentToolParams>(args) {
                info!(
                    agent_id = %self.agent_id,
                    conversation_id = %self.conversation_id,
                    subagent_name = "__self__",
                    mode = if agent_params.run_in_background { "background" } else { "foreground" },
                    "subagent_spawn"
                );
                return self
                    .handle_self_agent_tool(agent_params, id_for_bg, call_start)
                    .await;
            }
        }

        // Intercept sub_agent tool calls (AGENT_CONTEXT is only available here).
        if effective_name == "sub_agent" {
            if let Ok(sub_agent_params) = serde_json::from_str::<SubAgentToolParams>(args) {
                info!(
                    agent_id = %self.agent_id,
                    conversation_id = %self.conversation_id,
                    subagent_name = %sub_agent_params.subagent_name,
                    mode = if sub_agent_params.run_in_background { "background" } else { "foreground" },
                    "subagent_spawn"
                );
                return self
                    .handle_sub_agent_tool(sub_agent_params, id_for_bg, call_start)
                    .await;
            }
        }

        // If the name was repaired, rig-core won't find the tool under the
        // original name. Execute the tool ourselves and return Skip.
        if name_was_repaired {
            return self
                .execute_repaired_tool(&effective_name, args, id_for_bg, call_start)
                .await;
        }

        // Path 8: rig-core will dispatch. Store timing for on_tool_result.
        self.pending_tool_timings
            .insert(id_for_bg, (call_start, effective_name));

        ToolCallHookAction::Continue
    }

    async fn on_tool_result(
        &self,
        tool_name: &str,
        tool_call_id: Option<String>,
        internal_call_id: &str,
        args: &str,
        result: &str,
    ) -> HookAction {
        let id = tool_call_id.unwrap_or_else(|| internal_call_id.to_string());

        // Look up pending timing from on_tool_call (path 8 — rig-core dispatch)
        let (duration_ms, effective_name) =
            if let Some((_, (start, ename))) = self.pending_tool_timings.remove(&id) {
                (Some(start.elapsed().as_millis() as u64), ename)
            } else {
                (None, tool_name.to_string())
            };

        if let Some(dur) = duration_ms {
            self.metrics
                .record_tool_call_detailed(&effective_name, false, dur);
            if let Some(ref cm) = self.conversation_metrics {
                cm.record_tool_call(dur);
            }
        }

        info!(
            agent_id = %self.agent_id,
            conversation_id = %self.conversation_id,
            tool_name = %effective_name,
            tool_call_id = %id,
            duration_ms = duration_ms.unwrap_or(0),
            is_error = false,
            result = %Truncated::new(result, 80),
            "tool_call_complete"
        );

        self.event_bus.emit(BridgeEvent::new(
            BridgeEventType::ToolCallCompleted,
            &self.agent_id,
            &self.conversation_id,
            json!({"id": &id, "result": result, "is_error": false, "duration_ms": duration_ms, "tool_name": &effective_name}),
        ));

        // Mid-turn context-pressure tracking — counts bytes of tool output
        // this turn, emits ContextPressureWarning once past the threshold.
        self.note_tool_output_bytes(result.len());

        let args_value = serde_json::from_str(args).unwrap_or(serde_json::Value::Null);
        self.persist_tool_interaction(&effective_name, &id, &args_value, result, false);

        // Emit a structured TodoUpdated event when the todowrite tool completes.
        if tool_name == "todowrite" {
            if let Ok(parsed) = serde_json::from_str::<TodoWriteResult>(result) {
                self.event_bus.emit(BridgeEvent::new(
                    BridgeEventType::TodoUpdated,
                    &self.agent_id,
                    &self.conversation_id,
                    json!({"todos": &parsed.todos}),
                ));
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

/// A zero-allocation Display wrapper that truncates a string slice on output.
/// Only performs formatting work when actually rendered (i.e., when the log
/// event is enabled), making it truly zero-cost when filtered out.
struct Truncated<'a> {
    s: &'a str,
    max_len: usize,
}

impl<'a> Truncated<'a> {
    fn new(s: &'a str, max_len: usize) -> Self {
        Self { s, max_len }
    }
}

impl std::fmt::Display for Truncated<'_> {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        if self.s.len() <= self.max_len {
            f.write_str(self.s)
        } else {
            // Find a char-safe boundary at or before max_len
            let boundary = self.s.floor_char_boundary(self.max_len);
            write!(
                f,
                "{}...[truncated, {} bytes total]",
                &self.s[..boundary],
                self.s.len()
            )
        }
    }
}

impl ToolCallEmitter {
    /// Persist a tool call + result pair incrementally to SQLite.
    ///
    /// This is fire-and-forget: the write is enqueued on the storage channel
    /// and does not block the tool execution path. At turn end, the
    /// authoritative rebuild from rig's enriched_history replaces these
    /// incremental messages, ensuring consistency.
    fn persist_tool_interaction(
        &self,
        tool_name: &str,
        tool_call_id: &str,
        args: &serde_json::Value,
        result: &str,
        is_error: bool,
    ) {
        let (Some(storage), Some(shared)) = (&self.storage, &self.persisted_messages) else {
            return;
        };

        let tool_call_msg = Message {
            role: Role::Assistant,
            content: vec![ContentBlock::ToolCall(ToolCall {
                id: tool_call_id.to_string(),
                name: tool_name.to_string(),
                arguments: args.clone(),
            })],
            timestamp: chrono::Utc::now(),
            system_reminder: None,
        };

        let tool_result_msg = Message {
            role: Role::Tool,
            content: vec![ContentBlock::ToolResult(BridgeToolResult {
                tool_call_id: tool_call_id.to_string(),
                content: result.to_string(),
                is_error,
            })],
            timestamp: chrono::Utc::now(),
            system_reminder: None,
        };

        let messages = {
            let mut guard = shared.lock().unwrap();
            guard.push(tool_call_msg);
            guard.push(tool_result_msg);
            guard.clone()
        };

        storage.replace_messages(self.conversation_id.clone(), messages);
    }

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
        call_start: Instant,
    ) -> ToolCallHookAction {
        let executor = match self.tool_executors.get(tool_name) {
            Some(executor) => executor.clone(),
            None => {
                let error = format!(
                    "Tool '{}' resolved but executor not found (internal error)",
                    tool_name
                );
                let duration_ms = call_start.elapsed().as_millis() as u64;
                self.metrics
                    .record_tool_call_detailed(tool_name, true, duration_ms);
                warn!(
                    agent_id = %self.agent_id,
                    conversation_id = %self.conversation_id,
                    tool_name = tool_name,
                    duration_ms = duration_ms,
                    error = %error,
                    "tool_call_failed"
                );
                self.event_bus.emit(BridgeEvent::new(
                    BridgeEventType::ToolCallCompleted,
                    &self.agent_id,
                    &self.conversation_id,
                    json!({"id": &sse_id, "result": &error, "is_error": true, "duration_ms": duration_ms, "tool_name": tool_name}),
                ));
                self.persist_tool_interaction(
                    tool_name,
                    &sse_id,
                    &serde_json::Value::Null,
                    &error,
                    true,
                );
                return ToolCallHookAction::Skip { reason: error };
            }
        };

        let args_value: serde_json::Value =
            serde_json::from_str(args).unwrap_or(serde_json::Value::Object(serde_json::Map::new()));

        let (result_str, is_error) = match executor.execute(args_value.clone()).await {
            Ok(output) => (output, false),
            Err(e) => (format!("Toolset error: {}", e), true),
        };

        let duration_ms = call_start.elapsed().as_millis() as u64;
        self.metrics
            .record_tool_call_detailed(tool_name, is_error, duration_ms);
        if is_error {
            warn!(
                agent_id = %self.agent_id,
                conversation_id = %self.conversation_id,
                tool_name = tool_name,
                duration_ms = duration_ms,
                error = %Truncated::new(&result_str, 80),
                "tool_call_failed"
            );
        } else {
            info!(
                agent_id = %self.agent_id,
                conversation_id = %self.conversation_id,
                tool_name = tool_name,
                duration_ms = duration_ms,
                is_error = false,
                result = %Truncated::new(&result_str, 80),
                "tool_call_complete"
            );
        }

        self.event_bus.emit(BridgeEvent::new(
            BridgeEventType::ToolCallCompleted,
            &self.agent_id,
            &self.conversation_id,
            json!({"id": &sse_id, "result": &result_str, "is_error": is_error, "duration_ms": duration_ms, "tool_name": tool_name}),
        ));
        self.persist_tool_interaction(tool_name, &sse_id, &args_value, &result_str, is_error);

        ToolCallHookAction::Skip {
            reason: truncate_if_needed(result_str),
        }
    }

    /// Handle a bash tool call with `background: true`.
    ///
    /// Spawns the command asynchronously and sends a notification via the
    /// conversation's `notification_tx` when complete. Returns `Skip` with
    /// a JSON result containing the task_id so the tool server does not
    /// execute the bash tool itself.
    async fn handle_background_bash(
        &self,
        args: BashArgs,
        sse_id: String,
        call_start: Instant,
    ) -> ToolCallHookAction {
        let ctx = match AGENT_CONTEXT.try_with(|c| c.clone()) {
            Ok(ctx) => ctx,
            Err(_) => {
                let error = "Background bash requires a conversation context".to_string();
                let duration_ms = call_start.elapsed().as_millis() as u64;
                self.metrics
                    .record_tool_call_detailed("bash", true, duration_ms);
                warn!(
                    agent_id = %self.agent_id,
                    conversation_id = %self.conversation_id,
                    tool_name = "bash",
                    duration_ms = duration_ms,
                    error = %error,
                    "tool_call_failed"
                );
                return ToolCallHookAction::Skip { reason: error };
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

        // Record metrics for the background bash spawn (not the actual execution)
        let duration_ms = call_start.elapsed().as_millis() as u64;
        self.metrics
            .record_tool_call_detailed("bash", false, duration_ms);
        info!(
            agent_id = %self.agent_id,
            conversation_id = %self.conversation_id,
            tool_name = "bash",
            duration_ms = duration_ms,
            is_error = false,
            task_id = %task_id_clone,
            "tool_call_complete"
        );

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

        // Emit tool_call_result so the client sees the immediate response
        self.event_bus.emit(BridgeEvent::new(
            BridgeEventType::ToolCallCompleted,
            &self.agent_id,
            &self.conversation_id,
            json!({"id": &sse_id, "result": &result_json_clone, "is_error": false, "duration_ms": duration_ms, "tool_name": "bash"}),
        ));
        self.persist_tool_interaction(
            "bash",
            &sse_id,
            &serde_json::Value::Null,
            &result_json_clone,
            false,
        );

        ToolCallHookAction::Skip {
            reason: truncate_if_needed(result_json),
        }
    }

    /// Handle a self-delegation agent tool call by executing it here where
    /// AGENT_CONTEXT is available, then returning `Skip` so rig-core does not
    /// dispatch to a spawned task (where the task_local would be lost).
    async fn handle_self_agent_tool(
        &self,
        params: AgentToolParams,
        sse_id: String,
        call_start: Instant,
    ) -> ToolCallHookAction {
        let ctx = match AGENT_CONTEXT.try_with(|c| c.clone()) {
            Ok(ctx) => ctx,
            Err(_) => {
                let error = "Agent tool requires a conversation context".to_string();
                let duration_ms = call_start.elapsed().as_millis() as u64;
                self.metrics
                    .record_tool_call_detailed("agent", true, duration_ms);
                warn!(
                    agent_id = %self.agent_id, conversation_id = %self.conversation_id,
                    tool_name = "agent", duration_ms = duration_ms, error = %error, "tool_call_failed"
                );
                self.event_bus.emit(BridgeEvent::new(
                    BridgeEventType::ToolCallCompleted,
                    &self.agent_id,
                    &self.conversation_id,
                    json!({"id": &sse_id, "result": &error, "is_error": true, "duration_ms": duration_ms, "tool_name": "agent"}),
                ));
                self.persist_tool_interaction(
                    "agent",
                    &sse_id,
                    &serde_json::Value::Null,
                    &error,
                    true,
                );
                return ToolCallHookAction::Skip { reason: error };
            }
        };

        // Check depth limit
        if ctx.depth >= ctx.max_depth {
            let error = format!("Maximum agent depth ({}) reached", ctx.max_depth);
            let duration_ms = call_start.elapsed().as_millis() as u64;
            self.metrics
                .record_tool_call_detailed("agent", true, duration_ms);
            warn!(
                agent_id = %self.agent_id, conversation_id = %self.conversation_id,
                tool_name = "agent", duration_ms = duration_ms, error = %error, "tool_call_failed"
            );
            self.event_bus.emit(BridgeEvent::new(
                BridgeEventType::ToolCallCompleted,
                &self.agent_id,
                &self.conversation_id,
                json!({"id": &sse_id, "result": &error, "is_error": true, "duration_ms": duration_ms, "tool_name": "agent"}),
            ));
            self.persist_tool_interaction("agent", &sse_id, &serde_json::Value::Null, &error, true);
            return ToolCallHookAction::Skip { reason: error };
        }

        // Check task budget
        if let Err(e) = ctx.task_budget.try_acquire() {
            let duration_ms = call_start.elapsed().as_millis() as u64;
            self.metrics
                .record_tool_call_detailed("agent", true, duration_ms);
            warn!(
                agent_id = %self.agent_id, conversation_id = %self.conversation_id,
                tool_name = "agent", duration_ms = duration_ms, error = %e, "tool_call_failed"
            );
            self.event_bus.emit(BridgeEvent::new(
                BridgeEventType::ToolCallCompleted,
                &self.agent_id,
                &self.conversation_id,
                json!({"id": &sse_id, "result": &e, "is_error": true, "duration_ms": duration_ms, "tool_name": "agent"}),
            ));
            self.persist_tool_interaction("agent", &sse_id, &serde_json::Value::Null, &e, true);
            return ToolCallHookAction::Skip { reason: e };
        }

        // Self-delegation always targets "__self__"
        let subagent_name = "__self__";

        if params.run_in_background {
            let result = ctx
                .runner
                .run_background(subagent_name, &params.prompt, &params.description)
                .await;

            let (result_str, is_error) = match result {
                Ok(handle) => {
                    let json = serde_json::json!({
                        "task_id": handle.task_id,
                        "status": "running",
                        "message": "Background agent started. Its final output will appear in your next user turn — do not poll or wait."
                    })
                    .to_string();
                    (json, false)
                }
                Err(e) => (e, true),
            };

            let duration_ms = call_start.elapsed().as_millis() as u64;
            self.metrics
                .record_tool_call_detailed("agent", is_error, duration_ms);
            info!(
                agent_id = %self.agent_id, conversation_id = %self.conversation_id,
                tool_name = "agent", duration_ms = duration_ms, is_error = is_error,
                result = %Truncated::new(&result_str, 80), "tool_call_complete"
            );
            self.event_bus.emit(BridgeEvent::new(
                BridgeEventType::ToolCallCompleted,
                &self.agent_id,
                &self.conversation_id,
                json!({"id": &sse_id, "result": &result_str, "is_error": is_error, "duration_ms": duration_ms, "tool_name": "agent"}),
            ));
            self.persist_tool_interaction(
                "agent",
                &sse_id,
                &serde_json::Value::Null,
                &result_str,
                is_error,
            );
            ToolCallHookAction::Skip {
                reason: truncate_if_needed(result_str),
            }
        } else {
            let result = ctx
                .runner
                .run_foreground(subagent_name, &params.prompt, params.task_id.as_deref())
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

            let duration_ms = call_start.elapsed().as_millis() as u64;
            self.metrics
                .record_tool_call_detailed("agent", is_error, duration_ms);
            info!(
                agent_id = %self.agent_id, conversation_id = %self.conversation_id,
                tool_name = "agent", duration_ms = duration_ms, is_error = is_error,
                result = %Truncated::new(&result_str, 80), "tool_call_complete"
            );
            self.event_bus.emit(BridgeEvent::new(
                BridgeEventType::ToolCallCompleted,
                &self.agent_id,
                &self.conversation_id,
                json!({"id": &sse_id, "result": &result_str, "is_error": is_error, "duration_ms": duration_ms, "tool_name": "agent"}),
            ));
            self.persist_tool_interaction(
                "agent",
                &sse_id,
                &serde_json::Value::Null,
                &result_str,
                is_error,
            );
            ToolCallHookAction::Skip {
                reason: truncate_if_needed(result_str),
            }
        }
    }

    /// Handle a sub_agent tool call by executing it here where AGENT_CONTEXT is
    /// available, then returning `Skip` so rig-core does not dispatch to a
    /// spawned task (where the task_local would be lost).
    async fn handle_sub_agent_tool(
        &self,
        params: SubAgentToolParams,
        sse_id: String,
        call_start: Instant,
    ) -> ToolCallHookAction {
        let ctx = match AGENT_CONTEXT.try_with(|c| c.clone()) {
            Ok(ctx) => ctx,
            Err(_) => {
                let error = "Sub-agent tool requires a conversation context".to_string();
                let duration_ms = call_start.elapsed().as_millis() as u64;
                self.metrics
                    .record_tool_call_detailed("sub_agent", true, duration_ms);
                warn!(
                    agent_id = %self.agent_id, conversation_id = %self.conversation_id,
                    tool_name = "sub_agent", duration_ms = duration_ms, error = %error, "tool_call_failed"
                );
                self.event_bus.emit(BridgeEvent::new(
                    BridgeEventType::ToolCallCompleted,
                    &self.agent_id,
                    &self.conversation_id,
                    json!({"id": &sse_id, "result": &error, "is_error": true, "duration_ms": duration_ms, "tool_name": "sub_agent"}),
                ));
                self.persist_tool_interaction(
                    "sub_agent",
                    &sse_id,
                    &serde_json::Value::Null,
                    &error,
                    true,
                );
                return ToolCallHookAction::Skip { reason: error };
            }
        };

        // Check depth limit
        if ctx.depth >= ctx.max_depth {
            let error = format!("Maximum subagent depth ({}) reached", ctx.max_depth);
            let duration_ms = call_start.elapsed().as_millis() as u64;
            self.metrics
                .record_tool_call_detailed("sub_agent", true, duration_ms);
            warn!(
                agent_id = %self.agent_id, conversation_id = %self.conversation_id,
                tool_name = "sub_agent", duration_ms = duration_ms, error = %error, "tool_call_failed"
            );
            self.event_bus.emit(BridgeEvent::new(
                BridgeEventType::ToolCallCompleted,
                &self.agent_id,
                &self.conversation_id,
                json!({"id": &sse_id, "result": &error, "is_error": true, "duration_ms": duration_ms, "tool_name": "sub_agent"}),
            ));
            self.persist_tool_interaction(
                "sub_agent",
                &sse_id,
                &serde_json::Value::Null,
                &error,
                true,
            );
            return ToolCallHookAction::Skip { reason: error };
        }

        // Validate subagent exists
        let available = ctx.runner.available_subagents();
        let subagent_exists = available
            .iter()
            .any(|(name, _)| name == &params.subagent_name);
        if !subagent_exists {
            let error = if available.is_empty() {
                "No subagents available. This agent has no subagents configured.".to_string()
            } else {
                let names: Vec<&str> = available.iter().map(|(n, _)| n.as_str()).collect();
                format!(
                    "Unknown subagent '{}'. Available: [{}]",
                    params.subagent_name,
                    names.join(", ")
                )
            };
            let duration_ms = call_start.elapsed().as_millis() as u64;
            self.metrics
                .record_tool_call_detailed("sub_agent", true, duration_ms);
            warn!(
                agent_id = %self.agent_id, conversation_id = %self.conversation_id,
                tool_name = "sub_agent", duration_ms = duration_ms, error = %error, "tool_call_failed"
            );
            self.event_bus.emit(BridgeEvent::new(
                BridgeEventType::ToolCallCompleted,
                &self.agent_id,
                &self.conversation_id,
                json!({"id": &sse_id, "result": &error, "is_error": true, "duration_ms": duration_ms, "tool_name": "sub_agent"}),
            ));
            self.persist_tool_interaction(
                "sub_agent",
                &sse_id,
                &serde_json::Value::Null,
                &error,
                true,
            );
            return ToolCallHookAction::Skip { reason: error };
        }

        if params.run_in_background {
            let result = ctx
                .runner
                .run_background(&params.subagent_name, &params.prompt, &params.description)
                .await;

            let (result_str, is_error) = match result {
                Ok(handle) => {
                    let json = serde_json::json!({
                        "task_id": handle.task_id,
                        "status": "running",
                        "message": "Background subagent started. Its final output will appear in your next user turn — do not poll or wait."
                    })
                    .to_string();
                    (json, false)
                }
                Err(e) => (e, true),
            };

            let duration_ms = call_start.elapsed().as_millis() as u64;
            self.metrics
                .record_tool_call_detailed("sub_agent", is_error, duration_ms);
            info!(
                agent_id = %self.agent_id, conversation_id = %self.conversation_id,
                tool_name = "sub_agent", duration_ms = duration_ms, is_error = is_error,
                result = %Truncated::new(&result_str, 80), "tool_call_complete"
            );
            self.event_bus.emit(BridgeEvent::new(
                BridgeEventType::ToolCallCompleted,
                &self.agent_id,
                &self.conversation_id,
                json!({"id": &sse_id, "result": &result_str, "is_error": is_error, "duration_ms": duration_ms, "tool_name": "sub_agent"}),
            ));
            self.persist_tool_interaction(
                "sub_agent",
                &sse_id,
                &serde_json::Value::Null,
                &result_str,
                is_error,
            );
            ToolCallHookAction::Skip {
                reason: truncate_if_needed(result_str),
            }
        } else {
            let result = ctx
                .runner
                .run_foreground(
                    &params.subagent_name,
                    &params.prompt,
                    params.task_id.as_deref(),
                )
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

            let duration_ms = call_start.elapsed().as_millis() as u64;
            self.metrics
                .record_tool_call_detailed("sub_agent", is_error, duration_ms);
            info!(
                agent_id = %self.agent_id, conversation_id = %self.conversation_id,
                tool_name = "sub_agent", duration_ms = duration_ms, is_error = is_error,
                result = %Truncated::new(&result_str, 80), "tool_call_complete"
            );
            self.event_bus.emit(BridgeEvent::new(
                BridgeEventType::ToolCallCompleted,
                &self.agent_id,
                &self.conversation_id,
                json!({"id": &sse_id, "result": &result_str, "is_error": is_error, "duration_ms": duration_ms, "tool_name": "sub_agent"}),
            ));
            self.persist_tool_interaction(
                "sub_agent",
                &sse_id,
                &serde_json::Value::Null,
                &result_str,
                is_error,
            );
            ToolCallHookAction::Skip {
                reason: truncate_if_needed(result_str),
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use bridge_core::event::BridgeEventType;
    use rig::prelude::CompletionClient;
    type TestModel =
        <rig::providers::openai::CompletionsClient as CompletionClient>::CompletionModel;

    fn make_bus() -> Arc<EventBus> {
        Arc::new(EventBus::new(None, None, String::new(), String::new()))
    }

    #[tokio::test]
    async fn test_emitter_sends_tool_call_start() {
        let bus = make_bus();
        let mut ws_rx = bus.subscribe_ws();
        let emitter = ToolCallEmitter {
            event_bus: bus,
            cancel: CancellationToken::new(),
            tool_names: HashSet::new(),
            tool_executors: HashMap::new(),
            agent_id: "test-agent".to_string(),
            conversation_id: "test-conv".to_string(),
            permission_manager: Arc::new(PermissionManager::new()),
            agent_permissions: HashMap::new(),
            metrics: Arc::new(bridge_core::AgentMetrics::new()),
            conversation_metrics: None,
            pending_tool_timings: Arc::new(DashMap::new()),
            storage: None,
            persisted_messages: None,
            pressure_threshold_bytes: None,
            pressure_counter: std::sync::Arc::new(std::sync::atomic::AtomicUsize::new(0)),
            pressure_warned: std::sync::Arc::new(std::sync::atomic::AtomicBool::new(false)),
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

        let event = ws_rx.try_recv().expect("should have received an event");
        assert_eq!(event.event_type, BridgeEventType::ToolCallStarted);
        assert_eq!(event.data["id"], "call_123");
        assert_eq!(event.data["name"], "web_search");
        assert_eq!(
            event.data["arguments"],
            serde_json::json!({"query": "test"})
        );
    }

    #[tokio::test]
    async fn test_emitter_sends_tool_call_result() {
        let bus = make_bus();
        let mut ws_rx = bus.subscribe_ws();
        let emitter = ToolCallEmitter {
            event_bus: bus,
            cancel: CancellationToken::new(),
            tool_names: HashSet::new(),
            tool_executors: HashMap::new(),
            agent_id: "test-agent".to_string(),
            conversation_id: "test-conv".to_string(),
            permission_manager: Arc::new(PermissionManager::new()),
            agent_permissions: HashMap::new(),
            metrics: Arc::new(bridge_core::AgentMetrics::new()),
            conversation_metrics: None,
            pending_tool_timings: Arc::new(DashMap::new()),
            storage: None,
            persisted_messages: None,
            pressure_threshold_bytes: None,
            pressure_counter: std::sync::Arc::new(std::sync::atomic::AtomicUsize::new(0)),
            pressure_warned: std::sync::Arc::new(std::sync::atomic::AtomicBool::new(false)),
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

        let event = ws_rx.try_recv().expect("should have received an event");
        assert_eq!(event.event_type, BridgeEventType::ToolCallCompleted);
        assert_eq!(event.data["id"], "call_123");
        assert_eq!(event.data["result"], r#"{"results": ["page1"]}"#);
        assert_eq!(event.data["is_error"], false);
    }

    #[tokio::test]
    async fn test_emitter_returns_continue() {
        let bus = make_bus();
        let emitter = ToolCallEmitter {
            event_bus: bus,
            cancel: CancellationToken::new(),
            tool_names: HashSet::new(),
            tool_executors: HashMap::new(),
            agent_id: "test-agent".to_string(),
            conversation_id: "test-conv".to_string(),
            permission_manager: Arc::new(PermissionManager::new()),
            agent_permissions: HashMap::new(),
            metrics: Arc::new(bridge_core::AgentMetrics::new()),
            conversation_metrics: None,
            pending_tool_timings: Arc::new(DashMap::new()),
            storage: None,
            persisted_messages: None,
            pressure_threshold_bytes: None,
            pressure_counter: std::sync::Arc::new(std::sync::atomic::AtomicUsize::new(0)),
            pressure_warned: std::sync::Arc::new(std::sync::atomic::AtomicBool::new(false)),
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
        let bus = make_bus();
        let mut ws_rx = bus.subscribe_ws();
        let emitter = ToolCallEmitter {
            event_bus: bus,
            cancel: CancellationToken::new(),
            tool_names: HashSet::new(),
            tool_executors: HashMap::new(),
            agent_id: "test-agent".to_string(),
            conversation_id: "test-conv".to_string(),
            permission_manager: Arc::new(PermissionManager::new()),
            agent_permissions: HashMap::new(),
            metrics: Arc::new(bridge_core::AgentMetrics::new()),
            conversation_metrics: None,
            pending_tool_timings: Arc::new(DashMap::new()),
            storage: None,
            persisted_messages: None,
            pressure_threshold_bytes: None,
            pressure_counter: std::sync::Arc::new(std::sync::atomic::AtomicUsize::new(0)),
            pressure_warned: std::sync::Arc::new(std::sync::atomic::AtomicBool::new(false)),
        };

        PromptHook::<TestModel>::on_tool_call(
            &emitter,
            "my_tool",
            None, // no tool_call_id
            "internal_99",
            "{}",
        )
        .await;

        let event = ws_rx.try_recv().expect("should have received an event");
        assert_eq!(event.event_type, BridgeEventType::ToolCallStarted);
        assert_eq!(event.data["id"], "internal_99");
    }

    #[tokio::test]
    async fn test_emitter_handles_invalid_json_args() {
        let bus = make_bus();
        let mut ws_rx = bus.subscribe_ws();
        let emitter = ToolCallEmitter {
            event_bus: bus,
            cancel: CancellationToken::new(),
            tool_names: HashSet::new(),
            tool_executors: HashMap::new(),
            agent_id: "test-agent".to_string(),
            conversation_id: "test-conv".to_string(),
            permission_manager: Arc::new(PermissionManager::new()),
            agent_permissions: HashMap::new(),
            metrics: Arc::new(bridge_core::AgentMetrics::new()),
            conversation_metrics: None,
            pending_tool_timings: Arc::new(DashMap::new()),
            storage: None,
            persisted_messages: None,
            pressure_threshold_bytes: None,
            pressure_counter: std::sync::Arc::new(std::sync::atomic::AtomicUsize::new(0)),
            pressure_warned: std::sync::Arc::new(std::sync::atomic::AtomicBool::new(false)),
        };

        PromptHook::<TestModel>::on_tool_call(
            &emitter,
            "my_tool",
            Some("call_1".to_string()),
            "int_1",
            "not valid json",
        )
        .await;

        let event = ws_rx.try_recv().expect("should have received an event");
        assert_eq!(event.event_type, BridgeEventType::ToolCallStarted);
        assert_eq!(event.data["arguments"], "not valid json");
    }

    #[tokio::test]
    async fn test_emitter_intercepts_bash_background() {
        use std::sync::Arc;
        use tools::agent::{
            AgentContext, AgentTaskHandle, AgentTaskResult, SubAgentRunner, TaskBudget,
            AGENT_CONTEXT,
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
            depth: 0,
            max_depth: 3,
            task_budget: Arc::new(TaskBudget::new(50)),
        };

        let bus = make_bus();
        let mut ws_rx = bus.subscribe_ws();
        let emitter = ToolCallEmitter {
            event_bus: bus,
            cancel: CancellationToken::new(),
            tool_names: HashSet::new(),
            tool_executors: HashMap::new(),
            agent_id: "test-agent".to_string(),
            conversation_id: "test-conv".to_string(),
            permission_manager: Arc::new(PermissionManager::new()),
            agent_permissions: HashMap::new(),
            metrics: Arc::new(bridge_core::AgentMetrics::new()),
            conversation_metrics: None,
            pending_tool_timings: Arc::new(DashMap::new()),
            storage: None,
            persisted_messages: None,
            pressure_threshold_bytes: None,
            pressure_counter: std::sync::Arc::new(std::sync::atomic::AtomicUsize::new(0)),
            pressure_warned: std::sync::Arc::new(std::sync::atomic::AtomicBool::new(false)),
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

        // Verify events: ToolCallStarted + ToolCallCompleted
        let start_event = ws_rx.try_recv().expect("should have tool_call_start");
        assert_eq!(start_event.event_type, BridgeEventType::ToolCallStarted);
        assert_eq!(start_event.data["id"], "call_bg");

        let result_event = ws_rx.try_recv().expect("should have tool_call_result");
        assert_eq!(result_event.event_type, BridgeEventType::ToolCallCompleted);
        assert_eq!(result_event.data["id"], "call_bg");

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
        let bus = make_bus();
        let emitter = ToolCallEmitter {
            event_bus: bus,
            cancel: CancellationToken::new(),
            tool_names: HashSet::new(),
            tool_executors: HashMap::new(),
            agent_id: "test-agent".to_string(),
            conversation_id: "test-conv".to_string(),
            permission_manager: Arc::new(PermissionManager::new()),
            agent_permissions: HashMap::new(),
            metrics: Arc::new(bridge_core::AgentMetrics::new()),
            conversation_metrics: None,
            pending_tool_timings: Arc::new(DashMap::new()),
            storage: None,
            persisted_messages: None,
            pressure_threshold_bytes: None,
            pressure_counter: std::sync::Arc::new(std::sync::atomic::AtomicUsize::new(0)),
            pressure_warned: std::sync::Arc::new(std::sync::atomic::AtomicBool::new(false)),
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
    async fn test_emitter_intercepts_sub_agent_tool() {
        use std::sync::Arc;
        use tools::agent::{
            AgentContext, AgentTaskHandle, AgentTaskResult, SubAgentRunner, TaskBudget,
            AGENT_CONTEXT,
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
            depth: 0,
            max_depth: 3,
            task_budget: Arc::new(TaskBudget::new(50)),
        };

        let bus = make_bus();
        let mut ws_rx = bus.subscribe_ws();
        let emitter = ToolCallEmitter {
            event_bus: bus,
            cancel: CancellationToken::new(),
            tool_names: HashSet::new(),
            tool_executors: HashMap::new(),
            agent_id: "test-agent".to_string(),
            conversation_id: "test-conv".to_string(),
            permission_manager: Arc::new(PermissionManager::new()),
            agent_permissions: HashMap::new(),
            metrics: Arc::new(bridge_core::AgentMetrics::new()),
            conversation_metrics: None,
            pending_tool_timings: Arc::new(DashMap::new()),
            storage: None,
            persisted_messages: None,
            pressure_threshold_bytes: None,
            pressure_counter: std::sync::Arc::new(std::sync::atomic::AtomicUsize::new(0)),
            pressure_warned: std::sync::Arc::new(std::sync::atomic::AtomicBool::new(false)),
        };

        let action = AGENT_CONTEXT
            .scope(ctx, async {
                PromptHook::<TestModel>::on_tool_call(
                    &emitter,
                    "sub_agent",
                    Some("call_sub_agent".to_string()),
                    "int_sub_agent",
                    r#"{"description":"test task","prompt":"write hello world","subagentName":"coder"}"#,
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

        // Verify events: ToolCallStarted + ToolCallCompleted
        let start_event = ws_rx.try_recv().expect("should have tool_call_start");
        assert_eq!(start_event.event_type, BridgeEventType::ToolCallStarted);
        assert_eq!(start_event.data["id"], "call_sub_agent");
        assert_eq!(start_event.data["name"], "sub_agent");

        let result_event = ws_rx.try_recv().expect("should have tool_call_result");
        assert_eq!(result_event.event_type, BridgeEventType::ToolCallCompleted);
        assert_eq!(result_event.data["id"], "call_sub_agent");
        assert_eq!(result_event.data["is_error"], false);
        let result_str = result_event.data["result"].as_str().unwrap();
        assert!(result_str.contains("Result from coder"));
    }

    #[tokio::test]
    async fn test_emitter_intercepts_unknown_tool() {
        let bus = make_bus();
        let mut ws_rx = bus.subscribe_ws();
        let tool_names: HashSet<String> = ["bash", "read", "edit", "grep"]
            .iter()
            .map(|s| s.to_string())
            .collect();
        let emitter = ToolCallEmitter {
            event_bus: bus,
            cancel: CancellationToken::new(),
            tool_names,
            tool_executors: HashMap::new(),
            agent_id: "test-agent".to_string(),
            conversation_id: "test-conv".to_string(),
            permission_manager: Arc::new(PermissionManager::new()),
            agent_permissions: HashMap::new(),
            metrics: Arc::new(bridge_core::AgentMetrics::new()),
            conversation_metrics: None,
            pending_tool_timings: Arc::new(DashMap::new()),
            storage: None,
            persisted_messages: None,
            pressure_threshold_bytes: None,
            pressure_counter: std::sync::Arc::new(std::sync::atomic::AtomicUsize::new(0)),
            pressure_warned: std::sync::Arc::new(std::sync::atomic::AtomicBool::new(false)),
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

        // Should emit ToolCallStarted and ToolCallCompleted (error)
        let _start = ws_rx.try_recv().expect("should have tool_call_start");
        let result_event = ws_rx.try_recv().expect("should have tool_call_result");
        assert_eq!(result_event.event_type, BridgeEventType::ToolCallCompleted);
        assert_eq!(result_event.data["is_error"], true);
        let result_str = result_event.data["result"].as_str().unwrap();
        assert!(result_str.contains("Unknown tool"));
    }

    #[tokio::test]
    async fn test_emitter_allows_known_tools() {
        let bus = make_bus();
        let tool_names: HashSet<String> = ["bash", "read", "edit"]
            .iter()
            .map(|s| s.to_string())
            .collect();
        let emitter = ToolCallEmitter {
            event_bus: bus,
            cancel: CancellationToken::new(),
            tool_names,
            tool_executors: HashMap::new(),
            agent_id: "test-agent".to_string(),
            conversation_id: "test-conv".to_string(),
            permission_manager: Arc::new(PermissionManager::new()),
            agent_permissions: HashMap::new(),
            metrics: Arc::new(bridge_core::AgentMetrics::new()),
            conversation_metrics: None,
            pending_tool_timings: Arc::new(DashMap::new()),
            storage: None,
            persisted_messages: None,
            pressure_threshold_bytes: None,
            pressure_counter: std::sync::Arc::new(std::sync::atomic::AtomicUsize::new(0)),
            pressure_warned: std::sync::Arc::new(std::sync::atomic::AtomicBool::new(false)),
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
        let bus = make_bus();
        let emitter = ToolCallEmitter {
            event_bus: bus,
            cancel: CancellationToken::new(),
            tool_names: HashSet::new(),
            tool_executors: HashMap::new(),
            agent_id: "test-agent".to_string(),
            conversation_id: "test-conv".to_string(),
            permission_manager: Arc::new(PermissionManager::new()),
            agent_permissions: HashMap::new(),
            metrics: Arc::new(bridge_core::AgentMetrics::new()),
            conversation_metrics: None,
            pending_tool_timings: Arc::new(DashMap::new()),
            storage: None,
            persisted_messages: None,
            pressure_threshold_bytes: None,
            pressure_counter: std::sync::Arc::new(std::sync::atomic::AtomicUsize::new(0)),
            pressure_warned: std::sync::Arc::new(std::sync::atomic::AtomicBool::new(false)),
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
        let bus = make_bus();
        let mut ws_rx = bus.subscribe_ws();
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
            event_bus: bus,
            cancel: CancellationToken::new(),
            tool_names,
            tool_executors: executors,
            agent_id: "test-agent".to_string(),
            conversation_id: "test-conv".to_string(),
            permission_manager: Arc::new(PermissionManager::new()),
            agent_permissions: HashMap::new(),
            metrics: Arc::new(bridge_core::AgentMetrics::new()),
            conversation_metrics: None,
            pending_tool_timings: Arc::new(DashMap::new()),
            storage: None,
            persisted_messages: None,
            pressure_threshold_bytes: None,
            pressure_counter: std::sync::Arc::new(std::sync::atomic::AtomicUsize::new(0)),
            pressure_warned: std::sync::Arc::new(std::sync::atomic::AtomicBool::new(false)),
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

        // Should emit ToolCallStarted + ToolCallCompleted
        let _start = ws_rx.try_recv().expect("should have tool_call_start");
        let result_event = ws_rx.try_recv().expect("should have tool_call_result");
        assert_eq!(result_event.event_type, BridgeEventType::ToolCallCompleted);
        assert_eq!(result_event.data["is_error"], false);
        let result_str = result_event.data["result"].as_str().unwrap();
        assert!(result_str.contains("repaired_bash_output"));
    }

    #[tokio::test]
    async fn test_emitter_auto_repairs_whitespace() {
        let bus = make_bus();
        let mut ws_rx = bus.subscribe_ws();
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
            event_bus: bus,
            cancel: CancellationToken::new(),
            tool_names,
            tool_executors: executors,
            agent_id: "test-agent".to_string(),
            conversation_id: "test-conv".to_string(),
            permission_manager: Arc::new(PermissionManager::new()),
            agent_permissions: HashMap::new(),
            metrics: Arc::new(bridge_core::AgentMetrics::new()),
            conversation_metrics: None,
            pending_tool_timings: Arc::new(DashMap::new()),
            storage: None,
            persisted_messages: None,
            pressure_threshold_bytes: None,
            pressure_counter: std::sync::Arc::new(std::sync::atomic::AtomicUsize::new(0)),
            pressure_warned: std::sync::Arc::new(std::sync::atomic::AtomicBool::new(false)),
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

        let _start = ws_rx.try_recv().expect("should have tool_call_start");
        let result_event = ws_rx.try_recv().expect("should have tool_call_result");
        assert_eq!(result_event.event_type, BridgeEventType::ToolCallCompleted);
        assert_eq!(result_event.data["is_error"], false);
    }
}
