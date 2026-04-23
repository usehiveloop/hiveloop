//! `PromptHook` impl for `ToolCallEmitter` — the `on_tool_call` /
//! `on_tool_result` entrypoints rig-core invokes around each tool dispatch.
//!
//! `on_tool_call` is split into small helper methods that each resolve one
//! pre-dispatch concern (name repair → permissions → argument validation
//! → specialized tool interception). Each helper returns
//! `Result<State, ToolCallHookAction>` so the hot path can use `?`-style
//! early-return on `Skip` while preserving the exact execution order of
//! the original inline implementation.

use std::time::Instant;

use bridge_core::event::{BridgeEvent, BridgeEventType};
use rig::agent::{HookAction, PromptHook, ToolCallHookAction};
use rig::completion::CompletionModel;
use serde_json::json;
use tools::agent::SubAgentToolParams;
use tools::bash::BashArgs;
use tools::self_agent::AgentToolParams;
use tracing::{info, warn};

use super::permission::PermissionCtx;
use super::truncate::{truncate_if_needed, validate_tool_args, Truncated};
use super::ToolCallEmitter;

/// Step-by-step state shared across the `on_tool_call` helpers.
struct CallCtx {
    call_start: Instant,
    id: String,
    arguments: serde_json::Value,
}

impl ToolCallEmitter {
    /// Emit the `tool_call_start` log + event. First thing that happens
    /// for every call; never short-circuits.
    fn log_tool_call_start(&self, tool_name: &str, args: &str, ctx: &CallCtx) {
        info!(
            agent_id = %self.agent_id,
            conversation_id = %self.conversation_id,
            tool_name = tool_name,
            tool_call_id = %ctx.id,
            arguments = %Truncated::new(args, 100),
            "tool_call_start"
        );
        self.event_bus.emit(BridgeEvent::new(
            BridgeEventType::ToolCallStarted,
            &self.agent_id,
            &self.conversation_id,
            json!({"id": &ctx.id, "name": tool_name, "arguments": &ctx.arguments}),
        ));
    }

    /// Resolve the effective tool name via normalize/case-insensitive/fuzzy
    /// match. Returns `(effective_name, name_was_repaired)` or `Skip` with
    /// an unknown-tool error.
    fn resolve_effective_name(
        &self,
        tool_name: &str,
        ctx: &CallCtx,
    ) -> Result<(String, bool), ToolCallHookAction> {
        if self.tool_names.is_empty() {
            return Ok((tool_name.to_string(), false));
        }
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
                Ok((resolved, repaired))
            }
            None => {
                let error = self.unknown_tool_error(tool_name);
                let duration_ms = ctx.call_start.elapsed().as_millis() as u64;
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
                    json!({"id": &ctx.id, "result": &error, "is_error": true, "duration_ms": duration_ms, "tool_name": tool_name}),
                ));
                self.persist_tool_interaction(tool_name, &ctx.id, &ctx.arguments, &error, true);
                Err(ToolCallHookAction::Skip { reason: error })
            }
        }
    }

    /// Validate tool arguments against the tool's JSON schema. Catches
    /// malformed calls early so the agent can retry immediately.
    fn validate_args(&self, effective_name: &str, ctx: &CallCtx) -> Result<(), ToolCallHookAction> {
        let Some(executor) = self.tool_executors.get(effective_name) else {
            return Ok(());
        };
        let schema = executor.parameters_schema();
        let Err(validation_error) = validate_tool_args(&ctx.arguments, &schema) else {
            return Ok(());
        };
        let error = json!({
            "error": format!("Invalid arguments for tool '{}': {}", effective_name, validation_error)
        })
        .to_string();
        let duration_ms = ctx.call_start.elapsed().as_millis() as u64;
        self.metrics
            .record_tool_call_detailed(effective_name, true, duration_ms);
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
            json!({"id": &ctx.id, "result": &error, "is_error": true, "duration_ms": duration_ms, "tool_name": effective_name}),
        ));
        self.persist_tool_interaction(effective_name, &ctx.id, &ctx.arguments, &error, true);
        Err(ToolCallHookAction::Skip {
            reason: truncate_if_needed(error),
        })
    }

    /// Check for background bash / agent / sub_agent interception. Returns
    /// `Some(action)` if the call was fully handled here, or `None` to
    /// continue on to the generic dispatch path.
    async fn maybe_intercept_special(
        &self,
        effective_name: &str,
        args: &str,
        ctx: &CallCtx,
    ) -> Option<ToolCallHookAction> {
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
                    return Some(
                        self.handle_background_bash(bash_args, ctx.id.clone(), ctx.call_start)
                            .await,
                    );
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
                return Some(
                    self.handle_self_agent_tool(agent_params, ctx.id.clone(), ctx.call_start)
                        .await,
                );
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
                return Some(
                    self.handle_sub_agent_tool(sub_agent_params, ctx.id.clone(), ctx.call_start)
                        .await,
                );
            }
        }

        None
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

        let ctx = CallCtx {
            call_start,
            id,
            arguments,
        };

        self.log_tool_call_start(tool_name, args, &ctx);

        // 1. Resolve effective tool name.
        let (effective_name, name_was_repaired) = match self.resolve_effective_name(tool_name, &ctx)
        {
            Ok(v) => v,
            Err(action) => return action,
        };

        // 2. Check per-agent permissions (may await for approval).
        let perm_ctx = PermissionCtx {
            call_start: ctx.call_start,
            id: &ctx.id,
            arguments: &ctx.arguments,
        };
        if let Err(action) = self.enforce_permissions(&effective_name, &perm_ctx).await {
            return action;
        }

        // 3. Validate arguments against JSON schema.
        if let Err(action) = self.validate_args(&effective_name, &ctx) {
            return action;
        }

        // 4. Specialized tool interception (background bash / agent / sub_agent).
        if let Some(action) = self
            .maybe_intercept_special(&effective_name, args, &ctx)
            .await
        {
            return action;
        }

        // 5. If the name was repaired, rig-core won't find the tool under the
        //    original name. Execute the tool ourselves and return Skip.
        if name_was_repaired {
            return self
                .execute_repaired_tool(&effective_name, args, ctx.id, call_start)
                .await;
        }

        // 6. rig-core will dispatch. Store timing for on_tool_result.
        self.pending_tool_timings
            .insert(ctx.id, (call_start, effective_name));

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
        self.handle_tool_result(tool_name, tool_call_id, internal_call_id, args, result)
            .await
    }
}
