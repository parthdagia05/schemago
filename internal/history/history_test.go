package history

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/parthdagia05/schemago/internal/migration"
)

func TestValidateTableName(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantTable string
		wantErr   error
	}{
		{
			name:      "empty input uses default table name",
			input:     "",
			wantTable: DefaultTableName,
			wantErr:   nil,
		},
		{
			name:      "valid custom table name",
			input:     "custom_schema_history",
			wantTable: "custom_schema_history",
			wantErr:   nil,
		},
		{
			name:    "invalid table name with spaces",
			input:   "schema history",
			wantErr: ErrInvalidTableName,
		},
		{
			name:    "invalid table name with SQL injection characters",
			input:   "schemago_migrations; DROP TABLE users;--",
			wantErr: ErrInvalidTableName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateTableName(tt.input)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("error = %v, wantErr = %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantTable {
				t.Errorf("table = %q, want %q", got, tt.wantTable)
			}
		})
	}
}

func TestComputePending(t *testing.T) {
	disc1 := &migration.MigrationFile{
		Version:     1,
		RawVersion:  "0001",
		Description: "create_users",
		Filename:    "0001_create_users.sql",
		Checksum:    "checksum111",
	}
	disc2 := &migration.MigrationFile{
		Version:     2,
		RawVersion:  "0002",
		Description: "add_email",
		Filename:    "0002_add_email.sql",
		Checksum:    "checksum222",
	}
	disc3 := &migration.MigrationFile{
		Version:     3,
		RawVersion:  "0003",
		Description: "add_indexes",
		Filename:    "0003_add_indexes.sql",
		Checksum:    "checksum333",
	}

	t.Run("all migrations pending when none applied", func(t *testing.T) {
		discovered := []*migration.MigrationFile{disc1, disc2, disc3}
		var applied []*AppliedMigration

		pending, err := ComputePending(discovered, applied)
		if err != nil {
			t.Fatalf("ComputePending error: %v", err)
		}
		if len(pending) != 3 {
			t.Fatalf("expected 3 pending migrations, got %d", len(pending))
		}
		if pending[0].Version != 1 || pending[1].Version != 2 || pending[2].Version != 3 {
			t.Errorf("unexpected pending order: %v, %v, %v", pending[0].Version, pending[1].Version, pending[2].Version)
		}
	})

	t.Run("partial applied returns remaining pending", func(t *testing.T) {
		discovered := []*migration.MigrationFile{disc1, disc2, disc3}
		applied := []*AppliedMigration{
			{
				Version:   1,
				Name:      "0001_create_users.sql",
				AppliedAt: time.Now(),
				Checksum:  "checksum111",
			},
		}

		pending, err := ComputePending(discovered, applied)
		if err != nil {
			t.Fatalf("ComputePending error: %v", err)
		}
		if len(pending) != 2 {
			t.Fatalf("expected 2 pending migrations, got %d", len(pending))
		}
		if pending[0].Version != 2 || pending[1].Version != 3 {
			t.Errorf("unexpected pending items: %d, %d", pending[0].Version, pending[1].Version)
		}
	})

	t.Run("no pending when all migrations applied", func(t *testing.T) {
		discovered := []*migration.MigrationFile{disc1, disc2}
		applied := []*AppliedMigration{
			{Version: 1, Name: "0001_create_users.sql", Checksum: "checksum111"},
			{Version: 2, Name: "0002_add_email.sql", Checksum: "checksum222"},
		}

		pending, err := ComputePending(discovered, applied)
		if err != nil {
			t.Fatalf("ComputePending error: %v", err)
		}
		if len(pending) != 0 {
			t.Fatalf("expected 0 pending, got %d", len(pending))
		}
	})

	t.Run("returns ErrChecksumMismatch when file modified after application", func(t *testing.T) {
		modifiedDisc1 := &migration.MigrationFile{
			Version:     1,
			RawVersion:  "0001",
			Description: "create_users",
			Filename:    "0001_create_users.sql",
			Checksum:    "checksum_MODIFIED",
		}
		discovered := []*migration.MigrationFile{modifiedDisc1}
		applied := []*AppliedMigration{
			{Version: 1, Name: "0001_create_users.sql", Checksum: "checksum111"},
		}

		_, err := ComputePending(discovered, applied)
		if err == nil {
			t.Fatalf("expected checksum mismatch error, got nil")
		}
		if !errors.Is(err, ErrChecksumMismatch) {
			t.Fatalf("expected ErrChecksumMismatch, got: %v", err)
		}
	})

	t.Run("returns ErrMissingMigrationFile when applied record missing from disk", func(t *testing.T) {
		discovered := []*migration.MigrationFile{disc2}
		applied := []*AppliedMigration{
			{Version: 1, Name: "0001_create_users.sql", Checksum: "checksum111"},
		}

		_, err := ComputePending(discovered, applied)
		if err == nil {
			t.Fatalf("expected missing migration error, got nil")
		}
		if !errors.Is(err, ErrMissingMigrationFile) {
			t.Fatalf("expected ErrMissingMigrationFile, got: %v", err)
		}
	})
}

