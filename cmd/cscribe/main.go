// SPDX-License-Identifier: EUPL-1.2

// Command cscribe is the ChartScribe command line.
package main

import (
	"os"

	"github.com/wstein/chartscribe/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
