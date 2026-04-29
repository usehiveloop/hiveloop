\set ON_ERROR_STOP on

-- === user ===

INSERT INTO users (id, email, name, email_confirmed_at, created_at, updated_at)
VALUES (gen_random_uuid(), 'agent-test@example.com', 'Agent Test', NOW(), NOW(), NOW())
ON CONFLICT (email) DO UPDATE
  SET email_confirmed_at = COALESCE(users.email_confirmed_at, NOW());

-- === workspace ===

INSERT INTO orgs (id, name, rate_limit, active, plan_slug, byok, logo_url, created_at, updated_at)
VALUES (gen_random_uuid(), 'Agent Test Workspace', 1000, true, 'free', false, '', NOW(), NOW())
ON CONFLICT (name) DO NOTHING;

-- === membership ===

INSERT INTO org_memberships (id, user_id, org_id, role, created_at, updated_at)
SELECT gen_random_uuid(), u.id, o.id, 'owner', NOW(), NOW()
  FROM users u CROSS JOIN orgs o
  WHERE u.email = 'agent-test@example.com'
    AND o.name  = 'Agent Test Workspace'
ON CONFLICT (user_id, org_id) DO NOTHING;

-- === enabled OAUTH2 integrations ===

INSERT INTO in_integrations (id, unique_key, provider, display_name, meta, nango_config,
                             supports_rag_source, created_at, updated_at)
VALUES
  (gen_random_uuid(), 'github-test',     'github',     'GitHub',     '{}'::jsonb,
   jsonb_build_object('auth_mode','OAUTH2','callback_url','http://localhost:13004/oauth/callback',
     'webhook_url','https://fake-nango.local/webhook/in_github-test',
     'webhook_secret','whsec_fake_in_github-test','forward_webhooks',true),
   true,  NOW(), NOW()),
  (gen_random_uuid(), 'slack-test',      'slack',      'Slack',      '{}'::jsonb,
   jsonb_build_object('auth_mode','OAUTH2','callback_url','http://localhost:13004/oauth/callback',
     'webhook_url','https://fake-nango.local/webhook/in_slack-test',
     'webhook_secret','whsec_fake_in_slack-test','forward_webhooks',true),
   true,  NOW(), NOW()),
  (gen_random_uuid(), 'notion-test',     'notion',     'Notion',     '{}'::jsonb,
   jsonb_build_object('auth_mode','OAUTH2','callback_url','http://localhost:13004/oauth/callback',
     'webhook_url','https://fake-nango.local/webhook/in_notion-test',
     'webhook_secret','whsec_fake_in_notion-test','forward_webhooks',true),
   true,  NOW(), NOW()),
  (gen_random_uuid(), 'linear-test',     'linear',     'Linear',     '{}'::jsonb,
   jsonb_build_object('auth_mode','OAUTH2','callback_url','http://localhost:13004/oauth/callback',
     'webhook_url','https://fake-nango.local/webhook/in_linear-test',
     'webhook_secret','whsec_fake_in_linear-test','forward_webhooks',true),
   false, NOW(), NOW()),
  (gen_random_uuid(), 'asana-test',      'asana',      'Asana',      '{}'::jsonb,
   jsonb_build_object('auth_mode','OAUTH2','callback_url','http://localhost:13004/oauth/callback',
     'webhook_url','https://fake-nango.local/webhook/in_asana-test',
     'webhook_secret','whsec_fake_in_asana-test','forward_webhooks',true),
   false, NOW(), NOW()),
  (gen_random_uuid(), 'jira-test',       'jira',       'Jira',       '{}'::jsonb,
   jsonb_build_object('auth_mode','OAUTH2','callback_url','http://localhost:13004/oauth/callback',
     'webhook_url','https://fake-nango.local/webhook/in_jira-test',
     'webhook_secret','whsec_fake_in_jira-test','forward_webhooks',true),
   false, NOW(), NOW()),
  (gen_random_uuid(), 'salesforce-test', 'salesforce', 'Salesforce', '{}'::jsonb,
   jsonb_build_object('auth_mode','OAUTH2','callback_url','http://localhost:13004/oauth/callback',
     'webhook_url','https://fake-nango.local/webhook/in_salesforce-test',
     'webhook_secret','whsec_fake_in_salesforce-test','forward_webhooks',true),
   false, NOW(), NOW()),
  (gen_random_uuid(), 'railway-test',    'railway',    'Railway',    '{}'::jsonb,
   jsonb_build_object('auth_mode','OAUTH2','callback_url','http://localhost:13004/oauth/callback',
     'webhook_url','https://fake-nango.local/webhook/in_railway-test',
     'webhook_secret','whsec_fake_in_railway-test','forward_webhooks',true),
   false, NOW(), NOW())
