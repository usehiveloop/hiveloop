-- +goose Up
-- Track the credit debit and cost source used when billing generation rows.

ALTER TABLE generations
    ADD COLUMN IF NOT EXISTS credits_debited bigint DEFAULT 0 NOT NULL,
    ADD COLUMN IF NOT EXISTS billing_cost_source text DEFAULT '' NOT NULL;

-- +goose Down
ALTER TABLE generations
    DROP COLUMN IF EXISTS billing_cost_source,
    DROP COLUMN IF EXISTS credits_debited;
