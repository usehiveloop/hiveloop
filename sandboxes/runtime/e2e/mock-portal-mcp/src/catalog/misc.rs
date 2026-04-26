use serde_json::{json, Value};

use super::tool;

pub(super) fn definitions() -> Vec<Value> {
    vec![
        // ── Label ──
        tool(
            "listLabels",
            "List labels for a team",
            json!({
                "type": "object",
                "properties": {"teamId": {"type": "string"}},
                "required": ["teamId"]
            }),
        ),
        tool(
            "listWorkspaceLabels",
            "List all workspace labels",
            json!({"type": "object", "properties": {}, "required": []}),
        ),
        tool(
            "createLabel",
            "Create a new label",
            json!({
                "type": "object",
                "properties": {
                    "name": {"type": "string"},
                    "color": {"type": "string"},
                    "teamId": {"type": "string"}
                },
                "required": ["name"]
            }),
        ),
        // ── Comment ──
        tool(
            "createComment",
            "Create a comment on an issue",
            json!({
                "type": "object",
                "properties": {
                    "issueId": {"type": "string"},
                    "body": {"type": "string"}
                },
                "required": ["issueId", "body"]
            }),
        ),
        tool(
            "listComments",
            "List comments on an issue",
            json!({
                "type": "object",
                "properties": {"issueId": {"type": "string"}},
                "required": ["issueId"]
            }),
        ),
        tool(
            "updateComment",
            "Update a comment",
            json!({
                "type": "object",
                "properties": {
                    "commentId": {"type": "string"},
                    "body": {"type": "string"}
                },
                "required": ["commentId", "body"]
            }),
        ),
        // ── View ──
        tool(
            "createView",
            "Create a custom view",
            json!({
                "type": "object",
                "properties": {
                    "name": {"type": "string"},
                    "filters": {"type": "object"},
                    "teamId": {"type": "string"}
                },
                "required": ["name"]
            }),
        ),
        // ── Member ──
        tool(
            "listWorkspaceMembers",
            "List all workspace members",
            json!({"type": "object", "properties": {}, "required": []}),
        ),
        // ── Portal Agent ──
        tool(
            "pingHuman",
            "Ping a human team member for attention",
            json!({
                "type": "object",
                "properties": {
                    "message": {"type": "string", "description": "Message to send"},
                    "userId": {"type": "string", "description": "Target user ID (optional)"},
                    "urgency": {"type": "string", "description": "low, medium, high"}
                },
                "required": ["message"]
            }),
        ),
        tool(
            "pingMeBack",
            "Request a callback ping after a delay",
            json!({
                "type": "object",
                "properties": {
                    "delayMinutes": {"type": "integer"},
                    "context": {"type": "string"}
                },
                "required": ["delayMinutes"]
            }),
        ),
        tool(
            "submitApprovalRequest",
            "Submit an action for human approval",
            json!({
                "type": "object",
                "properties": {
                    "action": {"type": "string", "description": "The action requiring approval"},
                    "details": {"type": "string", "description": "Detailed description of the action"},
                    "issueId": {"type": "string", "description": "Related issue ID"}
                },
                "required": ["action", "details"]
            }),
        ),
    ]
}
