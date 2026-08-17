// Package config manages application configuration and precedence rules for schemago.
package config

import (
	"errors"
	"os"
	"strings"
	"time"
)

// DefaultTimeout represents the default connection timeout for database ping and handshake operations.
const DefaultTimeout = 5 * time.Second

// EnvDatabaseURL specifies the environment variable key used for database connection string resolution.
const EnvDatabaseURL = "DATABASE_URL"

// ErrMissingDatabaseURL indicates that no connection string was provided via flags or environment.
var ErrMissingDatabaseURL = errors.New("missing database connection string: provide --database-url flag or set DATABASE_URL environment variable")

// Config holds runtime configuration parameters for schemago.
type Config struct {
	DatabaseURL string        `json:"database_url"`
	Timeout     time.Duration `json:"timeout"`
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

// New constructs a Config instance by resolving the connection string and enforcing defaults.
func New(flagDatabaseURL string, timeout time.Duration) (Config, error) {
	dbURL := ResolveDatabaseURL(flagDatabaseURL)
	if dbURL == "" {
		return Config{}, ErrMissingDatabaseURL
	}

	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	return Config{
		DatabaseURL: dbURL,
		Timeout:     timeout,
	}, nil
}
