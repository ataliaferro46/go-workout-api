package exercise

import (
	"context"
	"errors"
	"fmt"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
)

// SeedIfEmpty inserts every row in seed that does not already exist in repo.
// Despite the name (kept for compatibility), it is now an idempotent "seed
// missing" pass: every existing row is left alone, every missing row is
// inserted. The canonical seed is merged with the Free Exercise DB
// (yuhonas / Everkinetic, public domain ~870 entries → ~500 after
// strength-only filtering) when the local table is sparse, fanning the
// library out to ~600 distinct exercises.
//
// The external fetch is best-effort — a network failure is logged but
// non-fatal so the boot path always completes with at least the canonical
// seed available.
func SeedIfEmpty(ctx context.Context, repo Repository, seed []domain.Exercise) (int, error) {
	existing, err := repo.List(ctx)
	if err != nil {
		return 0, fmt.Errorf("list: %w", err)
	}
	have := make(map[string]bool, len(existing))
	for _, e := range existing {
		have[e.ID] = true
	}
	merged := seed
	// Only attempt the external fetch when we're likely to gain rows —
	// once the table is well-populated, the round-trip is wasted work.
	if len(existing) < 200 {
		if ext, err := FetchExternalExercises(ctx); err == nil && len(ext) > 0 {
			merged = append(merged, ext...)
		}
	}
	inserted := 0
	for _, e := range merged {
		if have[e.ID] {
			continue
		}
		if err := repo.Create(ctx, e); err != nil {
			if errors.Is(err, ErrDuplicateName) {
				// Same name with a different id — almost always a dup
				// from the external source overlapping our canonical seed.
				// Skip silently.
				continue
			}
			return inserted, fmt.Errorf("insert %s: %w", e.ID, err)
		}
		inserted++
		have[e.ID] = true
	}
	return inserted, nil
}
