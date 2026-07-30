package main

import (
	"context"
	"fmt"
	"myGuy/pb"
	"strings"
)

func (s *server) Play(ctx context.Context, req *pb.PlayRequest) (*pb.PlayReply, error) {
	s.mu.Lock()
	username, ok := s.sessions[req.Token]
	s.mu.Unlock()

	if !ok {
		return nil, fmt.Errorf("not logged in: unknown or missing token")
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
	err := s.db.QueryRowContext(ctx,
		"UPDATE users SET points = points + $1 WHERE username = $2 RETURNING points",
		delta, username).Scan(&total)
	if err != nil {
		return nil, fmt.Errorf("Error in upgrading points in SQL : %w", err)
	}

	if strings.ToLower(strings.TrimSpace(req.Word)) == "status" {
		message = fmt.Sprintf("Your value as Human: %d", int(total))
	}

	return &pb.PlayReply{
		PointsChange: delta,
		TotalPoints:  total,
		Message:      message,
	}, nil
}
