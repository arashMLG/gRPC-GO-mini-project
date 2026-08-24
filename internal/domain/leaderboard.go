package domain

import "context"

// LeaderboardEntry is one row of the ranked scoreboard.
type LeaderboardEntry struct {
	Rank     int32
	Username string
	Points   int32
}

// LeaderboardRepository is the port for the ranked-score index. The Redis
// adapter backs it with a sorted set, but that is an implementation detail:
// a Postgres "ORDER BY points DESC" adapter would satisfy this same
// interface.
type LeaderboardRepository interface {
	// SetPoints records a single user's current score.
	SetPoints(ctx context.Context, username string, points int32) error

	// SetMany records many users' scores at once, used to warm the cache.
	SetMany(ctx context.Context, points map[string]int32) error

	// Top returns the highest-scoring n users, already ranked.
	Top(ctx context.Context, n int32) ([]LeaderboardEntry, error)
}

// LeaderboardNotifier is the port for fanning "the board changed" signals
// out to every client currently streaming the leaderboard. The in-memory
// adapter only reaches subscribers inside this process; a Redis Pub/Sub
// adapter implementing the same interface would reach subscribers on other
// server replicas.
type LeaderboardNotifier interface {
	// Subscribe returns a channel that receives a signal on each change,
	// plus a function that unsubscribes and closes the channel.
	Subscribe() (<-chan struct{}, func())

	// Notify signals every current subscriber.
	Notify()
}
