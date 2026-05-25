-- +goose Up
-- Conversation artifact tables

CREATE TABLE conversation_assets (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    conversation_id uuid NOT NULL,
    org_id uuid NOT NULL,
    sandbox_id uuid NOT NULL,
    path text NOT NULL,
    filename text NOT NULL,
    key text NOT NULL,
    public_url text NOT NULL,
    content_type text NOT NULL,
    bytes bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

ALTER TABLE ONLY conversation_assets
    ADD CONSTRAINT conversation_assets_pkey PRIMARY KEY (id);

CREATE INDEX idx_conv_asset_conv_created ON conversation_assets USING btree (conversation_id, created_at DESC);

CREATE UNIQUE INDEX idx_conversation_assets_key ON conversation_assets USING btree (key);

CREATE INDEX idx_conversation_assets_org_id ON conversation_assets USING btree (org_id);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'initial schema down migration is intentionally unsupported; reset or restore the database instead'; END $$;
-- +goose StatementEnd
