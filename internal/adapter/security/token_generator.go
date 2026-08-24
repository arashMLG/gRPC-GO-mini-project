package security

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"myGuy/internal/domain"
)

// RandomTokenGenerator implements domain.TokenGenerator using
// crypto/rand, so tokens are unguessable.
type RandomTokenGenerator struct {
	bytes int
}

// NewRandomTokenGenerator builds a generator producing tokens of the given
// entropy in bytes (the hex-encoded string is twice that many characters).
func NewRandomTokenGenerator(numBytes int) *RandomTokenGenerator {
	return &RandomTokenGenerator{bytes: numBytes}
}

var _ domain.TokenGenerator = (*RandomTokenGenerator)(nil)

func (g *RandomTokenGenerator) NewToken() (string, error) {
	b := make([]byte, g.bytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return hex.EncodeToString(b), nil
}
