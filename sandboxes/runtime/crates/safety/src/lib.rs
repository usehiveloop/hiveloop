pub mod json_repair;
pub mod repeat_detector;
pub mod thinking_guard;
pub mod xml_tool_repair;

use json_repair::JsonRepair;
use repeat_detector::RepeatToolCallDetector;
use thinking_guard::{OverthinkingDetector, OverthinkingStatus, ThinkingGuard};
use xml_tool_repair::XmlToolCallRepair;

use domain::SafetyConfig;

#[derive(Clone)]
pub struct SafetyHarness {
    config: SafetyConfig,
    thinking_guard: ThinkingGuard,
    xml_repair: XmlToolCallRepair,
    json_repair: JsonRepair,
}

impl SafetyHarness {
    pub fn new(config: SafetyConfig) -> Self {
        Self {
            config,
            thinking_guard: ThinkingGuard::new(),
            xml_repair: XmlToolCallRepair::new(),
            json_repair: JsonRepair::new(),
        }
    }

    pub fn config(&self) -> &SafetyConfig {
        &self.config
    }

    pub fn thinking_guard(&self) -> &ThinkingGuard {
        &self.thinking_guard
    }

    pub fn xml_repair(&self) -> &XmlToolCallRepair {
        &self.xml_repair
    }

    pub fn json_repair(&self) -> &JsonRepair {
        &self.json_repair
    }

    pub fn create_overthinking_detector(&self) -> OverthinkingDetector {
        OverthinkingDetector::new(self.config.overthinking.clone())
    }

    pub fn create_repeat_detector(&self) -> RepeatToolCallDetector {
        RepeatToolCallDetector::new(self.config.repeat_detection.clone())
    }
}

pub struct TurnSafety {
    pub overthinking: OverthinkingDetector,
    pub repeat_detector: RepeatToolCallDetector,
}

impl TurnSafety {
    pub fn new(harness: &SafetyHarness) -> Self {
        Self {
            overthinking: harness.create_overthinking_detector(),
            repeat_detector: harness.create_repeat_detector(),
        }
    }
}

pub fn overthinking_feedback(status: &OverthinkingStatus) -> String {
    let reason = status.reason();
    format!(
        "{reason}. Please stop reasoning and provide your response or tool calls directly. \
         If you need to use tools, call them immediately without further internal deliberation."
    )
}

pub fn xml_repair_reminder() -> String {
    "Auto-repaired XML-formatted tool calls. Please use the native JSON function calling format \
     going forward. Tool calls should be proper JSON, not XML tags."
        .to_string()
}

pub fn json_repair_reminder() -> String {
    "Auto-repaired malformed JSON arguments. Please ensure JSON arguments use double quotes for \
     strings, no trailing commas, proper brace balance, and valid escape sequences."
        .to_string()
}
