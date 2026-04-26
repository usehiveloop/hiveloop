//! Minimal tus.io v1.0.0 client. Implements only the three verbs the
//! `upload_to_workspace` tool needs:
//!
//! - `POST` (Creation extension) — open a new upload, returns `Location`.
//! - `HEAD` — query the server for the authoritative `Upload-Offset`.
//! - `PATCH` — append bytes from a given offset.
//!
//! Why in-house: the two existing tus client crates on crates.io
//! (`tus_async_client`, `rust-tus-client`) are unmaintained and don't
//! support `reqwest 0.13` / async streaming. The protocol is small enough
//! to roll directly.

use std::collections::HashMap;
use std::time::Duration;

use base64::Engine;
use bytes::Bytes;
use reqwest::header::{HeaderMap, HeaderName, HeaderValue};
use reqwest::{Body, Client, StatusCode};
use sha2::{Digest, Sha256};

const TUS_RESUMABLE: &str = "1.0.0";
const HDR_TUS_RESUMABLE: &str = "Tus-Resumable";
const HDR_UPLOAD_LENGTH: &str = "Upload-Length";
const HDR_UPLOAD_OFFSET: &str = "Upload-Offset";
const HDR_UPLOAD_METADATA: &str = "Upload-Metadata";
const HDR_UPLOAD_CHECKSUM: &str = "Upload-Checksum";
const CT_OFFSET_OCTET_STREAM: &str = "application/offset+octet-stream";

/// Errors returned by the TUS client. Variants are categorised so the caller
/// can decide retry semantics: `Transient` is safe to retry, `Mismatch`
/// means the caller needs to re-`HEAD` and realign, `Permanent` means stop.
#[derive(Debug, thiserror::Error)]
pub enum TusError {
    #[error("transient TUS error: {0}")]
    Transient(String),
    #[error("offset mismatch (server expected {server}, client sent {client})")]
    Mismatch { server: u64, client: u64 },
    #[error("permanent TUS error: {0}")]
    Permanent(String),
}

impl TusError {
    pub fn is_transient(&self) -> bool {
        matches!(self, TusError::Transient(_))
    }
}

/// Outcome of a single `PATCH` chunk: the server's new authoritative offset.
#[derive(Debug, Clone, Copy)]
pub struct PatchAck {
    pub upload_offset: u64,
}

#[derive(Clone)]
pub struct TusClient {
    client: Client,
    extra_headers: HashMap<String, String>,
    bearer_token: Option<String>,
}

impl TusClient {
    pub fn new(extra_headers: HashMap<String, String>, bearer_token: Option<String>) -> Self {
        // No body timeout — uploads are streaming and may run for minutes.
        // Per-connection idle is left to reqwest defaults.
        let client = Client::builder()
            .pool_idle_timeout(Duration::from_secs(90))
            .build()
            .expect("build reqwest client");
        Self {
            client,
            extra_headers,
            bearer_token,
        }
    }

    /// Build a header map with the TUS protocol header, configured extras,
    /// and (optionally) the Authorization bearer.
    fn base_headers(&self) -> HeaderMap {
        let mut h = HeaderMap::new();
        h.insert(HDR_TUS_RESUMABLE, HeaderValue::from_static(TUS_RESUMABLE));
        for (k, v) in &self.extra_headers {
            if let (Ok(name), Ok(val)) =
                (HeaderName::try_from(k.as_str()), HeaderValue::from_str(v))
            {
                h.insert(name, val);
            }
        }
        if let Some(token) = &self.bearer_token {
            if let Ok(val) = HeaderValue::from_str(&format!("Bearer {token}")) {
                h.insert("Authorization", val);
            }
        }
        h
    }

