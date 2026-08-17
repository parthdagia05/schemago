package config

import (
	"errors"
	"os"
	"testing"
	"time"
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

	t.Run("returns config with default timeout when timeout <= 0", func(t *testing.T) {
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
	})

	t.Run("uses custom timeout when provided", func(t *testing.T) {
		customTimeout := 10 * time.Second
		cfg, err := New("postgres://user:pass@localhost:5432/flagdb", customTimeout)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Timeout != customTimeout {
			t.Errorf("expected custom timeout %v, got %v", customTimeout, cfg.Timeout)
		}
	})
}
