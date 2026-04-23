use serde_json::Value;

/// Flatten a schemars-generated JSON Schema into a simplified form that
/// OpenAI-compatible APIs (including Fireworks) can handle.
///
/// This resolves `$ref` references by inlining definitions, removes
/// schemars-specific keys (`$schema`, `title`, `definitions`), and
/// simplifies enum patterns.
pub(super) fn flatten_schema(schema: &mut Value) {
    // Extract definitions first, then inline all $refs
    let defs = extract_definitions(schema);
    if !defs.is_empty() {
        inline_refs(schema, &defs);
    }
    // Remove schemars-specific top-level keys
    if let Value::Object(obj) = schema {
        obj.remove("$schema");
        obj.remove("title");
        obj.remove("definitions");
        obj.remove("$defs");
    }
    // Simplify oneOf/anyOf/allOf enum patterns throughout
    simplify_enums(schema);
    // Ensure every property node has a valid `type` — required by Gemini's API
    ensure_types(schema);
}

/// Extract `definitions` or `$defs` from the schema root.
fn extract_definitions(schema: &Value) -> serde_json::Map<String, Value> {
    if let Value::Object(obj) = schema {
        for key in &["definitions", "$defs"] {
            if let Some(Value::Object(defs)) = obj.get(*key) {
                return defs.clone();
            }
        }
    }
    serde_json::Map::new()
}

/// Recursively resolve `$ref` references by inlining the referenced definition.
fn inline_refs(value: &mut Value, defs: &serde_json::Map<String, Value>) {
    match value {
        Value::Object(obj) => {
            // Check if this object is a $ref
            if let Some(Value::String(ref_path)) = obj.get("$ref") {
                // Parse "#/definitions/Foo" or "#/$defs/Foo"
                let def_name = ref_path
                    .strip_prefix("#/definitions/")
                    .or_else(|| ref_path.strip_prefix("#/$defs/"));
                if let Some(name) = def_name {
                    if let Some(def) = defs.get(name) {
                        *value = def.clone();
                        // Recurse into the inlined definition (it may have nested refs)
                        inline_refs(value, defs);
                        return;
                    }
                }
            }
            // Recurse into all values
            for v in obj.values_mut() {
                inline_refs(v, defs);
            }
        }
        Value::Array(arr) => {
            for v in arr.iter_mut() {
                inline_refs(v, defs);
            }
        }
        _ => {}
    }
}

/// Simplify schemars enum patterns:
/// - `oneOf: [{enum: ["a"], type: "string"}, ...]` → `{type: "string", enum: ["a", ...]}`
/// - `anyOf: [{$ref: ...}, {type: "null"}]` → the inlined ref (already nullable via optional)
/// - `allOf: [{$ref: ...}]` → the inlined ref (single-item allOf)
fn simplify_enums(value: &mut Value) {
    match value {
        Value::Object(obj) => {
            // Simplify oneOf string enums
            if let Some(Value::Array(variants)) = obj.remove("oneOf") {
                let mut enum_values = Vec::new();
                let mut all_string_enums = true;
                let mut description = None;

                for variant in &variants {
                    if let Value::Object(v) = variant {
                        if v.get("type") == Some(&Value::String("string".to_string())) {
                            if let Some(Value::Array(vals)) = v.get("enum") {
                                enum_values.extend(vals.clone());
                                if description.is_none() {
                                    description = v.get("description").cloned();
                                }
                                continue;
                            }
                        }
                    }
                    all_string_enums = false;
                    break;
                }

                if all_string_enums && !enum_values.is_empty() {
                    obj.insert("type".to_string(), Value::String("string".to_string()));
                    obj.insert("enum".to_string(), Value::Array(enum_values));
                } else {
                    // Can't simplify — put it back
                    obj.insert("oneOf".to_string(), Value::Array(variants));
                }
            }

            // Simplify anyOf: [{...}, {type: "null"}] → just the non-null variant
            if let Some(Value::Array(variants)) = obj.remove("anyOf") {
                let non_null: Vec<_> = variants
                    .into_iter()
                    .filter(|v| {
                        v.as_object()
                            .map(|o| o.get("type") != Some(&Value::String("null".to_string())))
                            .unwrap_or(true)
                    })
                    .collect();

                if non_null.len() == 1 {
                    // Merge the single non-null variant into this object
                    if let Value::Object(inner) = &non_null[0] {
                        for (k, v) in inner {
                            obj.entry(k.clone()).or_insert(v.clone());
                        }
                    }
                } else {
                    obj.insert("anyOf".to_string(), Value::Array(non_null));
                }
            }

            // Simplify allOf with a single item
            if let Some(Value::Array(items)) = obj.remove("allOf") {
                if items.len() == 1 {
                    if let Value::Object(inner) = &items[0] {
                        for (k, v) in inner {
                            obj.entry(k.clone()).or_insert(v.clone());
                        }
                    }
                } else {
                    obj.insert("allOf".to_string(), Value::Array(items));
                }
            }

            // Recurse into all remaining values
            for v in obj.values_mut() {
                simplify_enums(v);
            }
        }
        Value::Array(arr) => {
            for v in arr.iter_mut() {
                simplify_enums(v);
            }
        }
        _ => {}
    }
}

/// Recursively ensure every schema node has a valid `type` field.
/// Gemini's API rejects schemas with missing or empty-string types.
fn ensure_types(value: &mut Value) {
    ensure_types_inner(value, false);
}

fn ensure_types_inner(value: &mut Value, is_schema_position: bool) {
    match value {
        Value::Object(obj) => {
            // Fix empty-string type
            if obj.get("type") == Some(&Value::String(String::new())) {
                obj.remove("type");
            }

            // Infer type from structure if missing
            if !obj.contains_key("type") {
                if obj.contains_key("properties") {
                    obj.insert("type".to_string(), Value::String("object".to_string()));
                } else if obj.contains_key("items") {
                    obj.insert("type".to_string(), Value::String("array".to_string()));
                } else if obj.contains_key("enum") {
                    obj.insert("type".to_string(), Value::String("string".to_string()));
                } else if is_schema_position {
                    // A leaf node in a schema position (under `properties` or `items`)
                    // with no type — default to string.
                    obj.insert("type".to_string(), Value::String("string".to_string()));
                }
            }

            // Recurse into `properties` values — each is a schema position
            if let Some(Value::Object(props)) = obj.get_mut("properties") {
                for v in props.values_mut() {
                    ensure_types_inner(v, true);
                }
            }

            // `items` is a schema position
            if let Some(items) = obj.get_mut("items") {
                ensure_types_inner(items, true);
            }

            // Recurse into other values (non-schema positions)
            for (key, v) in obj.iter_mut() {
                if key != "properties" && key != "items" {
                    ensure_types_inner(v, false);
                }
            }
        }
        Value::Array(arr) => {
            for v in arr.iter_mut() {
                ensure_types_inner(v, is_schema_position);
            }
        }
        _ => {}
    }
}
