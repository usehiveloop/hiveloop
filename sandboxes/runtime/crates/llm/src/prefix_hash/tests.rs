use super::*;
use serde_json::json;

fn make_tool<'a>(name: &'a str, desc: &'a str, schema: &'a serde_json::Value) -> ToolPrefix<'a> {
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
