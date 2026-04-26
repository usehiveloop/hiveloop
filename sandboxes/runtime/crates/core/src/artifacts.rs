use std::collections::HashMap;

use serde::{Deserialize, Serialize};

/// Per-agent workspace artifact upload configuration.
///
/// When present on an [`crate::agent::AgentDefinition`], bridge auto-registers
/// an `upload_to_workspace` tool that streams files from the agent's sandbox
/// to the control plane via a tus.io-compatible resumable upload protocol.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct ArtifactsConfig {
    /// tus.io creation endpoint on the control plane. A POST here returns a
    /// per-upload `Location` URL used for chunked PATCH operations.
    pub upload_url: String,

    /// Optional canonical download URL template returned to the agent
    /// alongside the upload result. Bridge does not call this — it's a hint
    /// the control plane includes for downstream consumers.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub download_url: Option<String>,

    /// Hard upper bound on file size in bytes. Files larger than this are
    /// rejected before any network I/O.
    pub max_size_bytes: u64,

    /// Allowed file types. Each entry is matched against the file's MIME type
    /// (e.g. `text/csv`, `video/mp4`) or its extension (e.g. `csv`, `mp4`).
    /// Wildcards are supported on the MIME-type side (e.g. `video/*`).
    pub accepted_file_types: Vec<String>,

    /// Maximum concurrent in-flight uploads for this agent. Defaults to a
    /// modest ceiling so a runaway agent can't saturate the network.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub max_concurrent_uploads: Option<u32>,

    /// Chunk size used for PATCH requests. Defaults to 8 MiB. Smaller values
    /// give finer-grained resume but more HTTP round-trips.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub chunk_size_bytes: Option<u64>,

    /// Extra headers forwarded on every upload request (creation + each
    /// chunk). Use for workspace IDs, tenant identifiers, etc.
    #[serde(default, skip_serializing_if = "HashMap::is_empty")]
    pub headers: HashMap<String, String>,
}

impl ArtifactsConfig {
    /// Default chunk size (8 MiB).
    pub const DEFAULT_CHUNK_SIZE_BYTES: u64 = 8 * 1024 * 1024;

    /// Default concurrent upload ceiling.
    pub const DEFAULT_MAX_CONCURRENT_UPLOADS: u32 = 4;

    /// Resolve the configured chunk size or fall back to the default.
    pub fn effective_chunk_size(&self) -> u64 {
        self.chunk_size_bytes
            .unwrap_or(Self::DEFAULT_CHUNK_SIZE_BYTES)
    }

    /// Resolve the configured concurrency cap or fall back to the default.
    pub fn effective_max_concurrent_uploads(&self) -> u32 {
        self.max_concurrent_uploads
            .unwrap_or(Self::DEFAULT_MAX_CONCURRENT_UPLOADS)
    }

    /// Static validation. Returns the first problem found.
    pub fn validate(&self) -> Result<(), String> {
        let upload = self.upload_url.trim();
        if upload.is_empty() {
            return Err("artifacts.upload_url must not be empty".to_string());
        }
        if !is_http_url(upload) {
            return Err(format!(
                "artifacts.upload_url must be an http(s) URL (got '{upload}')"
            ));
        }

        if let Some(dl) = &self.download_url {
            let dl = dl.trim();
            if !is_http_url(dl) {
                return Err(format!(
                    "artifacts.download_url must be an http(s) URL (got '{dl}')"
                ));
            }
        }

        if self.max_size_bytes == 0 {
            return Err("artifacts.max_size_bytes must be greater than zero".to_string());
        }

        if self.accepted_file_types.is_empty() {
            return Err("artifacts.accepted_file_types must not be empty".to_string());
        }
        for entry in &self.accepted_file_types {
            if entry.trim().is_empty() {
                return Err("artifacts.accepted_file_types contains an empty entry".to_string());
            }
        }

        if let Some(0) = self.max_concurrent_uploads {
            return Err("artifacts.max_concurrent_uploads must be greater than zero".to_string());
        }
        if let Some(0) = self.chunk_size_bytes {
            return Err("artifacts.chunk_size_bytes must be greater than zero".to_string());
        }

        Ok(())
    }

