use super::{apply_breakpoints, mark_message};
use serde_json::json;

#[test]
fn places_breakpoints_on_system_and_last_two() {
    let mut body = json!({
        "messages": [
            { "role": "system", "content": "you are helpful" },
            { "role": "user",   "content": "hi" },
            { "role": "assistant", "content": "hello" },
            { "role": "user", "content": "do work" },
            { "role": "assistant", "content": "ok" },
        ]
    });
    let n = apply_breakpoints(&mut body);
    assert_eq!(n, 3, "system + last 2 non-system => 3 markers");

    let messages = body.get("messages").unwrap().as_array().unwrap();
    assert!(messages[0].get("cache_control").is_some(), "system marked");
    assert!(
        messages[3].get("cache_control").is_some(),
        "second-last user marked"
    );
    assert!(
        messages[4].get("cache_control").is_some(),
        "last assistant marked"
    );
    // Middle messages NOT marked
    assert!(messages[1].get("cache_control").is_none());
    assert!(messages[2].get("cache_control").is_none());
}

#[test]
fn places_marker_on_last_content_block_when_array() {
    let mut msg = json!({
        "role": "user",
        "content": [
            { "type": "text", "text": "first" },
            { "type": "text", "text": "second" },
        ]
    });
    assert!(mark_message(&mut msg));
    let blocks = msg.get("content").unwrap().as_array().unwrap();
    assert!(
        blocks[0].get("cache_control").is_none(),
        "first block NOT marked"
    );
    assert!(
        blocks[1].get("cache_control").is_some(),
        "last block marked"
    );
}

#[test]
fn caps_at_two_system_messages() {
    let mut body = json!({
        "messages": [
            { "role": "system", "content": "s1" },
            { "role": "system", "content": "s2" },
            { "role": "system", "content": "s3" },
            { "role": "user", "content": "hi" },
        ]
    });
    let _ = apply_breakpoints(&mut body);
    let messages = body.get("messages").unwrap().as_array().unwrap();
    assert!(messages[0].get("cache_control").is_some());
    assert!(messages[1].get("cache_control").is_some());
    assert!(
        messages[2].get("cache_control").is_none(),
        "third system not marked (cap=2)"
    );
}

#[test]
fn handles_missing_messages() {
    let mut body = json!({ "model": "test" });
    let n = apply_breakpoints(&mut body);
    assert_eq!(n, 0);
}

#[test]
fn handles_only_one_message() {
    let mut body = json!({
        "messages": [
            { "role": "user", "content": "single message" },
        ]
    });
    let n = apply_breakpoints(&mut body);
    assert_eq!(n, 1, "the one message gets marked as last");
}

#[test]
fn does_not_double_mark_when_system_is_in_tail() {
    // Edge case: only 2 messages total, one system + one user. Both are in
    // both candidate sets; should mark each ONCE.
    let mut body = json!({
        "messages": [
            { "role": "system", "content": "sys" },
            { "role": "user",   "content": "u" },
        ]
    });
    let n = apply_breakpoints(&mut body);
    assert_eq!(n, 2);
    let messages = body.get("messages").unwrap().as_array().unwrap();
    // Each message has exactly one cache_control field
    assert_eq!(messages[0]["cache_control"], json!({"type": "ephemeral"}));
    assert_eq!(messages[1]["cache_control"], json!({"type": "ephemeral"}));
}
