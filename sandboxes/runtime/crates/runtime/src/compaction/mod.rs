//! Token-estimation and history-serialization helpers.
//!
//! Originally hosted an LLM-summarization "compaction" pass; that
//! mechanism has been removed in favor of immortal-mode chain handoff
//! (see `crate::immortal`). The remaining helpers — fast/precise token
//! count and a stable text serialization of message history — are still
//! used by the immortal flow's checkpoint extractor and pressure probe.
//!
//! The module name is kept for source-stability; consider renaming to
//! `tokens` or folding into `immortal/` in a future cleanup.

mod serialize;
mod tokens;

pub use serialize::serialize_history_for_summary;
pub use tokens::{estimate_tokens, estimate_tokens_fast};
