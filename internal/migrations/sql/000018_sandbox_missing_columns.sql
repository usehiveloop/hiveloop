-- +goose Up
-- Adds columns introduced in migration 000006 that may be missing when
-- the sandboxes table existed before the migration ran. Uses IF NOT EXISTS
-- so it is safe in both production (columns missing) and local/dev
-- (columns already present).

ALTER TABLE sandboxes ADD COLUMN IF NOT EXISTS sandbox_template_id uuid;
ALTER TABLE sandboxes ADD COLUMN IF NOT EXISTS snapshot_id text;
ALTER TABLE sandboxes ADD COLUMN IF NOT EXISTS provider_id text NOT NULL DEFAULT 'daytona';
ALTER TABLE sandboxes ADD COLUMN IF NOT EXISTS runtime_url text NOT NULL DEFAULT '';
ALTER TABLE sandboxes ADD COLUMN IF NOT EXISTS runtime_url_expires_at timestamp with time zone;
ALTER TABLE sandboxes ADD COLUMN IF NOT EXISTS encrypted_runtime_secret bytea NOT NULL DEFAULT '\x00';
ALTER TABLE sandboxes ADD COLUMN IF NOT EXISTS error_message text;
ALTER TABLE sandboxes ADD COLUMN IF NOT EXISTS last_active_at timestamp with time zone;
ALTER TABLE sandboxes ADD COLUMN IF NOT EXISTS stopped_at timestamp with time zone;
ALTER TABLE sandboxes ADD COLUMN IF NOT EXISTS memory_limit_bytes bigint NOT NULL DEFAULT 0;
ALTER TABLE sandboxes ADD COLUMN IF NOT EXISTS memory_used_bytes bigint NOT NULL DEFAULT 0;
ALTER TABLE sandboxes ADD COLUMN IF NOT EXISTS memory_peak_bytes bigint NOT NULL DEFAULT 0;
ALTER TABLE sandboxes ADD COLUMN IF NOT EXISTS cpu_quota text NOT NULL DEFAULT '';
ALTER TABLE sandboxes ADD COLUMN IF NOT EXISTS cpu_usage_usec bigint NOT NULL DEFAULT 0;
ALTER TABLE sandboxes ADD COLUMN IF NOT EXISTS cpu_throttled_count bigint NOT NULL DEFAULT 0;
ALTER TABLE sandboxes ADD COLUMN IF NOT EXISTS pid_count bigint NOT NULL DEFAULT 0;
ALTER TABLE sandboxes ADD COLUMN IF NOT EXISTS resource_checked_at timestamp with time zone;

ALTER TABLE sandboxes ALTER COLUMN runtime_url DROP DEFAULT;
ALTER TABLE sandboxes ALTER COLUMN encrypted_runtime_secret DROP DEFAULT;

-- +goose Down
ALTER TABLE sandboxes DROP COLUMN IF EXISTS sandbox_template_id;
ALTER TABLE sandboxes DROP COLUMN IF EXISTS snapshot_id;
ALTER TABLE sandboxes ALTER COLUMN provider_id DROP DEFAULT;
ALTER TABLE sandboxes DROP COLUMN IF EXISTS runtime_url;
ALTER TABLE sandboxes DROP COLUMN IF EXISTS runtime_url_expires_at;
ALTER TABLE sandboxes DROP COLUMN IF EXISTS encrypted_runtime_secret;
ALTER TABLE sandboxes DROP COLUMN IF EXISTS error_message;
ALTER TABLE sandboxes DROP COLUMN IF EXISTS last_active_at;
ALTER TABLE sandboxes DROP COLUMN IF EXISTS stopped_at;
ALTER TABLE sandboxes DROP COLUMN IF EXISTS memory_limit_bytes;
ALTER TABLE sandboxes DROP COLUMN IF EXISTS memory_used_bytes;
ALTER TABLE sandboxes DROP COLUMN IF EXISTS memory_peak_bytes;
ALTER TABLE sandboxes DROP COLUMN IF EXISTS cpu_quota;
ALTER TABLE sandboxes DROP COLUMN IF EXISTS cpu_usage_usec;
ALTER TABLE sandboxes DROP COLUMN IF EXISTS cpu_throttled_count;
ALTER TABLE sandboxes DROP COLUMN IF EXISTS pid_count;
ALTER TABLE sandboxes DROP COLUMN IF EXISTS resource_checked_at;
