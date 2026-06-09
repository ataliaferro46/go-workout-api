package plan

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresRepository persists generated workout plans across three tables:
// workout_plans (header), plan_days, plan_exercises. The domain.Exercise
// nested inside each PlanExercise is stored as JSONB so the snapshot is
// self-contained — a plan rendered tomorrow looks the way it did when
// generated, even if the in-code exercise library is later edited or pruned.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository returns a Repository backed by the given pool.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

// Create inserts the plan header, its days, and its exercises in a single
// transaction. The deferred Rollback is a no-op after a successful Commit, so
// either everything lands or nothing does.
func (r *PostgresRepository) Create(ctx context.Context, p domain.WorkoutPlan) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	warnings := p.Warnings
	if warnings == nil {
		warnings = []string{}
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO workout_plans
			(id, user_id, goal, experience, days_per_week, split, warnings, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, p.ID, p.UserID, string(p.Goal), string(p.Experience), p.DaysPerWeek,
		p.Split, warnings, p.CreatedAt); err != nil {
		return fmt.Errorf("insert plan: %w", err)
	}

	for _, day := range p.Days {
		// Weekday is nullable in the schema; pass nil when the request
		// didn't bind days to a calendar so old plans round-trip cleanly.
		var weekdayArg any
		if day.Weekday != "" {
			weekdayArg = day.Weekday
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO plan_days (plan_id, day_idx, name, weekday)
			VALUES ($1, $2, $3, $4)
		`, p.ID, day.Index, day.Name, weekdayArg); err != nil {
			return fmt.Errorf("insert day %d: %w", day.Index, err)
		}

		for _, ex := range day.Exercises {
			exJSON, err := json.Marshal(ex.Exercise)
			if err != nil {
				return fmt.Errorf("marshal exercise (day %d, order %d): %w",
					day.Index, ex.Order, err)
			}
			warmups := ex.Warmups
			if warmups == nil {
				warmups = []domain.WarmupSet{}
			}
			warmupsJSON, err := json.Marshal(warmups)
			if err != nil {
				return fmt.Errorf("marshal warmups (day %d, order %d): %w",
					day.Index, ex.Order, err)
			}
			var supersetArg any
			if ex.SupersetWith != nil {
				supersetArg = *ex.SupersetWith
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO plan_exercises
					(plan_id, day_idx, order_idx, exercise, warmups,
					 sets, reps_low, reps_high, rest_seconds,
					 set_type, superset_with, set_type_note)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
			`, p.ID, day.Index, ex.Order, exJSON, warmupsJSON,
				ex.Sets, ex.RepsLow, ex.RepsHigh, ex.RestSeconds,
				string(ex.SetType), supersetArg, ex.SetTypeNote); err != nil {
				return fmt.Errorf("insert exercise (day %d, order %d): %w",
					day.Index, ex.Order, err)
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// planSelect projects the columns that scanPlans expects. Callers add WHERE /
// ORDER BY clauses to filter and order the result set.
const planSelect = `
	SELECT p.id, p.user_id, p.goal, p.experience, p.days_per_week, p.split,
	       p.warnings, p.created_at,
	       d.day_idx, d.name, d.weekday,
	       e.order_idx, e.exercise, e.warmups, e.sets, e.reps_low, e.reps_high, e.rest_seconds,
	       e.set_type, e.superset_with, e.set_type_note
	FROM workout_plans p
	LEFT JOIN plan_days       d ON d.plan_id = p.id
	LEFT JOIN plan_exercises  e ON e.plan_id = p.id AND e.day_idx = d.day_idx
`

// Get returns the plan with the given ID or domain.ErrNotFound.
func (r *PostgresRepository) Get(ctx context.Context, id string) (domain.WorkoutPlan, error) {
	rows, err := r.pool.Query(ctx,
		planSelect+` WHERE p.id = $1 ORDER BY d.day_idx, e.order_idx`, id)
	if err != nil {
		return domain.WorkoutPlan{}, fmt.Errorf("query plan: %w", err)
	}
	defer rows.Close()

	plans, err := scanPlans(rows)
	if err != nil {
		return domain.WorkoutPlan{}, err
	}
	if len(plans) == 0 {
		return domain.WorkoutPlan{}, domain.ErrNotFound
	}
	return plans[0], nil
}

// ListByUser returns a user's plans newest first.
func (r *PostgresRepository) ListByUser(ctx context.Context, userID string) ([]domain.WorkoutPlan, error) {
	rows, err := r.pool.Query(ctx, planSelect+`
		WHERE p.user_id = $1
		ORDER BY p.created_at DESC, p.id, d.day_idx, e.order_idx
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("query plans: %w", err)
	}
	defer rows.Close()
	return scanPlans(rows)
}

// UpdateExercise replaces the exercise + warmups at (plan_id, day_idx,
// order_idx). Sets / reps / rest stay the same — only the movement and its
// per-percent warmup ramp change.
func (r *PostgresRepository) UpdateExercise(ctx context.Context, planID string, dayIdx, orderIdx int, ex domain.Exercise, warmups []domain.WarmupSet) error {
	exJSON, err := json.Marshal(ex)
	if err != nil {
		return fmt.Errorf("marshal exercise: %w", err)
	}
	if warmups == nil {
		warmups = []domain.WarmupSet{}
	}
	warmupsJSON, err := json.Marshal(warmups)
	if err != nil {
		return fmt.Errorf("marshal warmups: %w", err)
	}
	tag, err := r.pool.Exec(ctx, `
		UPDATE plan_exercises
		SET exercise = $1, warmups = $2
		WHERE plan_id = $3 AND day_idx = $4 AND order_idx = $5
	`, exJSON, warmupsJSON, planID, dayIdx, orderIdx)
	if err != nil {
		return fmt.Errorf("update plan_exercise: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// ReorderDays remaps plan_days.day_idx (and the matching plan_exercises.
// day_idx) for the given plan according to mapping (old → new). Run in a
// single transaction so the FK-linked rows on plan_exercises stay
// consistent.
//
// We rewrite in two phases via a temporary offset (+1000) to avoid the
// transient state where a row's new index collides with another row's
// old index — Postgres would reject the UPDATE under the PK constraint
// (plan_id, day_idx).
func (r *PostgresRepository) ReorderDays(ctx context.Context, planID string, mapping map[int]int) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	const offset = 1000
	// Phase 1: shift every day's index by +offset. Both plan_days and
	// plan_exercises participate; FK uses (plan_id, day_idx) so we have
	// to walk both tables.
	for oldIdx := range mapping {
		if _, err := tx.Exec(ctx,
			`UPDATE plan_days SET day_idx = $1 WHERE plan_id = $2 AND day_idx = $3`,
			oldIdx+offset, planID, oldIdx); err != nil {
			return fmt.Errorf("phase1 plan_days %d: %w", oldIdx, err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE plan_exercises SET day_idx = $1 WHERE plan_id = $2 AND day_idx = $3`,
			oldIdx+offset, planID, oldIdx); err != nil {
			return fmt.Errorf("phase1 plan_exercises %d: %w", oldIdx, err)
		}
	}
	// Phase 2: map each shifted index to its new value.
	for oldIdx, newIdx := range mapping {
		if _, err := tx.Exec(ctx,
			`UPDATE plan_days SET day_idx = $1 WHERE plan_id = $2 AND day_idx = $3`,
			newIdx, planID, oldIdx+offset); err != nil {
			return fmt.Errorf("phase2 plan_days %d: %w", oldIdx, err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE plan_exercises SET day_idx = $1 WHERE plan_id = $2 AND day_idx = $3`,
			newIdx, planID, oldIdx+offset); err != nil {
			return fmt.Errorf("phase2 plan_exercises %d: %w", oldIdx, err)
		}
	}
	return tx.Commit(ctx)
}

// Delete removes a plan and (via ON DELETE CASCADE) its days + exercises.
func (r *PostgresRepository) Delete(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM workout_plans WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete plan: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// scanPlans groups a plan-by-day-by-exercise row set into the corresponding
// slice of domain.WorkoutPlan. Rows for a single plan are contiguous (the
// queries above ORDER BY plan, day, order) so we track current plan and
// current day by index and append into them.
//
// Index-based tracking is used instead of pointers because slice append() can
// reallocate the backing array, leaving any pointer-into-old-slice dangling.
func scanPlans(rows pgx.Rows) ([]domain.WorkoutPlan, error) {
	plans := make([]domain.WorkoutPlan, 0)
	planIdx := -1
	for rows.Next() {
		var (
			id, userID, goal, exp, split string
			daysPerWeek                  int
			warnings                     []string
			createdAt                    time.Time

			// Nullable because of LEFT JOIN: a plan with no days returns one
			// row with NULL day/exercise columns; a day with no exercises
			// returns one row with NULL exercise columns.
			dayIdx        *int32
			dayName       *string
			dayWeekday    *string
			orderIdx      *int32
			exJSON        []byte
			warmupsJSON   []byte
			sets          *int32
			repsLow       *int32
			repsHigh      *int32
			restSec       *int32
			setType       *string
			supersetWith  *int32
			setTypeNote   *string
		)
		if err := rows.Scan(
			&id, &userID, &goal, &exp, &daysPerWeek, &split, &warnings, &createdAt,
			&dayIdx, &dayName, &dayWeekday,
			&orderIdx, &exJSON, &warmupsJSON, &sets, &repsLow, &repsHigh, &restSec,
			&setType, &supersetWith, &setTypeNote,
		); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		if planIdx < 0 || plans[planIdx].ID != id {
			plans = append(plans, domain.WorkoutPlan{
				ID:          id,
				UserID:      userID,
				Goal:        domain.Goal(goal),
				Experience:  domain.ExperienceLevel(exp),
				DaysPerWeek: daysPerWeek,
				Split:       split,
				Warnings:    warningsOrEmpty(warnings),
				CreatedAt:   createdAt,
				Days:        []domain.PlanDay{},
			})
			planIdx = len(plans) - 1
		}

		if dayIdx == nil {
			continue
		}
		// Ensure the current day exists on the current plan, then append the
		// exercise if any.
		days := plans[planIdx].Days
		if len(days) == 0 || days[len(days)-1].Index != int(*dayIdx) {
			plans[planIdx].Days = append(plans[planIdx].Days, domain.PlanDay{
				Index:     int(*dayIdx),
				Name:      deref(dayName),
				Weekday:   deref(dayWeekday),
				Exercises: []domain.PlanExercise{},
			})
		}

		if orderIdx == nil {
			continue
		}
		var ex domain.Exercise
		if err := json.Unmarshal(exJSON, &ex); err != nil {
			return nil, fmt.Errorf("unmarshal exercise: %w", err)
		}
		var warmups []domain.WarmupSet
		if len(warmupsJSON) > 0 {
			if err := json.Unmarshal(warmupsJSON, &warmups); err != nil {
				return nil, fmt.Errorf("unmarshal warmups: %w", err)
			}
		}
		dayN := len(plans[planIdx].Days) - 1
		var supersetPtr *int
		if supersetWith != nil {
			v := int(*supersetWith)
			supersetPtr = &v
		}
		plans[planIdx].Days[dayN].Exercises = append(
			plans[planIdx].Days[dayN].Exercises,
			domain.PlanExercise{
				Exercise:     ex,
				Order:        int(deref(orderIdx)),
				Warmups:      warmups,
				Sets:         int(deref(sets)),
				RepsLow:      int(deref(repsLow)),
				RepsHigh:     int(deref(repsHigh)),
				RestSeconds:  int(deref(restSec)),
				SetType:      domain.SetType(deref(setType)),
				SupersetWith: supersetPtr,
				SetTypeNote:  deref(setTypeNote),
			},
		)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}
	return plans, nil
}

func warningsOrEmpty(w []string) []string {
	if w == nil {
		return []string{}
	}
	return w
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}
