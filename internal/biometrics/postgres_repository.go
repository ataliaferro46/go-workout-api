package biometrics

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresReadingRepository persists biometric_readings using the UNIQUE
// constraint on (provider, idempotency_key) to make inserts idempotent.
type PostgresReadingRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresReadingRepository returns a Postgres reading repo.
func NewPostgresReadingRepository(pool *pgxpool.Pool) *PostgresReadingRepository {
	return &PostgresReadingRepository{pool: pool}
}

func (r *PostgresReadingRepository) Insert(ctx context.Context, reading domain.Reading) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO biometric_readings
			(id, user_id, provider, kind, value, recorded_at, ingested_at,
			 idempotency_key, raw_payload)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (provider, idempotency_key) DO NOTHING
	`,
		reading.ID, reading.UserID, reading.Provider, string(reading.Kind),
		reading.Value, reading.RecordedAt, reading.IngestedAt,
		reading.IdempotencyKey, reading.RawPayload,
	)
	if err != nil {
		return fmt.Errorf("insert reading: %w", err)
	}
	return nil
}

func (r *PostgresReadingRepository) LatestByUserAndKind(ctx context.Context, userID string, kind domain.Kind) (domain.Reading, error) {
	var reading domain.Reading
	var kindStr string
	err := r.pool.QueryRow(ctx, `
		SELECT id, user_id, provider, kind, value, recorded_at, ingested_at,
		       idempotency_key, raw_payload
		FROM biometric_readings
		WHERE user_id = $1 AND kind = $2
		ORDER BY recorded_at DESC
		LIMIT 1
	`, userID, string(kind)).Scan(
		&reading.ID, &reading.UserID, &reading.Provider, &kindStr,
		&reading.Value, &reading.RecordedAt, &reading.IngestedAt,
		&reading.IdempotencyKey, &reading.RawPayload,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Reading{}, domain.ErrNotFound
		}
		return domain.Reading{}, fmt.Errorf("query latest: %w", err)
	}
	reading.Kind = domain.Kind(kindStr)
	return reading, nil
}

func (r *PostgresReadingRepository) ListByUser(ctx context.Context, userID string) ([]domain.Reading, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, user_id, provider, kind, value, recorded_at, ingested_at,
		       idempotency_key, raw_payload
		FROM biometric_readings
		WHERE user_id = $1
		ORDER BY recorded_at DESC, id ASC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list readings: %w", err)
	}
	defer rows.Close()

	out := make([]domain.Reading, 0)
	for rows.Next() {
		var reading domain.Reading
		var kindStr string
		if err := rows.Scan(
			&reading.ID, &reading.UserID, &reading.Provider, &kindStr,
			&reading.Value, &reading.RecordedAt, &reading.IngestedAt,
			&reading.IdempotencyKey, &reading.RawPayload,
		); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		reading.Kind = domain.Kind(kindStr)
		out = append(out, reading)
	}
	return out, rows.Err()
}

// PostgresSyncStateRepository persists biometric_sync_state watermarks.
type PostgresSyncStateRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresSyncStateRepository(pool *pgxpool.Pool) *PostgresSyncStateRepository {
	return &PostgresSyncStateRepository{pool: pool}
}

func (r *PostgresSyncStateRepository) Get(ctx context.Context, userID, provider string) (SyncState, error) {
	var s SyncState
	var cursor *string
	err := r.pool.QueryRow(ctx, `
		SELECT user_id, provider, last_synced_at, last_event_cursor
		FROM biometric_sync_state
		WHERE user_id = $1 AND provider = $2
	`, userID, provider).Scan(&s.UserID, &s.Provider, &s.LastSyncedAt, &cursor)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SyncState{}, ErrSyncStateNotFound
		}
		return SyncState{}, fmt.Errorf("get sync state: %w", err)
	}
	if cursor != nil {
		s.LastEventCursor = *cursor
	}
	return s, nil
}

func (r *PostgresSyncStateRepository) Set(ctx context.Context, state SyncState) error {
	var cursor *string
	if state.LastEventCursor != "" {
		cursor = &state.LastEventCursor
	}
	if state.LastSyncedAt.IsZero() {
		state.LastSyncedAt = time.Now().UTC()
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO biometric_sync_state (user_id, provider, last_synced_at, last_event_cursor)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, provider) DO UPDATE SET
			last_synced_at    = EXCLUDED.last_synced_at,
			last_event_cursor = EXCLUDED.last_event_cursor
	`, state.UserID, state.Provider, state.LastSyncedAt, cursor)
	if err != nil {
		return fmt.Errorf("set sync state: %w", err)
	}
	return nil
}
