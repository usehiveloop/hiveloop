package testdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/migrations"
)

func ApplyMigrations(t testing.TB, db *gorm.DB) {
	t.Helper()

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	if current, ok := currentMigrationVersion(t.Context(), sqlDB); ok && current >= latestMigrationVersion {
		return
	}

	unlock, err := lockMigrationSetup(t.Context(), sqlDB)
	if err != nil {
		t.Fatalf("lock migration setup: %v", err)
	}
	defer unlock(t.Context())

	if current, ok := currentMigrationVersion(t.Context(), sqlDB); ok && current >= latestMigrationVersion {
		return
	}
	if _, err := migrations.Up(t.Context(), sqlDB); err == nil {
		return
	} else if stampLegacyInitialSchema(t, sqlDB, err) {
		if _, retryErr := migrations.Up(t.Context(), sqlDB); retryErr != nil {
			t.Fatalf("apply migrations after legacy schema stamp: %v", retryErr)
		} else {
			return
		}
	} else {
		t.Fatalf("apply migrations: %v", err)
	}
}

func currentMigrationVersion(ctx context.Context, db *sql.DB) (int64, bool) {
	var version int64
	err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version_id) FILTER (WHERE is_applied), 0) FROM goose_db_version`).Scan(&version)
	if err == nil {
		return version, true
	}
	if errors.Is(err, sql.ErrNoRows) {
		return 0, true
	}
	return 0, false
}

func lockMigrationSetup(ctx context.Context, db *sql.DB) (func(context.Context), error) {
	previousMaxOpen := db.Stats().MaxOpenConnections
	if previousMaxOpen == 1 {
		db.SetMaxOpenConns(2)
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		if previousMaxOpen == 1 {
			db.SetMaxOpenConns(previousMaxOpen)
		}
		return nil, err
	}
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock(hashtext('hivy_testdb_migrations'))`); err != nil {
		_ = conn.Close()
		if previousMaxOpen == 1 {
			db.SetMaxOpenConns(previousMaxOpen)
		}
		return nil, err
	}
	return func(ctx context.Context) {
		_, _ = conn.ExecContext(ctx, `SELECT pg_advisory_unlock(hashtext('hivy_testdb_migrations'))`)
		_ = conn.Close()
		if previousMaxOpen == 1 {
			db.SetMaxOpenConns(previousMaxOpen)
		}
	}, nil
}

func stampLegacyInitialSchema(t testing.TB, db *sql.DB, migrationErr error) bool {
	t.Helper()
	if !strings.Contains(migrationErr.Error(), "already exists") && !strings.Contains(migrationErr.Error(), "SQLSTATE 42P07") {
		return false
	}

	var version int64
	if err := db.QueryRowContext(t.Context(), `SELECT COALESCE(MAX(version_id) FILTER (WHERE is_applied), 0) FROM goose_db_version`).Scan(&version); err != nil {
		return false
	}
	if version != 1 {
		return false
	}

	if missing, err := missingMigratedTables(t.Context(), db); err != nil || len(missing) > 0 {
		if err != nil {
			t.Logf("legacy schema check failed: %v", err)
		} else {
			t.Logf("legacy schema check failed; missing tables: %s", strings.Join(missing, ", "))
		}
		return false
	}

	for version := int64(2); version <= 11; version++ {
		if _, err := db.ExecContext(t.Context(), `
				INSERT INTO goose_db_version (version_id, is_applied, tstamp)
				SELECT $1, true, now()
				WHERE NOT EXISTS (
					SELECT 1 FROM goose_db_version WHERE version_id = $1 AND is_applied
				)
			`, version); err != nil {
			t.Fatalf("stamp legacy migration version %d: %v", version, err)
		}
	}
	return true
}

func missingMigratedTables(ctx context.Context, db *sql.DB) ([]string, error) {
	const query = `
SELECT table_name
FROM information_schema.tables
WHERE table_schema = current_schema()
	AND table_name = ANY($1)`

	rows, err := db.QueryContext(ctx, query, "{"+strings.Join(migratedTables, ",")+"}")
	if err != nil {
		return nil, fmt.Errorf("query migrated tables: %w", err)
	}
	defer rows.Close()

	found := make(map[string]bool, len(migratedTables))
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, fmt.Errorf("scan migrated table: %w", err)
		}
		found[table] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate migrated tables: %w", err)
	}

	var missing []string
	for _, table := range migratedTables {
		if !found[table] {
			missing = append(missing, table)
		}
	}
	return missing, nil
}

var migratedTables = []string{
	"api_keys",
	"audit_log",
	"connections",
	"conversation_assets",
	"conversation_events",
	"credentials",
	"credit_ledger_entries",
	"custom_domains",
	"drive_assets",
	"email_verifications",
	"employee_assets",
	"employee_memory_events",
	"employee_sandbox_upgrades",
	"employee_schedule_runs",
	"employee_schedules",
	"employee_sessions",
	"employee_skills",
	"employee_trigger_deliveries",
	"employee_triggers",
	"employees",
	"failed_events",
	"generations",
	"hindsight_banks",
	"integrations",
	"oauth_accounts",
	"oauth_exchange_tokens",
	"org_invites",
	"org_memberships",
	"orgs",
	"otp_codes",
	"password_resets",
	"plans",
	"rag_embedding_models",
	"rag_external_identities",
	"rag_external_user_groups",
	"rag_index_attempt_errors",
	"rag_index_attempts",
	"rag_public_external_user_groups",
	"rag_search_settings",
	"rag_sources",
	"rag_sync_records",
	"rag_sync_states",
	"rag_user_external_user_groups",
	"refresh_tokens",
	"sandbox_templates",
	"sandboxes",
	"skills",
	"specialist_tasks",
	"subscription_change_quotes",
	"subscriptions",
	"tokens",
	"tool_usages",
	"usage",
	"users",
}

const latestMigrationVersion = 11
