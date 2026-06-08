package biometrics

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresTokenRepository is the Postgres implementation of TokenRepository,
// persisting ciphertexts produced by TokenStore. Plaintext tokens never
// arrive here.
type PostgresTokenRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresTokenRepository returns a TokenRepository backed by pool.
func NewPostgresTokenRepository(pool *pgxpool.Pool) *PostgresTokenRepository {
	return &PostgresTokenRepository{pool: pool}
}

// Upsert inserts or replaces the row for (user_id, provider). The single
// statement handles both first-connect and reconnect; we never leave a
// partial row.
func (r *PostgresTokenRepository) Upsert(ctx context.Context, row TokenRow) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO oauth_tokens
			(user_id, provider, access_token_enc, refresh_token_enc, nonce,
			 expires_at, scopes, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (user_id, provider) DO UPDATE SET
			access_token_enc  = EXCLUDED.access_token_enc,
			refresh_token_enc = EXCLUDED.refresh_token_enc,
			nonce             = EXCLUDED.nonce,
			expires_at        = EXCLUDED.expires_at,
			scopes            = EXCLUDED.scopes,
			updated_at        = EXCLUDED.updated_at
	`,
		row.UserID, row.Provider,
		row.AccessTokenEnc, row.RefreshTokenEnc, row.Nonce,
		row.ExpiresAt, row.Scopes,
		row.CreatedAt, row.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert oauth token: %w", err)
	}
	return nil
}

// Get loads the row for (user_id, provider) or returns ErrTokenNotFound.
func (r *PostgresTokenRepository) Get(ctx context.Context, userID, provider string) (TokenRow, error) {
	var row TokenRow
	err := r.pool.QueryRow(ctx, `
		SELECT user_id, provider, access_token_enc, refresh_token_enc, nonce,
		       expires_at, scopes, created_at, updated_at
		FROM oauth_tokens
		WHERE user_id = $1 AND provider = $2
	`, userID, provider).Scan(
		&row.UserID, &row.Provider,
		&row.AccessTokenEnc, &row.RefreshTokenEnc, &row.Nonce,
		&row.ExpiresAt, &row.Scopes,
		&row.CreatedAt, &row.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TokenRow{}, ErrTokenNotFound
		}
		return TokenRow{}, fmt.Errorf("get oauth token: %w", err)
	}
	return row, nil
}

// Delete removes the row. Returns ErrTokenNotFound if no row matched.
func (r *PostgresTokenRepository) Delete(ctx context.Context, userID, provider string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM oauth_tokens WHERE user_id = $1 AND provider = $2`, userID, provider)
	if err != nil {
		return fmt.Errorf("delete oauth token: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrTokenNotFound
	}
	return nil
}

// ListPairs implements pairLister: returns every (user_id, provider) tuple
// that currently has a token. Used by the polling daemon.
func (r *PostgresTokenRepository) ListPairs(ctx context.Context) ([]TokenPair, error) {
	rows, err := r.pool.Query(ctx, `SELECT user_id, provider FROM oauth_tokens`)
	if err != nil {
		return nil, fmt.Errorf("list pairs: %w", err)
	}
	defer rows.Close()
	out := []TokenPair{}
	for rows.Next() {
		var p TokenPair
		if err := rows.Scan(&p.UserID, &p.Provider); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
