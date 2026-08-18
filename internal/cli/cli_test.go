package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/parthdagia05/schemago/internal/config"
)

func TestParseGlobalFlags(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		wantURL       string
		wantRemaining []string
	}{
		{
			name:          "space-separated database-url flag",
			args:          []string{"--database-url", "postgres://localhost/test", "status"},
			wantURL:       "postgres://localhost/test",
			wantRemaining: []string{"status"},
		},
		{
			name:          "equals-separated database-url flag",
			args:          []string{"--database-url=postgres://localhost/test", "apply"},
			wantURL:       "postgres://localhost/test",
			wantRemaining: []string{"apply"},
		},
		{
			name:          "no flags provided",
			args:          []string{"plan"},
			wantURL:       "",
			wantRemaining: []string{"plan"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotURL, gotRest := ParseGlobalFlags(tt.args)
			if gotURL != tt.wantURL {
				t.Errorf("ParseGlobalFlags() gotURL = %q, want %q", gotURL, tt.wantURL)
			}
			if len(gotRest) != len(tt.wantRemaining) {
				t.Fatalf("ParseGlobalFlags() gotRest len = %d, want %d", len(gotRest), len(tt.wantRemaining))
			}
			for i := range gotRest {
				if gotRest[i] != tt.wantRemaining[i] {
					t.Errorf("ParseGlobalFlags() gotRest[%d] = %q, want %q", i, gotRest[i], tt.wantRemaining[i])
				}
			}
		})
	}
}

func TestParseFlags(t *testing.T) {
	args := []string{"--database-url", "postgres://localhost/test", "--dir", "my_migrations", "--table", "custom_history", "--sql", "--no-lock", "plan"}
	gotURL, gotDir, gotTable, gotShowSQL, gotNoLock, gotRest := ParseFlags(args)

	if gotURL != "postgres://localhost/test" {
		t.Errorf("gotURL = %q, want postgres://localhost/test", gotURL)
	}
	if gotDir != "my_migrations" {
		t.Errorf("gotDir = %q, want my_migrations", gotDir)
	}
	if gotTable != "custom_history" {
		t.Errorf("gotTable = %q, want custom_history", gotTable)
	}
	if !gotShowSQL {
		t.Errorf("gotShowSQL = false, want true")
	}
	if !gotNoLock {
		t.Errorf("gotNoLock = false, want true")
	}
	if len(gotRest) != 1 || gotRest[0] != "plan" {
		t.Errorf("gotRest = %v, want [plan]", gotRest)
	}
}

func TestRunWithWritersHelp(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	code := RunWithWriters([]string{"help"}, stdout, stderr)
	if code != 0 {
		t.Errorf("expected exit code 0 for help, got %d", code)
	}
	if !strings.Contains(stdout.String(), "schemago - a standalone database migration runner") {
		t.Errorf("expected usage info in stdout, got %s", stdout.String())
	}
}

func TestRunWithWritersUnknownCommand(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	code := RunWithWriters([]string{"unknown-cmd"}, stdout, stderr)
	if code != 2 {
		t.Errorf("expected exit code 2 for unknown command, got %d", code)
	}
	if !strings.Contains(stderr.String(), "unknown command") {
		t.Errorf("expected unknown command error in stderr, got %s", stderr.String())
	}
}

func TestRunWithWritersMissingDBConfig(t *testing.T) {
	origEnv := os.Getenv(config.EnvDatabaseURL)
	os.Setenv(config.EnvDatabaseURL, "")
	defer os.Setenv(config.EnvDatabaseURL, origEnv)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	code := RunWithWriters([]string{"status"}, stdout, stderr)
	if code != 1 {
		t.Errorf("expected exit code 1 when missing database URL, got %d", code)
	}
	if !strings.Contains(stderr.String(), "missing database connection string") {
		t.Errorf("expected missing database connection string error in stderr, got: %s", stderr.String())
	}
}

func TestRunWithWritersPlanMissingDBConfig(t *testing.T) {
	origEnv := os.Getenv(config.EnvDatabaseURL)
	os.Setenv(config.EnvDatabaseURL, "")
	defer os.Setenv(config.EnvDatabaseURL, origEnv)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	code := RunWithWriters([]string{"plan"}, stdout, stderr)
	if code != 1 {
		t.Errorf("expected exit code 1 when missing database URL, got %d", code)
	}
	if !strings.Contains(stderr.String(), "missing database connection string") {
		t.Errorf("expected missing database connection string error in stderr, got: %s", stderr.String())
	}
}

func TestRunWithWritersUnreachableDB(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	invalidURL := "postgres://invalid:invalid@127.0.0.1:59999/testdb?sslmode=disable"
	code := RunWithWriters([]string{"--database-url", invalidURL, "status"}, stdout, stderr)
	if code != 1 {
		t.Errorf("expected exit code 1 when database unreachable, got %d", code)
	}
	if !strings.Contains(stderr.String(), "schemago status error:") {
		t.Errorf("expected schemago status error in stderr, got: %s", stderr.String())
	}
}

