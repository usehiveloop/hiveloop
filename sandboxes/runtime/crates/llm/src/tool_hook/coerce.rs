//! Argument coercion: walk a JSON schema and convert string values that look
//! like the schema's actual primitive type (integer or number) into that type.
//!
//! Why: LLMs — particularly reasoning models — frequently emit tool-call
//! argument values as JSON strings even when the schema requires a primitive.
//! e.g. `bash` expects `timeout: integer`, model sends `"timeout": "300000"`.
//! Without coercion, `jsonschema` validation rejects it AND the downstream
//! `serde::Deserialize<BashArgs>` would also reject it. We've observed every
//! reasoning model in the bench (Qwen, Kimi, MiniMax) hitting this at least
//! once.
//!
//! Conservative by design:
//! - Only coerce when the schema's `type` is unambiguously the target
//!   primitive (`"integer"`, `"number"`, or `["integer", "null"]` /
//!   `["number", "null"]`).
//! - Use strict `str::parse` — trailing garbage rejects.
//! - Never touch values that are already the correct type, never touch
//!   strings when the schema accepts strings, never coerce booleans
//!   (string-to-bool is too ambiguous: yes/no/1/0/on/off).
//! - Recurse into `properties`, `items`, `additionalProperties`,
//!   and through `$ref`-free `allOf` branches that are pure schema
//!   refinements. Skip `oneOf`/`anyOf` to avoid guessing intent.

use serde_json::Value;

/// Walk `args` against `schema`, coercing string values into integers or
/// numbers when the schema requires those primitives. Mutates `args` in place.
pub(crate) fn coerce_args_against_schema(args: &mut Value, schema: &Value) {
    coerce_value(args, schema);
}

fn coerce_value(value: &mut Value, schema: &Value) {
    let Some(schema_obj) = schema.as_object() else {
        return;
    };

    // Recurse through `allOf` first — its branches refine the schema for
    // the same value. Skip oneOf/anyOf entirely (semantically ambiguous,
    // we don't know which branch the value should match).
    if let Some(all_of) = schema_obj.get("allOf").and_then(|v| v.as_array()) {
        for sub in all_of {
            coerce_value(value, sub);
        }
    }

    // Primitive coercion: only fires if the value is a string AND the
    // schema's `type` resolves unambiguously to integer or number.
    match resolve_target_type(schema_obj) {
        Some(TargetType::Integer) => {
            if let Some(s) = value.as_str() {
                if let Ok(n) = s.parse::<i64>() {
                    *value = Value::Number(n.into());
                }
            }
        }
        Some(TargetType::Number) => {
            if let Some(s) = value.as_str() {
                if let Ok(n) = s.parse::<f64>() {
                    if let Some(num) = serde_json::Number::from_f64(n) {
                        *value = Value::Number(num);
                    }
                }
            }
        }
        None => {}
    }

    // Recurse into nested structures.
    match value {
        Value::Object(map) => {
            // Per-property recursion.
            if let Some(props) = schema_obj.get("properties").and_then(|v| v.as_object()) {
                for (key, child) in map.iter_mut() {
                    if let Some(sub_schema) = props.get(key) {
                        coerce_value(child, sub_schema);
                    } else if let Some(ap) = schema_obj.get("additionalProperties") {
                        // additionalProperties: <schema> applies to keys not
                        // listed in `properties`.
                        coerce_value(child, ap);
                    }
                }
            } else if let Some(ap) = schema_obj.get("additionalProperties") {
                for child in map.values_mut() {
                    coerce_value(child, ap);
                }
            }
        }
        Value::Array(items) => {
            if let Some(item_schema) = schema_obj.get("items") {
                for child in items.iter_mut() {
                    coerce_value(child, item_schema);
                }
            }
        }
        _ => {}
    }
}

#[derive(Debug, Clone, Copy)]
enum TargetType {
    Integer,
    Number,
}

