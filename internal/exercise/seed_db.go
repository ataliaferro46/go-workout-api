package exercise

import (
	"context"
	"errors"
	"fmt"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
)

// SeedIfEmpty inserts seed into repo only when the repo is empty. It is the
// runtime equivalent of bundling seed data into the migration: on the first
// Postgres-mode boot the table is populated from seed.go, on subsequent
// boots (or after admin edits have added/changed rows) it is a no-op.
//
// The "empty" check is List length == 0. We tolerate ErrDuplicateName per
// row so a partial seed (interrupted boot, retry) converges instead of
// erroring out — every row that already exists is left alone, every row
// that's missing is inserted.
func SeedIfEmpty(ctx context.Context, repo Repository, seed []domain.Exercise) (int, error) {
	existing, err := repo.List(ctx)
	if err != nil {
		return 0, fmt.Errorf("list: %w", err)
	}
	if len(existing) > 0 {
		return 0, nil
	}
	inserted := 0
	for _, e := range seed {
		if err := repo.Create(ctx, e); err != nil {
			// On a retry mid-seed, some rows may already exist. Tolerate
			// duplicates so we converge; treat any other error as fatal so
			// we don't silently skip real failures.
			if errors.Is(err, ErrDuplicateName) {
				continue
			}
			return inserted, fmt.Errorf("insert %s: %w", e.ID, err)
		}
		inserted++
	}
	return inserted, nil
}
