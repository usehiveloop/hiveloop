use std::collections::HashMap;
use std::net::SocketAddr;
use std::sync::atomic::{AtomicU64, AtomicUsize, Ordering};
use std::sync::Arc;

use bridge_core::artifacts::ArtifactsConfig;
use tokio::io::AsyncWriteExt;

use crate::artifacts::UploadToWorkspaceTool;
use crate::ToolExecutor;

/// Build a minimal artifacts config aimed at a test server URL.
fn cfg(upload_url: &str, max_size: u64) -> ArtifactsConfig {
    ArtifactsConfig {
        upload_url: upload_url.to_string(),
        download_url: None,
        max_size_bytes: max_size,
        accepted_file_types: vec!["txt".into(), "csv".into(), "video/*".into()],
        max_concurrent_uploads: Some(2),
        chunk_size_bytes: Some(64),
        headers: HashMap::new(),
    }
}

#[tokio::test]
async fn rejects_oversize_file() {
    let dir = tempfile::tempdir().unwrap();
    let path = dir.path().join("big.txt");
    let mut f = tokio::fs::File::create(&path).await.unwrap();
    f.write_all(&[0u8; 200]).await.unwrap();
    f.flush().await.unwrap();

    let tool = UploadToWorkspaceTool::new(
        cfg("http://127.0.0.1:1/uploads", 100), // max < file size
        "agent_test".into(),
        None,
        None,
        None,
    );
    let args = serde_json::json!({ "path": path.to_string_lossy() });
    let err = tool.execute(args).await.unwrap_err();
    assert!(err.contains("exceeds artifacts.max_size_bytes"), "{err}");
}

#[tokio::test]
async fn rejects_unsupported_mime() {
    let dir = tempfile::tempdir().unwrap();
    let path = dir.path().join("doc.pdf");
    tokio::fs::write(&path, b"hello").await.unwrap();

    let tool = UploadToWorkspaceTool::new(
        cfg("http://127.0.0.1:1/uploads", 1000),
        "agent_test".into(),
        None,
        None,
        None,
    );
    let args = serde_json::json!({ "path": path.to_string_lossy() });
    let err = tool.execute(args).await.unwrap_err();
    assert!(err.contains("File type rejected"), "{err}");
}

#[tokio::test]
async fn rejects_missing_file() {
    let tool = UploadToWorkspaceTool::new(
        cfg("http://127.0.0.1:1/uploads", 1000),
        "agent_test".into(),
        None,
        None,
        None,
    );
    let args = serde_json::json!({ "path": "/tmp/__definitely_does_not_exist__.csv" });
    let err = tool.execute(args).await.unwrap_err();
    assert!(err.contains("not readable"), "{err}");
}

#[tokio::test]
async fn rejects_empty_file() {
    let dir = tempfile::tempdir().unwrap();
    let path = dir.path().join("empty.csv");
    tokio::fs::write(&path, b"").await.unwrap();

    let tool = UploadToWorkspaceTool::new(
        cfg("http://127.0.0.1:1/uploads", 1000),
        "agent_test".into(),
        None,
        None,
        None,
    );
    let args = serde_json::json!({ "path": path.to_string_lossy() });
    let err = tool.execute(args).await.unwrap_err();
    assert!(err.contains("empty"), "{err}");
}

/// Minimal in-memory TUS server used by integration tests below. Each
/// upload is identified by a generated id; chunks are concatenated in
/// memory and verified end-to-end. Failure modes are programmable so
/// individual tests can drive specific scenarios.
mod test_server {
    use super::*;
    use axum::extract::{Path as AxPath, State};
    use axum::http::{HeaderMap, StatusCode};
    use axum::response::IntoResponse;
    use axum::routing::{head as ax_head, post as ax_post};
    use axum::Router;
    use std::sync::Mutex;

    #[derive(Default)]
    pub(super) struct Upload {
        pub bytes: Vec<u8>,
    }

    pub(super) struct ServerState {
        pub uploads: Mutex<HashMap<String, Upload>>,
        pub fail_first_n_patches: AtomicUsize,
        pub force_offset_mismatch_once: AtomicUsize,
        pub next_id: AtomicU64,
    }

    impl ServerState {
        pub fn new() -> Arc<Self> {
            Arc::new(Self {
                uploads: Mutex::new(HashMap::new()),
                fail_first_n_patches: AtomicUsize::new(0),
                force_offset_mismatch_once: AtomicUsize::new(0),
                next_id: AtomicU64::new(0),
            })
        }
    }

    async fn create(
        State(state): State<Arc<ServerState>>,
        _headers: HeaderMap,
    ) -> impl IntoResponse {
        let id = state.next_id.fetch_add(1, Ordering::SeqCst);
        let id = format!("u{id}");
        state.uploads.lock().unwrap().insert(id.clone(), Upload::default());
        (
            StatusCode::CREATED,
            [
                ("Tus-Resumable", "1.0.0"),
                ("Location", &format!("/uploads/{id}")),
            ],
        )
            .into_response()
    }

    async fn head(
        State(state): State<Arc<ServerState>>,
        AxPath(id): AxPath<String>,
    ) -> impl IntoResponse {
        let uploads = state.uploads.lock().unwrap();
        match uploads.get(&id) {
            Some(u) => (
                StatusCode::NO_CONTENT,
                [
                    ("Tus-Resumable", "1.0.0".to_string()),
                    ("Upload-Offset", u.bytes.len().to_string()),
                ],
            )
                .into_response(),
            None => StatusCode::NOT_FOUND.into_response(),
        }
    }

