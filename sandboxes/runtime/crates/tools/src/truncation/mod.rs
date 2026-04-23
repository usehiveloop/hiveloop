mod core;
mod persist;
#[cfg(test)]
mod tests;

pub use core::{
    truncate_output, truncate_output_directed, TruncationDirection, TruncationResult, MAX_BYTES,
    MAX_LINES,
};
pub use persist::cleanup_old_outputs;
