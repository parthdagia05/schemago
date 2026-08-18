// Package cli routes schemago subcommands and handles command-line configuration.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/parthdagia05/schemago/internal/apply"
	"github.com/parthdagia05/schemago/internal/config"
	"github.com/parthdagia05/schemago/internal/db"
	"github.com/parthdagia05/schemago/internal/dryrun"
	"github.com/parthdagia05/schemago/internal/history"
	"github.com/parthdagia05/schemago/internal/lock"
	"github.com/parthdagia05/schemago/internal/migration"
	"github.com/parthdagia05/schemago/internal/plan"
	"github.com/parthdagia05/schemago/internal/status"
)

const (
	// ExitSuccess indicates successful execution with no errors or unapplied/drifted migrations.
	ExitSuccess = 0

	// ExitFailure indicates runtime failure, database error, transaction rollback, or pending/drifted status.
	ExitFailure = 1

	// ExitUsage indicates invalid command usage, missing arguments, or unknown subcommands.
	ExitUsage = 2
)

const usage = `schemago - a standalone database migration runner

Usage:
  schemago [flags] <command> [command flags]

Commands:
  status    Show which migrations have run and which are pending
  plan      Preview the changes that apply would make
  apply     Apply pending migrations, one at a time, in a transaction
  dry-run   Go through apply without touching the database
  help      Show this help

Global Flags:
  --database-url string   PostgreSQL connection string (overrides DATABASE_URL env var)
  --dir string            Directory containing migration files (default "migrations")
  --table string          Schema history table name (default "schemago_migrations")
  --sql                   Show full SQL statements in plan output
  --no-lock               Disable advisory locking during migration apply
  --json                  Output status, plan, apply, dry-run, and errors as structured JSON

Exit Codes:
  0    Success
  1    Execution failure, DB error, transaction rollback, or pending/drifted status
  2    CLI usage error, invalid flags, or unknown command

Run "schemago <command> --help" for details on a command.
`

// CLIErrorResponse structures CLI errors when --json is enabled.
type CLIErrorResponse struct {
	Command  string `json:"command,omitempty"`
	Error    string `json:"error"`
	ExitCode int    `json:"exit_code"`
}

// ParseFlags extracts global and command flags such as --database-url, --dir, --table, --sql, --no-lock, and --json from command arguments,
// preserving positional subcommand arguments.
func ParseFlags(args []string) (dbURL, dir, table string, showSQL, noLock, jsonOutput bool, rest []string) {
	rest = make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--database-url" || arg == "-database-url":
			if i+1 < len(args) {
				dbURL = args[i+1]
				i++
			}
		case strings.HasPrefix(arg, "--database-url="):
			dbURL = strings.TrimPrefix(arg, "--database-url=")
		case strings.HasPrefix(arg, "-database-url="):
			dbURL = strings.TrimPrefix(arg, "-database-url=")

		case arg == "--dir" || arg == "-dir" || arg == "--migrations-dir" || arg == "-migrations-dir":
			if i+1 < len(args) {
				dir = args[i+1]
				i++
			}
		case strings.HasPrefix(arg, "--dir="):
			dir = strings.TrimPrefix(arg, "--dir=")
		case strings.HasPrefix(arg, "-dir="):
			dir = strings.TrimPrefix(arg, "-dir=")
		case strings.HasPrefix(arg, "--migrations-dir="):
			dir = strings.TrimPrefix(arg, "--migrations-dir=")
		case strings.HasPrefix(arg, "-migrations-dir="):
			dir = strings.TrimPrefix(arg, "-migrations-dir=")

		case arg == "--table" || arg == "-table":
			if i+1 < len(args) {
				table = args[i+1]
				i++
			}
		case strings.HasPrefix(arg, "--table="):
			table = strings.TrimPrefix(arg, "--table=")
		case strings.HasPrefix(arg, "-table="):
			table = strings.TrimPrefix(arg, "-table=")

		case arg == "--sql" || arg == "-sql" || arg == "--show-sql" || arg == "--verbose" || arg == "-v":
			showSQL = true

		case arg == "--no-lock" || arg == "-no-lock" || arg == "--lock=false":
			noLock = true

		case arg == "--json" || arg == "-json" || arg == "--output=json":
			jsonOutput = true

		default:
			rest = append(rest, arg)
		}
	}
	return dbURL, dir, table, showSQL, noLock, jsonOutput, rest
}

