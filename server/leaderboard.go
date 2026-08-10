package main

import (
	"log"
	"myGuy/pb"
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
	rows, err := s.db.QueryContext(stream.Context(),
		`SELECT username, points FROM users
				ORDER BY points DESC , username ASC
				LIMIT $1`, topN)
	if err != nil {
		return err
	}
	defer rows.Close()
	var entries []*pb.LeaderboardEntry
	rank := int32(1)
	for rows.Next() {
		var username string
		var points int32
		if err := rows.Scan(&username, &points); err != nil {
			return err
		}
		entries = append(entries, &pb.LeaderboardEntry{
			Rank:     rank,
			Username: username,
			Points:   points,
		})
		rank++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return stream.Send(&pb.LeaderboardReply{Entries: entries})
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
