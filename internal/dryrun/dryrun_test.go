package dryrun

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/parthdagia05/schemago/internal/apply"
	"github.com/parthdagia05/schemago/internal/history"
	"github.com/parthdagia05/schemago/internal/migration"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	if err := history.EnsureTable(ctx, db, history.DefaultTableName); err != nil {
		t.Fatalf("failed to ensure history table: %v", err)
	}

	return db
}

func createTempMigration(t *testing.T, dir string, version int64, filename string, content string) *migration.MigrationFile {
	t.Helper()
	filePath := filepath.Join(dir, filename)
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp migration file %s: %v", filename, err)
	}

	checksum := migration.ComputeChecksum([]byte(content))
	return &migration.MigrationFile{
		Version:  version,
		Filename: filename,
		Path:     filePath,
		Checksum: checksum,
	}
}

func TestDryRun_NilDB(t *testing.T) {
	_, err := DryRun(context.Background(), nil, history.DefaultTableName, nil)
	if !errors.Is(err, apply.ErrNilDB) {
		t.Fatalf("expected ErrNilDB, got %v", err)
	}
}

func TestDryRun_EmptyPending(t *testing.T) {
	db := setupTestDB(t)
	res, err := DryRun(context.Background(), db, history.DefaultTableName, nil)
	if err != nil {
		t.Fatalf("unexpected error for empty pending: %v", err)
	}
	if res.TotalPending != 0 || len(res.Applied) != 0 || res.Failed != nil {
		t.Fatalf("expected empty result, got %+v", res)
	}
}

func TestDryRun_SuccessfulMigrations_LeavesDBUnchanged(t *testing.T) {
	db := setupTestDB(t)
	tempDir := t.TempDir()

	m1 := createTempMigration(t, tempDir, 1, "0001_create_users.sql", "CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT);")
	m2 := createTempMigration(t, tempDir, 2, "0002_create_posts.sql", "CREATE TABLE posts (id INTEGER PRIMARY KEY, user_id INTEGER);")

	pending := []*migration.MigrationFile{m1, m2}

	res, err := DryRun(context.Background(), db, history.DefaultTableName, pending)
	if err != nil {
		t.Fatalf("DryRun failed: %v", err)
	}

	if len(res.Applied) != 2 {
		t.Fatalf("expected 2 applied migrations in dry-run result, got %d", len(res.Applied))
	}
	if res.Failed != nil {
		t.Fatalf("expected no failed migrations in dry-run result, got %v", res.Failed)
	}

	// Verify tables DO NOT exist in DB (guaranteed unchanged)
	var tableName string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='users';").Scan(&tableName)
	if err == nil {
		t.Fatalf("users table was created in DB during dry-run (should NOT be persisted!)")
	}

	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='posts';").Scan(&tableName)
	if err == nil {
		t.Fatalf("posts table was created in DB during dry-run (should NOT be persisted!)")
	}

	// Verify history records table is EMPTY
	appliedRecords, err := history.GetAppliedMigrations(context.Background(), db, history.DefaultTableName)
	if err != nil {
		t.Fatalf("failed to query history table: %v", err)
	}
	if len(appliedRecords) != 0 {
		t.Fatalf("expected 0 history records after dry-run, got %d", len(appliedRecords))
	}
}

func TestDryRun_SQLSyntaxError_CatchesErrorAndLeavesDBUnchanged(t *testing.T) {
	db := setupTestDB(t)
	tempDir := t.TempDir()

	m1 := createTempMigration(t, tempDir, 1, "0001_create_users.sql", "CREATE TABLE users (id INTEGER PRIMARY KEY);")
	m2 := createTempMigration(t, tempDir, 2, "0002_failing_migration.sql", "INVALID SQL SYNTAX HERE;")

	pending := []*migration.MigrationFile{m1, m2}

	res, err := DryRun(context.Background(), db, history.DefaultTableName, pending)
	if err == nil {
		t.Fatalf("expected DryRun error on failing migration, got nil")
	}

	if len(res.Applied) != 1 {
		t.Fatalf("expected 1 migration in res.Applied before failure, got %d", len(res.Applied))
	}
	if res.Failed == nil || res.Failed.File.Version != 2 {
		t.Fatalf("expected res.Failed to be migration version 2, got %v", res.Failed)
	}

	// Verify m1 was NOT committed to DB despite running before m2 in dry-run
	var tableName string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='users';").Scan(&tableName)
	if err == nil {
		t.Fatalf("migration 1 (users table) was created in DB (dry-run must leave DB unchanged)")
	}

	// Verify history records table is EMPTY
	appliedRecords, err := history.GetAppliedMigrations(context.Background(), db, history.DefaultTableName)
	if err != nil {
		t.Fatalf("failed to query history table: %v", err)
	}
	if len(appliedRecords) != 0 {
		t.Fatalf("expected 0 history records after dry-run failure, got %d", len(appliedRecords))
	}
}

