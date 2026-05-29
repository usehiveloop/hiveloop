use regex::Regex;
use serde_json::{json, Value};

#[derive(Clone)]
pub struct XmlToolCallRepair {
    tool_call_re: Regex,
    invoke_re: Regex,
    param_re: Regex,
    named_param_re: Regex,
    tag_pair_re: Regex,
}

impl Default for XmlToolCallRepair {
    fn default() -> Self {
        Self::new()
    }
}

impl XmlToolCallRepair {
    pub fn new() -> Self {
        Self {
            tool_call_re: Regex::new(
                r"(?is)<tool_call\s*>\s*<function[=>]\s*(\w+)\s*>(.*?)</function\s*>\s*</tool_call\s*>",
            )
            .unwrap(),
            invoke_re: Regex::new(
                r#"(?is)<invoke\s+name\s*=\s*"(\w+)"\s*>(.*?)</invoke\s*>"#,
            )
            .unwrap(),
            param_re: Regex::new(
                r"(?is)<parameter[=>]\s*(\w+)\s*>(.*?)</parameter\s*>",
            )
            .unwrap(),
            named_param_re: Regex::new(
                r#"(?is)<parameter\s+name\s*=\s*"(\w+)"\s*>(.*?)</parameter\s*>"#,
            )
            .unwrap(),
            tag_pair_re: Regex::new(
                r"(?is)<(\w+)>(.*?)</\w+>",
            )
            .unwrap(),
        }
    }

    pub fn is_xml_tool_call(&self, content: &str) -> bool {
        self.tool_call_re.is_match(content) || self.invoke_re.is_match(content)
    }

    pub fn try_extract_tool_calls(
        &self,
        content: &str,
        known_tools: &[String],
    ) -> (String, Vec<ExtractedToolCall>) {
        let mut calls = Vec::new();
        let mut cleaned = content.to_string();

        for cap in self.tool_call_re.captures_iter(content) {
            let name = cap[1].to_string();
            let inner = cap[2].to_string();
            if let Some(call) = self.parse_parameters(&name, &inner) {
                cleaned = cleaned.replace(&cap[0], "");
                calls.push(call);
            }
        }

        for cap in self.invoke_re.captures_iter(content) {
            let name = cap[1].to_string();
            let inner = cap[2].to_string();
            if let Some(call) = self.parse_named_parameters(&name, &inner) {
                cleaned = cleaned.replace(&cap[0], "");
                calls.push(call);
            }
        }

        for tool_name in known_tools {
            let escaped_name = regex::escape(tool_name);
            let pattern = format!(r"(?is)<{escaped_name}\s*>(.*?)</{escaped_name}\s*>");
            let Ok(tool_re) = Regex::new(&pattern) else {
                continue;
            };

            let captures: Vec<_> = tool_re
                .captures_iter(&cleaned)
                .map(|cap| (cap[0].to_string(), cap[1].to_string()))
                .collect();

            for (full_match, inner) in captures {
                if let Some(call) = self.parse_parameters(tool_name, &inner) {
                    cleaned = cleaned.replace(&full_match, "");
                    calls.push(call);
                }
            }
        }

        cleaned = self.cleanup_empty_wrappers(&cleaned);

        (cleaned.trim().to_string(), calls)
    }

    fn parse_parameters(&self, name: &str, inner: &str) -> Option<ExtractedToolCall> {
        let mut args = serde_json::Map::new();

        for cap in self.param_re.captures_iter(inner) {
            let key = cap[1].to_string();
            let value = cap[2].trim().to_string();
            args.insert(key, Value::String(value));
        }

        if args.is_empty() {
            let tag_pairs = self.parse_tag_pairs(inner);
            if !tag_pairs.is_empty() {
                for (key, value) in tag_pairs {
                    args.insert(key, Value::String(value));
                }
            }
        }

        if args.is_empty() && !inner.trim().is_empty() {
            args.insert(
                "command".to_string(),
                Value::String(inner.trim().to_string()),
            );
        }

        if !args.is_empty() {
            Some(ExtractedToolCall {
                name: name.to_string(),
                id: format!("xml_{}", name),
                arguments: Value::Object(args),
            })
        } else {
            None
        }
    }

    fn parse_tag_pairs(&self, inner: &str) -> Vec<(String, String)> {
        let mut pairs = Vec::new();
        for cap in self.tag_pair_re.captures_iter(inner) {
            let key = cap[1].to_string();
            let value = cap[2].trim().to_string();
            pairs.push((key, value));
        }
        pairs
    }

    fn cleanup_empty_wrappers(&self, text: &str) -> String {
        let empty_tag_re = Regex::new(r"(?is)<\w+>\s*</\w+>").unwrap();
        let result = empty_tag_re.replace_all(text, "");
        result.trim().to_string()
    }