    /// `POST` the creation request. `metadata` becomes the
    /// `Upload-Metadata` header (TUS spec: comma-separated `key base64(value)`).
    /// Returns the absolute or origin-relative `Location` of the new upload.
    pub async fn create(
        &self,
        upload_url: &str,
        total_size: u64,
        metadata: &HashMap<String, String>,
    ) -> Result<String, TusError> {
        let mut headers = self.base_headers();
        headers.insert(
            HDR_UPLOAD_LENGTH,
            HeaderValue::from_str(&total_size.to_string())
                .map_err(|e| TusError::Permanent(format!("bad upload-length: {e}")))?,
        );
        if !metadata.is_empty() {
            let encoded = encode_metadata(metadata);
            headers.insert(
                HDR_UPLOAD_METADATA,
                HeaderValue::from_str(&encoded)
                    .map_err(|e| TusError::Permanent(format!("bad upload-metadata: {e}")))?,
            );
        }
        // Some TUS servers reject creation without a Content-Length: 0.
        headers.insert("Content-Length", HeaderValue::from_static("0"));

        let resp = self
            .client
            .post(upload_url)
            .headers(headers)
            .send()
            .await
            .map_err(|e| classify_send_err(&e))?;

        let status = resp.status();
        if status == StatusCode::CREATED {
            let location = resp
                .headers()
                .get("Location")
                .and_then(|v| v.to_str().ok())
                .ok_or_else(|| TusError::Permanent("creation response missing Location".into()))?
                .to_string();
            Ok(resolve_location(upload_url, &location))
        } else if status.is_server_error() {
            Err(TusError::Transient(format!(
                "create returned {status}: {}",
                read_body(resp).await
            )))
        } else {
            Err(TusError::Permanent(format!(
                "create returned {status}: {}",
                read_body(resp).await
            )))
        }
    }

    /// `HEAD` the upload to get the authoritative `Upload-Offset`.
    pub async fn head(&self, location: &str) -> Result<u64, TusError> {
        let resp = self
            .client
            .head(location)
            .headers(self.base_headers())
            .send()
            .await
            .map_err(|e| classify_send_err(&e))?;

        let status = resp.status();
        if status == StatusCode::NOT_FOUND || status == StatusCode::GONE {
            return Err(TusError::Permanent(format!(
                "upload no longer exists ({status})"
            )));
        }
        if status.is_server_error() {
            return Err(TusError::Transient(format!("HEAD returned {status}")));
        }
        if !status.is_success() && status != StatusCode::NO_CONTENT {
            return Err(TusError::Permanent(format!(
                "HEAD returned {status}: {}",
                read_body(resp).await
            )));
        }
        parse_offset(resp.headers())
    }

    /// `PATCH` a single chunk starting at `offset`. The body length must
    /// match `chunk_len` for the server to accept it. The chunk's SHA-256
    /// is sent as `Upload-Checksum` so the server can reject corruption.
    pub async fn patch_chunk(
        &self,
        location: &str,
        offset: u64,
        chunk: Bytes,
    ) -> Result<PatchAck, TusError> {
        let chunk_len = chunk.len() as u64;
        let mut hasher = Sha256::new();
        hasher.update(&chunk);
        let digest = hasher.finalize();
        let checksum_b64 = base64::engine::general_purpose::STANDARD.encode(digest);

        let mut headers = self.base_headers();
        headers.insert(
            HDR_UPLOAD_OFFSET,
            HeaderValue::from_str(&offset.to_string()).expect("u64 -> header"),
        );
        headers.insert(
            "Content-Type",
            HeaderValue::from_static(CT_OFFSET_OCTET_STREAM),
        );
        headers.insert(
            "Content-Length",
            HeaderValue::from_str(&chunk_len.to_string()).expect("u64 -> header"),
        );
        headers.insert(
            HDR_UPLOAD_CHECKSUM,
            HeaderValue::from_str(&format!("sha256 {checksum_b64}"))
                .expect("checksum is ascii base64"),
        );

        let resp = self
            .client
            .patch(location)
            .headers(headers)
            .body(Body::from(chunk))
            .send()
            .await
            .map_err(|e| classify_send_err(&e))?;

        let status = resp.status();
        if status == StatusCode::CONFLICT {
            // 409 Conflict — server's authoritative offset disagrees with
            // ours. Try to read its offset header so the caller can realign
            // without an extra HEAD round-trip.
            return match parse_offset(resp.headers()) {
                Ok(server_offset) => Err(TusError::Mismatch {
                    server: server_offset,
                    client: offset,
                }),
                Err(_) => Err(TusError::Mismatch {
                    server: offset, // unknown; caller should HEAD
                    client: offset,
                }),
            };
        }
        if status == StatusCode::NO_CONTENT || status == StatusCode::OK {
            let new_offset = parse_offset(resp.headers())?;
            return Ok(PatchAck {
                upload_offset: new_offset,
            });
        }
        if status.is_server_error() {
            return Err(TusError::Transient(format!(
                "PATCH returned {status}: {}",
                read_body(resp).await
            )));
        }
        Err(TusError::Permanent(format!(
            "PATCH returned {status}: {}",
            read_body(resp).await
        )))
    }
}

