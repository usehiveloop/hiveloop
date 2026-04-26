#![allow(dead_code, unused_imports, unused_mut, unused_variables)]

#[macro_use]
mod common;

use common::*;

// ===========================================================================
// rust-analyzer tests
// ===========================================================================

#[tokio::test]
#[ignore]
async fn test_go_document_symbols() {
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

    let symbols = manager.document_symbols(&file).await.unwrap();
    assert!(!symbols.is_empty(), "expected document symbols");

    let names: Vec<&str> = symbols.iter().map(|s| s.name.as_str()).collect();
    assert!(names.contains(&"greet"), "expected greet function symbol");
    assert!(names.contains(&"main"), "expected main function symbol");

    manager.shutdown().await;
}

#[tokio::test]
#[ignore]
async fn test_go_diagnostics() {
    let bin = lsp_bin("gopls");
    skip_if_not_installed!(bin);

    // Create a Go project with an unused import to trigger a diagnostic
    let tmp = tempfile::tempdir().unwrap();
    let root = tmp.path();
    std::fs::write(root.join("go.mod"), "module testproject\n\ngo 1.21\n").unwrap();
    std::fs::write(
        root.join("main.go"),
        r#"package main

import "fmt"
import "os"

func main() {
	fmt.Println("hello")
}
"#,
    )
    .unwrap();

    let manager = make_manager(
        root,
        vec![("go", vec![bin.to_string_lossy().into()], vec!["go"])],
    );

    let file = root.join("main.go");
    manager.open_document(&file).await.unwrap();
    wait_for_indexing(3).await;

    // See test_rust_diagnostics note about lsp-bridge 0.2 limitation.
    let diags = manager.diagnostics(&file).await.unwrap();
    eprintln!("go diagnostics count: {}", diags.len());

    manager.shutdown().await;
}

#[tokio::test]
#[ignore]
async fn test_ts_open_and_hover() {
    let bin = lsp_npm_bin("typescript-language-server");
    skip_if_not_installed!(bin);

    let project = create_ts_project();
    let manager = make_manager(
        project.path(),
        vec![(
            "typescript",
            vec![bin.to_string_lossy().into(), "--stdio".into()],
            vec!["ts", "tsx", "js", "jsx"],
        )],
    );

    let file = project.path().join("src").join("index.ts");
    manager.open_document(&file).await.unwrap();
    wait_for_indexing(5).await;

    // Hover on `Calculator` class name (line 0, col 13)
    let hover = manager.hover(&file, 0, 13).await.unwrap();
    assert!(hover.is_some(), "expected hover info on Calculator class");

    manager.shutdown().await;
}
