use std::path::{Path, PathBuf};
use std::sync::{Arc, OnceLock};

use dashmap::DashMap;
use tokio::sync::Mutex;

use crate::path::canonicalize_best_effort;

static MUTATION_QUEUE: OnceLock<DashMap<PathBuf, Arc<Mutex<()>>>> = OnceLock::new();

fn registry() -> &'static DashMap<PathBuf, Arc<Mutex<()>>> {
    MUTATION_QUEUE.get_or_init(DashMap::new)
}

pub async fn with_file_lock<F, Fut, T>(path: &Path, operation: F) -> T
where
    F: FnOnce() -> Fut,
    Fut: std::future::Future<Output = T>,
{
    let key = canonicalize_best_effort(path);
    let lock = registry()
        .entry(key)
        .or_insert_with(|| Arc::new(Mutex::new(())))
        .clone();
    let _guard = lock.lock().await;
    operation().await
}
