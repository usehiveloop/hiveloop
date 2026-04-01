-- Bootstrap script for the shared Postgres instance.
-- Runs once on first container startup (via /docker-entrypoint-initdb.d/).
--
-- The superuser is set by POSTGRES_USER env var (default: llmvault).
-- The default database (llmvault) is created automatically by POSTGRES_DB.

-- Extensions for the app database
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Nango database (separate from the main app database)
SELECT 'CREATE DATABASE nango OWNER ' || current_user
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'nango')\gexec
