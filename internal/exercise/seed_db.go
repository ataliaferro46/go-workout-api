package exercise

import (
	"context"
	"errors"
	"fmt"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
)

// SeedIfEmpty inserts every row in seed that does not already exist in repo.
// Despite the name (kept for compatibility), it is now an idempotent "seed
// missing" pass rather than an empty-only fill: every existing row is left
// alone, every missing row is inserted. This lets us add exercises in
// seed.go and have them reach a production DB on the next deploy without
// resorting to an UPDATE-laden migration.
//
// Duplicate-by-name conflicts are tolerated so a partial prior seed
// (interrupted boot, manual admin rename) doesn't stall the catch-up.
func SeedIfEmpty(ctx context.Context, repo Repository, seed []domain.Exercise) (int, error) {
	existing, err := repo.List(ctx)
	if err != nil {
		return 0, fmt.Errorf("list: %w", err)
	}
	have := make(map[string]bool, len(existing))
	for _, e := range existing {
		have[e.ID] = true
	}
	inserted := 0
	for _, e := range seed {
		if have[e.ID] {
			continue
		}
		if err := repo.Create(ctx, e); err != nil {
			if errors.Is(err, ErrDuplicateName) {
				continue
			}
			return inserted, fmt.Errorf("insert %s: %w", e.ID, err)
		}
		inserted++
	}
	return inserted, nil
}
