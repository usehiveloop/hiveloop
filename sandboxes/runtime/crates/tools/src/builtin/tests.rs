use super::*;
use crate::ToolRegistry;

#[test]
fn test_register_all_builtin_tools() {
    let mut registry = ToolRegistry::new();
    register_builtin_tools(&mut registry);
    // Should have at least the core tools registered
    assert!(registry.get("bash").is_some());
    assert!(registry.get("Read").is_some());
    assert!(registry.get("edit").is_some());
    assert!(registry.get("write").is_some());
    assert!(registry.get("RipGrep").is_some());
    assert!(registry.get("AstGrep").is_some());
    assert!(registry.get("Glob").is_some());
    assert!(registry.get("todowrite").is_some());
    assert!(registry.get("todoread").is_some());
    assert!(registry.get("batch").is_some());
}

#[test]
fn test_filtered_empty_list_registers_nothing() {
    let mut registry = ToolRegistry::new();
    register_filtered_builtin_tools(&mut registry, &[]);
    assert!(registry.list().is_empty());
}

#[test]
fn test_filtered_specific_tools() {
    let mut registry = ToolRegistry::new();
    let allowed = vec!["bash".to_string(), "Read".to_string()];
    register_filtered_builtin_tools(&mut registry, &allowed);
    assert!(registry.get("bash").is_some());
    assert!(registry.get("Read").is_some());
    assert!(registry.get("edit").is_none());
    assert!(registry.get("write").is_none());
}

#[test]
fn test_filtered_unknown_names_ignored() {
    let mut registry = ToolRegistry::new();
    let allowed = vec!["bash".to_string(), "nonexistent_tool".to_string()];
    register_filtered_builtin_tools(&mut registry, &allowed);
    assert!(registry.get("bash").is_some());
    assert!(registry.get("nonexistent_tool").is_none());
    // Only bash should be registered
    assert_eq!(registry.list().len(), 1);
}
