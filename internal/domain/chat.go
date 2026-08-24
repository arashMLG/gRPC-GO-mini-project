package domain

// ChatMessage is a single line of chat as the domain sees it. Note there is
// no token field: authentication happens before a message becomes a
// ChatMessage, so the domain only ever deals with an already-identified
// sender.
type ChatMessage struct {
	Username string
	Text     string
}

// ChatBroadcaster is the port for chat fan-out. The in-memory adapter
// delivers only to clients connected to this process; replacing it with a
// Redis Pub/Sub adapter would make chat work across multiple server
// replicas without the chat service changing at all.
type ChatBroadcaster interface {
	// Subscribe returns a channel of incoming messages plus a function that
	// unsubscribes and closes the channel.
	Subscribe() (<-chan ChatMessage, func())

	// Publish delivers a message to every current subscriber.
	Publish(msg ChatMessage)
}