    /// Decide whether a file with the given MIME type and extension is
    /// allowed by this config. `extension` is matched case-insensitively
    /// against bare entries; `mime` matches exact entries or wildcard
    /// patterns like `video/*`.
    pub fn is_accepted(&self, mime: &str, extension: Option<&str>) -> bool {
        let mime_lower = mime.to_ascii_lowercase();
        let ext_lower = extension.map(|e| e.to_ascii_lowercase());
        for entry in &self.accepted_file_types {
            let entry_lower = entry.trim().to_ascii_lowercase();
            if entry_lower.contains('/') {
                if let Some(prefix) = entry_lower.strip_suffix("/*") {
                    if mime_lower.starts_with(&format!("{prefix}/")) {
                        return true;
                    }
                } else if entry_lower == mime_lower {
                    return true;
                }
            } else if let Some(ext) = ext_lower.as_deref() {
                if entry_lower == ext {
                    return true;
                }
            }
        }
        false
    }
}

fn is_http_url(s: &str) -> bool {
    let lower = s.to_ascii_lowercase();
    (lower.starts_with("http://") && s.len() > "http://".len())
        || (lower.starts_with("https://") && s.len() > "https://".len())
}

#[cfg(test)]
mod tests {
    use super::*;

    fn base() -> ArtifactsConfig {
        ArtifactsConfig {
            upload_url: "https://cp.example.com/uploads".to_string(),
            download_url: None,
            max_size_bytes: 10_000_000,
            accepted_file_types: vec!["csv".to_string(), "video/*".to_string()],
            max_concurrent_uploads: None,
            chunk_size_bytes: None,
            headers: HashMap::new(),
        }
    }

    #[test]
    fn validates_minimal_config() {
        assert!(base().validate().is_ok());
    }

    #[test]
    fn rejects_empty_upload_url() {
        let mut c = base();
        c.upload_url = "   ".into();
        assert!(c.validate().is_err());
    }

    #[test]
    fn rejects_non_http_scheme() {
        let mut c = base();
        c.upload_url = "ftp://example.com/uploads".into();
        let err = c.validate().unwrap_err();
        assert!(err.contains("http(s)"));
    }

    #[test]
    fn rejects_zero_size() {
        let mut c = base();
        c.max_size_bytes = 0;
        assert!(c.validate().is_err());
    }

    #[test]
    fn rejects_empty_accepted_types() {
        let mut c = base();
        c.accepted_file_types = vec![];
        assert!(c.validate().is_err());
    }

    #[test]
    fn rejects_blank_accepted_entry() {
        let mut c = base();
        c.accepted_file_types = vec!["csv".into(), "  ".into()];
        assert!(c.validate().is_err());
    }

    #[test]
    fn rejects_zero_concurrency() {
        let mut c = base();
        c.max_concurrent_uploads = Some(0);
        assert!(c.validate().is_err());
    }

    #[test]
    fn rejects_zero_chunk_size() {
        let mut c = base();
        c.chunk_size_bytes = Some(0);
        assert!(c.validate().is_err());
    }

    #[test]
    fn accepts_by_extension() {
        let c = base();
        assert!(c.is_accepted("application/octet-stream", Some("csv")));
    }

    #[test]
    fn accepts_by_wildcard_mime() {
        let c = base();
        assert!(c.is_accepted("video/mp4", Some("mp4")));
        assert!(c.is_accepted("video/quicktime", None));
    }

    #[test]
    fn accepts_by_exact_mime() {
        let mut c = base();
        c.accepted_file_types = vec!["audio/mpeg".into()];
        assert!(c.is_accepted("audio/mpeg", Some("mp3")));
        assert!(!c.is_accepted("audio/wav", Some("wav")));
    }

    #[test]
    fn rejects_unmatched_type() {
        let c = base();
        assert!(!c.is_accepted("application/pdf", Some("pdf")));
    }

    #[test]
    fn defaults_apply() {
        let c = base();
        assert_eq!(
            c.effective_chunk_size(),
            ArtifactsConfig::DEFAULT_CHUNK_SIZE_BYTES
        );
        assert_eq!(
            c.effective_max_concurrent_uploads(),
            ArtifactsConfig::DEFAULT_MAX_CONCURRENT_UPLOADS
        );
    }
}
