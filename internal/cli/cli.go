// Package cli routes schemago subcommands.
package cli

import (
	"fmt"
	"io"
	"os"
)

const usage = `schemago - a standalone database migration runner

Usage:
  schemago <command> [flags]

Commands:
  status    Show which migrations have run and which are pending
  plan      Preview the changes that apply would make
  apply     Apply pending migrations, one at a time, in a transaction
  dry-run   Go through apply without touching the database
  help      Show this help

Run "schemago <command> --help" for details on a command.
`

// Run dispatches a subcommand and returns a process exit code.
func Run(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}

	cmd, rest := args[0], args[1:]
	switch cmd {
	case "status":
		return notImplemented(os.Stdout, "status")
	case "plan":
		return notImplemented(os.Stdout, "plan")
	case "apply":
		return notImplemented(os.Stdout, "apply")
	case "dry-run":
		return notImplemented(os.Stdout, "dry-run")
	case "help", "-h", "--help":
		fmt.Fprint(os.Stdout, usage)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "schemago: unknown command %q\n\n%s", cmd, usage)
		_ = rest
		return 2
	}
}

func notImplemented(w io.Writer, name string) int {
	fmt.Fprintf(w, "schemago %s: not implemented yet\n", name)
	return 1
}
