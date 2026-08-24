package postgres

import (
	"strings"
	"testing"
	"time"

	"myGuy/internal/domain"
)

// buildInsert assembles SQL by hand, so it is worth pinning down exactly what
// it emits: an off-by-one in the placeholder numbering would bind the wrong
// column values to the wrong rows.

func TestBuildInsertNumbersPlaceholdersPerRow(t *testing.T) {
	at := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	entries := []domain.LogEntry{
		{Username: "arash", Level: "INFO", Message: "first", Source: "test", LoggedAt: at},
		{Username: "sara", Level: "WARN", Message: "second", Source: "test", LoggedAt: at},
	}

	query, args := buildInsert(entries)

	const wantValues = "VALUES ($1,$2,$3,$4,$5),($6,$7,$8,$9,$10)"
	if !strings.HasSuffix(query, wantValues) {
		t.Errorf("query = %q, want it to end with %q", query, wantValues)
	}
	if !strings.HasPrefix(query, "INSERT INTO logs (username, level, message, source, logged_at) ") {
		t.Errorf("unexpected query prefix: %q", query)
	}

	if len(args) != len(entries)*logColumns {
		t.Fatalf("got %d args, want %d", len(args), len(entries)*logColumns)
	}

	// Arguments must line up with their placeholders, in row order.
	want := []any{
		"arash", "INFO", "first", "test", at,
		"sara", "WARN", "second", "test", at,
	}
	for i, w := range want {
		if args[i] != w {
			t.Errorf("arg %d = %v, want %v", i, args[i], w)
		}
	}
}

func TestBuildInsertSingleRow(t *testing.T) {
	entries := []domain.LogEntry{
		{Username: "arash", Level: "ERROR", Message: "boom", Source: "test", LoggedAt: time.Now().UTC()},
	}

	query, args := buildInsert(entries)

	if !strings.HasSuffix(query, "VALUES ($1,$2,$3,$4,$5)") {
		t.Errorf("query = %q", query)
	}
	if len(args) != logColumns {
		t.Errorf("got %d args, want %d", len(args), logColumns)
	}
}

// The placeholder count must stay within what Postgres accepts, which is what
// the chunking in BulkInsert is guarding.
func TestMaxRowsPerStatementStaysUnderPostgresLimit(t *testing.T) {
	if maxRowsPerStatement*logColumns > maxPlaceholders {
		t.Fatalf("%d rows x %d columns = %d placeholders, over the %d limit",
			maxRowsPerStatement, logColumns, maxRowsPerStatement*logColumns, maxPlaceholders)
	}
}

// A batch at the default size must fit in one statement, so the common path
// never pays for a transaction.
func TestDefaultBatchSizeFitsInOneStatement(t *testing.T) {
	const defaultBatchSize = 500
	if defaultBatchSize > maxRowsPerStatement {
		t.Fatalf("default batch of %d exceeds %d rows per statement", defaultBatchSize, maxRowsPerStatement)
	}
}
