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
		if _, err := tx.Exec(ctx, `
			INSERT INTO plan_days (plan_id, day_idx, name)
			VALUES ($1, $2, $3)
		`, p.ID, day.Index, day.Name); err != nil {
			return fmt.Errorf("insert day %d: %w", day.Index, err)
		}

		for _, ex := range day.Exercises {
			exJSON, err := json.Marshal(ex.Exercise)
			if err != nil {
				return fmt.Errorf("marshal exercise (day %d, order %d): %w",
					day.Index, ex.Order, err)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO plan_exercises
					(plan_id, day_idx, order_idx, exercise,
					 sets, reps_low, reps_high, rest_seconds)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			`, p.ID, day.Index, ex.Order, exJSON,
				ex.Sets, ex.RepsLow, ex.RepsHigh, ex.RestSeconds); err != nil {
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
	       d.day_idx, d.name,
	       e.order_idx, e.exercise, e.sets, e.reps_low, e.reps_high, e.rest_seconds
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
			dayIdx   *int32
			dayName  *string
			orderIdx *int32
			exJSON   []byte
			sets     *int32
			repsLow  *int32
			repsHigh *int32
			restSec  *int32
		)
		if err := rows.Scan(
			&id, &userID, &goal, &exp, &daysPerWeek, &split, &warnings, &createdAt,
			&dayIdx, &dayName,
			&orderIdx, &exJSON, &sets, &repsLow, &repsHigh, &restSec,
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
		dayN := len(plans[planIdx].Days) - 1
		plans[planIdx].Days[dayN].Exercises = append(
			plans[planIdx].Days[dayN].Exercises,
			domain.PlanExercise{
				Exercise:    ex,
				Order:       int(deref(orderIdx)),
				Sets:        int(deref(sets)),
				RepsLow:     int(deref(repsLow)),
				RepsHigh:    int(deref(repsHigh)),
				RestSeconds: int(deref(restSec)),
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
