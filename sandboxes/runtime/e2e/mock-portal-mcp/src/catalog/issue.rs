use serde_json::{json, Value};

use super::tool;

pub(super) fn definitions() -> Vec<Value> {
    vec![
        // ── Team ──
        tool(
            "listTeams",
            "List all teams in the workspace",
            json!({"type": "object", "properties": {}, "required": []}),
        ),
        tool(
            "getTeam",
            "Get details of a specific team",
            json!({"type": "object", "properties": {"teamId": {"type": "string", "description": "The team ID or key"}}, "required": ["teamId"]}),
        ),
        tool(
            "listTeamMembers",
            "List members of a team",
            json!({"type": "object", "properties": {"teamId": {"type": "string", "description": "The team ID or key"}}, "required": ["teamId"]}),
        ),
        // ── Issue ──
        tool(
            "createIssue",
            "Create a new issue on a team",
            json!({
                "type": "object",
                "properties": {
                    "teamId": {"type": "string", "description": "Team ID"},
                    "title": {"type": "string", "description": "Issue title"},
                    "description": {"type": "string", "description": "Issue description"},
                    "priority": {"type": "integer", "description": "Priority (0=none, 1=urgent, 2=high, 3=medium, 4=low)"},
                    "statusId": {"type": "string", "description": "Status ID"},
                    "assigneeId": {"type": "string", "description": "Assignee member ID"},
                    "labelIds": {"type": "array", "items": {"type": "string"}, "description": "Label IDs"}
                },
                "required": ["teamId", "title"]
            }),
        ),
        tool(
            "listTeamIssues",
            "List issues for a team",
            json!({
                "type": "object",
                "properties": {
                    "teamId": {"type": "string", "description": "Team ID or key"},
                    "status": {"type": "string", "description": "Filter by status name"},
                    "limit": {"type": "integer", "description": "Max results"}
                },
                "required": ["teamId"]
            }),
        ),
        tool(
            "getIssue",
            "Get a specific issue by identifier",
            json!({
                "type": "object",
                "properties": {"issueId": {"type": "string", "description": "Issue ID or identifier (e.g. ENG-42)"}},
                "required": ["issueId"]
            }),
        ),
        tool(
            "updateIssue",
            "Update an existing issue",
            json!({
                "type": "object",
                "properties": {
                    "issueId": {"type": "string", "description": "Issue ID or identifier"},
                    "title": {"type": "string"},
                    "description": {"type": "string"},
                    "priority": {"type": "integer"},
                    "statusId": {"type": "string"},
                    "assigneeId": {"type": "string"}
                },
                "required": ["issueId"]
            }),
        ),
        tool(
            "updateIssueStatus",
            "Update the status of an issue",
            json!({
                "type": "object",
                "properties": {
                    "issueId": {"type": "string", "description": "Issue ID"},
                    "statusId": {"type": "string", "description": "New status ID"}
                },
                "required": ["issueId", "statusId"]
            }),
        ),
        tool(
            "assignIssue",
            "Assign an issue to a member",
            json!({
                "type": "object",
                "properties": {
                    "issueId": {"type": "string"},
                    "assigneeId": {"type": "string"}
                },
                "required": ["issueId", "assigneeId"]
            }),
        ),
        tool(
            "deleteIssue",
            "Delete an issue",
            json!({
                "type": "object",
                "properties": {"issueId": {"type": "string"}},
                "required": ["issueId"]
            }),
        ),
        tool(
            "updateIssuePriority",
            "Update issue priority",
            json!({
                "type": "object",
                "properties": {
                    "issueId": {"type": "string"},
                    "priority": {"type": "integer"}
                },
                "required": ["issueId", "priority"]
            }),
        ),
        tool(
            "updateIssueAssignee",
            "Update issue assignee",
            json!({
                "type": "object",
                "properties": {
                    "issueId": {"type": "string"},
                    "assigneeId": {"type": "string"}
                },
                "required": ["issueId", "assigneeId"]
            }),
        ),
        tool(
            "updateIssueLabels",
            "Update issue labels",
            json!({
                "type": "object",
                "properties": {
                    "issueId": {"type": "string"},
                    "labelIds": {"type": "array", "items": {"type": "string"}}
                },
                "required": ["issueId", "labelIds"]
            }),
        ),
        tool(
            "updateIssueProject",
            "Update issue project",
            json!({
                "type": "object",
                "properties": {
                    "issueId": {"type": "string"},
                    "projectId": {"type": "string"}
                },
                "required": ["issueId", "projectId"]
            }),
        ),
        // ── Dependencies ──
        tool(
            "addIssueDependency",
            "Add a dependency between issues",
            json!({
                "type": "object",
                "properties": {
                    "issueId": {"type": "string"},
                    "relatedIssueId": {"type": "string"},
                    "dependencyType": {"type": "string", "description": "blocks or is_blocked_by"}
                },
                "required": ["issueId", "relatedIssueId", "dependencyType"]
            }),
        ),
        tool(
            "removeIssueDependency",
            "Remove a dependency",
            json!({
                "type": "object",
                "properties": {"dependencyId": {"type": "string"}},
                "required": ["dependencyId"]
            }),
        ),
        tool(
            "listIssueDependencies",
            "List dependencies for an issue",
            json!({
                "type": "object",
                "properties": {"issueId": {"type": "string"}},
                "required": ["issueId"]
            }),
        ),
        // ── Status ──
        tool(
            "listStatuses",
            "List all statuses for a team",
            json!({
                "type": "object",
                "properties": {"teamId": {"type": "string"}},
                "required": ["teamId"]
            }),
        ),
    ]
}
