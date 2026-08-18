// Package plan computes and formats preview reports of pending database migrations.
package plan

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/parthdagia05/schemago/internal/migration"
)

// Options configures formatting behavior for a migration plan preview.
type Options struct {
	ShowSQL bool `json:"show_sql"`
}

// Plan represents a preview of pending migrations to be applied.
type Plan struct {
	Pending []*migration.MigrationFile `json:"pending"`
}

// BuildPlan constructs a Plan for the provided pending migration files.
func BuildPlan(pending []*migration.MigrationFile) *Plan {
	return &Plan{
		Pending: pending,
	}
}

// FormatPlan writes a human-readable preview of the pending migration plan to w.
func FormatPlan(w io.Writer, p *Plan, opts Options) error {
	if p == nil || len(p.Pending) == 0 {
		_, err := fmt.Fprintln(w, "Nothing to apply. Database is up to date.")
		return err
	}

	total := len(p.Pending)
	if total == 1 {
		if _, err := fmt.Fprintf(w, "Found 1 pending migration to apply:\n\n"); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintf(w, "Found %d pending migrations to apply:\n\n", total); err != nil {
			return err
		}
	}

	for i, file := range p.Pending {
		if _, err := fmt.Fprintf(w, "%d. %s\n", i+1, file.Filename); err != nil {
			return err
		}

		if opts.ShowSQL {
			content, err := os.ReadFile(file.Path)
			if err != nil {
				return fmt.Errorf("failed to read migration file %q: %w", file.Path, err)
			}

			trimmed := strings.TrimSpace(string(content))
			if trimmed == "" {
				if _, err := fmt.Fprintln(w, "   (empty file)"); err != nil {
					return err
				}
			} else {
				lines := strings.Split(trimmed, "\n")
				for _, line := range lines {
					if _, err := fmt.Fprintf(w, "   %s\n", line); err != nil {
						return err
					}
				}
			}

			if i < len(p.Pending)-1 {
				if _, err := fmt.Fprintln(w); err != nil {
					return err
				}
			}
		}
	}

	return nil
}
