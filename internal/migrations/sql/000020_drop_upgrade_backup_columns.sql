-- +goose Up
-- Intentionally retained as a no-op. Backup metadata is still part of the
-- sandbox upgrade contract; 000021 restores these columns for environments
-- that already applied the earlier destructive version of this migration.
SELECT 1;

-- +goose Down
SELECT 1;
