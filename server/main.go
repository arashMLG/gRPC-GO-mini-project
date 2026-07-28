package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"myGuy/pb"
	"net"
	"os"
	"strings"
	"sync"

	_ "github.com/jackc/pgx/v5/stdlib"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc"
)

type server struct {
	pb.UnimplementedGreeterServer
	pb.UnimplementedGameServer

	db       *sql.DB
	mu       sync.Mutex
	sessions map[string]string
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

func (s *server) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterReply, error) {
	if req.Username == "" || req.Password == "" {
		return nil, fmt.Errorf("Fill Both the Username and Password bro")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)

	if err != nil {
		return nil, fmt.Errorf("Password Couldn't be hashed %w", &err)
	}

	_, err = s.db.ExecContext(ctx,
		"INSERT INTO users (username, password_hash) VALUES ($1, $2)",
		req.Username, string(hash))

	if err != nil {
		return nil, fmt.Errorf("Couldn't create the user (SQL error or Username taken) : %w", err)
	}
	return &pb.RegisterReply{Message: "Account Created for " + req.Username}, nil
}

func newToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *server) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginReply, error) {
	var hash string

	err := s.db.QueryRowContext(ctx,
		"SELECT password_hash FROM users WHERE username = $1",
		req.Username).Scan(&hash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("Username or Password invalid (HINT USERNAME)")
	} else if err != nil {
		return nil, fmt.Errorf("SQL query Error : %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)); err != nil {
		return nil, fmt.Errorf("Username or Password invalid (HINT PASSWORD)")
	}

	token, err := newToken()
	if err != nil {
		return nil, fmt.Errorf("Error creating token : %w", err)
	}

	s.mu.Lock()
	s.sessions[token] = req.Username
	s.mu.Unlock()

	return &pb.LoginReply{Token: token, Message: "Welcome to the GAME hahaha"}, nil
}

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
	})

	log.Printf("gRPC server listening on %s", lis.Addr())

	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
