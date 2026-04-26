#![allow(dead_code, unused_imports, unused_mut, unused_variables)]

#[macro_use]
mod common;

use common::*;

// ===========================================================================
// rust-analyzer tests
// ===========================================================================

#[tokio::test]
#[ignore]
async fn test_ts_document_symbols() {
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

    let symbols = manager.document_symbols(&file).await.unwrap();
    assert!(!symbols.is_empty(), "expected document symbols");

    let names: Vec<&str> = symbols.iter().map(|s| s.name.as_str()).collect();
    assert!(
        names.contains(&"Calculator"),
        "expected Calculator class symbol"
    );
    assert!(
        names.contains(&"multiply"),
        "expected multiply function symbol"
    );

    manager.shutdown().await;
}

#[tokio::test]
#[ignore]
async fn test_ts_go_to_definition() {
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

    // Go to definition of `multiply` call — hover on multiply at line 6, col 20
    // (using the function declaration itself to test definition works)
    let loc = manager.definition(&file, 6, 20).await.unwrap();
    assert!(loc.is_some(), "expected definition location for multiply");

    manager.shutdown().await;
}

#[tokio::test]
#[ignore]
async fn test_python_open_and_hover() {
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
    wait_for_indexing(5).await;

    // Hover on `greet` function (line 0, col 4)
    let hover = manager.hover(&file, 0, 4).await.unwrap();
    assert!(hover.is_some(), "expected hover info on greet function");

    manager.shutdown().await;
}
