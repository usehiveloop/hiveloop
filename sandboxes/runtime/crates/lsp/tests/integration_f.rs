#![allow(dead_code, unused_imports, unused_mut, unused_variables)]

#[macro_use]
mod common;

use common::*;
use lsp::LspManager;
use std::path::Path;

// ===========================================================================
// rust-analyzer tests
// ===========================================================================

#[tokio::test]
async fn test_missing_binary_marked_broken() {
    let tmp = tempfile::tempdir().unwrap();
    let root = tmp.path();
    let file = root.join("test.rs");
    std::fs::write(&file, "fn main() {}").unwrap();

    // Create a manager with a fake binary that doesn't exist
    let manager = make_manager(
        root,
        vec![(
            "rust",
            vec!["/nonexistent/fake-rust-analyzer".into()],
            vec!["rs"],
        )],
    );

    // First attempt should fail with BinaryNotFound
    let result = manager.open_document(&file).await;
    assert!(result.is_err());

    // Second attempt should also fail (broken server not retried)
    let result2 = manager.open_document(&file).await;
    assert!(result2.is_err());
}

#[tokio::test]
async fn test_relative_path_resolution() {
    let tmp = tempfile::tempdir().unwrap();
    let root = tmp.path();
    std::fs::create_dir_all(root.join("src")).unwrap();
    let file = root.join("src").join("main.rs");
    std::fs::write(&file, "fn main() {}").unwrap();

    let manager = LspManager::new(root.to_path_buf(), None);

    // has_server with a relative path should resolve against project_root
    let relative = Path::new("src/main.rs");
    assert!(
        manager.has_server(relative),
        "has_server should work with relative paths"
    );
}

#[tokio::test]
#[ignore]
async fn test_document_reopen_version_tracking() {
    let bin = lsp_bin("rust-analyzer");
    skip_if_not_installed!(bin);

    let project = create_rust_project();
    let manager = make_manager(
        project.path(),
        vec![("rust", vec![bin.to_string_lossy().into()], vec!["rs"])],
    );

    let file = project.path().join("src").join("lib.rs");

    // First open
    manager.open_document(&file).await.unwrap();
    wait_for_indexing(3).await;

    // Modify the file on disk
    let mut content = std::fs::read_to_string(&file).unwrap();
    content.push_str("\npub fn new_function() -> bool { true }\n");
    std::fs::write(&file, &content).unwrap();

    // Re-open should send didChange (version incremented)
    manager.open_document(&file).await.unwrap();
    wait_for_indexing(3).await;

    // Verify the server sees the updated content by checking symbols
    let symbols = manager.document_symbols(&file).await.unwrap();
    let names: Vec<&str> = symbols.iter().map(|s| s.name.as_str()).collect();
    assert!(
        names.contains(&"new_function"),
        "expected new_function in symbols after reopen"
    );

    manager.shutdown().await;
}
