use super::super::truncate::{truncate_output, MAX_OUTPUT_BYTES};

#[test]
fn test_truncate_output_short() {
    let short = b"hello";
    assert_eq!(truncate_output(short), "hello");
}

#[test]
fn test_truncate_output_spills_to_disk() {
    let long = vec![b'x'; MAX_OUTPUT_BYTES + 100];
    let result = truncate_output(&long);
    // Should contain head, tail, and a spill path
    assert!(
        result.contains("Output truncated"),
        "should mention truncation"
    );
    assert!(result.contains("saved to:"), "should include file path");
    assert!(
        result.contains("bridge_bash_"),
        "should reference a temp file"
    );

    // Extract the path and verify the file exists. The path is followed
    // by ". To find specific content, ..." — so stop at that sentence
    // boundary rather than the closing bracket.
    let path_start = result.find("saved to: ").unwrap() + "saved to: ".len();
    let path_end = result[path_start..].find(". To find").unwrap() + path_start;
    let spill_path = &result[path_start..path_end];
    let content = std::fs::read(spill_path).expect("spill file should be readable");
    assert_eq!(content.len(), MAX_OUTPUT_BYTES + 100);
    // Clean up
    let _ = std::fs::remove_file(spill_path);
}

#[test]
fn test_truncate_output_multibyte() {
    // Build bytes that, after from_utf8_lossy, produce multi-byte replacement chars.
    // Each invalid byte becomes U+FFFD (3 bytes in UTF-8), so the lossy string is larger
    // than the input. Use valid multi-byte UTF-8 instead: 'あ' = 3 bytes.
    // We need > MAX_OUTPUT_BYTES bytes of multi-byte chars so slicing at byte boundaries
    // without floor_char_boundary would panic.
    let ch = "あ"; // 3 bytes
    let repeat_count = (MAX_OUTPUT_BYTES / ch.len()) + 100;
    let big_string = ch.repeat(repeat_count);
    let result = truncate_output(big_string.as_bytes());
    // Main assertion: no panic. Also verify truncation happened.
    assert!(
        result.contains("Output truncated") || result.contains("[output truncated]"),
        "should be truncated"
    );
}