fn parse_offset(headers: &HeaderMap) -> Result<u64, TusError> {
    headers
        .get(HDR_UPLOAD_OFFSET)
        .and_then(|v| v.to_str().ok())
        .and_then(|s| s.parse::<u64>().ok())
        .ok_or_else(|| TusError::Permanent("response missing or invalid Upload-Offset".into()))
}

fn classify_send_err(e: &reqwest::Error) -> TusError {
    if e.is_timeout() || e.is_connect() || e.is_request() {
        TusError::Transient(format!("send error: {e}"))
    } else {
        TusError::Permanent(format!("send error: {e}"))
    }
}

async fn read_body(resp: reqwest::Response) -> String {
    resp.text().await.unwrap_or_else(|_| "<unreadable>".into())
}

/// Resolve a possibly-relative `Location` against the upload URL.
fn resolve_location(upload_url: &str, location: &str) -> String {
    if location.starts_with("http://") || location.starts_with("https://") {
        return location.to_string();
    }
    if let Some(scheme_end) = upload_url.find("://") {
        let after = &upload_url[scheme_end + 3..];
        if let Some(slash) = after.find('/') {
            let origin = &upload_url[..scheme_end + 3 + slash];
            if location.starts_with('/') {
                return format!("{origin}{location}");
            }
            return format!("{origin}/{location}");
        }
        // upload_url has no path; just append.
        if location.starts_with('/') {
            return format!("{upload_url}{location}");
        }
        return format!("{upload_url}/{location}");
    }
    location.to_string()
}

/// Encode a metadata map as `key base64(value), key base64(value)` per
/// the TUS Creation extension. Keys must be ASCII; non-ASCII keys are
/// dropped (the protocol forbids them).
fn encode_metadata(meta: &HashMap<String, String>) -> String {
    let mut parts: Vec<String> = meta
        .iter()
        .filter(|(k, _)| k.is_ascii() && !k.is_empty() && !k.contains(' ') && !k.contains(','))
        .map(|(k, v)| {
            let b64 = base64::engine::general_purpose::STANDARD.encode(v.as_bytes());
            format!("{k} {b64}")
        })
        .collect();
    // Sort for deterministic output (test-friendly + cache-friendly).
    parts.sort();
    parts.join(",")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn resolves_absolute_location() {
        assert_eq!(
            resolve_location("https://cp.example.com/uploads", "https://cdn.example.com/u/abc"),
            "https://cdn.example.com/u/abc"
        );
    }

    #[test]
    fn resolves_root_relative_location() {
        assert_eq!(
            resolve_location("https://cp.example.com/uploads", "/files/abc"),
            "https://cp.example.com/files/abc"
        );
    }

    #[test]
    fn resolves_path_relative_location() {
        assert_eq!(
            resolve_location("https://cp.example.com/uploads", "abc"),
            "https://cp.example.com/abc"
        );
    }

    #[test]
    fn metadata_is_base64_encoded_and_sorted() {
        let mut m = HashMap::new();
        m.insert("filename".into(), "hello.csv".into());
        m.insert("workspace".into(), "ws_42".into());
        let encoded = encode_metadata(&m);
        // "filename" < "workspace" alphabetically
        assert!(encoded.starts_with("filename "));
        assert!(encoded.contains(",workspace "));
    }

    #[test]
    fn metadata_drops_invalid_keys() {
        let mut m = HashMap::new();
        m.insert("ok".into(), "v".into());
        m.insert("has space".into(), "v".into());
        m.insert("has,comma".into(), "v".into());
        let encoded = encode_metadata(&m);
        assert!(encoded.starts_with("ok "));
        assert!(!encoded.contains("has space"));
        assert!(!encoded.contains("has,comma"));
    }
}
