// Package cli routes schemago subcommands and handles command-line configuration.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/parthdagia05/schemago/internal/config"
	"github.com/parthdagia05/schemago/internal/db"
	"github.com/parthdagia05/schemago/internal/history"
	"github.com/parthdagia05/schemago/internal/migration"
	"github.com/parthdagia05/schemago/internal/status"
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

Run "schemago <command> --help" for details on a command.
`

// ParseFlags extracts global flags such as --database-url, --dir, and --table from command arguments,
// preserving positional subcommand arguments.
func ParseFlags(args []string) (dbURL, dir, table string, rest []string) {
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

		default:
			rest = append(rest, arg)
		}
	}
	return dbURL, dir, table, rest
}

// ParseGlobalFlags extracts global flags such as --database-url from command arguments.
func ParseGlobalFlags(args []string) (dbURL string, rest []string) {
	dbURL, _, _, rest = ParseFlags(args)
	return dbURL, rest
}

// Run dispatches a subcommand and returns a process exit code.
func Run(args []string) int {
	return RunWithWriters(args, os.Stdout, os.Stderr)
}

// RunWithWriters dispatches subcommands with custom standard output and error writers for testability.
func RunWithWriters(args []string, stdout, stderr io.Writer) int {
	flagDBURL, flagDir, flagTable, rest := ParseFlags(args)

	if len(rest) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}

	cmd, cmdArgs := rest[0], rest[1:]

	// Secondary flag check in command arguments if placed after command (e.g. schemago status --database-url <url>)
	if subURL, subDir, subTable, _ := ParseFlags(cmdArgs); subURL != "" || subDir != "" || subTable != "" {
		if flagDBURL == "" {
			flagDBURL = subURL
		}
		if flagDir == "" {
			flagDir = subDir
		}
		if flagTable == "" {
			flagTable = subTable
		}
	}

	switch cmd {
	case "status":
		return handleStatus(stdout, stderr, flagDBURL, flagDir, flagTable)
	case "plan":
		return handleDatabaseCommand(stdout, stderr, "plan", flagDBURL)
	case "apply":
		return handleDatabaseCommand(stdout, stderr, "apply", flagDBURL)
	case "dry-run":
		return handleDatabaseCommand(stdout, stderr, "dry-run", flagDBURL)
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "schemago: unknown command %q\n\n%s", cmd, usage)
		return 2
	}
}

func handleStatus(stdout, stderr io.Writer, flagDBURL, flagDir, flagTable string) int {
	cfg, err := config.NewWithOpts(flagDBURL, flagDir, flagTable, config.DefaultTimeout)
	if err != nil {
		fmt.Fprintf(stderr, "schemago status error: %v\n", err)
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	conn, err := db.ConnectAndPing(ctx, cfg.DatabaseURL, cfg.Timeout)
	if err != nil {
		fmt.Fprintf(stderr, "schemago status error: %v\n", err)
		return 1
	}
	defer conn.Close()

	if err := history.EnsureTable(ctx, conn, cfg.TableName); err != nil {
		fmt.Fprintf(stderr, "schemago status error: %v\n", err)
		return 1
	}

	applied, err := history.GetAppliedMigrations(ctx, conn, cfg.TableName)
	if err != nil {
		fmt.Fprintf(stderr, "schemago status error: %v\n", err)
		return 1
	}

	discovered, err := migration.Discover(cfg.MigrationsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			discovered = nil
		} else {
			fmt.Fprintf(stderr, "schemago status error: %v\n", err)
			return 1
		}
	}

	report := status.BuildReport(discovered, applied)
	if err := status.FormatReport(stdout, report); err != nil {
		fmt.Fprintf(stderr, "schemago status error: %v\n", err)
		return 1
	}

	return report.ExitCode()
}

func handleDatabaseCommand(stdout, stderr io.Writer, cmdName string, flagDBURL string) int {
	cfg, err := config.New(flagDBURL, config.DefaultTimeout)
	if err != nil {
		fmt.Fprintf(stderr, "schemago %s error: %v\n", cmdName, err)
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	conn, err := db.ConnectAndPing(ctx, cfg.DatabaseURL, cfg.Timeout)
	if err != nil {
		fmt.Fprintf(stderr, "schemago %s error: %v\n", cmdName, err)
		return 1
	}
	defer conn.Close()

	return notImplemented(stdout, cmdName)
}

func notImplemented(w io.Writer, name string) int {
	fmt.Fprintf(w, "schemago %s: not implemented yet\n", name)
	return 1
}

