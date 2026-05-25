-- +goose Up
-- Provider-neutral employee gateway routes, inbound events, and outbound deliveries.

CREATE TABLE employee_gateway_routes (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    employee_id uuid NOT NULL,
    connection_id uuid,
    provider character varying(128) NOT NULL,
    name text NOT NULL DEFAULT '',
    enabled boolean NOT NULL DEFAULT true,
    config jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    revoked_at timestamp with time zone
);

ALTER TABLE ONLY employee_gateway_routes
    ADD CONSTRAINT employee_gateway_routes_pkey PRIMARY KEY (id);

CREATE INDEX idx_employee_gateway_routes_org_employee ON employee_gateway_routes USING btree (org_id, employee_id);
CREATE INDEX idx_employee_gateway_routes_connection_id ON employee_gateway_routes USING btree (connection_id);
CREATE INDEX idx_employee_gateway_routes_provider ON employee_gateway_routes USING btree (provider);
CREATE INDEX idx_employee_gateway_routes_enabled ON employee_gateway_routes USING btree (org_id, enabled);

ALTER TABLE ONLY employee_gateway_routes
    ADD CONSTRAINT fk_employee_gateway_routes_org FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;
ALTER TABLE ONLY employee_gateway_routes
    ADD CONSTRAINT fk_employee_gateway_routes_employee FOREIGN KEY (employee_id) REFERENCES employees(id) ON DELETE CASCADE;
ALTER TABLE ONLY employee_gateway_routes
    ADD CONSTRAINT fk_employee_gateway_routes_connection FOREIGN KEY (connection_id) REFERENCES connections(id) ON DELETE SET NULL;

CREATE TABLE employee_gateway_events (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    employee_id uuid NOT NULL,
    route_id uuid NOT NULL,
    employee_session_id uuid,
    provider character varying(128) NOT NULL,
    external_message_id text NOT NULL DEFAULT '',
    dedupe_key text NOT NULL DEFAULT '',
    thread_key text NOT NULL DEFAULT '',
    channel_id text NOT NULL DEFAULT '',
    thread_id text NOT NULL DEFAULT '',
    sender_id text NOT NULL DEFAULT '',
    status character varying(32) NOT NULL DEFAULT 'received',
    error text NOT NULL DEFAULT '',
    runtime_conversation_id text NOT NULL DEFAULT '',
    runtime_session_id text NOT NULL DEFAULT '',
    runtime_stream_id text NOT NULL DEFAULT '',
    runtime_trace_id text NOT NULL DEFAULT '',
    runtime_turn_id text NOT NULL DEFAULT '',
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    received_at timestamp with time zone NOT NULL DEFAULT now(),
    processed_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

ALTER TABLE ONLY employee_gateway_events
    ADD CONSTRAINT employee_gateway_events_pkey PRIMARY KEY (id);

CREATE INDEX idx_employee_gateway_events_route_received ON employee_gateway_events USING btree (route_id, received_at);
CREATE INDEX idx_employee_gateway_events_org_employee_received ON employee_gateway_events USING btree (org_id, employee_id, received_at);
CREATE INDEX idx_employee_gateway_events_status_received ON employee_gateway_events USING btree (status, received_at);
CREATE INDEX idx_employee_gateway_events_session_id ON employee_gateway_events USING btree (employee_session_id);
CREATE UNIQUE INDEX idx_employee_gateway_events_route_dedupe ON employee_gateway_events USING btree (route_id, dedupe_key) WHERE dedupe_key <> '';

ALTER TABLE ONLY employee_gateway_events
    ADD CONSTRAINT fk_employee_gateway_events_org FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;
ALTER TABLE ONLY employee_gateway_events
    ADD CONSTRAINT fk_employee_gateway_events_employee FOREIGN KEY (employee_id) REFERENCES employees(id) ON DELETE CASCADE;
ALTER TABLE ONLY employee_gateway_events
    ADD CONSTRAINT fk_employee_gateway_events_route FOREIGN KEY (route_id) REFERENCES employee_gateway_routes(id) ON DELETE CASCADE;
ALTER TABLE ONLY employee_gateway_events
    ADD CONSTRAINT fk_employee_gateway_events_session FOREIGN KEY (employee_session_id) REFERENCES employee_sessions(id) ON DELETE SET NULL;

CREATE TABLE employee_gateway_deliveries (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    employee_id uuid NOT NULL,
    route_id uuid NOT NULL,
    employee_session_id uuid NOT NULL,
    provider character varying(128) NOT NULL,
    dedupe_key text NOT NULL DEFAULT '',
    runtime_session_id text NOT NULL DEFAULT '',
    runtime_trace_id text NOT NULL DEFAULT '',
    runtime_turn_id text NOT NULL DEFAULT '',
    thread_key text NOT NULL DEFAULT '',
    channel_id text NOT NULL DEFAULT '',
    thread_id text NOT NULL DEFAULT '',
    response_text text NOT NULL DEFAULT '',
    provider_handles jsonb NOT NULL DEFAULT '[]'::jsonb,
    status character varying(32) NOT NULL DEFAULT 'sent',
    error text NOT NULL DEFAULT '',
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

ALTER TABLE ONLY employee_gateway_deliveries
    ADD CONSTRAINT employee_gateway_deliveries_pkey PRIMARY KEY (id);

CREATE INDEX idx_employee_gateway_deliveries_route_created ON employee_gateway_deliveries USING btree (route_id, created_at);
CREATE INDEX idx_employee_gateway_deliveries_session_id ON employee_gateway_deliveries USING btree (employee_session_id);
CREATE UNIQUE INDEX idx_employee_gateway_deliveries_route_dedupe ON employee_gateway_deliveries USING btree (route_id, dedupe_key) WHERE dedupe_key <> '';

ALTER TABLE ONLY employee_gateway_deliveries
    ADD CONSTRAINT fk_employee_gateway_deliveries_org FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;
ALTER TABLE ONLY employee_gateway_deliveries
    ADD CONSTRAINT fk_employee_gateway_deliveries_employee FOREIGN KEY (employee_id) REFERENCES employees(id) ON DELETE CASCADE;
ALTER TABLE ONLY employee_gateway_deliveries
    ADD CONSTRAINT fk_employee_gateway_deliveries_route FOREIGN KEY (route_id) REFERENCES employee_gateway_routes(id) ON DELETE CASCADE;
ALTER TABLE ONLY employee_gateway_deliveries
    ADD CONSTRAINT fk_employee_gateway_deliveries_session FOREIGN KEY (employee_session_id) REFERENCES employee_sessions(id) ON DELETE CASCADE;

CREATE UNIQUE INDEX idx_employee_sessions_gateway_active_resource
    ON employee_sessions USING btree (org_id, employee_id, source, source_id, source_resource_key)
    WHERE status = 'active' AND source = 'gateway';

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'employee gateway migration down is intentionally unsupported; reset or restore the database instead'; END $$;
-- +goose StatementEnd
