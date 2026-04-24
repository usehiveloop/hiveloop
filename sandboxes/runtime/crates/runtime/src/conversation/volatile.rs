/// Default turns between stable system-reminder refreshes when an agent
/// doesn't override `system_reminder_refresh_turns`.
const DEFAULT_SYSTEM_REMINDER_REFRESH_TURNS: u32 = 10;

/// Wire together per-message reminder extraction, date-change detection,
/// volatile reminder assembly, and final user-text layout into a single
/// one-call helper.
#[allow(clippy::too_many_arguments)]
pub(super) async fn build_layout_text(
    incoming: &super::params::IncomingMessage,
    date_tracker: &mut crate::system_reminder::DateTracker,
    immortal_state: &Option<crate::immortal::ImmortalState>,
    journal_state: &Option<std::sync::Arc<tools::journal::JournalState>>,
    standalone_agent: bool,
    turn_count: usize,
    ping_state: &Option<tools::ping_me_back::PingState>,
    tool_executors: &std::collections::HashMap<String, std::sync::Arc<dyn tools::ToolExecutor>>,
    system_reminder: &str,
    user_text: &str,
    system_reminder_refresh_turns: Option<u32>,
) -> String {
    let per_message_reminder = match incoming {
        super::params::IncomingMessage::User(msg) => msg
            .system_reminder
            .as_deref()
            .map(|r| format!("<system-reminder>\n{}\n</system-reminder>", r)),
        _ => None,
    };

    let date_change_reminder = date_tracker.check_date_change();

    let volatile_reminder = build_volatile_reminder(
        immortal_state,
        journal_state,
        standalone_agent,
        turn_count,
        per_message_reminder,
        ping_state,
        tool_executors,
    )
    .await;

    // The stable system reminder (skills + subagents + todos + date) is
    // prepended to the user message only on turn 0 and every N turns
    // thereafter. Re-emitting on every turn makes each user message a
    // fresh cache miss; never re-emitting lets the reminder go stale.
    // N comes from `AgentConfig::system_reminder_refresh_turns`, defaulting
    // to 10. Values <1 are clamped to 1 (every-turn refresh).
    let refresh_every = system_reminder_refresh_turns
        .map(|n| n.max(1) as usize)
        .unwrap_or(DEFAULT_SYSTEM_REMINDER_REFRESH_TURNS as usize);
    let is_refresh_turn = turn_count == 0 || turn_count.is_multiple_of(refresh_every);
    let effective_system_reminder = if is_refresh_turn { system_reminder } else { "" };

    assemble_final_user_text(
        date_change_reminder,
        effective_system_reminder,
        user_text,
        volatile_reminder,
    )
}

/// Assemble the final user-turn text with cache-aware layout:
///   `[date_change? head] [stable system_reminder] [user text] [volatile reminder tail]`
///
/// Stable content stays at deterministic head position so prior turns' user
/// messages remain byte-locked for cache reuse, and volatile content stays
/// at the tail so it never leaks into the cached region.
pub(super) fn assemble_final_user_text(
    date_change_reminder: Option<String>,
    system_reminder: &str,
    user_text: &str,
    volatile_reminder: String,
) -> String {
    let mut pieces: Vec<String> = Vec::with_capacity(4);
    if let Some(date_reminder) = date_change_reminder {
        pieces.push(date_reminder);
    }
    if !system_reminder.is_empty() {
        pieces.push(system_reminder.to_string());
    }
    pieces.push(user_text.to_string());
    if !volatile_reminder.is_empty() {
        pieces.push(volatile_reminder);
    }
    pieces.join("\n\n")
}

