package store

import (
	"embed"
	"errors"
	"fmt"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate applies all pending migrations. Migrations are embedded in the binary,
// so deployment needs no separate migration files or CLI.
func Migrate(dsn string) error {
	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", src, pgxScheme(dsn))
	if err != nil {
		return fmt.Errorf("init migrate: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("run migrations: %w", err)
	}
	return nil
}

// pgxScheme rewrites a postgres:// DSN to the pgx5:// scheme migrate expects.
func pgxScheme(dsn string) string {
	if s, ok := strings.CutPrefix(dsn, "postgres://"); ok {
		return "pgx5://" + s
	}
	if s, ok := strings.CutPrefix(dsn, "postgresql://"); ok {
		return "pgx5://" + s
	}
	return dsn
}
