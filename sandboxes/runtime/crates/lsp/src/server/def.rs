use std::collections::HashMap;

/// A server definition describes how to launch and configure an LSP server.
#[derive(Debug, Clone)]
pub struct ServerDef {
    /// Unique identifier for this server (e.g., "typescript", "rust")
    pub id: String,
    /// Command and arguments to launch the server
    pub command: Vec<String>,
    /// File extensions this server handles
    pub extensions: Vec<String>,
    /// Files/directories that indicate the project root
    pub root_markers: Vec<String>,
    /// Environment variables to set when spawning
    pub env: HashMap<String, String>,
    /// Custom initialization options
    pub init_options: Option<serde_json::Value>,
}

/// Helper to build a ServerDef concisely.
pub(super) fn server(
    id: &str,
    command: &[&str],
    extensions: &[&str],
    root_markers: &[&str],
) -> ServerDef {
    ServerDef {
        id: id.into(),
        command: command.iter().map(|s| s.to_string()).collect(),
        extensions: extensions.iter().map(|s| s.to_string()).collect(),
        root_markers: root_markers.iter().map(|s| s.to_string()).collect(),
        env: HashMap::new(),
        init_options: None,
    }
}

/// Helper to build a ServerDef with initialization options.
pub(super) fn server_with_init(
    id: &str,
    command: &[&str],
    extensions: &[&str],
    root_markers: &[&str],
    init_options: serde_json::Value,
) -> ServerDef {
    let mut def = server(id, command, extensions, root_markers);
    def.init_options = Some(init_options);
    def
}