    async fn patch(
        State(state): State<Arc<ServerState>>,
        AxPath(id): AxPath<String>,
        headers: HeaderMap,
        body: axum::body::Bytes,
    ) -> impl IntoResponse {
        // Programmable transient failures.
        if state.fail_first_n_patches.load(Ordering::SeqCst) > 0 {
            state.fail_first_n_patches.fetch_sub(1, Ordering::SeqCst);
            return (StatusCode::INTERNAL_SERVER_ERROR, "boom").into_response();
        }

        let client_offset: u64 = headers
            .get("Upload-Offset")
            .and_then(|v| v.to_str().ok())
            .and_then(|s| s.parse().ok())
            .unwrap_or(0);

        let mut uploads = state.uploads.lock().unwrap();
        let upload = match uploads.get_mut(&id) {
            Some(u) => u,
            None => return StatusCode::NOT_FOUND.into_response(),
        };
        let server_offset = upload.bytes.len() as u64;

        // Programmable forced 409 — once.
        if state.force_offset_mismatch_once.load(Ordering::SeqCst) > 0 {
            state.force_offset_mismatch_once.fetch_sub(1, Ordering::SeqCst);
            return (
                StatusCode::CONFLICT,
                [
                    ("Tus-Resumable", "1.0.0".to_string()),
                    ("Upload-Offset", server_offset.to_string()),
                ],
            )
                .into_response();
        }

        if client_offset != server_offset {
            return (
                StatusCode::CONFLICT,
                [
                    ("Tus-Resumable", "1.0.0".to_string()),
                    ("Upload-Offset", server_offset.to_string()),
                ],
            )
                .into_response();
        }

        upload.bytes.extend_from_slice(&body);
        let new_offset = upload.bytes.len() as u64;
        (
            StatusCode::NO_CONTENT,
            [
                ("Tus-Resumable", "1.0.0".to_string()),
                ("Upload-Offset", new_offset.to_string()),
            ],
        )
            .into_response()
    }

    pub(super) async fn spawn() -> (SocketAddr, Arc<ServerState>) {
        let state = ServerState::new();
        let app = Router::new()
            .route("/uploads", ax_post(create))
            .route("/uploads/{id}", ax_head(head).patch(patch))
            .with_state(state.clone());
        // bind to ephemeral port
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
        let addr = listener.local_addr().unwrap();
        tokio::spawn(async move {
            let _ = axum::serve(listener, app.into_make_service()).await;
        });
        (addr, state)
    }
}

#[tokio::test]
async fn streams_full_upload_in_chunks() {
    let (addr, state) = test_server::spawn().await;
    let dir = tempfile::tempdir().unwrap();
    let path = dir.path().join("data.csv");
    let payload: Vec<u8> = (0..200u32).map(|i| (i % 256) as u8).collect();
    tokio::fs::write(&path, &payload).await.unwrap();

    let tool = UploadToWorkspaceTool::new(
        cfg(&format!("http://{addr}/uploads"), 1024),
        "agent_test".into(),
        None,
        None,
        None,
    );
    let args = serde_json::json!({ "path": path.to_string_lossy() });
    let result = tool.execute(args).await.expect("upload should succeed");
    let parsed: serde_json::Value = serde_json::from_str(&result).unwrap();
    assert_eq!(parsed["size"], serde_json::json!(payload.len()));

    let uploads = state.uploads.lock().unwrap();
    let stored = uploads.values().next().expect("one upload recorded");
    assert_eq!(stored.bytes, payload);
}

#[tokio::test]
async fn retries_transient_500s() {
    let (addr, state) = test_server::spawn().await;
    state.fail_first_n_patches.store(2, Ordering::SeqCst);

    let dir = tempfile::tempdir().unwrap();
    let path = dir.path().join("data.csv");
    let payload: Vec<u8> = (0..150).map(|i| i as u8).collect();
    tokio::fs::write(&path, &payload).await.unwrap();

    let tool = UploadToWorkspaceTool::new(
        cfg(&format!("http://{addr}/uploads"), 1024),
        "agent_test".into(),
        None,
        None,
        None,
    );
    let args = serde_json::json!({ "path": path.to_string_lossy() });
    let result = tool.execute(args).await.expect("upload should succeed after retry");
    assert!(!result.is_empty());

    let uploads = state.uploads.lock().unwrap();
    let stored = uploads.values().next().unwrap();
    assert_eq!(stored.bytes, payload);
}

#[tokio::test]
async fn realigns_after_offset_mismatch() {
    let (addr, state) = test_server::spawn().await;
    state.force_offset_mismatch_once.store(1, Ordering::SeqCst);

    let dir = tempfile::tempdir().unwrap();
    let path = dir.path().join("data.csv");
    let payload: Vec<u8> = (0..150).map(|i| i as u8).collect();
    tokio::fs::write(&path, &payload).await.unwrap();

    let tool = UploadToWorkspaceTool::new(
        cfg(&format!("http://{addr}/uploads"), 1024),
        "agent_test".into(),
        None,
        None,
        None,
    );
    let args = serde_json::json!({ "path": path.to_string_lossy() });
    tool.execute(args).await.expect("upload should succeed after realign");

    let uploads = state.uploads.lock().unwrap();
    let stored = uploads.values().next().unwrap();
    assert_eq!(stored.bytes, payload);
}