// ParseGlobalFlags extracts global flags such as --database-url from command arguments.
func ParseGlobalFlags(args []string) (dbURL string, rest []string) {
	dbURL, _, _, _, _, _, rest = ParseFlags(args)
	return dbURL, rest
}

// Run dispatches a subcommand and returns a process exit code.
func Run(args []string) int {
	return RunWithWriters(args, os.Stdout, os.Stderr)
}

// RunWithWriters dispatches subcommands with custom standard output and error writers for testability.
func RunWithWriters(args []string, stdout, stderr io.Writer) int {
	flagDBURL, flagDir, flagTable, flagShowSQL, flagNoLock, flagJSON, rest := ParseFlags(args)

	if len(rest) == 0 {
		if flagJSON {
			writeError(stderr, "", errors.New("no subcommand provided"), true, ExitUsage)
		} else {
			fmt.Fprint(stderr, usage)
		}
		return ExitUsage
	}

	cmd, cmdArgs := rest[0], rest[1:]

	// Secondary flag check in command arguments if placed after command (e.g. schemago apply --database-url <url>)
	if subURL, subDir, subTable, subShowSQL, subNoLock, subJSON, _ := ParseFlags(cmdArgs); subURL != "" || subDir != "" || subTable != "" || subShowSQL || subNoLock || subJSON {
		if flagDBURL == "" {
			flagDBURL = subURL
		}
		if flagDir == "" {
			flagDir = subDir
		}
		if flagTable == "" {
			flagTable = subTable
		}
		if !flagShowSQL {
			flagShowSQL = subShowSQL
		}
		if !flagNoLock {
			flagNoLock = subNoLock
		}
		if !flagJSON {
			flagJSON = subJSON
		}
	}

	switch cmd {
	case "status":
		return handleStatus(stdout, stderr, flagDBURL, flagDir, flagTable, flagJSON)
	case "plan":
		return handlePlan(stdout, stderr, flagDBURL, flagDir, flagTable, flagShowSQL, flagJSON)
	case "apply":
		return handleApply(stdout, stderr, flagDBURL, flagDir, flagTable, flagNoLock, flagJSON)
	case "dry-run":
		return handleDryRun(stdout, stderr, flagDBURL, flagDir, flagTable, flagNoLock, flagJSON)
	case "help", "-h", "--help":
		if flagJSON {
			resp := map[string]string{
				"name":        "schemago",
				"description": "a standalone database migration runner",
				"usage":       "schemago [flags] <command> [command flags]",
			}
			data, _ := json.MarshalIndent(resp, "", "  ")
			fmt.Fprintln(stdout, string(data))
		} else {
			fmt.Fprint(stdout, usage)
		}
		return ExitSuccess
	default:
		if flagJSON {
			writeError(stderr, "", fmt.Errorf("unknown command %q", cmd), true, ExitUsage)
		} else {
			fmt.Fprintf(stderr, "schemago: unknown command %q\n\n%s", cmd, usage)
		}
		return ExitUsage
	}
}

func writeError(w io.Writer, cmdName string, err error, jsonOutput bool, code int) {
	if jsonOutput {
		resp := CLIErrorResponse{
			Command:  cmdName,
			Error:    err.Error(),
			ExitCode: code,
		}
		data, _ := json.MarshalIndent(resp, "", "  ")
		fmt.Fprintln(w, string(data))
	} else {
		if cmdName != "" {
			fmt.Fprintf(w, "schemago %s error: %v\n", cmdName, err)
		} else {
			fmt.Fprintf(w, "schemago error: %v\n", err)
		}
	}
}

