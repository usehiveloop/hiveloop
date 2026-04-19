//! Append-only history invariant guard.
//!
//! Prompt caching on every supported provider relies on a load-bearing
//! assumption: once a message lands in the conversation history, its
//! serialized bytes NEVER change. The provider caches by byte-prefix;
//! mutating any prior message invalidates everything after it.
//!
//! Bridge has two mechanisms that legitimately rewrite history in place —
//! compaction (`crates/runtime/src/compaction.rs`) and immortal chain reset
//! (`crates/runtime/src/immortal.rs`). Both are expected cache-bust events.
//!
//! What this module guards against is **unintentional** drift: tool hooks
//! that accidentally mutate a prior turn's message, serializers that emit
//! non-deterministic JSON, or any other silent corruption. Before each
//! LLM call the loop takes a fingerprint of the current history; after
//! the call (and before the next one) it verifies the prior turns' bytes
//! still match. Mismatches are logged as warnings — we do not abort the
//! request because the correct behavior on drift is "send it anyway, then
//! tell the operator their cache just died."

use sha2::{Digest, Sha256};
use tracing::warn;

/// Per-message SHA-256 digests over the serialized JSON form of
/// `rig::message::Message`. The index in `hashes` matches the index in
/// the `history` vec at the moment the fingerprint was taken.
#[derive(Clone, Debug, Default, PartialEq, Eq)]
pub struct HistoryFingerprint {
    hashes: Vec<[u8; 32]>,
}

impl HistoryFingerprint {
    /// Take a fingerprint of the current history. Messages that fail to
    /// serialize (shouldn't happen for rig types, but defensively) hash
    /// to zero; a subsequent verify will treat those as drift, which is
    /// the conservative behavior.
    pub fn take(messages: &[rig::message::Message]) -> Self {
        let hashes = messages.iter().map(hash_message).collect();
        Self { hashes }
    }

    /// Returns the recorded length.
    pub fn len(&self) -> usize {
        self.hashes.len()
    }

    pub fn is_empty(&self) -> bool {
        self.hashes.is_empty()
    }

    /// Verify that `messages[0..self.len()]` still matches the recorded
    /// fingerprint byte-for-byte. Returns `Ok(())` on match or
    /// `Err(index)` where `index` is the first message position whose
    /// hash has changed.
    ///
    /// If `messages` is shorter than `self.len()` — i.e. history was
    /// truncated — this counts as drift at the truncation point.
    pub fn verify_prefix(&self, messages: &[rig::message::Message]) -> Result<(), DriftReport> {
        if messages.len() < self.hashes.len() {
            return Err(DriftReport {
                first_drift_index: messages.len(),
                recorded_len: self.hashes.len(),
                current_len: messages.len(),
                kind: DriftKind::Truncated,
            });
        }
        for (i, expected) in self.hashes.iter().enumerate() {
            let actual = hash_message(&messages[i]);
            if &actual != expected {
                return Err(DriftReport {
                    first_drift_index: i,
                    recorded_len: self.hashes.len(),
                    current_len: messages.len(),
                    kind: DriftKind::Mutated,
                });
            }
        }
        Ok(())
    }

    /// Verify and, if drift is detected, emit a structured warning.
    /// Returns `true` iff the prefix is intact.
    pub fn verify_and_log(
        &self,
        messages: &[rig::message::Message],
        agent_id: &str,
        conversation_id: &str,
    ) -> bool {
        match self.verify_prefix(messages) {
            Ok(()) => true,
            Err(report) => {
                warn!(
                    agent_id = %agent_id,
                    conversation_id = %conversation_id,
                    first_drift_index = report.first_drift_index,
                    recorded_len = report.recorded_len,
                    current_len = report.current_len,
                    drift_kind = ?report.kind,
                    "history_prefix_drift_detected_cache_will_miss"
                );
                false
            }
        }
    }
}

/// Why verify_prefix failed.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum DriftKind {
    /// A message within the recorded range changed bytes.
    Mutated,
    /// History got shorter — prior messages removed or never carried over.
    Truncated,
}

