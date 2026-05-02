use super::*;

fn test_event_bus() -> Arc<EventBus> {
    Arc::new(EventBus::new(None, None, String::new(), String::new()))
}

#[tokio::test]
async fn test_request_and_approve() {
    let manager = Arc::new(PermissionManager::new());
    let event_bus = test_event_bus();

    let manager_clone = manager.clone();
    let event_bus_clone = event_bus.clone();
    let handle = tokio::spawn(async move {
        manager_clone
            .request_approval(
                "agent1",
                "conv1",
                "bash",
                "call_1",
                &json!({"command": "ls"}),
                &event_bus_clone,
                None,
                None,
            )
            .await
    });

    // Give it a moment to register
    tokio::task::yield_now().await;

    let pending = manager.list_pending("conv1");
    assert_eq!(pending.len(), 1);
    assert!(manager.resolve(
        &pending[0].id,
        ApprovalDecision::Approve,
        None,
        Some(&event_bus)
    ));

    let result = handle.await.unwrap();
    assert_eq!(result, Ok((ApprovalDecision::Approve, None)));
}

#[tokio::test]
async fn test_request_and_deny() {
    let manager = Arc::new(PermissionManager::new());
    let event_bus = test_event_bus();

    let manager_clone = manager.clone();
    let event_bus_clone = event_bus.clone();
    let handle = tokio::spawn(async move {
        manager_clone
            .request_approval(
                "agent1",
                "conv1",
                "bash",
                "call_1",
                &json!({"command": "rm -rf /"}),
                &event_bus_clone,
                None,
                None,
            )
            .await
    });

    // Give it a moment to register
    tokio::task::yield_now().await;

    let pending = manager.list_pending("conv1");
    assert_eq!(pending.len(), 1);
    assert!(manager.resolve(
        &pending[0].id,
        ApprovalDecision::Deny,
        Some("Not allowed".to_string()),
        Some(&event_bus)
    ));

    let result = handle.await.unwrap();
    assert_eq!(
        result,
        Ok((ApprovalDecision::Deny, Some("Not allowed".to_string())))
    );
}

#[tokio::test]
async fn test_cleanup_conversation() {
    let manager = Arc::new(PermissionManager::new());
    let event_bus = test_event_bus();

    let m2 = manager.clone();
    let _handle = tokio::spawn(async move {
        let _ = m2
            .request_approval(
                "agent1",
                "conv1",
                "bash",
                "call_1",
                &json!({}),
                &event_bus,
                None,
                None,
            )
            .await;
    });

    // Give it a moment to register
    tokio::task::yield_now().await;

    assert_eq!(manager.list_pending("conv1").len(), 1);
    manager.cleanup_conversation("conv1");
    assert_eq!(manager.list_pending("conv1").len(), 0);
}

#[tokio::test]
async fn test_list_pending_filters_by_conversation() {
    let manager = Arc::new(PermissionManager::new());
    let event_bus = test_event_bus();

    let m2 = manager.clone();
    let eb2 = event_bus.clone();
    let _h1 = tokio::spawn(async move {
        let _ = m2
            .request_approval(
                "agent1",
                "conv1",
                "bash",
                "call_1",
                &json!({}),
                &eb2,
                None,
                None,
            )
            .await;
    });

    let m3 = manager.clone();
    let eb3 = event_bus.clone();
    let _h2 = tokio::spawn(async move {
        let _ = m3
            .request_approval(
                "agent1",
                "conv2",
                "bash",
                "call_2",
                &json!({}),
                &eb3,
                None,
                None,
            )
            .await;
    });

    tokio::task::yield_now().await;

    assert_eq!(manager.list_pending("conv1").len(), 1);
    assert_eq!(manager.list_pending("conv2").len(), 1);
}

#[test]
fn test_resolve_nonexistent() {
    let manager = PermissionManager::new();
    assert!(!manager.resolve("nonexistent", ApprovalDecision::Approve, None, None));
}
