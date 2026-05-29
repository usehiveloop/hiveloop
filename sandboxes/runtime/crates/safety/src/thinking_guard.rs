use domain::OverthinkingConfig;
use regex::Regex;
use std::time::Instant;

#[derive(Clone)]
pub struct ThinkingGuard {
    think_tag_re: Regex,
    orphan_closing_re: Regex,
}

impl Default for ThinkingGuard {
    fn default() -> Self {
        Self::new()
    }
}

impl ThinkingGuard {
    pub fn new() -> Self {
        Self {
            think_tag_re: Regex::new(r"(?is)<\s*think\s*>.*?<\s*/\s*think\s*>").unwrap(),
            orphan_closing_re: Regex::new(r"(?i)<\s*/\s*think\s*>").unwrap(),
        }
    }

    pub fn strip_thinking(&self, text: &str) -> (String, bool) {
        let mut stripped = text.to_string();
        let mut had_tags = false;

        let cleaned = self.think_tag_re.replace_all(text, "");
        if cleaned != text {
            had_tags = true;
            stripped = cleaned.to_string();
        }

        if let Some(m) = self.orphan_closing_re.find(&stripped) {
            had_tags = true;
            stripped = stripped[m.end()..].to_string();
        }

        (stripped.trim().to_string(), had_tags)
    }

    pub fn extract_thinking_content(&self, text: &str) -> Option<String> {
        let captures: Vec<_> = self.think_tag_re.find_iter(text).collect();
        if captures.is_empty() {
            return None;
        }

        let thinking: Vec<&str> = captures.iter().map(|m| m.as_str()).collect();
        Some(thinking.join("\n"))
    }
}

pub struct OverthinkingDetector {
    thinking_start: Option<Instant>,
    thinking_token_count: u64,
    last_content_change_at: u64,
    last_content_hash: u64,
    config: OverthinkingConfig,
}

impl OverthinkingDetector {
    pub fn new(config: OverthinkingConfig) -> Self {
        Self {
            thinking_start: None,
            thinking_token_count: 0,
            last_content_change_at: 0,
            last_content_hash: 0,
            config,
        }
    }

    pub fn feed(&mut self, thinking_token: &str) -> OverthinkingStatus {
        if self.thinking_start.is_none() {
            self.thinking_start = Some(Instant::now());
        }

        self.thinking_token_count += 1;

        if let Some(start) = self.thinking_start {
            let elapsed = start.elapsed().as_secs();
            if elapsed >= self.config.max_duration_secs {
                return OverthinkingStatus::TimeLimitExceeded {
                    duration_secs: elapsed,
                    tokens: self.thinking_token_count,
                };
            }
        }

        if self.thinking_token_count >= self.config.max_tokens {
            return OverthinkingStatus::TokenLimitExceeded {
                tokens: self.thinking_token_count,
            };
        }

        let hash = hash_tail(thinking_token, 50);
        if hash != self.last_content_hash {
            self.last_content_hash = hash;
            self.last_content_change_at = self.thinking_token_count;
        } else {
            let stalled_for = self.thinking_token_count - self.last_content_change_at;
            if stalled_for >= self.config.stall_threshold {
                return OverthinkingStatus::Stalled { stalled_for };
            }
        }

        OverthinkingStatus::Ok
    }

    pub fn reset(&mut self) {
        self.thinking_start = None;
        self.thinking_token_count = 0;
        self.last_content_change_at = 0;
        self.last_content_hash = 0;
    }
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum OverthinkingStatus {
    Ok,
    TimeLimitExceeded { duration_secs: u64, tokens: u64 },
    TokenLimitExceeded { tokens: u64 },
    Stalled { stalled_for: u64 },
}

impl OverthinkingStatus {
    pub fn is_overthinking(&self) -> bool {
        !matches!(self, Self::Ok)
    }

