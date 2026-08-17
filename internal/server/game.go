package server

import (
	"context"
	"fmt"
	"log"
	"myGuy/internal/pb"
	"strings"

	"github.com/redis/go-redis/v9"
)

func (s *server) Play(ctx context.Context, req *pb.PlayRequest) (*pb.PlayReply, error) {
	username, err := s.lookupSession(ctx, req.Token)
	if err != nil {
		return nil, err
	}

	var delta int32
	var message string
	switch strings.ToLower(strings.TrimSpace(req.Word)) {
	case "cat":
		delta = 1
		message = "Good human"
	case "dog":
		delta = -1
		message = "Bad human"
	case "":
		delta = 0
		message = "Silent human"
	case "cog":
		delta = 0
		message = "what?"
	case "dat":
		delta = 0
		message = "what?"
	default:
		delta = 0
		message = "Unintelligible human"
	}
	var total int32
	err = s.db.QueryRowContext(ctx,
		"UPDATE users SET points = points + $1 WHERE username = $2 RETURNING points",
		delta, username).Scan(&total)
	if err != nil {
		return nil, fmt.Errorf("Error in upgrading points in SQL : %w", err)
	}

	if err := s.redis.ZAdd(ctx, leaderboardKey, redis.Z{Score: float64(total), Member: username}).Err(); err != nil {
		log.Printf("leaderboard: failed to update redis cache for %s: %v", username, err)
	}

	if strings.ToLower(strings.TrimSpace(req.Word)) == "status" {
		message = fmt.Sprintf("Your value as Human: %d", int(total))
	}

	s.notifyBoard()

	return &pb.PlayReply{
		PointsChange: delta,
		TotalPoints:  total,
		Message:      message,
	}, nil
}
