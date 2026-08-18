package apply

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

func TestApply_NilDB(t *testing.T) {
	_, err := Apply(context.Background(), nil, history.DefaultTableName, nil)
	if !errors.Is(err, ErrNilDB) {
		t.Fatalf("expected ErrNilDB, got %v", err)
	}
}

func TestApply_EmptyPending(t *testing.T) {
	db := setupTestDB(t)
	res, err := Apply(context.Background(), db, history.DefaultTableName, nil)
	if err != nil {
		t.Fatalf("unexpected error for empty pending: %v", err)
	}
	if res.TotalPending != 0 || len(res.Applied) != 0 || res.Failed != nil {
		t.Fatalf("expected empty result, got %+v", res)
	}
}

func TestApply_SuccessfulMigrations(t *testing.T) {
	db := setupTestDB(t)
	tempDir := t.TempDir()

	m1 := createTempMigration(t, tempDir, 1, "0001_create_users.sql", "CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT);")
	m2 := createTempMigration(t, tempDir, 2, "0002_create_posts.sql", "CREATE TABLE posts (id INTEGER PRIMARY KEY, user_id INTEGER);")

	pending := []*migration.MigrationFile{m1, m2}

	res, err := Apply(context.Background(), db, history.DefaultTableName, pending)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	if len(res.Applied) != 2 {
		t.Fatalf("expected 2 applied migrations, got %d", len(res.Applied))
	}
	if res.Failed != nil {
		t.Fatalf("expected no failed migrations, got %v", res.Failed)
	}

	// Verify tables exist in DB
	var tableName string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='users';").Scan(&tableName)
	if err != nil || tableName != "users" {
		t.Fatalf("users table was not created in DB")
	}

	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='posts';").Scan(&tableName)
	if err != nil || tableName != "posts" {
		t.Fatalf("posts table was not created in DB")
	}

	// Verify history records
	appliedRecords, err := history.GetAppliedMigrations(context.Background(), db, history.DefaultTableName)
	if err != nil {
		t.Fatalf("failed to query history table: %v", err)
	}
	if len(appliedRecords) != 2 {
		t.Fatalf("expected 2 history records, got %d", len(appliedRecords))
	}
	if appliedRecords[0].Version != 1 || appliedRecords[1].Version != 2 {
		t.Errorf("unexpected history record versions: %d, %d", appliedRecords[0].Version, appliedRecords[1].Version)
	}
}

func TestApply_MidMigrationFailureAndRollback(t *testing.T) {
	db := setupTestDB(t)
	tempDir := t.TempDir()

	m1 := createTempMigration(t, tempDir, 1, "0001_create_users.sql", "CREATE TABLE users (id INTEGER PRIMARY KEY);")
	m2 := createTempMigration(t, tempDir, 2, "0002_failing_migration.sql", "INVALID SQL SYNTAX HERE;")
	m3 := createTempMigration(t, tempDir, 3, "0003_create_posts.sql", "CREATE TABLE posts (id INTEGER PRIMARY KEY);")

	pending := []*migration.MigrationFile{m1, m2, m3}

	res, err := Apply(context.Background(), db, history.DefaultTableName, pending)
	if err == nil {
		t.Fatalf("expected Apply error on failing migration, got nil")
	}

	if len(res.Applied) != 1 {
		t.Fatalf("expected 1 applied migration before failure, got %d", len(res.Applied))
	}
	if res.Applied[0].File.Version != 1 {
		t.Errorf("expected migration 1 to be applied, got version %d", res.Applied[0].File.Version)
	}
	if res.Failed == nil || res.Failed.File.Version != 2 {
		t.Fatalf("expected res.Failed to be migration version 2, got %v", res.Failed)
	}

	// Verify m1 committed
	var tableName string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='users';").Scan(&tableName)
	if err != nil || tableName != "users" {
		t.Fatalf("migration 1 (users table) should be committed")
	}

	// Verify m2 was rolled back (no partial state / invalid tables created)
	// Verify m3 was NOT executed
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='posts';").Scan(&tableName)
	if err == nil {
		t.Fatalf("migration 3 (posts table) should not exist in DB")
	}

	// Verify history table contains ONLY migration 1
	records, err := history.GetAppliedMigrations(context.Background(), db, history.DefaultTableName)
	if err != nil {
		t.Fatalf("failed to query history records: %v", err)
	}
	if len(records) != 1 || records[0].Version != 1 {
		t.Fatalf("expected history table to contain only migration 1, got %d records", len(records))
	}

	// Verify re-running after fixing m2 is safe!
	m2Fixed := createTempMigration(t, tempDir, 2, "0002_failing_migration.sql", "CREATE TABLE categories (id INTEGER PRIMARY KEY);")
	remainingPending := []*migration.MigrationFile{m2Fixed, m3}

	resFix, errFix := Apply(context.Background(), db, history.DefaultTableName, remainingPending)
	if errFix != nil {
		t.Fatalf("re-running Apply after fix failed: %v", errFix)
	}
	if len(resFix.Applied) != 2 {
		t.Fatalf("expected 2 applied migrations on re-run, got %d", len(resFix.Applied))
	}
}

