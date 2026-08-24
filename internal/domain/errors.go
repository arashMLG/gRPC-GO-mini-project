package domain

import "errors"

// Domain errors are declared here rather than being invented ad hoc with
// fmt.Errorf at each call site. Because they are package-level sentinel
// values, callers can test for a specific failure with errors.Is instead of
// matching on error strings — which is what lets the transport layer map a
// business failure onto the right gRPC status code without knowing anything
// about Postgres or Redis.
var (
	// ErrUserExists is returned when registering a username that is taken.
	ErrUserExists = errors.New("username already taken")

	// ErrUserNotFound is returned when no user matches the given username.
	ErrUserNotFound = errors.New("user not found")

	// ErrInvalidCredentials is returned when a username/password pair does
	// not authenticate.
	ErrInvalidCredentials = errors.New("invalid username or password")

	// ErrNotLoggedIn is returned when a request carries a token that has no
	// live session behind it.
	ErrNotLoggedIn = errors.New("not logged in: unknown or missing token")

	// ErrEmptyCredentials is returned when username or password is blank.
	ErrEmptyCredentials = errors.New("username and password must both be provided")
)
