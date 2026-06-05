//go:build integration

package exercise

import (
	"context"
	"os"
	"testing"

	"github.com/ataliaferro46/go-workout-api/internal/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestPostgresRepository_Contract runs the shared exercise contract against
// a real Postgres instance. Gated behind the `integration` build tag so it
// does not run on `go test ./...`:
//
//	make db-up && make test-integration
//
// truncates between subtests so each one starts from an empty `exercises`
// table — even though the migration seeds it, the contract scenarios assume
// no preexisting rows.
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

func truncate(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `TRUNCATE exercises`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
}
