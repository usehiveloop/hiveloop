-- +goose Up
-- Skill catalog and specialist task tables

CREATE TABLE skills (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid,
    publisher_id uuid,
    slug text NOT NULL,
    name text NOT NULL,
    description text,
    category character varying(64) DEFAULT ''::character varying NOT NULL,
    source_type text NOT NULL,
    repo_url text,
    repo_subpath text,
    repo_ref text DEFAULT 'main'::text NOT NULL,
    bundle jsonb DEFAULT '{}'::jsonb NOT NULL,
    hydrated_commit_sha text,
    hydrated_at timestamp with time zone,
    hydration_error text,
    tags text[] DEFAULT '{}'::text[],
    integration_ids text[] DEFAULT '{}'::text[],
    install_count bigint DEFAULT 0 NOT NULL,
    featured boolean DEFAULT false NOT NULL,
    hidden boolean DEFAULT false NOT NULL,
    verified_at timestamp with time zone,
    status text DEFAULT 'draft'::text NOT NULL,
    public_skill_id uuid,
    origin_skill_id uuid,
    origin_org_id uuid,
    published_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE specialist_tasks (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    employee_id uuid NOT NULL,
    specialist_id uuid NOT NULL,
    sandbox_id uuid NOT NULL,
    conversation_id uuid NOT NULL,
    parent_conversation_type text NOT NULL,
    parent_conversation_id text NOT NULL,
    brief text NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb,
    created_at timestamp with time zone
);

ALTER TABLE ONLY skills
    ADD CONSTRAINT skills_pkey PRIMARY KEY (id);

ALTER TABLE ONLY specialist_tasks
    ADD CONSTRAINT specialist_tasks_pkey PRIMARY KEY (id);

CREATE INDEX idx_skills_category ON skills USING btree (category);

CREATE INDEX idx_skills_featured ON skills USING btree (featured);

CREATE INDEX idx_skills_hidden ON skills USING btree (hidden);

CREATE INDEX idx_skills_org_id ON skills USING btree (org_id);

CREATE INDEX idx_skills_origin_skill_id ON skills USING btree (origin_skill_id);

CREATE INDEX idx_skills_public_skill_id ON skills USING btree (public_skill_id);

CREATE INDEX idx_skills_publisher_id ON skills USING btree (publisher_id);

CREATE INDEX idx_skills_slug ON skills USING btree (slug);

CREATE INDEX idx_skills_status ON skills USING btree (status);

CREATE INDEX idx_specialist_task_org ON specialist_tasks USING btree (org_id);

CREATE INDEX idx_specialist_tasks_employee_id ON specialist_tasks USING btree (employee_id);

CREATE INDEX idx_specialist_tasks_parent_conversation_id ON specialist_tasks USING btree (parent_conversation_id);

CREATE INDEX idx_specialist_tasks_specialist_id ON specialist_tasks USING btree (specialist_id);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'initial schema down migration is intentionally unsupported; reset or restore the database instead'; END $$;
-- +goose StatementEnd
