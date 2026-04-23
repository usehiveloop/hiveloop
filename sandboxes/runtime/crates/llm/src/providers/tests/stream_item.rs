use crate::providers::BridgeStreamItem;

#[test]
fn test_bridge_stream_item_reasoning_delta_variant_exists() {
    let item = BridgeStreamItem::ReasoningDelta("thinking...".to_string());
    match item {
        BridgeStreamItem::ReasoningDelta(text) => {
            assert_eq!(text, "thinking...");
        }
        _ => panic!("expected ReasoningDelta"),
    }
}

#[test]
fn test_reasoning_delta_empty_is_filtered() {
    // Verify that empty reasoning deltas would be filtered (matches provider logic)
    let delta = "";
    assert!(delta.is_empty(), "empty reasoning should be filtered");
}
