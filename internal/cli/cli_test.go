package cli

import (
	"bytes"
	"encoding/json"
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
	args := []string{"--database-url", "postgres://localhost/test", "--dir", "my_migrations", "--table", "custom_history", "--sql", "--no-lock", "--json", "plan"}
	gotURL, gotDir, gotTable, gotShowSQL, gotNoLock, gotJSON, gotRest := ParseFlags(args)

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
	if !gotJSON {
		t.Errorf("gotJSON = false, want true")
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
	if code != ExitUsage {
		t.Errorf("expected exit code %d for unknown command, got %d", ExitUsage, code)
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
	if code != ExitFailure {
		t.Errorf("expected exit code %d when missing database URL, got %d", ExitFailure, code)
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
	if code != ExitFailure {
		t.Errorf("expected exit code %d when missing database URL, got %d", ExitFailure, code)
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
	if code != ExitFailure {
		t.Errorf("expected exit code %d when database unreachable, got %d", ExitFailure, code)
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
	if code != ExitFailure {
		t.Errorf("expected exit code %d when missing database URL, got %d", ExitFailure, code)
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
	if code != ExitFailure {
		t.Errorf("expected exit code %d when database unreachable, got %d", ExitFailure, code)
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

	if code1 != ExitSuccess || code2 != ExitSuccess {
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
	if code1 != ExitFailure {
		t.Fatalf("expected exit code 1 on failing migration, got %d", code1)
	}

	_ = os.WriteFile(m1, []byte("CREATE TABLE fixed (id INTEGER PRIMARY KEY);"), 0644)

	stdout2 := &bytes.Buffer{}
	stderr2 := &bytes.Buffer{}

	code2 := RunWithWriters([]string{"--database-url", dbPath, "--dir", tempDir, "apply"}, stdout2, stderr2)
	if code2 != ExitSuccess {
		t.Fatalf("expected exit code 0 on re-run after fix, got %d (err: %s)", code2, stderr2.String())
	}
}

func TestRunWithWritersDryRunMissingDBConfig(t *testing.T) {
	origEnv := os.Getenv(config.EnvDatabaseURL)
	os.Setenv(config.EnvDatabaseURL, "")
	defer os.Setenv(config.EnvDatabaseURL, origEnv)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	code := RunWithWriters([]string{"dry-run"}, stdout, stderr)
	if code != ExitFailure {
		t.Errorf("expected exit code 1 when missing database URL, got %d", code)
	}
	if !strings.Contains(stderr.String(), "missing database connection string") {
		t.Errorf("expected missing database connection string error in stderr, got: %s", stderr.String())
	}
}

func TestRunWithWritersDryRunUnreachableDB(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	invalidURL := "postgres://invalid:invalid@127.0.0.1:59999/testdb?sslmode=disable"
	code := RunWithWriters([]string{"--database-url", invalidURL, "dry-run"}, stdout, stderr)
	if code != ExitFailure {
		t.Errorf("expected exit code 1 when database unreachable, got %d", code)
	}
	if !strings.Contains(stderr.String(), "schemago dry-run error:") {
		t.Errorf("expected schemago dry-run error in stderr, got: %s", stderr.String())
	}
}

func TestRunWithWritersDryRun_SuccessLeavesDBUnchanged(t *testing.T) {
	tempDir := t.TempDir()
	dbDir := t.TempDir()
	dbPath := "sqlite:" + filepath.Join(dbDir, "test_dryrun.db")

	m1 := filepath.Join(tempDir, "0001_create_users.sql")
	_ = os.WriteFile(m1, []byte("CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT);"), 0644)
	m2 := filepath.Join(tempDir, "0002_create_posts.sql")
	_ = os.WriteFile(m2, []byte("CREATE TABLE posts (id INTEGER PRIMARY KEY, title TEXT);"), 0644)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	code := RunWithWriters([]string{"--database-url", dbPath, "--dir", tempDir, "dry-run"}, stdout, stderr)
	if code != ExitSuccess {
		t.Fatalf("expected exit code 0 for dry-run, got %d (err: %s)", code, stderr.String())
	}

	outStr := stdout.String()
	if !strings.Contains(outStr, "[DRY-RUN] Simulating 2 pending migrations") ||
		!strings.Contains(outStr, "✓ 0001_create_users.sql") ||
		!strings.Contains(outStr, "✓ 0002_create_posts.sql") ||
		!strings.Contains(outStr, "[DRY-RUN] Dry-run completed successfully. 2 migrations validated (0 changes committed).") {
		t.Errorf("unexpected stdout format:\n%s", outStr)
	}

	// Verify real apply after dry-run succeeds and applies the migrations cleanly
	stdoutApply := &bytes.Buffer{}
	stderrApply := &bytes.Buffer{}
	codeApply := RunWithWriters([]string{"--database-url", dbPath, "--dir", tempDir, "apply"}, stdoutApply, stderrApply)
	if codeApply != ExitSuccess {
		t.Fatalf("expected apply after dry-run to succeed, got %d (err: %s)", codeApply, stderrApply.String())
	}
	if !strings.Contains(stdoutApply.String(), "Successfully applied 2 migrations.") {
		t.Errorf("unexpected apply stdout after dry-run:\n%s", stdoutApply.String())
	}
}

func TestRunWithWritersDryRun_CatchesSQLError(t *testing.T) {
	tempDir := t.TempDir()
	dbDir := t.TempDir()
	dbPath := "sqlite:" + filepath.Join(dbDir, "test_dryrun_err.db")

	m1 := filepath.Join(tempDir, "0001_create_users.sql")
	_ = os.WriteFile(m1, []byte("CREATE TABLE users (id INTEGER PRIMARY KEY);"), 0644)
	m2 := filepath.Join(tempDir, "0002_failing.sql")
	_ = os.WriteFile(m2, []byte("INVALID SQL SYNTAX HERE;"), 0644)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	code := RunWithWriters([]string{"--database-url", dbPath, "--dir", tempDir, "dry-run"}, stdout, stderr)
	if code != ExitFailure {
		t.Fatalf("expected exit code 1 for dry-run with SQL error, got %d", code)
	}

	outStr := stdout.String()
	if !strings.Contains(outStr, "[DRY-RUN]") ||
		!strings.Contains(outStr, "✗ 0002_failing.sql FAILED") ||
		!strings.Contains(outStr, "No changes were made to the database.") {
		t.Errorf("unexpected dry-run failure output:\n%s", outStr)
	}

	if !strings.Contains(stderr.String(), "schemago dry-run error:") {
		t.Errorf("expected schemago dry-run error in stderr, got: %s", stderr.String())
	}
}

func TestJSONOutputFlagsAndErrors(t *testing.T) {
	tempDir := t.TempDir()
	dbDir := t.TempDir()
	dbPath := "sqlite:" + filepath.Join(dbDir, "test_json.db")

	m1 := filepath.Join(tempDir, "0001_create_users.sql")
	_ = os.WriteFile(m1, []byte("CREATE TABLE users (id INTEGER PRIMARY KEY);"), 0644)

	t.Run("status --json", func(t *testing.T) {
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		code := RunWithWriters([]string{"--database-url", dbPath, "--dir", tempDir, "--json", "status"}, stdout, stderr)
		if code != ExitFailure { // has pending migration
			t.Errorf("expected exit code %d for pending status, got %d", ExitFailure, code)
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
			t.Fatalf("expected valid JSON in stdout for status --json, got error %v; output:\n%s", err, stdout.String())
		}
		if _, ok := resp["items"]; !ok {
			t.Errorf("expected 'items' field in status JSON report")
		}
	})

	t.Run("apply --json", func(t *testing.T) {
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		code := RunWithWriters([]string{"--database-url", dbPath, "--dir", tempDir, "--json", "apply"}, stdout, stderr)
		if code != ExitSuccess {
			t.Fatalf("expected exit code %d for apply --json, got %d (err: %s)", ExitSuccess, code, stderr.String())
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
			t.Fatalf("expected valid JSON in stdout for apply --json, got error %v; output:\n%s", err, stdout.String())
		}
		if _, ok := resp["applied"]; !ok {
			t.Errorf("expected 'applied' field in apply JSON result")
		}
	})

	t.Run("JSON error format on missing db URL", func(t *testing.T) {
		origEnv := os.Getenv(config.EnvDatabaseURL)
		os.Setenv(config.EnvDatabaseURL, "")
		defer os.Setenv(config.EnvDatabaseURL, origEnv)

		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		code := RunWithWriters([]string{"--json", "status"}, stdout, stderr)
		if code != ExitFailure {
			t.Errorf("expected exit code %d when missing database URL, got %d", ExitFailure, code)
		}

		var errResp CLIErrorResponse
		if err := json.Unmarshal(stderr.Bytes(), &errResp); err != nil {
			t.Fatalf("expected valid JSON error response in stderr, got error %v; stderr:\n%s", err, stderr.String())
		}
		if errResp.ExitCode != ExitFailure {
			t.Errorf("expected exit_code %d in JSON error, got %d", ExitFailure, errResp.ExitCode)
		}
		if !strings.Contains(errResp.Error, "missing database connection string") {
			t.Errorf("unexpected error text in JSON response: %s", errResp.Error)
		}
	})
}
