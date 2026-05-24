-- +goose Up
-- Billing, subscriptions, credits, and usage tables

CREATE TABLE credit_ledger_entries (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    amount bigint NOT NULL,
    reason character varying(64) NOT NULL,
    ref_type character varying(64),
    ref_id character varying(64),
    expires_at timestamp with time zone,
    created_at timestamp with time zone
);

CREATE TABLE plans (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    slug character varying(64) NOT NULL,
    name character varying(128) NOT NULL,
    provider character varying(32) DEFAULT ''::character varying NOT NULL,
    features jsonb,
    monthly_credits bigint DEFAULT 0 NOT NULL,
    welcome_credits bigint DEFAULT 0 NOT NULL,
    price_cents bigint DEFAULT 0 NOT NULL,
    currency character varying(8) DEFAULT 'USD'::character varying NOT NULL,
    active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE subscription_change_quotes (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    subscription_id uuid NOT NULL,
    from_plan_id uuid NOT NULL,
    to_plan_id uuid NOT NULL,
    kind character varying(16) NOT NULL,
    amount_minor bigint NOT NULL,
    currency character varying(8) NOT NULL,
    proration_credit_minor bigint DEFAULT 0 NOT NULL,
    effective_at timestamp with time zone NOT NULL,
    paystack_reference character varying(128),
    expires_at timestamp with time zone NOT NULL,
    consumed_at timestamp with time zone,
    created_at timestamp with time zone
);

CREATE TABLE subscriptions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    plan_id uuid NOT NULL,
    provider character varying(32) NOT NULL,
    external_customer_id character varying(128) NOT NULL,
    status character varying(32) DEFAULT 'active'::character varying NOT NULL,
    current_period_start timestamp with time zone,
    current_period_end timestamp with time zone,
    canceled_at timestamp with time zone,
    cancel_at_period_end boolean DEFAULT false NOT NULL,
    pending_plan_id uuid,
    pending_change_at timestamp with time zone,
    renewal_attempts bigint DEFAULT 0 NOT NULL,
    last_renewal_attempt_at timestamp with time zone,
    last_renewal_error character varying(512) DEFAULT ''::character varying NOT NULL,
    payment_channel character varying(16) DEFAULT ''::character varying NOT NULL,
    payment_bank_name character varying(64) DEFAULT ''::character varying NOT NULL,
    payment_account_name character varying(128) DEFAULT ''::character varying NOT NULL,
    last_charge_reference character varying(128) DEFAULT ''::character varying NOT NULL,
    last_charge_amount bigint DEFAULT 0 NOT NULL,
    last_charged_at timestamp with time zone,
    card_last4 character varying(4) DEFAULT ''::character varying NOT NULL,
    card_brand character varying(32) DEFAULT ''::character varying NOT NULL,
    card_exp_month character varying(2) DEFAULT ''::character varying NOT NULL,
    card_exp_year character varying(4) DEFAULT ''::character varying NOT NULL,
    authorization_code character varying(128) DEFAULT ''::character varying NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE tool_usages (
    id text NOT NULL,
    org_id uuid NOT NULL,
    employee_id text NOT NULL,
    token_jti text NOT NULL,
    tool_name text NOT NULL,
    input text,
    pages_returned bigint DEFAULT 0,
    status text NOT NULL,
    error_message text,
    total_ms bigint,
    credits_used bigint DEFAULT 0,
    ip_address inet,
    created_at timestamp with time zone NOT NULL
);

CREATE TABLE usage (
    id bigint NOT NULL,
    org_id uuid NOT NULL,
    credential_id uuid NOT NULL,
    request_count bigint DEFAULT 0 NOT NULL,
    period_start timestamp with time zone NOT NULL,
    period_end timestamp with time zone NOT NULL,
    created_at timestamp with time zone
);

CREATE SEQUENCE usage_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE usage_id_seq OWNED BY usage.id;

ALTER TABLE ONLY usage ALTER COLUMN id SET DEFAULT nextval('usage_id_seq'::regclass);

ALTER TABLE ONLY credit_ledger_entries
    ADD CONSTRAINT credit_ledger_entries_pkey PRIMARY KEY (id);

ALTER TABLE ONLY plans
    ADD CONSTRAINT plans_pkey PRIMARY KEY (id);

ALTER TABLE ONLY subscription_change_quotes
    ADD CONSTRAINT subscription_change_quotes_pkey PRIMARY KEY (id);

ALTER TABLE ONLY subscriptions
    ADD CONSTRAINT subscriptions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY tool_usages
    ADD CONSTRAINT tool_usages_pkey PRIMARY KEY (id);

ALTER TABLE ONLY usage
    ADD CONSTRAINT usage_pkey PRIMARY KEY (id);

CREATE INDEX idx_credit_ledger_entries_expires_at ON credit_ledger_entries USING btree (expires_at);

CREATE INDEX idx_credit_ledger_entries_org_id ON credit_ledger_entries USING btree (org_id);

CREATE INDEX idx_credit_ledger_entries_ref_id ON credit_ledger_entries USING btree (ref_id);

CREATE INDEX idx_plans_provider ON plans USING btree (provider);

CREATE UNIQUE INDEX idx_plans_slug ON plans USING btree (slug);

CREATE INDEX idx_subscription_change_quotes_expires_at ON subscription_change_quotes USING btree (expires_at);

CREATE INDEX idx_subscription_change_quotes_org_id ON subscription_change_quotes USING btree (org_id);

CREATE UNIQUE INDEX idx_subscription_change_quotes_paystack_reference ON subscription_change_quotes USING btree (paystack_reference);

CREATE INDEX idx_subscription_change_quotes_subscription_id ON subscription_change_quotes USING btree (subscription_id);

CREATE INDEX idx_subscriptions_external_customer_id ON subscriptions USING btree (external_customer_id);

CREATE INDEX idx_subscriptions_org_id ON subscriptions USING btree (org_id);

CREATE INDEX idx_subscriptions_plan_id ON subscriptions USING btree (plan_id);

CREATE INDEX idx_tu_org_agent ON tool_usages USING btree (employee_id);

CREATE INDEX idx_tu_org_created ON tool_usages USING btree (org_id, created_at);

CREATE UNIQUE INDEX idx_usage_unique ON usage USING btree (org_id, credential_id, period_start);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'initial schema down migration is intentionally unsupported; reset or restore the database instead'; END $$;
-- +goose StatementEnd
