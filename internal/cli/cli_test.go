package cli

import (
	"bytes"
	"os"
	"strings"
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
	args := []string{"--database-url", "postgres://localhost/test", "--dir", "my_migrations", "--table", "custom_history", "status"}
	gotURL, gotDir, gotTable, gotRest := ParseFlags(args)

	if gotURL != "postgres://localhost/test" {
		t.Errorf("gotURL = %q, want postgres://localhost/test", gotURL)
	}
	if gotDir != "my_migrations" {
		t.Errorf("gotDir = %q, want my_migrations", gotDir)
	}
	if gotTable != "custom_history" {
		t.Errorf("gotTable = %q, want custom_history", gotTable)
	}
	if len(gotRest) != 1 || gotRest[0] != "status" {
		t.Errorf("gotRest = %v, want [status]", gotRest)
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
