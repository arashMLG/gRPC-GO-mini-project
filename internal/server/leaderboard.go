package server

import (
	"context"
	"database/sql"
	"log"
	"myGuy/internal/pb"

	"github.com/redis/go-redis/v9"
)

func (s *server) Leaderboard(req *pb.LeaderboardRequest, stream pb.Game_LeaderboardServer) error {
	topN := req.GetTopN()
	if topN <= 0 || topN > 100 {
		topN = 10
	}
	updates := make(chan struct{}, 1)
	s.mu.Lock()
	s.boardWatchers[updates] = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.boardWatchers, updates)
		close(updates)
		s.mu.Unlock()
	}()

	if err := s.sendBoard(stream, topN); err != nil {
		return err
	}

	for {
		select {
		case <-stream.Context().Done():
			return nil
		case <-updates:
			if err := s.sendBoard(stream, topN); err != nil {
				return err
			}
		}
	}
}

func (s *server) sendBoard(stream pb.Game_LeaderboardServer, topN int32) error {
	results, err := s.redis.ZRevRangeWithScores(stream.Context(), leaderboardKey, 0, int64(topN-1)).Result()
	if err != nil {
		return err
	}

	entries := make([]*pb.LeaderboardEntry, 0, len(results))
	for i, z := range results {
		entries = append(entries, &pb.LeaderboardEntry{
			Rank:     int32(i + 1),
			Username: z.Member.(string),
			Points:   int32(z.Score),
		})
	}
	return stream.Send(&pb.LeaderboardReply{Entries: entries})
}

// warmLeaderboardCache loads every user's points from Postgres into the
// Redis sorted set at startup, so the leaderboard is correct immediately
// even against a freshly started (empty) Redis instance. Postgres stays the
// durable source of truth; Redis is a fast index kept in sync on writes
// (see Play in game.go) and rebuilt here on boot.
func warmLeaderboardCache(ctx context.Context, db *sql.DB, rdb *redis.Client) error {
	rows, err := db.QueryContext(ctx, "SELECT username, points FROM users")
	if err != nil {
		return err
	}
	defer rows.Close()

	var members []redis.Z
	for rows.Next() {
		var username string
		var points int32
		if err := rows.Scan(&username, &points); err != nil {
			return err
		}
		members = append(members, redis.Z{Score: float64(points), Member: username})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(members) == 0 {
		return nil
	}
	return rdb.ZAdd(ctx, leaderboardKey, members...).Err()
}

func (s *server) notifyBoard() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for ch := range s.boardWatchers {
		select {
		case ch <- struct{}{}:
			// Ding
		default:
			// No ding :(
		}
	}
	log.Printf("leaderboard: Notified %d watchers.\n", len(s.boardWatchers))
}
