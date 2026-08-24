package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"myGuy/internal/domain"
)

// logColumns is how many placeholders each row contributes to the statement.
const logColumns = 5

// maxPlaceholders is Postgres's hard limit on bind parameters in a single
// statement (65535). Batches large enough to exceed it are split, which in
// practice never happens at the default batch size of 500 (2500 parameters)
// but keeps an aggressive configuration from producing a confusing driver
// error.
const maxPlaceholders = 65535

// maxRowsPerStatement is how many log rows fit in one statement.
const maxRowsPerStatement = maxPlaceholders / logColumns

// LogRepository implements domain.LogRepository against Postgres.
type LogRepository struct {
	db *sql.DB
}

func NewLogRepository(db *sql.DB) *LogRepository {
	return &LogRepository{db: db}
}

var _ domain.LogRepository = (*LogRepository)(nil)

// BulkInsert writes every entry using multi-row INSERT statements, so a
// batch of 500 costs one network round trip instead of 500. When a batch is
// large enough to need more than one statement, the statements run inside a
// transaction so the batch stays all-or-nothing and a retry cannot duplicate
// rows that already landed.
func (r *LogRepository) BulkInsert(ctx context.Context, entries []domain.LogEntry) error {
	if len(entries) == 0 {
		return nil
	}

	if len(entries) <= maxRowsPerStatement {
		// A single INSERT is already atomic, so no explicit transaction is
		// needed for the common case.
		query, args := buildInsert(entries)
		if _, err := r.db.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("bulk insert %d logs: %w", len(entries), err)
		}
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin log insert transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op once the tx is committed

	for start := 0; start < len(entries); start += maxRowsPerStatement {
		end := min(start+maxRowsPerStatement, len(entries))
		query, args := buildInsert(entries[start:end])
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("bulk insert logs [%d:%d]: %w", start, end, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit log insert: %w", err)
	}
	return nil
}

// buildInsert assembles one multi-row INSERT: the values clause becomes
// ($1,$2,$3,$4,$5),($6,$7,$8,$9,$10),... with the arguments in matching
// order. The column values are still bound parameters, never interpolated
// into the SQL text, so this is not a SQL-injection vector.
func buildInsert(entries []domain.LogEntry) (string, []any) {
	var sb strings.Builder
	sb.WriteString("INSERT INTO logs (username, level, message, source, logged_at) VALUES ")

	args := make([]any, 0, len(entries)*logColumns)
	for i, e := range entries {
		if i > 0 {
			sb.WriteByte(',')
		}
		base := i * logColumns
		sb.WriteByte('(')
		for c := 1; c <= logColumns; c++ {
			if c > 1 {
				sb.WriteByte(',')
			}
			sb.WriteByte('$')
			sb.WriteString(strconv.Itoa(base + c))
		}
		sb.WriteByte(')')

		args = append(args, e.Username, e.Level, e.Message, e.Source, e.LoggedAt)
	}
	return sb.String(), args
}