func TestDryRun_NonTransactionalStatement(t *testing.T) {
	db := setupTestDB(t)
	tempDir := t.TempDir()

	m1 := createTempMigration(t, tempDir, 1, "0001_concurrent.sql", "CREATE INDEX CONCURRENTLY idx_users ON users(email);")
	pending := []*migration.MigrationFile{m1}

	res, err := DryRun(context.Background(), db, history.DefaultTableName, pending)
	if err == nil {
		t.Fatalf("expected non-transactional statement error, got nil")
	}
	if !errors.Is(err, apply.ErrNonTransactional) {
		t.Fatalf("expected ErrNonTransactional, got: %v", err)
	}
	if res.Failed == nil || res.Failed.File.Version != 1 {
		t.Fatalf("expected res.Failed to point to migration 1")
	}
}

func TestFormatResult(t *testing.T) {
	t.Run("empty result", func(t *testing.T) {
		var buf bytes.Buffer
		err := FormatResult(&buf, &Result{TotalPending: 0})
		if err != nil {
			t.Fatalf("FormatResult unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "[DRY-RUN] Nothing to apply. Database is up to date.") {
			t.Errorf("unexpected output: %s", buf.String())
		}
	})

	t.Run("successful dry-run report", func(t *testing.T) {
		var buf bytes.Buffer
		res := &Result{
			TotalPending: 2,
			Applied: []*apply.MigrationResult{
				{
					File:     &migration.MigrationFile{Filename: "0001_create_users.sql"},
					Duration: 10 * time.Millisecond,
				},
				{
					File:     &migration.MigrationFile{Filename: "0002_add_email.sql"},
					Duration: 5 * time.Millisecond,
				},
			},
		}
		err := FormatResult(&buf, res)
		if err != nil {
			t.Fatalf("FormatResult unexpected error: %v", err)
		}
		output := buf.String()
		if !strings.Contains(output, "[DRY-RUN] Simulating 2 pending migrations") ||
			!strings.Contains(output, "✓ 0001_create_users.sql") ||
			!strings.Contains(output, "✓ 0002_add_email.sql") ||
			!strings.Contains(output, "[DRY-RUN] Dry-run completed successfully. 2 migrations validated (0 changes committed).") {
			t.Errorf("unexpected output:\n%s", output)
		}
	})

	t.Run("failing dry-run report", func(t *testing.T) {
		var buf bytes.Buffer
		res := &Result{
			TotalPending: 2,
			Applied: []*apply.MigrationResult{
				{
					File:     &migration.MigrationFile{Filename: "0001_create_users.sql"},
					Duration: 10 * time.Millisecond,
				},
			},
			Failed: &apply.MigrationResult{
				File:     &migration.MigrationFile{Filename: "0002_failing.sql"},
				Duration: 2 * time.Millisecond,
				Error:    errors.New("syntax error"),
			},
		}
		err := FormatResult(&buf, res)
		if err != nil {
			t.Fatalf("FormatResult unexpected error: %v", err)
		}
		output := buf.String()
		if !strings.Contains(output, "[DRY-RUN] Simulating 2 pending migrations") ||
			!strings.Contains(output, "✓ 0001_create_users.sql") ||
			!strings.Contains(output, "✗ 0002_failing.sql FAILED") ||
			!strings.Contains(output, "[DRY-RUN] Transaction rolled back cleanly. No changes were made to the database.") ||
			!strings.Contains(output, "[DRY-RUN] Dry-run failed. 1/2 migration(s) validated. Execution stopped on failure (0 changes committed).") {
			t.Errorf("unexpected output:\n%s", output)
		}
	})
}
