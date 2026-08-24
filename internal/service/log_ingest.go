package service

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"myGuy/internal/domain"
)

// IngestConfig tunes the batching worker pool.
type IngestConfig struct {
	// BatchSize is the number of entries that triggers an immediate flush.
	BatchSize int

	// FlushWindow is how long a partial batch may wait before being written
	// anyway. Without it, a quiet system would hold the last few logs
	// indefinitely.
	FlushWindow time.Duration

	// Workers is how many batches may be written concurrently.
	Workers int

	// QueueSize is how many submitted entries may wait in memory before
	// Submit starts blocking. This buffer is what absorbs a database outage.
	QueueSize int

	// MinBackoff and MaxBackoff bound the retry delay after a failed write.
	MinBackoff time.Duration
	MaxBackoff time.Duration
}

// DefaultIngestConfig is the assignment's target shape: flush at 500 entries
// or every 2 seconds, whichever comes first.
func DefaultIngestConfig() IngestConfig {
	return IngestConfig{
		BatchSize:   500,
		FlushWindow: 2 * time.Second,
		Workers:     4,
		QueueSize:   8192,
		MinBackoff:  100 * time.Millisecond,
		MaxBackoff:  2 * time.Second,
	}
}

// IngestStats is a snapshot of what the ingestor has done so far.
type IngestStats struct {
	Accepted int64 // entries taken from callers
	Written  int64 // entries confirmed written to the repository
	Batches  int64 // successful bulk inserts
	Retries  int64 // failed write attempts that were retried
	Queued   int64 // entries accepted but not yet written
}

// LogIngestor collects log entries and writes them to durable storage in
// bulk. It is the "worker pool" half of the logging pipeline:
//
//	Submit ──> ingest chan ──> dispatcher ──> batches chan ──> N workers ──> repo
//	                           (size/time)                     (bulk+retry)
//
// The dispatcher decides *when* a batch is complete; the workers decide
// *how* to get it written, including surviving the database being gone.
type LogIngestor struct {
	repo domain.LogRepository
	cfg  IngestConfig

	ingest  chan domain.LogEntry
	batches chan []domain.LogEntry

	// stopping is closed by Stop. Nothing ever closes `ingest`, because a
	// concurrent Submit would then panic with "send on closed channel";
	// signalling through a separate channel makes shutdown race-free.
	stopping  chan struct{}
	stopOnce  sync.Once
	startOnce sync.Once

	// hardCtx aborts in-flight retries. It is deliberately NOT cancelled by
	// Stop: a graceful stop must keep retrying so buffered logs still reach
	// the database.
	hardCtx context.Context

	wg sync.WaitGroup

	accepted atomic.Int64
	written  atomic.Int64
	batchCnt atomic.Int64
	retries  atomic.Int64
}

func NewLogIngestor(repo domain.LogRepository, cfg IngestConfig) *LogIngestor {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 1
	}
	if cfg.Workers <= 0 {
		cfg.Workers = 1
	}
	if cfg.QueueSize < cfg.BatchSize {
		cfg.QueueSize = cfg.BatchSize
	}
	if cfg.FlushWindow <= 0 {
		cfg.FlushWindow = time.Second
	}
	if cfg.MinBackoff <= 0 {
		cfg.MinBackoff = 100 * time.Millisecond
	}
	if cfg.MaxBackoff < cfg.MinBackoff {
		cfg.MaxBackoff = cfg.MinBackoff
	}

	return &LogIngestor{
		repo:     repo,
		cfg:      cfg,
		ingest:   make(chan domain.LogEntry, cfg.QueueSize),
		batches:  make(chan []domain.LogEntry, cfg.Workers*2),
		stopping: make(chan struct{}),
	}
}

// Start launches the dispatcher and the worker pool. The context aborts
// in-flight retries on a hard shutdown; use Stop for a graceful one.
func (i *LogIngestor) Start(ctx context.Context) {
	i.startOnce.Do(func() {
		i.hardCtx = ctx

		i.wg.Add(1)
		go i.dispatch()

		for n := 0; n < i.cfg.Workers; n++ {
			i.wg.Add(1)
			go i.work(n)
		}
		log.Printf("log ingest: started %d workers, batch=%d window=%s queue=%d",
			i.cfg.Workers, i.cfg.BatchSize, i.cfg.FlushWindow, i.cfg.QueueSize)
	})
}

