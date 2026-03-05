use crate::file_tools;
use crate::mock_data;
use crate::protocol::ToolResult;
use serde_json::{json, Value};
use std::path::Path;

/// Dispatch a tool call to the appropriate handler.
pub fn handle_tool_call(name: &str, args: &Value, workspace_dir: &Path) -> ToolResult {
    match name {
        // ── Team ──
        "listTeams" => ToolResult::text(json!([mock_data::team_eng()]).to_string()),
        "getTeam" => ToolResult::text(mock_data::team_eng().to_string()),
        "listTeamMembers" => ToolResult::text(mock_data::team_members().to_string()),

        // ── Issue ──
        "createIssue" => {
            let title = args
                .get("title")
                .and_then(|v| v.as_str())
                .unwrap_or("New Issue");
            ToolResult::text(json!({
                "id": format!("issue_{}", uuid::Uuid::new_v4().to_string().split('-').next().unwrap()),
                "identifier": "ENG-45",
                "title": title,
                "status": {"id": "status_backlog", "name": "Backlog"},
                "createdAt": chrono::Utc::now().to_rfc3339()
            }).to_string())
        }
        "listTeamIssues" => ToolResult::text(json!(mock_data::issues()).to_string()),
        "getIssue" => {
            let issue_id = args.get("issueId").and_then(|v| v.as_str()).unwrap_or("");
            match mock_data::get_issue(issue_id) {
                Some(issue) => ToolResult::text(issue.to_string()),
                None => ToolResult::text(
                    // Default to first issue if no match
                    mock_data::issues()
                        .first()
                        .cloned()
                        .unwrap_or(json!(null))
                        .to_string(),
                ),
            }
        }
        "updateIssue" => {
            ToolResult::text(json!({"success": true, "message": "Issue updated"}).to_string())
        }
        "updateIssueStatus" => ToolResult::text(
            json!({"success": true, "message": "Issue status updated"}).to_string(),
        ),
        "assignIssue" => {
            ToolResult::text(json!({"success": true, "message": "Issue assigned"}).to_string())
        }
        "deleteIssue" => {
            ToolResult::text(json!({"success": true, "message": "Issue deleted"}).to_string())
        }
        "updateIssuePriority" => {
            ToolResult::text(json!({"success": true, "message": "Priority updated"}).to_string())
        }
        "updateIssueAssignee" => {
            ToolResult::text(json!({"success": true, "message": "Assignee updated"}).to_string())
        }
        "updateIssueLabels" => {
            ToolResult::text(json!({"success": true, "message": "Labels updated"}).to_string())
        }
        "updateIssueProject" => {
            ToolResult::text(json!({"success": true, "message": "Project updated"}).to_string())
        }

        // ── Dependencies ──
        "addIssueDependency" => ToolResult::text(
            json!({"success": true, "id": "dep_new", "message": "Dependency added"}).to_string(),
        ),
        "removeIssueDependency" => {
            ToolResult::text(json!({"success": true, "message": "Dependency removed"}).to_string())
        }
        "listIssueDependencies" => ToolResult::text(mock_data::issue_dependencies().to_string()),

        // ── Status ──
        "listStatuses" => ToolResult::text(mock_data::statuses().to_string()),

        // ── Project ──
        "createProject" => {
            let name = args
                .get("name")
                .and_then(|v| v.as_str())
                .unwrap_or("New Project");
            ToolResult::text(json!({
                "id": format!("project_{}", uuid::Uuid::new_v4().to_string().split('-').next().unwrap()),
                "name": name,
                "status": "planned",
                "createdAt": chrono::Utc::now().to_rfc3339()
            }).to_string())
        }
        "getProject" => ToolResult::text(mock_data::project_q1().to_string()),
        "listProjects" => ToolResult::text(mock_data::projects().to_string()),
        "updateProject" => {
            ToolResult::text(json!({"success": true, "message": "Project updated"}).to_string())
        }
        "deleteProject" => {
            ToolResult::text(json!({"success": true, "message": "Project deleted"}).to_string())
        }

        // ── Document ──
        "createDocument" => {
            let title = args
                .get("title")
                .and_then(|v| v.as_str())
                .unwrap_or("New Document");
            ToolResult::text(json!({
                "id": format!("doc_{}", uuid::Uuid::new_v4().to_string().split('-').next().unwrap()),
                "title": title,
                "createdAt": chrono::Utc::now().to_rfc3339(),
                "message": "Document created successfully"
            }).to_string())
        }
        "getDocument" => {
            let doc_id = args
                .get("documentId")
                .and_then(|v| v.as_str())
                .unwrap_or("");
            let docs = mock_data::documents();
            let doc = docs.iter().find(|d| {
                d.get("id")
                    .and_then(|v| v.as_str())
                    .is_some_and(|id| id == doc_id)
            });
            ToolResult::text(doc.cloned().unwrap_or(docs[0].clone()).to_string())
        }
        "listDocuments" => ToolResult::text(json!(mock_data::documents()).to_string()),
        "updateDocument" => {
            ToolResult::text(json!({"success": true, "message": "Document updated"}).to_string())
        }
        "searchDocuments" => ToolResult::text(json!(mock_data::documents()).to_string()),
        "moveDocument" => {
            ToolResult::text(json!({"success": true, "message": "Document moved"}).to_string())
        }
        "reorderDocument" => {
            ToolResult::text(json!({"success": true, "message": "Document reordered"}).to_string())
        }

        // ── Label ──
        "listLabels" => ToolResult::text(mock_data::labels().to_string()),
        "listWorkspaceLabels" => ToolResult::text(mock_data::labels().to_string()),
        "createLabel" => {
            let name = args
                .get("name")
                .and_then(|v| v.as_str())
                .unwrap_or("new-label");
            ToolResult::text(
                json!({
                    "id": format!("label_{}", name.replace(' ', "_")),
                    "name": name,
                    "color": "#888888"
                })
                .to_string(),
            )
        }

        // ── Comment ──
        "createComment" => {
            let body = args.get("body").and_then(|v| v.as_str()).unwrap_or("");
            ToolResult::text(json!({
                "id": format!("comment_{}", uuid::Uuid::new_v4().to_string().split('-').next().unwrap()),
                "body": body,
                "createdAt": chrono::Utc::now().to_rfc3339(),
                "message": "Comment created successfully"
            }).to_string())
        }
        "listComments" => ToolResult::text(mock_data::comments().to_string()),
        "updateComment" => {
            ToolResult::text(json!({"success": true, "message": "Comment updated"}).to_string())
        }

        // ── View ──
        "createView" => {
            let name = args
                .get("name")
                .and_then(|v| v.as_str())
                .unwrap_or("New View");
            ToolResult::text(json!({
                "id": format!("view_{}", uuid::Uuid::new_v4().to_string().split('-').next().unwrap()),
                "name": name,
                "createdAt": chrono::Utc::now().to_rfc3339()
            }).to_string())
        }

        // ── Member ──
        "listWorkspaceMembers" => ToolResult::text(mock_data::workspace_members().to_string()),

        // ── Portal Agent ──
        "pingHuman" => {
            let message = args.get("message").and_then(|v| v.as_str()).unwrap_or("");
            ToolResult::text(
                json!({
                    "success": true,
                    "message": format!("Human pinged with message: {}", message),
                    "notifiedAt": chrono::Utc::now().to_rfc3339()
                })
                .to_string(),
            )
        }
        "pingMeBack" => {
            let delay = args
                .get("delayMinutes")
                .and_then(|v| v.as_i64())
                .unwrap_or(5);
            ToolResult::text(
                json!({
                    "success": true,
                    "message": format!("Ping scheduled in {} minutes", delay),
                    "scheduledAt": chrono::Utc::now().to_rfc3339()
                })
                .to_string(),
            )
        }
        "submitApprovalRequest" => {
            let action = args.get("action").and_then(|v| v.as_str()).unwrap_or("");
            ToolResult::text(json!({
                "success": true,
                "approvalId": format!("approval_{}", uuid::Uuid::new_v4().to_string().split('-').next().unwrap()),
                "action": action,
                "status": "pending",
                "message": "Approval request submitted"
            }).to_string())
        }

        // ── GitHub PR ──
        "listIssuePullRequests" => ToolResult::text(mock_data::issue_pull_requests().to_string()),
        "getPullRequest" => ToolResult::text(mock_data::pull_request_156().to_string()),
        "fetchPullRequestComments" => ToolResult::text(mock_data::pr_comments().to_string()),
        "fetchPullRequestReviews" => ToolResult::text(mock_data::pr_reviews().to_string()),
        "fetchPullRequestChecks" => ToolResult::text(mock_data::pr_checks().to_string()),
        "addPullRequestComment" => {
            let body = args.get("body").and_then(|v| v.as_str()).unwrap_or("");
            ToolResult::text(json!({
                "id": format!("pr_comment_{}", uuid::Uuid::new_v4().to_string().split('-').next().unwrap()),
                "body": body,
                "createdAt": chrono::Utc::now().to_rfc3339(),
                "message": "PR comment added"
            }).to_string())
        }
        "getGithubInstallationToken" => ToolResult::text(
            json!({
                "token": "ghs_mock_installation_token_for_testing",
                "expiresAt": "2025-12-31T23:59:59Z"
            })
            .to_string(),
        ),

        // ── File System Tools ──
        "Glob" => {
            let pattern = args
                .get("pattern")
                .and_then(|v| v.as_str())
                .unwrap_or("**/*");
            file_tools::glob(workspace_dir, pattern)
        }
        "Grep" => {
            let pattern = args.get("pattern").and_then(|v| v.as_str()).unwrap_or("");
            let file_glob = args.get("glob").and_then(|v| v.as_str());
            let path = args.get("path").and_then(|v| v.as_str());
            file_tools::grep(workspace_dir, pattern, file_glob, path)
        }
        "Read" => {
            let path = args.get("path").and_then(|v| v.as_str()).unwrap_or("");
            file_tools::read(workspace_dir, path)
        }

        _ => ToolResult::error(format!("unknown tool: {name}")),
    }
}
