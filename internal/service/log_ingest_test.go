package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"myGuy/internal/domain"
)

// fakeLogRepo is a domain.LogRepository that can be switched "down" to
// simulate the database being unreachable, and that records everything it was
// asked to write so tests can check for loss and for duplicates.
type fakeLogRepo struct {
	mu       sync.Mutex
	down     bool
	written  []domain.LogEntry
	attempts int
	failures int
	maxBatch int
}

func newFakeLogRepo() *fakeLogRepo { return &fakeLogRepo{} }

var _ domain.LogRepository = (*fakeLogRepo)(nil)

func (f *fakeLogRepo) BulkInsert(_ context.Context, entries []domain.LogEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.attempts++
	if f.down {
		f.failures++
		return errors.New("simulated outage: connection refused")
	}
	if len(entries) > f.maxBatch {
		f.maxBatch = len(entries)
	}
	f.written = append(f.written, entries...)
	return nil
}

func (f *fakeLogRepo) setDown(down bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.down = down
}

func (f *fakeLogRepo) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.written)
}

func (f *fakeLogRepo) stats() (attempts, failures, maxBatch int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.attempts, f.failures, f.maxBatch
}

// messages returns every message written, so a test can assert that the set
// is exactly what was submitted: nothing missing (loss) and nothing twice
// (duplicate writes from a retry).
func (f *fakeLogRepo) messages() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.written))
	for _, e := range f.written {
		out = append(out, e.Message)
	}
	return out
}

func testEntry(n int) domain.LogEntry {
	return domain.LogEntry{
		Username: "arash",
		Level:    "INFO",
		Message:  fmt.Sprintf("log-%d", n),
		Source:   "test",
		LoggedAt: time.Now().UTC(),
	}
}

// assertExactlyOnce checks the strongest property the pipeline promises:
// every submitted entry was written, and none was written more than once.
func assertExactlyOnce(t *testing.T, repo *fakeLogRepo, total int) {
	t.Helper()

	seen := make(map[string]int, total)
	for _, msg := range repo.messages() {
		seen[msg]++
	}

	var missing, duplicated int
	for n := 0; n < total; n++ {
		switch seen[fmt.Sprintf("log-%d", n)] {
		case 1:
		case 0:
			missing++
		default:
			duplicated++
		}
	}

	if missing != 0 || duplicated != 0 {
		t.Errorf("expected each of %d entries exactly once: %d missing, %d duplicated (repo holds %d rows)",
			total, missing, duplicated, repo.count())
	}
}

func TestFlushesWhenBatchSizeIsReached(t *testing.T) {
	repo := newFakeLogRepo()
	ing := NewLogIngestor(repo, IngestConfig{
		BatchSize: 50,
		// Long enough that the timer cannot be what causes the flush.
		FlushWindow: time.Hour,
		Workers:     2,
		QueueSize:   1000,
		MinBackoff:  time.Millisecond,
		MaxBackoff:  5 * time.Millisecond,
	})
	ing.Start(context.Background())

	for n := 0; n < 100; n++ {
		if err := ing.Submit(context.Background(), testEntry(n)); err != nil {
			t.Fatalf("submit %d: %v", n, err)
		}
	}

	waitFor(t, 2*time.Second, func() bool { return repo.count() == 100 })

	if _, _, maxBatch := repo.stats(); maxBatch != 50 {
		t.Errorf("largest batch was %d rows, want 50", maxBatch)
	}

	stopIngestor(t, ing)
	assertExactlyOnce(t, repo, 100)
}

func TestFlushesPartialBatchWhenWindowElapses(t *testing.T) {
	repo := newFakeLogRepo()
	ing := NewLogIngestor(repo, IngestConfig{
		// Far larger than what we submit, so only the timer can flush it.
		BatchSize:   10000,
		FlushWindow: 50 * time.Millisecond,
		Workers:     1,
		QueueSize:   100,
		MinBackoff:  time.Millisecond,
		MaxBackoff:  5 * time.Millisecond,
	})
	ing.Start(context.Background())

	for n := 0; n < 3; n++ {
		if err := ing.Submit(context.Background(), testEntry(n)); err != nil {
			t.Fatalf("submit %d: %v", n, err)
		}
	}

	waitFor(t, 2*time.Second, func() bool { return repo.count() == 3 })

	stopIngestor(t, ing)
	assertExactlyOnce(t, repo, 3)
}

