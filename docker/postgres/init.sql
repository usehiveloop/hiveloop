-- Bootstrap script for the unified Postgres instance.
-- Runs once on first container startup (via /docker-entrypoint-initdb.d/).
--
-- The superuser is set by POSTGRES_USER env var (default: ziraloop).
-- The default database is created by POSTGRES_DB env var.

-- Extensions for the app database (already selected by default)
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Test database (isolated from dev data)
SELECT 'CREATE DATABASE ziraloop_test OWNER ' || current_user
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'ziraloop_test')\gexec

-- Vault-specific test database (for Vault KMS e2e tests)
SELECT 'CREATE DATABASE ziraloop_vault_test OWNER ' || current_user
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'ziraloop_vault_test')\gexec
