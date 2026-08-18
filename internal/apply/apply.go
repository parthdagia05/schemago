// Package apply executes pending database migrations within isolated transactions,
// ensuring clean rollback on failure and automatic history table tracking.
package apply

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/parthdagia05/schemago/internal/history"
	"github.com/parthdagia05/schemago/internal/migration"
)

var (
	// ErrNilDB indicates an uninitialized database handle was passed to Apply.
	ErrNilDB = errors.New("database connection handle is nil")

	// ErrNonTransactional indicates that a migration file contains statements that cannot run inside a transaction block.
	ErrNonTransactional = errors.New("statement cannot run inside a transaction block")
)

// nonTransactionalRegex matches SQL statements known to fail within transaction blocks in PostgreSQL.
var nonTransactionalRegex = regexp.MustCompile(`(?i)\b(CREATE\s+INDEX\s+CONCURRENTLY|DROP\s+INDEX\s+CONCURRENTLY|VACUUM|REINDEX|CREATE\s+DATABASE|DROP\s+DATABASE)\b`)

// TxBeginner abstracts transaction initiation for database connections (*sql.DB).
type TxBeginner interface {
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

// TxBeginnerContext abstracts context-aware transaction initiation.
type TxBeginnerContext interface {
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

// MigrationResult holds execution metrics and status for an individual migration.
type MigrationResult struct {
	File         *migration.MigrationFile `json:"file"`
	Duration     time.Duration            `json:"-"`
	DurationMs   int64                    `json:"duration_ms"`
	Error        error                    `json:"-"`
	ErrMessage   string                   `json:"error,omitempty"`
	StatementIdx int                      `json:"statement_index,omitempty"`
	LineNumber   int                      `json:"line_number,omitempty"`
	StatementSQL string                   `json:"statement_sql,omitempty"`
}

type migrationResultJSON struct {
	File         *migration.MigrationFile `json:"file"`
	DurationMs   int64                    `json:"duration_ms"`
	Error        string                   `json:"error,omitempty"`
	StatementIdx int                      `json:"statement_index,omitempty"`
	LineNumber   int                      `json:"line_number,omitempty"`
	StatementSQL string                   `json:"statement_sql,omitempty"`
}

// MarshalJSON provides custom JSON serialization for MigrationResult.
func (m *MigrationResult) MarshalJSON() ([]byte, error) {
	var errStr string
	if m.Error != nil {
		errStr = m.Error.Error()
	} else if m.ErrMessage != "" {
		errStr = m.ErrMessage
	}

	durMs := m.DurationMs
	if durMs == 0 && m.Duration > 0 {
		durMs = m.Duration.Milliseconds()
	}

	return json.Marshal(migrationResultJSON{
		File:         m.File,
		DurationMs:   durMs,
		Error:        errStr,
		StatementIdx: m.StatementIdx,
		LineNumber:   m.LineNumber,
		StatementSQL: m.StatementSQL,
	})
}

// Result holds the overall outcome of an apply operation.
type Result struct {
	Applied      []*MigrationResult `json:"applied"`
	Failed       *MigrationResult   `json:"failed,omitempty"`
	TotalPending int                `json:"total_pending"`
}

// IsNonTransactional checks if SQL content contains commands prohibited inside transaction blocks.
func IsNonTransactional(sqlContent string) bool {
	return nonTransactionalRegex.MatchString(sqlContent)
}

// Apply executes pending migrations sequentially, each wrapped in its own database transaction.
// If a migration fails, its transaction is rolled back and execution halts immediately.
func Apply(ctx context.Context, db TxBeginnerContext, tableName string, pending []*migration.MigrationFile) (*Result, error) {
	if db == nil {
		return nil, ErrNilDB
	}

	res := &Result{
		Applied:      make([]*MigrationResult, 0, len(pending)),
		TotalPending: len(pending),
	}

	if len(pending) == 0 {
		return res, nil
	}

	tableName, err := history.ValidateTableName(tableName)
	if err != nil {
		return nil, err
	}

	for _, file := range pending {
		if file == nil {
			continue
		}

		if q, ok := db.(history.Execer); ok {
			applied, err := history.GetAppliedMigrations(ctx, q, tableName)
			if err == nil {
				alreadyApplied := false
				for _, app := range applied {
					if app.Version == file.Version {
						alreadyApplied = true
						break
					}
				}
				if alreadyApplied {
					continue
				}
			}
		}

		start := time.Now()

		content, err := os.ReadFile(file.Path)
		if err != nil {
			migErr := fmt.Errorf("failed to read migration file %q: %w", file.Path, err)
			dur := time.Since(start)
			res.Failed = &MigrationResult{
				File:       file,
				Duration:   dur,
				DurationMs: dur.Milliseconds(),
				Error:      migErr,
				ErrMessage: migErr.Error(),
			}
			return res, migErr
		}

		sqlContent := string(content)
		if IsNonTransactional(sqlContent) {
			migErr := fmt.Errorf("%w: migration %q contains non-transactional statement", ErrNonTransactional, file.Filename)
			dur := time.Since(start)
			res.Failed = &MigrationResult{
				File:       file,
				Duration:   dur,
				DurationMs: dur.Milliseconds(),
				Error:      migErr,
				ErrMessage: migErr.Error(),
			}
			return res, migErr
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			migErr := fmt.Errorf("failed to begin transaction for migration %q: %w", file.Filename, err)
			dur := time.Since(start)
			res.Failed = &MigrationResult{
				File:       file,
				Duration:   dur,
				DurationMs: dur.Milliseconds(),
				Error:      migErr,
				ErrMessage: migErr.Error(),
			}
			return res, migErr
		}

		stmts := migration.SplitStatements(sqlContent)
		for _, stmt := range stmts {
			if strings.TrimSpace(stmt.SQL) == "" {
				continue
			}
			if _, err := tx.ExecContext(ctx, stmt.SQL); err != nil {
				_ = tx.Rollback()
				migErr := fmt.Errorf("failed to execute statement %d (line %d) in migration %q: %w", stmt.Index, stmt.LineNumber, file.Filename, err)
				dur := time.Since(start)
				res.Failed = &MigrationResult{
					File:         file,
					Duration:     dur,
					DurationMs:   dur.Milliseconds(),
					Error:        migErr,
					ErrMessage:   migErr.Error(),
					StatementIdx: stmt.Index,
					LineNumber:   stmt.LineNumber,
					StatementSQL: strings.TrimSpace(stmt.SQL),
				}
				return res, migErr
			}
		}

		checksum := file.Checksum
		if checksum == "" && file.Path != "" {
			computed, err := migration.ComputeFileChecksum(file.Path)
			if err == nil {
				checksum = computed
			}
		}

		record := &history.AppliedMigration{
			Version:   file.Version,
			Name:      file.Filename,
			AppliedAt: time.Now().UTC(),
			Checksum:  checksum,
		}

		if err := history.RecordMigration(ctx, tx, tableName, record); err != nil {
			_ = tx.Rollback()
			migErr := fmt.Errorf("failed to record migration %q in history table: %w", file.Filename, err)
			dur := time.Since(start)
			res.Failed = &MigrationResult{
				File:       file,
				Duration:   dur,
				DurationMs: dur.Milliseconds(),
				Error:      migErr,
				ErrMessage: migErr.Error(),
			}
			return res, migErr
		}

		if err := tx.Commit(); err != nil {
			_ = tx.Rollback()
			migErr := fmt.Errorf("failed to commit transaction for migration %q: %w", file.Filename, err)
			dur := time.Since(start)
			res.Failed = &MigrationResult{
				File:       file,
				Duration:   dur,
				DurationMs: dur.Milliseconds(),
				Error:      migErr,
				ErrMessage: migErr.Error(),
			}
			return res, migErr
		}

		duration := time.Since(start)
		res.Applied = append(res.Applied, &MigrationResult{
			File:       file,
			Duration:   duration,
			DurationMs: duration.Milliseconds(),
		})
	}

	return res, nil
}

// FormatResult writes a structured, human-readable summary of the apply execution to w.
func FormatResult(w io.Writer, res *Result) error {
	if res == nil || res.TotalPending == 0 || (len(res.Applied) == 0 && res.Failed == nil) {
		_, err := fmt.Fprintln(w, "Nothing to apply. Database is up to date.")
		return err
	}

	total := res.TotalPending
	if total == 1 {
		if _, err := fmt.Fprintf(w, "Applying 1 pending migration:\n\n"); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintf(w, "Applying %d pending migrations:\n\n", total); err != nil {
			return err
		}
	}

	for _, item := range res.Applied {
		durStr := item.Duration.Round(time.Millisecond).String()
		if item.Duration < time.Millisecond {
			durStr = "<1ms"
		}
		if _, err := fmt.Fprintf(w, "  ✓ %s (%s)\n", item.File.Filename, durStr); err != nil {
			return err
		}
	}

	if res.Failed != nil {
		durStr := res.Failed.Duration.Round(time.Millisecond).String()
		if res.Failed.Duration < time.Millisecond {
			durStr = "<1ms"
		}
		if _, err := fmt.Fprintf(w, "  ✗ %s FAILED (%s)\n", res.Failed.File.Filename, durStr); err != nil {
			return err
		}
		if res.Failed.StatementIdx > 0 && res.Failed.LineNumber > 0 {
			if _, err := fmt.Fprintf(w, "    Error at statement %d (line %d): %v\n", res.Failed.StatementIdx, res.Failed.LineNumber, res.Failed.Error); err != nil {
				return err
			}
		} else {
			if _, err := fmt.Fprintf(w, "    Error: %v\n", res.Failed.Error); err != nil {
				return err
			}
		}
		if res.Failed.StatementSQL != "" {
			firstLine := strings.Split(res.Failed.StatementSQL, "\n")[0]
			if len(firstLine) > 80 {
				firstLine = firstLine[:77] + "..."
			}
			if _, err := fmt.Fprintf(w, "    Statement: %s\n", firstLine); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, "    Migration rolled back cleanly.\n"); err != nil {
			return err
		}
	}

	appliedCount := len(res.Applied)
	if res.Failed == nil {
		if appliedCount == 1 {
			if _, err := fmt.Fprintf(w, "\nSuccessfully applied 1 migration.\n"); err != nil {
				return err
			}
		} else {
			if _, err := fmt.Fprintf(w, "\nSuccessfully applied %d migrations.\n", appliedCount); err != nil {
				return err
			}
		}
	} else {
		if _, err := fmt.Fprintf(w, "\nApplied %d/%d migration(s). Execution stopped on failure.\n", appliedCount, total); err != nil {
			return err
		}
	}

	return nil
}
