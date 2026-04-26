#![allow(dead_code)]

//! Shared helpers for LSP integration tests. Not itself a test binary (Cargo
//! treats files under `tests/common/` as modules, not as separate test bins).

use std::collections::HashMap;
use std::path::{Path, PathBuf};
use std::time::Duration;

use lsp::config::LspServerConfig;
use lsp::LspManager;

/// Project root (two levels up from crates/lsp/).
pub fn project_root() -> PathBuf {
    let manifest = PathBuf::from(env!("CARGO_MANIFEST_DIR"));
    manifest
        .parent()
        .and_then(|p| p.parent())
        .expect("could not find project root")
        .to_path_buf()
}

/// Absolute path to a binary in `.lsp-servers/bin/`.
pub fn lsp_bin(name: &str) -> PathBuf {
    project_root().join(".lsp-servers").join("bin").join(name)
}

/// Absolute path to an npm binary in `.lsp-servers/node_modules/.bin/`.
pub fn lsp_npm_bin(name: &str) -> PathBuf {
    project_root()
        .join(".lsp-servers")
        .join("node_modules")
        .join(".bin")
        .join(name)
}

/// Return early if the binary doesn't exist (prints a skip message).
#[macro_export]
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
pub fn make_manager(
    project_root: &Path,
    servers: Vec<(&str, Vec<String>, Vec<&str>)>,
) -> LspManager {
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
pub async fn wait_for_indexing(secs: u64) {
    tokio::time::sleep(Duration::from_secs(secs)).await;
}

// ---------------------------------------------------------------------------
// Project scaffolding helpers
// ---------------------------------------------------------------------------

pub fn create_rust_project() -> tempfile::TempDir {
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

pub fn create_go_project() -> tempfile::TempDir {
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

pub fn create_ts_project() -> tempfile::TempDir {
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

pub fn create_python_project() -> tempfile::TempDir {
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

pub fn create_vue_project() -> tempfile::TempDir {
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
