-- +goose Up
ALTER TABLE employee_sandbox_upgrades
  ADD COLUMN IF NOT EXISTS backup_key text,
  ADD COLUMN IF NOT EXISTS backup_sha256 text,
  ADD COLUMN IF NOT EXISTS backup_bytes bigint NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE employee_sandbox_upgrades
  DROP COLUMN IF EXISTS backup_key,
  DROP COLUMN IF EXISTS backup_sha256,
  DROP COLUMN IF EXISTS backup_bytes;
