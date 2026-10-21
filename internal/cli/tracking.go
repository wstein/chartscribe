// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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

// trackingPlan is the new text of one note and the state of the file it was
// computed from.
type trackingPlan struct {
	entry trackingNote
	path  string
	old   string
	text  string
	mode  os.FileMode
	size  int64
	mtime time.Time
}

// beforeWrite runs between planning and writing; tests use it to change a note.
var beforeWrite = func() {}

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
				p, ok, err := planTracking(cmd, x.Root, n.ID, n.Title, n.Path, path, by[n.ID])
				if err != nil {
					return failure(cmd, opts, exitEnv, "io", err)
				}
				if ok {
					plans = append(plans, p)
				}
			}
			if write {
				if err := applyTracking(plans); err != nil {
					return failure(cmd, opts, exitEnv, "io", err)
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
// disk (not the normalised copy of the index). ok is false when nothing changes
// or the note must not be touched (with a warning on stderr).
func planTracking(cmd *cobra.Command, root, id, title, rel, path string, issues []tracking.Issue) (p trackingPlan, ok bool, err error) {
	skip := func(why string) (trackingPlan, bool, error) {
		fmt.Fprintf(cmd.ErrOrStderr(), "skipped %s: %s\n", rel, why)
		return p, false, nil
	}
	info, err := os.Lstat(path)
	if err != nil {
		return p, false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return skip("a symbolic link is not rewritten")
	}
	if !info.Mode().IsRegular() {
		return skip("not a regular file")
	}
	if real, err := filepath.EvalSymlinks(path); err != nil || !within(root, real) {
		return skip("outside the collection")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return p, false, err
	}
	text := string(data)
	if strings.Contains(text, "\r") {
		return skip("CRLF line ends are not rewritten")
	}
	_, body, perr := note.Parse(text)
	if perr != nil || !strings.HasSuffix(text, body) {
		return p, false, nil
	}
	if note.StaleTracking(body) {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning %s: a second ## Tracking section is stale and stays; remove it by hand\n", rel)
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
	return trackingPlan{entry: entry, path: path, old: text, text: head + newBody,
		mode: info.Mode().Perm(), size: info.Size(), mtime: info.ModTime()}, true, nil
}

// within reports whether path is inside the real path of root.
func within(root, path string) bool {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(realRoot, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// applyTracking writes all plans or none: it creates every temporary file
// first, checks that each note is unchanged since planning, then renames. A
// rename that fails after others succeeded is reported with both lists.
func applyTracking(plans []trackingPlan) error {
	if len(plans) == 0 {
		return nil
	}
	temps := make([]string, len(plans))
	cleanup := func() {
		for _, t := range temps {
			if t != "" {
				_ = os.Remove(t)
			}
		}
	}
	allUnwritten := func() string {
		var names []string
		for _, p := range plans {
			names = append(names, p.entry.Path)
		}
		return strings.Join(names, ", ")
	}
	for i, p := range plans {
		if p.mode&0o200 == 0 {
			cleanup()
			return fmt.Errorf("%s is not writable; not written: %s", p.entry.Path, allUnwritten())
		}
		f, err := os.CreateTemp(filepath.Dir(p.path), ".cscribe-tracking-*")
		if err != nil {
			cleanup()
			return fmt.Errorf("%s: %v; not written: %s", p.entry.Path, err, allUnwritten())
		}
		temps[i] = f.Name()
		_, werr := f.WriteString(p.text)
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		if werr == nil {
			werr = os.Chmod(f.Name(), p.mode)
		}
		if werr != nil {
			cleanup()
			return fmt.Errorf("%s: %v; not written: %s", p.entry.Path, werr, allUnwritten())
		}
	}
	beforeWrite()
	for _, p := range plans {
		info, err := os.Lstat(p.path)
		data, rerr := os.ReadFile(p.path)
		if err != nil || rerr != nil || info.Mode()&os.ModeSymlink != 0 || info.Size() != p.size || !info.ModTime().Equal(p.mtime) || string(data) != p.old {
			cleanup()
			return fmt.Errorf("%s changed since it was read; not written: %s", p.entry.Path, allUnwritten())
		}
	}
	var written, rest []string
	for i, p := range plans {
		if err := os.Rename(temps[i], p.path); err != nil {
			cleanup()
			for _, q := range plans[i:] {
				rest = append(rest, q.entry.Path)
			}
			return fmt.Errorf("%s: %v; written: %s; not written: %s", p.entry.Path, err, strings.Join(written, ", "), strings.Join(rest, ", "))
		}
		temps[i] = ""
		written = append(written, p.entry.Path)
	}
	return nil
}
