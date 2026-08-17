package status

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/parthdagia05/schemago/internal/history"
	"github.com/parthdagia05/schemago/internal/migration"
)

func TestBuildReport(t *testing.T) {
	now := time.Date(2026, 8, 17, 22, 30, 0, 0, time.UTC)

	disc1 := &migration.MigrationFile{Version: 1, Filename: "0001_init.sql", Checksum: "checksum1"}
	disc2 := &migration.MigrationFile{Version: 2, Filename: "0002_add_users.sql", Checksum: "checksum2"}
	disc3 := &migration.MigrationFile{Version: 3, Filename: "0003_add_index.sql", Checksum: "checksum3"}

	app1 := &history.AppliedMigration{Version: 1, Name: "0001_init.sql", AppliedAt: now, Checksum: "checksum1"}

	t.Run("empty discovered and empty applied", func(t *testing.T) {
		report := BuildReport(nil, nil)
		if len(report.Items) != 0 {
			t.Errorf("expected 0 items, got %d", len(report.Items))
		}
		if report.ExitCode() != 0 {
			t.Errorf("expected exit code 0 for empty migrations, got %d", report.ExitCode())
		}
	})

	t.Run("all migrations pending when none applied", func(t *testing.T) {
		discovered := []*migration.MigrationFile{disc1, disc2}
		report := BuildReport(discovered, nil)

		if len(report.Items) != 2 {
			t.Fatalf("expected 2 items, got %d", len(report.Items))
		}
		if report.PendingCount != 2 || report.AppliedCount != 0 {
			t.Errorf("unexpected counts: pending=%d, applied=%d", report.PendingCount, report.AppliedCount)
		}
		if report.Items[0].State != StatePending || report.Items[1].State != StatePending {
			t.Errorf("expected all items to be pending")
		}
		if report.ExitCode() != 1 {
			t.Errorf("expected exit code 1 when pending migrations exist, got %d", report.ExitCode())
		}
	})

	t.Run("partial applied and pending", func(t *testing.T) {
		discovered := []*migration.MigrationFile{disc1, disc2, disc3}
		applied := []*history.AppliedMigration{app1}

		report := BuildReport(discovered, applied)

		if len(report.Items) != 3 {
			t.Fatalf("expected 3 items, got %d", len(report.Items))
		}
		if report.AppliedCount != 1 || report.PendingCount != 2 {
			t.Errorf("unexpected counts: applied=%d, pending=%d", report.AppliedCount, report.PendingCount)
		}
		if report.Items[0].State != StateApplied {
			t.Errorf("expected item 0 to be applied, got %s", report.Items[0].State)
		}
		if report.Items[1].State != StatePending || report.Items[2].State != StatePending {
			t.Errorf("expected items 1 and 2 to be pending")
		}
		if report.ExitCode() != 1 {
			t.Errorf("expected exit code 1 when pending migrations exist, got %d", report.ExitCode())
		}
	})

	t.Run("all migrations applied", func(t *testing.T) {
		app2 := &history.AppliedMigration{Version: 2, Name: "0002_add_users.sql", AppliedAt: now, Checksum: "checksum2"}
		discovered := []*migration.MigrationFile{disc1, disc2}
		applied := []*history.AppliedMigration{app1, app2}

		report := BuildReport(discovered, applied)

		if len(report.Items) != 2 {
			t.Fatalf("expected 2 items, got %d", len(report.Items))
		}
		if report.AppliedCount != 2 || report.PendingCount != 0 {
			t.Errorf("unexpected counts: applied=%d, pending=%d", report.AppliedCount, report.PendingCount)
		}
		if report.ExitCode() != 0 {
			t.Errorf("expected exit code 0 when all migrations applied, got %d", report.ExitCode())
		}
	})

	t.Run("checksum mismatch drift detected", func(t *testing.T) {
		disc1Modified := &migration.MigrationFile{Version: 1, Filename: "0001_init.sql", Checksum: "MODIFIED_CHECKSUM"}
		discovered := []*migration.MigrationFile{disc1Modified}
		applied := []*history.AppliedMigration{app1}

		report := BuildReport(discovered, applied)

		if len(report.Items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(report.Items))
		}
		if report.Items[0].State != StateDrifted {
			t.Errorf("expected item 0 state to be drifted, got %s", report.Items[0].State)
		}
		if report.DriftedCount != 1 {
			t.Errorf("expected DriftedCount=1, got %d", report.DriftedCount)
		}
		if report.ExitCode() != 1 {
			t.Errorf("expected exit code 1 when drift exists, got %d", report.ExitCode())
		}
	})

	t.Run("applied migration missing from disk", func(t *testing.T) {
		applied := []*history.AppliedMigration{app1}

		report := BuildReport(nil, applied)

		if len(report.Items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(report.Items))
		}
		if report.Items[0].State != StateMissing {
			t.Errorf("expected item 0 state to be missing, got %s", report.Items[0].State)
		}
		if report.MissingCount != 1 {
			t.Errorf("expected MissingCount=1, got %d", report.MissingCount)
		}
		if report.ExitCode() != 1 {
			t.Errorf("expected exit code 1 when missing migration exists, got %d", report.ExitCode())
		}
	})
}

func TestFormatReport(t *testing.T) {
	now := time.Date(2026, 8, 17, 22, 30, 0, 0, time.UTC)

	t.Run("format empty report", func(t *testing.T) {
		buf := &bytes.Buffer{}
		err := FormatReport(buf, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "No migrations found.") {
			t.Errorf("expected 'No migrations found.', got: %s", buf.String())
		}
	})

	t.Run("format report with applied, pending, and drift", func(t *testing.T) {
		disc1 := &migration.MigrationFile{Version: 1, Filename: "0001_init.sql", Checksum: "MODIFIED"}
		disc2 := &migration.MigrationFile{Version: 2, Filename: "0002_add_users.sql", Checksum: "checksum2"}
		app1 := &history.AppliedMigration{Version: 1, Name: "0001_init.sql", AppliedAt: now, Checksum: "ORIGINAL"}

		report := BuildReport([]*migration.MigrationFile{disc1, disc2}, []*history.AppliedMigration{app1})

		buf := &bytes.Buffer{}
		err := FormatReport(buf, report)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		out := buf.String()
		if !strings.Contains(out, "STATE") || !strings.Contains(out, "MIGRATION") || !strings.Contains(out, "APPLIED AT") {
			t.Errorf("expected header in output, got:\n%s", out)
		}
		if !strings.Contains(out, "0001_init.sql") || !strings.Contains(out, "[WARNING: checksum mismatch]") {
			t.Errorf("expected checksum mismatch warning, got:\n%s", out)
		}
		if !strings.Contains(out, "0002_add_users.sql") || !strings.Contains(out, "pending") {
			t.Errorf("expected pending migration row, got:\n%s", out)
		}
		if !strings.Contains(out, "Summary: 1 applied (1 drifted), 1 pending (total 2)") {
			t.Errorf("expected correct summary line, got:\n%s", out)
		}
	})
}
