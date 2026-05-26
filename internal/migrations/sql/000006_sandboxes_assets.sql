-- +goose Up
-- Sandbox, upload, and asset tables

CREATE TABLE custom_domains (
    id uuid NOT NULL,
    org_id uuid NOT NULL,
    domain character varying(255) NOT NULL,
    verified boolean DEFAULT false,
    verified_at timestamp with time zone,
    cname_target character varying(255) NOT NULL,
    acme_dns_subdomain character varying(255),
    acme_dns_username character varying(255),
    acme_dns_password character varying(255),
    acme_dns_server_url character varying(255),
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE drive_assets (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    employee_id uuid NOT NULL,
    filename text NOT NULL,
    content_type text NOT NULL,
    size bigint NOT NULL,
    s3_key text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE employee_assets (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    employee_id uuid NOT NULL,
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

CREATE TABLE sandbox_templates (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    slug text NOT NULL,
    tags jsonb DEFAULT '[]'::jsonb NOT NULL,
    size text DEFAULT 'medium'::text NOT NULL,
    base_template_id uuid,
    build_commands text DEFAULT ''::text NOT NULL,
    provider_id text DEFAULT 'daytona'::text NOT NULL,
    external_id text,
    base_image_ref text,
    build_status text DEFAULT 'pending'::text NOT NULL,
    build_error text,
    build_logs text DEFAULT ''::text NOT NULL,
    config jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE sandboxes (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid,
    employee_id uuid,
    sandbox_template_id uuid,
    snapshot_id text,
    provider_id text DEFAULT 'daytona'::text NOT NULL,
    external_id text NOT NULL,
    runtime_url text NOT NULL,
    runtime_url_expires_at timestamp with time zone,
    encrypted_runtime_secret bytea NOT NULL,
    status text DEFAULT 'creating'::text NOT NULL,
    error_message text,
    last_active_at timestamp with time zone,
    stopped_at timestamp with time zone,
    memory_limit_bytes bigint DEFAULT 0 NOT NULL,
    memory_used_bytes bigint DEFAULT 0 NOT NULL,
    memory_peak_bytes bigint DEFAULT 0 NOT NULL,
    cpu_quota text DEFAULT ''::text NOT NULL,
    cpu_usage_usec bigint DEFAULT 0 NOT NULL,
    cpu_throttled_count bigint DEFAULT 0 NOT NULL,
    pid_count bigint DEFAULT 0 NOT NULL,
    resource_checked_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

ALTER TABLE ONLY custom_domains
    ADD CONSTRAINT custom_domains_pkey PRIMARY KEY (id);

ALTER TABLE ONLY drive_assets
    ADD CONSTRAINT drive_assets_pkey PRIMARY KEY (id);

ALTER TABLE ONLY employee_assets
    ADD CONSTRAINT employee_assets_pkey PRIMARY KEY (id);

ALTER TABLE ONLY sandbox_templates
    ADD CONSTRAINT sandbox_templates_pkey PRIMARY KEY (id);

ALTER TABLE ONLY sandboxes
    ADD CONSTRAINT sandboxes_pkey PRIMARY KEY (id);

CREATE UNIQUE INDEX idx_custom_domains_domain ON custom_domains USING btree (domain);

CREATE INDEX idx_custom_domains_org_id ON custom_domains USING btree (org_id);

CREATE INDEX idx_drive_asset_employee ON drive_assets USING btree (employee_id);

CREATE INDEX idx_drive_asset_org ON drive_assets USING btree (org_id);

CREATE UNIQUE INDEX idx_drive_assets_s3_key ON drive_assets USING btree (s3_key);

CREATE INDEX idx_emp_asset_employee_created ON employee_assets USING btree (employee_id, created_at DESC);

CREATE UNIQUE INDEX idx_employee_assets_key ON employee_assets USING btree (key);

CREATE INDEX idx_employee_assets_org_id ON employee_assets USING btree (org_id);

CREATE INDEX idx_sandbox_templates_base_template_id ON sandbox_templates USING btree (base_template_id);

CREATE INDEX idx_sandbox_templates_org_id ON sandbox_templates USING btree (org_id);

CREATE UNIQUE INDEX idx_sandbox_templates_slug ON sandbox_templates USING btree (slug);

CREATE INDEX idx_sandboxes_employee_id ON sandboxes USING btree (employee_id);

CREATE INDEX idx_sandboxes_org_id ON sandboxes USING btree (org_id);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'initial schema down migration is intentionally unsupported; reset or restore the database instead'; END $$;
-- +goose StatementEnd