    fn parse_named_parameters(&self, name: &str, inner: &str) -> Option<ExtractedToolCall> {
        let mut args = serde_json::Map::new();

        for cap in self.named_param_re.captures_iter(inner) {
            let key = cap[1].to_string();
            let value = cap[2].trim().to_string();
            args.insert(key, Value::String(value));
        }

        if !args.is_empty() {
            Some(ExtractedToolCall {
                name: name.to_string(),
                id: format!("xml_{}", name),
                arguments: Value::Object(args),
            })
        } else {
            None
        }
    }
}

#[derive(Debug, Clone, PartialEq)]
pub struct ExtractedToolCall {
    pub name: String,
    pub id: String,
    pub arguments: Value,
}

impl ExtractedToolCall {
    pub fn to_json_tool_call(&self) -> Value {
        json!({
            "id": self.id,
            "type": "function",
            "function": {
                "name": self.name,
                "arguments": self.arguments.to_string(),
            }
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn known_tools() -> Vec<String> {
        vec![
            "bash".to_string(),
            "read_file".to_string(),
            "write_file".to_string(),
            "edit_file".to_string(),
        ]
    }

    #[test]
    fn detects_tool_call_xml() {
        let repair = XmlToolCallRepair::new();
        let content = "<tool_call><function=bash><parameter=command>ls -la</parameter><parameter=timeout_seconds>120</parameter></function></tool_call>";
        assert!(repair.is_xml_tool_call(content));
    }

    #[test]
    fn detects_invoke_xml() {
        let repair = XmlToolCallRepair::new();
        let content = r#"<function_calls><invoke name="read_file"><parameter name="path">/tmp/test</parameter></invoke></function_calls>"#;
        assert!(repair.is_xml_tool_call(content));
    }

    #[test]
    fn extracts_tool_call_block() {
        let repair = XmlToolCallRepair::new();
        let content = "text before <tool_call><function=bash><parameter=command>ls -la</parameter></function></tool_call> text after";
        let (cleaned, calls) = repair.try_extract_tool_calls(content, &known_tools());

        assert_eq!(calls.len(), 1);
        assert_eq!(calls[0].name, "bash");
        assert_eq!(calls[0].arguments["command"], "ls -la");
        assert!(cleaned.contains("text before"));
        assert!(cleaned.contains("text after"));
        assert!(!cleaned.contains("tool_call"));
    }

    #[test]
    fn extracts_invoke_block() {
        let repair = XmlToolCallRepair::new();
        let content = r#"<function_calls><invoke name="read_file"><parameter name="path">/tmp/file.txt</parameter></invoke></function_calls>"#;
        let (cleaned, calls) = repair.try_extract_tool_calls(content, &known_tools());

        assert_eq!(calls.len(), 1);
        assert_eq!(calls[0].name, "read_file");
        assert_eq!(calls[0].arguments["path"], "/tmp/file.txt");
        assert!(cleaned.is_empty());
    }

    #[test]
    fn extracts_direct_tool_tag() {
        let repair = XmlToolCallRepair::new();
        let content =
            "<bash><command>echo hello</command><timeout_seconds>30</timeout_seconds></bash>";
        let (cleaned, calls) = repair.try_extract_tool_calls(content, &known_tools());

        assert_eq!(calls.len(), 1);
        assert_eq!(calls[0].name, "bash");
        assert_eq!(calls[0].arguments["command"], "echo hello");
        assert_eq!(calls[0].arguments["timeout_seconds"], "30");
        assert!(cleaned.is_empty());
    }

    #[test]
    fn does_not_match_unknown_tool_in_direct_tag() {
        let repair = XmlToolCallRepair::new();
        let content = "<unknown_tag>some content</unknown_tag>";
        let (cleaned, calls) = repair.try_extract_tool_calls(content, &known_tools());

        assert!(calls.is_empty());
        assert_eq!(cleaned, "<unknown_tag>some content</unknown_tag>");
    }

    #[test]
    fn normal_text_not_detected() {
        let repair = XmlToolCallRepair::new();
        let content = "I will use the bash tool to list files.\nls -la";
        assert!(!repair.is_xml_tool_call(content));
        let (cleaned, calls) = repair.try_extract_tool_calls(content, &known_tools());
        assert!(calls.is_empty());
        assert_eq!(cleaned, content);
    }

    #[test]
    fn html_tags_not_false_positive() {
        let repair = XmlToolCallRepair::new();
        let content = "<div class='code'>print('hello')</div>";
        assert!(!repair.is_xml_tool_call(content));
    }

    #[test]
    fn fallback_command_when_no_parameters() {
        let repair = XmlToolCallRepair::new();
        let content = "<tool_call><function=bash>ls -la /tmp</function></tool_call>";
        let (_cleaned, calls) = repair.try_extract_tool_calls(content, &known_tools());

        assert_eq!(calls.len(), 1);
        assert_eq!(calls[0].name, "bash");
        assert_eq!(calls[0].arguments["command"], "ls -la /tmp");
    }
}
