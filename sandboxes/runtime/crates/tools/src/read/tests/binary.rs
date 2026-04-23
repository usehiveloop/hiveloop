//! Binary detection, PDF/image handling, SVG, and hard-gate tests.

use crate::read::*;
use crate::ToolExecutor;

#[tokio::test]
async fn test_read_binary_file_error() {
    let dir = tempfile::tempdir().expect("create temp dir");
    let file_path = dir.path().join("data.bin");
    std::fs::write(&file_path, [0x00, 0x01, 0x02]).expect("write");

    let tool = ReadTool::new();
    let args = serde_json::json!({
        "file_path": file_path.to_str().unwrap()
    });

    let err = tool.execute(args).await.unwrap_err();
    assert!(err.contains("Binary file detected"));
    assert!(err.contains("bytes"), "should mention file size");
    assert!(err.contains("bash tool"), "should suggest bash tool");
}

#[tokio::test]
async fn test_read_binary_extension_detected() {
    let dir = tempfile::tempdir().expect("create temp dir");
    let file_path = dir.path().join("program.exe");
    // Write normal text — extension alone should trigger binary detection
    std::fs::write(&file_path, "this is text content").expect("write");

    let tool = ReadTool::new();
    let args = serde_json::json!({
        "file_path": file_path.to_str().unwrap()
    });

    let err = tool.execute(args).await.unwrap_err();
    assert!(err.contains("Binary file detected"));
}

#[tokio::test]
async fn test_read_binary_threshold_detection() {
    let dir = tempfile::tempdir().expect("create temp dir");
    let file_path = dir.path().join("data.dat2");
    // Create file with >30% non-printable bytes (but no null bytes)
    let mut bytes = Vec::new();
    for i in 0u8..100 {
        if i < 35 {
            bytes.push(1); // non-printable (SOH)
        } else {
            bytes.push(b'A'); // printable
        }
    }
    std::fs::write(&file_path, &bytes).expect("write");

    let tool = ReadTool::new();
    let args = serde_json::json!({
        "file_path": file_path.to_str().unwrap()
    });

    let err = tool.execute(args).await.unwrap_err();
    assert!(err.contains("Binary file detected"));
}

#[tokio::test]
async fn test_read_pdf_returns_base64() {
    let dir = tempfile::tempdir().expect("create temp dir");
    let file_path = dir.path().join("test.pdf");
    std::fs::write(&file_path, b"%PDF-1.4 fake pdf content").expect("write");

    let tool = ReadTool::new();
    let args = serde_json::json!({
        "file_path": file_path.to_str().unwrap()
    });

    let result = tool.execute(args).await.expect("should succeed for PDF");
    let parsed: serde_json::Value = serde_json::from_str(&result).expect("parse");
    assert_eq!(parsed["type"], "file");
    assert_eq!(parsed["format"], "pdf");
    assert!(parsed["data"].is_string());
    assert!(parsed["size_bytes"].as_u64().unwrap() > 0);
}

#[tokio::test]
async fn test_read_hard_gate_large_file_without_limit() {
    let dir = tempfile::tempdir().expect("create temp dir");
    let file_path = dir.path().join("large.txt");
    let content: String = (0..DEFAULT_LINE_LIMIT + 500)
        .map(|i| format!("line {i}\n"))
        .collect();
    std::fs::write(&file_path, &content).expect("write");

    let tool = ReadTool::new();
    let args = serde_json::json!({
        "file_path": file_path.to_str().unwrap()
    });

    let err = tool.execute(args).await.unwrap_err();
    assert!(
        err.contains("has ") && err.contains("lines"),
        "error should name the actual line count: {err}"
    );
    assert!(
        err.contains("Ranged read"),
        "error should offer ranged read option: {err}"
    );
    assert!(
        err.contains("RipGrep"),
        "error should offer RipGrep option: {err}"
    );
    assert!(
        err.contains("self_agent"),
        "error should offer self_agent option: {err}"
    );
}

