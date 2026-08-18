// Package apply executes pending database migrations within isolated transactions,
// ensuring clean rollback on failure and automatic history table tracking.
package apply

import (
	"context"
	"database/sql"
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
	File     *migration.MigrationFile `json:"file"`
	Duration time.Duration            `json:"duration"`
	Error    error                    `json:"error,omitempty"`
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

		start := time.Now()

		content, err := os.ReadFile(file.Path)
		if err != nil {
			migErr := fmt.Errorf("failed to read migration file %q: %w", file.Path, err)
			res.Failed = &MigrationResult{
				File:     file,
				Duration: time.Since(start),
				Error:    migErr,
			}
			return res, migErr
		}

		sqlContent := string(content)
		if IsNonTransactional(sqlContent) {
			migErr := fmt.Errorf("%w: migration %q contains non-transactional statement", ErrNonTransactional, file.Filename)
			res.Failed = &MigrationResult{
				File:     file,
				Duration: time.Since(start),
				Error:    migErr,
			}
			return res, migErr
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			migErr := fmt.Errorf("failed to begin transaction for migration %q: %w", file.Filename, err)
			res.Failed = &MigrationResult{
				File:     file,
				Duration: time.Since(start),
				Error:    migErr,
			}
			return res, migErr
		}

		if strings.TrimSpace(sqlContent) != "" {
			if _, err := tx.ExecContext(ctx, sqlContent); err != nil {
				_ = tx.Rollback()
				migErr := fmt.Errorf("failed to execute migration %q: %w", file.Filename, err)
				res.Failed = &MigrationResult{
					File:     file,
					Duration: time.Since(start),
					Error:    migErr,
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
			res.Failed = &MigrationResult{
				File:     file,
				Duration: time.Since(start),
				Error:    migErr,
			}
			return res, migErr
		}

		if err := tx.Commit(); err != nil {
			_ = tx.Rollback()
			migErr := fmt.Errorf("failed to commit transaction for migration %q: %w", file.Filename, err)
			res.Failed = &MigrationResult{
				File:     file,
				Duration: time.Since(start),
				Error:    migErr,
			}
			return res, migErr
		}

		duration := time.Since(start)
		res.Applied = append(res.Applied, &MigrationResult{
			File:     file,
			Duration: duration,
		})
	}

	return res, nil
}

// FormatResult writes a structured, human-readable summary of the apply execution to w.
func FormatResult(w io.Writer, res *Result) error {
	if res == nil || res.TotalPending == 0 {
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
		if _, err := fmt.Fprintf(w, "    Error: %v\n", res.Failed.Error); err != nil {
			return err
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
