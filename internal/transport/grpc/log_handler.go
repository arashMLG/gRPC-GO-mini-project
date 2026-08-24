package grpc

import (
	"errors"
	"fmt"
	"io"
	"time"

	"myGuy/internal/domain"
	"myGuy/internal/pb"
	"myGuy/internal/service"
)

// LogHandler implements pb.LogIngestServer.
type LogHandler struct {
	pb.UnimplementedLogIngestServer

	auth   *service.AuthService
	ingest *service.LogIngestor
}

func NewLogHandler(auth *service.AuthService, ingest *service.LogIngestor) *LogHandler {
	return &LogHandler{auth: auth, ingest: ingest}
}

var _ pb.LogIngestServer = (*LogHandler)(nil)

// Ingest consumes a client-streamed sequence of log entries and answers once
// with a summary.
//
// The important difference from a unary RPC is not just message count: this
// loop can apply backpressure. When the pipeline is saturated, Submit blocks,
// this loop stops calling Recv, gRPC stops reading from the socket, and the
// client's Send eventually blocks too. A unary API has nowhere to put that
// signal, so an overloaded server can only fail requests or buffer without
// limit.
func (h *LogHandler) Ingest(stream pb.LogIngest_IngestServer) error {
	var (
		username string
		accepted int64
		before   = h.ingest.Stats()
	)

	for {
		in, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			// The client finished. Client-streaming RPCs answer with
			// SendAndClose exactly once.
			after := h.ingest.Stats()
			return stream.SendAndClose(&pb.IngestSummary{
				Accepted: accepted,
				Written:  after.Written - before.Written,
				Retries:  after.Retries - before.Retries,
				Message: fmt.Sprintf("accepted %d entries from %s (%d still queued server-wide)",
					accepted, username, after.Queued),
			})
		}
		if err != nil {
			return err
		}

		// Resolve the token once per stream rather than once per entry: a
		// session lookup is a Redis round trip, and paying it per log line
		// would cost more than the write we are trying to batch.
		if username == "" {
			resolved, err := h.auth.Authenticate(stream.Context(), in.GetToken())
			if err != nil {
				return toStatusError(err)
			}
			username = resolved
		}

		entry := domain.LogEntry{
			Username: username,
			Level:    in.GetLevel(),
			Message:  in.GetMessage(),
			Source:   in.GetSource(),
			LoggedAt: loggedAt(in.GetLoggedAtUnixMs()),
		}

		if err := h.ingest.Submit(stream.Context(), entry); err != nil {
			return toStatusError(err)
		}
		accepted++
	}
}

// loggedAt falls back to the server clock when a client sends no timestamp.
func loggedAt(unixMs int64) time.Time {
	if unixMs <= 0 {
		return time.Now().UTC()
	}
	return time.UnixMilli(unixMs).UTC()
}
