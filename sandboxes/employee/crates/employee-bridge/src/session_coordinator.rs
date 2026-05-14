use dashmap::DashMap;
use domain::SessionId;
use std::sync::Mutex;

pub struct SessionCoordinator {
    sessions: DashMap<SessionId, Mutex<Vec<String>>>,
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

    pub fn submit_or_queue(&self, session_id: &SessionId, text: String) -> Submission {
        match self.sessions.get(session_id) {
            Some(queue) => {
                queue.lock().unwrap().push(text);
                Submission::Queued
            }
            None => {
                self.sessions
                    .insert(session_id.clone(), Mutex::new(Vec::new()));
                Submission::RunNow
            }
        }
    }

    pub fn finish_turn(&self, session_id: &SessionId) -> Vec<String> {
        self.sessions
            .remove(session_id)
            .map(|(_, queue)| queue.into_inner().unwrap())
            .unwrap_or_default()
    }
}
