pub mod diff;
pub mod poller;
pub mod updater;

pub use diff::{compute_diff, AgentDiff};
pub use poller::{fetch_agents, fetch_conversations, run_sync_loop};
pub use updater::apply_diff;
