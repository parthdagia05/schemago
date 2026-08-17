package migration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Discover scans the specified directory for migration files, validates their names,
// checks for duplicate versions, and returns them ordered by version ascending.
func Discover(dirPath string) ([]*MigrationFile, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read migrations directory %q: %w", dirPath, err)
	}

	var files []*MigrationFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filename := entry.Name()
		if strings.HasPrefix(filename, ".") {
			continue
		}

		filePath := filepath.Join(dirPath, filename)
		migrationFile, err := ParseFilename(filePath)
		if err != nil {
			return nil, fmt.Errorf("invalid migration file in directory %q: %w", dirPath, err)
		}

		files = append(files, migrationFile)
	}

	sortedFiles, err := ValidateAndSort(files)
	if err != nil {
		return nil, fmt.Errorf("invalid migration set in directory %q: %w", dirPath, err)
	}

	return sortedFiles, nil
}

// DiscoverMigrations is an alias for Discover to locate and validate migration files in dirPath.
func DiscoverMigrations(dirPath string) ([]*MigrationFile, error) {
	return Discover(dirPath)
}
