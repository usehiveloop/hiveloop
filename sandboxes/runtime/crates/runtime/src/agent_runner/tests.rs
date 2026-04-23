use super::session_store::extract_conversation_id;
use super::AgentSessionStore;

#[test]
fn test_session_store_get_or_create_empty() {
    let store = AgentSessionStore::new(String::new(), None);
    let history = store.get_or_create("task-1");
    assert!(history.is_empty());
}

#[test]
fn test_session_store_save_and_retrieve() {
    let store = AgentSessionStore::new(String::new(), None);
    let history = vec![rig::message::Message::user("hello")];
    store.save("task-1".to_string(), history.clone());
    let retrieved = store.get_or_create("task-1");
    assert_eq!(retrieved.len(), 1);
}

#[test]
fn test_session_store_remove_by_prefix() {
    let store = AgentSessionStore::new(String::new(), None);
    store.save(
        "conv-123-task-1".to_string(),
        vec![rig::message::Message::user("a")],
    );
    store.save(
        "conv-123-task-2".to_string(),
        vec![rig::message::Message::user("b")],
    );
    store.save(
        "conv-456-task-1".to_string(),
        vec![rig::message::Message::user("c")],
    );

    store.remove_by_prefix("conv-123");

    assert!(store.get_or_create("conv-123-task-1").is_empty());
    assert!(store.get_or_create("conv-123-task-2").is_empty());
    assert_eq!(store.get_or_create("conv-456-task-1").len(), 1);
}

// ── Fix #6: Indexed session store tests ────────────────────────────

#[test]
fn test_session_store_indexed_removal_with_uuid_keys() {
    let store = AgentSessionStore::new(String::new(), None);
    // Use realistic UUID-format task_ids: "{conv_uuid}-{task_uuid}"
    let conv_id = "a1b2c3d4-e5f6-7890-abcd-ef1234567890";
    let task1 = format!("{}-{}", conv_id, "11111111-1111-1111-1111-111111111111");
    let task2 = format!("{}-{}", conv_id, "22222222-2222-2222-2222-222222222222");
    let other = format!(
        "{}-{}",
        "ffffffff-ffff-ffff-ffff-ffffffffffff", "33333333-3333-3333-3333-333333333333"
    );

    store.save(task1.clone(), vec![rig::message::Message::user("a")]);
    store.save(task2.clone(), vec![rig::message::Message::user("b")]);
    store.save(other.clone(), vec![rig::message::Message::user("c")]);

    assert_eq!(store.len(), 3);

    // Remove by conversation prefix (UUID = 36 chars)
    store.remove_by_prefix(conv_id);

    assert!(store.get_or_create(&task1).is_empty());
    assert!(store.get_or_create(&task2).is_empty());
    assert_eq!(store.get_or_create(&other).len(), 1);
    assert_eq!(store.len(), 1);
}

#[test]
fn test_session_store_len_and_is_empty() {
    let store = AgentSessionStore::new(String::new(), None);
    assert!(store.is_empty());
    assert_eq!(store.len(), 0);

    store.save("task-1".to_string(), vec![]);
    assert!(!store.is_empty());
    assert_eq!(store.len(), 1);
}

#[test]
fn test_extract_conversation_id_valid() {
    // UUID is 36 chars: 8-4-4-4-12
    let conv_id = "a1b2c3d4-e5f6-7890-abcd-ef1234567890";
    let task_uuid = "11111111-1111-1111-1111-111111111111";
    let task_id = format!("{}-{}", conv_id, task_uuid);
    let result = extract_conversation_id(&task_id);
    assert_eq!(result, Some(conv_id.to_string()));
}

#[test]
fn test_extract_conversation_id_too_short() {
    assert_eq!(extract_conversation_id("short"), None);
    assert_eq!(extract_conversation_id(""), None);
}

#[test]
fn test_session_store_fallback_for_non_uuid_keys() {
    let store = AgentSessionStore::new(String::new(), None);
    // Non-UUID keys that won't match the index — should still be cleaned up via fallback
    store.save(
        "myprefix-task-1".to_string(),
        vec![rig::message::Message::user("a")],
    );
    store.save(
        "myprefix-task-2".to_string(),
        vec![rig::message::Message::user("b")],
    );

    store.remove_by_prefix("myprefix");

    assert!(store.get_or_create("myprefix-task-1").is_empty());
    assert!(store.get_or_create("myprefix-task-2").is_empty());
}
