package migration

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDiscover_ValidMigrations(t *testing.T) {
	tempDir := t.TempDir()

	filesToCreate := []string{
		"0003_add_user_index.sql",
		"0001_create_users.sql",
		"0002_create_posts.sql",
	}

	for _, name := range filesToCreate {
		err := os.WriteFile(filepath.Join(tempDir, name), []byte("-- migration content"), 0644)
		if err != nil {
			t.Fatalf("failed to create temp file %s: %v", name, err)
		}
	}

	migrations, err := Discover(tempDir)
	if err != nil {
		t.Fatalf("Discover unexpected error: %v", err)
	}

	if len(migrations) != 3 {
		t.Fatalf("expected 3 migrations, got %d", len(migrations))
	}

	expectedVersions := []int64{1, 2, 3}
	expectedDescs := []string{"create_users", "create_posts", "add_user_index"}
	expectedNames := []string{"0001_create_users.sql", "0002_create_posts.sql", "0003_add_user_index.sql"}

	for i, m := range migrations {
		if m.Version != expectedVersions[i] {
			t.Errorf("migration[%d] version = %d, want %d", i, m.Version, expectedVersions[i])
		}
		if m.Description != expectedDescs[i] {
			t.Errorf("migration[%d] description = %q, want %q", i, m.Description, expectedDescs[i])
		}
		if m.Filename != expectedNames[i] {
			t.Errorf("migration[%d] filename = %q, want %q", i, m.Filename, expectedNames[i])
		}
		expectedPath := filepath.Join(tempDir, expectedNames[i])
		if m.Path != expectedPath {
			t.Errorf("migration[%d] path = %q, want %q", i, m.Path, expectedPath)
		}
	}
}

func TestDiscover_TimestampVersionOrder(t *testing.T) {
	tempDir := t.TempDir()

	filesToCreate := []string{
		"20260817000002_add_index.sql",
		"20260817000001_init.sql",
	}

	for _, name := range filesToCreate {
		if err := os.WriteFile(filepath.Join(tempDir, name), []byte("-- sql"), 0644); err != nil {
			t.Fatalf("failed to create temp file: %v", err)
		}
	}

	migrations, err := Discover(tempDir)
	if err != nil {
		t.Fatalf("Discover unexpected error: %v", err)
	}

	if len(migrations) != 2 {
		t.Fatalf("expected 2 migrations, got %d", len(migrations))
	}

	if migrations[0].Version != 20260817000001 || migrations[1].Version != 20260817000002 {
		t.Errorf("incorrect timestamp order: %d, %d", migrations[0].Version, migrations[1].Version)
	}
}

func TestDiscover_EmptyDirectory(t *testing.T) {
	tempDir := t.TempDir()

	migrations, err := Discover(tempDir)
	if err != nil {
		t.Fatalf("Discover empty dir unexpected error: %v", err)
	}

	if len(migrations) != 0 {
		t.Errorf("expected 0 migrations in empty dir, got %d", len(migrations))
	}
}

func TestDiscover_NonExistentDirectory(t *testing.T) {
	nonExistent := filepath.Join(t.TempDir(), "nonexistent_dir")

	_, err := Discover(nonExistent)
	if err == nil {
		t.Fatalf("expected error for non-existent directory, got nil")
	}

	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected os.ErrNotExist error, got: %v", err)
	}
}

func TestDiscover_DuplicateVersion(t *testing.T) {
	tempDir := t.TempDir()

	filesToCreate := []string{
		"0001_create_users.sql",
		"0001_create_accounts.sql",
	}

	for _, name := range filesToCreate {
		if err := os.WriteFile(filepath.Join(tempDir, name), []byte("-- sql"), 0644); err != nil {
			t.Fatalf("failed to create file: %v", err)
		}
	}

	_, err := Discover(tempDir)
	if err == nil {
		t.Fatalf("expected error for duplicate version, got nil")
	}

	if !errors.Is(err, ErrDuplicateVersion) {
		t.Errorf("expected ErrDuplicateVersion, got: %v", err)
	}
}

func TestDiscover_MalformedFilename(t *testing.T) {
	malformedCases := []string{
		"invalid.sql",
		"0001_missing_ext.txt",
		"0001create_users.sql",
		"abc_create_users.sql",
		"_no_version.sql",
	}

	for _, filename := range malformedCases {
		t.Run(filename, func(t *testing.T) {
			tempDir := t.TempDir()
			if err := os.WriteFile(filepath.Join(tempDir, filename), []byte("-- sql"), 0644); err != nil {
				t.Fatalf("failed to create file: %v", err)
			}

			_, err := Discover(tempDir)
			if err == nil {
				t.Fatalf("expected error for malformed file %q, got nil", filename)
			}

			if !errors.Is(err, ErrInvalidFilename) {
				t.Errorf("expected ErrInvalidFilename for %q, got: %v", filename, err)
			}
		})
	}
}

func TestDiscover_IgnoresHiddenFilesAndSubdirectories(t *testing.T) {
	tempDir := t.TempDir()

	// Subdirectory containing sql file
	subDir := filepath.Join(tempDir, "subfolder")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("failed to create subfolder: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "0099_nested.sql"), []byte("-- nested"), 0644); err != nil {
		t.Fatalf("failed to create nested file: %v", err)
	}

	// Hidden files
	if err := os.WriteFile(filepath.Join(tempDir, ".gitkeep"), []byte(""), 0644); err != nil {
		t.Fatalf("failed to create .gitkeep: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, ".DS_Store"), []byte(""), 0644); err != nil {
		t.Fatalf("failed to create .DS_Store: %v", err)
	}

	// Valid migration file
	if err := os.WriteFile(filepath.Join(tempDir, "0001_init.sql"), []byte("-- init"), 0644); err != nil {
		t.Fatalf("failed to create valid file: %v", err)
	}

	migrations, err := Discover(tempDir)
	if err != nil {
		t.Fatalf("Discover unexpected error: %v", err)
	}

	if len(migrations) != 1 {
		t.Fatalf("expected 1 migration (ignoring hidden files and subdirs), got %d", len(migrations))
	}

	if migrations[0].Filename != "0001_init.sql" {
		t.Errorf("expected 0001_init.sql, got %s", migrations[0].Filename)
	}
}

func TestDiscover_ExistingMigrationsDirectory(t *testing.T) {
	migrations, err := Discover(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatalf("Discover(%q) error: %v", DefaultMigrationsDir, err)
	}

	if len(migrations) < 2 {
		t.Fatalf("expected at least 2 migrations in default directory, got %d", len(migrations))
	}

	if migrations[0].Filename != "0001_create_users_table.sql" {
		t.Errorf("unexpected first migration: %s", migrations[0].Filename)
	}
	if migrations[1].Filename != "0002_add_email_index.sql" {
		t.Errorf("unexpected second migration: %s", migrations[1].Filename)
	}
}

func TestDiscoverMigrations_Alias(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "0001_init.sql"), []byte("-- init"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	migrations, err := DiscoverMigrations(tempDir)
	if err != nil {
		t.Fatalf("DiscoverMigrations unexpected error: %v", err)
	}

	if len(migrations) != 1 || migrations[0].Filename != "0001_init.sql" {
		t.Errorf("unexpected result from DiscoverMigrations alias: %v", migrations)
	}
}
