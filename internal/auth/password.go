package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"golang.org/x/crypto/bcrypt"
)

// bcryptCost is high enough to take ~100ms on modern hardware — the standard
// modern minimum. Raising it on faster servers is fine; lowering it would be
// a regression.
const bcryptCost = 12

// HashPassword returns the bcrypt hash of password. The hash includes salt,
// cost, and version — the cost can be raised in the future without breaking
// previously-stored hashes.
func HashPassword(password string) (string, error) {
	if len(password) < 8 {
		return "", errors.New("password must be at least 8 characters")
	}
	if len(password) > 72 {
		// bcrypt has a 72-byte input limit. Reject loudly instead of
		// silently truncating, which would make long passwords effectively
		// equal to their first 72 bytes.
		return "", errors.New("password must be at most 72 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// VerifyPassword returns nil if password matches hash, or a non-nil error
// otherwise. Constant-time semantics are provided by bcrypt internally.
func VerifyPassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

// randomToken returns a hex-encoded 32-byte random token. Used for session
// cookies and verification links. With 256 bits of entropy, brute-force
// guessing is computationally infeasible.
func randomToken() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand cannot meaningfully fail on a working OS; if it does,
		// the process can't make safe authentication decisions and panic is
		// the right signal.
		panic("auth: rand.Read failed: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}

// hashToken returns the hex-encoded SHA-256 of token. The repository stores
// this digest, not the plaintext token, so a database dump alone cannot be
// used to impersonate users.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
