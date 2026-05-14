use std::num::NonZeroUsize;
use std::sync::{Arc, OnceLock};

use chrono::{DateTime, Utc};
use dashmap::{DashMap, DashSet};
use domain::{ConfigStore, HistoryMessage, InboundEvent};
use lru::LruCache;
use slack_morphism::prelude::*;
use tokio::sync::{Mutex, OnceCell};

use crate::{GatewayError, Result};

static GLOBAL_CONTEXT: OnceLock<Arc<SlackContext>> = OnceLock::new();

pub struct SlackContext {
    pub client: Arc<SlackHyperClient>,
    pub bot_token: SlackApiToken,
    pub config: ConfigStore,
    pub bot_user_id: OnceCell<String>,
    pub inbound_sink: OnceCell<tokio::sync::mpsc::Sender<InboundEvent>>,
    pub engaged_threads: DashSet<String>,
    pub seen_event_ids: Mutex<LruCache<String, ()>>,
    pub user_display_name_cache: DashMap<String, CachedDisplayName>,
    pub thread_history_cache: DashMap<String, CachedThreadHistory>,
    pub bot_message_timestamps: DashSet<String>,
    pub synthetic_thread_sessions: DashSet<String>,
    pub assistant_thread_metadata: DashMap<String, AssistantThreadMetadata>,
}

#[allow(dead_code)]
#[derive(Clone)]
pub struct AssistantThreadMetadata {
    pub user_id: String,
    pub channel_id: String,
    pub thread_ts: String,
    pub fetched_at: DateTime<Utc>,
}

#[derive(Clone)]
pub struct CachedDisplayName {
    pub name: String,
    pub fetched_at: DateTime<Utc>,
}

#[derive(Clone)]
pub struct CachedThreadHistory {
    pub messages: Vec<HistoryMessage>,
    pub fetched_at: DateTime<Utc>,
}

impl SlackContext {
    pub fn new(
        client: Arc<SlackHyperClient>,
        bot_token: SlackApiToken,
        config: ConfigStore,
    ) -> Self {
        Self {
            client,
            bot_token,
            config,
            bot_user_id: OnceCell::new(),
            inbound_sink: OnceCell::new(),
            engaged_threads: DashSet::new(),
            seen_event_ids: Mutex::new(LruCache::new(NonZeroUsize::new(8192).unwrap())),
            user_display_name_cache: DashMap::new(),
            thread_history_cache: DashMap::new(),
            bot_message_timestamps: DashSet::new(),
            synthetic_thread_sessions: DashSet::new(),
            assistant_thread_metadata: DashMap::new(),
        }
    }

    pub fn set_runtime_state(
        &self,
        sink: tokio::sync::mpsc::Sender<InboundEvent>,
        bot_user_id: String,
    ) {
        let _ = self.bot_user_id.set(bot_user_id);
        let _ = self.inbound_sink.set(sink);
    }

    pub fn bot_user_id_str(&self) -> &str {
        self.bot_user_id
            .get()
            .map(|s| s.as_str())
            .unwrap_or_default()
    }

    pub fn open_api_session(&self) -> SlackClientSession<'_, SlackClientHyperHttpsConnector> {
        self.client.open_session(&self.bot_token)
    }
}

pub fn install_global(context: Arc<SlackContext>) -> Result<()> {
    GLOBAL_CONTEXT
        .set(context)
        .map_err(|_| GatewayError::Other(anyhow::anyhow!("slack gateway already running")))
}

pub fn global() -> Option<&'static Arc<SlackContext>> {
    GLOBAL_CONTEXT.get()
}
