// Package status computes, reports, and formats the execution state of database migrations.
package status

import (
	"fmt"
	"io"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/parthdagia05/schemago/internal/history"
	"github.com/parthdagia05/schemago/internal/migration"
)

// StatusState represents the current application state of a migration script.
type StatusState string

const (
	// StateApplied indicates the migration has been executed against the database and matches local files.
	StateApplied StatusState = "applied"

	// StatePending indicates the migration file exists on disk but has not been applied to the database.
	StatePending StatusState = "pending"

	// StateDrifted indicates the migration was applied but its local file checksum differs from stored checksum.
	StateDrifted StatusState = "drifted"

	// StateMissing indicates an applied migration record exists in the database but is missing from disk.
	StateMissing StatusState = "missing"
)

// Item holds the status summary of a single migration version.
type Item struct {
	Version      int64       `json:"version"`
	Name         string      `json:"name"`
	State        StatusState `json:"state"`
	AppliedAt    *time.Time  `json:"applied_at,omitempty"`
	DBChecksum   string      `json:"db_checksum,omitempty"`
	FileChecksum string      `json:"file_checksum,omitempty"`
}

// Report aggregates migration status items and summary statistics.
type Report struct {
	Items        []*Item `json:"items"`
	AppliedCount int     `json:"applied_count"`
	PendingCount int     `json:"pending_count"`
	DriftedCount int     `json:"drifted_count"`
	MissingCount int     `json:"missing_count"`
}

// HasPending returns true if there are unapplied (pending) migrations.
func (r *Report) HasPending() bool {
	return r.PendingCount > 0
}

// HasDrift returns true if any applied migration has a checksum mismatch or is missing from disk.
func (r *Report) HasDrift() bool {
	return r.DriftedCount > 0 || r.MissingCount > 0
}

// ExitCode returns 0 if all migrations are cleanly applied with no pending or drifted scripts.
// Returns 1 if there are pending migrations or integrity issues (for CI pipeline gating).
func (r *Report) ExitCode() int {
	if r.HasPending() || r.HasDrift() {
		return 1
	}
	return 0
}

// BuildReport calculates the status of all discovered migration files and applied history records.
func BuildReport(discovered []*migration.MigrationFile, applied []*history.AppliedMigration) *Report {
	discMap := make(map[int64]*migration.MigrationFile, len(discovered))
	for _, disc := range discovered {
		discMap[disc.Version] = disc
	}

	appMap := make(map[int64]*history.AppliedMigration, len(applied))
	for _, app := range applied {
		appMap[app.Version] = app
	}

	allVersionsMap := make(map[int64]struct{}, len(discovered)+len(applied))
	for v := range discMap {
		allVersionsMap[v] = struct{}{}
	}
	for v := range appMap {
		allVersionsMap[v] = struct{}{}
	}

	versions := make([]int64, 0, len(allVersionsMap))
	for v := range allVersionsMap {
		versions = append(versions, v)
	}
	sort.Slice(versions, func(i, j int) bool {
		return versions[i] < versions[j]
	})

	report := &Report{
		Items: make([]*Item, 0, len(versions)),
	}

	for _, v := range versions {
		disc := discMap[v]
		app := appMap[v]

		item := &Item{
			Version: v,
		}

		switch {
		case disc != nil && app != nil:
			item.Name = disc.Filename
			appliedTime := app.AppliedAt
			item.AppliedAt = &appliedTime
			item.DBChecksum = app.Checksum
			item.FileChecksum = disc.Checksum

			// Check for checksum drift
			if app.Checksum != "" && disc.Checksum != "" && app.Checksum != disc.Checksum {
				item.State = StateDrifted
				report.DriftedCount++
			} else {
				item.State = StateApplied
			}
			report.AppliedCount++

		case disc != nil && app == nil:
			item.Name = disc.Filename
			item.State = StatePending
			item.FileChecksum = disc.Checksum
			report.PendingCount++

		case disc == nil && app != nil:
			item.Name = app.Name
			appliedTime := app.AppliedAt
			item.AppliedAt = &appliedTime
			item.DBChecksum = app.Checksum
			item.State = StateMissing
			report.MissingCount++
			report.AppliedCount++
		}

		report.Items = append(report.Items, item)
	}

	return report
}

// FormatReport writes a clean, log-friendly tabular view of the status report to w.
func FormatReport(w io.Writer, report *Report) error {
	if report == nil || len(report.Items) == 0 {
		_, err := fmt.Fprintln(w, "No migrations found.")
		return err
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "STATE\tMIGRATION\tAPPLIED AT")

	for _, item := range report.Items {
		var stateStr, appliedAtStr string

		switch item.State {
		case StateApplied:
			stateStr = "applied"
			if item.AppliedAt != nil {
				appliedAtStr = item.AppliedAt.UTC().Format("2006-01-02 15:04:05 UTC")
			} else {
				appliedAtStr = "-"
			}
		case StatePending:
			stateStr = "pending"
			appliedAtStr = "-"
		case StateDrifted:
			stateStr = "applied"
			if item.AppliedAt != nil {
				appliedAtStr = fmt.Sprintf("%s [WARNING: checksum mismatch]", item.AppliedAt.UTC().Format("2006-01-02 15:04:05 UTC"))
			} else {
				appliedAtStr = "- [WARNING: checksum mismatch]"
			}
		case StateMissing:
			stateStr = "applied"
			if item.AppliedAt != nil {
				appliedAtStr = fmt.Sprintf("%s [WARNING: missing from disk]", item.AppliedAt.UTC().Format("2006-01-02 15:04:05 UTC"))
			} else {
				appliedAtStr = "- [WARNING: missing from disk]"
			}
		}

		fmt.Fprintf(tw, "%s\t%s\t%s\n", stateStr, item.Name, appliedAtStr)
	}

	if err := tw.Flush(); err != nil {
		return err
	}

	total := len(report.Items)
	if report.HasDrift() {
		driftDetails := ""
		if report.DriftedCount > 0 && report.MissingCount > 0 {
			driftDetails = fmt.Sprintf(" (%d drifted, %d missing)", report.DriftedCount, report.MissingCount)
		} else if report.DriftedCount > 0 {
			driftDetails = fmt.Sprintf(" (%d drifted)", report.DriftedCount)
		} else if report.MissingCount > 0 {
			driftDetails = fmt.Sprintf(" (%d missing)", report.MissingCount)
		}

		_, err := fmt.Fprintf(w, "\nSummary: %d applied%s, %d pending (total %d)\n",
			report.AppliedCount, driftDetails, report.PendingCount, total)
		return err
	}

	_, err := fmt.Fprintf(w, "\nSummary: %d applied, %d pending (total %d)\n",
		report.AppliedCount, report.PendingCount, total)
	return err
}
