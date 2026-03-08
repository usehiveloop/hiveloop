#!/bin/bash
set -e

# Create ZITADEL database and user on the shared Postgres instance.
# This script runs as the superuser (POSTGRES_USER) during container init.

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    DO \$\$
    BEGIN
        IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'zitadel') THEN
            CREATE ROLE zitadel WITH LOGIN PASSWORD 'zitadel';
        END IF;
    END
    \$\$;
EOSQL

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" <<-EOSQL
    SELECT 'CREATE DATABASE zitadel OWNER $POSTGRES_USER'
    WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'zitadel')\gexec
    GRANT ALL PRIVILEGES ON DATABASE zitadel TO zitadel;
EOSQL

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "zitadel" <<-EOSQL
    GRANT CREATE ON DATABASE zitadel TO zitadel;
    GRANT ALL ON SCHEMA public TO zitadel;
EOSQL

echo "ZITADEL database and user created."