func handleStatus(stdout, stderr io.Writer, flagDBURL, flagDir, flagTable string, jsonOutput bool) int {
	cfg, err := config.NewWithOpts(flagDBURL, flagDir, flagTable, config.DefaultTimeout)
	if err != nil {
		writeError(stderr, "status", err, jsonOutput, ExitFailure)
		return ExitFailure
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	conn, err := db.ConnectAndPing(ctx, cfg.DatabaseURL, cfg.Timeout)
	if err != nil {
		writeError(stderr, "status", err, jsonOutput, ExitFailure)
		return ExitFailure
	}
	defer conn.Close()

	if err := history.EnsureTable(ctx, conn, cfg.TableName); err != nil {
		writeError(stderr, "status", err, jsonOutput, ExitFailure)
		return ExitFailure
	}

	applied, err := history.GetAppliedMigrations(ctx, conn, cfg.TableName)
	if err != nil {
		writeError(stderr, "status", err, jsonOutput, ExitFailure)
		return ExitFailure
	}

	discovered, err := migration.Discover(cfg.MigrationsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			discovered = nil
		} else {
			writeError(stderr, "status", err, jsonOutput, ExitFailure)
			return ExitFailure
		}
	}

	report := status.BuildReport(discovered, applied)
	if jsonOutput {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			writeError(stderr, "status", err, jsonOutput, ExitFailure)
			return ExitFailure
		}
		fmt.Fprintln(stdout, string(data))
		return report.ExitCode()
	}

	if err := status.FormatReport(stdout, report); err != nil {
		writeError(stderr, "status", err, jsonOutput, ExitFailure)
		return ExitFailure
	}

	return report.ExitCode()
}

func handlePlan(stdout, stderr io.Writer, flagDBURL, flagDir, flagTable string, showSQL, jsonOutput bool) int {
	cfg, err := config.NewWithOpts(flagDBURL, flagDir, flagTable, config.DefaultTimeout)
	if err != nil {
		writeError(stderr, "plan", err, jsonOutput, ExitFailure)
		return ExitFailure
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	conn, err := db.ConnectAndPing(ctx, cfg.DatabaseURL, cfg.Timeout)
	if err != nil {
		writeError(stderr, "plan", err, jsonOutput, ExitFailure)
		return ExitFailure
	}
	defer conn.Close()

	if err := history.EnsureTable(ctx, conn, cfg.TableName); err != nil {
		writeError(stderr, "plan", err, jsonOutput, ExitFailure)
		return ExitFailure
	}

	applied, err := history.GetAppliedMigrations(ctx, conn, cfg.TableName)
	if err != nil {
		writeError(stderr, "plan", err, jsonOutput, ExitFailure)
		return ExitFailure
	}

	discovered, err := migration.Discover(cfg.MigrationsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			discovered = nil
		} else {
			writeError(stderr, "plan", err, jsonOutput, ExitFailure)
			return ExitFailure
		}
	}

	pending, err := history.ComputePending(discovered, applied)
	if err != nil {
		writeError(stderr, "plan", err, jsonOutput, ExitFailure)
		return ExitFailure
	}

	p := plan.BuildPlan(pending)
	if jsonOutput {
		data, err := json.MarshalIndent(p, "", "  ")
		if err != nil {
			writeError(stderr, "plan", err, jsonOutput, ExitFailure)
			return ExitFailure
		}
		fmt.Fprintln(stdout, string(data))
		return ExitSuccess
	}

	if err := plan.FormatPlan(stdout, p, plan.Options{ShowSQL: showSQL}); err != nil {
		writeError(stderr, "plan", err, jsonOutput, ExitFailure)
		return ExitFailure
	}

	return ExitSuccess
}