ON CONFLICT (provider) DO UPDATE
  SET unique_key          = EXCLUDED.unique_key,
      nango_config        = EXCLUDED.nango_config,
      display_name        = EXCLUDED.display_name,
      supports_rag_source = EXCLUDED.supports_rag_source,
      updated_at          = NOW();

-- === api key (plaintext: hvl_sk_aaaaaaaa…aaaa, scopes=all) ===

INSERT INTO api_keys (id, org_id, name, key_hash, key_prefix, scopes, created_at)
SELECT gen_random_uuid(),
       o.id,
       'Agent Test Full Access',
       '9e3dd7697a52b5aa304ce5863f44059c2c81185ae9358394085cfc0da0c5e914',
       'hvl_sk_aaaaaaaa',
       ARRAY['all']::text[],
       NOW()
  FROM orgs o
  WHERE o.name = 'Agent Test Workspace'
ON CONFLICT (key_hash) DO NOTHING;

-- === starter agent ===

INSERT INTO agents (id, org_id, name, description, category, system_prompt, model,
                    provider_prompts, tools, mcp_servers, skills, integrations,
                    agent_config, permissions, resources, team, shared_memory,
                    sandbox_tools, setup_commands, status, agent_type, is_system,
                    provider_group, created_at, updated_at)
SELECT gen_random_uuid(),
       o.id,
       'Test Agent',
       'Pre-seeded agent for browser-driven tests',
       'general',
       'You are a helpful test agent.',
       'claude-sonnet-4-5',
       '{}'::jsonb, '{}'::jsonb, '{}'::jsonb, '{}'::jsonb, '{}'::jsonb,
       '{}'::jsonb, '{}'::jsonb, '{}'::jsonb,
       '', false,
       '{}'::text[], '{}'::text[],
       'active', 'agent', false, '',
       NOW(), NOW()
  FROM orgs o
  WHERE o.name = 'Agent Test Workspace'
    AND NOT EXISTS (
      SELECT 1 FROM agents a
      WHERE a.org_id = o.id AND a.name = 'Test Agent' AND a.deleted_at IS NULL
    );

-- === identity variety ===

INSERT INTO users (id, email, name, email_confirmed_at, created_at, updated_at)
VALUES (gen_random_uuid(), 'agent-member@example.com', 'Agent Member', NOW(), NOW(), NOW())
ON CONFLICT (email) DO UPDATE
  SET email_confirmed_at = COALESCE(users.email_confirmed_at, NOW());

INSERT INTO org_memberships (id, user_id, org_id, role, created_at, updated_at)
SELECT gen_random_uuid(), u.id, o.id, 'member', NOW(), NOW()
  FROM users u CROSS JOIN orgs o
  WHERE u.email = 'agent-member@example.com'
    AND o.name  = 'Agent Test Workspace'
ON CONFLICT (user_id, org_id) DO NOTHING;

INSERT INTO users (id, email, name, email_confirmed_at, banned_at, ban_reason, created_at, updated_at)
VALUES (gen_random_uuid(), 'agent-banned@example.com', 'Agent Banned',
        NOW(), NOW(), 'test fixture - banned', NOW(), NOW())
ON CONFLICT (email) DO UPDATE
  SET banned_at  = COALESCE(users.banned_at, NOW()),
      ban_reason = COALESCE(users.ban_reason, 'test fixture - banned');

INSERT INTO users (id, email, name, email_confirmed_at, created_at, updated_at)
VALUES (gen_random_uuid(), 'agent-other@example.com', 'Agent Other', NOW(), NOW(), NOW())
ON CONFLICT (email) DO UPDATE
  SET email_confirmed_at = COALESCE(users.email_confirmed_at, NOW());

