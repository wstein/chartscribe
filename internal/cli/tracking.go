// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/wstein/chartscribe/internal/note"
	"github.com/wstein/chartscribe/internal/tracking"
)

// ghRunner starts gh; tests replace it so that the real gh is never called.
var ghRunner tracking.Runner = tracking.ExecRunner{}

type jsonIssue struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"`
	URL    string `json:"url"`
}

type trackingNote struct {
	ID     string      `json:"id"`
	Title  string      `json:"title"`
	Path   string      `json:"path"`
	Action string      `json:"action"` // add, update or remove
	Issues []jsonIssue `json:"issues"`
}

type trackingDocument struct {
	Version int            `json:"version"`
	Command string         `json:"command"`
	OK      bool           `json:"ok"`
	Write   bool           `json:"write"`
	Notes   int            `json:"notes"`   // notes read
	Issues  int            `json:"issues"`  // issues returned by gh
	Changed int            `json:"changed"` // notes whose section changes (changed with --write)
	Plan    []trackingNote `json:"plan"`
}

// trackingPlan is the new text of one note.
type trackingPlan struct {
	entry trackingNote
	path  string
	text  string
	mode  os.FileMode
}

func trackingCommand(opts *globalOptions) *cobra.Command {
	var write bool
	cmd := &cobra.Command{
		Use:   "tracking",
		Short: "Rewrite the Tracking section of the notes from GitHub issues",
		Long: `Read the issues of the repository of the working directory with
gh issue list --state all --limit 200 and rewrite only the generated "## Tracking"
section of each note: one line "- #<number> <title> (<state>) <url>" per issue whose
title or body has a line "Chartroom: <id> <title>" (the id of the note). A note without
issues loses the section; a note without the section gets it after its last line.
Without --write only the plan is printed: path, add, update or remove, and the issue
count. Bytes outside the section never change. A note with CRLF line ends is reported
on stderr and left alone. With --json: {write, notes, issues, changed, plan: [{id,
title, path, action, issues}]}.
Exit status: 0 done, 2 usage error, 3 environment error (no collection, gh missing or
failing, 200 issues returned, a note that cannot be written).`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			x, err := loadIndex(cmd, opts)
			if err != nil {
				return err
			}
			issues, err := tracking.Fetch(cmd.Context(), ghRunner)
			if err != nil {
				return failure(cmd, opts, exitEnv, "gh", err)
			}
			by := tracking.ByNote(issues)
			var plans []trackingPlan
			for _, n := range x.Notes {
				path := filepath.Join(x.Root, filepath.FromSlash(n.Path))
				p, ok, err := planTracking(cmd, n.ID, n.Title, n.Path, path, by[n.ID])
				if err != nil {
					return failure(cmd, opts, exitEnv, "io", err)
				}
				if ok {
					plans = append(plans, p)
				}
			}
			if write {
				for _, p := range plans {
					if err := os.WriteFile(p.path, []byte(p.text), p.mode); err != nil {
						return failure(cmd, opts, exitEnv, "io", err)
					}
				}
			}
			doc := trackingDocument{Version: 1, Command: "tracking", OK: true, Write: write, Notes: len(x.Notes), Issues: len(issues), Changed: len(plans), Plan: []trackingNote{}}
			for _, p := range plans {
				doc.Plan = append(doc.Plan, p.entry)
			}
			if opts.json {
				return finish(writeJSON(cmd.OutOrStdout(), doc))
			}
			var rows []string
			for _, p := range plans {
				rows = append(rows, fmt.Sprintf("%s\t%s\t%d", p.entry.Path, p.entry.Action, len(p.entry.Issues)))
			}
			if write {
				rows = append(rows, fmt.Sprintf("tracking: %d notes, %d changed", doc.Notes, doc.Changed))
			} else {
				rows = append(rows, fmt.Sprintf("tracking: %d notes, %d to change (dry run; --write applies)", doc.Notes, doc.Changed))
			}
			return finish(lines(cmd.OutOrStdout(), rows))
		},
	}
	cmd.Flags().BoolVar(&write, "write", false, "Write the changed sections (default: print the plan only)")
	return cmd
}

// planTracking computes the new text of one note from the file as it is on
// disk (not the normalised copy of the index). ok is false when nothing changes.
func planTracking(cmd *cobra.Command, id, title, rel, path string, issues []tracking.Issue) (p trackingPlan, ok bool, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return p, false, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return p, false, err
	}
	text := string(data)
	if strings.Contains(text, "\r") {
		fmt.Fprintf(cmd.ErrOrStderr(), "skipped %s: CRLF line ends are not rewritten\n", rel)
		return p, false, nil
	}
	_, body, perr := note.Parse(text)
	if perr != nil || !strings.HasSuffix(text, body) {
		return p, false, nil
	}
	lines := make([]string, 0, len(issues))
	entry := trackingNote{ID: id, Title: title, Path: rel, Issues: []jsonIssue{}}
	for _, is := range issues {
		lines = append(lines, tracking.Line(is))
		entry.Issues = append(entry.Issues, jsonIssue{is.Number, is.Title, strings.ToLower(is.State), is.URL})
	}
	newBody := note.ReplaceTracking(body, lines)
	if newBody == body {
		return p, false, nil
	}
	had := note.StripTracking(body) != body
	switch {
	case len(lines) == 0:
		entry.Action = "remove"
	case had:
		entry.Action = "update"
	default:
		entry.Action = "add"
	}
	head := text[:len(text)-len(body)]
	return trackingPlan{entry: entry, path: path, text: head + newBody, mode: info.Mode().Perm()}, true, nil
}
