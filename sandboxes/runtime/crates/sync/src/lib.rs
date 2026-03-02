pub mod diff;
pub mod poller;
pub mod updater;

pub use diff::{compute_diff, AgentDiff};
pub use poller::{fetch_agents, run_sync_loop};
pub use updater::apply_diff;
