use super::*;
use bridge_core::event::BridgeEventType;

fn make_event(conv_id: &str) -> BridgeEvent {
    BridgeEvent::new(
        BridgeEventType::ConversationCreated,
        "agent-1",
        conv_id,
        serde_json::json!({}),
    )
}

#[tokio::test]
async fn test_delivery_exits_on_cancel() {
    let (_tx, rx) = mpsc::unbounded_channel();
    let client = Client::new();
    let cancel = CancellationToken::new();
    let config = WebhookConfig::default();

    let cancel_clone = cancel.clone();
    let handle = tokio::spawn(async move {
        run_delivery(
            rx,
            client,
            cancel_clone,
            config,
            "https://example.com".to_string(),
            "secret".to_string(),
            None,
        )
        .await;
    });

    tokio::time::sleep(Duration::from_millis(10)).await;
    cancel.cancel();

    tokio::time::timeout(Duration::from_secs(2), handle)
        .await
        .expect("should exit on cancel")
        .expect("should not panic");
}

#[tokio::test]
async fn test_delivery_body_is_json_array() {
    use wiremock::matchers::method;
    use wiremock::{Mock, MockServer, ResponseTemplate};

    let mock_server = MockServer::start().await;
    Mock::given(method("POST"))
        .respond_with(ResponseTemplate::new(200))
        .mount(&mock_server)
        .await;

    let config = WebhookConfig {
        max_retries: 0,
        delivery_timeout_secs: 5,
        worker_idle_timeout_secs: 2,
        ..WebhookConfig::default()
    };

    let (tx, rx) = mpsc::unbounded_channel();
    let client = Client::new();
    let cancel = CancellationToken::new();

    tx.send(make_event("conv-1")).unwrap();

    let cancel_clone = cancel.clone();
    let url = mock_server.uri();
    let handle = tokio::spawn(async move {
        run_delivery(
            rx,
            client,
            cancel_clone,
            config,
            url,
            "secret".to_string(),
            None,
        )
        .await;
    });

    tokio::time::sleep(Duration::from_millis(500)).await;
    cancel.cancel();
    handle.await.unwrap();

    let requests = mock_server.received_requests().await.unwrap();
    assert_eq!(requests.len(), 1);

    let body: serde_json::Value = serde_json::from_slice(&requests[0].body).expect("valid JSON");
    assert!(body.is_array(), "body must be a JSON array");
    assert_eq!(body.as_array().unwrap().len(), 1);

    // Verify no secrets in the delivered payload
    let event = &body[0];
    assert!(event.get("webhook_url").is_none());
    assert!(event.get("webhook_secret").is_none());
    assert!(event["event_id"].is_string());
    assert!(event["sequence_number"].is_number());
}

#[tokio::test]
async fn test_delivery_sequence_numbers_preserved() {
    use wiremock::matchers::method;
    use wiremock::{Mock, MockServer, ResponseTemplate};

    let mock_server = MockServer::start().await;
    Mock::given(method("POST"))
        .respond_with(ResponseTemplate::new(200))
        .mount(&mock_server)
        .await;

    let config = WebhookConfig {
        max_retries: 0,
        delivery_timeout_secs: 5,
        worker_idle_timeout_secs: 2,
        ..WebhookConfig::default()
    };

    let (tx, rx) = mpsc::unbounded_channel();
    let client = Client::new();
    let cancel = CancellationToken::new();

    // Send events with pre-stamped sequence numbers (as EventBus would)
    for i in 1..=5 {
        let mut event = make_event("conv-1");
        event.sequence_number = i;
        tx.send(event).unwrap();
    }

    let cancel_clone = cancel.clone();
    let url = mock_server.uri();
    let handle = tokio::spawn(async move {
        run_delivery(
            rx,
            client,
            cancel_clone,
            config,
            url,
            "secret".to_string(),
            None,
        )
        .await;
    });

    tokio::time::sleep(Duration::from_secs(1)).await;
    cancel.cancel();
    handle.await.unwrap();

    let requests = mock_server.received_requests().await.unwrap();
    let mut all_seq: Vec<u64> = Vec::new();
    for req in &requests {
        let batch: Vec<serde_json::Value> =
            serde_json::from_slice(&req.body).expect("valid JSON array");
        for event in &batch {
            all_seq.push(event["sequence_number"].as_u64().unwrap());
        }
    }

    assert_eq!(all_seq, vec![1, 2, 3, 4, 5]);
}

#[tokio::test]
async fn test_cross_conversation_delivery() {
    use wiremock::matchers::method;
    use wiremock::{Mock, MockServer, ResponseTemplate};

    let mock_server = MockServer::start().await;
    Mock::given(method("POST"))
        .respond_with(ResponseTemplate::new(200))
        .mount(&mock_server)
        .await;

    let config = WebhookConfig {
        max_retries: 0,
        delivery_timeout_secs: 5,
        worker_idle_timeout_secs: 2,
        ..WebhookConfig::default()
    };

    let (tx, rx) = mpsc::unbounded_channel();
    let client = Client::new();
    let cancel = CancellationToken::new();

    for i in 0..3 {
        let mut e_a = make_event("conv-A");
        e_a.sequence_number = (i * 2 + 1) as u64;
        tx.send(e_a).unwrap();

        let mut e_b = make_event("conv-B");
        e_b.sequence_number = (i * 2 + 2) as u64;
        tx.send(e_b).unwrap();
    }

    let cancel_clone = cancel.clone();
    let url = mock_server.uri();
    let handle = tokio::spawn(async move {
        run_delivery(
            rx,
            client,
            cancel_clone,
            config,
            url,
            "secret".to_string(),
            None,
        )
        .await;
    });

    tokio::time::sleep(Duration::from_secs(1)).await;
    cancel.cancel();
    handle.await.unwrap();

    let requests = mock_server.received_requests().await.unwrap();
    let mut conv_a: Vec<u64> = Vec::new();
    let mut conv_b: Vec<u64> = Vec::new();
    for req in &requests {
        let batch: Vec<serde_json::Value> =
            serde_json::from_slice(&req.body).expect("valid JSON array");
        for event in &batch {
            let conv = event["conversation_id"].as_str().unwrap();
            let seq = event["sequence_number"].as_u64().unwrap();
            match conv {
                "conv-A" => conv_a.push(seq),
                "conv-B" => conv_b.push(seq),
                _ => panic!("unexpected conversation"),
            }
        }
    }

    assert_eq!(conv_a.len(), 3);
    assert_eq!(conv_b.len(), 3);
    // Each conversation's events are in order
    assert_eq!(conv_a, vec![1, 3, 5]);
    assert_eq!(conv_b, vec![2, 4, 6]);
}
