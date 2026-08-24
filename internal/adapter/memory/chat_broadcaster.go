// Package memory contains adapters that keep their state in this process.
// They are the simplest possible implementations of the fan-out ports, and
// the ones to replace first if the server ever runs as more than one
// replica: a Redis Pub/Sub adapter satisfying the same interfaces would let
// chat and leaderboard updates cross process boundaries.
package memory

import (
	"sync"

	"myGuy/internal/domain"
)

// chatBuffer is how many messages a slow subscriber may fall behind before
// its messages start being dropped.
const chatBuffer = 32

// ChatBroadcaster implements domain.ChatBroadcaster for subscribers inside
// this process.
type ChatBroadcaster struct {
	mu   sync.Mutex
	subs map[chan domain.ChatMessage]struct{}
}

func NewChatBroadcaster() *ChatBroadcaster {
	return &ChatBroadcaster{subs: make(map[chan domain.ChatMessage]struct{})}
}

var _ domain.ChatBroadcaster = (*ChatBroadcaster)(nil)

// Subscribe registers a new subscriber and returns its channel along with an
// unsubscribe function. The unsubscribe function is idempotent, so calling it
// from a defer and again on an error path is safe.
func (b *ChatBroadcaster) Subscribe() (<-chan domain.ChatMessage, func()) {
	ch := make(chan domain.ChatMessage, chatBuffer)

	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			// Remove and close under the same lock Publish holds, so a
			// concurrent Publish can never send on a closed channel.
			b.mu.Lock()
			delete(b.subs, ch)
			close(ch)
			b.mu.Unlock()
		})
	}
	return ch, unsubscribe
}

// Publish delivers a message to every subscriber. Sends are non-blocking: a
// subscriber that is not draining its channel gets its messages dropped
// rather than stalling the publisher, which matters because the publisher
// holds the lock that every other subscriber needs.
func (b *ChatBroadcaster) Publish(msg domain.ChatMessage) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- msg:
		default:
		}
	}
}
