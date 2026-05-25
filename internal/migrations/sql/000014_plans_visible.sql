-- +goose Up
-- Separate public catalog visibility from whether an existing plan is still usable.

ALTER TABLE plans
    ADD COLUMN IF NOT EXISTS visible boolean DEFAULT true NOT NULL;

CREATE INDEX IF NOT EXISTS idx_plans_visible ON plans USING btree (visible);

-- +goose Down
ALTER TABLE plans
    DROP COLUMN IF EXISTS visible;
