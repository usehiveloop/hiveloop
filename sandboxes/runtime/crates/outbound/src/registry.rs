use std::sync::Arc;

use crate::OutboundChannel;

#[derive(Clone, Default)]
pub struct OutboundRegistry {
    channels: Vec<Arc<dyn OutboundChannel>>,
}

impl OutboundRegistry {
    pub fn new() -> Self {
        Self {
            channels: Vec::new(),
        }
    }

    pub fn with_channel(mut self, channel: Arc<dyn OutboundChannel>) -> Self {
        self.channels.push(channel);
        self
    }

    pub fn add(&mut self, channel: Arc<dyn OutboundChannel>) {
        self.channels.push(channel);
    }

    pub fn channels(&self) -> &[Arc<dyn OutboundChannel>] {
        &self.channels
    }

    pub fn matching<'a>(
        &'a self,
        event_type: &'a str,
    ) -> impl Iterator<Item = &'a Arc<dyn OutboundChannel>> {
        self.channels
            .iter()
            .filter(move |channel| channel.accepts(event_type))
    }

    pub fn find(&self, name: &str) -> Option<Arc<dyn OutboundChannel>> {
        self.channels
            .iter()
            .find(|channel| channel.name() == name)
            .cloned()
    }

    pub fn names(&self) -> Vec<String> {
        self.channels.iter().map(|c| c.name().to_string()).collect()
    }
}
