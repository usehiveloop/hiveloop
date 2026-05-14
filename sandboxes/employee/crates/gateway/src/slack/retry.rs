use std::future::Future;
use std::time::Duration;

const BASE_BACKOFF_MS: u64 = 1500;

pub async fn with_retry<T, E, F, Fut>(max_attempts: u32, mut operation: F) -> Result<T, E>
where
    F: FnMut() -> Fut,
    Fut: Future<Output = Result<T, E>>,
    E: std::fmt::Display,
{
    let mut attempts: u32 = 0;
    let total_attempts = max_attempts.max(1);
    loop {
        attempts += 1;
        match operation().await {
            Ok(value) => return Ok(value),
            Err(error) => {
                if attempts >= total_attempts || !looks_retryable(&error) {
                    return Err(error);
                }
                let delay_ms = BASE_BACKOFF_MS * attempts as u64;
                tokio::time::sleep(Duration::from_millis(delay_ms)).await;
            }
        }
    }
}

fn looks_retryable<E: std::fmt::Display>(error: &E) -> bool {
    let rendered = error.to_string().to_lowercase();
    rendered.contains("rate_limited")
        || rendered.contains("ratelimited")
        || rendered.contains("429")
        || rendered.contains("timeout")
        || rendered.contains("connection")
        || rendered.contains("temporarily_unavailable")
        || rendered.contains("service_unavailable")
        || rendered.contains("502")
        || rendered.contains("503")
        || rendered.contains("504")
}