/// Decide whether a schema's `type` field unambiguously requires integer or
/// number. Returns None if the type is missing, ambiguous, accepts string,
/// or is a non-numeric primitive — in any of those cases coercion would be
/// either wrong or a no-op.
fn resolve_target_type(schema: &serde_json::Map<String, Value>) -> Option<TargetType> {
    let ty = schema.get("type")?;
    match ty {
        Value::String(s) => match s.as_str() {
            "integer" => Some(TargetType::Integer),
            "number" => Some(TargetType::Number),
            _ => None,
        },
        Value::Array(variants) => {
            // Treat ["integer", "null"] etc. as integer-only.
            // Reject any union that includes "string" — that means the
            // schema legitimately accepts strings, so the value-as-string
            // is the model's intent, not a mis-encoding.
            let mut found: Option<TargetType> = None;
            let mut has_string = false;
            for v in variants {
                let s = v.as_str()?;
                match s {
                    "null" => {} // allowed alongside primitive
                    "string" => has_string = true,
                    "integer" => match found {
                        None => found = Some(TargetType::Integer),
                        Some(TargetType::Number) => return None,
                        _ => {}
                    },
                    "number" => match found {
                        None => found = Some(TargetType::Number),
                        Some(TargetType::Integer) => {
                            // integer + number both allowed -> number is wider
                            found = Some(TargetType::Number);
                        }
                        _ => {}
                    },
                    // Any other type in the union (object, array, boolean) -> ambiguous.
                    _ => return None,
                }
            }
            if has_string {
                None
            } else {
                found
            }
        }
        _ => None,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    fn coerce(mut v: Value, s: Value) -> Value {
        coerce_args_against_schema(&mut v, &s);
        v
    }

    #[test]
    fn coerces_string_to_integer_at_top_level_property() {
        let v = coerce(
            json!({ "timeout": "300000" }),
            json!({
                "type": "object",
                "properties": { "timeout": { "type": "integer" } }
            }),
        );
        assert_eq!(v, json!({ "timeout": 300000 }));
    }

    #[test]
    fn coerces_string_to_number_at_top_level_property() {
        let v = coerce(
            json!({ "ratio": "0.5" }),
            json!({
                "type": "object",
                "properties": { "ratio": { "type": "number" } }
            }),
        );
        assert_eq!(v, json!({ "ratio": 0.5 }));
    }

    #[test]
    fn coerces_when_type_is_integer_or_null() {
        let v = coerce(
            json!({ "limit": "30" }),
            json!({
                "type": "object",
                "properties": {
                    "limit": { "type": ["integer", "null"] }
                }
            }),
        );
        assert_eq!(v, json!({ "limit": 30 }));
    }

    #[test]
    fn does_not_coerce_when_schema_accepts_string() {
        let v = coerce(
            json!({ "value": "300000" }),
            json!({
                "type": "object",
                "properties": {
                    "value": { "type": ["integer", "string"] }
                }
            }),
        );
        // Untouched: schema accepts strings, so the model's choice stands.
        assert_eq!(v, json!({ "value": "300000" }));
    }

    #[test]
    fn does_not_coerce_garbage_string() {
        let v = coerce(
            json!({ "n": "300000foo" }),
            json!({
                "type": "object",
                "properties": { "n": { "type": "integer" } }
            }),
        );
        assert_eq!(v, json!({ "n": "300000foo" }));
    }

    #[test]
    fn does_not_coerce_boolean_strings() {
        let v = coerce(
            json!({ "enabled": "true" }),
            json!({
                "type": "object",
                "properties": { "enabled": { "type": "boolean" } }
            }),
        );
        assert_eq!(v, json!({ "enabled": "true" }));
    }

    #[test]
    fn recurses_into_nested_objects() {
        let v = coerce(
            json!({ "outer": { "count": "5" } }),
            json!({
                "type": "object",
                "properties": {
                    "outer": {
                        "type": "object",
                        "properties": { "count": { "type": "integer" } }
                    }
                }
            }),
        );
        assert_eq!(v, json!({ "outer": { "count": 5 } }));
    }

    #[test]
    fn recurses_into_arrays_of_integers() {
        let v = coerce(
            json!({ "ids": ["1", "2", "3"] }),
            json!({
                "type": "object",
                "properties": {
                    "ids": {
                        "type": "array",
                        "items": { "type": "integer" }
                    }
                }
            }),
        );
        assert_eq!(v, json!({ "ids": [1, 2, 3] }));
    }

    #[test]
    fn handles_batch_tool_nested_tool_calls_arguments_property() {
        // Mirrors the `batch` tool: an array of objects each carrying a
        // `parameters` object whose schema is open. Without a sub-schema
        // for `parameters`, we leave it untouched — the inner tool's own
        // executor receives the raw object and is expected to handle it.
        let v = coerce(
            json!({ "tool_calls": [{ "tool": "Read", "parameters": { "file_path": "/x" } }] }),
            json!({
                "type": "object",
                "properties": {
                    "tool_calls": {
                        "type": "array",
                        "items": {
                            "type": "object",
                            "properties": {
                                "tool": { "type": "string" },
                                "parameters": { "type": "object" }
                            }
                        }
                    }
                }
            }),
        );
        assert_eq!(
            v,
            json!({ "tool_calls": [{ "tool": "Read", "parameters": { "file_path": "/x" } }] })
        );
    }

    #[test]
    fn no_op_when_value_already_correct_type() {
        let v = coerce(
            json!({ "n": 42 }),
            json!({
                "type": "object",
                "properties": { "n": { "type": "integer" } }
            }),
        );
        assert_eq!(v, json!({ "n": 42 }));
    }

    #[test]
    fn no_op_when_schema_has_no_type() {
        let v = coerce(
            json!({ "anything": "string-or-not" }),
            json!({
                "type": "object",
                "properties": { "anything": {} }
            }),
        );
        assert_eq!(v, json!({ "anything": "string-or-not" }));
    }

    #[test]
    fn skips_oneof_branches() {
        // oneOf is too ambiguous to coerce safely — leave the value alone.
        let v = coerce(
            json!({ "x": "5" }),
            json!({
                "type": "object",
                "properties": {
                    "x": {
                        "oneOf": [
                            { "type": "integer" },
                            { "type": "string" }
                        ]
                    }
                }
            }),
        );
        assert_eq!(v, json!({ "x": "5" }));
    }

    #[test]
    fn allof_refines_into_integer() {
        let v = coerce(
            json!({ "x": "12" }),
            json!({
                "type": "object",
                "properties": {
                    "x": { "allOf": [{ "type": "integer" }, { "minimum": 0 }] }
                }
            }),
        );
        assert_eq!(v, json!({ "x": 12 }));
    }

    #[test]
    fn handles_additional_properties_schema() {
        let v = coerce(
            json!({ "headers": { "retry": "5" } }),
            json!({
                "type": "object",
                "properties": {
                    "headers": {
                        "type": "object",
                        "additionalProperties": { "type": "integer" }
                    }
                }
            }),
        );
        assert_eq!(v, json!({ "headers": { "retry": 5 } }));
    }

    #[test]
    fn negative_integers_coerce() {
        let v = coerce(
            json!({ "n": "-42" }),
            json!({
                "type": "object",
                "properties": { "n": { "type": "integer" } }
            }),
        );
        assert_eq!(v, json!({ "n": -42 }));
    }

    #[test]
    fn empty_string_does_not_coerce() {
        let v = coerce(
            json!({ "n": "" }),
            json!({
                "type": "object",
                "properties": { "n": { "type": "integer" } }
            }),
        );
        // empty string fails str::parse -> stays as string, validator will reject
        assert_eq!(v, json!({ "n": "" }));
    }

    #[test]
    fn whitespace_padding_does_not_coerce() {
        // " 42 " is rejected by str::parse — keep strict.
        let v = coerce(
            json!({ "n": " 42 " }),
            json!({
                "type": "object",
                "properties": { "n": { "type": "integer" } }
            }),
        );
        assert_eq!(v, json!({ "n": " 42 " }));
    }
}
