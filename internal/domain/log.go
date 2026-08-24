package domain

import (
	"context"
	"errors"
	"time"
)

// ErrIngestorStopped is returned when a log is submitted to an ingestor that
// is shutting down and no longer accepting work.
var ErrIngestorStopped = errors.New("log ingestor is stopped")

// LogEntry is one application log line.
type LogEntry struct {
	Username string
	Level    string
	Message  string
	Source   string
	LoggedAt time.Time
}

// LogRepository is the port for durable log storage.
//
// Note that the only write method is a bulk one. That is deliberate: the
// interface refuses to offer a one-row insert, so no caller can accidentally
// fall back to writing logs one at a time. The batching policy is enforced by
// the shape of the port, not by everyone remembering to be careful.
type LogRepository interface {
	// BulkInsert writes an entire batch. It must be all-or-nothing: a
	// partial write would make a retry produce duplicates.
	BulkInsert(ctx context.Context, entries []LogEntry) error
}