// Submit hands one entry to the pipeline.
//
// It blocks when the queue is full rather than dropping the entry. That
// blocking is the whole no-loss guarantee: it propagates backpressure up
// through the gRPC stream to the client, which is the correct response to
// "the database is gone and memory is filling up". The alternative — a
// non-blocking send with a default case — would silently discard exactly the
// logs you most want during an incident.
func (i *LogIngestor) Submit(ctx context.Context, entry domain.LogEntry) error {
	select {
	case <-i.stopping:
		return domain.ErrIngestorStopped
	default:
	}

	select {
	case i.ingest <- entry:
		i.accepted.Add(1)
		return nil
	case <-i.stopping:
		return domain.ErrIngestorStopped
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Stop stops accepting new entries, then drains and writes everything already
// accepted. It returns ctx.Err() if the drain does not finish in time, so a
// caller can bound how long shutdown waits on a database that never returns.
func (i *LogIngestor) Stop(ctx context.Context) error {
	i.stopOnce.Do(func() { close(i.stopping) })

	done := make(chan struct{})
	go func() {
		i.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		s := i.Stats()
		log.Printf("log ingest: stopped cleanly (accepted=%d written=%d batches=%d retries=%d)",
			s.Accepted, s.Written, s.Batches, s.Retries)
		return nil
	case <-ctx.Done():
		s := i.Stats()
		log.Printf("log ingest: shutdown deadline hit with %d entries unwritten", s.Queued)
		return ctx.Err()
	}
}

// Stats returns a snapshot of the pipeline's counters.
func (i *LogIngestor) Stats() IngestStats {
	accepted := i.accepted.Load()
	written := i.written.Load()
	return IngestStats{
		Accepted: accepted,
		Written:  written,
		Batches:  i.batchCnt.Load(),
		Retries:  i.retries.Load(),
		Queued:   accepted - written,
	}
}

// dispatch is the batching half: it accumulates entries and cuts a batch
// when either the size threshold or the time window is reached, whichever
// happens first.
func (i *LogIngestor) dispatch() {
	defer i.wg.Done()

	ticker := time.NewTicker(i.cfg.FlushWindow)
	defer ticker.Stop()

	batch := make([]domain.LogEntry, 0, i.cfg.BatchSize)

	// flush hands the current batch to the workers and starts a new one. The
	// slice is copied because `batch` is reused; handing the same backing
	// array to a worker would let the dispatcher overwrite entries the worker
	// has not written yet.
	flush := func() bool {
		if len(batch) == 0 {
			return true
		}
		out := make([]domain.LogEntry, len(batch))
		copy(out, batch)
		batch = batch[:0]

		select {
		case i.batches <- out:
			return true
		case <-i.hardCtx.Done():
			return false
		}
	}

	finish := func() {
		// Drain whatever is still buffered so a graceful stop loses nothing.
		for {
			select {
			case entry := <-i.ingest:
				batch = append(batch, entry)
				if len(batch) >= i.cfg.BatchSize && !flush() {
					close(i.batches)
					return
				}
			default:
				flush()
				close(i.batches)
				return
			}
		}
	}

	for {
		select {
		case entry := <-i.ingest:
			batch = append(batch, entry)
			if len(batch) >= i.cfg.BatchSize {
				if !flush() {
					finish()
					return
				}
			}

		case <-ticker.C:
			// Time-based flush: a partial batch that has waited long enough
			// gets written anyway.
			if !flush() {
				finish()
				return
			}

		case <-i.stopping:
			finish()
			return

		case <-i.hardCtx.Done():
			close(i.batches)
			return
		}
	}
}

// work is one worker: pull a completed batch, write it, repeat.
func (i *LogIngestor) work(id int) {
	defer i.wg.Done()

	for batch := range i.batches {
		i.writeWithRetry(id, batch)
	}
}

// writeWithRetry is the outage-survival half. It retries a failed bulk insert
// with exponential backoff and does not give up, because giving up would mean
// losing the batch. It stops only on a hard abort.
//
// While it is retrying it is not reading from i.batches, so batches pile up,
// the dispatcher stops draining i.ingest, and Submit eventually blocks. That
// chain is the backpressure that keeps memory bounded instead of growing
// until the process is killed.
func (i *LogIngestor) writeWithRetry(id int, batch []domain.LogEntry) {
	backoff := i.cfg.MinBackoff

	for attempt := 1; ; attempt++ {
		err := i.repo.BulkInsert(i.hardCtx, batch)
		if err == nil {
			i.written.Add(int64(len(batch)))
			i.batchCnt.Add(1)
			if attempt > 1 {
				log.Printf("log ingest: worker %d wrote %d entries after %d attempts",
					id, len(batch), attempt)
			}
			return
		}

		// A hard abort is the only thing that abandons a batch.
		if i.hardCtx.Err() != nil {
			log.Printf("log ingest: worker %d abandoning %d entries on shutdown: %v",
				id, len(batch), err)
			return
		}

		i.retries.Add(1)
		if attempt == 1 || attempt%10 == 0 {
			log.Printf("log ingest: worker %d failed to write %d entries (attempt %d), retrying in %s: %v",
				id, len(batch), attempt, backoff, err)
		}

		select {
		case <-time.After(backoff):
		case <-i.hardCtx.Done():
			return
		}

		backoff = min(backoff*2, i.cfg.MaxBackoff)
	}
}
