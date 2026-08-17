// Package cli routes schemago subcommands and handles command-line configuration.
package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/parthdagia05/schemago/internal/config"
	"github.com/parthdagia05/schemago/internal/db"
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

Run "schemago <command> --help" for details on a command.
`

// ParseGlobalFlags extracts global flags such as --database-url from command arguments,
// preserving positional subcommand arguments.
func ParseGlobalFlags(args []string) (dbURL string, rest []string) {
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
		default:
			rest = append(rest, arg)
		}
	}
	return dbURL, rest
}

// Run dispatches a subcommand and returns a process exit code.
func Run(args []string) int {
	return RunWithWriters(args, os.Stdout, os.Stderr)
}

// RunWithWriters dispatches subcommands with custom standard output and error writers for testability.
func RunWithWriters(args []string, stdout, stderr io.Writer) int {
	flagDBURL, rest := ParseGlobalFlags(args)

	if len(rest) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}

	cmd, cmdArgs := rest[0], rest[1:]

	// Secondary flag check in command arguments if placed after command (e.g. schemago status --database-url <url>)
	if flagDBURL == "" {
		subDBURL, _ := ParseGlobalFlags(cmdArgs)
		if subDBURL != "" {
			flagDBURL = subDBURL
		}
	}

	switch cmd {
	case "status":
		return handleDatabaseCommand(stdout, stderr, "status", flagDBURL)
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
