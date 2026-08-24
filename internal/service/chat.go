package service

import "myGuy/internal/domain"

// ChatService owns chat fan-out. It is deliberately thin: today it only
// forwards to the broadcaster port, but it is the seam where chat rules
// would go — message length limits, profanity filtering, rate limiting, or
// persisting history — without any of that leaking into the gRPC handler.
type ChatService struct {
	broadcaster domain.ChatBroadcaster
}

func NewChatService(broadcaster domain.ChatBroadcaster) *ChatService {
	return &ChatService{broadcaster: broadcaster}
}

// Subscribe returns a channel of incoming messages plus its unsubscribe
// function.
func (s *ChatService) Subscribe() (<-chan domain.ChatMessage, func()) {
	return s.broadcaster.Subscribe()
}

// Send publishes a message from an already-authenticated user.
func (s *ChatService) Send(username, text string) {
	s.broadcaster.Publish(domain.ChatMessage{Username: username, Text: text})
}
