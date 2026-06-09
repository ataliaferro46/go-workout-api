package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/mail"
	"strings"
	"time"
)

// Service orchestrates the auth flow: signup → verify → login → session
// lifecycle. Constructed once at boot; concurrency-safe.
type Service struct {
	users         UserRepository
	sessions      SessionRepository
	verifications VerificationRepository
	emailer       Emailer

	verifyURLBase   string        // e.g. "https://go-workout-api.fly.dev"
	sessionLifetime time.Duration // default 30 days
	verifyLifetime  time.Duration // default 24 hours
}

// Config gathers Service tunables.
type Config struct {
	VerifyURLBase   string        // e.g. "https://go-workout-api.fly.dev"
	SessionLifetime time.Duration // 0 → 30 days
	VerifyLifetime  time.Duration // 0 → 24 hours
}

// NewService constructs a Service.
func NewService(
	users UserRepository,
	sessions SessionRepository,
	verifications VerificationRepository,
	emailer Emailer,
	cfg Config,
) *Service {
	if cfg.SessionLifetime == 0 {
		cfg.SessionLifetime = 30 * 24 * time.Hour
	}
	if cfg.VerifyLifetime == 0 {
		cfg.VerifyLifetime = 24 * time.Hour
	}
	return &Service{
		users:           users,
		sessions:        sessions,
		verifications:   verifications,
		emailer:         emailer,
		verifyURLBase:   strings.TrimRight(cfg.VerifyURLBase, "/"),
		sessionLifetime: cfg.SessionLifetime,
		verifyLifetime:  cfg.VerifyLifetime,
	}
}

// Signup creates an unverified user, persists them, and sends a verification
// email. Returns the User (sanitized) on success.
func (s *Service) Signup(ctx context.Context, email, password string) (User, error) {
	email = strings.TrimSpace(email)
	if _, err := mail.ParseAddress(email); err != nil {
		return User{}, &ValidationError{Message: "invalid email address"}
	}
	hash, err := HashPassword(password)
	if err != nil {
		return User{}, &ValidationError{Message: err.Error()}
	}

	now := time.Now().UTC()
	u := User{
		ID:        newID(),
		Email:     email,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.users.Create(ctx, UserCredentials{User: u, PasswordHash: hash}); err != nil {
		return User{}, err // ErrEmailTaken bubbles up
	}

	// Issue a verification token and email it. Failures here are
	// non-fatal — the user can request another via /v1/auth/verify/resend.
	v := Verification{
		Token:     randomToken(),
		UserID:    u.ID,
		ExpiresAt: now.Add(s.verifyLifetime),
		CreatedAt: now,
	}
	v.TokenHash = hashToken(v.Token)
	if err := s.verifications.Insert(ctx, v); err != nil {
		return u, err
	}
	verifyURL := s.verifyURLBase + "/verify?token=" + v.Token
	if err := s.emailer.SendVerification(ctx, u.Email, verifyURL); err != nil {
		// User exists; verification token is stored; the email send failed.
		// Surface as a soft error so the caller can show a "resend"
		// affordance, but the signup itself succeeded.
		return u, err
	}
	return u, nil
}

// VerifyEmail consumes a verification token and marks the user as verified.
// Returns the user so the handler can immediately establish a session.
func (s *Service) VerifyEmail(ctx context.Context, token string) (User, error) {
	if token == "" {
		return User{}, ErrTokenInvalid
	}
	now := time.Now().UTC()
	v, err := s.verifications.Consume(ctx, hashToken(token), now)
	if err != nil {
		return User{}, err
	}
	if err := s.users.MarkVerified(ctx, v.UserID, now); err != nil {
		return User{}, err
	}
	c, err := s.users.GetByID(ctx, v.UserID)
	if err != nil {
		return User{}, err
	}
	return c.User, nil
}

// ResendVerification issues a fresh verification token for an unverified
// user. Idempotent — re-running for an already-verified email is a no-op.
func (s *Service) ResendVerification(ctx context.Context, email string) error {
	c, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		// Don't leak whether the email exists: return nil and silently
		// drop. The user-facing handler returns 204 either way.
		if errors.Is(err, ErrUserNotFound) {
			return nil
		}
		return err
	}
	if c.User.EmailVerifiedAt != nil {
		return nil
	}
	now := time.Now().UTC()
	v := Verification{
		Token:     randomToken(),
		UserID:    c.User.ID,
		ExpiresAt: now.Add(s.verifyLifetime),
		CreatedAt: now,
	}
	v.TokenHash = hashToken(v.Token)
	if err := s.verifications.Insert(ctx, v); err != nil {
		return err
	}
	return s.emailer.SendVerification(ctx, c.User.Email, s.verifyURLBase+"/verify?token="+v.Token)
}

