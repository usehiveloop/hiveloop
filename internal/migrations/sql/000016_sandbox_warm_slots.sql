-- +goose Up
-- Warm sandbox capacity for providers that provision slower service resources.

CREATE TABLE sandbox_warm_slots (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    provider_id text NOT NULL,
    mode text NOT NULL,
    status text DEFAULT 'warming'::text NOT NULL,
    external_id text NOT NULL,
    endpoint_url text NOT NULL,
    runtime_image text NOT NULL,
    runtime_port integer DEFAULT 7080 NOT NULL,
    region text DEFAULT ''::text NOT NULL,
    claimed_sandbox_id uuid,
    encrypted_runtime_secret bytea NOT NULL,
    error_message text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

ALTER TABLE ONLY sandbox_warm_slots
    ADD CONSTRAINT sandbox_warm_slots_pkey PRIMARY KEY (id);

ALTER TABLE ONLY sandbox_warm_slots
    ADD CONSTRAINT fk_sandbox_warm_slots_claimed_sandbox
    FOREIGN KEY (claimed_sandbox_id) REFERENCES sandboxes(id) ON DELETE SET NULL;

CREATE UNIQUE INDEX idx_sandbox_warm_slots_provider_external
    ON sandbox_warm_slots USING btree (provider_id, external_id);

CREATE INDEX idx_sandbox_warm_slots_pool_status
    ON sandbox_warm_slots USING btree (provider_id, mode, status, created_at);

CREATE INDEX idx_sandbox_warm_slots_claimed_sandbox_id
    ON sandbox_warm_slots USING btree (claimed_sandbox_id);

-- +goose Down
DROP TABLE IF EXISTS sandbox_warm_slots;
