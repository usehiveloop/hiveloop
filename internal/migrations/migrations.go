package migrations

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

//go:embed sql/*.sql
var migrationFS embed.FS

func provider(db *sql.DB) (*goose.Provider, error) {
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return nil, fmt.Errorf("creating goose lock: %w", err)
	}
	fsys, err := fs.Sub(migrationFS, "sql")
	if err != nil {
		return nil, fmt.Errorf("opening embedded migrations: %w", err)
	}
	return goose.NewProvider(
		goose.DialectPostgres,
		db,
		fsys,
		goose.WithSessionLocker(locker),
		goose.WithDisableGlobalRegistry(true),
	)
}

func Up(ctx context.Context, db *sql.DB) ([]*goose.MigrationResult, error) {
	p, err := provider(db)
	if err != nil {
		return nil, err
	}
	defer p.Close()
	return p.Up(ctx)
}

func Status(ctx context.Context, db *sql.DB) ([]*goose.MigrationStatus, error) {
	p, err := provider(db)
	if err != nil {
		return nil, err
	}
	defer p.Close()
	return p.Status(ctx)
}

func Version(ctx context.Context, db *sql.DB) (int64, error) {
	p, err := provider(db)
	if err != nil {
		return 0, err
	}
	defer p.Close()
	return p.GetDBVersion(ctx)
}

func HasPending(ctx context.Context, db *sql.DB) (bool, error) {
	p, err := provider(db)
	if err != nil {
		return false, err
	}
	defer p.Close()
	return p.HasPending(ctx)
}
