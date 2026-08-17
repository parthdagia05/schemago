package config

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/parthdagia05/schemago/internal/history"
	"github.com/parthdagia05/schemago/internal/migration"
)

func TestResolveDatabaseURL(t *testing.T) {
	origEnv := os.Getenv(EnvDatabaseURL)
	defer os.Setenv(EnvDatabaseURL, origEnv)

	tests := []struct {
		name      string
		envVal    string
		flagVal   string
		wantValue string
	}{
		{
			name:      "flag overrides env",
			envVal:    "postgres://user:pass@localhost:5432/envdb",
			flagVal:   "postgres://user:pass@localhost:5432/flagdb",
			wantValue: "postgres://user:pass@localhost:5432/flagdb",
		},
		{
			name:      "env fallback when flag is empty",
			envVal:    "postgres://user:pass@localhost:5432/envdb",
			flagVal:   "",
			wantValue: "postgres://user:pass@localhost:5432/envdb",
		},
		{
			name:      "whitespace flag is trimmed and ignored",
			envVal:    "postgres://user:pass@localhost:5432/envdb",
			flagVal:   "   ",
			wantValue: "postgres://user:pass@localhost:5432/envdb",
		},
		{
			name:      "empty when both are empty",
			envVal:    "",
			flagVal:   "",
			wantValue: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv(EnvDatabaseURL, tt.envVal)
			got := ResolveDatabaseURL(tt.flagVal)
			if got != tt.wantValue {
				t.Errorf("ResolveDatabaseURL(%q) with ENV=%q got %q, want %q", tt.flagVal, tt.envVal, got, tt.wantValue)
			}
		})
	}
}

func TestResolveMigrationsDir(t *testing.T) {
	origEnv := os.Getenv(EnvMigrationsDir)
	defer os.Setenv(EnvMigrationsDir, origEnv)

	t.Run("defaults to migration.DefaultMigrationsDir", func(t *testing.T) {
		os.Setenv(EnvMigrationsDir, "")
		got := ResolveMigrationsDir("")
		if got != migration.DefaultMigrationsDir {
			t.Errorf("got %q, want %q", got, migration.DefaultMigrationsDir)
		}
	})

	t.Run("flag overrides env and default", func(t *testing.T) {
		os.Setenv(EnvMigrationsDir, "env_dir")
		got := ResolveMigrationsDir("flag_dir")
		if got != "flag_dir" {
			t.Errorf("got %q, want flag_dir", got)
		}
	})

	t.Run("env fallback when flag empty", func(t *testing.T) {
		os.Setenv(EnvMigrationsDir, "env_dir")
		got := ResolveMigrationsDir("")
		if got != "env_dir" {
			t.Errorf("got %q, want env_dir", got)
		}
	})
}

func TestResolveTableName(t *testing.T) {
	origEnv := os.Getenv(EnvTableName)
	defer os.Setenv(EnvTableName, origEnv)

	t.Run("defaults to history.DefaultTableName", func(t *testing.T) {
		os.Setenv(EnvTableName, "")
		got := ResolveTableName("")
		if got != history.DefaultTableName {
			t.Errorf("got %q, want %q", got, history.DefaultTableName)
		}
	})

	t.Run("flag overrides env and default", func(t *testing.T) {
		os.Setenv(EnvTableName, "env_table")
		got := ResolveTableName("flag_table")
		if got != "flag_table" {
			t.Errorf("got %q, want flag_table", got)
		}
	})
}

func TestNew(t *testing.T) {
	origEnv := os.Getenv(EnvDatabaseURL)
	defer os.Setenv(EnvDatabaseURL, origEnv)

	t.Run("returns error when missing database connection string", func(t *testing.T) {
		os.Setenv(EnvDatabaseURL, "")
		_, err := New("", 0)
		if !errors.Is(err, ErrMissingDatabaseURL) {
			t.Errorf("expected ErrMissingDatabaseURL, got %v", err)
		}
	})

	t.Run("returns config with default timeout and defaults when options empty", func(t *testing.T) {
		os.Setenv(EnvDatabaseURL, "postgres://user:pass@localhost:5432/testdb")
		cfg, err := New("", 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Timeout != DefaultTimeout {
			t.Errorf("expected default timeout %v, got %v", DefaultTimeout, cfg.Timeout)
		}
		if cfg.DatabaseURL != "postgres://user:pass@localhost:5432/testdb" {
			t.Errorf("unexpected database URL: %s", cfg.DatabaseURL)
		}
		if cfg.MigrationsDir != migration.DefaultMigrationsDir {
			t.Errorf("expected default migrations dir %q, got %q", migration.DefaultMigrationsDir, cfg.MigrationsDir)
		}
		if cfg.TableName != history.DefaultTableName {
			t.Errorf("expected default table name %q, got %q", history.DefaultTableName, cfg.TableName)
		}
	})

	t.Run("uses custom options when provided via NewWithOpts", func(t *testing.T) {
		customTimeout := 10 * time.Second
		cfg, err := NewWithOpts("postgres://user:pass@localhost:5432/flagdb", "custom_dir", "custom_table", customTimeout)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Timeout != customTimeout {
			t.Errorf("expected custom timeout %v, got %v", customTimeout, cfg.Timeout)
		}
		if cfg.MigrationsDir != "custom_dir" {
			t.Errorf("expected custom dir, got %s", cfg.MigrationsDir)
		}
		if cfg.TableName != "custom_table" {
			t.Errorf("expected custom table, got %s", cfg.TableName)
		}
	})
}
