package redis

import (
	"context"
	"fmt"

	"myGuy/internal/domain"

	goredis "github.com/redis/go-redis/v9"
)

// leaderboardKey is the sorted set holding every player's score.
const leaderboardKey = "leaderboard"

// LeaderboardRepository implements domain.LeaderboardRepository with a Redis
// sorted set. The set stays ordered as scores are written (ZADD is O(log n)),
// so reading the top N is a cheap walk rather than a fresh sort.
type LeaderboardRepository struct {
	client *goredis.Client
}

func NewLeaderboardRepository(client *goredis.Client) *LeaderboardRepository {
	return &LeaderboardRepository{client: client}
}

var _ domain.LeaderboardRepository = (*LeaderboardRepository)(nil)

func (r *LeaderboardRepository) SetPoints(ctx context.Context, username string, points int32) error {
	err := r.client.ZAdd(ctx, leaderboardKey, goredis.Z{
		Score:  float64(points),
		Member: username,
	}).Err()
	if err != nil {
		return fmt.Errorf("set leaderboard points: %w", err)
	}
	return nil
}

func (r *LeaderboardRepository) SetMany(ctx context.Context, points map[string]int32) error {
	if len(points) == 0 {
		return nil
	}
	members := make([]goredis.Z, 0, len(points))
	for username, score := range points {
		members = append(members, goredis.Z{Score: float64(score), Member: username})
	}
	if err := r.client.ZAdd(ctx, leaderboardKey, members...).Err(); err != nil {
		return fmt.Errorf("set leaderboard points in bulk: %w", err)
	}
	return nil
}

func (r *LeaderboardRepository) Top(ctx context.Context, n int32) ([]domain.LeaderboardEntry, error) {
	results, err := r.client.ZRevRangeWithScores(ctx, leaderboardKey, 0, int64(n-1)).Result()
	if err != nil {
		return nil, fmt.Errorf("read leaderboard: %w", err)
	}

	entries := make([]domain.LeaderboardEntry, 0, len(results))
	for i, z := range results {
		username, ok := z.Member.(string)
		if !ok {
			return nil, fmt.Errorf("leaderboard member %v is not a string", z.Member)
		}
		entries = append(entries, domain.LeaderboardEntry{
			Rank:     int32(i + 1),
			Username: username,
			Points:   int32(z.Score),
		})
	}
	return entries, nil
}
