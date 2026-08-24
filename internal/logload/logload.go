// Package logload is a load generator for the log ingestion pipeline. It
// exists so the database-outage scenario can be demonstrated against a real
// server: start it, pause Postgres for ten seconds, and watch the server
// absorb, retry, and recover without dropping anything.
package logload

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"myGuy/internal/pb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Run parses flags, logs in, and streams synthetic log entries at a fixed
// rate for a fixed duration.
func Run() {
	fs := flag.NewFlagSet("logload", flag.ExitOnError)
	addr := fs.String("addr", envOr("SERVER_ADDR", "localhost:50051"), "server address")
	username := fs.String("user", "logbot", "username to log in as")
	password := fs.String("pass", "logbot-password", "password to log in with")
	rate := fs.Int("rate", 200, "log entries per second")
	duration := fs.Duration("duration", 30*time.Second, "how long to keep sending")
	source := fs.String("source", "logload", "value for the entry's source field")
	_ = fs.Parse(os.Args[1:])

	conn, err := grpc.NewClient(*addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("couldn't connect: %v", err)
	}
	defer conn.Close()

	game := pb.NewGameClient(conn)
	logs := pb.NewLogIngestClient(conn)

	token := login(game, *username, *password)

	// One stream carries every entry. Opening a stream per entry, or using a
	// unary call, would pay a connection round trip per log line.
	stream, err := logs.Ingest(context.Background())
	if err != nil {
		log.Fatalf("couldn't open ingest stream: %v", err)
	}

	interval := time.Second / time.Duration(max(*rate, 1))
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	deadline := time.Now().Add(*duration)
	report := time.NewTicker(time.Second)
	defer report.Stop()

	log.Printf("streaming %d entries/sec for %s to %s", *rate, *duration, *addr)

	var sent int64
	for time.Now().Before(deadline) {
		select {
		case <-ticker.C:
			entry := &pb.LogEntry{
				Token:          token,
				Level:          levelFor(sent),
				Message:        fmt.Sprintf("synthetic log line %d", sent),
				Source:         *source,
				LoggedAtUnixMs: time.Now().UnixMilli(),
			}
			// Send blocks when the server stops reading, which is exactly
			// what should happen while the database is down. A stalled
			// counter here is backpressure working, not a bug.
			if err := stream.Send(entry); err != nil {
				log.Fatalf("send failed after %d entries: %v", sent, err)
			}
			sent++

		case <-report.C:
			log.Printf("sent %d entries so far", sent)
		}
	}

	summary, err := stream.CloseAndRecv()
	if err != nil {
		log.Fatalf("closing stream failed after %d entries: %v", sent, err)
	}

	log.Printf("done: client sent %d, server accepted %d, wrote %d, retried %d",
		sent, summary.GetAccepted(), summary.GetWritten(), summary.GetRetries())
	log.Printf("server says: %s", summary.GetMessage())
}

// login registers the account first, ignoring an "already exists" failure so
// the tool can be run repeatedly.
func login(client pb.GameClient, username, password string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := client.Register(ctx, &pb.RegisterRequest{Username: username, Password: password}); err != nil {
		log.Printf("register skipped (%v)", err)
	}

	reply, err := client.Login(ctx, &pb.LoginRequest{Username: username, Password: password})
	if err != nil {
		log.Fatalf("login failed: %v", err)
	}
	return reply.GetToken()
}

func levelFor(n int64) string {
	switch {
	case n%50 == 0:
		return "ERROR"
	case n%10 == 0:
		return "WARN"
	default:
		return "INFO"
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
