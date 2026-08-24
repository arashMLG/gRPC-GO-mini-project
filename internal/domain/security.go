package domain

// PasswordHasher is the port for password hashing. Keeping it behind an
// interface means the auth service never imports bcrypt, so unit tests can
// inject a trivial fake hasher and run instantly instead of paying bcrypt's
// deliberate (and, in tests, useless) cost on every call.
type PasswordHasher interface {
	Hash(password string) (string, error)
	Compare(hash, password string) error
}

// TokenGenerator is the port for creating opaque session tokens. Injecting
// it lets tests supply predictable tokens instead of random ones.
type TokenGenerator interface {
	NewToken() (string, error)
}
