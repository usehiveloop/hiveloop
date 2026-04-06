pub mod harness;

pub use harness::{
    ConversationTurn, SseEvent, SseStream, TestHarness, ToolCallLogEntry, WebhookEntry, WebhookLog,
    WsEvent, WsEventStream,
};

/// Log a test step with a visible separator. Use this before every assertion
/// and significant operation so the test output reads like a narrative.
#[macro_export]
macro_rules! step {
    ($($arg:tt)*) => {
        eprintln!("\n  \x1b[36m▸\x1b[0m {}", format!($($arg)*));
    };
}

/// Log a passed assertion with a green checkmark.
#[macro_export]
macro_rules! check {
    ($expr:expr, $($arg:tt)*) => {{
        let msg = format!($($arg)*);
        assert!($expr, "{}", msg);
        eprintln!("    \x1b[32m✓\x1b[0m {}", msg);
    }};
}

/// Log a passed equality assertion with a green checkmark.
#[macro_export]
macro_rules! check_eq {
    ($left:expr, $right:expr, $($arg:tt)*) => {{
        let msg = format!($($arg)*);
        assert_eq!($left, $right, "{}", msg);
        eprintln!("    \x1b[32m✓\x1b[0m {} (= {:?})", msg, $right);
    }};
}
