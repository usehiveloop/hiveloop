use serde_json::{json, Value};

/// Returns the full list of MCP tool definitions (name, description, inputSchema).
pub fn tool_definitions() -> Vec<Value> {
    vec![
        // ── Team ──
        tool("listTeams", "List all teams in the workspace", json!({"type": "object", "properties": {}, "required": []})),
        tool("getTeam", "Get details of a specific team", json!({"type": "object", "properties": {"teamId": {"type": "string", "description": "The team ID or key"}}, "required": ["teamId"]})),
        tool("listTeamMembers", "List members of a team", json!({"type": "object", "properties": {"teamId": {"type": "string", "description": "The team ID or key"}}, "required": ["teamId"]})),

        // ── Issue ──
        tool("createIssue", "Create a new issue on a team", json!({
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
        })),
        tool("listTeamIssues", "List issues for a team", json!({
            "type": "object",
            "properties": {
                "teamId": {"type": "string", "description": "Team ID or key"},
                "status": {"type": "string", "description": "Filter by status name"},
                "limit": {"type": "integer", "description": "Max results"}
            },
            "required": ["teamId"]
        })),
        tool("getIssue", "Get a specific issue by identifier", json!({
            "type": "object",
            "properties": {"issueId": {"type": "string", "description": "Issue ID or identifier (e.g. ENG-42)"}},
            "required": ["issueId"]
        })),
        tool("updateIssue", "Update an existing issue", json!({
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
        })),
        tool("updateIssueStatus", "Update the status of an issue", json!({
            "type": "object",
            "properties": {
                "issueId": {"type": "string", "description": "Issue ID"},
                "statusId": {"type": "string", "description": "New status ID"}
            },
            "required": ["issueId", "statusId"]
        })),
        tool("assignIssue", "Assign an issue to a member", json!({
            "type": "object",
            "properties": {
                "issueId": {"type": "string"},
                "assigneeId": {"type": "string"}
            },
            "required": ["issueId", "assigneeId"]
        })),
        tool("deleteIssue", "Delete an issue", json!({
            "type": "object",
            "properties": {"issueId": {"type": "string"}},
            "required": ["issueId"]
        })),
        tool("updateIssuePriority", "Update issue priority", json!({
            "type": "object",
            "properties": {
                "issueId": {"type": "string"},
                "priority": {"type": "integer"}
            },
            "required": ["issueId", "priority"]
        })),
        tool("updateIssueAssignee", "Update issue assignee", json!({
            "type": "object",
            "properties": {
                "issueId": {"type": "string"},
                "assigneeId": {"type": "string"}
            },
            "required": ["issueId", "assigneeId"]
        })),
        tool("updateIssueLabels", "Update issue labels", json!({
            "type": "object",
            "properties": {
                "issueId": {"type": "string"},
                "labelIds": {"type": "array", "items": {"type": "string"}}
            },
            "required": ["issueId", "labelIds"]
        })),
        tool("updateIssueProject", "Update issue project", json!({
            "type": "object",
            "properties": {
                "issueId": {"type": "string"},
                "projectId": {"type": "string"}
            },
            "required": ["issueId", "projectId"]
        })),

        // ── Dependencies ──
        tool("addIssueDependency", "Add a dependency between issues", json!({
            "type": "object",
            "properties": {
                "issueId": {"type": "string"},
                "relatedIssueId": {"type": "string"},
                "type": {"type": "string", "description": "blocks or is_blocked_by"}
            },
            "required": ["issueId", "relatedIssueId", "type"]
        })),
        tool("removeIssueDependency", "Remove a dependency", json!({
            "type": "object",
            "properties": {"dependencyId": {"type": "string"}},
            "required": ["dependencyId"]
        })),
        tool("listIssueDependencies", "List dependencies for an issue", json!({
            "type": "object",
            "properties": {"issueId": {"type": "string"}},
            "required": ["issueId"]
        })),

        // ── Status ──
        tool("listStatuses", "List all statuses for a team", json!({
            "type": "object",
            "properties": {"teamId": {"type": "string"}},
            "required": ["teamId"]
        })),

        // ── Project ──
        tool("createProject", "Create a new project", json!({
            "type": "object",
            "properties": {
                "name": {"type": "string"},
                "description": {"type": "string"},
                "targetDate": {"type": "string"}
            },
            "required": ["name"]
        })),
        tool("getProject", "Get project details", json!({
            "type": "object",
            "properties": {"projectId": {"type": "string"}},
            "required": ["projectId"]
        })),
        tool("listProjects", "List all projects", json!({"type": "object", "properties": {}, "required": []})),
        tool("updateProject", "Update a project", json!({
            "type": "object",
            "properties": {
                "projectId": {"type": "string"},
                "name": {"type": "string"},
                "description": {"type": "string"},
                "status": {"type": "string"}
            },
            "required": ["projectId"]
        })),
        tool("deleteProject", "Delete a project", json!({
            "type": "object",
            "properties": {"projectId": {"type": "string"}},
            "required": ["projectId"]
        })),

        // ── Document ──
        tool("createDocument", "Create a new document", json!({
            "type": "object",
            "properties": {
                "title": {"type": "string"},
                "content": {"type": "string"},
                "projectId": {"type": "string", "description": "Optional project to attach to"}
            },
            "required": ["title", "content"]
        })),
        tool("getDocument", "Get a document by ID", json!({
            "type": "object",
            "properties": {"documentId": {"type": "string"}},
            "required": ["documentId"]
        })),
        tool("listDocuments", "List all documents", json!({
            "type": "object",
            "properties": {"projectId": {"type": "string", "description": "Filter by project"}},
            "required": []
        })),
        tool("updateDocument", "Update a document", json!({
            "type": "object",
            "properties": {
                "documentId": {"type": "string"},
                "title": {"type": "string"},
                "content": {"type": "string"}
            },
            "required": ["documentId"]
        })),
        tool("searchDocuments", "Search documents by query", json!({
            "type": "object",
            "properties": {"query": {"type": "string"}},
            "required": ["query"]
        })),
        tool("moveDocument", "Move a document to a different project", json!({
            "type": "object",
            "properties": {
                "documentId": {"type": "string"},
                "projectId": {"type": "string"}
            },
            "required": ["documentId", "projectId"]
        })),
        tool("reorderDocument", "Reorder a document within its project", json!({
            "type": "object",
            "properties": {
                "documentId": {"type": "string"},
                "position": {"type": "integer"}
            },
            "required": ["documentId", "position"]
        })),

        // ── Label ──
        tool("listLabels", "List labels for a team", json!({
            "type": "object",
            "properties": {"teamId": {"type": "string"}},
            "required": ["teamId"]
        })),
        tool("listWorkspaceLabels", "List all workspace labels", json!({"type": "object", "properties": {}, "required": []})),
        tool("createLabel", "Create a new label", json!({
            "type": "object",
            "properties": {
                "name": {"type": "string"},
                "color": {"type": "string"},
                "teamId": {"type": "string"}
            },
            "required": ["name"]
        })),

        // ── Comment ──
        tool("createComment", "Create a comment on an issue", json!({
            "type": "object",
            "properties": {
                "issueId": {"type": "string"},
                "body": {"type": "string"}
            },
            "required": ["issueId", "body"]
        })),
        tool("listComments", "List comments on an issue", json!({
            "type": "object",
            "properties": {"issueId": {"type": "string"}},
            "required": ["issueId"]
        })),
        tool("updateComment", "Update a comment", json!({
            "type": "object",
            "properties": {
                "commentId": {"type": "string"},
                "body": {"type": "string"}
            },
            "required": ["commentId", "body"]
        })),

        // ── View ──
        tool("createView", "Create a custom view", json!({
            "type": "object",
            "properties": {
                "name": {"type": "string"},
                "filters": {"type": "object"},
                "teamId": {"type": "string"}
            },
            "required": ["name"]
        })),

        // ── Member ──
        tool("listWorkspaceMembers", "List all workspace members", json!({"type": "object", "properties": {}, "required": []})),

        // ── Portal Agent ──
        tool("pingHuman", "Ping a human team member for attention", json!({
            "type": "object",
            "properties": {
                "message": {"type": "string", "description": "Message to send"},
                "userId": {"type": "string", "description": "Target user ID (optional)"},
                "urgency": {"type": "string", "description": "low, medium, high"}
            },
            "required": ["message"]
        })),
        tool("pingMeBack", "Request a callback ping after a delay", json!({
            "type": "object",
            "properties": {
                "delayMinutes": {"type": "integer"},
                "context": {"type": "string"}
            },
            "required": ["delayMinutes"]
        })),
        tool("submitApprovalRequest", "Submit an action for human approval", json!({
            "type": "object",
            "properties": {
                "action": {"type": "string", "description": "The action requiring approval"},
                "details": {"type": "string", "description": "Detailed description of the action"},
                "issueId": {"type": "string", "description": "Related issue ID"}
            },
            "required": ["action", "details"]
        })),

        // ── GitHub PR ──
        tool("listIssuePullRequests", "List pull requests linked to an issue", json!({
            "type": "object",
            "properties": {"issueId": {"type": "string"}},
            "required": ["issueId"]
        })),
        tool("getPullRequest", "Get details of a pull request", json!({
            "type": "object",
            "properties": {
                "prNumber": {"type": "integer", "description": "PR number"},
                "repoFullName": {"type": "string", "description": "owner/repo"}
            },
            "required": ["prNumber"]
        })),
        tool("fetchPullRequestComments", "Fetch comments on a pull request", json!({
            "type": "object",
            "properties": {"prNumber": {"type": "integer"}},
            "required": ["prNumber"]
        })),
        tool("fetchPullRequestReviews", "Fetch reviews on a pull request", json!({
            "type": "object",
            "properties": {"prNumber": {"type": "integer"}},
            "required": ["prNumber"]
        })),
        tool("fetchPullRequestChecks", "Fetch CI checks on a pull request", json!({
            "type": "object",
            "properties": {"prNumber": {"type": "integer"}},
            "required": ["prNumber"]
        })),
        tool("addPullRequestComment", "Add a comment on a pull request", json!({
            "type": "object",
            "properties": {
                "prNumber": {"type": "integer"},
                "body": {"type": "string"},
                "path": {"type": "string", "description": "File path for inline comment"},
                "line": {"type": "integer", "description": "Line number for inline comment"}
            },
            "required": ["prNumber", "body"]
        })),
        tool("getGithubInstallationToken", "Get a GitHub installation token", json!({
            "type": "object",
            "properties": {"installationId": {"type": "string"}},
            "required": []
        })),

        // ── File System Tools ──
        tool("Glob", "Search for files matching a glob pattern in the workspace", json!({
            "type": "object",
            "properties": {
                "pattern": {"type": "string", "description": "Glob pattern (e.g. **/*.rs, src/**/*.ts)"}
            },
            "required": ["pattern"]
        })),
        tool("Grep", "Search file contents using a regex pattern", json!({
            "type": "object",
            "properties": {
                "pattern": {"type": "string", "description": "Regex pattern to search for"},
                "glob": {"type": "string", "description": "Optional file glob filter"},
                "path": {"type": "string", "description": "Optional subdirectory to search in"}
            },
            "required": ["pattern"]
        })),
        tool("Read", "Read the contents of a file", json!({
            "type": "object",
            "properties": {
                "path": {"type": "string", "description": "File path relative to workspace root"}
            },
            "required": ["path"]
        })),
    ]
}

fn tool(name: &str, description: &str, input_schema: Value) -> Value {
    json!({
        "name": name,
        "description": description,
        "inputSchema": input_schema
    })
}
