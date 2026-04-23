use super::common::MockRunner;
use crate::agent::{AgentContext, SubAgentTool, TaskBudget, AGENT_CONTEXT};
use crate::ToolExecutor;
use std::sync::Arc;
use tokio::sync::mpsc;

#[test]
fn test_task_budget_basic_acquire() {
    let budget = TaskBudget::new(3);
    assert_eq!(budget.remaining(), 3);
    assert_eq!(budget.used(), 0);

    assert!(budget.try_acquire().is_ok());
    assert_eq!(budget.remaining(), 2);
    assert_eq!(budget.used(), 1);

    assert!(budget.try_acquire().is_ok());
    assert!(budget.try_acquire().is_ok());
    assert_eq!(budget.remaining(), 0);
    assert_eq!(budget.used(), 3);
}

#[test]
fn test_task_budget_exhaustion() {
    let budget = TaskBudget::new(2);
    assert!(budget.try_acquire().is_ok());
    assert!(budget.try_acquire().is_ok());

    let err = budget.try_acquire();
    assert!(err.is_err());
    assert!(err.unwrap_err().contains("Task budget exhausted"));
    // Should not have incremented past max
    assert_eq!(budget.used(), 2);
}

#[test]
fn test_task_budget_acquire_many_success() {
    let budget = TaskBudget::new(10);
    assert!(budget.try_acquire_many(5).is_ok());
    assert_eq!(budget.used(), 5);
    assert_eq!(budget.remaining(), 5);

    assert!(budget.try_acquire_many(5).is_ok());
    assert_eq!(budget.used(), 10);
    assert_eq!(budget.remaining(), 0);
}

#[test]
fn test_task_budget_acquire_many_insufficient() {
    let budget = TaskBudget::new(5);
    assert!(budget.try_acquire_many(3).is_ok());

    let err = budget.try_acquire_many(5);
    assert!(err.is_err());
    assert!(err.unwrap_err().contains("Cannot spawn 5 tasks"));
    // Should have rolled back
    assert_eq!(budget.used(), 3);
}

#[test]
fn test_task_budget_zero_max() {
    let budget = TaskBudget::new(0);
    assert_eq!(budget.remaining(), 0);
    assert!(budget.try_acquire().is_err());
}

#[test]
fn test_task_budget_thread_safety() {
    let budget = Arc::new(TaskBudget::new(100));
    let handles: Vec<_> = (0..10)
        .map(|_| {
            let b = budget.clone();
            std::thread::spawn(move || {
                let mut acquired = 0;
                for _ in 0..20 {
                    if b.try_acquire().is_ok() {
                        acquired += 1;
                    }
                }
                acquired
            })
        })
        .collect();

    let total: usize = handles
        .into_iter()
        .map(|h| h.join().unwrap())
        .collect::<Vec<_>>()
        .into_iter()
        .sum();
    assert_eq!(
        total, 100,
        "exactly 100 slots should be acquired across all threads"
    );
    assert_eq!(budget.used(), 100);
    assert_eq!(budget.remaining(), 0);
}

#[tokio::test]
async fn test_task_budget_enforced_by_sub_agent_tool() {
    // Budget of 1 — second call should fail
    let budget = Arc::new(TaskBudget::new(1));
    let (tx, _rx) = mpsc::channel(16);
    let ctx = AgentContext {
        runner: Arc::new(MockRunner {
            subagents: vec![("coder".to_string(), "A coding agent".to_string())],
        }),
        notification_tx: tx,
        depth: 0,
        max_depth: 3,
        task_budget: budget,
    };

    let tool = SubAgentTool::new();
    let args = serde_json::json!({
        "description": "test",
        "prompt": "do something",
        "subagentName": "coder"
    });

    // First call should succeed
    let result1 = AGENT_CONTEXT
        .scope(ctx.clone(), async { tool.execute(args.clone()).await })
        .await;
    assert!(result1.is_ok());

    // Second call should fail — budget exhausted
    let result2 = AGENT_CONTEXT
        .scope(ctx.clone(), async { tool.execute(args.clone()).await })
        .await;
    assert!(result2.is_err());
    assert!(result2.unwrap_err().contains("Task budget exhausted"));
}
