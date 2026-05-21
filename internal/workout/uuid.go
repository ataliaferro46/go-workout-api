package workout

import (
	"crypto/rand"
	"encoding/hex"
)

// NewUUID returns a random RFC 4122 version 4 UUID string. It uses crypto/rand
// so identifiers are not predictable. A failure to read from the system CSPRNG
// is treated as fatal, because we cannot safely mint identifiers without it.
func NewUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("workout: failed to read random bytes: " + err.Error())
	}
	// Set the version (4) and variant (RFC 4122) bits.
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return hex.EncodeToString(b[0:4]) + "-" +
		hex.EncodeToString(b[4:6]) + "-" +
		hex.EncodeToString(b[6:8]) + "-" +
		hex.EncodeToString(b[8:10]) + "-" +
		hex.EncodeToString(b[10:16])
}
