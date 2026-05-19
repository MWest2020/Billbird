//go:build integration

// Package integration spins up a real Postgres against which the
// migrations, the plan-entry store, the API-token store, and the
// plan-vs-actual computation are exercised.
//
// Run with:
//
//	go test -tags=integration ./internal/integration/...
//
// Without the `integration` build tag the file is invisible, so a
// plain `go test ./...` stays fast and dependency-light.
package integration

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mwesterweel/billbird/internal/db"
)

// migrationsURL returns a file:// URL for the repository's migrations/
// folder, regardless of the test process's working directory. We
// resolve relative to this source file (always two directories above
// the integration test package).
func migrationsURL(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// internal/integration/postgres_test.go -> repo root
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	return "file://" + filepath.Join(repoRoot, "migrations")
}

var portCounter int32 = 16432

// startPostgres boots an embedded Postgres on a free ephemeral port,
// applies every migration in migrations/, and returns the connection
// pool plus a shutdown function. Failing assertions in setup call
// t.Fatalf — tests get a usable pool or they do not run.
func startPostgres(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()

	port, err := pickPort()
	if err != nil {
		t.Fatalf("pick port: %v", err)
	}

	runtimePath := t.TempDir()
	dataPath := t.TempDir()
	binariesPath := filepath.Join(runtimePath, "bin")

	pg := embeddedpostgres.NewDatabase(
		embeddedpostgres.DefaultConfig().
			Port(port).
			RuntimePath(runtimePath).
			DataPath(dataPath).
			BinariesPath(binariesPath).
			Username("billbird").
			Password("billbird").
			Database("billbird").
			StartTimeout(120*time.Second),
	)
	if err := pg.Start(); err != nil {
		t.Fatalf("starting embedded postgres: %v", err)
	}

	dsn := fmt.Sprintf(
		"postgres://billbird:billbird@127.0.0.1:%d/billbird?sslmode=disable",
		port,
	)

	if err := db.MigrateFrom(dsn, migrationsURL(t)); err != nil {
		_ = pg.Stop()
		t.Fatalf("running migrations: %v", err)
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		_ = pg.Stop()
		t.Fatalf("connecting pool: %v", err)
	}

	shutdown := func() {
		pool.Close()
		if err := pg.Stop(); err != nil {
			t.Logf("warning: stopping embedded postgres: %v", err)
		}
	}
	return pool, shutdown
}

// pickPort allocates a free TCP port. We bump a counter to avoid
// collisions across parallel embedded-postgres processes within a
// single test binary, then verify the port is actually free.
func pickPort() (uint32, error) {
	for i := 0; i < 20; i++ {
		candidate := uint32(atomic.AddInt32(&portCounter, 1))
		addr := fmt.Sprintf("127.0.0.1:%d", candidate)
		l, err := net.Listen("tcp", addr)
		if err == nil {
			_ = l.Close()
			return candidate, nil
		}
	}
	return 0, fmt.Errorf("no free port found")
}

// ensureLinux narrows the test surface: embedded-postgres downloads a
// platform-specific binary and we only validate the Linux path. The
// CI/dev box is Linux per project conventions; skip elsewhere so
// running on a developer Mac does not silently fall over.
func ensureLinux(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skipf("integration tests only verified on linux; got %s", runtime.GOOS)
	}
}
