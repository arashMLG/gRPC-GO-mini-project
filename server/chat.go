package main

import (
	"log"
	"myGuy/pb"
)

func (s *server) Chat(stream pb.Game_ChatServer) error {
	outbox := make(chan *pb.ChatMessage)
	s.mu.Lock()
	s.clients[outbox] = true
	s.mu.Unlock()

	go func() {
		for msg := range outbox {
			if err := stream.Send(msg); err != nil {
				return
			}
		}
	}()

	var username string
	for {
		in, err := stream.Recv()
		if err != nil {
			break
		}
		if username == "" {
			u, err := s.lookupSession(stream.Context(), in.GetToken())
			if err != nil {
				continue
			}
			username = u
		}
		log.Printf("%s said: %s", username, in.GetText())
		s.broadcast(&pb.ChatMessage{Username: username, Text: in.GetText()})
	}
	s.mu.Lock()
	delete(s.clients, outbox)
	close(outbox)
	s.mu.Unlock()
	return nil
}

func (s *server) broadcast(msg *pb.ChatMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for ch := range s.clients {
		ch <- msg
	}
}
