-- +goose Up
-- Integration catalog and user connection tables

CREATE TABLE connections (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid,
    user_id uuid NOT NULL,
    integration_id uuid NOT NULL,
    nango_connection_id text NOT NULL,
    meta jsonb DEFAULT '{}'::jsonb,
    webhook_configured boolean DEFAULT true NOT NULL,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE integrations (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    unique_key text NOT NULL,
    provider text NOT NULL,
    display_name text NOT NULL,
    org_id uuid,
    employee_id uuid,
    custom_app boolean DEFAULT false NOT NULL,
    meta jsonb DEFAULT '{}'::jsonb,
    nango_config jsonb DEFAULT '{}'::jsonb,
    managed_by text DEFAULT ''::text NOT NULL,
    managed_id text DEFAULT ''::text NOT NULL,
    managed_hash text DEFAULT ''::text NOT NULL,
    required boolean DEFAULT false NOT NULL,
    supports_rag_source boolean DEFAULT false NOT NULL,
    deleted_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

ALTER TABLE ONLY connections
    ADD CONSTRAINT connections_pkey PRIMARY KEY (id);

ALTER TABLE ONLY integrations
    ADD CONSTRAINT integrations_pkey PRIMARY KEY (id);

CREATE INDEX idx_connections_integration_id ON connections USING btree (integration_id);

CREATE INDEX idx_connections_org_id ON connections USING btree (org_id);

CREATE INDEX idx_connections_user_id ON connections USING btree (user_id);

CREATE INDEX idx_integrations_custom_app ON integrations USING btree (custom_app);

CREATE INDEX idx_integrations_deleted_at ON integrations USING btree (deleted_at);

CREATE INDEX idx_integrations_employee_id ON integrations USING btree (employee_id);

CREATE INDEX idx_integrations_managed_by ON integrations USING btree (managed_by);

CREATE INDEX idx_integrations_managed_id ON integrations USING btree (managed_id);

CREATE INDEX idx_integrations_org_id ON integrations USING btree (org_id);

CREATE INDEX idx_integrations_provider ON integrations USING btree (provider);

CREATE UNIQUE INDEX idx_integrations_unique_key ON integrations USING btree (unique_key);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'initial schema down migration is intentionally unsupported; reset or restore the database instead'; END $$;
-- +goose StatementEnd
