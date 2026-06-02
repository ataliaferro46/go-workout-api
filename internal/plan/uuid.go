package plan

import (
	"crypto/rand"
	"encoding/hex"
)

// NewUUID returns a random RFC 4122 version 4 UUID string. The implementation
// mirrors workout.NewUUID — kept package-local so the plan package has no
// dependency on the workout package.
func NewUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("plan: failed to read random bytes: " + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return hex.EncodeToString(b[0:4]) + "-" +
		hex.EncodeToString(b[4:6]) + "-" +
		hex.EncodeToString(b[6:8]) + "-" +
		hex.EncodeToString(b[8:10]) + "-" +
		hex.EncodeToString(b[10:16])
}
