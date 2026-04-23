use super::*;
use wiremock::matchers::{body_json, method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

#[test]
fn test_web_search_description_is_rich() {
    let tool = WebSearchTool::new("http://unused".to_string());
    let desc = tool.description();
    assert!(!desc.is_empty());
    assert!(
        !desc.contains("{{year}}"),
        "template variable should be replaced"
    );
    let current_year = chrono::Utc::now().format("%Y").to_string();
    assert!(desc.contains(&current_year), "should contain current year");
    assert!(
        desc.contains("knowledge cutoff"),
        "should mention knowledge cutoff"
    );
}

fn serper_response() -> serde_json::Value {
    serde_json::json!({
        "searchParameters": { "q": "rust async", "type": "search", "engine": "google" },
        "knowledgeGraph": {
            "title": "Rust Programming Language",
            "description": "Rust is a multi-paradigm, general-purpose programming language."
        },
        "organic": [
            {
                "title": "Understanding Async Await in Rust",
                "link": "https://tokio.rs/tokio/tutorial",
                "snippet": "The Tokio runtime powers async Rust applications.",
                "position": 1
            },
            {
                "title": "Rust by Example - Async/Await",
                "link": "https://doc.rust-lang.org/rust-by-example/async/await.html",
                "snippet": "Async functions in Rust return a Future.",
                "position": 2
            }
        ],
        "peopleAlsoAsk": [
            {
                "question": "Is Rust good for async programming?",
                "snippet": "Yes, Rust has first-class async/await support."
            }
        ]
    })
}

#[tokio::test]
async fn test_serper_response_parsing() {
    let server = MockServer::start().await;

    Mock::given(method("POST"))
        .and(path("/search"))
        .respond_with(ResponseTemplate::new(200).set_body_json(serper_response()))
        .expect(1)
        .mount(&server)
        .await;

    let tool = WebSearchTool::new(format!("{}/search", server.uri()));
    let args = serde_json::json!({ "query": "rust async" });
    let result = tool.execute(args).await.expect("execute should succeed");

    let results: Vec<SearchResult> = serde_json::from_str(&result).expect("should parse results");

    // Knowledge graph + 2 organic = 3 results
    assert_eq!(results.len(), 3);

    // First result is knowledge graph
    assert_eq!(results[0].title, "Rust Programming Language");
    assert!(results[0]
        .snippet
        .contains("multi-paradigm, general-purpose"));
    assert_eq!(results[0].position, Some(0));

    // Organic results
    assert_eq!(results[1].title, "Understanding Async Await in Rust");
    assert_eq!(results[1].url, "https://tokio.rs/tokio/tutorial");
    assert_eq!(results[1].position, Some(1));

    assert_eq!(results[2].title, "Rust by Example - Async/Await");
    assert_eq!(results[2].position, Some(2));
}

#[tokio::test]
async fn test_post_body_contains_query() {
    let server = MockServer::start().await;

    Mock::given(method("POST"))
        .and(path("/search"))
        .and(body_json(serde_json::json!({ "q": "hello world" })))
        .respond_with(
            ResponseTemplate::new(200).set_body_json(serde_json::json!({ "organic": [] })),
        )
        .expect(1)
        .mount(&server)
        .await;

    let tool = WebSearchTool::new(format!("{}/search", server.uri()));
    let args = serde_json::json!({ "query": "hello world" });
    let result = tool.execute(args).await.expect("execute should succeed");

    let results: Vec<SearchResult> = serde_json::from_str(&result).expect("should parse results");
    assert!(results.is_empty());
}

#[tokio::test]
async fn test_empty_query_returns_error() {
    let tool = WebSearchTool::new("http://unused".to_string());
    let args = serde_json::json!({ "query": "  " });
    let err = tool.execute(args).await.unwrap_err();
    assert!(err.contains("must not be empty"));
}

#[test]
fn test_parse_empty_organic() {
    let body = r#"{ "organic": [] }"#;
    let results = parse_serper_response(body).expect("parse");
    assert!(results.is_empty());
}

#[test]
fn test_parse_missing_knowledge_graph() {
    let body = r#"{
        "organic": [{
            "title": "Example",
            "link": "https://example.com",
            "snippet": "An example.",
            "position": 1
        }]
    }"#;
    let results = parse_serper_response(body).expect("parse");
    assert_eq!(results.len(), 1);
    assert_eq!(results[0].title, "Example");
    assert_eq!(results[0].url, "https://example.com");
    assert_eq!(results[0].position, Some(1));
}

#[test]
fn test_parse_knowledge_graph_with_attributes() {
    let body = r#"{
        "knowledgeGraph": {
            "title": "Rust",
            "description": "A systems language.",
            "attributes": {
                "Developer": "Mozilla",
                "License": "MIT"
            }
        },
        "organic": []
    }"#;
    let results = parse_serper_response(body).expect("parse");
    assert_eq!(results.len(), 1);
    assert_eq!(results[0].title, "Rust");
    assert!(results[0].snippet.contains("systems language"));
    // Attributes are appended (order may vary in HashMap, so just check they're present)
    assert!(
        results[0].snippet.contains("Mozilla") || results[0].snippet.contains("MIT"),
        "snippet should contain attribute values"
    );
}

#[test]
fn test_parse_invalid_json() {
    let result = parse_serper_response("not json");
    assert!(result.is_err());
    assert!(result.unwrap_err().contains("Failed to parse JSON"));
}
