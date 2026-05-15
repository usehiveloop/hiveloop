use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct SlackConfig {
    #[serde(default)]
    pub ignore_users: Vec<String>,
    #[serde(default = "yes")]
    pub typing_indicator: bool,
    #[serde(default = "yes")]
    pub reactions_enabled: bool,
    #[serde(default = "default_progressive_messages")]
    pub progressive_messages: ProgressiveMessages,
    #[serde(default = "default_max_message_length")]
    pub max_message_length: u32,
    #[serde(default = "yes")]
    pub split_long_replies: bool,
    #[serde(default)]
    pub allowed_channels: Vec<String>,
    #[serde(default = "yes")]
    pub require_mention: bool,
    #[serde(default)]
    pub strict_mention: bool,
    #[serde(default)]
    pub free_response_channels: Vec<String>,
    #[serde(default)]
    pub allow_bots: AllowBotsMode,
    #[serde(default)]
    pub reply_broadcast: bool,
    #[serde(default)]
    pub reply_prefix: String,
    #[serde(default)]
    pub thread_context: ThreadContextConfig,
    #[serde(default = "yes")]
    pub mrkdwn_translation: bool,
    #[serde(default = "yes")]
    pub fetch_user_names: bool,
    #[serde(default = "yes")]
    pub extract_blocks_text: bool,
    #[serde(default = "yes")]
    pub extract_link_unfurls: bool,
    #[serde(default = "yes")]
    pub download_attachments: bool,
    #[serde(default = "yes")]
    pub inline_text_files: bool,
    #[serde(default = "default_inline_text_max_bytes")]
    pub inline_text_max_bytes: u64,
    #[serde(default = "yes")]
    pub reply_in_thread: bool,
    #[serde(default)]
    pub channel_prompts: std::collections::HashMap<String, String>,
    #[serde(default)]
    pub retry_max_attempts: Option<u32>,
}

fn default_inline_text_max_bytes() -> u64 {
    102_400
}

fn yes() -> bool {
    true
}

fn default_max_message_length() -> u32 {
    39000
}

fn default_progressive_messages() -> ProgressiveMessages {
    ProgressiveMessages {
        enabled: true,
        edit_interval_ms: 1500,
    }
}

impl Default for SlackConfig {
    fn default() -> Self {
        Self {
            ignore_users: Vec::new(),
            typing_indicator: true,
            reactions_enabled: true,
            progressive_messages: ProgressiveMessages {
                enabled: true,
                edit_interval_ms: 1500,
            },
            max_message_length: default_max_message_length(),
            split_long_replies: true,
            allowed_channels: Vec::new(),
            require_mention: true,
            strict_mention: false,
            free_response_channels: Vec::new(),
            allow_bots: AllowBotsMode::default(),
            reply_broadcast: false,
            reply_prefix: String::new(),
            thread_context: ThreadContextConfig::default(),
            mrkdwn_translation: true,
            fetch_user_names: true,
            extract_blocks_text: true,
            extract_link_unfurls: true,
            download_attachments: true,
            inline_text_files: true,
            inline_text_max_bytes: 102_400,
            reply_in_thread: true,
            channel_prompts: std::collections::HashMap::new(),
            retry_max_attempts: Some(3),
        }
    }
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
#[serde(rename_all = "snake_case")]
pub enum AllowBotsMode {
    #[default]
    None,
    Mentions,
    All,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct ProgressiveMessages {
    pub enabled: bool,
    pub edit_interval_ms: u32,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct ThreadContextConfig {
    pub enabled: bool,
    pub max_messages: u32,
    pub cache_ttl_seconds: u64,
}

impl Default for ThreadContextConfig {
    fn default() -> Self {
        Self {
            enabled: false,
            max_messages: 20,
            cache_ttl_seconds: 60,
        }
    }
}
