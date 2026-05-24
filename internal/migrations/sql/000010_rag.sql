-- +goose Up
-- RAG source, indexing, identity, and search tables

CREATE TABLE rag_embedding_models (
    id text NOT NULL,
    provider text NOT NULL,
    model_name text NOT NULL,
    dimension bigint NOT NULL,
    max_input_tokens bigint NOT NULL,
    dataset_name text NOT NULL,
    query_prefix text,
    passage_prefix text,
    pricing_per_1m_tokens_usd numeric NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE rag_external_identities (
    id bigint NOT NULL,
    org_id uuid NOT NULL,
    user_id uuid NOT NULL,
    rag_source_id uuid NOT NULL,
    provider text NOT NULL,
    external_user_id text NOT NULL,
    external_user_login text,
    external_user_emails text[],
    updated_at timestamp with time zone
);

CREATE SEQUENCE rag_external_identities_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE rag_external_identities_id_seq OWNED BY rag_external_identities.id;

CREATE TABLE rag_external_user_groups (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    rag_source_id uuid NOT NULL,
    external_user_group_id text NOT NULL,
    display_name text NOT NULL,
    gives_anyone_access boolean DEFAULT false NOT NULL,
    member_emails text[],
    updated_at timestamp with time zone
);

CREATE TABLE rag_index_attempt_errors (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    index_attempt_id uuid NOT NULL,
    rag_source_id uuid NOT NULL,
    document_id text,
    document_link text,
    entity_id text,
    failed_time_range_start timestamp with time zone,
    failed_time_range_end timestamp with time zone,
    failure_message text NOT NULL,
    is_resolved boolean DEFAULT false NOT NULL,
    error_type text,
    time_created timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);

CREATE TABLE rag_index_attempts (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    rag_source_id uuid NOT NULL,
    embedding_model_id text,
    from_beginning boolean DEFAULT false NOT NULL,
    status text NOT NULL,
    new_docs_indexed bigint DEFAULT 0,
    total_docs_indexed bigint DEFAULT 0,
    docs_removed_from_index bigint DEFAULT 0,
    docs_estimated integer,
    error_msg text,
    full_exception_trace text,
    poll_range_start timestamp with time zone,
    poll_range_end timestamp with time zone,
    checkpoint_pointer text,
    celery_task_id text,
    cancellation_requested boolean DEFAULT false NOT NULL,
    total_batches bigint,
    completed_batches bigint DEFAULT 0 NOT NULL,
    total_failures_batch_level bigint DEFAULT 0 NOT NULL,
    total_chunks bigint DEFAULT 0 NOT NULL,
    last_progress_time timestamp with time zone,
    last_batches_completed_count bigint DEFAULT 0 NOT NULL,
    heartbeat_counter bigint DEFAULT 0 NOT NULL,
    last_heartbeat_value bigint DEFAULT 0 NOT NULL,
    last_heartbeat_time timestamp with time zone,
    time_created timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    time_started timestamp with time zone,
    time_updated timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);

CREATE TABLE rag_public_external_user_groups (
    external_user_group_id text NOT NULL,
    rag_source_id uuid NOT NULL,
    stale boolean DEFAULT false NOT NULL
);

CREATE TABLE rag_search_settings (
    org_id uuid NOT NULL,
    embedding_model_id character varying(128) NOT NULL,
    embedding_dim bigint NOT NULL,
    "normalize" boolean DEFAULT true NOT NULL,
    query_prefix text,
    passage_prefix text,
    embedding_precision character varying(16) DEFAULT 'float'::character varying NOT NULL,
    reduced_dimension integer,
    multipass_indexing boolean DEFAULT true NOT NULL,
    reranker_model_id character varying(128),
    hybrid_alpha double precision DEFAULT 0.7 NOT NULL,
    index_name character varying(256) NOT NULL,
    enable_contextual_rag boolean DEFAULT false NOT NULL,
    contextual_ragllm_name character varying(128),
    contextual_ragllm_provider character varying(64),
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE rag_sources (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    kind character varying(32) NOT NULL,
    name text NOT NULL,
    status character varying(32) NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    config jsonb DEFAULT '{}'::jsonb NOT NULL,
    connection_id uuid,
    access_type character varying(16) NOT NULL,
    indexing_start timestamp with time zone,
    last_successful_index_time timestamp with time zone,
    last_time_perm_sync timestamp with time zone,
    last_pruned timestamp with time zone,
    refresh_freq_seconds integer,
    prune_freq_seconds integer,
    perm_sync_freq_seconds integer,
    total_docs_indexed bigint DEFAULT 0 NOT NULL,
    in_repeated_error_state boolean DEFAULT false NOT NULL,
    deletion_failure_message text,
    creator_id uuid,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE rag_sync_records (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    entity_id uuid NOT NULL,
    sync_type text NOT NULL,
    sync_status text NOT NULL,
    num_docs_synced bigint DEFAULT 0 NOT NULL,
    sync_start_time timestamp with time zone NOT NULL,
    sync_end_time timestamp with time zone
);

CREATE TABLE rag_sync_states (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    rag_source_id uuid NOT NULL,
    status character varying(32) NOT NULL,
    in_repeated_error_state boolean DEFAULT false NOT NULL,
    access_type character varying(16) NOT NULL,
    auto_sync_options jsonb,
    last_time_perm_sync timestamp with time zone,
    last_time_external_group_sync timestamp with time zone,
    last_successful_index_time timestamp with time zone,
    last_pruned timestamp with time zone,
    last_time_hierarchy_fetch timestamp with time zone,
    total_docs_indexed bigint DEFAULT 0 NOT NULL,
    indexing_trigger character varying(16),
    processing_mode character varying(16) DEFAULT 'REGULAR'::character varying NOT NULL,
    deletion_failure_message text,
    creator_id uuid,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE rag_user_external_user_groups (
    user_id uuid NOT NULL,
    external_user_group_id text NOT NULL,
    rag_source_id uuid NOT NULL,
    stale boolean DEFAULT false NOT NULL
);

ALTER TABLE ONLY rag_external_identities ALTER COLUMN id SET DEFAULT nextval('rag_external_identities_id_seq'::regclass);

ALTER TABLE ONLY rag_embedding_models
    ADD CONSTRAINT rag_embedding_models_pkey PRIMARY KEY (id);

ALTER TABLE ONLY rag_external_identities
    ADD CONSTRAINT rag_external_identities_pkey PRIMARY KEY (id);

ALTER TABLE ONLY rag_external_user_groups
    ADD CONSTRAINT rag_external_user_groups_pkey PRIMARY KEY (id);

ALTER TABLE ONLY rag_index_attempt_errors
    ADD CONSTRAINT rag_index_attempt_errors_pkey PRIMARY KEY (id);

ALTER TABLE ONLY rag_index_attempts
    ADD CONSTRAINT rag_index_attempts_pkey PRIMARY KEY (id);

ALTER TABLE ONLY rag_public_external_user_groups
    ADD CONSTRAINT rag_public_external_user_groups_pkey PRIMARY KEY (external_user_group_id, rag_source_id);

ALTER TABLE ONLY rag_search_settings
    ADD CONSTRAINT rag_search_settings_pkey PRIMARY KEY (org_id);

ALTER TABLE ONLY rag_sources
    ADD CONSTRAINT rag_sources_pkey PRIMARY KEY (id);

ALTER TABLE ONLY rag_sync_records
    ADD CONSTRAINT rag_sync_records_pkey PRIMARY KEY (id);

ALTER TABLE ONLY rag_sync_states
    ADD CONSTRAINT rag_sync_states_pkey PRIMARY KEY (id);

ALTER TABLE ONLY rag_user_external_user_groups
    ADD CONSTRAINT rag_user_external_user_groups_pkey PRIMARY KEY (user_id, external_user_group_id, rag_source_id);

CREATE INDEX idx_rag_external_identity_org ON rag_external_identities USING btree (org_id);

CREATE INDEX idx_rag_external_identity_source ON rag_external_identities USING btree (rag_source_id);

CREATE INDEX idx_rag_external_user_groups_org_id ON rag_external_user_groups USING btree (org_id);

CREATE INDEX idx_rag_index_attempt_errors_index_attempt_id ON rag_index_attempt_errors USING btree (index_attempt_id);

CREATE INDEX idx_rag_index_attempt_errors_org_id ON rag_index_attempt_errors USING btree (org_id);

CREATE INDEX idx_rag_index_attempt_errors_rag_source_id ON rag_index_attempt_errors USING btree (rag_source_id);

CREATE INDEX idx_rag_index_attempts_org_id ON rag_index_attempts USING btree (org_id);

CREATE INDEX idx_rag_index_attempts_rag_source_id ON rag_index_attempts USING btree (rag_source_id);

CREATE INDEX idx_rag_index_attempts_status ON rag_index_attempts USING btree (status);

CREATE INDEX idx_rag_index_attempts_time_created ON rag_index_attempts USING btree (time_created);

CREATE INDEX idx_rag_search_settings_embedding_model_id ON rag_search_settings USING btree (embedding_model_id);

CREATE INDEX idx_rag_sync_records_org_id ON rag_sync_records USING btree (org_id);

CREATE INDEX idx_rag_sync_state_last_pruned ON rag_sync_states USING btree (last_pruned);

CREATE INDEX idx_rag_sync_states_org_id ON rag_sync_states USING btree (org_id);

CREATE UNIQUE INDEX uq_rag_external_identity_provider_ext_id_org ON rag_external_identities USING btree (org_id, provider, external_user_id);

CREATE UNIQUE INDEX uq_rag_external_identity_user_source ON rag_external_identities USING btree (user_id, rag_source_id);

CREATE UNIQUE INDEX uq_rag_external_user_group_source_ext ON rag_external_user_groups USING btree (rag_source_id, external_user_group_id);

CREATE UNIQUE INDEX uq_rag_sync_state_rag_source_id ON rag_sync_states USING btree (rag_source_id);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'initial schema down migration is intentionally unsupported; reset or restore the database instead'; END $$;
-- +goose StatementEnd
