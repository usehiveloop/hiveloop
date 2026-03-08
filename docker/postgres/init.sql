-- Bootstrap script for the unified Postgres instance.
-- Runs once on first container startup (via /docker-entrypoint-initdb.d/).
--
-- The superuser is set by POSTGRES_USER env var (default: proxybridge).
-- The default database is created by POSTGRES_DB env var.

-- Extensions for the app database (already selected by default)
CREATE EXTENSION IF NOT EXISTS "pgcrypto";
