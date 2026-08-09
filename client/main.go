package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"myGuy/pb"
	"os"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {

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
	fmt.Printf("Connected to chatroom. type \"/chat\" to start chatting and \"/game\" to go back to game.\"/quit\" to quit.\n")

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

	token := LoginRep.GetToken()
	fmt.Println(LoginRep.GetMessage())
	state := 0
	for {
		text := ask("> ")
		if text == "/quit" {
			break
		}
		if text == "/game" && state == 1 {
			state = 0
			continue
		}
		if text == "/chat" && state == 0 {
			state = 1
			continue
		}
		switch state {
		case 0:
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			rep, err := client.Play(ctx, &pb.PlayRequest{Token: token, Word: text})
			cancel()
			if err != nil {
				log.Printf("Idk error : %v", err)
				continue
			}
			fmt.Printf("What gods think of you as a number : %+d\nWhat gods think of you in text: %s\n",
				rep.GetPointsChange(), rep.GetMessage())
		case 1:
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
