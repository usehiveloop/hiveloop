use super::*;
use crate::ToolExecutor;

#[tokio::test]
async fn test_ping_me_back_returns_immediately() {
    let state = PingState::new();
    let tool = PingMeBackTool::new(state.clone());

    let start = std::time::Instant::now();
    let args = serde_json::json!({
        "seconds": 300,
        "message": "check the deployment"
    });
    let result = tool.execute(args).await.expect("should succeed");
    let elapsed = start.elapsed();

    // Should return immediately, not block for 300 seconds
    assert!(elapsed < std::time::Duration::from_secs(1));
    assert!(result.contains("Ping scheduled"));
    assert!(result.contains("300 seconds"));
    assert!(result.contains("Ping ID:"));

    // State should have one pending ping
    let pings = state.list().await;
    assert_eq!(pings.len(), 1);
    assert_eq!(pings[0].message, "check the deployment");
}

#[tokio::test]
async fn test_cancel_ping() {
    let state = PingState::new();
    let id = state.add("test".to_string(), 300).await;

    let tool = CancelPingTool::new(state.clone());
    let result = tool
        .execute(serde_json::json!({ "id": id }))
        .await
        .expect("should succeed");
    assert!(result.contains("cancelled"));

    assert!(state.list().await.is_empty());
}

#[tokio::test]
async fn test_cancel_nonexistent_ping() {
    let state = PingState::new();
    let tool = CancelPingTool::new(state);
    let result = tool
        .execute(serde_json::json!({ "id": "nonexistent" }))
        .await;
    assert!(result.is_err());
}

#[tokio::test]
async fn test_pop_fired() {
    let state = PingState::new();
    // Add a ping that fires immediately (1 second)
    state.add("immediate".to_string(), 0).await;

    // Wait a tiny bit
    tokio::time::sleep(std::time::Duration::from_millis(10)).await;

    // But seconds=0 is rejected by the tool — test via state directly
    // Add with delay 0 would still set fires_at to now, so pop_fired should get it
}

#[tokio::test]
async fn test_zero_seconds_rejected() {
    let state = PingState::new();
    let tool = PingMeBackTool::new(state);
    let result = tool
        .execute(serde_json::json!({ "seconds": 0, "message": "nope" }))
        .await;
    assert!(result.is_err());
}

#[tokio::test]
async fn test_format_pending_pings_empty() {
    assert_eq!(format_pending_pings_reminder(&[]), "");
}

#[tokio::test]
async fn test_format_pending_pings_with_items() {
    let pings = vec![PendingPing {
        id: "abc123".to_string(),
        message: "check build".to_string(),
        fires_at: tokio::time::Instant::now() + std::time::Duration::from_secs(120),
        delay_secs: 120,
        created_at: chrono::Utc::now(),
    }];
    let text = format_pending_pings_reminder(&pings);
    assert!(text.contains("abc123"));
    assert!(text.contains("check build"));
    assert!(text.contains("Pending Ping-Me-Back"));
}

#[tokio::test]
async fn test_clamps_to_max_delay() {
    let state = PingState::new();
    let tool = PingMeBackTool::new(state.clone());
    let result = tool
        .execute(serde_json::json!({ "seconds": 99999, "message": "long" }))
        .await
        .expect("should succeed");
    assert!(result.contains("3600 seconds"));

    let pings = state.list().await;
    assert_eq!(pings[0].delay_secs, 3600); // clamped to max
}
