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
    system_reminder: &str,
    user_text: &str,
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
    )
    .await;

    assemble_final_user_text(
        date_change_reminder,
        system_reminder,
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

    if standalone_agent && turn_count.is_multiple_of(5) {
        let env_section = crate::environment::EnvironmentSnapshot::collect().format_reminder();
        append_volatile(
            &mut volatile_reminder,
            format!("<system-reminder>\n{}\n</system-reminder>", env_section),
        );
    }

    if let Some(pmr) = per_message_reminder {
        append_volatile(&mut volatile_reminder, pmr);
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
