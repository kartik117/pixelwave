package testutil

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// NewTestPostgres spins up a real postgres:16-alpine container and returns
// an open *sql.DB -- real schema, real constraints, real DISTINCT ON
// queries, not a SQLite stand-in pretending to be Postgres.
func NewTestPostgres(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("pixelwave"),
		tcpostgres.WithUsername("pixelwave"),
		tcpostgres.WithPassword("pixelwave"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}
	t.Cleanup(func() {
		_ = container.Terminate(ctx)
	})

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get postgres connection string: %v", err)
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("failed to open postgres connection: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}
