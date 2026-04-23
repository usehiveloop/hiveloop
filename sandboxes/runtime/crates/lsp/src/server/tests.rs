use super::*;

#[test]
fn test_builtin_servers_count() {
    let servers = builtin_servers();
    // Should have 30+ servers
    assert!(
        servers.len() >= 30,
        "expected at least 30 servers, got {}",
        servers.len()
    );
}

#[test]
fn test_builtin_server_ids() {
    let servers = builtin_servers();
    let ids: Vec<&str> = servers.iter().map(|s| s.id.as_str()).collect();
    // Original 4
    assert!(ids.contains(&"typescript"));
    assert!(ids.contains(&"rust"));
    assert!(ids.contains(&"go"));
    assert!(ids.contains(&"python"));
    // New servers
    assert!(ids.contains(&"deno"));
    assert!(ids.contains(&"vue"));
    assert!(ids.contains(&"svelte"));
    assert!(ids.contains(&"clangd"));
    assert!(ids.contains(&"bash"));
    assert!(ids.contains(&"haskell"));
    assert!(ids.contains(&"nixd"));
    assert!(ids.contains(&"zig"));
}

#[test]
fn test_rust_server_extensions() {
    let servers = builtin_servers();
    let rust = servers.iter().find(|s| s.id == "rust").unwrap();
    assert_eq!(rust.extensions, vec!["rs"]);
    assert!(rust.root_markers.contains(&"Cargo.toml".to_string()));
}

#[test]
fn test_all_servers_have_commands() {
    let servers = builtin_servers();
    for server in &servers {
        assert!(
            !server.command.is_empty(),
            "server '{}' has empty command",
            server.id
        );
    }
}

#[test]
fn test_all_servers_have_extensions() {
    let servers = builtin_servers();
    for server in &servers {
        assert!(
            !server.extensions.is_empty(),
            "server '{}' has no extensions",
            server.id
        );
    }
}

#[test]
fn test_find_root_with_marker() {
    let tmp = tempfile::tempdir().unwrap();
    let project = tmp.path().join("project");
    std::fs::create_dir_all(&project).unwrap();
    std::fs::write(project.join("Cargo.toml"), "").unwrap();

    let src = project.join("src");
    std::fs::create_dir_all(&src).unwrap();
    let file = src.join("main.rs");
    std::fs::write(&file, "").unwrap();

    let root = find_root(&file, &["Cargo.toml".to_string()]);
    assert_eq!(root, Some(project));
}

#[test]
fn test_find_root_no_marker() {
    let tmp = tempfile::tempdir().unwrap();
    let file = tmp.path().join("orphan.rs");
    std::fs::write(&file, "").unwrap();

    let root = find_root(&file, &["Cargo.toml".to_string()]);
    // May find a Cargo.toml higher up or return None
    // The important thing is it doesn't panic
    let _ = root;
}
