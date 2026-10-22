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
	"github.com/wstein/chartscribe/internal/fix"
	"github.com/wstein/chartscribe/internal/stale"
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
	// Warnings (source-stale) never change ok or the exit status; present only when there is one.
	Warnings []jsonFinding `json:"warnings,omitempty"`
	Skipped  []string      `json:"skipped,omitempty"`
	Fix      *fixDocument  `json:"fix,omitempty"`
}

type fixDocument struct {
	Mode    string       `json:"mode"` // plan or apply
	Applied bool         `json:"applied"`
	Before  int          `json:"findings_before"`
	After   int          `json:"findings_after"`
	Edits   []jsonEdit   `json:"edits"`
	Refused []jsonRefuse `json:"refused"`
}

type jsonEdit struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Rule string `json:"rule"`
	Old  string `json:"old"`
	New  string `json:"new"`
}

type jsonRefuse struct {
	File   string `json:"file"`
	Reason string `json:"reason"`
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
	var fixFlag, apply, checkSources, gitFlag bool
	cmd := &cobra.Command{
		Use:     "validate",
		Aliases: []string{"lint"},
		Short:   "Check the collection against the note rules",
		Long: `Check every note of the collection against the note rules and report each finding with
the rule name, the note and a message. Files are only read: no network, no write.

--check-sources also verifies that each cited file (Source lines of kind file and measurement)
exists below the source root, is a regular file and, for "lines A-B", that the range is real;
decision and url sources are never fetched. --check-sources --git also warns (source-stale; never
an error, exit status unchanged) when a cited file has a commit newer than the note, using read-only git.
.cscribe.toml keys: source_root, check_sources, check_stale.

Findings go to stderr, the summary to stdout; with --json one document goes to stdout.

--fix plans the safe fixes and shows each exact edit; --apply (with --fix) writes them. Only certain
fixes are planned, and each is kept only if re-running validate on the result shows fewer findings and
no new one. Fixable rules: ` + strings.Join(fix.Rules, ", ") + `. Note bodies are not changed.

Exit status: 0 clean, 1 findings, 2 usage error, 3 environment error.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, r := range rules {
				if !contains(validate.Rules, r) && !contains(validate.WarningRules, r) {
					return failure(cmd, opts, exitUsage, "usage",
						fmt.Errorf("unknown rule %q (known: %s)", r, strings.Join(append(append([]string{}, validate.Rules...), validate.WarningRules...), ", ")))
				}
			}
			if fixFlag && (checkSources || gitFlag) {
				return failure(cmd, opts, exitUsage, "usage", errors.New("--check-sources and --git cannot be combined with --fix"))
			}
			if apply && !fixFlag {
				return failure(cmd, opts, exitUsage, "usage", errors.New("--apply needs --fix"))
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
			cfg, err := collection.LoadConfig(root)
			if err != nil {
				return failure(cmd, opts, exitEnv, "io", err)
			}
			if fixFlag {
				return runFix(cmd, opts, root, rules, apply)
			}
			check := checkSources || cfg.CheckSources
			if gitFlag && !check {
				return failure(cmd, opts, exitUsage, "usage", errors.New("--git needs --check-sources (or check_sources = true)"))
			}
			srcRoot := collection.ResolveSourceRoot(root, cfg)
			res, err := validate.RunWith(root, nil, validate.Options{CheckSources: check, SourceRoot: srcRoot})
			if err != nil {
				return failure(cmd, opts, exitEnv, "io", err)
			}
			findings := filterRules(res.Findings, rules)
			var st *stale.Result
			if check && (gitFlag || cfg.CheckStale) {
				r := stale.Check(root, srcRoot, res.Sources, stale.ExecRunner)
				st = &r
			}
			if err := printValidate(cmd, opts, res.Notes, findings, st, rules); err != nil {
				return runtimeFailure{err}
			}
			if len(findings) > 0 {
				return exitError{code: exitFindings}
			}
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&rules, "rule", nil, "Limit the output to this rule (repeatable or comma-separated)")
	cmd.Flags().BoolVar(&fixFlag, "fix", false, "Plan the safe fixes of the findings (writes nothing without --apply)")
	cmd.Flags().BoolVar(&checkSources, "check-sources", false, "Verify that cited files exist and that their line ranges are real")
	cmd.Flags().BoolVar(&gitFlag, "git", false, "With --check-sources: warn about cited files with a commit newer than the note (read-only git)")
	cmd.Flags().BoolVar(&apply, "apply", false, "With --fix: write the planned fixes")
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

// printValidate writes the findings and, when st is not nil (the staleness check ran), its
// warnings or its skip message. Warnings are filtered by --rule like findings.
func printValidate(cmd *cobra.Command, opts *globalOptions, notes int, findings []validate.Finding, st *stale.Result, rules []string) error {
	var warnings []stale.Warning
	if st != nil {
		for _, w := range st.Warnings {
			if len(rules) == 0 || contains(rules, w.Rule) {
				warnings = append(warnings, w)
			}
		}
	}
	if opts.json {
		doc := validateDocument{Version: 1, Command: "validate", OK: len(findings) == 0, Notes: notes, Findings: []jsonFinding{}}
		for _, f := range findings {
			doc.Findings = append(doc.Findings, jsonFinding{Rule: f.Rule, Note: f.Note, Message: f.Message})
		}
		for _, w := range warnings {
			doc.Warnings = append(doc.Warnings, jsonFinding{Rule: w.Rule, Note: w.Note, Message: w.Message})
		}
		if st != nil {
			doc.Skipped = st.Skipped
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
	for _, w := range warnings {
		if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s: %s\n", w.Note, w.Message); err != nil {
			return err
		}
	}
	if st != nil {
		for _, msg := range st.Skipped {
			if _, err := fmt.Fprintln(cmd.ErrOrStderr(), msg); err != nil {
				return err
			}
		}
	}
	summary := fmt.Sprintf("validate: %d notes, %d findings", notes, len(findings))
	if st != nil {
		summary += fmt.Sprintf(", %d warnings", len(warnings))
	}
	_, err := fmt.Fprintln(cmd.OutOrStdout(), summary)
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
