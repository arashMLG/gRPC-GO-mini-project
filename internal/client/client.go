package client

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"myGuy/internal/pb"
	"os"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Run connects to the gRPC server, logs the user in, and drives the
// interactive chat/game/leaderboard REPL until the user quits.
func Run() {

	addr := os.Getenv("SERVER_ADDR")
	if addr == "" {
		addr = "localhost:50051"
	}

	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Couldn't connect: %v", err)
	}
	defer conn.Close()

	client := pb.NewGameClient(conn)

	reader := bufio.NewScanner(os.Stdin)

	ask := func(prompt string) string {
		fmt.Print(prompt)
		reader.Scan()
		return strings.TrimSpace(reader.Text())
	}

	username := ask("Username :")
	password := ask("Password :")

	if strings.ToLower(ask("Register new account? (y/n):  ")) == "y" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, err := client.Register(ctx, &pb.RegisterRequest{Username: username, Password: password})
		cancel()
		if err != nil {
			log.Fatalf("Register failed : %v", err)
		}
		fmt.Println("Account created")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	LoginRep, err := client.Login(ctx, &pb.LoginRequest{Username: username, Password: password})
	cancel()
	if err != nil {
		log.Fatalf("Login failed: %v", err)
	}

	stream, err := client.Chat(context.Background())
	if err != nil {
		log.Fatalf("couldn't open chat : %v", err)
	}
	fmt.Printf("Connected to chatroom. type \"/chat\" to start chatting and \"/game\" to go back to game \"/leaderboard\" to turn leaderboard ON or OFF.\"/quit\" to quit.\n")

	go func() {
		for {
			msg, err := stream.Recv()
			if err != nil {
				return
			}
			if username != msg.GetUsername() {
				fmt.Printf("\n%s said :%s\n> ", msg.GetUsername(), msg.GetText())
			}
		}
	}()
	boardCtx, stopBoard := context.WithCancel(context.Background())
	stopBoard()
	boardOn := false

	token := LoginRep.GetToken()
	fmt.Println(LoginRep.GetMessage())
	const (
		modeGame = iota
		modeChat
	)
	state := modeGame
	for {
		text := ask("> ")
		if text == "/leaderboard" {
			if boardOn {
				stopBoard()
				boardOn = false
				fmt.Println("Leaderboard Off!")
			} else {
				boardCtx, stopBoard = context.WithCancel(context.Background())
				boardOn = true
				go watchBoard(client, boardCtx)
				fmt.Println("Leaderboard On!")
			}
			continue
		}
		if text == "/quit" {
			break
		}
		if text == "/game" {
			state = modeGame
			continue
		}
		if text == "/chat" {
			state = modeChat
			continue
		}
		switch state {
		case modeGame:
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			rep, err := client.Play(ctx, &pb.PlayRequest{Token: token, Word: text})
			cancel()
			if err != nil {
				log.Printf("Idk error : %v", err)
				continue
			}
			fmt.Printf("What gods think of you as a number : %+d\nWhat gods think of you in text: %s\n",
				rep.GetPointsChange(), rep.GetMessage())
		case modeChat:
			if text == "" {
				continue
			}
			if err := stream.Send(&pb.ChatMessage{Token: token, Text: text}); err != nil {
				log.Printf("send failed: %v", err)
				break
			}
		}

	}
	fmt.Printf("meow")
}

func watchBoard(client pb.GameClient, ctx context.Context) {
	stream, err := client.Leaderboard(ctx, &pb.LeaderboardRequest{TopN: 5})
	if err != nil {
		log.Printf("leaderboard failed: %v", err)
		return
	}
	for {
		reply, err := stream.Recv()
		if err != nil {
			return
		}
		fmt.Printf("\n--- LEADERBOARD ---\n")
		for _, e := range reply.GetEntries() {
			fmt.Printf("%d. %-12s %d\n", e.GetRank(), e.GetUsername(), e.GetPoints())
		}
		fmt.Print("> ")
	}
}
