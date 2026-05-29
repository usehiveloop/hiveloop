use dashmap::DashMap;
use domain::{InboundEvent, SessionId};
use std::sync::Mutex;

pub struct SessionCoordinator {
    sessions: DashMap<SessionId, Mutex<Vec<InboundEvent>>>,
}

pub enum Submission {
    RunNow,
    Queued,
}

impl SessionCoordinator {
    pub fn new() -> Self {
        Self {
            sessions: DashMap::new(),
        }
    }

    pub fn submit_or_queue(&self, inbound: InboundEvent) -> Submission {
        let session_id = inbound.session_id.clone();
        match self.sessions.get(&session_id) {
            Some(queue) => {
                queue.lock().unwrap().push(inbound);
                Submission::Queued
            }
            None => {
                self.sessions.insert(session_id, Mutex::new(Vec::new()));
                Submission::RunNow
            }
        }
    }

    pub fn reserve(&self, session_id: &SessionId) {
        self.sessions
            .insert(session_id.clone(), Mutex::new(Vec::new()));
    }

    pub fn finish_turn(&self, session_id: &SessionId) -> Vec<InboundEvent> {
        self.sessions
            .remove(session_id)
            .map(|(_, queue)| queue.into_inner().unwrap())
            .unwrap_or_default()
    }

    pub fn drain_queued(&self, session_id: &SessionId) -> Vec<InboundEvent> {
        self.sessions
            .get(session_id)
            .map(|queue| {
                let mut queue = queue.lock().unwrap();
                std::mem::take(&mut *queue)
            })
            .unwrap_or_default()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use domain::{reply::MessageHandle, Attachment};

    fn inbound(session: &str, envelope: &str, text: &str) -> InboundEvent {
        InboundEvent {
            envelope_id: envelope.to_string(),
            session_id: SessionId::from(session.to_string()),
            user: "U123".to_string(),
            user_display_name: Some("Ada".to_string()),
            text: text.to_string(),
            attachments: vec![Attachment {
                url: "https://example.test/log.txt".to_string(),
                mime_type: "text/plain".to_string(),
                name: "log.txt".to_string(),
                size_bytes: Some(42),
            }],
            raw: serde_json::json!({"source": "test"}),
            inbound_handle: MessageHandle {
                channel: "C123".to_string(),
                ts: envelope.to_string(),
            },
            is_direct_message: false,
            is_directly_addressed: true,
            link_previews: Vec::new(),
            agent_definition: None,
        }
    }

    #[test]
    fn queues_same_session_while_turn_is_active() {
        let coordinator = SessionCoordinator::new();
        let first = inbound("C123-T1", "E1", "first");
        let second = inbound("C123-T1", "E2", "second");

        assert!(matches!(
            coordinator.submit_or_queue(first),
            Submission::RunNow
        ));
        assert!(matches!(
            coordinator.submit_or_queue(second),
            Submission::Queued
        ));

        let queued = coordinator.finish_turn(&SessionId::from("C123-T1".to_string()));
        assert_eq!(queued.len(), 1);
        assert_eq!(queued[0].text, "second");
        assert_eq!(queued[0].attachments.len(), 1);
    }

    #[test]
    fn different_sessions_run_independently() {
        let coordinator = SessionCoordinator::new();

        assert!(matches!(
            coordinator.submit_or_queue(inbound("C123-T1", "E1", "first")),
            Submission::RunNow
        ));
        assert!(matches!(
            coordinator.submit_or_queue(inbound("C123-T2", "E2", "second")),
            Submission::RunNow
        ));

        assert!(coordinator
            .finish_turn(&SessionId::from("C123-T1".to_string()))
            .is_empty());
        assert!(coordinator
            .finish_turn(&SessionId::from("C123-T2".to_string()))
            .is_empty());
    }
}