func handleApply(stdout, stderr io.Writer, flagDBURL, flagDir, flagTable string, noLock, jsonOutput bool) int {
	cfg, err := config.NewWithOpts(flagDBURL, flagDir, flagTable, config.DefaultTimeout)
	if err != nil {
		writeError(stderr, "apply", err, jsonOutput, ExitFailure)
		return ExitFailure
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	dbConn, err := db.ConnectAndPing(ctx, cfg.DatabaseURL, cfg.Timeout)
	if err != nil {
		writeError(stderr, "apply", err, jsonOutput, ExitFailure)
		return ExitFailure
	}
	defer dbConn.Close()

	conn, err := dbConn.Conn(ctx)
	if err != nil {
		writeError(stderr, "apply", err, jsonOutput, ExitFailure)
		return ExitFailure
	}
	defer conn.Close()

	if !noLock {
		l := lock.New(conn, lock.GenerateLockID(cfg.TableName))
		if err := l.Lock(ctx); err != nil {
			writeError(stderr, "apply", err, jsonOutput, ExitFailure)
			return ExitFailure
		}
		defer func() {
			_ = l.Unlock(context.Background())
		}()
	}

	if err := history.EnsureTable(ctx, conn, cfg.TableName); err != nil {
		writeError(stderr, "apply", err, jsonOutput, ExitFailure)
		return ExitFailure
	}

	applied, err := history.GetAppliedMigrations(ctx, conn, cfg.TableName)
	if err != nil {
		writeError(stderr, "apply", err, jsonOutput, ExitFailure)
		return ExitFailure
	}

	discovered, err := migration.Discover(cfg.MigrationsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			discovered = nil
		} else {
			writeError(stderr, "apply", err, jsonOutput, ExitFailure)
			return ExitFailure
		}
	}

	pending, err := history.ComputePending(discovered, applied)
	if err != nil {
		writeError(stderr, "apply", err, jsonOutput, ExitFailure)
		return ExitFailure
	}

	res, applyErr := apply.Apply(ctx, conn, cfg.TableName, pending)
	if jsonOutput {
		if res != nil {
			data, err := json.MarshalIndent(res, "", "  ")
			if err == nil {
				fmt.Fprintln(stdout, string(data))
			}
		}
		if applyErr != nil {
			if res == nil {
				writeError(stderr, "apply", applyErr, jsonOutput, ExitFailure)
			}
			return ExitFailure
		}
		return ExitSuccess
	}

	if formatErr := apply.FormatResult(stdout, res); formatErr != nil {
		writeError(stderr, "apply", formatErr, jsonOutput, ExitFailure)
		return ExitFailure
	}

	if applyErr != nil {
		writeError(stderr, "apply", applyErr, jsonOutput, ExitFailure)
		return ExitFailure
	}

	return ExitSuccess
}

func handleDryRun(stdout, stderr io.Writer, flagDBURL, flagDir, flagTable string, noLock, jsonOutput bool) int {
	cfg, err := config.NewWithOpts(flagDBURL, flagDir, flagTable, config.DefaultTimeout)
	if err != nil {
		writeError(stderr, "dry-run", err, jsonOutput, ExitFailure)
		return ExitFailure
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	dbConn, err := db.ConnectAndPing(ctx, cfg.DatabaseURL, cfg.Timeout)
	if err != nil {
		writeError(stderr, "dry-run", err, jsonOutput, ExitFailure)
		return ExitFailure
	}
	defer dbConn.Close()

	conn, err := dbConn.Conn(ctx)
	if err != nil {
		writeError(stderr, "dry-run", err, jsonOutput, ExitFailure)
		return ExitFailure
	}
	defer conn.Close()

	if !noLock {
		l := lock.New(conn, lock.GenerateLockID(cfg.TableName))
		if err := l.Lock(ctx); err != nil {
			writeError(stderr, "dry-run", err, jsonOutput, ExitFailure)
			return ExitFailure
		}
		defer func() {
			_ = l.Unlock(context.Background())
		}()
	}

	applied, err := history.GetAppliedMigrations(ctx, conn, cfg.TableName)
	if err != nil {
		writeError(stderr, "dry-run", err, jsonOutput, ExitFailure)
		return ExitFailure
	}

	discovered, err := migration.Discover(cfg.MigrationsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			discovered = nil
		} else {
			writeError(stderr, "dry-run", err, jsonOutput, ExitFailure)
			return ExitFailure
		}
	}

	pending, err := history.ComputePending(discovered, applied)
	if err != nil {
		writeError(stderr, "dry-run", err, jsonOutput, ExitFailure)
		return ExitFailure
	}

	res, dryRunErr := dryrun.DryRun(ctx, conn, cfg.TableName, pending)
	if jsonOutput {
		if res != nil {
			data, err := json.MarshalIndent(res, "", "  ")
			if err == nil {
				fmt.Fprintln(stdout, string(data))
			}
		}
		if dryRunErr != nil {
			if res == nil {
				writeError(stderr, "dry-run", dryRunErr, jsonOutput, ExitFailure)
			}
			return ExitFailure
		}
		return ExitSuccess
	}

	if formatErr := dryrun.FormatResult(stdout, res); formatErr != nil {
		writeError(stderr, "dry-run", formatErr, jsonOutput, ExitFailure)
		return ExitFailure
	}

	if dryRunErr != nil {
		writeError(stderr, "dry-run", dryRunErr, jsonOutput, ExitFailure)
		return ExitFailure
	}

	return ExitSuccess
}