/// Report of the first position where the recorded fingerprint no longer
/// matches the current history.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct DriftReport {
    pub first_drift_index: usize,
    pub recorded_len: usize,
    pub current_len: usize,
    pub kind: DriftKind,
}

fn hash_message(m: &rig::message::Message) -> [u8; 32] {
    // serde_json::to_vec preserves Map insertion order, which for rig's
    // Message types (built from enums + Vec) means byte-stability across
    // identical inputs. If this ever stops being true, this guard will
    // flag it — exactly what we want.
    let bytes = serde_json::to_vec(m).unwrap_or_default();
    let mut hasher = Sha256::new();
    hasher.update(&bytes);
    hasher.finalize().into()
}

#[cfg(test)]
mod tests {
    use super::*;
    use rig::message::Message;

    fn msg_user(s: &str) -> Message {
        Message::user(s)
    }

    #[test]
    fn empty_history_has_empty_fingerprint() {
        let fp = HistoryFingerprint::take(&[]);
        assert!(fp.is_empty());
        assert!(fp.verify_prefix(&[]).is_ok());
    }

    #[test]
    fn identical_history_matches() {
        let h = vec![msg_user("a"), msg_user("b"), msg_user("c")];
        let fp = HistoryFingerprint::take(&h);
        assert_eq!(fp.len(), 3);
        assert!(fp.verify_prefix(&h).is_ok());
    }

    #[test]
    fn appending_is_fine_for_prefix_verification() {
        let h1 = vec![msg_user("a"), msg_user("b")];
        let fp = HistoryFingerprint::take(&h1);

        let mut h2 = h1.clone();
        h2.push(msg_user("c"));
        h2.push(msg_user("d"));
        assert!(
            fp.verify_prefix(&h2).is_ok(),
            "append-only growth must not trigger drift"
        );
    }

    #[test]
    fn mutating_a_prior_message_flags_drift() {
        let h1 = vec![msg_user("original"), msg_user("second")];
        let fp = HistoryFingerprint::take(&h1);

        let h2 = vec![msg_user("MUTATED"), msg_user("second")];
        let err = fp.verify_prefix(&h2).unwrap_err();
        assert_eq!(err.first_drift_index, 0);
        assert_eq!(err.kind, DriftKind::Mutated);
    }

    #[test]
    fn mutating_middle_message_flags_correct_index() {
        let h1 = vec![msg_user("a"), msg_user("b"), msg_user("c"), msg_user("d")];
        let fp = HistoryFingerprint::take(&h1);

        let h2 = vec![
            msg_user("a"),
            msg_user("b"),
            msg_user("MUTATED"),
            msg_user("d"),
        ];
        let err = fp.verify_prefix(&h2).unwrap_err();
        assert_eq!(err.first_drift_index, 2);
        assert_eq!(err.kind, DriftKind::Mutated);
    }

    #[test]
    fn truncation_flags_drift() {
        let h1 = vec![msg_user("a"), msg_user("b"), msg_user("c")];
        let fp = HistoryFingerprint::take(&h1);

        let h2 = vec![msg_user("a")];
        let err = fp.verify_prefix(&h2).unwrap_err();
        assert_eq!(err.kind, DriftKind::Truncated);
        assert_eq!(err.first_drift_index, 1);
        assert_eq!(err.recorded_len, 3);
        assert_eq!(err.current_len, 1);
    }

    #[test]
    fn verify_and_log_returns_true_on_match() {
        let h = vec![msg_user("ok")];
        let fp = HistoryFingerprint::take(&h);
        assert!(fp.verify_and_log(&h, "a", "c"));
    }

    #[test]
    fn verify_and_log_returns_false_on_drift() {
        let h1 = vec![msg_user("ok")];
        let fp = HistoryFingerprint::take(&h1);

        let h2 = vec![msg_user("drifted")];
        assert!(!fp.verify_and_log(&h2, "a", "c"));
    }
}
