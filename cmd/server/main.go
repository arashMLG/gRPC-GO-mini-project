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
	"os/signal"
	"syscall"
	"time"

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
	// signalCtx is cancelled on Ctrl-C or SIGTERM, which starts the graceful
	// shutdown sequence at the bottom of this function.
	signalCtx, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals()

	ctx := context.Background()
	cfg := config.Load()

	// --- infrastructure: open the connections the adapters will wrap -------
	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Error configuring database : %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)

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
	userRepo := postgresadapter.NewUserRepository(db)
	logRepo := postgresadapter.NewLogRepository(db)
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

	// The log ingestor runs its own goroutines, so unlike the other services
	// it has a lifecycle: Start here, Stop during shutdown below.
	logIngestor := service.NewLogIngestor(logRepo, service.DefaultIngestConfig())

	// ingestCtx is NOT the signal context. Cancelling it aborts in-flight
	// retries and abandons buffered logs, so it stays alive through shutdown
	// and only Stop's deadline bounds the drain.
	ingestCtx, abortIngest := context.WithCancel(context.Background())
	defer abortIngest()
	logIngestor.Start(ingestCtx)

	if err := boardService.WarmCache(ctx); err != nil {
		log.Fatalf("Error warming leaderboard cache : %v", err)
	}

	// --- transport: the inbound adapter, injected with the services -------
	gameHandler := grpctransport.NewGameHandler(authService, gameService, chatService, boardService)
	logHandler := grpctransport.NewLogHandler(authService, logIngestor)

	lis, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		log.Fatalf("Error in listening : %v", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterGameServer(grpcServer, gameHandler)
	pb.RegisterLogIngestServer(grpcServer, logHandler)

	go func() {
		log.Printf("gRPC server listening on %s", lis.Addr())
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("Failed to serve: %v", err)
		}
	}()

	// --- shutdown ---------------------------------------------------------
	<-signalCtx.Done()
	log.Println("Shutting down: refusing new work, draining buffered logs")

	// Stop accepting RPCs first, so no new logs arrive while draining.
	grpcServer.GracefulStop()

	drainCtx, cancelDrain := context.WithTimeout(context.Background(), cfg.ShutdownGrace)
	defer cancelDrain()

	if err := logIngestor.Stop(drainCtx); err != nil {
		log.Printf("Log drain did not finish within %s: %v", cfg.ShutdownGrace, err)
	}
	log.Println("Shutdown complete")
}
