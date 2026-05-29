package workout

import (
	"context"
	"fmt"
	"time"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresRepository persists logged workouts in Postgres. It is the
// production-shaped alternative to InMemoryRepository and satisfies the same
// Repository interface — the Service does not know or care which is in use.
//
// Workouts and their exercises live in two tables. Reads use a single LEFT
// JOIN per query, never N+1; writes happen inside one transaction so a
// workout never lands without its exercises (or vice versa).
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository returns a Repository backed by the given pool. The
// caller owns the pool's lifecycle.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

// Create inserts the workout header and its exercises in a single
// transaction. The deferred Rollback is safe even after a successful Commit
// (it becomes a no-op), so this method cannot leak a half-written workout —
// neither on a SQL error nor on a panic.
func (r *PostgresRepository) Create(ctx context.Context, w domain.Workout) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		INSERT INTO workouts (id, user_id, name, notes, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`, w.ID, w.UserID, w.Name, w.Notes, w.CreatedAt); err != nil {
		return fmt.Errorf("insert workout: %w", err)
	}
	for i, ex := range w.Exercises {
		if _, err := tx.Exec(ctx, `
			INSERT INTO workout_exercises
				(workout_id, position, name, sets, reps, weight_kg)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, w.ID, i, ex.Name, ex.Sets, ex.Reps, ex.WeightKG); err != nil {
			return fmt.Errorf("insert exercise %d (%q): %w", i, ex.Name, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// workoutSelect projects the columns scanWorkouts expects. Callers append
// WHERE / ORDER BY clauses.
const workoutSelect = `
	SELECT w.id, w.user_id, w.name, w.notes, w.created_at,
	       e.position, e.name, e.sets, e.reps, e.weight_kg
	FROM workouts w
	LEFT JOIN workout_exercises e ON e.workout_id = w.id
`

// Get returns the workout with the given ID or domain.ErrNotFound.
func (r *PostgresRepository) Get(ctx context.Context, id string) (domain.Workout, error) {
	rows, err := r.pool.Query(ctx,
		workoutSelect+` WHERE w.id = $1 ORDER BY e.position`, id)
	if err != nil {
		return domain.Workout{}, fmt.Errorf("query workout: %w", err)
	}
	defer rows.Close()

	workouts, err := scanWorkouts(rows)
	if err != nil {
		return domain.Workout{}, err
	}
	if len(workouts) == 0 {
		return domain.Workout{}, domain.ErrNotFound
	}
	return workouts[0], nil
}

// ListByUser returns a user's workouts newest first. The result is a non-nil
// slice so JSON callers get [] rather than null.
func (r *PostgresRepository) ListByUser(ctx context.Context, userID string) ([]domain.Workout, error) {
	rows, err := r.pool.Query(ctx, workoutSelect+`
		WHERE w.user_id = $1
		ORDER BY w.created_at DESC, w.id, e.position
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("query workouts: %w", err)
	}
	defer rows.Close()
	return scanWorkouts(rows)
}

// Delete removes a workout and (via ON DELETE CASCADE) its exercises.
func (r *PostgresRepository) Delete(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM workouts WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete workout: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// scanWorkouts groups a workout-by-exercise row set into the corresponding
// slice of domain.Workout. The input must be ordered such that all rows for a
// single workout are contiguous (true for both Get and ListByUser above).
//
// We track the current workout by index rather than by pointer because the
// backing slice is grown with append() and a stale pointer would dangle after
// a reallocation.
func scanWorkouts(rows pgx.Rows) ([]domain.Workout, error) {
	out := make([]domain.Workout, 0)
	currentIdx := -1
	for rows.Next() {
		var (
			id, userID, name, notes string
			createdAt               time.Time
			position                *int32
			exName                  *string
			sets, reps              *int32
			weightKg                *float64
		)
		if err := rows.Scan(
			&id, &userID, &name, &notes, &createdAt,
			&position, &exName, &sets, &reps, &weightKg,
		); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		if currentIdx < 0 || out[currentIdx].ID != id {
			out = append(out, domain.Workout{
				ID:        id,
				UserID:    userID,
				Name:      name,
				Notes:     notes,
				CreatedAt: createdAt,
				Exercises: []domain.LoggedExercise{},
			})
			currentIdx = len(out) - 1
		}
		// position is NULL only when the LEFT JOIN found no exercises for this
		// workout — leave Exercises as the empty slice initialized above.
		if position != nil {
			out[currentIdx].Exercises = append(out[currentIdx].Exercises, domain.LoggedExercise{
				Name:     deref(exName),
				Sets:     int(deref(sets)),
				Reps:     int(deref(reps)),
				WeightKG: deref(weightKg),
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}
	return out, nil
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}
