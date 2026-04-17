//! Integration tests for the LSP manager.
//!
//! Tests marked `#[ignore]` require real LSP server binaries installed via:
//!   ./scripts/setup-lsp-servers.sh
//!
//! Run with: cargo test -p lsp -- --ignored

use std::collections::HashMap;
use std::path::{Path, PathBuf};
use std::time::Duration;

use lsp::config::LspServerConfig;
use lsp::error::LspError;
use lsp::LspManager;

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/// Project root (two levels up from crates/lsp/).
fn project_root() -> PathBuf {
    let manifest = PathBuf::from(env!("CARGO_MANIFEST_DIR"));
    manifest
        .parent()
        .and_then(|p| p.parent())
        .expect("could not find project root")
        .to_path_buf()
}

/// Absolute path to a binary in `.lsp-servers/bin/`.
fn lsp_bin(name: &str) -> PathBuf {
    project_root().join(".lsp-servers").join("bin").join(name)
}

/// Absolute path to an npm binary in `.lsp-servers/node_modules/.bin/`.
fn lsp_npm_bin(name: &str) -> PathBuf {
    project_root()
        .join(".lsp-servers")
        .join("node_modules")
        .join(".bin")
        .join(name)
}

/// Return early if the binary doesn't exist (prints a skip message).
macro_rules! skip_if_not_installed {
    ($bin:expr) => {
        if !$bin.exists() {
            eprintln!(
                "SKIP: {} not installed (run ./scripts/setup-lsp-servers.sh)",
                $bin.display()
            );
            return;
        }
    };
}

/// Build an `LspManager` with custom server configs pointing to local binaries.
fn make_manager(project_root: &Path, servers: Vec<(&str, Vec<String>, Vec<&str>)>) -> LspManager {
    let mut custom = HashMap::new();
    for (id, command, extensions) in servers {
        custom.insert(
            id.to_string(),
            LspServerConfig {
                command,
                extensions: extensions.iter().map(|s| s.to_string()).collect(),
                env: HashMap::new(),
                initialization_options: None,
                disabled: false,
            },
        );
    }
    LspManager::new(project_root.to_path_buf(), Some(custom))
}

/// Wait for the server to index after opening a document.
async fn wait_for_indexing(secs: u64) {
    tokio::time::sleep(Duration::from_secs(secs)).await;
}

// ---------------------------------------------------------------------------
// Project scaffolding helpers
// ---------------------------------------------------------------------------

fn create_rust_project() -> tempfile::TempDir {
    let tmp = tempfile::tempdir().unwrap();
    let root = tmp.path();

    std::fs::write(
        root.join("Cargo.toml"),
        r#"[package]
name = "test-project"
version = "0.1.0"
edition = "2021"
"#,
    )
    .unwrap();

    std::fs::create_dir_all(root.join("src")).unwrap();
    std::fs::write(
        root.join("src").join("lib.rs"),
        r#"pub struct Greeter {
    pub name: String,
}

impl Greeter {
    pub fn greet(&self) -> String {
        format!("Hello, {}!", self.name)
    }
}

pub fn add(a: i32, b: i32) -> i32 {
    a + b
}

pub fn use_add() -> i32 {
    add(1, 2)
}

// Type error: assigning &str to i32
pub fn type_error() -> i32 {
    let x: i32 = "not a number";
    x
}
"#,
    )
    .unwrap();

    tmp
}

fn create_go_project() -> tempfile::TempDir {
    let tmp = tempfile::tempdir().unwrap();
    let root = tmp.path();

    std::fs::write(root.join("go.mod"), "module testproject\n\ngo 1.21\n").unwrap();

    std::fs::write(
        root.join("main.go"),
        r#"package main

import "fmt"

func greet(name string) string {
	return fmt.Sprintf("Hello, %s!", name)
}

func main() {
	fmt.Println(greet("World"))
}
"#,
    )
    .unwrap();

    tmp
}

fn create_ts_project() -> tempfile::TempDir {
    let tmp = tempfile::tempdir().unwrap();
    let root = tmp.path();

    std::fs::write(
        root.join("package.json"),
        r#"{"name": "test-project", "version": "1.0.0"}"#,
    )
    .unwrap();

    std::fs::write(
        root.join("tsconfig.json"),
        r#"{
  "compilerOptions": {
    "target": "ES2020",
    "module": "commonjs",
    "strict": true,
    "outDir": "./dist"
  },
  "include": ["src"]
}"#,
    )
    .unwrap();

    std::fs::create_dir_all(root.join("src")).unwrap();
    std::fs::write(
        root.join("src").join("index.ts"),
        r#"export class Calculator {
    add(a: number, b: number): number {
        return a + b;
    }
}

export function multiply(a: number, b: number): number {
    return a * b;
}

const calc = new Calculator();
const result: number = calc.add(1, 2);
"#,
    )
    .unwrap();

    tmp
}

fn create_python_project() -> tempfile::TempDir {
    let tmp = tempfile::tempdir().unwrap();
    let root = tmp.path();

    std::fs::create_dir_all(root.join("src")).unwrap();
    std::fs::write(
        root.join("src").join("main.py"),
        r#"def greet(name: str) -> str:
    return f"Hello, {name}!"

def add(a: int, b: int) -> int:
    return a + b

# Type error: returning str where int expected
def type_error() -> int:
    return "not a number"
"#,
    )
    .unwrap();

    tmp
}

fn create_vue_project() -> tempfile::TempDir {
    let tmp = tempfile::tempdir().unwrap();
    let root = tmp.path();

    std::fs::write(
        root.join("package.json"),
        r#"{"name": "vue-test", "version": "1.0.0"}"#,
    )
    .unwrap();

    std::fs::create_dir_all(root.join("src")).unwrap();
    std::fs::write(
        root.join("src").join("App.vue"),
        r#"<template>
  <div>{{ message }}</div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
const message = ref('Hello Vue!')
</script>
"#,
    )
    .unwrap();

    tmp
}

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

// ===========================================================================
// gopls tests
// ===========================================================================

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

// ===========================================================================
// typescript-language-server tests
// ===========================================================================

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

// ===========================================================================
// pyright tests
// ===========================================================================

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

// ===========================================================================
// vue-language-server tests
// ===========================================================================

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

// ===========================================================================
// Cross-cutting tests (no #[ignore], no real servers needed)
// ===========================================================================

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
