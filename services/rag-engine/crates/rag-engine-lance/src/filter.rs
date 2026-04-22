//! Injection-safe SQL filter builder.
//!
//! LanceDB's SQL filter does NOT support bound parameters, so we emit
//! pre-validated literal strings. All callers pass through the helpers
//! below; no user input ever reaches LanceDB via string concatenation.
//!
//! The rules:
//!   * string literals are escaped by doubling single quotes (SQL-92)
//!   * identifiers come from a closed, compile-time set (`crate::schema::col`)
//!   * ACL tokens are treated as opaque strings — we do not interpret them,
//!     but we DO escape them.
//!
//! Phase 2 ignores the client-supplied `custom_sql_filter` field; the
//! server layer rejects it with `InvalidArgument` before we see it.

/// Escape a string literal for inclusion inside a single-quoted SQL literal.
pub fn escape_sql_str(s: &str) -> String {
    s.replace('\'', "''")
}

/// Emit `col = 'literal'`.
pub fn eq_str(column: &str, value: &str) -> String {
    format!("{column} = '{}'", escape_sql_str(value))
}

/// Emit `array_has(col, 'literal')`.
pub fn array_has(column: &str, value: &str) -> String {
    format!("array_has({column}, '{}')", escape_sql_str(value))
}

/// Emit `array_has_any(col, [lit1, lit2, ...])`.
/// Returns `None` if `values` is empty.
pub fn array_has_any(column: &str, values: &[String]) -> Option<String> {
    if values.is_empty() {
        return None;
    }
    let items = values
        .iter()
        .map(|v| format!("'{}'", escape_sql_str(v)))
        .collect::<Vec<_>>()
        .join(", ");
    Some(format!("array_has_any({column}, [{items}])"))
}

/// Emit `col IN ('a', 'b', ...)`. Returns `None` if `values` is empty.
pub fn in_list(column: &str, values: &[String]) -> Option<String> {
    if values.is_empty() {
        return None;
    }
    let items = values
        .iter()
        .map(|v| format!("'{}'", escape_sql_str(v)))
        .collect::<Vec<_>>()
        .join(", ");
    Some(format!("{column} IN ({items})"))
}

/// Combine multiple clauses with `AND`. Empty list returns `None`.
pub fn and_all(parts: Vec<String>) -> Option<String> {
    if parts.is_empty() {
        None
    } else if parts.len() == 1 {
        Some(parts.into_iter().next().unwrap())
    } else {
        Some(
            parts
                .into_iter()
                .map(|p| format!("({p})"))
                .collect::<Vec<_>>()
                .join(" AND "),
        )
    }
}

/// Combine with `OR`.
pub fn or_all(parts: Vec<String>) -> Option<String> {
    if parts.is_empty() {
        None
    } else if parts.len() == 1 {
        Some(parts.into_iter().next().unwrap())
    } else {
        Some(
            parts
                .into_iter()
                .map(|p| format!("({p})"))
                .collect::<Vec<_>>()
                .join(" OR "),
        )
    }
}