/// Build the volatile reminder tail for the current turn.
///
/// "Volatile" here means content that changes per-turn — immortal chain
/// progress, environment snapshots, per-message reminders, pending ping
/// timers. Keeping these at the tail (rather than head) preserves prompt
/// cache hits on prior turns' user messages.
pub(super) async fn build_volatile_reminder(
    immortal_state: &Option<crate::immortal::ImmortalState>,
    journal_state: &Option<std::sync::Arc<tools::journal::JournalState>>,
    standalone_agent: bool,
    turn_count: usize,
    per_message_reminder: Option<String>,
    ping_state: &Option<tools::ping_me_back::PingState>,
    tool_executors: &std::collections::HashMap<String, std::sync::Arc<dyn tools::ToolExecutor>>,
) -> String {
    let mut volatile_reminder = String::new();
    let append_volatile = |acc: &mut String, block: String| {
        if block.is_empty() {
            return;
        }
        if acc.is_empty() {
            *acc = block;
        } else {
            acc.push_str("\n\n");
            acc.push_str(&block);
        }
    };

    if let (Some(ref imm_state), Some(ref js)) = (immortal_state, journal_state) {
        let journal_count = js.entries().await.len();
        let immortal_section = crate::system_reminder::SystemReminder::new()
            .with_immortal_context(imm_state.current_chain_index, journal_count)
            .build();
        append_volatile(&mut volatile_reminder, immortal_section);
    }

    // Environment reminder fires on turn 0 (so the agent knows where it is
    // from the very first turn) and every 5 turns thereafter as a refresh
    // for memory/CPU/disk which drift. The workspace_dir line is the
    // load-bearing piece — agents otherwise invent `/tmp`, `/workspace`,
    // and other phantom paths. Previously gated on `standalone_agent` but
    // every agent benefits from knowing its CWD, not just sandboxed ones.
    // The pre-installed-tools section still only makes sense inside the
    // dev-box sandbox template; `standalone_agent` controls that.
    if turn_count.is_multiple_of(5) {
        let env_section = crate::environment::EnvironmentSnapshot::collect()
            .format_reminder_with_options(standalone_agent);
        append_volatile(
            &mut volatile_reminder,
            format!("<system-reminder>\n{}\n</system-reminder>", env_section),
        );
    }

    if let Some(pmr) = per_message_reminder {
        append_volatile(&mut volatile_reminder, pmr);
    }

    // Live todos block — fetched from the registered todo tool every turn so
    // the model always sees the current list, not a conversation-start
    // snapshot.
    match current_todos(tool_executors).await {
        Some(todos) if !todos.is_empty() => {
            tracing::info!(todo_count = todos.len(), "volatile_todos_injected");
            let todos_section = crate::system_reminder::SystemReminder::new()
                .with_todos(&todos)
                .build();
            append_volatile(&mut volatile_reminder, todos_section);
        }
        Some(_) => {
            tracing::debug!("volatile_todos_empty");
        }
        None => {
            tracing::warn!(
                has_todoread = tool_executors.contains_key("todoread"),
                has_todowrite = tool_executors.contains_key("todowrite"),
                "volatile_todos_none — tool downcast failed"
            );
        }
    }

    if let Some(ref ps) = ping_state {
        let pings = ps.list().await;
        let ping_reminder = tools::ping_me_back::format_pending_pings_reminder(&pings);
        if !ping_reminder.is_empty() {
            append_volatile(
                &mut volatile_reminder,
                format!("<system-reminder>\n{}\n</system-reminder>", ping_reminder),
            );
        }
    }

    volatile_reminder
}

/// Fetch the current live todo list from the registered `todoread` /
/// `todowrite` tool (whichever is in the registry — both share the same
/// `TodoState`). Returns `None` when neither tool is registered, and
/// `Some(vec![])` when the tool is present but the list is empty.
///
/// The downcast pattern mirrors `supervisor::helpers::get_todos_from_registry`
/// but lives here so `build_volatile_reminder` doesn't need to reach into
/// the supervisor module.
async fn current_todos(
    tool_executors: &std::collections::HashMap<String, std::sync::Arc<dyn tools::ToolExecutor>>,
) -> Option<Vec<crate::system_reminder::TodoItem>> {
    // Prefer `todoread` (read-only intent) but fall back to `todowrite` —
    // both hold the same shared `TodoState` instance.
    let state = tool_executors
        .get("todoread")
        .and_then(|t| {
            t.as_ref()
                .as_any()
                .downcast_ref::<tools::todo::TodoReadTool>()
                .map(|tool| tool.state().clone())
        })
        .or_else(|| {
            tool_executors.get("todowrite").and_then(|t| {
                t.as_ref()
                    .as_any()
                    .downcast_ref::<tools::todo::TodoWriteTool>()
                    .map(|tool| tool.state().clone())
            })
        })?;
    let todos = state.get().await;
    Some(
        todos
            .into_iter()
            .map(|t| crate::system_reminder::TodoItem {
                content: t.content,
                status: t.status,
                priority: t.priority,
            })
            .collect(),
    )
}
