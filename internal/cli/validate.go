// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/wstein/chartscribe/internal/collection"
	"github.com/wstein/chartscribe/internal/validate"
)

// globalOptions are the persistent flags of the root command.
type globalOptions struct {
	root string
	json bool
}

type jsonFinding struct {
	Rule    string `json:"rule"`
	Note    string `json:"note"`
	Message string `json:"message"`
}

type validateDocument struct {
	Version  int           `json:"version"`
	Command  string        `json:"command"`
	OK       bool          `json:"ok"`
	Notes    int           `json:"notes"`
	Findings []jsonFinding `json:"findings"`
}

type errorDocument struct {
	Version int    `json:"version"`
	Command string `json:"command"`
	OK      bool   `json:"ok"`
	Error   string `json:"error"`
	Message string `json:"message"`
}

func writeJSON(w io.Writer, doc any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

// failure turns an error of validate into the documented output and exit code:
// under --json one error document on stdout, otherwise "Error: ..." on stderr
// (written by Run).
func failure(cmd *cobra.Command, opts *globalOptions, code int, kind string, err error) error {
	if !opts.json {
		return exitError{code: code, err: err}
	}
	doc := errorDocument{Version: 1, Command: cmd.Name(), OK: false, Error: kind, Message: err.Error()}
	if werr := writeJSON(cmd.OutOrStdout(), doc); werr != nil {
		return runtimeFailure{werr}
	}
	return exitError{code: code}
}

func validateCommand(opts *globalOptions) *cobra.Command {
	var rules []string
	cmd := &cobra.Command{
		Use:     "validate",
		Aliases: []string{"lint"},
		Short:   "Check the collection against the note rules",
		Long: `Check every note of the collection against the note rules and report each finding with
the rule name, the note and a message. Files are only read: no network, no write.

Findings go to stderr, the summary to stdout; with --json one document goes to stdout.
Exit status: 0 clean, 1 findings, 2 usage error, 3 environment error.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, r := range rules {
				if !contains(validate.Rules, r) {
					return failure(cmd, opts, exitUsage, "usage",
						fmt.Errorf("unknown rule %q (known: %s)", r, strings.Join(validate.Rules, ", ")))
				}
			}
			cwd, err := os.Getwd()
			if err != nil {
				return failure(cmd, opts, exitEnv, "io", err)
			}
			root, err := collection.FindRoot(opts.root, cwd)
			if errors.Is(err, collection.ErrNoCollection) {
				return failure(cmd, opts, exitEnv, "no-collection", err)
			}
			if err != nil {
				return failure(cmd, opts, exitEnv, "io", err)
			}
			res, err := validate.Run(root)
			if err != nil {
				return failure(cmd, opts, exitEnv, "io", err)
			}
			findings := filterRules(res.Findings, rules)
			if err := printValidate(cmd, opts, res.Notes, findings); err != nil {
				return runtimeFailure{err}
			}
			if len(findings) > 0 {
				return exitError{code: exitFindings}
			}
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&rules, "rule", nil, "Limit the output to this rule (repeatable or comma-separated)")
	return cmd
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func filterRules(in []validate.Finding, rules []string) []validate.Finding {
	if len(rules) == 0 {
		return in
	}
	var out []validate.Finding
	for _, f := range in {
		if contains(rules, f.Rule) {
			out = append(out, f)
		}
	}
	return out
}

func printValidate(cmd *cobra.Command, opts *globalOptions, notes int, findings []validate.Finding) error {
	if opts.json {
		doc := validateDocument{Version: 1, Command: "validate", OK: len(findings) == 0, Notes: notes, Findings: []jsonFinding{}}
		for _, f := range findings {
			doc.Findings = append(doc.Findings, jsonFinding{Rule: f.Rule, Note: f.Note, Message: f.Message})
		}
		return writeJSON(cmd.OutOrStdout(), doc)
	}
	for _, f := range findings {
		line := f.Message
		if f.Note != "" {
			line = f.Note + ": " + f.Message
		}
		if _, err := fmt.Fprintln(cmd.ErrOrStderr(), line); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "validate: %d notes, %d findings\n", notes, len(findings))
	return err
}

// jsonUsageError reports a usage error of validate or init (an unknown flag or an
// extra argument, found by cobra before the command runs) as the JSON error
// document when the arguments ask for --json.
func jsonUsageError(root *cobra.Command, args []string, stdout io.Writer, err error) bool {
	jsonRequested := false
	for _, a := range args {
		if a == "--" {
			break
		}
		if a == "--json" || a == "--json=true" {
			jsonRequested = true
		}
	}
	if !jsonRequested {
		return false
	}
	target, _, ferr := root.Find(args)
	if ferr != nil || target == nil || (!jsonCommands[target.Name()]) {
		return false
	}
	doc := errorDocument{Version: 1, Command: target.Name(), OK: false, Error: "usage", Message: err.Error()}
	return writeJSON(stdout, doc) == nil
}

// jsonCommands are the commands whose usage errors are reported as JSON under --json.
var jsonCommands = map[string]bool{"validate": true, "init": true, "new": true, "list": true, "tags": true, "links": true, "backlinks": true}
