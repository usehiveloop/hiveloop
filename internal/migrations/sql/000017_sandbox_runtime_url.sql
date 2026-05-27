-- +goose Up
-- Adds runtime_url and runtime_url_expires_at columns to sandboxes
-- in case the sandboxes table was created before migration 000006 ran.

ALTER TABLE sandboxes ADD COLUMN IF NOT EXISTS runtime_url text NOT NULL DEFAULT '';
ALTER TABLE sandboxes ADD COLUMN IF NOT EXISTS runtime_url_expires_at timestamp with time zone;
ALTER TABLE sandboxes ALTER COLUMN runtime_url DROP DEFAULT;

-- +goose Down
ALTER TABLE sandboxes DROP COLUMN IF EXISTS runtime_url;
ALTER TABLE sandboxes DROP COLUMN IF EXISTS runtime_url_expires_at;
