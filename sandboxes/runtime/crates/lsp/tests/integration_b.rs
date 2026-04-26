#![allow(dead_code, unused_imports, unused_mut, unused_variables)]

#[macro_use]
mod common;

use common::*;

// ===========================================================================
// rust-analyzer tests
// ===========================================================================

#[tokio::test]
#[ignore]
async fn test_rust_document_symbols() {
    let bin = lsp_bin("rust-analyzer");
    skip_if_not_installed!(bin);

    let project = create_rust_project();
    let manager = make_manager(
        project.path(),
        vec![("rust", vec![bin.to_string_lossy().into()], vec!["rs"])],
    );

    let file = project.path().join("src").join("lib.rs");
    manager.open_document(&file).await.unwrap();
    wait_for_indexing(5).await;

    let symbols = manager.document_symbols(&file).await.unwrap();
    assert!(!symbols.is_empty(), "expected document symbols");

    let names: Vec<&str> = symbols.iter().map(|s| s.name.as_str()).collect();
    assert!(names.contains(&"Greeter"), "expected Greeter struct symbol");
    assert!(names.contains(&"add"), "expected add function symbol");

    manager.shutdown().await;
}

#[tokio::test]
#[ignore]
async fn test_rust_diagnostics() {
    let bin = lsp_bin("rust-analyzer");
    skip_if_not_installed!(bin);

    let project = create_rust_project();
    let manager = make_manager(
        project.path(),
        vec![("rust", vec![bin.to_string_lossy().into()], vec!["rs"])],
    );

    let file = project.path().join("src").join("lib.rs");
    manager.open_document(&file).await.unwrap();
    wait_for_indexing(3).await;

    // lsp-bridge 0.2 receives publishDiagnostics notifications but does not
    // store them on the document state, so get_diagnostics always returns [].
    // This test verifies the diagnostics() call doesn't error out.
    let diags = manager.diagnostics(&file).await.unwrap();
    eprintln!("rust diagnostics count: {}", diags.len());

    manager.shutdown().await;
}

#[tokio::test]
#[ignore]
async fn test_go_open_and_hover() {
    let bin = lsp_bin("gopls");
    skip_if_not_installed!(bin);

    let project = create_go_project();
    let manager = make_manager(
        project.path(),
        vec![("go", vec![bin.to_string_lossy().into()], vec!["go"])],
    );

    let file = project.path().join("main.go");
    manager.open_document(&file).await.unwrap();
    wait_for_indexing(5).await;

    // Hover on `greet` function definition (line 4, col 5 — on 'g' of 'greet')
    let hover = manager.hover(&file, 4, 5).await.unwrap();
    assert!(hover.is_some(), "expected hover info on greet function");

    manager.shutdown().await;
}
