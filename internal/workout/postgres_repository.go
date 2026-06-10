package workout

import (
	"context"
	"encoding/json"
	"errors"
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

	var planArg any
	var planDayArg any
	if w.PlanID != "" {
		planArg = w.PlanID
	}
	if w.PlanDayIdx > 0 {
		planDayArg = w.PlanDayIdx
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO workouts (id, user_id, name, notes, created_at, plan_id, plan_day_idx)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, w.ID, w.UserID, w.Name, w.Notes, w.CreatedAt, planArg, planDayArg); err != nil {
		return fmt.Errorf("insert workout: %w", err)
	}
	for i, ex := range w.Exercises {
		prescriptionJSON := []byte("{}")
		if ex.Prescription != nil {
			b, err := json.Marshal(ex.Prescription)
			if err != nil {
				return fmt.Errorf("marshal prescription %d: %w", i, err)
			}
			prescriptionJSON = b
		}
		targetReps := ex.TargetReps
		if targetReps == nil {
			targetReps = []int{}
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO workout_exercises
				(workout_id, position, name, sets, reps, weight_kg, prescription, target_reps)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, w.ID, i, ex.Name, ex.Sets, ex.Reps, ex.WeightKG, prescriptionJSON, targetReps); err != nil {
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
	       w.plan_id, w.plan_day_idx, w.type,
	       e.position, e.name, e.sets, e.reps, e.weight_kg,
	       e.prescription, e.target_reps
	FROM workouts w
	LEFT JOIN workout_exercises e ON e.workout_id = w.id
`

// Get returns the workout with the given ID or domain.ErrNotFound.
// LoggedSets are eagerly attached to each exercise — the workout-in-progress
// page needs them to render which sets are already done.
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
	w := workouts[0]
	setsByPos, err := r.GetSets(ctx, id)
	if err != nil {
		return domain.Workout{}, err
	}
	for i := range w.Exercises {
		if logs, ok := setsByPos[i]; ok {
			w.Exercises[i].LoggedSets = logs
		}
	}
	if w.Type == "cardio" {
		cs, err := r.GetCardioSession(ctx, id)
		if err != nil {
			return domain.Workout{}, err
		}
		w.CardioSession = cs
	}
	return w, nil
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

// CreateCardio inserts a cardio workout (workouts row + cardio_sessions
// row) in one transaction.
func (r *PostgresRepository) CreateCardio(ctx context.Context, w domain.Workout) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		INSERT INTO workouts (id, user_id, name, notes, type, created_at)
		VALUES ($1, $2, $3, $4, 'cardio', $5)
	`, w.ID, w.UserID, w.Name, w.Notes, w.CreatedAt); err != nil {
		return fmt.Errorf("insert cardio workout: %w", err)
	}
	if w.CardioSession != nil {
		cs := w.CardioSession
		var dist any
		if cs.DistanceKM > 0 {
			dist = cs.DistanceKM
		}
		var hr any
		if cs.AvgHR > 0 {
			hr = cs.AvgHR
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO cardio_sessions
				(workout_id, activity, intensity, duration_minutes, distance_km,
				 avg_hr, calories, calories_source, notes)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`, w.ID, cs.Activity, cs.Intensity, cs.DurationMinutes, dist,
			hr, cs.Calories, cs.CaloriesSource, cs.Notes); err != nil {
			return fmt.Errorf("insert cardio session: %w", err)
		}
	}
	return tx.Commit(ctx)
}

// GetCardioSession returns a cardio session detail attached to a workout
// id, or nil if the workout isn't cardio / has no row.
func (r *PostgresRepository) GetCardioSession(ctx context.Context, workoutID string) (*domain.CardioSession, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT activity, intensity, duration_minutes, distance_km,
		       avg_hr, calories, calories_source, notes
		FROM cardio_sessions WHERE workout_id = $1
	`, workoutID)
	var cs domain.CardioSession
	var dist *float64
	var hr *int32
	if err := row.Scan(&cs.Activity, &cs.Intensity, &cs.DurationMinutes,
		&dist, &hr, &cs.Calories, &cs.CaloriesSource, &cs.Notes); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get cardio session: %w", err)
	}
	if dist != nil {
		cs.DistanceKM = *dist
	}
	if hr != nil {
		cs.AvgHR = int(*hr)
	}
	return &cs, nil
}

// LastSetsForExercise returns the most recent logged sets for the given
// exercise name belonging to userID. Used by the live workout page to
// show "last time you did this: 3 × 8 @ 80 kg" as a reference. Looks
// back across all workouts and returns the sets from the latest matching
// exercise instance.
func (r *PostgresRepository) LastSetsForExercise(ctx context.Context, userID, exerciseName string) ([]domain.LoggedSet, time.Time, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT w.id, e.position
		FROM workouts w
		JOIN workout_exercises e ON e.workout_id = w.id
		WHERE w.user_id = $1 AND lower(e.name) = lower($2)
		ORDER BY w.created_at DESC
		LIMIT 1
	`, userID, exerciseName)
	var workoutID string
	var position int32
	if err := row.Scan(&workoutID, &position); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, time.Time{}, nil
		}
		return nil, time.Time{}, fmt.Errorf("find last exercise: %w", err)
	}
	rows, err := r.pool.Query(ctx, `
		SELECT set_number, reps, weight_kg, completed_at
		FROM workout_sets
		WHERE workout_id = $1 AND exercise_position = $2
		ORDER BY set_number
	`, workoutID, position)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("query last sets: %w", err)
	}
	defer rows.Close()
	out := make([]domain.LoggedSet, 0)
	var when time.Time
	for rows.Next() {
		var num, reps int32
		var weight float64
		var completedAt time.Time
		if err := rows.Scan(&num, &reps, &weight, &completedAt); err != nil {
			return nil, time.Time{}, err
		}
		out = append(out, domain.LoggedSet{
			SetNumber: int(num), Reps: int(reps), WeightKG: weight, CompletedAt: completedAt,
		})
		if completedAt.After(when) {
			when = completedAt
		}
	}
	return out, when, rows.Err()
}

// UpdateExercise patches the prescription (sets, reps, weight, target
// reps, set type) of a single workout_exercises row. Used by the inline
// editor on the live workout page so the user can adjust their working
// scheme mid-session ("I'm going to do 4 sets instead of 3").
func (r *PostgresRepository) UpdateExercise(ctx context.Context, workoutID string, position int, sets, reps int, weightKG float64, targetReps []int, prescription *domain.ExercisePrescription) error {
	tr := targetReps
	if tr == nil {
		tr = []int{}
	}
	prescriptionJSON := []byte("{}")
	if prescription != nil {
		b, err := json.Marshal(prescription)
		if err != nil {
			return fmt.Errorf("marshal prescription: %w", err)
		}
		prescriptionJSON = b
	}
	tag, err := r.pool.Exec(ctx, `
		UPDATE workout_exercises
		SET sets = $3, reps = $4, weight_kg = $5,
		    target_reps = $6, prescription = $7
		WHERE workout_id = $1 AND position = $2
	`, workoutID, position, sets, reps, weightKG, tr, prescriptionJSON)
	if err != nil {
		return fmt.Errorf("update exercise: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// SwapExerciseName replaces the exercise name at (workout_id, position).
// Used by the in-workout swap-with-similar flow — the user picks a new
// movement, the row's name updates in place, the prescription
// metadata (set type / warmups) is wiped because they may no longer
// apply, and any already-logged sets are preserved.
func (r *PostgresRepository) SwapExerciseName(ctx context.Context, workoutID string, position int, newName string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE workout_exercises
		SET name = $3, prescription = '{}'::jsonb
		WHERE workout_id = $1 AND position = $2
	`, workoutID, position, newName)
	if err != nil {
		return fmt.Errorf("swap exercise: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// LogSet records a per-set log entry. Idempotent on (workout_id,
// exercise_position, set_number) — re-logging the same set overwrites
// the previous values, so the front-end can let the user correct typos
// by re-submitting.
func (r *PostgresRepository) LogSet(ctx context.Context, workoutID string, exercisePosition, setNumber, reps int, weightKG float64, completedAt time.Time) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO workout_sets
			(workout_id, exercise_position, set_number, reps, weight_kg, completed_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (workout_id, exercise_position, set_number)
		DO UPDATE SET reps = EXCLUDED.reps,
		              weight_kg = EXCLUDED.weight_kg,
		              completed_at = EXCLUDED.completed_at
	`, workoutID, exercisePosition, setNumber, reps, weightKG, completedAt)
	if err != nil {
		return fmt.Errorf("log set: %w", err)
	}
	return nil
}

// GetSets returns all logged sets for a workout, ordered by exercise
// position and set number.
func (r *PostgresRepository) GetSets(ctx context.Context, workoutID string) (map[int][]domain.LoggedSet, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT exercise_position, set_number, reps, weight_kg, completed_at
		FROM workout_sets
		WHERE workout_id = $1
		ORDER BY exercise_position, set_number
	`, workoutID)
	if err != nil {
		return nil, fmt.Errorf("query sets: %w", err)
	}
	defer rows.Close()
	out := make(map[int][]domain.LoggedSet)
	for rows.Next() {
		var pos, setNum, reps int32
		var weight float64
		var completedAt time.Time
		if err := rows.Scan(&pos, &setNum, &reps, &weight, &completedAt); err != nil {
			return nil, fmt.Errorf("scan set: %w", err)
		}
		out[int(pos)] = append(out[int(pos)], domain.LoggedSet{
			SetNumber: int(setNum), Reps: int(reps), WeightKG: weight, CompletedAt: completedAt,
		})
	}
	return out, rows.Err()
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
			planID                  *string
			planDayIdx              *int32
			workoutType             string
			position                *int32
			exName                  *string
			sets, reps              *int32
			weightKg                *float64
			prescriptionJSON        []byte
			targetReps              []int32
		)
		if err := rows.Scan(
			&id, &userID, &name, &notes, &createdAt,
			&planID, &planDayIdx, &workoutType,
			&position, &exName, &sets, &reps, &weightKg,
			&prescriptionJSON, &targetReps,
		); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		if currentIdx < 0 || out[currentIdx].ID != id {
			out = append(out, domain.Workout{
				ID:         id,
				UserID:     userID,
				Name:       name,
				Notes:      notes,
				Type:       workoutType,
				PlanID:     deref(planID),
				PlanDayIdx: int(deref(planDayIdx)),
				CreatedAt:  createdAt,
				Exercises:  []domain.LoggedExercise{},
			})
			currentIdx = len(out) - 1
		}
		// position is NULL only when the LEFT JOIN found no exercises for this
		// workout — leave Exercises as the empty slice initialized above.
		if position != nil {
			le := domain.LoggedExercise{
				Name:     deref(exName),
				Sets:     int(deref(sets)),
				Reps:     int(deref(reps)),
				WeightKG: deref(weightKg),
			}
			if len(targetReps) > 0 {
				le.TargetReps = make([]int, len(targetReps))
				for i, v := range targetReps {
					le.TargetReps[i] = int(v)
				}
			}
			if len(prescriptionJSON) > 0 && string(prescriptionJSON) != "{}" {
				var p domain.ExercisePrescription
				if err := json.Unmarshal(prescriptionJSON, &p); err == nil &&
					(p.SetType != "" || p.SetTypeNote != "" || len(p.Warmups) > 0 || p.RestSeconds > 0) {
					le.Prescription = &p
				}
			}
			out[currentIdx].Exercises = append(out[currentIdx].Exercises, le)
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
