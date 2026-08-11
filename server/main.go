package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"myGuy/pb"
	"net"
	"os"
	"sync"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
)

const sessionTTL = 24 * time.Hour
const leaderboardKey = "leaderboard"

func sessionKey(token string) string {
	return fmt.Sprintf("session:%s", token)
}

type server struct {
	pb.UnimplementedGreeterServer
	pb.UnimplementedGameServer

	db            *sql.DB
	redis         *redis.Client
	mu            sync.Mutex
	clients       map[chan *pb.ChatMessage]bool
	boardWatchers map[chan struct{}]bool
}

func (s *server) SayHelloWorld(ctx context.Context, req *pb.HelloWorldRequest) (*pb.HelloWorldReplay, error) {
	name := req.GetName()
	if name == "" {
		name = "Cat got your tongue?"
	}
	log.Printf("SayHelloWorld request recieved from %q", &name)
	return &pb.HelloWorldReplay{
		Message: fmt.Sprintf("Hello World! Hello, %s!!!", name),
	}, nil
}

func (s *server) lookupSession(ctx context.Context, token string) (string, error) {
	username, err := s.redis.Get(ctx, sessionKey(token)).Result()
	if err == redis.Nil {
		return "", fmt.Errorf("not logged in: unkownn or missing token")
	}
	if err != nil {
		return "", fmt.Errorf("session lookup failed: %w", err)
	}
	return username, nil
}

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://localhost:5432/database?sslmode=disable"
	}

	db, err := sql.Open("pgx", dsn)

	if err != nil {
		log.Fatalf("Error Configuring database : %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("Error Opening database : %v")
	}

	log.Println("Connected to PostgreSQL")

	redeisAddr := os.Getenv("REDIS_ADDR")
	if redeisAddr == "" {
		redeisAddr = "localhost:6379"
	}
	rdb := redis.NewClient(&redis.Options{Addr: redeisAddr})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("Error connecting to Redis : %v", err)
	}
	log.Println("Connected to Redis")

	if err := warmLeaderboardCache(context.Background(), db, rdb); err != nil {
		log.Fatalf("Error warming leaderboard cache : %v", err)
	}

	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Error in Listening : %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterGameServer(s, &server{
		db:            db,
		redis:         rdb,
		clients:       make(map[chan *pb.ChatMessage]bool),
		boardWatchers: make(map[chan struct{}]bool),
	})

	log.Printf("gRPC server listening on %s", lis.Addr())

	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
