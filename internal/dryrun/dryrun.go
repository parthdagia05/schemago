// Package dryrun executes pending database migrations within a single transaction
// that is guaranteed to be rolled back, enabling dry-run validation of migration scripts
// without persisting changes to the database.
package dryrun

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/parthdagia05/schemago/internal/apply"
	"github.com/parthdagia05/schemago/internal/history"
	"github.com/parthdagia05/schemago/internal/migration"
)

// Result holds the outcome of a dry-run migration execution.
type Result struct {
	Applied      []*apply.MigrationResult `json:"applied"`
	Failed       *apply.MigrationResult   `json:"failed,omitempty"`
	TotalPending int                      `json:"total_pending"`
}

// DryRun executes pending migrations sequentially inside a single database transaction
// that is ALWAYS rolled back at completion or failure. This validates SQL syntax and execution
// path while guaranteeing zero persistent modifications to the database.
func DryRun(ctx context.Context, db apply.TxBeginnerContext, tableName string, pending []*migration.MigrationFile) (*Result, error) {
	if db == nil {
		return nil, apply.ErrNilDB
	}

	res := &Result{
		Applied:      make([]*apply.MigrationResult, 0, len(pending)),
		TotalPending: len(pending),
	}

	if len(pending) == 0 {
		return res, nil
	}

	tableName, err := history.ValidateTableName(tableName)
	if err != nil {
		return nil, err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin dry-run transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	for _, file := range pending {
		if file == nil {
			continue
		}

		start := time.Now()

		content, err := os.ReadFile(file.Path)
		if err != nil {
			migErr := fmt.Errorf("failed to read migration file %q: %w", file.Path, err)
			dur := time.Since(start)
			res.Failed = &apply.MigrationResult{
				File:       file,
				Duration:   dur,
				DurationMs: dur.Milliseconds(),
				Error:      migErr,
				ErrMessage: migErr.Error(),
			}
			return res, migErr
		}

		sqlContent := string(content)
		if apply.IsNonTransactional(sqlContent) {
			migErr := fmt.Errorf("%w: migration %q contains non-transactional statement", apply.ErrNonTransactional, file.Filename)
			dur := time.Since(start)
			res.Failed = &apply.MigrationResult{
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
				migErr := fmt.Errorf("failed to execute statement %d (line %d) in migration %q: %w", stmt.Index, stmt.LineNumber, file.Filename, err)
				dur := time.Since(start)
				res.Failed = &apply.MigrationResult{
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
			migErr := fmt.Errorf("failed to record migration %q in history table: %w", file.Filename, err)
			dur := time.Since(start)
			res.Failed = &apply.MigrationResult{
				File:       file,
				Duration:   dur,
				DurationMs: dur.Milliseconds(),
				Error:      migErr,
				ErrMessage: migErr.Error(),
			}
			return res, migErr
		}

		duration := time.Since(start)
		res.Applied = append(res.Applied, &apply.MigrationResult{
			File:       file,
			Duration:   duration,
			DurationMs: duration.Milliseconds(),
		})
	}

	return res, nil
}

// FormatResult writes a structured, human-readable summary of the dry-run execution to w,
// clearly distinguishing it from a live apply operation.
func FormatResult(w io.Writer, res *Result) error {
	if res == nil || res.TotalPending == 0 || (len(res.Applied) == 0 && res.Failed == nil) {
		_, err := fmt.Fprintln(w, "[DRY-RUN] Nothing to apply. Database is up to date.")
		return err
	}

	total := res.TotalPending
	if total == 1 {
		if _, err := fmt.Fprintf(w, "[DRY-RUN] Simulating 1 pending migration (changes will NOT be committed):\n\n"); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintf(w, "[DRY-RUN] Simulating %d pending migrations (changes will NOT be committed):\n\n", total); err != nil {
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
		if _, err := fmt.Fprintf(w, "    [DRY-RUN] Transaction rolled back cleanly. No changes were made to the database.\n"); err != nil {
			return err
		}
	}

	appliedCount := len(res.Applied)
	if res.Failed == nil {
		if appliedCount == 1 {
			if _, err := fmt.Fprintf(w, "\n[DRY-RUN] Dry-run completed successfully. 1 migration validated (0 changes committed).\n"); err != nil {
				return err
			}
		} else {
			if _, err := fmt.Fprintf(w, "\n[DRY-RUN] Dry-run completed successfully. %d migrations validated (0 changes committed).\n", appliedCount); err != nil {
				return err
			}
		}
	} else {
		if _, err := fmt.Fprintf(w, "\n[DRY-RUN] Dry-run failed. %d/%d migration(s) validated. Execution stopped on failure (0 changes committed).\n", appliedCount, total); err != nil {
			return err
		}
	}

	return nil
}
