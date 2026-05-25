-- +goose Up
-- Runtime-backed specialist tasks and event classification.

ALTER TABLE specialist_tasks
    ADD COLUMN specialist_slug text,
    ADD COLUMN employee_session_id text,
    ADD COLUMN status varchar(64) NOT NULL DEFAULT 'running',
    ADD COLUMN updated_at timestamp with time zone,
    ADD COLUMN ended_at timestamp with time zone;

UPDATE specialist_tasks
SET
    specialist_slug = COALESCE(NULLIF(metadata->>'specialist_slug', ''), specialist_id::text),
    employee_session_id = COALESCE(NULLIF(parent_conversation_id, ''), conversation_id::text);

ALTER TABLE specialist_tasks
    ALTER COLUMN specialist_slug SET NOT NULL,
    ALTER COLUMN employee_session_id SET NOT NULL,
    ALTER COLUMN specialist_id DROP NOT NULL,
    ALTER COLUMN conversation_id DROP NOT NULL;

ALTER TABLE specialist_tasks
    DROP CONSTRAINT IF EXISTS fk_specialist_tasks_specialist;

CREATE INDEX idx_specialist_tasks_specialist_slug ON specialist_tasks USING btree (specialist_slug);
CREATE INDEX idx_specialist_tasks_employee_session_id ON specialist_tasks USING btree (employee_session_id);
CREATE INDEX idx_specialist_tasks_status ON specialist_tasks USING btree (status);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'runtime specialists migration down is intentionally unsupported; reset or restore the database instead'; END $$;
-- +goose StatementEnd
