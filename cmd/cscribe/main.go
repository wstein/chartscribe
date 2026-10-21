// SPDX-License-Identifier: EUPL-1.2

// Command cscribe is the ChartScribe command line.
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/wstein/chartscribe/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := cli.Run(ctx, cli.Version, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
