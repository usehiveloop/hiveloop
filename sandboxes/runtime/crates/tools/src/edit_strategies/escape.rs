use super::Replacer;

/// EscapeNormalizedReplacer — normalize escape sequences.
pub(super) struct EscapeNormalizedReplacer;

fn normalize_escapes(s: &str) -> String {
    s.replace("\\n", "\n")
        .replace("\\t", "\t")
        .replace("\\\"", "\"")
        .replace("\\'", "'")
        .replace("\\\\", "\\")
}

impl Replacer for EscapeNormalizedReplacer {
    fn name(&self) -> &str {
        "escape_normalized"
    }

    fn try_replace(
        &self,
        content: &str,
        old_string: &str,
        new_string: &str,
        replace_all: bool,
    ) -> Option<(String, usize)> {
        let norm_old = normalize_escapes(old_string);
        if norm_old == old_string {
            // No escape normalization possible
            return None;
        }

        let count = content.matches(&norm_old).count();
        if count == 0 {
            return None;
        }
        if count > 1 && !replace_all {
            return None;
        }

        let norm_new = normalize_escapes(new_string);
        if replace_all {
            Some((content.replace(&norm_old, &norm_new), count))
        } else {
            Some((content.replacen(&norm_old, &norm_new, 1), 1))
        }
    }
}
