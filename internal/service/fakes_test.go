package service

import (
	"context"
	"sync"
	"time"

	"myGuy/internal/domain"
)

// These fakes are the entire point of the refactor: because the services
// depend on domain interfaces instead of *sql.DB and *redis.Client, the
// tests in this package run with no Postgres and no Redis anywhere.

// fakeUserRepo is an in-memory domain.UserRepository.
type fakeUserRepo struct {
	mu    sync.Mutex
	users map[string]*domain.User
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{users: make(map[string]*domain.User)}
}

var _ domain.UserRepository = (*fakeUserRepo)(nil)

func (f *fakeUserRepo) Create(_ context.Context, username, passwordHash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, exists := f.users[username]; exists {
		return domain.ErrUserExists
	}
	f.users[username] = &domain.User{Username: username, PasswordHash: passwordHash}
	return nil
}

func (f *fakeUserRepo) FindByUsername(_ context.Context, username string) (*domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	user, ok := f.users[username]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	copied := *user
	return &copied, nil
}

func (f *fakeUserRepo) AddPoints(_ context.Context, username string, delta int32) (int32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	user, ok := f.users[username]
	if !ok {
		return 0, domain.ErrUserNotFound
	}
	user.Points += delta
	return user.Points, nil
}

func (f *fakeUserRepo) ListAll(_ context.Context) ([]domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	all := make([]domain.User, 0, len(f.users))
	for _, u := range f.users {
		all = append(all, *u)
	}
	return all, nil
}

// fakeSessionRepo is an in-memory domain.SessionRepository. It ignores TTL,
// which is fine because nothing under test depends on expiry.
type fakeSessionRepo struct {
	mu       sync.Mutex
	sessions map[string]string
}

func newFakeSessionRepo() *fakeSessionRepo {
	return &fakeSessionRepo{sessions: make(map[string]string)}
}

var _ domain.SessionRepository = (*fakeSessionRepo)(nil)

func (f *fakeSessionRepo) Save(_ context.Context, token, username string, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sessions[token] = username
	return nil
}

func (f *fakeSessionRepo) Username(_ context.Context, token string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	username, ok := f.sessions[token]
	if !ok {
		return "", domain.ErrNotLoggedIn
	}
	return username, nil
}

func (f *fakeSessionRepo) Delete(_ context.Context, token string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.sessions, token)
	return nil
}

// fakeLeaderboardRepo records what the service wrote, so tests can assert on
// cache updates without inspecting Redis.
type fakeLeaderboardRepo struct {
	mu     sync.Mutex
	points map[string]int32
}

func newFakeLeaderboardRepo() *fakeLeaderboardRepo {
	return &fakeLeaderboardRepo{points: make(map[string]int32)}
}

var _ domain.LeaderboardRepository = (*fakeLeaderboardRepo)(nil)

func (f *fakeLeaderboardRepo) SetPoints(_ context.Context, username string, points int32) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.points[username] = points
	return nil
}

func (f *fakeLeaderboardRepo) SetMany(_ context.Context, points map[string]int32) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for username, score := range points {
		f.points[username] = score
	}
	return nil
}

func (f *fakeLeaderboardRepo) Top(_ context.Context, n int32) ([]domain.LeaderboardEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	// Simple selection sort by score; the fake only ever holds a handful of
	// entries, so clarity beats efficiency here.
	remaining := make(map[string]int32, len(f.points))
	for k, v := range f.points {
		remaining[k] = v
	}
	var entries []domain.LeaderboardEntry
	for rank := int32(1); rank <= n && len(remaining) > 0; rank++ {
		bestName, bestScore := "", int32(0)
		first := true
		for name, score := range remaining {
			if first || score > bestScore || (score == bestScore && name < bestName) {
				bestName, bestScore, first = name, score, false
			}
		}
		entries = append(entries, domain.LeaderboardEntry{Rank: rank, Username: bestName, Points: bestScore})
		delete(remaining, bestName)
	}
	return entries, nil
}

// countingNotifier records how many times the board was signalled.
type countingNotifier struct {
	mu      sync.Mutex
	notifes int
}

var _ domain.LeaderboardNotifier = (*countingNotifier)(nil)

func (n *countingNotifier) Subscribe() (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	return ch, func() { close(ch) }
}

func (n *countingNotifier) Notify() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.notifes++
}

func (n *countingNotifier) count() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.notifes
}

// plainHasher is a stand-in for bcrypt. Real bcrypt is deliberately slow;
// in tests that cost buys nothing, so injecting this keeps the suite fast.
type plainHasher struct{}

var _ domain.PasswordHasher = plainHasher{}

func (plainHasher) Hash(password string) (string, error) { return "hashed:" + password, nil }

func (plainHasher) Compare(hash, password string) error {
	if hash != "hashed:"+password {
		return domain.ErrInvalidCredentials
	}
	return nil
}

// staticTokens hands out predictable tokens so assertions can name them.
type staticTokens struct {
	mu     sync.Mutex
	next   int
	prefix string
}

var _ domain.TokenGenerator = (*staticTokens)(nil)

func (s *staticTokens) NewToken() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next++
	return s.prefix + string(rune('0'+s.next)), nil
}
