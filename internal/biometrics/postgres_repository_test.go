//go:build integration

package biometrics

import (
	"context"
	"os"
	"testing"

	"github.com/ataliaferro46/go-workout-api/internal/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresReadingRepository_Contract(t *testing.T) {
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

	testReadingRepositoryContract(t, func(t *testing.T) ReadingRepository {
		truncateReadings(t, pool)
		return NewPostgresReadingRepository(pool)
	})
}

func truncateReadings(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `TRUNCATE biometric_readings`); err != nil {
		t.Fatalf("truncate readings: %v", err)
	}
}
