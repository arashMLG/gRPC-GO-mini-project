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
	conn, err := grpc.NewClient("localhost:50051",
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
	token := LoginRep.GetToken()
	fmt.Println(LoginRep.GetMessage())
	for {
		word := ask("> ")
		if word == "quit" {
			break
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		rep, err := client.Play(ctx, &pb.PlayRequest{Token: token, Word: word})
		cancel()
		if err != nil {
			log.Printf("Idk error : %v", err)
			continue
		}
		fmt.Printf("What gods think of you as a number : %+d\nWhat gods think of you in text: %s\n",
			rep.GetPointsChange(), rep.GetMessage())

	}
	fmt.Printf("meow")
}
