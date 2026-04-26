use serde_json::{json, Value};

use super::tool;

pub(super) fn definitions() -> Vec<Value> {
    vec![
        // ── GitHub PR ──
        tool(
            "listIssuePullRequests",
            "List pull requests linked to an issue",
            json!({
                "type": "object",
                "properties": {"issueId": {"type": "string"}},
                "required": ["issueId"]
            }),
        ),
        tool(
            "getPullRequest",
            "Get details of a pull request",
            json!({
                "type": "object",
                "properties": {
                    "prNumber": {"type": "integer", "description": "PR number"},
                    "repoFullName": {"type": "string", "description": "owner/repo"}
                },
                "required": ["prNumber"]
            }),
        ),
        tool(
            "fetchPullRequestComments",
            "Fetch comments on a pull request",
            json!({
                "type": "object",
                "properties": {"prNumber": {"type": "integer"}},
                "required": ["prNumber"]
            }),
        ),
        tool(
            "fetchPullRequestReviews",
            "Fetch reviews on a pull request",
            json!({
                "type": "object",
                "properties": {"prNumber": {"type": "integer"}},
                "required": ["prNumber"]
            }),
        ),
        tool(
            "fetchPullRequestChecks",
            "Fetch CI checks on a pull request",
            json!({
                "type": "object",
                "properties": {"prNumber": {"type": "integer"}},
                "required": ["prNumber"]
            }),
        ),
        tool(
            "addPullRequestComment",
            "Add a comment on a pull request",
            json!({
                "type": "object",
                "properties": {
                    "prNumber": {"type": "integer"},
                    "body": {"type": "string"},
                    "path": {"type": "string", "description": "File path for inline comment"},
                    "line": {"type": "integer", "description": "Line number for inline comment"}
                },
                "required": ["prNumber", "body"]
            }),
        ),
        tool(
            "getGithubInstallationToken",
            "Get a GitHub installation token",
            json!({
                "type": "object",
                "properties": {"installationId": {"type": "string"}},
                "required": []
            }),
        ),
        // ── File System Tools ──
        tool(
            "Glob",
            "Search for files matching a glob pattern in the workspace",
            json!({
                "type": "object",
                "properties": {
                    "pattern": {"type": "string", "description": "Glob pattern (e.g. **/*.rs, src/**/*.ts)"}
                },
                "required": ["pattern"]
            }),
        ),
        tool(
            "Grep",
            "Search file contents using a regex pattern",
            json!({
                "type": "object",
                "properties": {
                    "pattern": {"type": "string", "description": "Regex pattern to search for"},
                    "glob": {"type": "string", "description": "Optional file glob filter"},
                    "path": {"type": "string", "description": "Optional subdirectory to search in"}
                },
                "required": ["pattern"]
            }),
        ),
        tool(
            "Read",
            "Read the contents of a file",
            json!({
                "type": "object",
                "properties": {
                    "path": {"type": "string", "description": "File path relative to workspace root"}
                },
                "required": ["path"]
            }),
        ),
    ]
}
