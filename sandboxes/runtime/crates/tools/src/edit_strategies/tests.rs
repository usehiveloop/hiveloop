use super::block_anchor::BlockAnchorReplacer;
use super::escape::EscapeNormalizedReplacer;
use super::indentation::IndentationFlexibleReplacer;
use super::line_trimmed::LineTrimmedReplacer;
use super::multi_occurrence::MultiOccurrenceReplacer;
use super::simple::SimpleReplacer;
use super::whitespace::WhitespaceNormalizedReplacer;
use super::{all_strategies, Replacer};

#[test]
fn test_simple_exact_match() {
    let r = SimpleReplacer;
    let result = r.try_replace("hello world", "world", "rust", false);
    assert_eq!(result, Some(("hello rust".to_string(), 1)));
}

#[test]
fn test_simple_no_match() {
    let r = SimpleReplacer;
    assert!(r.try_replace("hello", "world", "rust", false).is_none());
}

#[test]
fn test_line_trimmed_match() {
    let r = LineTrimmedReplacer;
    let content = "  hello\n  world\n";
    let result = r.try_replace(content, "hello\nworld", "foo\nbar", false);
    assert!(result.is_some());
    let (new_content, count) = result.unwrap();
    assert_eq!(count, 1);
    assert!(new_content.contains("foo"));
}

#[test]
fn test_whitespace_normalized() {
    let r = WhitespaceNormalizedReplacer;
    let content = "hello   world\n";
    let result = r.try_replace(content, "hello world", "hello rust", false);
    assert!(result.is_some());
}

#[test]
fn test_indentation_flexible() {
    let r = IndentationFlexibleReplacer;
    let content = "    fn main() {\n        println!(\"old\");\n    }\n";
    let old = "fn main() {\n    println!(\"old\");\n}";
    let new = "fn main() {\n    println!(\"new\");\n}";
    let result = r.try_replace(content, old, new, false);
    assert!(result.is_some());
    let (new_content, _) = result.unwrap();
    assert!(new_content.contains("new"));
}

#[test]
fn test_escape_normalized() {
    let r = EscapeNormalizedReplacer;
    let content = "line one\nline two\n";
    let result = r.try_replace(content, "line one\\nline two", "replaced", false);
    assert!(result.is_some());
}

#[test]
fn test_block_anchor_fuzzy() {
    let r = BlockAnchorReplacer;
    let content = "fn main() {\n    println!(\"hello\");\n}\n";
    // Slightly different: extra space
    let old = "fn main()  {\n    println!(\"hello\");\n}";
    let new = "fn main() {\n    println!(\"world\");\n}";
    let result = r.try_replace(content, old, new, false);
    assert!(result.is_some());
}

#[test]
fn test_multi_occurrence_replace_all() {
    let r = MultiOccurrenceReplacer;
    let content = "aaa\nbbb\naaa\n";
    let result = r.try_replace(content, "aaa", "ccc", true);
    assert!(result.is_some());
    let (new_content, count) = result.unwrap();
    assert_eq!(count, 2);
    assert!(!new_content.contains("aaa"));
}

#[test]
fn test_all_strategies_chain() {
    let strategies = all_strategies();
    assert_eq!(strategies.len(), 9);

    // Test that the chain finds an exact match
    let content = "hello world";
    for strategy in &strategies {
        if let Some((result, count)) = strategy.try_replace(content, "world", "rust", false) {
            assert_eq!(result, "hello rust");
            assert_eq!(count, 1);
            assert_eq!(strategy.name(), "simple"); // Should be found by first strategy
            return;
        }
    }
    panic!("No strategy matched");
}
