//! S3 / MinIO / local-disk connection wiring for LanceDB.
//!
//! The spike learned that `lancedb`'s `storage_options` builder is the
//! only path that actually reaches the Rust side; structured credential
//! fields get silently dropped. We go through `storage_options`
//! exclusively.

use std::collections::HashMap;
use std::sync::Arc;
use std::time::Duration;

use lancedb::{connect, Connection};
use tracing::debug;

use crate::error::{LanceStoreError, Result};

/// How to talk to the storage backend.
#[derive(Debug, Clone)]
pub enum StoreConfig {
    /// Local disk (tempdir etc.). `uri` is a filesystem path or `memory://`.
    Local { uri: String },
    /// S3-compatible (MinIO, real S3). `uri` is `s3://bucket/prefix`.
    S3 {
        uri: String,
        access_key_id: String,
        secret_access_key: String,
        /// For MinIO / custom endpoints; `None` for real AWS S3.
        endpoint: Option<String>,
        region: String,
        /// True for MinIO running on HTTP.
        allow_http: bool,
    },
}

/// A handle to an open LanceDB database connection. Cheap to clone.
#[derive(Clone)]
pub struct LanceStore {
    inner: Arc<Connection>,
}

impl LanceStore {
    /// Open (or lazily materialize) a LanceDB connection at `config`.
    pub async fn open(config: StoreConfig) -> Result<Self> {
        let conn = match config {
            StoreConfig::Local { uri } => {
                debug!(%uri, "opening lancedb (local)");
                connect(&uri)
                    // Read-your-own-writes for tests; in production we may want a
                    // non-zero interval, but 2B callers explicitly want strong
                    // consistency (tests read back what they just wrote).
                    .read_consistency_interval(Duration::from_secs(0))
                    .execute()
                    .await
                    .map_err(|e| LanceStoreError::Connect(format!("{e}")))?
            }
            StoreConfig::S3 {
                uri,
                access_key_id,
                secret_access_key,
                endpoint,
                region,
                allow_http,
            } => {
                debug!(%uri, ?endpoint, %region, allow_http, "opening lancedb (s3)");
                let mut opts: HashMap<String, String> = HashMap::new();
                opts.insert("access_key_id".into(), access_key_id);
                opts.insert("secret_access_key".into(), secret_access_key);
                opts.insert("region".into(), region);
                if let Some(ep) = endpoint {
                    opts.insert("endpoint".into(), ep);
                }
                if allow_http {
                    opts.insert("allow_http".into(), "true".into());
                }
                // MinIO / LocalStack don't provide a real IMDS endpoint; disabling
                // saves us the 5s probe timeout on each open.
                opts.insert("aws_ec2_metadata_disabled".into(), "true".into());
                // Needed because lance-core uses multi-part rename semantics under
                // S3 and MinIO's default config disables this.
                opts.insert("aws_s3_allow_unsafe_rename".into(), "true".into());

                connect(&uri)
                    .storage_options(opts)
                    .read_consistency_interval(Duration::from_secs(0))
                    .execute()
                    .await
                    .map_err(|e| LanceStoreError::Connect(format!("{e}")))?
            }
        };
        Ok(Self {
            inner: Arc::new(conn),
        })
    }

    /// Borrow the underlying `Connection`. Internal to this crate; the
    /// dataset/ingest/search modules use it directly.
    pub(crate) fn conn(&self) -> &Connection {
        &self.inner
    }
}
