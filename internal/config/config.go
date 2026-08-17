// Package config manages application configuration and precedence rules for schemago.
package config

import (
	"errors"
	"os"
	"strings"
	"time"

	"github.com/parthdagia05/schemago/internal/history"
	"github.com/parthdagia05/schemago/internal/migration"
)

// DefaultTimeout represents the default connection timeout for database ping and handshake operations.
const DefaultTimeout = 5 * time.Second

// EnvDatabaseURL specifies the environment variable key used for database connection string resolution.
const EnvDatabaseURL = "DATABASE_URL"

// EnvMigrationsDir specifies the environment variable key for the migrations directory.
const EnvMigrationsDir = "MIGRATIONS_DIR"

// EnvTableName specifies the environment variable key for the schema history table name.
const EnvTableName = "MIGRATIONS_TABLE"

// ErrMissingDatabaseURL indicates that no connection string was provided via flags or environment.
var ErrMissingDatabaseURL = errors.New("missing database connection string: provide --database-url flag or set DATABASE_URL environment variable")

// Config holds runtime configuration parameters for schemago.
type Config struct {
	DatabaseURL   string        `json:"database_url"`
	Timeout       time.Duration `json:"timeout"`
	MigrationsDir string        `json:"migrations_dir"`
	TableName     string        `json:"table_name"`
}

// ResolveDatabaseURL returns the connection string based on configuration precedence:
// 1. Explicitly provided CLI flag value (if non-empty).
// 2. Value set in the DATABASE_URL environment variable.
func ResolveDatabaseURL(flagValue string) string {
	if strings.TrimSpace(flagValue) != "" {
		return strings.TrimSpace(flagValue)
	}
	return strings.TrimSpace(os.Getenv(EnvDatabaseURL))
}

// ResolveMigrationsDir returns the migrations directory path based on precedence:
// 1. Explicitly provided CLI flag value.
// 2. Value set in the MIGRATIONS_DIR environment variable.
// 3. Default directory path ("migrations").
func ResolveMigrationsDir(flagValue string) string {
	if strings.TrimSpace(flagValue) != "" {
		return strings.TrimSpace(flagValue)
	}
	if envVal := strings.TrimSpace(os.Getenv(EnvMigrationsDir)); envVal != "" {
		return envVal
	}
	return migration.DefaultMigrationsDir
}

// ResolveTableName returns the history table name based on precedence:
// 1. Explicitly provided CLI flag value.
// 2. Value set in the MIGRATIONS_TABLE environment variable.
// 3. Default table name ("schemago_migrations").
func ResolveTableName(flagValue string) string {
	if strings.TrimSpace(flagValue) != "" {
		return strings.TrimSpace(flagValue)
	}
	if envVal := strings.TrimSpace(os.Getenv(EnvTableName)); envVal != "" {
		return envVal
	}
	return history.DefaultTableName
}

// New constructs a Config instance by resolving the connection string and enforcing defaults.
func New(flagDatabaseURL string, timeout time.Duration) (Config, error) {
	return NewWithOpts(flagDatabaseURL, "", "", timeout)
}

// NewWithOpts constructs a Config instance with explicit flag overrides for database URL, directory, and table name.
func NewWithOpts(flagDatabaseURL, flagDir, flagTable string, timeout time.Duration) (Config, error) {
	dbURL := ResolveDatabaseURL(flagDatabaseURL)
	if dbURL == "" {
		return Config{}, ErrMissingDatabaseURL
	}

	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	dir := ResolveMigrationsDir(flagDir)
	table := ResolveTableName(flagTable)

	return Config{
		DatabaseURL:   dbURL,
		Timeout:       timeout,
		MigrationsDir: dir,
		TableName:     table,
	}, nil
}

