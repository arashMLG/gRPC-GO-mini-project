package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"myGuy/pb"

	"golang.org/x/crypto/bcrypt"
)

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
