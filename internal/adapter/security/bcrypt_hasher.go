// Package security contains adapters for cryptographic concerns: password
// hashing and session-token generation.
package security

import (
	"fmt"

	"myGuy/internal/domain"

	"golang.org/x/crypto/bcrypt"
)

// BcryptHasher implements domain.PasswordHasher using bcrypt.
type BcryptHasher struct {
	cost int
}

// NewBcryptHasher builds a hasher at the given cost. Passing the cost in
// rather than hardcoding it means tests can drop it to the minimum, where
// bcrypt is fast enough not to dominate the test run.
func NewBcryptHasher(cost int) *BcryptHasher {
	return &BcryptHasher{cost: cost}
}

var _ domain.PasswordHasher = (*BcryptHasher)(nil)

func (h *BcryptHasher) Hash(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), h.cost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

func (h *BcryptHasher) Compare(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}