    pub fn reason(&self) -> String {
        match self {
            Self::Ok => String::new(),
            Self::TimeLimitExceeded {
                duration_secs,
                tokens,
            } => format!("Thinking exceeded {duration_secs}s time limit ({tokens} tokens emitted)"),
            Self::TokenLimitExceeded { tokens } => {
                format!("Thinking exceeded {tokens} token limit")
            }
            Self::Stalled { stalled_for } => {
                format!("Thinking stalled — {stalled_for} tokens without new content")
            }
        }
    }
}

fn hash_tail(text: &str, n: usize) -> u64 {
    use std::hash::{Hash, Hasher};
    let tail = if text.len() > n {
        &text[text.len() - n..]
    } else {
        text
    };
    let mut hasher = std::collections::hash_map::DefaultHasher::new();
    tail.hash(&mut hasher);
    hasher.finish()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn strips_complete_think_block() {
        let guard = ThinkingGuard::new();
        let (result, had_tags) = guard.strip_thinking("<think>step 1\nstep 2</think>final answer");
        assert_eq!(result, "final answer");
        assert!(had_tags);
    }

    #[test]
    fn handles_whitespace_in_tags() {
        let guard = ThinkingGuard::new();
        let (result, had_tags) = guard.strip_thinking("< think >reasoning< / think >actual output");
        assert_eq!(result, "actual output");
        assert!(had_tags);
    }

    #[test]
    fn strips_orphan_closing_tag() {
        let guard = ThinkingGuard::new();
        let (result, had_tags) = guard.strip_thinking("stale reasoning</think>final answer");
        assert_eq!(result, "final answer");
        assert!(had_tags);
    }

    #[test]
    fn passes_clean_text_unchanged() {
        let guard = ThinkingGuard::new();
        let (result, had_tags) = guard.strip_thinking("regular output without tags");
        assert_eq!(result, "regular output without tags");
        assert!(!had_tags);
    }

    #[test]
    fn extracts_thinking_content() {
        let guard = ThinkingGuard::new();
        let thinking = guard.extract_thinking_content("<think>step 1\nstep 2</think>final");
        assert_eq!(thinking, Some("<think>step 1\nstep 2</think>".to_string()));
    }

    #[test]
    fn returns_none_when_no_think_tags() {
        let guard = ThinkingGuard::new();
        let thinking = guard.extract_thinking_content("plain text");
        assert_eq!(thinking, None);
    }

    #[test]
    fn overthinking_detector_time_limit() {
        let config = OverthinkingConfig {
            enabled: true,
            max_duration_secs: 0,
            max_tokens: 10000,
            stall_threshold: 500,
        };
        let mut detector = OverthinkingDetector::new(config);
        let status = detector.feed("test");
        assert_eq!(
            status,
            OverthinkingStatus::TimeLimitExceeded {
                duration_secs: 0,
                tokens: 1
            }
        );
    }

    #[test]
    fn overthinking_detector_token_limit() {
        let config = OverthinkingConfig {
            enabled: true,
            max_duration_secs: 999,
            max_tokens: 2,
            stall_threshold: 500,
        };
        let mut detector = OverthinkingDetector::new(config);
        assert_eq!(detector.feed("a"), OverthinkingStatus::Ok);
        assert_eq!(
            detector.feed("b"),
            OverthinkingStatus::TokenLimitExceeded { tokens: 2 }
        );
    }

    #[test]
    fn overthinking_detector_stall_detection() {
        let config = OverthinkingConfig {
            enabled: true,
            max_duration_secs: 999,
            max_tokens: 10000,
            stall_threshold: 3,
        };
        let mut detector = OverthinkingDetector::new(config);

        detector.feed("unique content 1");
        detector.feed("unique content 2");
        detector.feed("unique content 3");

        assert_eq!(detector.feed("same"), OverthinkingStatus::Ok);
        assert_eq!(detector.feed("same"), OverthinkingStatus::Ok);
        assert_eq!(detector.feed("same"), OverthinkingStatus::Ok);
        assert_eq!(
            detector.feed("same"),
            OverthinkingStatus::Stalled { stalled_for: 3 }
        );
    }

    #[test]
    fn overthinking_detector_reset() {
        let config = OverthinkingConfig {
            enabled: true,
            max_duration_secs: 999,
            max_tokens: 10,
            stall_threshold: 500,
        };
        let mut detector = OverthinkingDetector::new(config);
        detector.feed("a");
        detector.feed("b");
        detector.feed("c");
        detector.reset();
        assert_eq!(detector.feed("d"), OverthinkingStatus::Ok);
        assert_eq!(detector.thinking_token_count, 1);
    }
}
