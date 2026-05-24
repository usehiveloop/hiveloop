-- +goose Up
-- Cross-domain foreign key constraints

ALTER TABLE ONLY public.api_keys
    ADD CONSTRAINT fk_api_keys_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.conversation_assets
    ADD CONSTRAINT fk_conversation_assets_conversation FOREIGN KEY (conversation_id) REFERENCES public.employee_sessions(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.conversation_events
    ADD CONSTRAINT fk_conversation_events_conversation FOREIGN KEY (conversation_id) REFERENCES public.employee_sessions(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.credentials
    ADD CONSTRAINT fk_credentials_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.drive_assets
    ADD CONSTRAINT fk_drive_assets_employee FOREIGN KEY (employee_id) REFERENCES public.employees(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.drive_assets
    ADD CONSTRAINT fk_drive_assets_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.employee_assets
    ADD CONSTRAINT fk_employee_assets_employee FOREIGN KEY (employee_id) REFERENCES public.employees(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.employee_assets
    ADD CONSTRAINT fk_employee_assets_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.employee_assets
    ADD CONSTRAINT fk_employee_assets_sandbox FOREIGN KEY (sandbox_id) REFERENCES public.sandboxes(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.employee_memory_events
    ADD CONSTRAINT fk_employee_memory_events_employee FOREIGN KEY (employee_id) REFERENCES public.employees(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.employee_memory_events
    ADD CONSTRAINT fk_employee_memory_events_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.employee_memory_events
    ADD CONSTRAINT fk_employee_memory_events_sandbox FOREIGN KEY (sandbox_id) REFERENCES public.sandboxes(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.employee_sandbox_upgrades
    ADD CONSTRAINT fk_employee_sandbox_upgrades_employee FOREIGN KEY (employee_id) REFERENCES public.employees(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.employee_sandbox_upgrades
    ADD CONSTRAINT fk_employee_sandbox_upgrades_new_sandbox FOREIGN KEY (new_sandbox_id) REFERENCES public.sandboxes(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.employee_sandbox_upgrades
    ADD CONSTRAINT fk_employee_sandbox_upgrades_old_sandbox FOREIGN KEY (old_sandbox_id) REFERENCES public.sandboxes(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.employee_sandbox_upgrades
    ADD CONSTRAINT fk_employee_sandbox_upgrades_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.employee_schedule_runs
    ADD CONSTRAINT fk_employee_schedule_runs_employee FOREIGN KEY (employee_id) REFERENCES public.employees(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.employee_schedule_runs
    ADD CONSTRAINT fk_employee_schedule_runs_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.employee_schedule_runs
    ADD CONSTRAINT fk_employee_schedule_runs_sandbox FOREIGN KEY (sandbox_id) REFERENCES public.sandboxes(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.employee_schedule_runs
    ADD CONSTRAINT fk_employee_schedule_runs_schedule FOREIGN KEY (schedule_id) REFERENCES public.employee_schedules(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.employee_schedules
    ADD CONSTRAINT fk_employee_schedules_employee FOREIGN KEY (employee_id) REFERENCES public.employees(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.employee_schedules
    ADD CONSTRAINT fk_employee_schedules_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.employee_schedules
    ADD CONSTRAINT fk_employee_schedules_sandbox FOREIGN KEY (sandbox_id) REFERENCES public.sandboxes(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.employee_sessions
    ADD CONSTRAINT fk_employee_sessions_credential FOREIGN KEY (credential_id) REFERENCES public.credentials(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.employee_sessions
    ADD CONSTRAINT fk_employee_sessions_employee FOREIGN KEY (employee_id) REFERENCES public.employees(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.employee_sessions
    ADD CONSTRAINT fk_employee_sessions_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.employee_sessions
    ADD CONSTRAINT fk_employee_sessions_sandbox FOREIGN KEY (sandbox_id) REFERENCES public.sandboxes(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.employee_sessions
    ADD CONSTRAINT fk_employee_sessions_token FOREIGN KEY (token_id) REFERENCES public.tokens(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.employee_skills
    ADD CONSTRAINT fk_employee_skills_employee FOREIGN KEY (employee_id) REFERENCES public.employees(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.employee_skills
    ADD CONSTRAINT fk_employee_skills_skill FOREIGN KEY (skill_id) REFERENCES public.skills(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.employee_trigger_deliveries
    ADD CONSTRAINT fk_employee_trigger_deliveries_connection FOREIGN KEY (connection_id) REFERENCES public.connections(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.employee_trigger_deliveries
    ADD CONSTRAINT fk_employee_trigger_deliveries_conversation FOREIGN KEY (conversation_id) REFERENCES public.employee_sessions(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.employee_trigger_deliveries
    ADD CONSTRAINT fk_employee_trigger_deliveries_employee FOREIGN KEY (employee_id) REFERENCES public.employees(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.employee_trigger_deliveries
    ADD CONSTRAINT fk_employee_trigger_deliveries_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.employee_trigger_deliveries
    ADD CONSTRAINT fk_employee_trigger_deliveries_trigger FOREIGN KEY (trigger_id) REFERENCES public.employee_triggers(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.employee_triggers
    ADD CONSTRAINT fk_employee_triggers_connection FOREIGN KEY (connection_id) REFERENCES public.connections(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.employee_triggers
    ADD CONSTRAINT fk_employee_triggers_employee FOREIGN KEY (employee_id) REFERENCES public.employees(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.employee_triggers
    ADD CONSTRAINT fk_employee_triggers_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.employees
    ADD CONSTRAINT fk_employees_credential FOREIGN KEY (credential_id) REFERENCES public.credentials(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.employees
    ADD CONSTRAINT fk_employees_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.employees
    ADD CONSTRAINT fk_employees_sandbox_template FOREIGN KEY (sandbox_template_id) REFERENCES public.sandbox_templates(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.hindsight_banks
    ADD CONSTRAINT fk_hindsight_banks_employee FOREIGN KEY (employee_id) REFERENCES public.employees(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.connections
    ADD CONSTRAINT fk_connections_integration FOREIGN KEY (integration_id) REFERENCES public.integrations(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.connections
    ADD CONSTRAINT fk_connections_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.connections
    ADD CONSTRAINT fk_connections_user FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.integrations
    ADD CONSTRAINT fk_integrations_employee FOREIGN KEY (employee_id) REFERENCES public.employees(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.integrations
    ADD CONSTRAINT fk_integrations_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.oauth_accounts
    ADD CONSTRAINT fk_oauth_accounts_user FOREIGN KEY (user_id) REFERENCES public.users(id);

ALTER TABLE ONLY public.org_invites
    ADD CONSTRAINT fk_org_invites_invited_by FOREIGN KEY (invited_by_id) REFERENCES public.users(id);

ALTER TABLE ONLY public.org_invites
    ADD CONSTRAINT fk_org_invites_org FOREIGN KEY (org_id) REFERENCES public.orgs(id);

ALTER TABLE ONLY public.org_memberships
    ADD CONSTRAINT fk_org_memberships_org FOREIGN KEY (org_id) REFERENCES public.orgs(id);

ALTER TABLE ONLY public.org_memberships
    ADD CONSTRAINT fk_org_memberships_user FOREIGN KEY (user_id) REFERENCES public.users(id);

ALTER TABLE ONLY public.rag_index_attempt_errors
    ADD CONSTRAINT fk_rag_index_attempt_errors_index_attempt FOREIGN KEY (index_attempt_id) REFERENCES public.rag_index_attempts(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.rag_sources
    ADD CONSTRAINT fk_rag_sources_connection FOREIGN KEY (connection_id) REFERENCES public.connections(id);

ALTER TABLE ONLY public.sandbox_templates
    ADD CONSTRAINT fk_sandbox_templates_base_template FOREIGN KEY (base_template_id) REFERENCES public.sandbox_templates(id);

ALTER TABLE ONLY public.sandbox_templates
    ADD CONSTRAINT fk_sandbox_templates_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.sandboxes
    ADD CONSTRAINT fk_sandboxes_employee FOREIGN KEY (employee_id) REFERENCES public.employees(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.sandboxes
    ADD CONSTRAINT fk_sandboxes_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.sandboxes
    ADD CONSTRAINT fk_sandboxes_sandbox_template FOREIGN KEY (sandbox_template_id) REFERENCES public.sandbox_templates(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.skills
    ADD CONSTRAINT fk_skills_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.skills
    ADD CONSTRAINT fk_skills_publisher FOREIGN KEY (publisher_id) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.specialist_tasks
    ADD CONSTRAINT fk_specialist_tasks_conversation FOREIGN KEY (conversation_id) REFERENCES public.employee_sessions(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.specialist_tasks
    ADD CONSTRAINT fk_specialist_tasks_employee FOREIGN KEY (employee_id) REFERENCES public.employees(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.specialist_tasks
    ADD CONSTRAINT fk_specialist_tasks_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.specialist_tasks
    ADD CONSTRAINT fk_specialist_tasks_sandbox FOREIGN KEY (sandbox_id) REFERENCES public.sandboxes(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.specialist_tasks
    ADD CONSTRAINT fk_specialist_tasks_specialist FOREIGN KEY (specialist_id) REFERENCES public.employees(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.subscription_change_quotes
    ADD CONSTRAINT fk_subscription_change_quotes_subscription FOREIGN KEY (subscription_id) REFERENCES public.subscriptions(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.subscriptions
    ADD CONSTRAINT fk_subscriptions_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.subscriptions
    ADD CONSTRAINT fk_subscriptions_pending_plan FOREIGN KEY (pending_plan_id) REFERENCES public.plans(id);

ALTER TABLE ONLY public.subscriptions
    ADD CONSTRAINT fk_subscriptions_plan FOREIGN KEY (plan_id) REFERENCES public.plans(id);

ALTER TABLE ONLY public.tokens
    ADD CONSTRAINT fk_tokens_credential FOREIGN KEY (credential_id) REFERENCES public.credentials(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.tokens
    ADD CONSTRAINT fk_tokens_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.usage
    ADD CONSTRAINT fk_usage_credential FOREIGN KEY (credential_id) REFERENCES public.credentials(id);

ALTER TABLE ONLY public.usage
    ADD CONSTRAINT fk_usage_org FOREIGN KEY (org_id) REFERENCES public.orgs(id);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'initial schema down migration is intentionally unsupported; reset or restore the database instead'; END $$;
-- +goose StatementEnd
