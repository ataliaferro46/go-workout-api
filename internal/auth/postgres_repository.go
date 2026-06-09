package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// pgUniqueViolation is Postgres's SQLSTATE for unique-constraint violation —
// the way we detect "email already registered" without a separate SELECT.
const pgUniqueViolation = "23505"

// --- users -------------------------------------------------------------

type PostgresUserRepository struct{ pool *pgxpool.Pool }

func NewPostgresUserRepository(pool *pgxpool.Pool) *PostgresUserRepository {
	return &PostgresUserRepository{pool: pool}
}

func (r *PostgresUserRepository) Create(ctx context.Context, c UserCredentials) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO users (id, email, password_hash, email_verified_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, c.User.ID, c.User.Email, c.PasswordHash, c.User.EmailVerifiedAt,
		c.User.CreatedAt, c.User.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return ErrEmailTaken
		}
		return fmt.Errorf("insert user: %w", err)
	}
	return nil
}

func (r *PostgresUserRepository) GetByID(ctx context.Context, id string) (UserCredentials, error) {
	return r.scanOne(ctx, `WHERE id = $1`, id)
}

func (r *PostgresUserRepository) GetByEmail(ctx context.Context, email string) (UserCredentials, error) {
	return r.scanOne(ctx, `WHERE lower(email) = lower($1)`, email)
}

func (r *PostgresUserRepository) scanOne(ctx context.Context, where string, arg any) (UserCredentials, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, email, password_hash, email_verified_at,
		       height_cm, weight_kg, birth_date, sex,
		       created_at, updated_at
		FROM users `+where+` LIMIT 1`, arg)

	var c UserCredentials
	var verified *time.Time
	var height *int32
	var weight *float64
	var birth *time.Time
	var sex *string
	err := row.Scan(&c.User.ID, &c.User.Email, &c.PasswordHash, &verified,
		&height, &weight, &birth, &sex,
		&c.User.CreatedAt, &c.User.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return UserCredentials{}, ErrUserNotFound
		}
		return UserCredentials{}, fmt.Errorf("get user: %w", err)
	}
	c.User.EmailVerifiedAt = verified
	if height != nil {
		v := int(*height)
		c.User.HeightCM = &v
	}
	c.User.WeightKG = weight
	c.User.BirthDate = birth
	if sex != nil {
		c.User.Sex = *sex
	}
	return c, nil
}

// UpdateProfile persists user-edited profile fields (height/weight/birth/sex).
// Any pointer field set to nil leaves the column unchanged.
func (r *PostgresUserRepository) UpdateProfile(ctx context.Context, userID string, heightCM *int, weightKG *float64, birth *time.Time, sex *string, now time.Time) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE users SET
			height_cm  = COALESCE($2, height_cm),
			weight_kg  = COALESCE($3, weight_kg),
			birth_date = COALESCE($4, birth_date),
			sex        = COALESCE($5, sex),
			updated_at = $6
		WHERE id = $1
	`, userID, heightCM, weightKG, birth, sex, now)
	if err != nil {
		return fmt.Errorf("update profile: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

func (r *PostgresUserRepository) MarkVerified(ctx context.Context, userID string, when time.Time) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE users SET email_verified_at = $2, updated_at = $2
		WHERE id = $1
	`, userID, when)
	if err != nil {
		return fmt.Errorf("mark verified: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

// --- sessions ----------------------------------------------------------

type PostgresSessionRepository struct{ pool *pgxpool.Pool }

func NewPostgresSessionRepository(pool *pgxpool.Pool) *PostgresSessionRepository {
	return &PostgresSessionRepository{pool: pool}
}

func (r *PostgresSessionRepository) Insert(ctx context.Context, s Session) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO sessions (token_hash, user_id, expires_at, created_at, user_agent, ip)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, s.TokenHash, s.UserID, s.ExpiresAt, s.CreatedAt, s.UserAgent, s.IP)
	if err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	return nil
}

func (r *PostgresSessionRepository) GetByTokenHash(ctx context.Context, tokenHash string) (Session, error) {
	var s Session
	err := r.pool.QueryRow(ctx, `
		SELECT token_hash, user_id, expires_at, created_at, user_agent, ip
		FROM sessions WHERE token_hash = $1
	`, tokenHash).Scan(&s.TokenHash, &s.UserID, &s.ExpiresAt, &s.CreatedAt, &s.UserAgent, &s.IP)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Session{}, ErrSessionExpired
		}
		return Session{}, fmt.Errorf("get session: %w", err)
	}
	if time.Now().After(s.ExpiresAt) {
		return Session{}, ErrSessionExpired
	}
	return s, nil
}

func (r *PostgresSessionRepository) Delete(ctx context.Context, tokenHash string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func (r *PostgresSessionRepository) DeleteAllForUser(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("delete sessions for user: %w", err)
	}
	return nil
}

// --- verifications -----------------------------------------------------

type PostgresVerificationRepository struct{ pool *pgxpool.Pool }

func NewPostgresVerificationRepository(pool *pgxpool.Pool) *PostgresVerificationRepository {
	return &PostgresVerificationRepository{pool: pool}
}

func (r *PostgresVerificationRepository) Insert(ctx context.Context, v Verification) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO email_verifications (token_hash, user_id, expires_at, created_at)
		VALUES ($1, $2, $3, $4)
	`, v.TokenHash, v.UserID, v.ExpiresAt, v.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert verification: %w", err)
	}
	return nil
}

func (r *PostgresVerificationRepository) Consume(ctx context.Context, tokenHash string, when time.Time) (Verification, error) {
	// Two-step but atomic via a CTE: load the row, check it's not expired and
	// not already consumed, mark consumed, return the row. RETURNING gives
	// us the post-update view in one round trip.
	var v Verification
	var consumed *time.Time
	err := r.pool.QueryRow(ctx, `
		UPDATE email_verifications
		SET consumed_at = $2
		WHERE token_hash = $1 AND consumed_at IS NULL AND expires_at > $2
		RETURNING token_hash, user_id, expires_at, created_at, consumed_at
	`, tokenHash, when).Scan(&v.TokenHash, &v.UserID, &v.ExpiresAt, &v.CreatedAt, &consumed)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Verification{}, ErrTokenInvalid
		}
		// Anything else is unexpected — the WHERE clause guarantees there's
		// either one matching row or none.
		if strings.Contains(err.Error(), "no rows") {
			return Verification{}, ErrTokenInvalid
		}
		return Verification{}, fmt.Errorf("consume verification: %w", err)
	}
	v.ConsumedAt = consumed
	return v, nil
}
