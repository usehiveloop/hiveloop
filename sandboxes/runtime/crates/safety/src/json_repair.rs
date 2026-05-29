use regex::Regex;
use serde_json::{json, Value};

#[derive(Debug, Clone)]
pub struct JsonRepair {
    trailing_comma_re: Regex,
    single_quote_string_re: Regex,
    unquoted_key_re: Regex,
}

impl Default for JsonRepair {
    fn default() -> Self {
        Self::new()
    }
}

impl JsonRepair {
    pub fn new() -> Self {
        Self {
            trailing_comma_re: Regex::new(r",\s*([}\]])").unwrap(),
            single_quote_string_re: Regex::new(r"'([^']*)'").unwrap(),
            unquoted_key_re: Regex::new(r#"([{,]\s*)([a-zA-Z_]\w*)(\s*:)"#).unwrap(),
        }
    }

    pub fn repair(&self, raw: &str) -> (Value, bool) {
        let trimmed = raw.trim();

        if trimmed.is_empty() {
            return (json!({}), false);
        }

        if let Ok(value) = serde_json::from_str::<Value>(trimmed) {
            if value.is_object() || value.is_array() {
                return (value, false);
            }
        }

        let extracted = self.extract_json_object(trimmed);
        if let Ok(value) = serde_json::from_str::<Value>(&extracted) {
            return (value, true);
        }

        let fixed = self.basic_repairs(&extracted);
        if let Ok(value) = serde_json::from_str::<Value>(&fixed) {
            return (value, true);
        }

        let balanced = self.balance_braces(&fixed);
        if let Ok(value) = serde_json::from_str::<Value>(&balanced) {
            return (value, true);
        }

        let closed = self.close_unterminated_strings(&balanced);
        if let Ok(value) = serde_json::from_str::<Value>(&closed) {
            return (value, true);
        }

        tracing::warn!(
            raw = %trimmed,
            "unable to repair malformed JSON arguments, returning empty object"
        );
        (json!({}), true)
    }

    fn extract_json_object(&self, raw: &str) -> String {
        if let Some(start) = raw.find('{') {
            if let Some(end) = raw.rfind('}') {
                if start < end {
                    return raw[start..=end].to_string();
                }
            }
            return raw[start..].to_string();
        }
        if let Some(start) = raw.find('[') {
            if let Some(end) = raw.rfind(']') {
                if start < end {
                    return raw[start..=end].to_string();
                }
            }
            return raw[start..].to_string();
        }
        raw.to_string()
    }

    fn basic_repairs(&self, json_str: &str) -> String {
        let mut fixed = json_str.to_string();

        fixed = self.trailing_comma_re.replace_all(&fixed, "$1").to_string();

        fixed = self.fix_single_quotes(&fixed);

        fixed = self
            .unquoted_key_re
            .replace_all(&fixed, r#"$1"$2"$3"#)
            .to_string();

        fixed = fixed.replace("None", "null");
        fixed = fixed.replace("True", "true");
        fixed = fixed.replace("False", "false");

        if let Some(last_brace) = fixed.rfind('}') {
            fixed = fixed[..=last_brace].to_string();
        }

        fixed
    }

    fn fix_single_quotes(&self, json_str: &str) -> String {
        let mut result = String::new();
        let mut last_end = 0;

        for cap in self.single_quote_string_re.captures_iter(json_str) {
            let m = cap.get(0).unwrap();
            result.push_str(&json_str[last_end..m.start()]);
            result.push('"');
            result.push_str(&cap[1]);
            result.push('"');
            last_end = m.end();
        }
        result.push_str(&json_str[last_end..]);
        result
    }

    fn balance_braces(&self, json_str: &str) -> String {
        let mut open_braces: i32 = 0;
        let mut in_string = false;
        let mut escaped = false;

        for ch in json_str.chars() {
            match ch {
                '"' if !escaped => in_string = !in_string,
                '{' if !in_string => open_braces += 1,
                '}' if !in_string => open_braces -= 1,
                _ => {}
            }
            escaped = ch == '\\' && !escaped;
        }

        let mut balanced = json_str.to_string();
        for _ in 0..open_braces.max(0) {
            balanced.push('}');
        }
        balanced
    }

    fn close_unterminated_strings(&self, json_str: &str) -> String {
        let mut in_string = false;
        let mut escaped = false;

        for ch in json_str.chars() {
            match ch {
                '"' if !escaped => in_string = !in_string,
                _ => {}
            }
            escaped = ch == '\\' && !escaped;
        }

        if in_string {
            let mut closed = json_str.to_string();
            closed.push('"');
            closed
        } else {
            json_str.to_string()
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn valid_json_passes_through() {
        let repair = JsonRepair::new();
        let (result, was_repaired) = repair.repair(r#"{"key": "value"}"#);
        assert_eq!(result["key"], "value");
        assert!(!was_repaired);
    }

    #[test]
    fn fixes_trailing_comma() {
        let repair = JsonRepair::new();
        let (result, was_repaired) = repair.repair(r#"{"key": "value",}"#);
        assert_eq!(result["key"], "value");
        assert!(was_repaired);
    }

    #[test]
    fn fixes_single_quotes_to_double() {
        let repair = JsonRepair::new();
        let (result, was_repaired) = repair.repair("{'key': 'value'}");
        assert_eq!(result["key"], "value");
        assert!(was_repaired);
    }

    #[test]
    fn fixes_unquoted_keys() {
        let repair = JsonRepair::new();
        let (result, was_repaired) = repair.repair(r#"{key: "value"}"#);
        assert_eq!(result["key"], "value");
        assert!(was_repaired);
    }

    #[test]
    fn fixes_python_none_to_null() {
        let repair = JsonRepair::new();
        let (result, was_repaired) = repair.repair(r#"{"key": None}"#);
        assert_eq!(result["key"], serde_json::Value::Null);
        assert!(was_repaired);
    }

    #[test]
    fn fixes_python_bool_to_json_bool() {
        let repair = JsonRepair::new();
        let (result, was_repaired) = repair.repair(r#"{"a": True, "b": False}"#);
        assert_eq!(result["a"], true);
        assert_eq!(result["b"], false);
        assert!(was_repaired);
    }

    #[test]
    fn balances_truncated_braces() {
        let repair = JsonRepair::new();
        let (result, was_repaired) = repair.repair(r#"{"path": "/tmp/file""#);
        assert_eq!(result["path"], "/tmp/file");
        assert!(was_repaired);
    }

    #[test]
    fn extracts_json_from_prose() {
        let repair = JsonRepair::new();
        let (result, was_repaired) =
            repair.repair(r#"Here is the result: {"path": "/tmp/file.txt"} Hope this helps!"#);
        assert_eq!(result["path"], "/tmp/file.txt");
        assert!(was_repaired);
    }

    #[test]
    fn handles_completely_malformed_input() {
        let repair = JsonRepair::new();
        let (result, was_repaired) = repair.repair("not json at all");
        assert_eq!(result, json!({}));
        assert!(was_repaired);
    }

    #[test]
    fn handles_empty_input() {
        let repair = JsonRepair::new();
        let (result, was_repaired) = repair.repair("");
        assert_eq!(result, json!({}));
        assert!(!was_repaired);
    }

    #[test]
    fn fixes_multiple_issues() {
        let repair = JsonRepair::new();
        let (result, was_repaired) =
            repair.repair(r#"{command: 'ls -la', 'timeout_seconds': 30, env: {'HOME': '/root'},}"#);
        assert_eq!(result["command"], "ls -la");
        assert_eq!(result["timeout_seconds"], 30);
        assert_eq!(result["env"]["HOME"], "/root");
        assert!(was_repaired);
    }
}