func TestRunWithWritersApplyMissingDBConfig(t *testing.T) {
	origEnv := os.Getenv(config.EnvDatabaseURL)
	os.Setenv(config.EnvDatabaseURL, "")
	defer os.Setenv(config.EnvDatabaseURL, origEnv)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	code := RunWithWriters([]string{"apply"}, stdout, stderr)
	if code != 1 {
		t.Errorf("expected exit code 1 when missing database URL, got %d", code)
	}
	if !strings.Contains(stderr.String(), "missing database connection string") {
		t.Errorf("expected missing database connection string error in stderr, got: %s", stderr.String())
	}
}

func TestRunWithWritersApplyUnreachableDB(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	invalidURL := "postgres://invalid:invalid@127.0.0.1:59999/testdb?sslmode=disable"
	code := RunWithWriters([]string{"--database-url", invalidURL, "apply"}, stdout, stderr)
	if code != 1 {
		t.Errorf("expected exit code 1 when database unreachable, got %d", code)
	}
	if !strings.Contains(stderr.String(), "schemago apply error:") {
		t.Errorf("expected schemago apply error in stderr, got: %s", stderr.String())
	}
}

func TestRunWithWritersApply_ConcurrentRunners(t *testing.T) {
	tempDir := t.TempDir()

	m1 := filepath.Join(tempDir, "0001_create_users.sql")
	_ = os.WriteFile(m1, []byte("CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT);"), 0644)
	m2 := filepath.Join(tempDir, "0002_create_posts.sql")
	_ = os.WriteFile(m2, []byte("CREATE TABLE posts (id INTEGER PRIMARY KEY, title TEXT);"), 0644)

	dbDir := t.TempDir()
	dbPath := "sqlite:" + filepath.Join(dbDir, "test_concurrent.db")

	var wg sync.WaitGroup
	wg.Add(2)

	out1 := &bytes.Buffer{}
	err1 := &bytes.Buffer{}
	code1Ch := make(chan int, 1)

	out2 := &bytes.Buffer{}
	err2 := &bytes.Buffer{}
	code2Ch := make(chan int, 1)

	go func() {
		defer wg.Done()
		code := RunWithWriters([]string{"--database-url", dbPath, "--dir", tempDir, "apply"}, out1, err1)
		code1Ch <- code
	}()

	go func() {
		defer wg.Done()
		code := RunWithWriters([]string{"--database-url", dbPath, "--dir", tempDir, "apply"}, out2, err2)
		code2Ch <- code
	}()

	wg.Wait()

	code1 := <-code1Ch
	code2 := <-code2Ch

	if code1 != 0 || code2 != 0 {
		t.Fatalf("expected exit code 0 for both concurrent runners, got code1=%d (err: %s), code2=%d (err: %s)", code1, err1.String(), code2, err2.String())
	}

	combinedOutput := out1.String() + "\n" + out2.String()

	if !strings.Contains(combinedOutput, "Successfully applied 2 migrations.") {
		t.Errorf("expected output to contain successful application of 2 migrations, got:\nRunner 1:\n%s\nRunner 2:\n%s", out1.String(), out2.String())
	}

	if !strings.Contains(combinedOutput, "Nothing to apply. Database is up to date.") {
		t.Errorf("expected waiter runner output to contain 'Nothing to apply. Database is up to date.', got:\nRunner 1:\n%s\nRunner 2:\n%s", out1.String(), out2.String())
	}
}

func TestRunWithWritersApply_LockReleasedOnFailure(t *testing.T) {
	tempDir := t.TempDir()
	dbDir := t.TempDir()
	dbPath := "sqlite:" + filepath.Join(dbDir, "test_failure.db")

	m1 := filepath.Join(tempDir, "0001_failing.sql")
	_ = os.WriteFile(m1, []byte("INVALID SQL SYNTAX;"), 0644)

	stdout1 := &bytes.Buffer{}
	stderr1 := &bytes.Buffer{}

	code1 := RunWithWriters([]string{"--database-url", dbPath, "--dir", tempDir, "apply"}, stdout1, stderr1)
	if code1 != 1 {
		t.Fatalf("expected exit code 1 on failing migration, got %d", code1)
	}

	_ = os.WriteFile(m1, []byte("CREATE TABLE fixed (id INTEGER PRIMARY KEY);"), 0644)

	stdout2 := &bytes.Buffer{}
	stderr2 := &bytes.Buffer{}

	code2 := RunWithWriters([]string{"--database-url", dbPath, "--dir", tempDir, "apply"}, stdout2, stderr2)
	if code2 != 0 {
		t.Fatalf("expected exit code 0 on re-run after fix, got %d (err: %s)", code2, stderr2.String())
	}
}


