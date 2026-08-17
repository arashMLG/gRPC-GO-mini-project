package server

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"myGuy/internal/pb"
	"net"
	"os"
	"sync"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
)

// sessionTTL controls how long a login token stays valid in Redis.
const sessionTTL = 24 * time.Hour

// leaderboardKey is the Redis sorted-set key backing the leaderboard cache.
const leaderboardKey = "leaderboard"

func sessionKey(token string) string {
	return "session:" + token
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

// lookupSession resolves a login token to a username via Redis, shared by
// Play and Chat so both auth checks go through the same path.
func (s *server) lookupSession(ctx context.Context, token string) (string, error) {
	username, err := s.redis.Get(ctx, sessionKey(token)).Result()
	if err == redis.Nil {
		return "", fmt.Errorf("not logged in: unknown or missing token")
	}
	if err != nil {
		return "", fmt.Errorf("session lookup failed: %w", err)
	}
	return username, nil
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

// Run starts the gRPC server: connects to Postgres and Redis, warms the
// leaderboard cache, and blocks serving requests until the process exits.
func Run() {
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

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
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
