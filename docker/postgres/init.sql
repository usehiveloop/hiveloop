-- This file is executed on first container startup only.
-- GORM handles schema migrations, so we just ensure the database exists.
-- The database is created by POSTGRES_DB env var, this is a placeholder
-- for any additional bootstrap like extensions.

CREATE EXTENSION IF NOT EXISTS "pgcrypto";
