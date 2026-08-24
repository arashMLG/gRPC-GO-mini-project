package memory

import (
	"sync"
	"testing"
	"time"

	"myGuy/internal/domain"
)

func TestPublishReachesEverySubscriber(t *testing.T) {
	b := NewChatBroadcaster()

	first, stopFirst := b.Subscribe()
	defer stopFirst()
	second, stopSecond := b.Subscribe()
	defer stopSecond()

	b.Publish(domain.ChatMessage{Username: "arash", Text: "hello"})

	for i, ch := range []<-chan domain.ChatMessage{first, second} {
		select {
		case msg := <-ch:
			if msg.Username != "arash" || msg.Text != "hello" {
				t.Errorf("subscriber %d got %+v, want {arash hello}", i, msg)
			}
		case <-time.After(time.Second):
			t.Errorf("subscriber %d received nothing", i)
		}
	}
}

func TestUnsubscribedSubscriberStopsReceiving(t *testing.T) {
	b := NewChatBroadcaster()

	ch, stop := b.Subscribe()
	stop()

	// Publishing after unsubscribe must not panic on the closed channel.
	b.Publish(domain.ChatMessage{Username: "arash", Text: "hello"})

	if _, open := <-ch; open {
		t.Fatal("channel should be closed and drained after unsubscribe")
	}
}

func TestUnsubscribeIsIdempotent(t *testing.T) {
	b := NewChatBroadcaster()

	_, stop := b.Subscribe()
	stop()
	stop() // a second call must not panic by double-closing
}

// A subscriber that never reads must not be able to stall the publisher.
// The original implementation sent on an unbuffered channel while holding
// the hub's lock, so one stuck client froze chat for everyone; this test
// pins down the non-blocking behaviour that replaced it.
func TestSlowSubscriberDoesNotBlockPublisher(t *testing.T) {
	b := NewChatBroadcaster()

	_, stopStalled := b.Subscribe() // deliberately never drained
	defer stopStalled()

	healthy, stopHealthy := b.Subscribe()
	defer stopHealthy()

	done := make(chan struct{})
	go func() {
		defer close(done)
		// Far more messages than the buffer can hold.
		for i := 0; i < chatBuffer*4; i++ {
			b.Publish(domain.ChatMessage{Username: "arash", Text: "flood"})
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("publisher blocked on a subscriber that was not reading")
	}

	select {
	case <-healthy:
	case <-time.After(time.Second):
		t.Fatal("healthy subscriber received nothing")
	}
}

func TestConcurrentSubscribePublishUnsubscribe(t *testing.T) {
	b := NewChatBroadcaster()

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch, stop := b.Subscribe()
			go func() {
				for range ch {
				}
			}()
			b.Publish(domain.ChatMessage{Username: "arash", Text: "concurrent"})
			stop()
		}()
	}
	wg.Wait()
}

func TestNotifierSignalsWatchers(t *testing.T) {
	n := NewLeaderboardNotifier()

	ch, stop := n.Subscribe()
	defer stop()

	n.Notify()

	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("watcher was not signalled")
	}
}

// Repeated notifications must collapse rather than queue up: the signal
// carries no data, so one pending "something changed" is as informative as
// twenty.
func TestNotifierCoalescesRepeatedSignals(t *testing.T) {
	n := NewLeaderboardNotifier()

	ch, stop := n.Subscribe()
	defer stop()

	for i := 0; i < 100; i++ {
		n.Notify()
	}

	<-ch // the one pending signal
	select {
	case <-ch:
		t.Fatal("expected signals to coalesce into a single pending notification")
	case <-time.After(50 * time.Millisecond):
	}
}
