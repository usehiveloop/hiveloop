-- +goose Up
-- Core identity, organizations, and auth tables

CREATE TABLE email_verifications (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    token_hash text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    used_at timestamp with time zone,
    created_at timestamp with time zone
);

CREATE TABLE oauth_accounts (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    provider text NOT NULL,
    provider_user_id text NOT NULL,
    provider_user_email text,
    provider_user_login text,
    verified_emails text[],
    last_synced_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE oauth_exchange_tokens (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    token_hash text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    used_at timestamp with time zone,
    created_at timestamp with time zone
);

CREATE TABLE org_invites (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    email text NOT NULL,
    role text NOT NULL,
    token_hash text NOT NULL,
    invited_by_id uuid NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    accepted_at timestamp with time zone,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE org_memberships (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    org_id uuid NOT NULL,
    role text DEFAULT 'owner'::text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE orgs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    rate_limit bigint DEFAULT 1000 NOT NULL,
    active boolean DEFAULT true NOT NULL,
    allowed_origins text[],
    plan_slug character varying(64) DEFAULT 'free'::character varying NOT NULL,
    byok boolean DEFAULT false NOT NULL,
    logo_url text DEFAULT ''::text NOT NULL,
    website character varying(500) DEFAULT ''::character varying NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    prompt_company text DEFAULT ''::text NOT NULL,
    onboarded boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE otp_codes (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    email text NOT NULL,
    token_hash text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    used_at timestamp with time zone,
    created_at timestamp with time zone
);

CREATE TABLE password_resets (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    token_hash text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    used_at timestamp with time zone,
    created_at timestamp with time zone
);

CREATE TABLE refresh_tokens (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    token_hash text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone
);

CREATE TABLE users (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    email text NOT NULL,
    password_hash text,
    name text,
    email_confirmed_at timestamp with time zone,
    banned_at timestamp with time zone,
    ban_reason text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

ALTER TABLE ONLY email_verifications
    ADD CONSTRAINT email_verifications_pkey PRIMARY KEY (id);

ALTER TABLE ONLY oauth_accounts
    ADD CONSTRAINT oauth_accounts_pkey PRIMARY KEY (id);

ALTER TABLE ONLY oauth_exchange_tokens
    ADD CONSTRAINT oauth_exchange_tokens_pkey PRIMARY KEY (id);

ALTER TABLE ONLY org_invites
    ADD CONSTRAINT org_invites_pkey PRIMARY KEY (id);

ALTER TABLE ONLY org_memberships
    ADD CONSTRAINT org_memberships_pkey PRIMARY KEY (id);

ALTER TABLE ONLY orgs
    ADD CONSTRAINT orgs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY otp_codes
    ADD CONSTRAINT otp_codes_pkey PRIMARY KEY (id);

ALTER TABLE ONLY password_resets
    ADD CONSTRAINT password_resets_pkey PRIMARY KEY (id);

ALTER TABLE ONLY refresh_tokens
    ADD CONSTRAINT refresh_tokens_pkey PRIMARY KEY (id);

ALTER TABLE ONLY users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);

CREATE UNIQUE INDEX idx_email_verifications_token_hash ON email_verifications USING btree (token_hash);

CREATE INDEX idx_email_verifications_user_id ON email_verifications USING btree (user_id);

CREATE UNIQUE INDEX idx_membership_user_org ON org_memberships USING btree (user_id, org_id);

CREATE UNIQUE INDEX idx_oauth_exchange_tokens_token_hash ON oauth_exchange_tokens USING btree (token_hash);

CREATE INDEX idx_oauth_exchange_tokens_user_id ON oauth_exchange_tokens USING btree (user_id);

CREATE UNIQUE INDEX idx_oauth_provider_uid ON oauth_accounts USING btree (provider, provider_user_id);

CREATE UNIQUE INDEX idx_oauth_user_provider ON oauth_accounts USING btree (user_id, provider);

CREATE INDEX idx_org_invites_email ON org_invites USING btree (email);

CREATE INDEX idx_org_invites_org_id ON org_invites USING btree (org_id);

CREATE UNIQUE INDEX idx_org_invites_token_hash ON org_invites USING btree (token_hash);

CREATE UNIQUE INDEX idx_orgs_name ON orgs USING btree (name);

CREATE INDEX idx_otp_codes_email ON otp_codes USING btree (email);

CREATE UNIQUE INDEX idx_otp_codes_token_hash ON otp_codes USING btree (token_hash);

CREATE UNIQUE INDEX idx_password_resets_token_hash ON password_resets USING btree (token_hash);

CREATE INDEX idx_password_resets_user_id ON password_resets USING btree (user_id);

CREATE UNIQUE INDEX idx_refresh_tokens_token_hash ON refresh_tokens USING btree (token_hash);

CREATE INDEX idx_refresh_tokens_user_id ON refresh_tokens USING btree (user_id);

CREATE UNIQUE INDEX idx_users_email ON users USING btree (email);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'initial schema down migration is intentionally unsupported; reset or restore the database instead'; END $$;
-- +goose StatementEnd
