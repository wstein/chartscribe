// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/wstein/chartscribe/internal/fix"
	"github.com/wstein/chartscribe/internal/validate"
)

// runFix plans the fixes and, with apply, writes them. Findings and exit status follow validate: they
// describe the collection as it is now (plan) or as it is left (apply).
func runFix(cmd *cobra.Command, opts *globalOptions, root string, rules []string, apply bool) error {
	plan, err := fix.Build(root, rules)
	if err != nil {
		return failure(cmd, opts, exitEnv, "io", err)
	}
	res := validate.Result{Notes: plan.Notes, Findings: plan.Before}
	if apply && len(plan.Edits) > 0 {
		if err := plan.Apply(root); err != nil {
			return failure(cmd, opts, exitEnv, "io", err)
		}
		if res, err = validate.Run(root); err != nil {
			return failure(cmd, opts, exitEnv, "io", err)
		}
	}
	findings := filterRules(res.Findings, rules)
	if opts.json {
		if err := printFixJSON(cmd, plan, res.Notes, findings, apply); err != nil {
			return runtimeFailure{err}
		}
	} else if err := printFixHuman(cmd, plan, res.Notes, findings, apply); err != nil {
		return runtimeFailure{err}
	}
	if len(findings) > 0 {
		return exitError{code: exitFindings}
	}
	return nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

func editedFiles(plan *fix.Plan) int {
	files := map[string]bool{}
	for _, e := range plan.Edits {
		files[e.File] = true
	}
	return len(files)
}

func printFixHuman(cmd *cobra.Command, plan *fix.Plan, notes int, findings []validate.Finding, apply bool) error {
	out, errOut := cmd.OutOrStdout(), cmd.ErrOrStderr()
	for _, f := range findings {
		line := f.Message
		if f.Note != "" {
			line = f.Note + ": " + f.Message
		}
		if _, err := fmt.Fprintln(errOut, line); err != nil {
			return err
		}
	}
	var b strings.Builder
	if apply {
		if len(plan.Edits) > 0 {
			fmt.Fprintf(&b, "fix: applied %s in %s\n", plural(len(plan.Edits), "edit", "edits"), plural(editedFiles(plan), "file", "files"))
		} else {
			b.WriteString("fix: nothing to fix\n")
		}
	}
	fmt.Fprintf(&b, "validate: %d notes, %d findings\n", notes, len(findings))
	if !apply {
		if len(plan.Edits) == 0 {
			b.WriteString("fix: nothing to fix\n")
		} else {
			fmt.Fprintf(&b, "fix: %s in %s planned; findings %d -> %d; --apply writes them\n",
				plural(len(plan.Edits), "edit", "edits"), plural(editedFiles(plan), "file", "files"), len(plan.Before), len(plan.After))
			for _, e := range plan.Edits {
				fmt.Fprintf(&b, "%s:%d [%s]\n", e.File, e.Line, e.Rule)
				if e.Old != "" {
					for _, l := range strings.Split(e.Old, "\n") {
						fmt.Fprintf(&b, "- %s\n", l)
					}
				}
				for _, l := range strings.Split(e.New, "\n") {
					fmt.Fprintf(&b, "+ %s\n", l)
				}
			}
		}
	}
	for _, r := range plan.Refused {
		fmt.Fprintf(&b, "fix refused: %s: %s\n", r.File, r.Reason)
	}
	_, err := fmt.Fprint(out, b.String())
	return err
}

func printFixJSON(cmd *cobra.Command, plan *fix.Plan, notes int, findings []validate.Finding, apply bool) error {
	doc := validateDocument{Version: 1, Command: "validate", OK: len(findings) == 0, Notes: notes, Findings: []jsonFinding{}}
	for _, f := range findings {
		doc.Findings = append(doc.Findings, jsonFinding{Rule: f.Rule, Note: f.Note, Message: f.Message})
	}
	fd := &fixDocument{Mode: "plan", Before: len(plan.Before), After: len(plan.After), Edits: []jsonEdit{}, Refused: []jsonRefuse{}}
	if apply {
		fd.Mode, fd.Applied = "apply", len(plan.Edits) > 0
		fd.After = len(findings)
	}
	for _, e := range plan.Edits {
		fd.Edits = append(fd.Edits, jsonEdit{File: e.File, Line: e.Line, Rule: e.Rule, Old: e.Old, New: e.New})
	}
	for _, r := range plan.Refused {
		fd.Refused = append(fd.Refused, jsonRefuse{File: r.File, Reason: r.Reason})
	}
	doc.Fix = fd
	return writeJSON(cmd.OutOrStdout(), doc)
}
