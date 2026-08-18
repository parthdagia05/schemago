package plan

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/parthdagia05/schemago/internal/migration"
)

func TestBuildPlan(t *testing.T) {
	disc1 := &migration.MigrationFile{Version: 1, Filename: "0001_create_users.sql"}
	disc2 := &migration.MigrationFile{Version: 2, Filename: "0002_add_index.sql"}

	p := BuildPlan([]*migration.MigrationFile{disc1, disc2})
	if len(p.Pending) != 2 {
		t.Fatalf("expected 2 pending migrations in plan, got %d", len(p.Pending))
	}
	if p.Pending[0].Filename != "0001_create_users.sql" || p.Pending[1].Filename != "0002_add_index.sql" {
		t.Errorf("unexpected pending items order in plan")
	}
}

func TestFormatPlanEmpty(t *testing.T) {
	buf := &bytes.Buffer{}
	err := FormatPlan(buf, nil, Options{})
	if err != nil {
		t.Fatalf("unexpected error formatting empty plan: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Nothing to apply") || !strings.Contains(out, "Database is up to date") {
		t.Errorf("expected clear nothing to apply message, got: %q", out)
	}
}

func TestFormatPlanWithPending(t *testing.T) {
	disc1 := &migration.MigrationFile{Version: 1, Filename: "0001_create_users.sql"}
	disc2 := &migration.MigrationFile{Version: 2, Filename: "0002_add_index.sql"}
	p := BuildPlan([]*migration.MigrationFile{disc1, disc2})

	buf := &bytes.Buffer{}
	err := FormatPlan(buf, p, Options{ShowSQL: false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Found 2 pending migrations to apply:") {
		t.Errorf("expected header in output, got: %q", out)
	}
	if !strings.Contains(out, "1. 0001_create_users.sql") || !strings.Contains(out, "2. 0002_add_index.sql") {
		t.Errorf("expected list of migrations in output, got: %q", out)
	}
}

func TestFormatPlanWithShowSQL(t *testing.T) {
	tmpDir := t.TempDir()

	file1 := filepath.Join(tmpDir, "0001_create_users.sql")
	sql1 := "CREATE TABLE users (\n    id SERIAL PRIMARY KEY\n);"
	if err := os.WriteFile(file1, []byte(sql1), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	file2 := filepath.Join(tmpDir, "0002_add_index.sql")
	sql2 := "CREATE INDEX idx_users ON users(id);"
	if err := os.WriteFile(file2, []byte(sql2), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	disc1 := &migration.MigrationFile{Version: 1, Filename: "0001_create_users.sql", Path: file1}
	disc2 := &migration.MigrationFile{Version: 2, Filename: "0002_add_index.sql", Path: file2}

	p := BuildPlan([]*migration.MigrationFile{disc1, disc2})

	buf := &bytes.Buffer{}
	err := FormatPlan(buf, p, Options{ShowSQL: true})
	if err != nil {
		t.Fatalf("unexpected error formatting plan with SQL: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "1. 0001_create_users.sql") || !strings.Contains(out, "2. 0002_add_index.sql") {
		t.Errorf("expected filenames in output, got: %q", out)
	}
	if !strings.Contains(out, "   CREATE TABLE users (") || !strings.Contains(out, "   CREATE INDEX idx_users ON users(id);") {
		t.Errorf("expected indented SQL in output, got:\n%s", out)
	}
}
