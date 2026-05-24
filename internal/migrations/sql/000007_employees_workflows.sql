-- +goose Up
-- Employee runtime, schedules, triggers, and generation tables

CREATE TABLE employee_memory_events (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    employee_id uuid NOT NULL,
    sandbox_id uuid NOT NULL,
    session_id character varying(255) NOT NULL,
    event_type character varying(128) NOT NULL,
    source character varying(128) DEFAULT 'manual'::character varying NOT NULL,
    payload jsonb DEFAULT '{}'::jsonb NOT NULL,
    event_at timestamp with time zone NOT NULL,
    retained_at timestamp with time zone,
    created_at timestamp with time zone
);

CREATE TABLE employee_sandbox_upgrades (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    employee_id uuid NOT NULL,
    old_sandbox_id uuid,
    new_sandbox_id uuid,
    status character varying(32) DEFAULT 'queued'::character varying NOT NULL,
    phase character varying(64) DEFAULT 'queued'::character varying NOT NULL,
    backup_key text,
    backup_sha256 text,
    backup_bytes bigint DEFAULT 0 NOT NULL,
    error_message text,
    completed_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE employee_schedule_runs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    employee_id uuid NOT NULL,
    schedule_id uuid NOT NULL,
    sandbox_id uuid NOT NULL,
    bridge_job_id character varying(255) NOT NULL,
    run_key character varying(500) NOT NULL,
    status character varying(64) DEFAULT 'running'::character varying NOT NULL,
    scheduled_at timestamp with time zone,
    started_at timestamp with time zone,
    completed_at timestamp with time zone,
    duration_ms bigint,
    error text DEFAULT ''::text NOT NULL,
    event_payload jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE employee_schedules (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    employee_id uuid NOT NULL,
    sandbox_id uuid NOT NULL,
    bridge_job_id character varying(255) NOT NULL,
    status character varying(64) DEFAULT 'active'::character varying NOT NULL,
    channel character varying(255) DEFAULT ''::character varying NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    task_prompt text DEFAULT ''::text NOT NULL,
    interval_seconds bigint,
    repeat_count bigint,
    repeat_completed bigint DEFAULT 0 NOT NULL,
    next_run_at timestamp with time zone,
    last_run_at timestamp with time zone,
    last_status character varying(64) DEFAULT ''::character varying NOT NULL,
    last_error text DEFAULT ''::text NOT NULL,
    created_by_session character varying(255) DEFAULT ''::character varying NOT NULL,
    bridge_created_at timestamp with time zone,
    cancelled_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE employee_sessions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    employee_id uuid NOT NULL,
    sandbox_id uuid NOT NULL,
    runtime_conversation_id text NOT NULL,
    source text DEFAULT ''::text NOT NULL,
    source_id uuid,
    source_resource_key text DEFAULT ''::text NOT NULL,
    credential_id uuid,
    token_id uuid,
    status text DEFAULT 'active'::text NOT NULL,
    name text,
    integration_scopes jsonb DEFAULT '{}'::jsonb,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    ended_at timestamp with time zone
);

CREATE TABLE employee_skills (
    employee_id uuid NOT NULL,
    skill_id uuid NOT NULL,
    created_at timestamp with time zone
);

CREATE TABLE employee_trigger_deliveries (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    employee_id uuid NOT NULL,
    trigger_id uuid NOT NULL,
    connection_id uuid,
    delivery_id text NOT NULL,
    event_key text DEFAULT ''::text NOT NULL,
    resource_key text DEFAULT ''::text NOT NULL,
    conversation_id uuid NOT NULL,
    runtime_conversation_id text DEFAULT ''::text NOT NULL,
    runtime_session_id text DEFAULT ''::text NOT NULL,
    runtime_stream_id text DEFAULT ''::text NOT NULL,
    runtime_trace_id text DEFAULT ''::text NOT NULL,
    runtime_turn_id text DEFAULT ''::text NOT NULL,
    payload jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone
);

CREATE TABLE employee_triggers (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    employee_id uuid NOT NULL,
    trigger_type character varying(32) DEFAULT 'webhook'::character varying NOT NULL,
    connection_id uuid,
    trigger_keys text[] DEFAULT '{}'::text[] NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    conditions jsonb,
    instructions text DEFAULT ''::text NOT NULL,
    secret_key text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE employees (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid,
    credential_id uuid,
    sandbox_template_id uuid,
    instructions text,
    model text NOT NULL,
    tools jsonb DEFAULT '{}'::jsonb NOT NULL,
    mcp_servers jsonb DEFAULT '{}'::jsonb NOT NULL,
    skills jsonb DEFAULT '{}'::jsonb NOT NULL,
    runtime_config jsonb DEFAULT '{}'::jsonb NOT NULL,
    permissions jsonb DEFAULT '{}'::jsonb NOT NULL,
    resources jsonb DEFAULT '{}'::jsonb NOT NULL,
    shared_memory boolean DEFAULT false NOT NULL,
    attached_specialists text[] DEFAULT '{}'::text[] NOT NULL,
    sandbox_tools text[] DEFAULT '{}'::text[],
    setup_commands text[] DEFAULT '{}'::text[],
    encrypted_env_vars bytea,
    status text DEFAULT 'active'::text NOT NULL,
    last_memory_refreshed_at timestamp with time zone,
    memory_refresh_status character varying(32) DEFAULT ''::character varying NOT NULL,
    memory_refresh_error text DEFAULT ''::text NOT NULL,
    last_proxy_token_refreshed_at timestamp with time zone,
    harness character varying(32) DEFAULT ''::character varying NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE failed_events (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    trigger_id uuid NOT NULL,
    event_type text NOT NULL,
    payload jsonb NOT NULL,
    error text NOT NULL,
    attempt_count bigint NOT NULL,
    failed_at timestamp with time zone NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    retried_at timestamp with time zone,
    retried_task_id text
);

CREATE TABLE generations (
    id text NOT NULL,
    org_id uuid NOT NULL,
    credential_id uuid NOT NULL,
    token_jti text NOT NULL,
    provider_id text NOT NULL,
    model text,
    request_path text,
    is_streaming boolean DEFAULT false,
    input_tokens bigint DEFAULT 0,
    output_tokens bigint DEFAULT 0,
    cached_tokens bigint DEFAULT 0,
    reasoning_tokens bigint DEFAULT 0,
    cost numeric(12,8) DEFAULT 0,
    ttfb_ms bigint,
    total_ms bigint,
    upstream_status bigint,
    user_id text,
    tags text[],
    error_type text,
    error_message text,
    ip_address inet,
    created_at timestamp with time zone NOT NULL,
    is_system boolean DEFAULT false NOT NULL,
    billed_at timestamp with time zone,
    billing_error text
);

CREATE TABLE hindsight_banks (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    employee_id uuid,
    bank_id text NOT NULL,
    config_hash text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

ALTER TABLE ONLY employee_memory_events
    ADD CONSTRAINT employee_memory_events_pkey PRIMARY KEY (id);

ALTER TABLE ONLY employee_sandbox_upgrades
    ADD CONSTRAINT employee_sandbox_upgrades_pkey PRIMARY KEY (id);

ALTER TABLE ONLY employee_schedule_runs
    ADD CONSTRAINT employee_schedule_runs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY employee_schedules
    ADD CONSTRAINT employee_schedules_pkey PRIMARY KEY (id);

ALTER TABLE ONLY employee_sessions
    ADD CONSTRAINT employee_sessions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY employee_skills
    ADD CONSTRAINT employee_skills_pkey PRIMARY KEY (employee_id, skill_id);

ALTER TABLE ONLY employee_trigger_deliveries
    ADD CONSTRAINT employee_trigger_deliveries_pkey PRIMARY KEY (id);

ALTER TABLE ONLY employee_triggers
    ADD CONSTRAINT employee_triggers_pkey PRIMARY KEY (id);

ALTER TABLE ONLY employees
    ADD CONSTRAINT employees_pkey PRIMARY KEY (id);

ALTER TABLE ONLY failed_events
    ADD CONSTRAINT failed_events_pkey PRIMARY KEY (id);

ALTER TABLE ONLY generations
    ADD CONSTRAINT generations_pkey PRIMARY KEY (id);

ALTER TABLE ONLY hindsight_banks
    ADD CONSTRAINT hindsight_banks_pkey PRIMARY KEY (id);

CREATE INDEX idx_employee_memory_events_event_at ON employee_memory_events USING btree (event_at);

CREATE INDEX idx_employee_memory_events_event_type ON employee_memory_events USING btree (event_type);

CREATE INDEX idx_employee_memory_events_retained_at ON employee_memory_events USING btree (retained_at);

CREATE INDEX idx_employee_memory_events_sandbox_id ON employee_memory_events USING btree (sandbox_id);

CREATE INDEX idx_employee_memory_scope ON employee_memory_events USING btree (org_id, employee_id, session_id);

CREATE INDEX idx_employee_org_id ON employees USING btree (org_id);

CREATE INDEX idx_employee_sandbox_upgrades_employee_id ON employee_sandbox_upgrades USING btree (employee_id);

CREATE INDEX idx_employee_sandbox_upgrades_new_sandbox_id ON employee_sandbox_upgrades USING btree (new_sandbox_id);

CREATE INDEX idx_employee_sandbox_upgrades_old_sandbox_id ON employee_sandbox_upgrades USING btree (old_sandbox_id);

CREATE INDEX idx_employee_sandbox_upgrades_org_id ON employee_sandbox_upgrades USING btree (org_id);

CREATE INDEX idx_employee_sandbox_upgrades_status ON employee_sandbox_upgrades USING btree (status);

CREATE UNIQUE INDEX idx_employee_schedule_employee_bridge ON employee_schedules USING btree (employee_id, bridge_job_id);

CREATE UNIQUE INDEX idx_employee_schedule_run_key ON employee_schedule_runs USING btree (schedule_id, run_key);

CREATE INDEX idx_employee_schedule_runs_bridge_job_id ON employee_schedule_runs USING btree (bridge_job_id);

CREATE INDEX idx_employee_schedule_runs_employee_id ON employee_schedule_runs USING btree (employee_id);

CREATE INDEX idx_employee_schedule_runs_org_id ON employee_schedule_runs USING btree (org_id);

CREATE INDEX idx_employee_schedule_runs_sandbox_id ON employee_schedule_runs USING btree (sandbox_id);

CREATE INDEX idx_employee_schedule_runs_scheduled_at ON employee_schedule_runs USING btree (scheduled_at);

CREATE INDEX idx_employee_schedule_runs_status ON employee_schedule_runs USING btree (status);

CREATE INDEX idx_employee_schedules_cancelled_at ON employee_schedules USING btree (cancelled_at);

CREATE INDEX idx_employee_schedules_next_run_at ON employee_schedules USING btree (next_run_at);

CREATE INDEX idx_employee_schedules_org_id ON employee_schedules USING btree (org_id);

CREATE INDEX idx_employee_schedules_sandbox_id ON employee_schedules USING btree (sandbox_id);

CREATE INDEX idx_employee_schedules_status ON employee_schedules USING btree (status);

CREATE INDEX idx_employee_session_org_employee ON employee_sessions USING btree (org_id, employee_id);

CREATE INDEX idx_employee_sessions_credential_id ON employee_sessions USING btree (credential_id);

CREATE INDEX idx_employee_sessions_runtime_conversation_id ON employee_sessions USING btree (runtime_conversation_id);

CREATE INDEX idx_employee_sessions_source ON employee_sessions USING btree (source);

CREATE INDEX idx_employee_sessions_source_id ON employee_sessions USING btree (source_id);

CREATE INDEX idx_employee_sessions_source_resource_key ON employee_sessions USING btree (source_resource_key);

CREATE INDEX idx_employee_trigger_deliveries_connection_id ON employee_trigger_deliveries USING btree (connection_id);

CREATE INDEX idx_employee_trigger_deliveries_conversation_id ON employee_trigger_deliveries USING btree (conversation_id);

CREATE INDEX idx_employee_trigger_deliveries_delivery_id ON employee_trigger_deliveries USING btree (delivery_id);

CREATE INDEX idx_employee_trigger_deliveries_event_key ON employee_trigger_deliveries USING btree (event_key);

CREATE INDEX idx_employee_trigger_deliveries_resource_key ON employee_trigger_deliveries USING btree (resource_key);

CREATE INDEX idx_employee_trigger_deliveries_runtime_conversation_id ON employee_trigger_deliveries USING btree (runtime_conversation_id);

CREATE INDEX idx_employee_trigger_deliveries_runtime_session_id ON employee_trigger_deliveries USING btree (runtime_session_id);

CREATE INDEX idx_employee_trigger_deliveries_trigger_id ON employee_trigger_deliveries USING btree (trigger_id);

CREATE INDEX idx_employee_triggers_connection_id ON employee_triggers USING btree (connection_id);

CREATE INDEX idx_employee_triggers_employee_id ON employee_triggers USING btree (employee_id);

CREATE INDEX idx_employee_triggers_org_id ON employee_triggers USING btree (org_id);

CREATE INDEX idx_employee_triggers_trigger_type ON employee_triggers USING btree (trigger_type);

CREATE INDEX idx_employees_credential_id ON employees USING btree (credential_id);

CREATE INDEX idx_failed_events_event_type ON failed_events USING btree (event_type);

CREATE INDEX idx_failed_events_failed_at ON failed_events USING btree (failed_at);

CREATE INDEX idx_failed_events_org_id ON failed_events USING btree (org_id);

CREATE INDEX idx_failed_events_status ON failed_events USING btree (status);

CREATE INDEX idx_failed_events_trigger_id ON failed_events USING btree (trigger_id);

CREATE INDEX idx_gen_org_created ON generations USING btree (org_id, created_at);

CREATE INDEX idx_gen_org_credential ON generations USING btree (credential_id);

CREATE INDEX idx_gen_org_model ON generations USING btree (model);

CREATE INDEX idx_gen_org_provider ON generations USING btree (provider_id);

CREATE INDEX idx_gen_org_user ON generations USING btree (user_id);

CREATE UNIQUE INDEX idx_hindsight_banks_bank_id ON hindsight_banks USING btree (bank_id);

CREATE INDEX idx_hindsight_banks_employee_id ON hindsight_banks USING btree (employee_id);

CREATE INDEX idx_trigger_delivery_org_employee_created ON employee_trigger_deliveries USING btree (org_id, employee_id, created_at);

CREATE INDEX idx_trigger_delivery_org_employee_session_created ON employee_trigger_deliveries USING btree (org_id, employee_id, runtime_session_id, created_at);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'initial schema down migration is intentionally unsupported; reset or restore the database instead'; END $$;
-- +goose StatementEnd
