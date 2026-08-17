// Package migration defines conventions, parsing logic, and validation rules for migration files.
package migration

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	// DefaultMigrationsDir specifies the default directory path for migration files.
	DefaultMigrationsDir = "migrations"

	// FileExtension specifies the required file extension for migration scripts.
	FileExtension = ".sql"
)

var (
	// ErrInvalidFilename indicates that a file name does not match the migration naming convention.
	ErrInvalidFilename = errors.New("invalid migration filename format")

	// ErrDuplicateVersion indicates multiple migration files share the same numeric version.
	ErrDuplicateVersion = errors.New("duplicate migration version detected")
)

// filenameRegex matches filenames like 0001_create_users.sql or 20260817000001_initial_schema.sql.
var filenameRegex = regexp.MustCompile(`^([0-9]+)_([a-zA-Z0-9_-]+)\.sql$`)

// MigrationFile holds parsed details for a single migration script.
type MigrationFile struct {
	Version     int64
	RawVersion  string
	Description string
	Filename    string
	Path        string
	Checksum    string
}

// ParseFilename parses a migration filename into a MigrationFile metadata struct.
// Returns ErrInvalidFilename if the filename does not strictly conform to <version>_<description>.sql.
func ParseFilename(path string) (*MigrationFile, error) {
	filename := filepath.Base(path)
	matches := filenameRegex.FindStringSubmatch(filename)
	if len(matches) != 3 {
		return nil, fmt.Errorf("%w: %q (expected format: <version>_<description>.sql)", ErrInvalidFilename, filename)
	}

	rawVersion := matches[1]
	description := matches[2]

	version, err := strconv.ParseInt(rawVersion, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to parse version number %q: %v", ErrInvalidFilename, rawVersion, err)
	}

	if strings.TrimSpace(description) == "" {
		return nil, fmt.Errorf("%w: description component cannot be empty in %q", ErrInvalidFilename, filename)
	}

	return &MigrationFile{
		Version:     version,
		RawVersion:  rawVersion,
		Description: description,
		Filename:    filename,
		Path:        path,
	}, nil
}

// ValidateAndSort validates ordering rules for a slice of MigrationFile structs.
// It checks for duplicate version numbers and sorts the list in ascending numerical order.
func ValidateAndSort(files []*MigrationFile) ([]*MigrationFile, error) {
	if len(files) == 0 {
		return nil, nil
	}

	sorted := make([]*MigrationFile, len(files))
	copy(sorted, files)

	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Version == sorted[j].Version {
			return sorted[i].Filename < sorted[j].Filename
		}
		return sorted[i].Version < sorted[j].Version
	})

	seenVersions := make(map[int64]string, len(sorted))
	for _, file := range sorted {
		if prevFile, exists := seenVersions[file.Version]; exists {
			return nil, fmt.Errorf("%w: version %d is defined in both %q and %q", ErrDuplicateVersion, file.Version, prevFile, file.Filename)
		}
		seenVersions[file.Version] = file.Filename
	}

	return sorted, nil
}

// ComputeChecksum calculates and returns the hex-encoded SHA-256 hash of the given content.
func ComputeChecksum(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// ComputeFileChecksum reads the file at path and returns its hex-encoded SHA-256 hash.
func ComputeFileChecksum(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file for checksum: %w", err)
	}
	return ComputeChecksum(data), nil
}
