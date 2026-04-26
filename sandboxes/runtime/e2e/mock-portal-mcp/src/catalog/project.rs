use serde_json::{json, Value};

use super::tool;

pub(super) fn definitions() -> Vec<Value> {
    vec![
        // ── Project ──
        tool(
            "createProject",
            "Create a new project",
            json!({
                "type": "object",
                "properties": {
                    "name": {"type": "string"},
                    "description": {"type": "string"},
                    "targetDate": {"type": "string"}
                },
                "required": ["name"]
            }),
        ),
        tool(
            "getProject",
            "Get project details",
            json!({
                "type": "object",
                "properties": {"projectId": {"type": "string"}},
                "required": ["projectId"]
            }),
        ),
        tool(
            "listProjects",
            "List all projects",
            json!({"type": "object", "properties": {}, "required": []}),
        ),
        tool(
            "updateProject",
            "Update a project",
            json!({
                "type": "object",
                "properties": {
                    "projectId": {"type": "string"},
                    "name": {"type": "string"},
                    "description": {"type": "string"},
                    "status": {"type": "string"}
                },
                "required": ["projectId"]
            }),
        ),
        tool(
            "deleteProject",
            "Delete a project",
            json!({
                "type": "object",
                "properties": {"projectId": {"type": "string"}},
                "required": ["projectId"]
            }),
        ),
        // ── Document ──
        tool(
            "createDocument",
            "Create a new document",
            json!({
                "type": "object",
                "properties": {
                    "title": {"type": "string"},
                    "content": {"type": "string"},
                    "projectId": {"type": "string", "description": "Optional project to attach to"}
                },
                "required": ["title", "content"]
            }),
        ),
        tool(
            "getDocument",
            "Get a document by ID",
            json!({
                "type": "object",
                "properties": {"documentId": {"type": "string"}},
                "required": ["documentId"]
            }),
        ),
        tool(
            "listDocuments",
            "List all documents",
            json!({
                "type": "object",
                "properties": {"projectId": {"type": "string", "description": "Filter by project"}},
                "required": []
            }),
        ),
        tool(
            "updateDocument",
            "Update a document",
            json!({
                "type": "object",
                "properties": {
                    "documentId": {"type": "string"},
                    "title": {"type": "string"},
                    "content": {"type": "string"}
                },
                "required": ["documentId"]
            }),
        ),
        tool(
            "searchDocuments",
            "Search documents by query",
            json!({
                "type": "object",
                "properties": {"query": {"type": "string"}},
                "required": ["query"]
            }),
        ),
        tool(
            "moveDocument",
            "Move a document to a different project",
            json!({
                "type": "object",
                "properties": {
                    "documentId": {"type": "string"},
                    "projectId": {"type": "string"}
                },
                "required": ["documentId", "projectId"]
            }),
        ),
        tool(
            "reorderDocument",
            "Reorder a document within its project",
            json!({
                "type": "object",
                "properties": {
                    "documentId": {"type": "string"},
                    "position": {"type": "integer"}
                },
                "required": ["documentId", "position"]
            }),
        ),
    ]
}
