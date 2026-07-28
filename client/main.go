package main

import (
	"context"
	"log"
	"myGuy/pb"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	name := ""

	if len(os.Args) > 1 {
		name = os.Args[1]
	}

	conn, err := grpc.NewClient(
		"localhost:50051",
		grpc.WithTransportCredentials(insecure.NewCredentials()), // No TLS
	)

	if err != nil {
		log.Fatalf("Couldn't connect: %v", err)
	}

	defer conn.Close()

	client := pb.NewGreeterClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	resp, err := client.SayHelloWorld(ctx, &pb.HelloWorldRequest{Name: name})
	if err != nil {
		log.Fatalf("Recevied error in Greeting: %v", err)
	}
	log.Printf("Server replied %s", resp.GetMessage())
}
