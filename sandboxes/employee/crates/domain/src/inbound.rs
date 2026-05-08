use crate::{reply::MessageHandle, session::SessionId};
use serde::{Deserialize, Serialize};

#[derive(Debug, Clone)]
pub struct InboundEvent {
    pub envelope_id: String,
    pub session_id: SessionId,
    pub user: String,
    pub user_display_name: Option<String>,
    pub text: String,
    pub attachments: Vec<Attachment>,
    pub raw: serde_json::Value,
    pub inbound_handle: MessageHandle,
    pub is_direct_message: bool,
    pub is_directly_addressed: bool,
    pub link_previews: Vec<LinkPreview>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Attachment {
    pub url: String,
    pub mime_type: String,
    pub name: String,
    #[serde(default)]
    pub size_bytes: Option<u64>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LinkPreview {
    pub url: String,
    pub title: Option<String>,
    pub description: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct HistoryMessage {
    pub user: String,
    pub user_display_name: Option<String>,
    pub text: String,
    pub ts: String,
    pub is_bot: bool,
}