// This is requirement 3 in miniature: the database is unreachable while logs
// keep arriving, then it comes back. Nothing may crash and nothing may be
// lost. The outage is scaled down so the test stays fast; see
// TestSurvivesTenSecondDatabaseOutage for the literal ten-second version.
func TestNoLogsLostWhileDatabaseIsDown(t *testing.T) {
	repo := newFakeLogRepo()
	repo.setDown(true)

	ing := NewLogIngestor(repo, IngestConfig{
		BatchSize:   50,
		FlushWindow: 10 * time.Millisecond,
		Workers:     4,
		QueueSize:   2048,
		MinBackoff:  5 * time.Millisecond,
		MaxBackoff:  20 * time.Millisecond,
	})
	ing.Start(context.Background())

	const total = 1000
	for n := 0; n < total; n++ {
		if err := ing.Submit(context.Background(), testEntry(n)); err != nil {
			t.Fatalf("submit %d during outage: %v", n, err)
		}
	}

	// While the database is down nothing may reach it, and the pipeline must
	// still be alive and accepting.
	time.Sleep(100 * time.Millisecond)
	if got := repo.count(); got != 0 {
		t.Fatalf("%d entries reached a database that was down", got)
	}
	if _, failures, _ := repo.stats(); failures == 0 {
		t.Fatal("expected failed write attempts while the database was down")
	}
	if queued := ing.Stats().Queued; queued != total {
		t.Errorf("%d entries queued, want all %d held in memory", queued, total)
	}

	// The database comes back.
	repo.setDown(false)

	stopIngestor(t, ing)

	if got := repo.count(); got != total {
		t.Errorf("wrote %d entries, want %d", got, total)
	}
	assertExactlyOnce(t, repo, total)

	if s := ing.Stats(); s.Written != total {
		t.Errorf("stats report %d written, want %d", s.Written, total)
	}
}

// Requirement 3 as literally specified: a ten-second outage. Skipped under
// -short so the everyday test run stays quick.
func TestSurvivesTenSecondDatabaseOutage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping the literal 10-second outage simulation in -short mode")
	}

	repo := newFakeLogRepo()
	ing := NewLogIngestor(repo, IngestConfig{
		BatchSize:   500,
		FlushWindow: 2 * time.Second,
		Workers:     4,
		QueueSize:   8192,
		MinBackoff:  100 * time.Millisecond,
		MaxBackoff:  time.Second,
	})
	ing.Start(context.Background())

	const total = 5000
	submitted := 0

	// Some traffic lands before the outage starts.
	for ; submitted < 1000; submitted++ {
		if err := ing.Submit(context.Background(), testEntry(submitted)); err != nil {
			t.Fatalf("submit %d: %v", submitted, err)
		}
	}
	waitFor(t, 5*time.Second, func() bool { return repo.count() > 0 })

	// --- the database goes away for ten seconds --------------------------
	repo.setDown(true)
	outageStart := time.Now()

	// Traffic keeps arriving throughout the outage.
	for time.Since(outageStart) < 10*time.Second {
		if submitted >= total {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if err := ing.Submit(context.Background(), testEntry(submitted)); err != nil {
			t.Fatalf("submit %d during outage: %v", submitted, err)
		}
		submitted++
		time.Sleep(2 * time.Millisecond)
	}

	duringOutage := repo.count()

	// --- the database comes back -----------------------------------------
	repo.setDown(false)

	for ; submitted < total; submitted++ {
		if err := ing.Submit(context.Background(), testEntry(submitted)); err != nil {
			t.Fatalf("submit %d after recovery: %v", submitted, err)
		}
	}

	stopIngestor(t, ing)

	attempts, failures, _ := repo.stats()
	t.Logf("outage lasted %s; %d entries written before recovery; %d/%d write attempts failed",
		time.Since(outageStart).Round(time.Millisecond), duringOutage, failures, attempts)

	if failures == 0 {
		t.Error("expected write failures during the simulated outage")
	}
	if got := repo.count(); got != total {
		t.Errorf("wrote %d entries, want all %d", got, total)
	}
	assertExactlyOnce(t, repo, total)
}

