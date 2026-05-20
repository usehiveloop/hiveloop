-- Bootstrap script for the unified Postgres instance.
-- Runs once on first container startup (via /docker-entrypoint-initdb.d/).
--
-- The superuser is set by POSTGRES_USER env var (default: hivy).
-- The default database is created by POSTGRES_DB env var.

-- Extensions for the app database (already selected by default)
CREATE EXTENSION IF NOT EXISTS "pgcrypto";
CREATE EXTENSION IF NOT EXISTS "vector";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";

-- Test database (isolated from dev data)
SELECT 'CREATE DATABASE hivy_test OWNER ' || current_user
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'hivy_test')\gexec

-- Vault-specific test database (for Vault KMS e2e tests)
SELECT 'CREATE DATABASE hivy_vault_test OWNER ' || current_user
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'hivy_vault_test')\gexec

-- Local Nango database (used by the real Nango docker-compose service)
SELECT 'CREATE DATABASE nango OWNER ' || current_user
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'nango')\gexec
