package grpc

import (
	"context"
	"fmt"
	"log"

	"myGuy/internal/pb"
)

// GreeterHandler implements the Greeter service from the .proto file.
//
// Note: as in the original code, this service is implemented but never
// registered in cmd/server, so it is not reachable by clients. It is kept
// here so nothing is silently deleted — either register it in the
// composition root to make it live, or drop this file and the Greeter
// service from test.proto.
type GreeterHandler struct {
	pb.UnimplementedGreeterServer
}

func NewGreeterHandler() *GreeterHandler {
	return &GreeterHandler{}
}

var _ pb.GreeterServer = (*GreeterHandler)(nil)

func (h *GreeterHandler) SayHelloWorld(ctx context.Context, req *pb.HelloWorldRequest) (*pb.HelloWorldReplay, error) {
	name := req.GetName()
	if name == "" {
		name = "Cat got your tongue?"
	}
	log.Printf("SayHelloWorld request received from %q", name)
	return &pb.HelloWorldReplay{
		Message: fmt.Sprintf("Hello World! Hello, %s!!!", name),
	}, nil
}
