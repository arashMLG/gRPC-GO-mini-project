package memory

import (
	"sync"

	"myGuy/internal/domain"
)

// LeaderboardNotifier implements domain.LeaderboardNotifier for subscribers
// inside this process.
type LeaderboardNotifier struct {
	mu   sync.Mutex
	subs map[chan struct{}]struct{}
}

func NewLeaderboardNotifier() *LeaderboardNotifier {
	return &LeaderboardNotifier{subs: make(map[chan struct{}]struct{})}
}

var _ domain.LeaderboardNotifier = (*LeaderboardNotifier)(nil)

// Subscribe registers a watcher. The channel has a buffer of one because the
// signal carries no data: a subscriber that has one pending signal already
// knows the board changed, so a second one adds nothing.
func (n *LeaderboardNotifier) Subscribe() (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)

	n.mu.Lock()
	n.subs[ch] = struct{}{}
	n.mu.Unlock()

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			n.mu.Lock()
			delete(n.subs, ch)
			close(ch)
			n.mu.Unlock()
		})
	}
	return ch, unsubscribe
}

// Notify signals every watcher, skipping any that already has a pending
// signal it has not consumed yet.
func (n *LeaderboardNotifier) Notify() {
	n.mu.Lock()
	defer n.mu.Unlock()
	for ch := range n.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