// A full queue must block the submitter rather than silently discarding
// entries. This is what makes "without loss" true even when the outage
// outlasts the buffer.
func TestSubmitAppliesBackpressureInsteadOfDropping(t *testing.T) {
	repo := newFakeLogRepo()
	repo.setDown(true)

	ing := NewLogIngestor(repo, IngestConfig{
		BatchSize:   4,
		FlushWindow: 5 * time.Millisecond,
		Workers:     1,
		QueueSize:   8,
		MinBackoff:  5 * time.Millisecond,
		MaxBackoff:  10 * time.Millisecond,
	})
	ing.Start(context.Background())

	const total = 200
	submitDone := make(chan error, 1)
	go func() {
		for n := 0; n < total; n++ {
			if err := ing.Submit(context.Background(), testEntry(n)); err != nil {
				submitDone <- err
				return
			}
		}
		submitDone <- nil
	}()

	// With the database down and a queue of 8, the submitter cannot possibly
	// have pushed all 200 entries: it must be parked in Submit.
	select {
	case err := <-submitDone:
		t.Fatalf("submitting %d entries into a queue of 8 finished while the database was down (err=%v); entries were dropped rather than held", total, err)
	case <-time.After(200 * time.Millisecond):
	}

	repo.setDown(false)

	select {
	case err := <-submitDone:
		if err != nil {
			t.Fatalf("submit failed after recovery: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("submitters never unblocked after the database recovered")
	}

	stopIngestor(t, ing)
	assertExactlyOnce(t, repo, total)
}

func TestStopDrainsBufferedEntries(t *testing.T) {
	repo := newFakeLogRepo()
	ing := NewLogIngestor(repo, IngestConfig{
		BatchSize: 1000, // never reached
		// Long enough that only the shutdown drain can flush these.
		FlushWindow: time.Hour,
		Workers:     2,
		QueueSize:   256,
		MinBackoff:  time.Millisecond,
		MaxBackoff:  5 * time.Millisecond,
	})
	ing.Start(context.Background())

	const total = 37
	for n := 0; n < total; n++ {
		if err := ing.Submit(context.Background(), testEntry(n)); err != nil {
			t.Fatalf("submit %d: %v", n, err)
		}
	}

	stopIngestor(t, ing)

	if got := repo.count(); got != total {
		t.Errorf("drain wrote %d entries, want %d", got, total)
	}
	assertExactlyOnce(t, repo, total)
}

func TestSubmitAfterStopIsRejected(t *testing.T) {
	repo := newFakeLogRepo()
	ing := NewLogIngestor(repo, DefaultIngestConfig())
	ing.Start(context.Background())
	stopIngestor(t, ing)

	err := ing.Submit(context.Background(), testEntry(0))
	if !errors.Is(err, domain.ErrIngestorStopped) {
		t.Fatalf("got %v, want ErrIngestorStopped", err)
	}
}

func TestConcurrentSubmittersLoseNothing(t *testing.T) {
	repo := newFakeLogRepo()
	ing := NewLogIngestor(repo, IngestConfig{
		BatchSize:   32,
		FlushWindow: 10 * time.Millisecond,
		Workers:     4,
		QueueSize:   1024,
		MinBackoff:  time.Millisecond,
		MaxBackoff:  5 * time.Millisecond,
	})
	ing.Start(context.Background())

	const (
		producers  = 8
		perProduce = 125
		total      = producers * perProduce
	)

	var wg sync.WaitGroup
	for p := 0; p < producers; p++ {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			for n := 0; n < perProduce; n++ {
				if err := ing.Submit(context.Background(), testEntry(p*perProduce+n)); err != nil {
					t.Errorf("producer %d submit %d: %v", p, n, err)
					return
				}
			}
		}(p)
	}
	wg.Wait()

	stopIngestor(t, ing)

	if got := repo.count(); got != total {
		t.Errorf("wrote %d entries, want %d", got, total)
	}
	assertExactlyOnce(t, repo, total)
}

// stopIngestor shuts the pipeline down and fails the test if the drain does
// not complete, so a hang shows up as a clear failure rather than a timeout.
func stopIngestor(t *testing.T, ing *LogIngestor) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := ing.Stop(ctx); err != nil {
		t.Fatalf("ingestor did not drain: %v", err)
	}
}

// waitFor polls until cond holds or the deadline passes.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("condition not met within %s", timeout)
}
