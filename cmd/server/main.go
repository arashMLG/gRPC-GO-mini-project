// Command server is the composition root: the single place that knows which
// concrete implementation satisfies each port. Every other package depends
// on interfaces only, so changing a backing store means editing the wiring
// below and nothing else.
package main

import (
	"context"
	"database/sql"
	"log"
	"net"

	memoryadapter "myGuy/internal/adapter/memory"
	postgresadapter "myGuy/internal/adapter/postgres"
	redisadapter "myGuy/internal/adapter/redis"
	securityadapter "myGuy/internal/adapter/security"
	"myGuy/internal/config"
	"myGuy/internal/pb"
	"myGuy/internal/service"
	grpctransport "myGuy/internal/transport/grpc"

	_ "github.com/jackc/pgx/v5/stdlib"
	goredis "github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc"
)

func main() {
	ctx := context.Background()
	cfg := config.Load()

	// --- infrastructure: open the connections the adapters will wrap -------
	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Error configuring database : %v", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("Error opening database : %v", err)
	}
	log.Println("Connected to PostgreSQL")

	rdb := goredis.NewClient(&goredis.Options{Addr: cfg.RedisAddr})
	defer rdb.Close()

	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("Error connecting to Redis : %v", err)
	}
	log.Println("Connected to Redis")

	// --- adapters: concrete implementations of the domain ports -----------
	// Swapping any single line here changes where that concern is stored
	// without touching a service. Moving sessions back into process memory,
	// for instance, means writing a memory session repository and changing
	// only the sessionRepo line.
	userRepo := postgresadapter.NewUserRepository(db)
	sessionRepo := redisadapter.NewSessionRepository(rdb)
	boardRepo := redisadapter.NewLeaderboardRepository(rdb)
	hasher := securityadapter.NewBcryptHasher(bcrypt.DefaultCost)
	tokens := securityadapter.NewRandomTokenGenerator(16)
	chatHub := memoryadapter.NewChatBroadcaster()
	boardNotifier := memoryadapter.NewLeaderboardNotifier()

	// --- services: business logic, injected with the adapters above -------
	authService := service.NewAuthService(userRepo, sessionRepo, hasher, tokens, cfg.SessionTTL)
	gameService := service.NewGameService(userRepo, boardRepo, boardNotifier)
	chatService := service.NewChatService(chatHub)
	boardService := service.NewLeaderboardService(userRepo, boardRepo, boardNotifier)

	// Rebuild the leaderboard cache from durable storage before serving, so
	// a fresh or emptied Redis does not serve an empty board.
	if err := boardService.WarmCache(ctx); err != nil {
		log.Fatalf("Error warming leaderboard cache : %v", err)
	}

	// --- transport: the inbound adapter, injected with the services -------
	handler := grpctransport.NewGameHandler(authService, gameService, chatService, boardService)

	lis, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		log.Fatalf("Error in listening : %v", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterGameServer(grpcServer, handler)

	log.Printf("gRPC server listening on %s", lis.Addr())
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
