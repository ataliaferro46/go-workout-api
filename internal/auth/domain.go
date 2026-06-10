// Package auth provides email/password authentication, email verification,
// and server-side session management. The user identifier exposed to the rest
// of the codebase (User.ID) is the same opaque string the existing handlers
// expect — auth replaces the X-User-ID header placeholder without changing
// downstream code.
//
// See ARCHITECTURE.md ADRs 064–067 for the surrounding decisions.
package auth

import (
	"errors"
	"time"
)

// Sentinel errors for the service surface. Mapped to HTTP status codes by
// the handler — kept here so future callers (admin tools, CLI) can branch
// on category without inventing their own taxonomy.
var (
	ErrEmailTaken       = errors.New("email already registered")
	ErrInvalidLogin     = errors.New("invalid email or password")
	ErrEmailNotVerified = errors.New("email not verified")
	ErrTokenInvalid     = errors.New("token invalid or expired")
	ErrSessionExpired   = errors.New("session expired")
	ErrUserNotFound     = errors.New("user not found")
)

// User is an authenticated account. Password hash never leaves the package —
// the Service returns a sanitized copy to the handler.
//
// Profile fields (HeightCM, WeightKG, BirthDate, Sex) are optional. They
// enable calorie estimation via the MET formula when a wearable isn't
// connected, and BMI display on the settings page. Sex is free-form to
// avoid imposing a schema-level enum on a personal field.
type User struct {
	ID              string     `json:"id"`
	Email           string     `json:"email"`
	EmailVerifiedAt *time.Time `json:"email_verified_at,omitempty"`
	HeightCM        *int       `json:"height_cm,omitempty"`
	WeightKG        *float64   `json:"weight_kg,omitempty"`
	BirthDate       *time.Time `json:"birth_date,omitempty"`
	Sex             string     `json:"sex,omitempty"`
	BodyFatPercentage *float64 `json:"body_fat_percentage,omitempty"`
	WorkoutTime     string     `json:"workout_time,omitempty"` // "HH:MM" local
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// UserCredentials carries the password hash in memory; it never round-trips
// out of the auth package.
type UserCredentials struct {
	User         User
	PasswordHash string
}

// Session is a logged-in session bound to a user. The plaintext Token is
// what the client carries in the cookie; the server stores SHA-256(Token)
// as the row's primary key so a database dump cannot be used to forge
// sessions.
type Session struct {
	Token     string // plaintext — only available immediately after creation
	TokenHash string // SHA-256 hex of Token — what the repository stores
	UserID    string
	ExpiresAt time.Time
	CreatedAt time.Time
	UserAgent string
	IP        string
}

// Verification is a one-shot token sent to a user's email to prove
// ownership. The Token is the value embedded in the verification link;
// TokenHash is what the repository persists.
type Verification struct {
	Token      string // plaintext — only available immediately after creation
	TokenHash  string
	UserID     string
	ExpiresAt  time.Time
	CreatedAt  time.Time
	ConsumedAt *time.Time
}
