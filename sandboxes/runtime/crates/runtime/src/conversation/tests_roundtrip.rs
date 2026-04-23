use super::convert_messages;
use bridge_core::conversation::{ContentBlock, Message, Role, ToolCall, ToolResult};
use serde_json::json;

fn make_message(role: Role, content: Vec<ContentBlock>) -> Message {
    Message {
        role,
        content,
        timestamp: chrono::Utc::now(),
        system_reminder: None,
    }
}

#[test]
fn test_convert_messages_full_conversation() {
    let messages = vec![
        make_message(
            Role::User,
            vec![ContentBlock::Text {
                text: "Review auth.rs".into(),
            }],
        ),
        make_message(
            Role::Assistant,
            vec![
                ContentBlock::Text {
                    text: "I'll read the file.".into(),
                },
                ContentBlock::ToolCall(ToolCall {
                    id: "call_001".into(),
                    name: "read_file".into(),
                    arguments: json!({"path": "src/auth.rs"}),
                }),
            ],
        ),
        make_message(
            Role::Tool,
            vec![ContentBlock::ToolResult(ToolResult {
                tool_call_id: "call_001".into(),
                content: "fn login() { ... }".into(),
                is_error: false,
            })],
        ),
        make_message(
            Role::Assistant,
            vec![ContentBlock::Text {
                text: "The auth module looks good.".into(),
            }],
        ),
    ];

    let rig_messages = convert_messages(&messages);
    assert_eq!(rig_messages.len(), 4);

    assert_eq!(
        rig_messages[0],
        rig::message::Message::user("Review auth.rs")
    );
    match &rig_messages[1] {
        rig::message::Message::Assistant { content, .. } => {
            assert_eq!(content.iter().count(), 2);
        }
        _ => panic!("expected Assistant"),
    }
    assert_eq!(
        rig_messages[2],
        rig::message::Message::tool_result("call_001", "fn login() { ... }")
    );
    assert_eq!(
        rig_messages[3],
        rig::message::Message::assistant("The auth module looks good.")
    );
}

#[test]
fn test_convert_messages_multiple_tool_results_expanded() {
    let messages = vec![make_message(
        Role::Tool,
        vec![
            ContentBlock::ToolResult(ToolResult {
                tool_call_id: "call_a".into(),
                content: "result a".into(),
                is_error: false,
            }),
            ContentBlock::ToolResult(ToolResult {
                tool_call_id: "call_b".into(),
                content: "result b".into(),
                is_error: false,
            }),
        ],
    )];

    let rig_messages = convert_messages(&messages);
    assert_eq!(rig_messages.len(), 2);
    assert_eq!(
        rig_messages[0],
        rig::message::Message::tool_result("call_a", "result a")
    );
    assert_eq!(
        rig_messages[1],
        rig::message::Message::tool_result("call_b", "result b")
    );
}

#[test]
fn test_convert_messages_skips_system() {
    let messages = vec![
        make_message(
            Role::System,
            vec![ContentBlock::Text {
                text: "You are helpful.".into(),
            }],
        ),
        make_message(Role::User, vec![ContentBlock::Text { text: "hi".into() }]),
    ];
    let rig_messages = convert_messages(&messages);
    assert_eq!(rig_messages.len(), 1);
    assert_eq!(rig_messages[0], rig::message::Message::user("hi"));
}

#[test]
fn test_roundtrip_multi_turn_with_tools() {
    // Simulates a realistic multi-turn conversation that would be
    // sent by the control plane for hydration.
    let messages = vec![
        make_message(
            Role::User,
            vec![ContentBlock::Text {
                text: "Find security issues in auth.rs".into(),
            }],
        ),
        make_message(
            Role::Assistant,
            vec![
                ContentBlock::Text {
                    text: "I'll read the file.".into(),
                },
                ContentBlock::ToolCall(ToolCall {
                    id: "call_1".into(),
                    name: "read_file".into(),
                    arguments: json!({"path": "src/auth.rs"}),
                }),
            ],
        ),
        make_message(
            Role::Tool,
            vec![ContentBlock::ToolResult(ToolResult {
                tool_call_id: "call_1".into(),
                content: "pub fn login() {}".into(),
                is_error: false,
            })],
        ),
        make_message(
            Role::Assistant,
            vec![
                ContentBlock::Text {
                    text: "Let me also check for rate limiting.".into(),
                },
                ContentBlock::ToolCall(ToolCall {
                    id: "call_2".into(),
                    name: "grep".into(),
                    arguments: json!({"pattern": "rate_limit", "path": "src/"}),
                }),
            ],
        ),
        make_message(
            Role::Tool,
            vec![ContentBlock::ToolResult(ToolResult {
                tool_call_id: "call_2".into(),
                content: "No matches found.".into(),
                is_error: false,
            })],
        ),
        make_message(
            Role::Assistant,
            vec![ContentBlock::Text {
                text: "No rate limiting found. This is a security issue.".into(),
            }],
        ),
        make_message(
            Role::User,
            vec![ContentBlock::Text {
                text: "Fix it.".into(),
            }],
        ),
    ];

    let rig_messages = convert_messages(&messages);

    // 7 input messages → 7 rig messages (all preserved)
    assert_eq!(rig_messages.len(), 7);

    // Verify the sequence of roles
    assert!(matches!(
        rig_messages[0],
        rig::message::Message::User { .. }
    ));
    assert!(matches!(
        rig_messages[1],
        rig::message::Message::Assistant { .. }
    ));
    // Tool result is modeled as User in rig
    assert!(matches!(
        rig_messages[2],
        rig::message::Message::User { .. }
    ));
    assert!(matches!(
        rig_messages[3],
        rig::message::Message::Assistant { .. }
    ));
    assert!(matches!(
        rig_messages[4],
        rig::message::Message::User { .. }
    ));
    assert!(matches!(
        rig_messages[5],
        rig::message::Message::Assistant { .. }
    ));
    assert!(matches!(
        rig_messages[6],
        rig::message::Message::User { .. }
    ));
}
