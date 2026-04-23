use std::sync::atomic::{AtomicUsize, Ordering};

/// Tracks and limits the total number of subagent tasks spawned
/// within a single conversation's lifetime.
///
/// Shared (via `Arc`) across all subagent depths within a conversation,
/// ensuring a global ceiling on resource consumption regardless of nesting.
pub struct TaskBudget {
    /// Current count of spawned tasks (foreground + background).
    spawned: AtomicUsize,
    /// Maximum allowed tasks per conversation.
    max_tasks: usize,
}

impl TaskBudget {
    /// Create a new budget with the given maximum.
    pub fn new(max_tasks: usize) -> Self {
        Self {
            spawned: AtomicUsize::new(0),
            max_tasks,
        }
    }

    /// Try to acquire a task slot. Returns `Err` if budget exhausted.
    pub fn try_acquire(&self) -> Result<(), String> {
        // Optimistic increment — roll back on failure.
        let prev = self.spawned.fetch_add(1, Ordering::Relaxed);
        if prev >= self.max_tasks {
            self.spawned.fetch_sub(1, Ordering::Relaxed);
            Err(format!(
                "Task budget exhausted: {} of {} task slots used. \
                 Wait for existing tasks to complete before spawning more.",
                prev, self.max_tasks
            ))
        } else {
            Ok(())
        }
    }

    /// Try to acquire `n` task slots atomically. Returns `Err` if insufficient.
    pub fn try_acquire_many(&self, n: usize) -> Result<(), String> {
        let prev = self.spawned.fetch_add(n, Ordering::Relaxed);
        if prev + n > self.max_tasks {
            self.spawned.fetch_sub(n, Ordering::Relaxed);
            Err(format!(
                "Cannot spawn {} tasks: only {} of {} slots remaining.",
                n,
                self.max_tasks.saturating_sub(prev),
                self.max_tasks
            ))
        } else {
            Ok(())
        }
    }

    /// Returns the number of remaining task slots.
    pub fn remaining(&self) -> usize {
        self.max_tasks
            .saturating_sub(self.spawned.load(Ordering::Relaxed))
    }

    /// Returns the current number of spawned tasks.
    pub fn used(&self) -> usize {
        self.spawned.load(Ordering::Relaxed)
    }
}
