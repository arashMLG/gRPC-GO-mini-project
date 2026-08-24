package domain

import (
	"context"
	"time"
)

// SessionRepository is the port for login-session storage. The Redis adapter
// implements it, but nothing in the service layer knows that: swapping in a
// Postgres table or an in-memory map is a wiring change in cmd/server, not a
// change to any business logic.
type SessionRepository interface {
	// Save stores token -> username for the given lifetime.
	Save(ctx context.Context, token, username string, ttl time.Duration) error

	// Username resolves a token to its owner, returning ErrNotLoggedIn when
	// the token is unknown or has expired.
	Username(ctx context.Context, token string) (string, error)

	// Delete removes a session, ending it early (logout).
	Delete(ctx context.Context, token string) error
}
