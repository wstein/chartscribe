// SPDX-License-Identifier: EUPL-1.2

// Package cli implements the cscribe command line.
package cli

import (
	"fmt"
	"io"
)

// Version is the cscribe version. Release builds set it with
// -ldflags "-X github.com/wstein/chartscribe/internal/cli.Version=<version>".
var Version = "dev"

const usage = "usage: cscribe version\n"

// Run executes cscribe with the given arguments (without the program name)
// and returns the process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "version":
		if len(args) > 1 {
			fmt.Fprintln(stderr, "cscribe: version takes no arguments")
			return 2
		}
		fmt.Fprintf(stdout, "cscribe %s\n", Version)
		return 0
	default:
		fmt.Fprintf(stderr, "cscribe: unknown command %q\n%s", args[0], usage)
		return 2
	}
}
