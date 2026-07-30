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

	_ "github.com/jackc/pgx/v5/stdlib"
	"google.golang.org/grpc"
)

type server struct {
	pb.UnimplementedGreeterServer
	pb.UnimplementedGameServer

	db       *sql.DB
	mu       sync.Mutex
	sessions map[string]string
	clients  map[chan *pb.ChatMessage]bool
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

	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Error in Listening : %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterGameServer(s, &server{
		db:       db,
		sessions: make(map[string]string),
		clients:  make(map[chan *pb.ChatMessage]bool),
	})

	log.Printf("gRPC server listening on %s", lis.Addr())

	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