#[tokio::test]
async fn test_read_hard_gate_passes_when_limit_explicit() {
    let dir = tempfile::tempdir().expect("create temp dir");
    let file_path = dir.path().join("large.txt");
    let content: String = (0..DEFAULT_LINE_LIMIT + 500)
        .map(|i| format!("line {i}\n"))
        .collect();
    std::fs::write(&file_path, &content).expect("write");

    let tool = ReadTool::new();
    let args = serde_json::json!({
        "file_path": file_path.to_str().unwrap(),
        "limit": 100,
    });

    let result = tool.execute(args).await.expect("execute should succeed");
    let parsed: ReadResult = serde_json::from_str(&result).expect("parse");
    assert_eq!(parsed.lines_read, 100);
    assert!(parsed.truncated, "should report truncation vs full file");
}

#[tokio::test]
async fn test_read_hard_max_bytes_rejects_huge_file() {
    // Build a file that exceeds HARD_MAX_BYTES (10MB) without allocating
    // 10MB of distinct content: write a repeated pattern.
    let dir = tempfile::tempdir().expect("create temp dir");
    let file_path = dir.path().join("huge.txt");
    let chunk = "x".repeat(1024); // 1KB
    let mut f = std::fs::File::create(&file_path).expect("create");
    // 11MB = 11_264 chunks — above HARD_MAX_BYTES.
    for _ in 0..11_264 {
        std::io::Write::write_all(&mut f, chunk.as_bytes()).expect("write");
        std::io::Write::write_all(&mut f, b"\n").expect("write");
    }
    drop(f);

    let tool = ReadTool::new();
    let args = serde_json::json!({
        "file_path": file_path.to_str().unwrap(),
        "limit": 10,  // even with limit, huge files are refused.
    });

    let err = tool.execute(args).await.unwrap_err();
    assert!(
        err.contains("too large"),
        "error should flag the size: {err}"
    );
    assert!(
        err.contains("RipGrep"),
        "error should steer toward RipGrep: {err}"
    );
    assert!(
        err.contains("self_agent"),
        "error should steer toward self_agent: {err}"
    );
}

#[tokio::test]
async fn test_read_directory_hard_gate_without_limit() {
    let dir = tempfile::tempdir().expect("create temp dir");
    let dir_path = dir.path();
    // Create >DEFAULT_LINE_LIMIT entries.
    for i in 0..(DEFAULT_LINE_LIMIT + 10) {
        std::fs::write(dir_path.join(format!("file_{i:05}.txt")), "x").expect("write");
    }

    let tool = ReadTool::new();
    let args = serde_json::json!({
        "file_path": dir_path.to_str().unwrap()
    });

    let err = tool.execute(args).await.unwrap_err();
    assert!(
        err.contains("entries"),
        "error should mention entries: {err}"
    );
    assert!(
        err.contains("Glob"),
        "error should steer toward Glob: {err}"
    );
    assert!(
        err.contains("self_agent"),
        "error should steer toward self_agent: {err}"
    );
}

#[tokio::test]
async fn test_read_image_file_returns_base64() {
    let dir = tempfile::tempdir().expect("create temp dir");
    let file_path = dir.path().join("test.png");
    // Write some binary data with a null byte to trigger binary detection
    std::fs::write(&file_path, [0x89, 0x50, 0x4E, 0x47, 0x00, 0x0D, 0x0A]).expect("write");

    let tool = ReadTool::new();
    let args = serde_json::json!({
        "file_path": file_path.to_str().unwrap()
    });

    let result = tool.execute(args).await.expect("should succeed for image");
    let parsed: serde_json::Value = serde_json::from_str(&result).expect("parse");
    assert_eq!(parsed["type"], "image");
    assert_eq!(parsed["format"], "png");
    assert!(parsed["data"].is_string());
    assert!(parsed["size_bytes"].as_u64().unwrap() > 0);
}

#[tokio::test]
async fn test_read_svg_as_text() {
    let dir = tempfile::tempdir().expect("create temp dir");
    let file_path = dir.path().join("icon.svg");
    std::fs::write(
        &file_path,
        r#"<svg xmlns="http://www.w3.org/2000/svg"><circle r="50"/></svg>"#,
    )
    .expect("write");

    let tool = ReadTool::new();
    let args = serde_json::json!({
        "file_path": file_path.to_str().unwrap()
    });

    let result = tool.execute(args).await.expect("should succeed for SVG");
    let parsed: ReadResult = serde_json::from_str(&result).expect("parse");
    assert!(parsed.content.contains("<svg"));
}
