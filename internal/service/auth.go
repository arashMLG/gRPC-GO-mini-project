// Package service holds the application's business logic. Every type here
// depends only on the interfaces declared in internal/domain — never on
// *sql.DB, a Redis client, or a protobuf type. That is what makes these
// services testable with fakes and independent of the infrastructure they
// happen to run against today.
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"myGuy/internal/domain"
)

// AuthService owns registration, login, and token verification.
type AuthService struct {
	users      domain.UserRepository
	sessions   domain.SessionRepository
	hasher     domain.PasswordHasher
	tokens     domain.TokenGenerator
	sessionTTL time.Duration
}

// NewAuthService is the constructor-injection point: every collaborator is
// passed in as an interface, so the caller (the composition root in
// cmd/server) decides which concrete implementations get used.
func NewAuthService(
	users domain.UserRepository,
	sessions domain.SessionRepository,
	hasher domain.PasswordHasher,
	tokens domain.TokenGenerator,
	sessionTTL time.Duration,
) *AuthService {
	return &AuthService{
		users:      users,
		sessions:   sessions,
		hasher:     hasher,
		tokens:     tokens,
		sessionTTL: sessionTTL,
	}
}

// Register creates a new account.
func (s *AuthService) Register(ctx context.Context, username, password string) error {
	if username == "" || password == "" {
		return domain.ErrEmptyCredentials
	}

	hash, err := s.hasher.Hash(password)
	if err != nil {
		return fmt.Errorf("register: %w", err)
	}

	// The repository already maps a duplicate-key violation to
	// domain.ErrUserExists, so this passes straight through.
	return s.users.Create(ctx, username, hash)
}

// Login verifies credentials and, on success, opens a session and returns
// its token.
func (s *AuthService) Login(ctx context.Context, username, password string) (string, error) {
	user, err := s.users.FindByUsername(ctx, username)
	if errors.Is(err, domain.ErrUserNotFound) {
		return "", domain.ErrInvalidCredentials
	}
	if err != nil {
		return "", fmt.Errorf("login: %w", err)
	}

	if err := s.hasher.Compare(user.PasswordHash, password); err != nil {
		return "", domain.ErrInvalidCredentials
	}

	token, err := s.tokens.NewToken()
	if err != nil {
		return "", fmt.Errorf("login: %w", err)
	}

	if err := s.sessions.Save(ctx, token, username, s.sessionTTL); err != nil {
		return "", fmt.Errorf("login: %w", err)
	}
	return token, nil
}

// Authenticate resolves a session token to the username behind it, returning
// domain.ErrNotLoggedIn when the token is unknown or expired. Every
// authenticated RPC funnels through this one method.
func (s *AuthService) Authenticate(ctx context.Context, token string) (string, error) {
	return s.sessions.Username(ctx, token)
}

// Logout ends a session early.
func (s *AuthService) Logout(ctx context.Context, token string) error {
	return s.sessions.Delete(ctx, token)
}
