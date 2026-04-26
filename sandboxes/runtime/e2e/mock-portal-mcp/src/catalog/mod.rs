mod github;
mod issue;
mod misc;
mod project;

use serde_json::{json, Value};

/// Returns the full list of MCP tool definitions (name, description, inputSchema).
pub fn tool_definitions() -> Vec<Value> {
    let mut out = Vec::new();
    out.extend(issue::definitions());
    out.extend(project::definitions());
    out.extend(misc::definitions());
    out.extend(github::definitions());
    out
}

pub(super) fn tool(name: &str, description: &str, input_schema: Value) -> Value {
    json!({
        "name": name,
        "description": description,
        "inputSchema": input_schema
    })
}
