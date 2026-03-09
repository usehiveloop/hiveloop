-- Bootstrap script for the unified Postgres instance.
-- Runs once on first container startup (via /docker-entrypoint-initdb.d/).
--
-- The superuser is set by POSTGRES_USER env var (default: llmvault).
-- The default database is created by POSTGRES_DB env var.

-- Extensions for the app database (already selected by default)
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Test database (isolated from dev data)
SELECT 'CREATE DATABASE llmvault_test OWNER ' || current_user
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'llmvault_test')\gexec
