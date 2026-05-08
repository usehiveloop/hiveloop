use agent::model_helpers::{build_user_content, describe_model};
use domain::ModelConfig;

#[test]
fn model_descriptions_are_human_readable() {
    let config = ModelConfig::OpenaiCompatible {
        base_url: "https://openrouter.ai/api/v1".into(),
        model_id: "deepseek/deepseek-v4-flash".into(),
        api_key_env: "KEY".into(),
        temperature: Some(0.3),
        max_output_tokens: Some(1024),
        reasoning_effort: None,
        extra_headers: Default::default(),
        fallback: None,
    };
    let desc = describe_model(&config);
    assert_eq!(desc, "deepseek/deepseek-v4-flash @ https://openrouter.ai/api/v1");
}

#[test]
fn user_content_builds_from_text() {
    let content = build_user_content("hello world".into(), vec![]);
    assert_eq!(content.role, "user");
    let text = content.parts.iter()
        .filter_map(|p| if let adk_rust::Part::Text { text } = p { Some(text.as_str()) } else { None })
        .collect::<Vec<_>>()
        .join("");
    assert_eq!(text, "hello world");
}

#[test]
fn user_content_includes_images() {
    let content = build_user_content("describe this".into(), vec![
        agent::ImageInput { mime_type: "image/png".into(), data: vec![0, 1, 2] }
    ]);
    assert_eq!(content.role, "user");
    let has_image = content.parts.iter().any(|p| matches!(p, adk_rust::Part::InlineData { .. }));
    assert!(has_image, "must contain inline image data");
}
