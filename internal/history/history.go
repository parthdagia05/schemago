// Package history manages the schema history table, tracks applied migrations,
// and computes pending migrations against discovered migration files.
package history

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/parthdagia05/schemago/internal/migration"
)

const (
	// DefaultTableName is the default database table name used for tracking applied migrations.
	DefaultTableName = "schemago_migrations"
)

var (
	// ErrInvalidTableName indicates that the provided table name contains invalid characters.
	ErrInvalidTableName = errors.New("invalid history table name")

	// ErrChecksumMismatch indicates an applied migration's stored checksum differs from the disk file checksum.
	ErrChecksumMismatch = errors.New("migration file checksum mismatch")

	// ErrMissingMigrationFile indicates an applied migration record exists in the DB but is absent on disk.
	ErrMissingMigrationFile = errors.New("applied migration missing from local migration directory")
)

// tableNameRegex matches valid SQL identifier table names (alphanumeric and underscores).
var tableNameRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// AppliedMigration represents a row in the migration history table.
type AppliedMigration struct {
	Version   int64     `json:"version"`
	Name      string    `json:"name"`
	AppliedAt time.Time `json:"applied_at"`
	Checksum  string    `json:"checksum"`
}

// Execer abstracts database execution methods for compatibility with *sql.DB and *sql.Tx.
type Execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// ValidateTableName ensures the table name is a valid SQL identifier.
func ValidateTableName(tableName string) (string, error) {
	if tableName == "" {
		return DefaultTableName, nil
	}
	if !tableNameRegex.MatchString(tableName) {
		return "", fmt.Errorf("%w: %q (must be a valid identifier)", ErrInvalidTableName, tableName)
	}
	return tableName, nil
}

// EnsureTable creates the migration history table if it does not already exist.
func EnsureTable(ctx context.Context, db Execer, tableName string) error {
	tableName, err := ValidateTableName(tableName)
	if err != nil {
		return err
	}

	query := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		version BIGINT PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
		checksum VARCHAR(64) NOT NULL
	);`, tableName)

	if _, err := db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("failed to create history table %q: %w", tableName, err)
	}

	return nil
}

// RecordMigration inserts an applied migration record into the history table.
func RecordMigration(ctx context.Context, db Execer, tableName string, record *AppliedMigration) error {
	tableName, err := ValidateTableName(tableName)
	if err != nil {
		return err
	}

	if record == nil {
		return errors.New("record cannot be nil")
	}

	appliedAt := record.AppliedAt
	if appliedAt.IsZero() {
		appliedAt = time.Now().UTC()
	}

	query := fmt.Sprintf(`INSERT INTO %s (version, name, applied_at, checksum)
		VALUES ($1, $2, $3, $4)`, tableName)

	if _, err := db.ExecContext(ctx, query, record.Version, record.Name, appliedAt.Format(time.RFC3339), record.Checksum); err != nil {
		return fmt.Errorf("failed to record migration version %d (%q): %w", record.Version, record.Name, err)
	}

	return nil
}

// GetAppliedMigrations retrieves all applied migration records ordered by version ascending.
func GetAppliedMigrations(ctx context.Context, db Execer, tableName string) ([]*AppliedMigration, error) {
	tableName, err := ValidateTableName(tableName)
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`SELECT version, name, applied_at, checksum FROM %s ORDER BY version ASC`, tableName)

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		if isTableNotExistError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query applied migrations from %q: %w", tableName, err)
	}
	defer rows.Close()

	var records []*AppliedMigration
	for rows.Next() {
		var rec AppliedMigration
		var appliedAtVal any
		if err := rows.Scan(&rec.Version, &rec.Name, &appliedAtVal, &rec.Checksum); err != nil {
			return nil, fmt.Errorf("failed to scan applied migration record: %w", err)
		}

		switch v := appliedAtVal.(type) {
		case time.Time:
			rec.AppliedAt = v
		case string:
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				rec.AppliedAt = t
			} else if t, err := time.Parse("2006-01-02 15:04:05.999999999-07:00", v); err == nil {
				rec.AppliedAt = t
			} else if t, err := time.Parse("2006-01-02 15:04:05", v); err == nil {
				rec.AppliedAt = t
			}
		case []byte:
			if t, err := time.Parse(time.RFC3339, string(v)); err == nil {
				rec.AppliedAt = t
			}
		}

		records = append(records, &rec)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading applied migration rows: %w", err)
	}

	return records, nil
}

// ComputePending compares discovered migration files against applied database records.
// It verifies checksum consistency for already applied migrations and returns unapplied (pending) files.
func ComputePending(discovered []*migration.MigrationFile, applied []*AppliedMigration) ([]*migration.MigrationFile, error) {
	appliedMap := make(map[int64]*AppliedMigration, len(applied))
	for _, app := range applied {
		appliedMap[app.Version] = app
	}

	discoveredMap := make(map[int64]*migration.MigrationFile, len(discovered))
	for _, disc := range discovered {
		discoveredMap[disc.Version] = disc
	}

	// Verify all applied migrations still exist on disk
	for _, app := range applied {
		if _, exists := discoveredMap[app.Version]; !exists {
			return nil, fmt.Errorf("%w: version %d (%q) recorded in database is missing from local files", ErrMissingMigrationFile, app.Version, app.Name)
		}
	}

	var pending []*migration.MigrationFile
	for _, disc := range discovered {
		app, isApplied := appliedMap[disc.Version]
		if !isApplied {
			pending = append(pending, disc)
			continue
		}

		discChecksum := disc.Checksum
		if discChecksum == "" && disc.Path != "" {
			computed, err := migration.ComputeFileChecksum(disc.Path)
			if err == nil {
				discChecksum = computed
			}
		}

		if app.Checksum != "" && discChecksum != "" && app.Checksum != discChecksum {
			return nil, fmt.Errorf("%w: migration %q (version %d) checksum changed after application (db: %s, file: %s)",
				ErrChecksumMismatch, disc.Filename, disc.Version, app.Checksum, discChecksum)
		}
	}

	return pending, nil
}

func isTableNotExistError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "does not exist") ||
		strings.Contains(msg, "no such table") ||
		strings.Contains(msg, "undefined_table") ||
		strings.Contains(msg, "42p01")
}
