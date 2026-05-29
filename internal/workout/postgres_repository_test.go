//go:build integration

package workout

import (
	"context"
	"os"
	"testing"

	"github.com/ataliaferro46/go-workout-api/internal/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestPostgresRepository_Contract runs the shared repository contract against
// a real Postgres instance. The test is gated behind the `integration` build
// tag so it does not run on every `go test ./...` invocation:
//
//	TEST_DATABASE_URL=postgres://workout:workout@localhost:5432/workout_test?sslmode=disable \
//	  go test -tags=integration ./internal/workout/...
//
// `docker compose up -d postgres` brings up a database that matches the URL
// in the Makefile's db-test target.
func TestPostgresRepository_Contract(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping Postgres contract")
	}

	ctx := context.Background()
	if err := db.Migrate(dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	testRepositoryContract(t, func(t *testing.T) Repository {
		truncate(t, pool)
		return NewPostgresRepository(pool)
	})
}

// truncate clears every table the workout repo touches so each subtest starts
// from a clean slate. CASCADE handles the workout_exercises FK automatically.
func truncate(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`TRUNCATE workouts CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
}
