package service

import (
	"context"
	"fmt"
	"log"

	"myGuy/internal/domain"
)

// PlayResult is what a single turn produced.
type PlayResult struct {
	Delta   int32
	Total   int32
	Message string
}

// GameService owns the scoring turn: apply the word's score to the player's
// durable total, mirror the new total into the leaderboard index, and tell
// watchers the board moved.
type GameService struct {
	users       domain.UserRepository
	leaderboard domain.LeaderboardRepository
	notifier    domain.LeaderboardNotifier
}

func NewGameService(
	users domain.UserRepository,
	leaderboard domain.LeaderboardRepository,
	notifier domain.LeaderboardNotifier,
) *GameService {
	return &GameService{users: users, leaderboard: leaderboard, notifier: notifier}
}

// Play scores one word for an already-authenticated user. Note the username
// is a parameter rather than a token: authentication happened in the
// transport layer, so this service has no opinion about how callers prove
// who they are.
func (s *GameService) Play(ctx context.Context, username, word string) (PlayResult, error) {
	delta, message := domain.ScoreWord(word)

	// Postgres is the durable source of truth, so it is written first.
	total, err := s.users.AddPoints(ctx, username, delta)
	if err != nil {
		return PlayResult{}, fmt.Errorf("play: %w", err)
	}

	// The leaderboard is a cache derived from that truth. A failure here
	// leaves the cache briefly stale rather than failing the player's turn,
	// and the next successful write (or a restart's cache warm) repairs it.
	if err := s.leaderboard.SetPoints(ctx, username, total); err != nil {
		log.Printf("leaderboard: failed to update cache for %s: %v", username, err)
	}

	if domain.IsStatusWord(word) {
		message = fmt.Sprintf("Your value as Human: %d", total)
	}

	s.notifier.Notify()

	return PlayResult{Delta: delta, Total: total, Message: message}, nil
}
