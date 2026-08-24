// Package postgres contains the Postgres-backed adapters: the concrete
// implementations of the domain repository ports that know about SQL.
// Everything Postgres-specific — table names, SQL strings, driver error
// codes — is confined to this package.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"myGuy/internal/domain"

	"github.com/jackc/pgx/v5/pgconn"
)

// uniqueViolation is the Postgres SQLSTATE for a unique-constraint breach,
// which is how a duplicate username surfaces from the driver.
const uniqueViolation = "23505"

// UserRepository implements domain.UserRepository against Postgres.
type UserRepository struct {
	db *sql.DB
}

// NewUserRepository builds a Postgres-backed user repository. The *sql.DB is
// injected rather than created here so the composition root owns the
// connection's lifetime.
func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

// compile-time proof that this adapter satisfies the port. If the interface
// and the adapter ever drift apart, the build breaks here with a clear
// message instead of somewhere in cmd/server.
var _ domain.UserRepository = (*UserRepository)(nil)

func (r *UserRepository) Create(ctx context.Context, username, passwordHash string) error {
	_, err := r.db.ExecContext(ctx,
		"INSERT INTO users (username, password_hash) VALUES ($1, $2)",
		username, passwordHash)
	if err != nil {
		// Translate the driver's error into a domain error so callers never
		// have to know what SQLSTATE 23505 means.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return domain.ErrUserExists
		}
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

func (r *UserRepository) FindByUsername(ctx context.Context, username string) (*domain.User, error) {
	user := domain.User{Username: username}
	err := r.db.QueryRowContext(ctx,
		"SELECT password_hash, points FROM users WHERE username = $1",
		username).Scan(&user.PasswordHash, &user.Points)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find user: %w", err)
	}
	return &user, nil
}

func (r *UserRepository) AddPoints(ctx context.Context, username string, delta int32) (int32, error) {
	var total int32
	err := r.db.QueryRowContext(ctx,
		"UPDATE users SET points = points + $1 WHERE username = $2 RETURNING points",
		delta, username).Scan(&total)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, domain.ErrUserNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("add points: %w", err)
	}
	return total, nil
}

func (r *UserRepository) ListAll(ctx context.Context) ([]domain.User, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT username, points FROM users")
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []domain.User
	for rows.Next() {
		var u domain.User
		if err := rows.Scan(&u.Username, &u.Points); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate users: %w", err)
	}
	return users, nil
}