func TestApply_NonTransactionalStatement(t *testing.T) {
	db := setupTestDB(t)
	tempDir := t.TempDir()

	m1 := createTempMigration(t, tempDir, 1, "0001_concurrent.sql", "CREATE INDEX CONCURRENTLY idx_users ON users(email);")
	pending := []*migration.MigrationFile{m1}

	res, err := Apply(context.Background(), db, history.DefaultTableName, pending)
	if err == nil {
		t.Fatalf("expected non-transactional statement error, got nil")
	}
	if !errors.Is(err, ErrNonTransactional) {
		t.Fatalf("expected ErrNonTransactional, got: %v", err)
	}
	if res.Failed == nil || res.Failed.File.Version != 1 {
		t.Fatalf("expected res.Failed to point to migration 1")
	}
}

func TestIsNonTransactional(t *testing.T) {
	tests := []struct {
		sql  string
		want bool
	}{
		{"CREATE TABLE foo (id INT);", false},
		{"CREATE INDEX idx_foo ON foo(id);", false},
		{"CREATE INDEX CONCURRENTLY idx_foo ON foo(id);", true},
		{"drop index concurrently idx_foo;", true},
		{"VACUUM FULL;", true},
		{"REINDEX TABLE foo;", true},
		{"CREATE DATABASE testdb;", true},
		{"DROP DATABASE testdb;", true},
	}

	for _, tt := range tests {
		got := IsNonTransactional(tt.sql)
		if got != tt.want {
			t.Errorf("IsNonTransactional(%q) = %v, want %v", tt.sql, got, tt.want)
		}
	}
}

func TestFormatResult(t *testing.T) {
	t.Run("empty result", func(t *testing.T) {
		var buf bytes.Buffer
		err := FormatResult(&buf, &Result{TotalPending: 0})
		if err != nil {
			t.Fatalf("FormatResult unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "Nothing to apply. Database is up to date.") {
			t.Errorf("unexpected output: %s", buf.String())
		}
	})

	t.Run("successful apply report", func(t *testing.T) {
		var buf bytes.Buffer
		res := &Result{
			TotalPending: 2,
			Applied: []*MigrationResult{
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
		if !strings.Contains(output, "Applying 2 pending migrations:") ||
			!strings.Contains(output, "✓ 0001_create_users.sql") ||
			!strings.Contains(output, "✓ 0002_add_email.sql") ||
			!strings.Contains(output, "Successfully applied 2 migrations.") {
			t.Errorf("unexpected output:\n%s", output)
		}
	})

	t.Run("failing apply report", func(t *testing.T) {
		var buf bytes.Buffer
		res := &Result{
			TotalPending: 2,
			Applied: []*MigrationResult{
				{
					File:     &migration.MigrationFile{Filename: "0001_create_users.sql"},
					Duration: 10 * time.Millisecond,
				},
			},
			Failed: &MigrationResult{
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
		if !strings.Contains(output, "Applying 2 pending migrations:") ||
			!strings.Contains(output, "✓ 0001_create_users.sql") ||
			!strings.Contains(output, "✗ 0002_failing.sql FAILED") ||
			!strings.Contains(output, "Migration rolled back cleanly.") ||
			!strings.Contains(output, "Applied 1/2 migration(s). Execution stopped on failure.") {
			t.Errorf("unexpected output:\n%s", output)
		}
	})
}
