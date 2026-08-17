package migration

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestParseFilename(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		wantVersion int64
		wantRaw     string
		wantDesc    string
		wantErr     error
	}{
		{
			name:        "valid sequential 4-digit filename",
			path:        "migrations/0001_create_users_table.sql",
			wantVersion: 1,
			wantRaw:     "0001",
			wantDesc:    "create_users_table",
			wantErr:     nil,
		},
		{
			name:        "valid timestamp filename",
			path:        "migrations/20260817000001_add_email_index.sql",
			wantVersion: 20260817000001,
			wantRaw:     "20260817000001",
			wantDesc:    "add_email_index",
			wantErr:     nil,
		},
		{
			name:        "valid filename with hyphens and underscores",
			path:        "0003_alter_table_v2-add-column.sql",
			wantVersion: 3,
			wantRaw:     "0003",
			wantDesc:    "alter_table_v2-add-column",
			wantErr:     nil,
		},
		{
			name:    "missing separator",
			path:    "0001create_users.sql",
			wantErr: ErrInvalidFilename,
		},
		{
			name:    "non-sql extension",
			path:    "0001_create_users.txt",
			wantErr: ErrInvalidFilename,
		},
		{
			name:    "non-numeric version",
			path:    "abc_create_users.sql",
			wantErr: ErrInvalidFilename,
		},
		{
			name:    "no version prefix",
			path:    "create_users.sql",
			wantErr: ErrInvalidFilename,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseFilename(tt.path)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("ParseFilename(%q) expected error, got nil", tt.path)
				}
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ParseFilename(%q) error = %v, wantErr = %v", tt.path, err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("ParseFilename(%q) unexpected error: %v", tt.path, err)
			}

			if got.Version != tt.wantVersion {
				t.Errorf("Version = %d, want %d", got.Version, tt.wantVersion)
			}
			if got.RawVersion != tt.wantRaw {
				t.Errorf("RawVersion = %q, want %q", got.RawVersion, tt.wantRaw)
			}
			if got.Description != tt.wantDesc {
				t.Errorf("Description = %q, want %q", got.Description, tt.wantDesc)
			}
		})
	}
}

func TestValidateAndSort(t *testing.T) {
	t.Run("sorts out of order files correctly", func(t *testing.T) {
		f1, _ := ParseFilename("0002_add_index.sql")
		f2, _ := ParseFilename("0001_create_table.sql")
		f3, _ := ParseFilename("0005_add_column.sql")

		files := []*MigrationFile{f1, f2, f3}
		sorted, err := ValidateAndSort(files)
		if err != nil {
			t.Fatalf("ValidateAndSort unexpected error: %v", err)
		}

		if len(sorted) != 3 {
			t.Fatalf("expected 3 sorted files, got %d", len(sorted))
		}
		if sorted[0].Version != 1 || sorted[1].Version != 2 || sorted[2].Version != 5 {
			t.Errorf("incorrect sort order: %v, %v, %v", sorted[0].Version, sorted[1].Version, sorted[2].Version)
		}
	})

	t.Run("fails on duplicate versions", func(t *testing.T) {
		f1, _ := ParseFilename("0001_create_users.sql")
		f2, _ := ParseFilename("0001_create_profiles.sql")

		files := []*MigrationFile{f1, f2}
		_, err := ValidateAndSort(files)
		if err == nil {
			t.Fatalf("expected error on duplicate version, got nil")
		}
		if !errors.Is(err, ErrDuplicateVersion) {
			t.Fatalf("expected ErrDuplicateVersion, got %v", err)
		}
	})
}

func TestComputeChecksum(t *testing.T) {
	content := []byte("CREATE TABLE users (id SERIAL PRIMARY KEY);")
	hash1 := ComputeChecksum(content)
	hash2 := ComputeChecksum(content)

	if hash1 == "" {
		t.Fatal("expected non-empty hash string")
	}
	if hash1 != hash2 {
		t.Errorf("expected deterministic hash output, got %s and %s", hash1, hash2)
	}
	if len(hash1) != 64 {
		t.Errorf("expected SHA-256 hex length 64, got %d", len(hash1))
	}
}

func TestComputeFileChecksum(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "test.sql")
	content := []byte("-- test migration sql")
	if err := os.WriteFile(tempFile, content, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	checksum, err := ComputeFileChecksum(tempFile)
	if err != nil {
		t.Fatalf("ComputeFileChecksum error: %v", err)
	}
	if checksum != ComputeChecksum(content) {
		t.Errorf("checksum mismatch: got %s, want %s", checksum, ComputeChecksum(content))
	}

	_, err = ComputeFileChecksum(filepath.Join(t.TempDir(), "nonexistent.sql"))
	if err == nil {
		t.Error("expected error for non-existent file, got nil")
	}
}
