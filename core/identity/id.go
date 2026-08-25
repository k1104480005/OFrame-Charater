package identity

import (
	"crypto/rand"
	"fmt"
)

// mustID returns a random UUIDv4 string used for package, material, and anchor
// identifiers. crypto/rand is effectively non-failing on supported platforms;
// a failure here is a hard invariant violation, so we panic rather than
// propagate a partial identifier.
func mustID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("identity: crypto/rand failed: %v", err))
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
