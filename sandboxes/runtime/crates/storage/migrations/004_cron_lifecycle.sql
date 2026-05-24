ALTER TABLE cron_jobs ADD COLUMN repeat_count INTEGER;
ALTER TABLE cron_jobs ADD COLUMN repeat_completed INTEGER NOT NULL DEFAULT 0;
ALTER TABLE cron_jobs ADD COLUMN state TEXT NOT NULL DEFAULT 'active';
ALTER TABLE cron_jobs ADD COLUMN last_run_at TEXT;
ALTER TABLE cron_jobs ADD COLUMN last_status TEXT;
ALTER TABLE cron_jobs ADD COLUMN last_error TEXT;
