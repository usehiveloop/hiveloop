-- +goose Up
-- Conversation event and artifact tables

CREATE TABLE public.conversation_assets (
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

CREATE TABLE public.conversation_events (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    conversation_id uuid NOT NULL,
    event_id text NOT NULL,
    event_type text NOT NULL,
    employee_id text NOT NULL,
    runtime_conversation_id text NOT NULL,
    "timestamp" timestamp with time zone NOT NULL,
    sequence_number bigint NOT NULL,
    data jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone
);

ALTER TABLE ONLY public.conversation_assets
    ADD CONSTRAINT conversation_assets_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.conversation_events
    ADD CONSTRAINT conversation_events_pkey PRIMARY KEY (id);

CREATE INDEX idx_conv_asset_conv_created ON public.conversation_assets USING btree (conversation_id, created_at DESC);

CREATE UNIQUE INDEX idx_conversation_assets_key ON public.conversation_assets USING btree (key);

CREATE INDEX idx_conversation_assets_org_id ON public.conversation_assets USING btree (org_id);

CREATE INDEX idx_conversation_events_employee_id ON public.conversation_events USING btree (employee_id);

CREATE INDEX idx_conversation_events_event_type ON public.conversation_events USING btree (event_type);

CREATE INDEX idx_conversation_events_org_id ON public.conversation_events USING btree (org_id);

CREATE INDEX idx_conversation_events_runtime_conversation_id ON public.conversation_events USING btree (runtime_conversation_id);

CREATE INDEX idx_event_conv_created ON public.conversation_events USING btree (conversation_id, created_at);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'initial schema down migration is intentionally unsupported; reset or restore the database instead'; END $$;
-- +goose StatementEnd
