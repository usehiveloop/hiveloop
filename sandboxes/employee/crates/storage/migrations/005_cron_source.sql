ALTER TABLE cron_jobs ADD COLUMN source TEXT NOT NULL DEFAULT 'cron';
ALTER TABLE cron_jobs ADD COLUMN delegated_session_id TEXT;