func TestComputePending_WithDiskFiles(t *testing.T) {
	tempDir := t.TempDir()
	file1Path := filepath.Join(tempDir, "0001_init.sql")
	content1 := []byte("CREATE TABLE foo (id INT);")
	if err := os.WriteFile(file1Path, content1, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	checksum1 := migration.ComputeChecksum(content1)

	disc1 := &migration.MigrationFile{
		Version:     1,
		RawVersion:  "0001",
		Description: "init",
		Filename:    "0001_init.sql",
		Path:        file1Path,
		Checksum:    "", // testing auto calculation from path
	}

	applied := []*AppliedMigration{
		{Version: 1, Name: "0001_init.sql", Checksum: checksum1},
	}

	pending, err := ComputePending([]*migration.MigrationFile{disc1}, applied)
	if err != nil {
		t.Fatalf("ComputePending error: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected 0 pending, got %d", len(pending))
	}
}

func TestRecordMigration_NilRecord(t *testing.T) {
	err := RecordMigration(context.Background(), nil, DefaultTableName, nil)
	if err == nil {
		t.Fatal("expected error when recording nil record, got nil")
	}
}

type mockExecer struct {
	execErr  error
	queryErr error
	rows     *sql.Rows
}

func (m *mockExecer) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return nil, m.execErr
}

func (m *mockExecer) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return m.rows, m.queryErr
}

func (m *mockExecer) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return nil
}

func TestEnsureTable(t *testing.T) {
	t.Run("returns error on invalid table name", func(t *testing.T) {
		err := EnsureTable(context.Background(), &mockExecer{}, "invalid table")
		if err == nil || !errors.Is(err, ErrInvalidTableName) {
			t.Fatalf("expected ErrInvalidTableName, got %v", err)
		}
	})

	t.Run("returns error on exec failure", func(t *testing.T) {
		execErr := errors.New("db error")
		mock := &mockExecer{execErr: execErr}
		err := EnsureTable(context.Background(), mock, DefaultTableName)
		if err == nil || !errors.Is(err, execErr) {
			t.Fatalf("expected db error, got %v", err)
		}
	})

	t.Run("success on valid table name", func(t *testing.T) {
		mock := &mockExecer{}
		err := EnsureTable(context.Background(), mock, DefaultTableName)
		if err != nil {
			t.Fatalf("EnsureTable unexpected error: %v", err)
		}
	})
}

func TestRecordMigration(t *testing.T) {
	t.Run("returns error on invalid table name", func(t *testing.T) {
		rec := &AppliedMigration{Version: 1, Name: "0001_init.sql", Checksum: "abc"}
		err := RecordMigration(context.Background(), &mockExecer{}, "bad name!", rec)
		if err == nil || !errors.Is(err, ErrInvalidTableName) {
			t.Fatalf("expected ErrInvalidTableName, got %v", err)
		}
	})

	t.Run("returns error on exec failure", func(t *testing.T) {
		execErr := errors.New("write failure")
		mock := &mockExecer{execErr: execErr}
		rec := &AppliedMigration{Version: 1, Name: "0001_init.sql", Checksum: "abc"}
		err := RecordMigration(context.Background(), mock, DefaultTableName, rec)
		if err == nil || !errors.Is(err, execErr) {
			t.Fatalf("expected exec error, got %v", err)
		}
	})

	t.Run("success on valid record", func(t *testing.T) {
		mock := &mockExecer{}
		rec := &AppliedMigration{Version: 1, Name: "0001_init.sql", Checksum: "abc"}
		err := RecordMigration(context.Background(), mock, DefaultTableName, rec)
		if err != nil {
			t.Fatalf("RecordMigration unexpected error: %v", err)
		}
	})
}

func TestGetAppliedMigrations(t *testing.T) {
	t.Run("returns error on invalid table name", func(t *testing.T) {
		_, err := GetAppliedMigrations(context.Background(), &mockExecer{}, "bad table!")
		if err == nil || !errors.Is(err, ErrInvalidTableName) {
			t.Fatalf("expected ErrInvalidTableName, got %v", err)
		}
	})

	t.Run("returns error on query failure", func(t *testing.T) {
		queryErr := errors.New("read failure")
		mock := &mockExecer{queryErr: queryErr}
		_, err := GetAppliedMigrations(context.Background(), mock, DefaultTableName)
		if err == nil || !errors.Is(err, queryErr) {
			t.Fatalf("expected query error, got %v", err)
		}
	})
}
