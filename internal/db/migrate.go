package db

import (
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// Migrate runs every pending migration in ./migrations against the
// given database. Convenience wrapper around MigrateFrom for the
// default deployment layout.
func Migrate(databaseURL string) error {
	return MigrateFrom(databaseURL, "file://migrations")
}

// MigrateFrom runs migrations from an explicit source URL. Tests pass
// an absolute file:// URL so they work regardless of the test
// process's working directory.
func MigrateFrom(databaseURL, sourceURL string) error {
	m, err := migrate.New(sourceURL, databaseURL)
	if err != nil {
		return fmt.Errorf("creating migrator: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("running migrations: %w", err)
	}

	return nil
}
