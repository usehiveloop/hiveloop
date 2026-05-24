-- +goose Up
-- Credentials, API keys, tokens, and audit tables

CREATE TABLE api_keys (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    name text NOT NULL,
    key_hash text NOT NULL,
    key_prefix text NOT NULL,
    scopes text[] NOT NULL,
    expires_at timestamp with time zone,
    last_used_at timestamp with time zone,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone
);

CREATE TABLE audit_log (
    id bigint NOT NULL,
    org_id uuid NOT NULL,
    credential_id uuid,
    action text NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb,
    ip_address inet,
    created_at timestamp with time zone
);

CREATE SEQUENCE audit_log_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE audit_log_id_seq OWNED BY audit_log.id;

CREATE TABLE credentials (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid,
    label text DEFAULT ''::text NOT NULL,
    base_url text NOT NULL,
    auth_scheme text NOT NULL,
    encrypted_key bytea NOT NULL,
    wrapped_dek bytea NOT NULL,
    remaining bigint,
    refill_amount bigint,
    refill_interval text,
    last_refill_at timestamp with time zone,
    provider_id text DEFAULT ''::text,
    meta jsonb DEFAULT '{}'::jsonb,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone
);

CREATE TABLE tokens (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    credential_id uuid NOT NULL,
    jti text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    remaining bigint,
    refill_amount bigint,
    refill_interval text,
    last_refill_at timestamp with time zone,
    scopes jsonb,
    meta jsonb DEFAULT '{}'::jsonb,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone
);

ALTER TABLE ONLY audit_log ALTER COLUMN id SET DEFAULT nextval('audit_log_id_seq'::regclass);

ALTER TABLE ONLY api_keys
    ADD CONSTRAINT api_keys_pkey PRIMARY KEY (id);

ALTER TABLE ONLY audit_log
    ADD CONSTRAINT audit_log_pkey PRIMARY KEY (id);

ALTER TABLE ONLY credentials
    ADD CONSTRAINT credentials_pkey PRIMARY KEY (id);

ALTER TABLE ONLY tokens
    ADD CONSTRAINT tokens_pkey PRIMARY KEY (id);

CREATE UNIQUE INDEX idx_api_keys_key_hash ON api_keys USING btree (key_hash);

CREATE INDEX idx_api_keys_org_id ON api_keys USING btree (org_id);

CREATE INDEX idx_audit_credential ON audit_log USING btree (credential_id);

CREATE INDEX idx_audit_org_created ON audit_log USING btree (org_id, created_at);

CREATE INDEX idx_credentials_org_id ON credentials USING btree (org_id);

CREATE INDEX idx_tokens_credential_id ON tokens USING btree (credential_id);

CREATE UNIQUE INDEX idx_tokens_jti ON tokens USING btree (jti);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'initial schema down migration is intentionally unsupported; reset or restore the database instead'; END $$;
-- +goose StatementEnd
