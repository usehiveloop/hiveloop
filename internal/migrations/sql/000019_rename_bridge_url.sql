-- +goose Up
-- +goose StatementBegin
DO $rename_bridge_runtime$
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'sandboxes' AND column_name = 'bridge_url') AND NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'sandboxes' AND column_name = 'runtime_url') THEN
    ALTER TABLE sandboxes RENAME COLUMN bridge_url TO runtime_url;
  END IF;
  IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'sandboxes' AND column_name = 'bridge_url_expires_at') AND NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'sandboxes' AND column_name = 'runtime_url_expires_at') THEN
    ALTER TABLE sandboxes RENAME COLUMN bridge_url_expires_at TO runtime_url_expires_at;
  END IF;
END $rename_bridge_runtime$;
-- +goose StatementEnd

-- +goose Down
-- No-op.
