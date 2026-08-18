// Package db provides database connection management, health checks, and error formatting for schemago.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	// Register pgx and sqlite database drivers for database/sql compatibility.
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

// DriverName specifies the database driver identifier used by database/sql.
const DriverName = "pgx"

// DefaultTimeout represents the fallback duration for establishing connection and health checks.
const DefaultTimeout = 5 * time.Second

// ErrNilDB indicates an attempt to perform operations on an uninitialized database handle.
var ErrNilDB = errors.New("database connection handle is nil")

// ErrEmptyConnectionString indicates that an empty connection string was supplied.
var ErrEmptyConnectionString = errors.New("database connection string cannot be empty")

// FormatError inspects database and network errors to provide clear, actionable feedback.
func FormatError(err error, timeout time.Duration) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("database connection timed out after %v", timeout)
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return fmt.Errorf("database connection timed out after %v: %w", timeout, err)
	}

	errStr := err.Error()

	switch {
	case strings.Contains(errStr, "connection refused"):
		return fmt.Errorf("database unreachable: connection refused by host: %w", err)
	case strings.Contains(errStr, "no such host") || strings.Contains(errStr, "lookup"):
		return fmt.Errorf("database unreachable: host name resolution failed: %w", err)
	case strings.Contains(errStr, "password authentication failed") || strings.Contains(errStr, "auth failed"):
		return fmt.Errorf("database authentication failed: invalid credentials: %w", err)
	case strings.Contains(errStr, "database") && strings.Contains(errStr, "does not exist"):
		return fmt.Errorf("database name error: target database does not exist: %w", err)
	case strings.Contains(errStr, "cannot parse"):
		return fmt.Errorf("invalid database connection string: %w", err)
	default:
		return fmt.Errorf("database connection error: %w", err)
	}
}

// Connect creates a database connection handle using the appropriate database driver.
func Connect(connectionString string) (*sql.DB, error) {
	if strings.TrimSpace(connectionString) == "" {
		return nil, ErrEmptyConnectionString
	}

	driver := DriverName
	dsn := connectionString
	if strings.HasPrefix(connectionString, "sqlite:") || strings.HasPrefix(connectionString, "file:") || connectionString == ":memory:" {
		driver = "sqlite"
		if strings.HasPrefix(connectionString, "sqlite:") {
			dsn = strings.TrimPrefix(connectionString, "sqlite:")
		}
	}

	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database driver: %w", err)
	}

	return db, nil
}

// Ping tests whether the database server is reachable within the context deadline.
func Ping(ctx context.Context, db *sql.DB, timeout time.Duration) error {
	if db == nil {
		return ErrNilDB
	}

	if err := db.PingContext(ctx); err != nil {
		return FormatError(err, timeout)
	}

	return nil
}

// ConnectAndPing combines connection initialization and health verification under a specified timeout.
func ConnectAndPing(ctx context.Context, connectionString string, timeout time.Duration) (*sql.DB, error) {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	db, err := Connect(connectionString)
	if err != nil {
		return nil, FormatError(err, timeout)
	}

	pingCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := Ping(pingCtx, db, timeout); err != nil {
		_ = db.Close()
		return nil, err
	}

	return db, nil
}
