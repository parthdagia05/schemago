// Command schemago is a standalone database migration runner.
package main

import (
	"os"

	"github.com/parthdagia05/schemago/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
