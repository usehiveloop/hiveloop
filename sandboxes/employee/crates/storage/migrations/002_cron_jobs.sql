CREATE TABLE IF NOT EXISTS cron_jobs (
    id TEXT PRIMARY KEY NOT NULL,
    channel TEXT NOT NULL,
    task_prompt TEXT NOT NULL,
    cron_expression TEXT,
    interval_seconds INTEGER,
    next_run_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    created_by_session TEXT NOT NULL
);
