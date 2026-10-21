// SPDX-License-Identifier: EUPL-1.2

// Package cli implements the cscribe command line.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

// Version is the cscribe version. Release builds set it with
// -ldflags "-X github.com/wstein/chartscribe/internal/cli.Version=<version>".
var Version = "dev"

const agentHints = `
Are you an AI? Use these resources ONLY IF your task specifically asks you to:
  Help a human understand, set up or use ChartScribe:
    https://raw.githubusercontent.com/wstein/chartscribe/main/docs/agent-guide.md
  Debug or investigate a problem with ChartScribe:
    https://raw.githubusercontent.com/wstein/chartscribe/main/llms.txt
`

// Exit codes (see specs/001-note-tooling/contracts/validate-output.md).
const (
	exitOK       = 0
	exitFindings = 1
	exitUsage    = 2
	exitEnv      = 3
)

// exitError carries a non-usage exit code out of a command. Run is the only
// place that maps errors to exit codes.
type exitError struct {
	code int
	err  error
}

func (e exitError) Error() string { return e.err.Error() }
func (e exitError) Unwrap() error { return e.err }

// runtimeFailure is a failure that is neither a usage nor an environment error
// (for example a failed write to stdout).
type runtimeFailure struct{ error }

// Run executes cscribe with the given arguments (without the program name)
// and returns the process exit code. Each call owns a fresh command tree.
func Run(ctx context.Context, version string, args []string, stdout, stderr io.Writer) int {
	if err := ctx.Err(); err != nil {
		return report(stderr, err, exitFindings)
	}
	root := newRoot(version, stdout, stderr)
	if args == nil {
		args = []string{} // cobra reads os.Args for nil
	}
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		if isUsage(err) && jsonUsageError(root, args, stdout, err) {
			return exitUsage
		}
		return exitStatus(stderr, err)
	}
	return exitOK
}

func isUsage(err error) bool {
	var coded exitError
	var failure runtimeFailure
	return !errors.As(err, &coded) && !errors.As(err, &failure)
}

func exitStatus(stderr io.Writer, err error) int {
	var coded exitError
	if errors.As(err, &coded) {
		if coded.err != nil {
			report(stderr, coded.err, coded.code)
		}
		return coded.code
	}
	var failure runtimeFailure
	if errors.As(err, &failure) {
		return report(stderr, err, exitFindings)
	}
	return report(stderr, err, exitUsage)
}

func report(stderr io.Writer, err error, code int) int {
	fmt.Fprintln(stderr, "Error:", strings.NewReplacer("\r", " ", "\n", " ").Replace(err.Error()))
	return code
}

func newRoot(version string, stdout, stderr io.Writer) *cobra.Command {
	if version == "" {
		version = "dev"
	}
	root := &cobra.Command{
		Use:           "cscribe",
		Short:         "Validate and maintain a Zettelkasten note collection",
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.NoArgs,
		RunE:          func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetIn(strings.NewReader(""))
	opts := &globalOptions{}
	root.PersistentFlags().StringVar(&opts.root, "root", "", "Collection root (default: the nearest directory with a notes/ folder)")
	root.PersistentFlags().BoolVar(&opts.json, "json", false, "Write one JSON document to stdout")
	root.CompletionOptions.DisableDefaultCmd = true
	// Only the root help carries the agent hint; subcommand help stays plain.
	defaultHelp := root.HelpFunc()
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		defaultHelp(cmd, args)
		if cmd == root {
			fmt.Fprint(cmd.OutOrStdout(), agentHints)
		}
	})
	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print the cscribe version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "cscribe %s\n", version); err != nil {
				return runtimeFailure{err}
			}
			return nil
		},
	})
	root.AddCommand(validateCommand(opts))
	root.AddCommand(initCommand(opts))
	root.AddCommand(newCommand(opts))
	root.AddCommand(exportCommand(opts))
	root.AddCommand(listCommand(opts), tagsCommand(opts), linksCommand(opts), backlinksCommand(opts))
	root.AddCommand(trackingCommand(opts))
	return root
}
