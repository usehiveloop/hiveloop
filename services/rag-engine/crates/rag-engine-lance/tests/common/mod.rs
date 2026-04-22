//! Shared test harness: boots a real MinIO via testcontainers, creates
//! a fresh bucket per test, hands back a [`LanceStore`] pointed at it.
//!
//! Per TESTING.md: mocks are only allowed for embedder/reranker; storage
//! is always real.

#![allow(dead_code)]

use std::sync::OnceLock;

use anyhow::Result;
use aws_config::BehaviorVersion;
use aws_credential_types::Credentials;
use aws_sdk_s3::config::Region;
use rag_engine_lance::{LanceStore, StoreConfig};
use testcontainers::runners::AsyncRunner;
use testcontainers::ContainerAsync;
use testcontainers::ImageExt;
use testcontainers_modules::minio::MinIO;
use uuid::Uuid;

/// A running MinIO container — dropped at test end (testcontainers
/// shuts the container down on drop). Keep this alive for the
/// duration of a test.
pub struct MinioHarness {
    _container: ContainerAsync<MinIO>,
    pub endpoint: String,
    pub bucket: String,
    pub store: LanceStore,
}

/// Spin up MinIO and create a fresh bucket. Each test gets its own
/// MinIO (tests are isolated). Callers can also use
/// [`shared_minio_harness`] for a faster-but-less-isolated flavor.
pub async fn minio_harness() -> Result<MinioHarness> {
    // Pin to the "latest" tag since the default tag hard-coded by
    // testcontainers-modules may not be present locally and a pull
    // against docker hub mid-test can exceed the test timeout.
    let container = MinIO::default().with_tag("latest").start().await?;
    let host = container.get_host().await?;
    let port = container.get_host_port_ipv4(9000).await?;
    let endpoint = format!("http://{host}:{port}");
    let bucket = format!("rag-test-{}", Uuid::new_v4().simple());

    let s3 = s3_client(&endpoint).await;
    s3.create_bucket().bucket(&bucket).send().await?;

    let uri = format!("s3://{bucket}");
    let store = LanceStore::open(StoreConfig::S3 {
        uri,
        access_key_id: "minioadmin".into(),
        secret_access_key: "minioadmin".into(),
        endpoint: Some(endpoint.clone()),
        region: "us-east-1".into(),
        allow_http: true,
    })
    .await?;

    Ok(MinioHarness {
        _container: container,
        endpoint,
        bucket,
        store,
    })
}

async fn s3_client(endpoint: &str) -> aws_sdk_s3::Client {
    let creds = Credentials::new("minioadmin", "minioadmin", None, None, "test");
    let cfg = aws_sdk_s3::Config::builder()
        .behavior_version(BehaviorVersion::latest())
        .region(Region::new("us-east-1"))
        .endpoint_url(endpoint.to_string())
        .force_path_style(true)
        .credentials_provider(creds)
        .build();
    aws_sdk_s3::Client::from_conf(cfg)
}

/// A tiny deterministic vector of size `dim`. The same `seed` produces
/// the same vector; tests use this to make BYTE-IDENTICAL comparisons
/// after round-trips.
pub fn make_vector(dim: usize, seed: u64) -> Vec<f32> {
    use rand::rngs::StdRng;
    use rand::Rng;
    use rand::SeedableRng;
    let mut rng = StdRng::seed_from_u64(seed);
    let mut v = Vec::with_capacity(dim);
    for _ in 0..dim {
        // `random::<f32>()` samples from StandardUniform in rand 0.9.
        v.push(rng.random::<f32>());
    }
    // Normalize so vectors aren't trivially zero-distance.
    let norm = v.iter().map(|x| x * x).sum::<f32>().sqrt().max(1e-6);
    for x in v.iter_mut() {
        *x /= norm;
    }
    v
}

/// Ensure we only build one test subscriber, even if tests run in
/// parallel.
pub fn init_tracing_once() {
    static ONCE: OnceLock<()> = OnceLock::new();
    ONCE.get_or_init(|| {
        let _ = tracing_subscriber::fmt()
            .with_env_filter("rag_engine_lance=debug,lancedb=warn")
            .with_test_writer()
            .try_init();
    });
}
