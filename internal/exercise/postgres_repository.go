package exercise

import (
	"context"
	"errors"
	"fmt"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresRepository persists the exercise library in a single `exercises`
// table with TEXT[] columns for the slice-shaped fields. It satisfies the
// same Repository interface as InMemoryRepository — the Service is unaware of
// which implementation it holds.
//
// The Scan path uses []string for the array columns and converts to the
// typed slice (MuscleGroup, Equipment, BodyPart) at the boundary. The reverse
// direction works because Go converts a typed string slice to []string
// implicitly when passing to pgx if we do the conversion explicitly first;
// we do.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository returns a Repository backed by the given pool.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

// pgUniqueViolation is Postgres's SQLSTATE for a unique-constraint violation.
const pgUniqueViolation = "23505"

func (r *PostgresRepository) Create(ctx context.Context, e domain.Exercise) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO exercises
			(id, name, primary_muscle, secondary_muscles, pattern,
			 required_equipment, compound, min_level, contraindications, region)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`,
		e.ID,
		e.Name,
		string(e.PrimaryMuscle),
		muscleGroupsToStrings(e.SecondaryMuscles),
		string(e.Pattern),
		equipmentToStrings(e.RequiredEquipment),
		e.Compound,
		string(e.MinLevel),
		bodyPartsToStrings(e.Contraindications),
		e.Region,
	)
	if err != nil {
		return mapInsertError("insert exercise", err)
	}
	return nil
}

func (r *PostgresRepository) Get(ctx context.Context, id string) (domain.Exercise, error) {
	row := r.pool.QueryRow(ctx, exerciseSelect+` WHERE id = $1`, id)
	e, err := scanExercise(row.Scan)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Exercise{}, domain.ErrNotFound
		}
		return domain.Exercise{}, fmt.Errorf("get exercise: %w", err)
	}
	return e, nil
}

func (r *PostgresRepository) Update(ctx context.Context, e domain.Exercise) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE exercises SET
			name = $2,
			primary_muscle = $3,
			secondary_muscles = $4,
			pattern = $5,
			required_equipment = $6,
			compound = $7,
			min_level = $8,
			contraindications = $9,
			region = $10
		WHERE id = $1
	`,
		e.ID,
		e.Name,
		string(e.PrimaryMuscle),
		muscleGroupsToStrings(e.SecondaryMuscles),
		string(e.Pattern),
		equipmentToStrings(e.RequiredEquipment),
		e.Compound,
		string(e.MinLevel),
		bodyPartsToStrings(e.Contraindications),
		e.Region,
	)
	if err != nil {
		return mapInsertError("update exercise", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) Delete(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM exercises WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete exercise: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) List(ctx context.Context) ([]domain.Exercise, error) {
	rows, err := r.pool.Query(ctx, exerciseSelect+` ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list exercises: %w", err)
	}
	defer rows.Close()
	return scanExercises(rows)
}

func (r *PostgresRepository) ListByPattern(ctx context.Context, pattern domain.MovementPattern) ([]domain.Exercise, error) {
	rows, err := r.pool.Query(ctx,
		exerciseSelect+` WHERE pattern = $1 ORDER BY id`,
		string(pattern))
	if err != nil {
		return nil, fmt.Errorf("list by pattern: %w", err)
	}
	defer rows.Close()
	return scanExercises(rows)
}

const exerciseSelect = `
	SELECT id, name, primary_muscle, secondary_muscles, pattern,
	       required_equipment, compound, min_level, contraindications, region
	FROM exercises
`

// scanFn is what both Row.Scan and Rows.Scan satisfy, so scanExercise can be
// used in single-row (Get) and multi-row (List, ListByPattern) contexts.
type scanFn func(dest ...any) error

func scanExercise(scan scanFn) (domain.Exercise, error) {
	var (
		id, name, primary, pattern, minLevel, region string
		secondary, equipment, contras                []string
		compound                                     bool
	)
	if err := scan(
		&id, &name, &primary, &secondary, &pattern,
		&equipment, &compound, &minLevel, &contras, &region,
	); err != nil {
		return domain.Exercise{}, err
	}
	return domain.Exercise{
		ID:                id,
		Name:              name,
		PrimaryMuscle:     domain.MuscleGroup(primary),
		SecondaryMuscles:  stringsToMuscleGroups(secondary),
		Pattern:           domain.MovementPattern(pattern),
		RequiredEquipment: stringsToEquipment(equipment),
		Compound:          compound,
		MinLevel:          domain.ExperienceLevel(minLevel),
		Contraindications: stringsToBodyParts(contras),
		Region:            region,
	}, nil
}

func scanExercises(rows pgx.Rows) ([]domain.Exercise, error) {
	out := make([]domain.Exercise, 0)
	for rows.Next() {
		e, err := scanExercise(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan exercise: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}
	return out, nil
}

func mapInsertError(op string, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
		return ErrDuplicateName
	}
	return fmt.Errorf("%s: %w", op, err)
}

// --- typed-slice <-> []string conversion helpers ---------------------------
//
// We store typed-string slices ([]MuscleGroup, []Equipment, []BodyPart) in
// TEXT[] columns. pgx v5 supports []string scan/encode natively; the typed
// versions require either registered codecs or explicit conversion. Explicit
// conversion is the simpler choice for a single repository.

func muscleGroupsToStrings(in []domain.MuscleGroup) []string {
	if in == nil {
		return []string{}
	}
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = string(v)
	}
	return out
}

func equipmentToStrings(in []domain.Equipment) []string {
	if in == nil {
		return []string{}
	}
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = string(v)
	}
	return out
}

func bodyPartsToStrings(in []domain.BodyPart) []string {
	if in == nil {
		return []string{}
	}
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = string(v)
	}
	return out
}

func stringsToMuscleGroups(in []string) []domain.MuscleGroup {
	if len(in) == 0 {
		return nil
	}
	out := make([]domain.MuscleGroup, len(in))
	for i, v := range in {
		out[i] = domain.MuscleGroup(v)
	}
	return out
}

func stringsToEquipment(in []string) []domain.Equipment {
	if len(in) == 0 {
		return nil
	}
	out := make([]domain.Equipment, len(in))
	for i, v := range in {
		out[i] = domain.Equipment(v)
	}
	return out
}

func stringsToBodyParts(in []string) []domain.BodyPart {
	if len(in) == 0 {
		return nil
	}
	out := make([]domain.BodyPart, len(in))
	for i, v := range in {
		out[i] = domain.BodyPart(v)
	}
	return out
}
