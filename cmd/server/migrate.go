package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"

	"github.com/caarlos0/env/v11"
	"github.com/usehivy/hivy/internal/db"
	"github.com/usehivy/hivy/internal/migrations"
)

type migrateDBConfig struct {
	DatabaseURL string `env:"HIVY_DATABASE_URL"`
	DBHost      string `env:"HIVY_DB_HOST"`
	DBPort      int    `env:"HIVY_DB_PORT" envDefault:"5432"`
	DBUser      string `env:"HIVY_DB_USER"`
	DBPassword  string `env:"HIVY_DB_PASSWORD"`
	DBName      string `env:"HIVY_DB_NAME"`
	DBSSLMode   string `env:"HIVY_DB_SSLMODE" envDefault:"disable"`
}

func loadMigrationDSN() (string, error) {
	cfg := migrateDBConfig{}
	if err := env.Parse(&cfg); err != nil {
		return "", fmt.Errorf("parsing migration database config: %w", err)
	}
	if cfg.DatabaseURL != "" {
		return cfg.DatabaseURL, nil
	}
	if cfg.DBHost == "" || cfg.DBUser == "" || cfg.DBPassword == "" || cfg.DBName == "" {
		return "", fmt.Errorf("set HIVY_DATABASE_URL or HIVY_DB_HOST, HIVY_DB_USER, HIVY_DB_PASSWORD, and HIVY_DB_NAME")
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		url.QueryEscape(cfg.DBUser),
		url.QueryEscape(cfg.DBPassword),
		cfg.DBHost,
		cfg.DBPort,
		cfg.DBName,
		cfg.DBSSLMode,
	), nil
}

func runMigrate(ctx context.Context, args []string) error {
	subcmd := "up"
	if len(args) > 0 {
		subcmd = args[0]
	}

	dsn, err := loadMigrationDSN()
	if err != nil {
		return err
	}
	gormDB, err := db.New(ctx, dsn)
	if err != nil {
		return err
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		return fmt.Errorf("getting sql db: %w", err)
	}
	defer sqlDB.Close()

	switch subcmd {
	case "up":
		results, err := migrations.Up(ctx, sqlDB)
		if err != nil {
			return err
		}
		for _, result := range results {
			slog.Info("migration applied", "version", result.Source.Version, "path", result.Source.Path, "duration", result.Duration)
		}
		if len(results) == 0 {
			slog.Info("migrations already current")
		}
		return nil
	case "status":
		statuses, err := migrations.Status(ctx, sqlDB)
		if err != nil {
			return err
		}
		for _, status := range statuses {
			slog.Info("migration status", "version", status.Source.Version, "path", status.Source.Path, "state", status.State, "applied_at", status.AppliedAt)
		}
		return nil
	case "version":
		version, err := migrations.Version(ctx, sqlDB)
		if err != nil {
			return err
		}
		slog.Info("migration version", "version", version)
		return nil
	default:
		return fmt.Errorf("unknown migrate command %q (use: up, status, version)", subcmd)
	}
}
