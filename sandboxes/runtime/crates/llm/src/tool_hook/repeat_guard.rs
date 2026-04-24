//! Identical-consecutive-tool-call guard.
//!
//! Some models (notably Qwen3.6-plus when the chat template strips prior
//! `<think>` blocks — see
//! <https://github.com/badlogic/pi-mono/issues/3325>) re-emit the same
//! tool call with the same arguments turn after turn because they lose
//! track of what they've already done. Without this guard the
//! conversation runs to `MaxTurnError` while the context balloons with
//! identical result copies — one of our benches hit 6M prompt tokens and
//! $2 in billing before stopping.
//!
//! The guard tracks the last tool call's (name, canonical_args). When
//! the same pair repeats a third time in a row, we short-circuit the
//! dispatch and return a synthetic result that tells the model to stop
//! repeating itself. The first two repeats go through untouched (a
//! legitimate re-read of a file that was just modified is allowed).
//!
//! Comparison is structural on `serde_json::Value` via its `PartialEq`
//! impl, so key-order differences don't defeat the check. Serde_json's
//! default Value::Object is a `BTreeMap` when the `preserve_order`
//! feature is off (our workspace default), so `to_string()` is also
//! canonical — but we rely on `Value::eq`, not the string form, to be
//! safe regardless.

use serde_json::Value;

/// Number of identical consecutive calls allowed before the guard fires.
/// Third (or later) consecutive call gets intercepted. Tuned so one
/// legitimate retry (e.g. re-reading a file the model just edited) still
/// goes through; three in a row is clearly a loop.
pub(super) const REPEAT_THRESHOLD: usize = 3;

/// Mutable state tracked across tool calls within a single conversation.
/// Threaded through `ToolCallEmitter` as `Arc<Mutex<RepeatGuardState>>`
/// so every turn's emitter clone shares it.
#[derive(Default, Debug)]
pub struct RepeatGuardState {
    /// `(tool_name, args_value, consecutive_count)`. `None` before the
    /// first call. Resets whenever a call with different name or args
    /// arrives.
    last: Option<(String, Value, usize)>,
}

impl RepeatGuardState {
    /// Record a tool call and decide whether to intercept it.
    ///
    /// Returns `Some(hint)` when the current call is the `REPEAT_THRESHOLD`-th
    /// or later identical consecutive call — the caller should return
    /// `Skip { reason: hint }` to the tool dispatch loop so the real tool
    /// never runs. Returns `None` when the call should proceed normally.
    ///
    /// Non-repeating calls reset the counter, so a single interleaved
    /// different call breaks the streak and lets the model resume.
    pub(super) fn record(&mut self, tool_name: &str, arguments: &Value) -> Option<String> {
        let is_same = matches!(&self.last, Some((n, a, _)) if n == tool_name && a == arguments);

        if is_same {
            let count = {
                let (_, _, c) = self.last.as_mut().expect("guarded by is_same");
                *c += 1;
                *c
            };
            if count >= REPEAT_THRESHOLD {
                return Some(build_hint(tool_name, count));
            }
            None
        } else {
            self.last = Some((tool_name.to_string(), arguments.clone(), 1));
            None
        }
    }
}

fn build_hint(tool_name: &str, count: usize) -> String {
    format!(
        "Repeat-call guard: you have called `{tool_name}` with identical arguments {count} \
         times in a row. The result has been the same every time. This call was NOT executed. \
         Stop repeating it and do something different: (a) use the information you already have \
         and advance to the next step of the task, (b) call a different tool, or \
         (c) call `{tool_name}` with different arguments."
    )
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn first_two_identical_calls_pass() {
        let mut g = RepeatGuardState::default();
        assert!(g.record("Read", &json!({"file_path": "/x"})).is_none());
        assert!(g.record("Read", &json!({"file_path": "/x"})).is_none());
    }

    #[test]
    fn third_identical_call_trips() {
        let mut g = RepeatGuardState::default();
        g.record("Read", &json!({"file_path": "/x"}));
        g.record("Read", &json!({"file_path": "/x"}));
        let hint = g.record("Read", &json!({"file_path": "/x"}));
        assert!(hint.is_some(), "expected third identical call to trip");
        let msg = hint.unwrap();
        assert!(msg.contains("Repeat-call guard"));
        assert!(msg.contains("Read"));
        assert!(msg.contains("3"));
    }

    #[test]
    fn different_tool_resets_streak() {
        let mut g = RepeatGuardState::default();
        g.record("Read", &json!({"file_path": "/x"}));
        g.record("Read", &json!({"file_path": "/x"}));
        // A different tool call breaks the run.
        assert!(g.record("bash", &json!({"command": "ls"})).is_none());
        // Back to Read — streak starts over.
        assert!(g.record("Read", &json!({"file_path": "/x"})).is_none());
        assert!(g.record("Read", &json!({"file_path": "/x"})).is_none());
    }

    #[test]
    fn different_args_reset_streak() {
        let mut g = RepeatGuardState::default();
        g.record("Read", &json!({"file_path": "/x"}));
        g.record("Read", &json!({"file_path": "/x"}));
        // Different arg value — new streak.
        assert!(g.record("Read", &json!({"file_path": "/y"})).is_none());
        assert!(g.record("Read", &json!({"file_path": "/y"})).is_none());
    }

    #[test]
    fn key_order_does_not_defeat_match() {
        // Value::eq is structural; serde_json's Object is BTreeMap so
        // parsing either order produces the same internal state, but
        // this belt-and-braces test catches regressions if someone
        // enables the `preserve_order` feature later.
        let mut g = RepeatGuardState::default();
        let a: Value = serde_json::from_str(r#"{"a":1,"b":2}"#).unwrap();
        let b: Value = serde_json::from_str(r#"{"b":2,"a":1}"#).unwrap();
        g.record("t", &a);
        g.record("t", &b);
        assert!(
            g.record("t", &a).is_some(),
            "key-order variants must count as identical"
        );
    }

    #[test]
    fn subsequent_repeats_keep_firing() {
        let mut g = RepeatGuardState::default();
        g.record("Read", &json!({"file_path": "/x"}));
        g.record("Read", &json!({"file_path": "/x"}));
        assert!(g.record("Read", &json!({"file_path": "/x"})).is_some());
        // 4th, 5th, ... should also fire.
        assert!(g.record("Read", &json!({"file_path": "/x"})).is_some());
        assert!(g.record("Read", &json!({"file_path": "/x"})).is_some());
    }
}
