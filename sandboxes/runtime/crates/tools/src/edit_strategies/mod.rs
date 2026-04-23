//! Edit matching strategies inspired by OpenCode.
//!
//! When `apply_edit()` tries to find `old_string` in `content`, it uses a
//! chain of increasingly fuzzy strategies. The first match wins.

mod block_anchor;
mod context_aware;
mod escape;
mod indentation;
mod line_trimmed;
mod multi_occurrence;
mod simple;
mod trimmed_boundary;
mod whitespace;

use block_anchor::BlockAnchorReplacer;
use context_aware::ContextAwareReplacer;
use escape::EscapeNormalizedReplacer;
use indentation::IndentationFlexibleReplacer;
use line_trimmed::LineTrimmedReplacer;
use multi_occurrence::MultiOccurrenceReplacer;
use simple::SimpleReplacer;
use trimmed_boundary::TrimmedBoundaryReplacer;
use whitespace::WhitespaceNormalizedReplacer;

/// Trait for replacement strategies.
pub(crate) trait Replacer {
    #[allow(dead_code)]
    fn name(&self) -> &str;
    /// Try to replace `old_string` with `new_string` in `content`.
    /// Returns `Some(new_content)` on success, `None` if the strategy cannot match.
    fn try_replace(
        &self,
        content: &str,
        old_string: &str,
        new_string: &str,
        replace_all: bool,
    ) -> Option<(String, usize)>;
}

/// Return the ordered list of all strategies.
pub(crate) fn all_strategies() -> Vec<Box<dyn Replacer>> {
    vec![
        Box::new(SimpleReplacer),
        Box::new(LineTrimmedReplacer),
        Box::new(BlockAnchorReplacer),
        Box::new(WhitespaceNormalizedReplacer),
        Box::new(IndentationFlexibleReplacer),
        Box::new(EscapeNormalizedReplacer),
        Box::new(TrimmedBoundaryReplacer),
        Box::new(ContextAwareReplacer),
        Box::new(MultiOccurrenceReplacer),
    ]
}

#[cfg(test)]
mod tests;
