package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"myGuy/internal/domain"
)

func newAuthServiceForTest() (*AuthService, *fakeUserRepo, *fakeSessionRepo) {
	users := newFakeUserRepo()
	sessions := newFakeSessionRepo()
	svc := NewAuthService(users, sessions, plainHasher{}, &staticTokens{prefix: "token-"}, time.Hour)
	return svc, users, sessions
}

func TestRegisterRejectsEmptyCredentials(t *testing.T) {
	svc, _, _ := newAuthServiceForTest()

	for _, tc := range []struct{ name, username, password string }{
		{"no username", "", "hunter2"},
		{"no password", "arash", ""},
		{"neither", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := svc.Register(context.Background(), tc.username, tc.password)
			if !errors.Is(err, domain.ErrEmptyCredentials) {
				t.Fatalf("got %v, want ErrEmptyCredentials", err)
			}
		})
	}
}

func TestRegisterRejectsDuplicateUsername(t *testing.T) {
	svc, _, _ := newAuthServiceForTest()
	ctx := context.Background()

	if err := svc.Register(ctx, "arash", "hunter2"); err != nil {
		t.Fatalf("first register failed: %v", err)
	}

	err := svc.Register(ctx, "arash", "different")
	if !errors.Is(err, domain.ErrUserExists) {
		t.Fatalf("got %v, want ErrUserExists", err)
	}
}

func TestLoginReturnsTokenThatAuthenticates(t *testing.T) {
	svc, _, _ := newAuthServiceForTest()
	ctx := context.Background()

	if err := svc.Register(ctx, "arash", "hunter2"); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	token, err := svc.Login(ctx, "arash", "hunter2")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	if token == "" {
		t.Fatal("login returned an empty token")
	}

	username, err := svc.Authenticate(ctx, token)
	if err != nil {
		t.Fatalf("authenticate failed: %v", err)
	}
	if username != "arash" {
		t.Fatalf("got username %q, want %q", username, "arash")
	}
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	svc, _, _ := newAuthServiceForTest()
	ctx := context.Background()

	if err := svc.Register(ctx, "arash", "hunter2"); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	_, err := svc.Login(ctx, "arash", "wrong")
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("got %v, want ErrInvalidCredentials", err)
	}
}

// A login attempt for a user that does not exist must fail exactly the same
// way as a wrong password, so the error cannot be used to discover which
// usernames are registered.
func TestLoginOnUnknownUserLooksLikeWrongPassword(t *testing.T) {
	svc, _, _ := newAuthServiceForTest()

	_, err := svc.Login(context.Background(), "nobody", "hunter2")
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("got %v, want ErrInvalidCredentials", err)
	}
}

func TestAuthenticateRejectsUnknownToken(t *testing.T) {
	svc, _, _ := newAuthServiceForTest()

	_, err := svc.Authenticate(context.Background(), "not-a-real-token")
	if !errors.Is(err, domain.ErrNotLoggedIn) {
		t.Fatalf("got %v, want ErrNotLoggedIn", err)
	}
}

func TestLogoutInvalidatesTheToken(t *testing.T) {
	svc, _, _ := newAuthServiceForTest()
	ctx := context.Background()

	if err := svc.Register(ctx, "arash", "hunter2"); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	token, err := svc.Login(ctx, "arash", "hunter2")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}

	if err := svc.Logout(ctx, token); err != nil {
		t.Fatalf("logout failed: %v", err)
	}

	if _, err := svc.Authenticate(ctx, token); !errors.Is(err, domain.ErrNotLoggedIn) {
		t.Fatalf("got %v, want ErrNotLoggedIn after logout", err)
	}
}
