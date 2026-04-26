#![allow(dead_code, unused_imports, unused_mut, unused_variables)]

#[macro_use]
mod common;

use common::*;

// ===========================================================================
// rust-analyzer tests
// ===========================================================================

#[tokio::test]
#[ignore]
async fn test_rust_open_document() {
    let bin = lsp_bin("rust-analyzer");
    skip_if_not_installed!(bin);

    let project = create_rust_project();
    let manager = make_manager(
        project.path(),
        vec![("rust", vec![bin.to_string_lossy().into()], vec!["rs"])],
    );

    let file = project.path().join("src").join("lib.rs");
    manager.open_document(&file).await.unwrap();

    // If we got here without error, the server spawned and accepted the document
    manager.shutdown().await;
}

#[tokio::test]
#[ignore]
async fn test_rust_hover() {
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

    // Hover on `String` in the struct field (line 1, col 14)
    let hover = manager.hover(&file, 1, 14).await.unwrap();
    assert!(hover.is_some(), "expected hover info on String type");

    manager.shutdown().await;
}

#[tokio::test]
#[ignore]
async fn test_rust_go_to_definition() {
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

    // Go to definition of `add` call in `use_add()` (line 15, col 4)
    let loc = manager.definition(&file, 15, 4).await.unwrap();
    assert!(loc.is_some(), "expected definition location for add()");

    let loc = loc.unwrap();
    // Should point to the `add` function declaration (line 10)
    assert_eq!(
        loc.range.start.line, 10,
        "definition should point to add fn declaration"
    );

    manager.shutdown().await;
}
