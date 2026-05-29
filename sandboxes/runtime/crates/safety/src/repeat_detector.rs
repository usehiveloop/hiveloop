use domain::RepeatDetectionConfig;
use std::hash::{Hash, Hasher};

pub struct RepeatToolCallDetector {
    history: Vec<(String, u64)>,
    config: RepeatDetectionConfig,
}

impl RepeatToolCallDetector {
    pub fn new(config: RepeatDetectionConfig) -> Self {
        Self {
            history: Vec::new(),
            config,
        }
    }

    pub fn check(&mut self, tool_name: &str, args: &serde_json::Value) -> Option<String> {
        let args_hash = hash_json_value(args);

        let consecutive = self
            .history
            .iter()
            .rev()
            .take_while(|(name, hash)| name == tool_name && *hash == args_hash)
            .count();

        if consecutive + 1 > self.config.max_consecutive {
            return Some(format!(
                "You have called '{tool_name}' {times} times consecutively with identical arguments. \
                 This is not productive. Re-examine the tool results and try a different approach. \
                 If you need help understanding how to use '{tool_name}' correctly, re-read its \
                 description and use different arguments.",
                times = consecutive + 1
            ));
        }

        let total = self
            .history
            .iter()
            .filter(|(name, hash)| name == tool_name && *hash == args_hash)
            .count();

        if total + 1 > self.config.max_total {
            return Some(format!(
                "You have called '{tool_name}' {times} times total with identical arguments. \
                 The results will not change. Use different arguments, try a different tool, \
                 or explain to the user why the task cannot be completed with the available tools.",
                times = total + 1
            ));
        }

        self.history.push((tool_name.to_string(), args_hash));
        None
    }

    pub fn len(&self) -> usize {
        self.history.len()
    }

    pub fn is_empty(&self) -> bool {
        self.history.is_empty()
    }
}

fn hash_json_value(value: &serde_json::Value) -> u64 {
    let mut hasher = std::collections::hash_map::DefaultHasher::new();
    value.to_string().hash(&mut hasher);
    hasher.finish()
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn rejects_consecutive_identical_calls() {
        let mut detector = RepeatToolCallDetector::new(RepeatDetectionConfig::default());
        let args = json!({"command": "ls"});

        assert!(detector.check("bash", &args).is_none());
        assert!(detector.check("bash", &args).is_none());
        assert!(detector.check("bash", &args).is_none());

        let result = detector.check("bash", &args);
        assert!(result.is_some());
        assert!(result.unwrap().contains("4 times consecutively"));
    }

    #[test]
    fn resets_consecutive_count_on_different_tool() {
        let mut detector = RepeatToolCallDetector::new(RepeatDetectionConfig::default());
        let args = json!({"command": "ls"});

        assert!(detector.check("bash", &args).is_none());
        assert!(detector.check("bash", &args).is_none());
        assert!(detector.check("read_file", &args).is_none());
        assert!(detector.check("bash", &args).is_none());
    }

    #[test]
    fn rejects_total_identical_calls() {
        let config = RepeatDetectionConfig {
            enabled: true,
            max_consecutive: 10,
            max_total: 2,
        };
        let mut detector = RepeatToolCallDetector::new(config);
        let args = json!({"command": "ls"});

        detector.check("bash", &args);
        detector.check("read_file", &json!({"path": "/tmp"}));
        detector.check("bash", &args);

        let result = detector.check("bash", &args);
        assert!(result.is_some());
        assert!(result.unwrap().contains("3 times total"));
    }

    #[test]
    fn different_arguments_reset_consecutive_count() {
        let mut detector = RepeatToolCallDetector::new(RepeatDetectionConfig::default());

        assert!(detector.check("bash", &json!({"command": "ls"})).is_none());
        assert!(detector.check("bash", &json!({"command": "ls"})).is_none());
        assert!(detector.check("bash", &json!({"command": "pwd"})).is_none());
        assert!(detector.check("bash", &json!({"command": "ls"})).is_none());
    }
}
