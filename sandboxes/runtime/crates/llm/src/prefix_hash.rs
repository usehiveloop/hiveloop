//! Prefix-hash computation for prompt-cache observability.
//!
//! The cacheable prefix of an LLM request is, in byte order:
//!     [ tools array ] → [ system preamble ] → [ messages... ]
//!
//! Bridge builds the first two halves deterministically — preamble is set
//! once at agent construction, tools come from a name-sorted registry. A
//! SHA-256 digest of `(preamble || tools)` therefore fingerprints the
//! stable prefix: if the digest changes across two agents that should be
//! equivalent, something has silently broken cache reuse.
//!
//! The digest is computed once at agent build time, stored on `BridgeAgent`,
//! and logged on every `llm_request_start`/`llm_request_complete`. Grep the
//! logs for a single agent across many requests — the hash should never move.

use sha2::{Digest, Sha256};

/// A tool definition as it contributes to the cacheable prefix.
///
/// Includes name, description, and the JSON Schema. Any change in any of
/// these bytes is a full cache bust.
pub struct ToolPrefix<'a> {
    pub name: &'a str,
    pub description: &'a str,
    pub schema: &'a serde_json::Value,
}

/// Compute a hex SHA-256 digest of the cacheable prefix.
///
/// Callers must pass tools in the exact order Bridge will send them
/// (name-sorted, after any registry filtering). Schemas must be in the
/// exact form Bridge will serialize. The function canonicalizes JSON
/// serialization via `serde_json::to_vec` which preserves Map insertion
/// order — upstream code is responsible for using deterministic map types
/// (BTreeMap / IndexMap) when building schemas.
pub fn compute_prefix_hash(preamble: &str, tools: &[ToolPrefix<'_>]) -> String {
    let mut hasher = Sha256::new();
    hasher.update(b"PREAMBLE:");
    hasher.update((preamble.len() as u64).to_be_bytes());
    hasher.update(preamble.as_bytes());
    hasher.update(b"\nTOOLS:");
    hasher.update((tools.len() as u64).to_be_bytes());
    for t in tools {
        hasher.update(b"\nTOOL:");
        hasher.update((t.name.len() as u64).to_be_bytes());
        hasher.update(t.name.as_bytes());
        hasher.update(b":");
        hasher.update((t.description.len() as u64).to_be_bytes());
        hasher.update(t.description.as_bytes());
        hasher.update(b":SCHEMA:");
        let schema_bytes = serde_json::to_vec(t.schema).unwrap_or_default();
        hasher.update((schema_bytes.len() as u64).to_be_bytes());
        hasher.update(&schema_bytes);
    }
    format!("{:x}", hasher.finalize())
}

/// Hash only the preamble (system prompt). Useful for log diagnostics when
/// debugging cache misses — tells you whether a prefix drift is in the
/// preamble or in the tool list.
pub fn compute_preamble_hash(preamble: &str) -> String {
    let mut hasher = Sha256::new();
    hasher.update(preamble.as_bytes());
    format!("{:x}", hasher.finalize())
}

/// Hash only the tools portion of the prefix. Complements
/// [`compute_preamble_hash`] for split diagnostics.
pub fn compute_tools_hash(tools: &[ToolPrefix<'_>]) -> String {
    let mut hasher = Sha256::new();
    hasher.update((tools.len() as u64).to_be_bytes());
    for t in tools {
        hasher.update(b"\nTOOL:");
        hasher.update((t.name.len() as u64).to_be_bytes());
        hasher.update(t.name.as_bytes());
        hasher.update(b":");
        hasher.update((t.description.len() as u64).to_be_bytes());
        hasher.update(t.description.as_bytes());
        hasher.update(b":SCHEMA:");
        let schema_bytes = serde_json::to_vec(t.schema).unwrap_or_default();
        hasher.update((schema_bytes.len() as u64).to_be_bytes());
        hasher.update(&schema_bytes);
    }
    format!("{:x}", hasher.finalize())
}

/// Convenience: split-hash counterpart of [`prefix_hash_from_definitions`].
pub fn split_hashes_from_definitions(
    preamble: &str,
    tools: &[rig::completion::ToolDefinition],
) -> (String, String) {
    let prefixes: Vec<ToolPrefix> = tools
        .iter()
        .map(|t| ToolPrefix {
            name: &t.name,
            description: &t.description,
            schema: &t.parameters,
        })
        .collect();
    (
        compute_preamble_hash(preamble),
        compute_tools_hash(&prefixes),
    )
}

/// Heuristic hygiene check: look for markers in the preamble that commonly
/// indicate dynamic interpolation (timestamps, years, UUIDs, ISO dates).
///
/// Returns the specific markers found. A hit does NOT mean the preamble is
/// wrong — e.g. a system prompt legitimately referencing "as of 2023" is
/// static. It means "look twice; this might be drifting the prefix hash on
/// every rebuild."
pub fn suspected_volatile_markers(preamble: &str) -> Vec<&'static str> {
    let mut hits = Vec::new();
    // ISO-8601-ish dates: "2024-12-31", "2025-01-01T..."
    let iso_date = regex_lite_find(preamble, |c| c.is_ascii_digit(), 4, b'-')
        && contains_pattern(preamble, b"-", 2);
    if iso_date {
        hits.push("iso-date");
    }
    // UUIDs: 8-4-4-4-12 hex
    if preamble.as_bytes().windows(36).any(is_uuid_shape) {
        hits.push("uuid");
    }
    // "Current date" / "Today is" — explicit date narration
    let lower_count = preamble.to_ascii_lowercase();
    if lower_count.contains("today is ") || lower_count.contains("current date") {
        hits.push("current-date-phrase");
    }
    // Long digit runs — likely unix timestamp or request id
    if preamble
        .as_bytes()
        .windows(10)
        .any(|w| w.iter().all(|b| b.is_ascii_digit()))
    {
        hits.push("long-digit-run");
    }
    hits
}

fn is_uuid_shape(w: &[u8]) -> bool {
    // 8 hex - 4 hex - 4 hex - 4 hex - 12 hex
    let groups = [(0usize, 8usize), (9, 13), (14, 18), (19, 23), (24, 36)];
    let dashes = [8, 13, 18, 23];
    for (a, b) in groups {
        if !w[a..b].iter().all(|c| c.is_ascii_hexdigit()) {
            return false;
        }
    }
    for d in dashes {
        if w[d] != b'-' {
            return false;
        }
    }
    true
}

fn regex_lite_find<F: Fn(char) -> bool>(s: &str, f: F, run: usize, sep: u8) -> bool {
    let bytes = s.as_bytes();
    let mut i = 0;
    while i + run < bytes.len() {
        let window = &bytes[i..i + run];
        if window.iter().all(|b| f(*b as char)) && bytes.get(i + run) == Some(&sep) {
            return true;
        }
        i += 1;
    }
    false
}

fn contains_pattern(s: &str, pat: &[u8], min_occurrences: usize) -> bool {
    s.as_bytes()
        .windows(pat.len())
        .filter(|w| *w == pat)
        .count()
        >= min_occurrences
}

/// Convenience constructor from `rig::completion::ToolDefinition` slices.
pub fn prefix_hash_from_definitions(
    preamble: &str,
    tools: &[rig::completion::ToolDefinition],
) -> String {
    let prefixes: Vec<ToolPrefix> = tools
        .iter()
        .map(|t| ToolPrefix {
            name: &t.name,
            description: &t.description,
            schema: &t.parameters,
        })
        .collect();
    compute_prefix_hash(preamble, &prefixes)
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    fn make_tool<'a>(
        name: &'a str,
        desc: &'a str,
        schema: &'a serde_json::Value,
    ) -> ToolPrefix<'a> {
        ToolPrefix {
            name,
            description: desc,
            schema,
        }
    }

    #[test]
    fn empty_prefix_is_stable() {
        let a = compute_prefix_hash("", &[]);
        let b = compute_prefix_hash("", &[]);
        assert_eq!(a, b);
        assert_eq!(a.len(), 64, "sha256 hex is 64 chars");
    }

    #[test]
    fn same_inputs_produce_same_hash() {
        let schema = json!({"type":"object","properties":{"x":{"type":"string"}}});
        let a = compute_prefix_hash(
            "you are helpful",
            &[make_tool("bash", "run shell", &schema)],
        );
        let b = compute_prefix_hash(
            "you are helpful",
            &[make_tool("bash", "run shell", &schema)],
        );
        assert_eq!(a, b);
    }

    #[test]
    fn preamble_change_changes_hash() {
        let schema = json!({"type":"object"});
        let a = compute_prefix_hash("you are helpful", &[make_tool("bash", "d", &schema)]);
        let b = compute_prefix_hash("you are helpful.", &[make_tool("bash", "d", &schema)]);
        assert_ne!(a, b);
    }

    #[test]
    fn tool_name_change_changes_hash() {
        let schema = json!({});
        let a = compute_prefix_hash("p", &[make_tool("bash", "d", &schema)]);
        let b = compute_prefix_hash("p", &[make_tool("Bash", "d", &schema)]);
        assert_ne!(a, b);
    }

    #[test]
    fn tool_description_change_changes_hash() {
        let schema = json!({});
        let a = compute_prefix_hash("p", &[make_tool("bash", "one", &schema)]);
        let b = compute_prefix_hash("p", &[make_tool("bash", "two", &schema)]);
        assert_ne!(a, b);
    }

    #[test]
    fn tool_schema_change_changes_hash() {
        let s1 = json!({"type":"object"});
        let s2 = json!({"type":"object","required":["x"]});
        let a = compute_prefix_hash("p", &[make_tool("bash", "d", &s1)]);
        let b = compute_prefix_hash("p", &[make_tool("bash", "d", &s2)]);
        assert_ne!(a, b);
    }

    #[test]
    fn tool_order_change_changes_hash() {
        // This enforces the caller's obligation to pre-sort tools. If the
        // caller swaps order, the hash must flip — exactly matching what
        // the provider cache will do.
        let schema = json!({});
        let a = compute_prefix_hash(
            "p",
            &[make_tool("a", "d1", &schema), make_tool("b", "d2", &schema)],
        );
        let b = compute_prefix_hash(
            "p",
            &[make_tool("b", "d2", &schema), make_tool("a", "d1", &schema)],
        );
        assert_ne!(a, b);
    }

    #[test]
    fn preamble_and_tools_hashes_compose_into_prefix_hash() {
        // The pair of split hashes identifies the same prefix as the combined
        // hash — different bytes, but same "did anything change" answer.
        let schema = json!({"type":"object"});
        let tools = [make_tool("bash", "run", &schema)];
        let preamble = "be helpful";

        let a1 = compute_preamble_hash(preamble);
        let a2 = compute_preamble_hash(preamble);
        let b1 = compute_tools_hash(&tools);
        let b2 = compute_tools_hash(&tools);

        assert_eq!(a1, a2);
        assert_eq!(b1, b2);
        assert_eq!(a1.len(), 64);
        assert_eq!(b1.len(), 64);
        assert_ne!(
            a1, b1,
            "hashes of preamble vs tools must be distinguishable"
        );
    }

    #[test]
    fn suspected_volatile_markers_catches_iso_date() {
        assert!(suspected_volatile_markers("Today: 2026-04-18").contains(&"iso-date"));
    }

    #[test]
    fn suspected_volatile_markers_catches_uuid() {
        let p = "request_id=550e8400-e29b-41d4-a716-446655440000 go";
        assert!(suspected_volatile_markers(p).contains(&"uuid"));
    }

    #[test]
    fn suspected_volatile_markers_catches_today_phrase() {
        assert!(suspected_volatile_markers("Today is Friday.").contains(&"current-date-phrase"));
    }

    #[test]
    fn suspected_volatile_markers_catches_digit_run() {
        // unix timestamp shape
        assert!(suspected_volatile_markers("Timestamp: 1745000000").contains(&"long-digit-run"));
    }

    #[test]
    fn suspected_volatile_markers_passes_static_preamble() {
        let static_preamble =
            "You are a helpful agent. Use the tools available to accomplish the user's task.";
        assert!(suspected_volatile_markers(static_preamble).is_empty());
    }

    #[test]
    fn length_prefixing_avoids_collisions() {
        // Without length prefixes, "ab"+"c" and "a"+"bc" would collide.
        // Our scheme includes byte-lengths so they cannot.
        let schema = json!({});
        let a = compute_prefix_hash("ab", &[make_tool("c", "d", &schema)]);
        let b = compute_prefix_hash("a", &[make_tool("bc", "d", &schema)]);
        assert_ne!(a, b);
    }
}
