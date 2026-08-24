package domain

import "context"

// User is the core account entity. It is a plain struct with no database
// tags and no protobuf types: the domain layer must not know how a user is
// stored or how it travels over the wire.
type User struct {
	Username     string
	PasswordHash string
	Points       int32
}

// UserRepository is the port for durable user storage. The Postgres adapter
// in internal/adapter/postgres implements it; services depend on this
// interface, never on *sql.DB.
type UserRepository interface {
	// Create stores a new user. It returns ErrUserExists if the username is
	// already taken.
	Create(ctx context.Context, username, passwordHash string) error

	// FindByUsername returns the user, or ErrUserNotFound if there is none.
	FindByUsername(ctx context.Context, username string) (*User, error)

	// AddPoints atomically adds delta to the user's score and returns the new
	// total. It returns ErrUserNotFound if the user does not exist.
	AddPoints(ctx context.Context, username string, delta int32) (int32, error)

	// ListAll returns every user, used to rebuild the leaderboard cache.
	ListAll(ctx context.Context) ([]User, error)
}
