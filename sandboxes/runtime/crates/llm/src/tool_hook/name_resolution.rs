//! Tool-name normalization, resolution, and unknown-tool error construction.

use super::ToolCallEmitter;

/// Normalize a tool name by stripping common LLM artifacts.
pub(super) fn normalize_tool_name(name: &str) -> String {
    let mut s = name.trim().to_string();

    // Strip wrapping double quotes: "bash" → bash
    if s.len() >= 2 && s.starts_with('"') && s.ends_with('"') {
        s = s[1..s.len() - 1].to_string();
    }
    // Strip wrapping single quotes: 'bash' → bash
    if s.len() >= 2 && s.starts_with('\'') && s.ends_with('\'') {
        s = s[1..s.len() - 1].to_string();
    }
    // Strip wrapping backticks: `bash` → bash
    if s.len() >= 2 && s.starts_with('`') && s.ends_with('`') {
        s = s[1..s.len() - 1].to_string();
    }

    // Trim again in case of nested whitespace
    s.trim().to_string()
}

impl ToolCallEmitter {
    /// Try to resolve a raw tool name to a known canonical tool name.
    ///
    /// Resolution order:
    /// 1. Exact match (fast path)
    /// 2. Exact match after normalization (trim, strip quotes/backticks)
    /// 3. Case-insensitive match
    /// 4. High-confidence Levenshtein match (score > 0.8)
    ///
    /// Returns `None` if the name cannot be confidently resolved.
    pub(super) fn resolve_tool_name(&self, raw_name: &str) -> Option<String> {
        // 1. Exact match (most common case)
        if self.tool_names.contains(raw_name) {
            return Some(raw_name.to_string());
        }

        // 2. Normalize and try exact match
        let normalized = normalize_tool_name(raw_name);
        if normalized != raw_name && self.tool_names.contains(&normalized) {
            return Some(normalized);
        }

        // 3. Case-insensitive match on the normalized name
        let lower = normalized.to_lowercase();
        for known in &self.tool_names {
            if known.to_lowercase() == lower {
                return Some(known.clone());
            }
        }

        // 4. High-confidence Levenshtein match (>0.8)
        let mut best: Option<(String, f64)> = None;
        for known in &self.tool_names {
            let score = strsim::normalized_levenshtein(&lower, &known.to_lowercase());
            if score > best.as_ref().map_or(0.0, |(_, d)| *d) {
                best = Some((known.clone(), score));
            }
        }
        if let Some((name, score)) = best {
            if score > 0.8 {
                return Some(name);
            }
        }

        None
    }

    /// Build an error message for a tool name that could not be resolved.
    ///
    /// Includes a lower-confidence Levenshtein suggestion (>0.4) to help the
    /// model self-correct on the next attempt.
    pub(super) fn unknown_tool_error(&self, name: &str) -> String {
        let normalized = normalize_tool_name(name);
        let lower = normalized.to_lowercase();

        // Levenshtein distance suggestion
        let mut best: Option<(&str, f64)> = None;
        for known in &self.tool_names {
            let score = strsim::normalized_levenshtein(&lower, &known.to_lowercase());
            if score > best.as_ref().map_or(0.0, |(_, d)| *d) {
                best = Some((known, score));
            }
        }

        let names: Vec<&str> = self.tool_names.iter().map(|s| s.as_str()).collect();
        if let Some((suggestion, score)) = best {
            if score > 0.4 {
                return format!(
                    "Unknown tool '{}'. Did you mean '{}'? Available tools: [{}]",
                    name,
                    suggestion,
                    names.join(", ")
                );
            }
        }

        format!(
            "Unknown tool '{}'. Available tools: [{}]",
            name,
            names.join(", ")
        )
    }
}
