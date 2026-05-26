use crate::AgentDefinition;
use arc_swap::ArcSwap;
use std::collections::HashMap;
use std::sync::Arc;

/// Lock-free, atomic snapshot of the current `AgentDefinition`.
///
/// Read path: `snapshot()` returns an `Arc<AgentDefinition>`. The session actor
/// captures one snapshot at the start of every turn — config updates land on
/// the *next* turn, never mid-turn.
///
/// Write path: `replace()` is atomic; old snapshots stay valid until callers
/// drop them.
#[derive(Clone)]
pub struct ConfigStore {
    inner: Arc<ArcSwap<AgentDefinition>>,
    runtime_env: Arc<ArcSwap<HashMap<String, String>>>,
}

impl ConfigStore {
    pub fn new(initial: AgentDefinition) -> Self {
        Self::with_runtime_env(initial, HashMap::new())
    }

    pub fn with_runtime_env(
        initial: AgentDefinition,
        runtime_env: HashMap<String, String>,
    ) -> Self {
        Self {
            inner: Arc::new(ArcSwap::from_pointee(initial)),
            runtime_env: Arc::new(ArcSwap::from_pointee(runtime_env)),
        }
    }

    pub fn snapshot(&self) -> Arc<AgentDefinition> {
        self.inner.load_full()
    }

    pub fn runtime_env(&self) -> Arc<HashMap<String, String>> {
        self.runtime_env.load_full()
    }

    pub fn set_runtime_env(&self, overrides: HashMap<String, String>) {
        self.runtime_env.store(Arc::new(overrides));
    }

    pub fn merge_runtime_env(
        &self,
        updates: HashMap<String, String>,
    ) -> Arc<HashMap<String, String>> {
        let mut merged = (*self.runtime_env.load_full()).clone();
        merged.extend(updates);
        let merged = Arc::new(merged);
        self.runtime_env.store(merged.clone());
        merged
    }

    pub fn replace(&self, def: AgentDefinition) {
        self.inner.store(Arc::new(def));
    }
}
