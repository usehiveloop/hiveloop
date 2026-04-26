#![allow(dead_code, unused_imports, unused_mut, unused_variables)]

#[macro_use]
mod common;

use common::*;
use lsp::{LspError, LspManager};

// ===========================================================================
// rust-analyzer tests
// ===========================================================================

#[tokio::test]
#[ignore]
async fn test_python_diagnostics() {
    let bin = lsp_npm_bin("pyright-langserver");
    skip_if_not_installed!(bin);

    let project = create_python_project();
    let manager = make_manager(
        project.path(),
        vec![(
            "python",
            vec![bin.to_string_lossy().into(), "--stdio".into()],
            vec!["py", "pyi"],
        )],
    );

    let file = project.path().join("src").join("main.py");
    manager.open_document(&file).await.unwrap();
    wait_for_indexing(3).await;

    // See test_rust_diagnostics note about lsp-bridge 0.2 limitation.
    let diags = manager.diagnostics(&file).await.unwrap();
    eprintln!("python diagnostics count: {}", diags.len());

    manager.shutdown().await;
}

#[tokio::test]
#[ignore]
async fn test_vue_open_document() {
    let bin = lsp_npm_bin("vue-language-server");
    skip_if_not_installed!(bin);

    let project = create_vue_project();
    let manager = make_manager(
        project.path(),
        vec![(
            "vue",
            vec![bin.to_string_lossy().into(), "--stdio".into()],
            vec!["vue"],
        )],
    );

    let file = project.path().join("src").join("App.vue");
    manager.open_document(&file).await.unwrap();

    // If we got here without error, the server spawned and accepted the document
    manager.shutdown().await;
}

#[tokio::test]
async fn test_no_server_for_unknown_extension() {
    let tmp = tempfile::tempdir().unwrap();
    let root = tmp.path();
    let file = root.join("test.xyz");
    std::fs::write(&file, "content").unwrap();

    let manager = LspManager::new(root.to_path_buf(), None);

    let result = manager.open_document(&file).await;
    assert!(result.is_err());
    match result.unwrap_err() {
        LspError::NoServerForExtension { ext, path } => {
            assert_eq!(ext, "xyz");
            assert!(path.contains("test.xyz"));
        }
        other => panic!("expected NoServerForExtension, got: {other}"),
    }
}