INSERT INTO orgs (id, name, rate_limit, active, plan_slug, byok, logo_url, created_at, updated_at)
VALUES (gen_random_uuid(), 'Other Workspace', 1000, true, 'free', false, '', NOW(), NOW())
ON CONFLICT (name) DO NOTHING;

INSERT INTO org_memberships (id, user_id, org_id, role, created_at, updated_at)
SELECT gen_random_uuid(), u.id, o.id, 'owner', NOW(), NOW()
  FROM users u CROSS JOIN orgs o
  WHERE u.email = 'agent-other@example.com'
    AND o.name  = 'Other Workspace'
ON CONFLICT (user_id, org_id) DO NOTHING;

INSERT INTO orgs (id, name, rate_limit, active, plan_slug, byok, logo_url, created_at, updated_at)
VALUES (gen_random_uuid(), 'Agent Test Paid Workspace', 5000, true, 'pro', false, '', NOW(), NOW())
ON CONFLICT (name) DO UPDATE
  SET plan_slug  = EXCLUDED.plan_slug,
      rate_limit = EXCLUDED.rate_limit;

INSERT INTO org_memberships (id, user_id, org_id, role, created_at, updated_at)
SELECT gen_random_uuid(), u.id, o.id, 'owner', NOW(), NOW()
  FROM users u CROSS JOIN orgs o
  WHERE u.email = 'agent-test@example.com'
    AND o.name  = 'Agent Test Paid Workspace'
ON CONFLICT (user_id, org_id) DO NOTHING;

-- === pre-existing revoked connection ===

INSERT INTO in_connections (id, org_id, user_id, in_integration_id, nango_connection_id, meta,
                            webhook_configured, revoked_at, created_at, updated_at)
SELECT gen_random_uuid(), o.id, u.id, i.id,
       'previously-revoked-conn-id',
       '{}'::jsonb, true,
       NOW() - INTERVAL '7 days',
       NOW() - INTERVAL '14 days', NOW() - INTERVAL '7 days'
  FROM users u, orgs o, in_integrations i
  WHERE u.email = 'agent-test@example.com'
    AND o.name  = 'Agent Test Workspace'
    AND i.provider = 'github'
    AND NOT EXISTS (
      SELECT 1 FROM in_connections WHERE nango_connection_id = 'previously-revoked-conn-id'
    );

-- === summary ===

\echo
\echo Seeded:
SELECT 'user (admin)'    AS what, u.email AS detail
  FROM users u WHERE u.email = 'agent-test@example.com'
UNION ALL
SELECT 'user (member)',    email FROM users WHERE email = 'agent-member@example.com'
UNION ALL
SELECT 'user (banned)',    email FROM users WHERE email = 'agent-banned@example.com'
UNION ALL
SELECT 'user (cross-org)', email FROM users WHERE email = 'agent-other@example.com'
UNION ALL
SELECT 'org (free)',       name  FROM orgs  WHERE name  = 'Agent Test Workspace'
UNION ALL
SELECT 'org (paid)',       name  FROM orgs  WHERE name  = 'Agent Test Paid Workspace'
UNION ALL
SELECT 'org (cross)',      name  FROM orgs  WHERE name  = 'Other Workspace'
UNION ALL
SELECT 'integrations',     string_agg(provider, ', ' ORDER BY provider)
  FROM in_integrations  WHERE unique_key LIKE '%-test' AND deleted_at IS NULL
UNION ALL
SELECT 'api_key',          'hvl_sk_aaaaaaaa…  scopes=' || array_to_string(k.scopes, ',')
  FROM api_keys k JOIN orgs o ON o.id = k.org_id
  WHERE o.name = 'Agent Test Workspace' AND k.revoked_at IS NULL
UNION ALL
SELECT 'agent',            a.name FROM agents a JOIN orgs o ON o.id = a.org_id
  WHERE o.name = 'Agent Test Workspace' AND a.deleted_at IS NULL
UNION ALL
SELECT 'revoked conn',     'github · revoked 7d ago' FROM in_connections
  WHERE nango_connection_id = 'previously-revoked-conn-id';
