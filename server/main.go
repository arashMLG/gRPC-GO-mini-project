package main

import (
	"context"
	"fmt"
	"log"
	"myGuy/pb"
	"net"

	"google.golang.org/grpc"
)

type server struct {
	pb.UnimplementedGreeterServer
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
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatal("Recevied error in Listening: %v", err)
	}

	s := grpc.NewServer()
	pb.RegisterGreeterServer(s, &server{})

	log.Printf("Listening on %s", lis.Addr())

	if err := s.Serve(lis); err != nil {
		log.Fatalf("Recived error in Serving: %v", err)
	}
}
