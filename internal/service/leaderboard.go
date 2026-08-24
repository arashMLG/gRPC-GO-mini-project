package service

import (
	"context"
	"fmt"

	"myGuy/internal/domain"
)

const (
	defaultTopN = 10
	maxTopN     = 100
)

// LeaderboardService owns reading the ranked board, subscribing to changes,
// and rebuilding the cache from durable storage.
type LeaderboardService struct {
	users       domain.UserRepository
	leaderboard domain.LeaderboardRepository
	notifier    domain.LeaderboardNotifier
}

func NewLeaderboardService(
	users domain.UserRepository,
	leaderboard domain.LeaderboardRepository,
	notifier domain.LeaderboardNotifier,
) *LeaderboardService {
	return &LeaderboardService{users: users, leaderboard: leaderboard, notifier: notifier}
}

// Top returns the highest-scoring players, clamping the requested size to a
// sane range. The clamp lives here rather than in the gRPC handler because
// it is a rule about the game, not about the wire protocol.
func (s *LeaderboardService) Top(ctx context.Context, n int32) ([]domain.LeaderboardEntry, error) {
	if n <= 0 || n > maxTopN {
		n = defaultTopN
	}
	return s.leaderboard.Top(ctx, n)
}

// Subscribe returns a channel signalled whenever the board changes, plus the
// function that unsubscribes it.
func (s *LeaderboardService) Subscribe() (<-chan struct{}, func()) {
	return s.notifier.Subscribe()
}

// WarmCache rebuilds the leaderboard index from durable storage. It runs at
// startup so a freshly started or emptied cache is correct immediately
// instead of filling in only as players happen to take turns.
func (s *LeaderboardService) WarmCache(ctx context.Context) error {
	users, err := s.users.ListAll(ctx)
	if err != nil {
		return fmt.Errorf("warm leaderboard cache: %w", err)
	}
	if len(users) == 0 {
		return nil
	}

	points := make(map[string]int32, len(users))
	for _, u := range users {
		points[u.Username] = u.Points
	}
	if err := s.leaderboard.SetMany(ctx, points); err != nil {
		return fmt.Errorf("warm leaderboard cache: %w", err)
	}
	return nil
}
