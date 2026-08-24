// Package grpc is the inbound adapter: it turns gRPC calls into service
// calls. It is the only package that imports both the generated protobuf
// types and the service layer, which keeps protobuf out of the business
// logic entirely.
package grpc

import (
	"context"
	"log"

	"myGuy/internal/pb"
	"myGuy/internal/service"
)

// GameHandler implements pb.GameServer by delegating to the services. It
// holds no state of its own beyond its injected collaborators — all the
// state lives behind the repositories.
type GameHandler struct {
	pb.UnimplementedGameServer

	auth  *service.AuthService
	game  *service.GameService
	chat  *service.ChatService
	board *service.LeaderboardService
}

// NewGameHandler injects the services this handler delegates to.
func NewGameHandler(
	auth *service.AuthService,
	game *service.GameService,
	chat *service.ChatService,
	board *service.LeaderboardService,
) *GameHandler {
	return &GameHandler{auth: auth, game: game, chat: chat, board: board}
}

var _ pb.GameServer = (*GameHandler)(nil)

func (h *GameHandler) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterReply, error) {
	if err := h.auth.Register(ctx, req.GetUsername(), req.GetPassword()); err != nil {
		return nil, toStatusError(err)
	}
	return &pb.RegisterReply{Message: "Account Created for " + req.GetUsername()}, nil
}

func (h *GameHandler) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginReply, error) {
	token, err := h.auth.Login(ctx, req.GetUsername(), req.GetPassword())
	if err != nil {
		return nil, toStatusError(err)
	}
	return &pb.LoginReply{Token: token, Message: "Welcome to the GAME hahaha"}, nil
}

func (h *GameHandler) Play(ctx context.Context, req *pb.PlayRequest) (*pb.PlayReply, error) {
	username, err := h.auth.Authenticate(ctx, req.GetToken())
	if err != nil {
		return nil, toStatusError(err)
	}

	result, err := h.game.Play(ctx, username, req.GetWord())
	if err != nil {
		return nil, toStatusError(err)
	}

	return &pb.PlayReply{
		PointsChange: result.Delta,
		TotalPoints:  result.Total,
		Message:      result.Message,
	}, nil
}

func (h *GameHandler) Chat(stream pb.Game_ChatServer) error {
	messages, unsubscribe := h.chat.Subscribe()
	defer unsubscribe()

	// One goroutine drains the subscription onto the wire. It ends on its own
	// when unsubscribe closes the channel.
	go func() {
		for msg := range messages {
			if err := stream.Send(&pb.ChatMessage{Username: msg.Username, Text: msg.Text}); err != nil {
				return
			}
		}
	}()

	// The token is resolved once for the stream rather than on every message,
	// since a session lookup is now a network round trip to Redis.
	var username string
	for {
		in, err := stream.Recv()
		if err != nil {
			return nil
		}
		if username == "" {
			resolved, err := h.auth.Authenticate(stream.Context(), in.GetToken())
			if err != nil {
				continue
			}
			username = resolved
		}
		log.Printf("%s said: %s", username, in.GetText())
		h.chat.Send(username, in.GetText())
	}
}

func (h *GameHandler) Leaderboard(req *pb.LeaderboardRequest, stream pb.Game_LeaderboardServer) error {
	updates, unsubscribe := h.board.Subscribe()
	defer unsubscribe()

	// Send the current board immediately so a new watcher does not stare at
	// nothing until somebody happens to score.
	if err := h.sendBoard(stream, req.GetTopN()); err != nil {
		return err
	}

	for {
		select {
		case <-stream.Context().Done():
			return nil
		case _, ok := <-updates:
			if !ok {
				return nil
			}
			if err := h.sendBoard(stream, req.GetTopN()); err != nil {
				return err
			}
		}
	}
}

func (h *GameHandler) sendBoard(stream pb.Game_LeaderboardServer, topN int32) error {
	entries, err := h.board.Top(stream.Context(), topN)
	if err != nil {
		return toStatusError(err)
	}

	reply := &pb.LeaderboardReply{Entries: make([]*pb.LeaderboardEntry, 0, len(entries))}
	for _, e := range entries {
		reply.Entries = append(reply.Entries, &pb.LeaderboardEntry{
			Rank:     e.Rank,
			Username: e.Username,
			Points:   e.Points,
		})
	}
	return stream.Send(reply)
}