// Login verifies the credentials and creates a session. Returns
// (Session, User) on success. The Session.Token is the plaintext value the
// caller should set as the session cookie.
func (s *Service) Login(ctx context.Context, email, password, userAgent, ip string) (Session, User, error) {
	email = strings.TrimSpace(email)
	c, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		// Constant-time-equivalent: run a dummy bcrypt against a known
		// hash so attackers can't distinguish "user exists" from "user
		// missing" via response time.
		VerifyPassword(dummyBcryptHash, password)
		return Session{}, User{}, ErrInvalidLogin
	}
	if err := VerifyPassword(c.PasswordHash, password); err != nil {
		return Session{}, User{}, ErrInvalidLogin
	}
	if c.User.EmailVerifiedAt == nil {
		return Session{}, User{}, ErrEmailNotVerified
	}

	now := time.Now().UTC()
	sess := Session{
		Token:     randomToken(),
		UserID:    c.User.ID,
		ExpiresAt: now.Add(s.sessionLifetime),
		CreatedAt: now,
		UserAgent: userAgent,
		IP:        ip,
	}
	sess.TokenHash = hashToken(sess.Token)
	if err := s.sessions.Insert(ctx, sess); err != nil {
		return Session{}, User{}, err
	}
	return sess, c.User, nil
}

// Logout deletes the session bound to token.
func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.sessions.Delete(ctx, hashToken(token))
}

// UpdateProfile updates the user's optional profile fields. nil values
// are no-ops; only provided fields get written.
func (s *Service) UpdateProfile(ctx context.Context, userID string, heightCM *int, weightKG *float64, birth *time.Time, sex *string) (User, error) {
	if err := s.users.UpdateProfile(ctx, userID, heightCM, weightKG, birth, sex, time.Now().UTC()); err != nil {
		return User{}, err
	}
	c, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return User{}, err
	}
	return c.User, nil
}

// SessionUser looks up the user behind a session token. The middleware uses
// this on every request to convert the cookie value into a user id.
func (s *Service) SessionUser(ctx context.Context, token string) (User, error) {
	if token == "" {
		return User{}, ErrSessionExpired
	}
	sess, err := s.sessions.GetByTokenHash(ctx, hashToken(token))
	if err != nil {
		return User{}, err
	}
	c, err := s.users.GetByID(ctx, sess.UserID)
	if err != nil {
		return User{}, err
	}
	return c.User, nil
}

// --- helpers -----------------------------------------------------------

// ValidationError mirrors domain.ValidationError shape so the transport
// layer's existing error mapping works without importing domain — auth
// is a leaf package, the rest of the codebase can absorb it without
// import cycles.
type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

// newID returns a 16-byte hex user id. Same convention as the existing
// workout.NewUUID.
func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("auth: rand.Read failed: " + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return hex.EncodeToString(b[0:4]) + "-" + hex.EncodeToString(b[4:6]) + "-" +
		hex.EncodeToString(b[6:8]) + "-" + hex.EncodeToString(b[8:10]) + "-" +
		hex.EncodeToString(b[10:16])
}

// dummyBcryptHash is a pre-computed bcrypt hash used to make password
// verification take the same time for "user not found" as it does for
// "wrong password" — a small defense against timing-attack user
// enumeration. Plaintext was "dummyDoesNotMatter".
const dummyBcryptHash = "$2a$12$8Vk5b7lJYE/lh8VFFiZJWeNvKWqVeXkP/qBLvCm0Z9V8U7T4r/Z8."
