use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;

use domain::ConfigStore;
use skills::SkillWriter;
use storage::{ConfigRepo, EventRepo, SessionRepo};

#[derive(Clone)]
pub struct ApiState {
    pub config_store: ConfigStore,
    pub config_repo: Arc<dyn ConfigRepo>,
    pub session_repo: Arc<dyn SessionRepo>,
    pub event_repo: Arc<dyn EventRepo>,
    pub bearer_token: Arc<String>,
    pub gateway_ready: Arc<AtomicBool>,
    pub config_loaded: Arc<AtomicBool>,
    pub skill_writer: Arc<SkillWriter>,
}

impl ApiState {
    pub fn new(
        config_store: ConfigStore,
        config_repo: Arc<dyn ConfigRepo>,
        session_repo: Arc<dyn SessionRepo>,
        event_repo: Arc<dyn EventRepo>,
        bearer_token: String,
        skill_writer: Arc<SkillWriter>,
    ) -> Self {
        Self {
            config_store,
            config_repo,
            session_repo,
            event_repo,
            bearer_token: Arc::new(bearer_token),
            gateway_ready: Arc::new(AtomicBool::new(false)),
            config_loaded: Arc::new(AtomicBool::new(false)),
            skill_writer,
        }
    }

    pub fn mark_gateway_ready(&self) {
        self.gateway_ready.store(true, Ordering::Release);
    }

    pub fn mark_config_loaded(&self) {
        self.config_loaded.store(true, Ordering::Release);
    }

    pub fn is_ready(&self) -> bool {
        self.gateway_ready.load(Ordering::Acquire) && self.config_loaded.load(Ordering::Acquire)
    }
}
