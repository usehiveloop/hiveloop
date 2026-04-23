use super::Replacer;

/// SimpleReplacer — exact string match.
pub(super) struct SimpleReplacer;

impl Replacer for SimpleReplacer {
    fn name(&self) -> &str {
        "simple"
    }

    fn try_replace(
        &self,
        content: &str,
        old_string: &str,
        new_string: &str,
        replace_all: bool,
    ) -> Option<(String, usize)> {
        let count = content.matches(old_string).count();
        if count == 0 {
            return None;
        }
        if count > 1 && !replace_all {
            return None; // Ambiguous — let caller handle the error
        }
        if replace_all {
            Some((content.replace(old_string, new_string), count))
        } else {
            Some((content.replacen(old_string, new_string, 1), 1))
        }
    }
}
