// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/wstein/chartscribe/internal/collection"
	"github.com/wstein/chartscribe/internal/pin"
	"github.com/wstein/chartscribe/internal/sources"
)

// sourcesGit is the git of the sources commands; tests may replace it.
var sourcesGit pin.Git = pin.ExecGit{}

type jsonSource struct {
	Note    string   `json:"note"`
	Line    int      `json:"line"`
	Path    string   `json:"path"`
	From    int      `json:"from"`
	To      int      `json:"to"`
	Pin     string   `json:"pin,omitempty"`
	Status  string   `json:"status"`
	NewFrom int      `json:"new_from,omitempty"`
	NewTo   int      `json:"new_to,omitempty"`
	NewPath string   `json:"new_path,omitempty"`
	Action  string   `json:"action"`
	Reason  string   `json:"reason,omitempty"`
	Hunks   []string `json:"hunks,omitempty"`
}

type sourceCounts struct {
	Current  int `json:"current"`
	Moved    int `json:"moved"`
	Changed  int `json:"changed"`
	Renamed  int `json:"renamed"`
	Missing  int `json:"missing"`
	Unknown  int `json:"unknown"`
	Unpinned int `json:"unpinned"`
}

type sourcesPlanDocument struct {
	Mode    string       `json:"mode"` // plan or apply
	Applied bool         `json:"applied"`
	Edits   []jsonEdit   `json:"edits"`
	Refused []jsonRefuse `json:"refused"`
}

type sourcesDocument struct {
	Version int                  `json:"version"`
	Command string               `json:"command"`
	OK      bool                 `json:"ok"`
	Notes   int                  `json:"notes"`
	Counts  sourceCounts         `json:"counts"`
	Sources []jsonSource         `json:"sources"`
	Skipped []string             `json:"skipped,omitempty"`
	Plan    *sourcesPlanDocument `json:"plan,omitempty"`
}

func sourcesCommand(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sources",
		Short: "Pin, check and update the line ranges of file Source lines",
		Long: `Keep the line ranges of file Source lines true after the cited file changes.

A file Source line can carry the commit its range was cited at:
  Source: <path>, lines A-B @<commit>
check compares the range with the file at HEAD (read-only git) and reports each pinned source as
current, moved (same text, other lines), changed (the text was edited), renamed or missing.
pin adds the pin to unpinned lines; update rewrites moved ranges. Both plan by default and write only
with --apply. Exit status: 0 nothing stale, 1 stale sources, 2 usage error, 3 environment error.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	cmd.AddCommand(sourcesCheckCommand(opts), sourcesPinCommand(opts), sourcesUpdateCommand(opts))
	return cmd
}

// sourcesEnv is what the three commands share: the collection and the notes they act on.
type sourcesEnv struct {
	root, srcRoot string
	only          []string
}

func sourcesSetup(cmd *cobra.Command, opts *globalOptions, args []string, all, required bool) (*sourcesEnv, error) {
	if required && all == (len(args) > 0) {
		return nil, failure(cmd, opts, exitUsage, "usage", errors.New("name the notes to act on, or use --all (not both)"))
	}
	if all && len(args) > 0 {
		return nil, failure(cmd, opts, exitUsage, "usage", errors.New("--all cannot be combined with note paths"))
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, failure(cmd, opts, exitEnv, "io", err)
	}
	root, err := collection.FindRoot(opts.root, cwd)
	if errors.Is(err, collection.ErrNoCollection) {
		return nil, failure(cmd, opts, exitEnv, "no-collection", err)
	}
	if err != nil {
		return nil, failure(cmd, opts, exitEnv, "io", err)
	}
	cfg, err := collection.LoadConfig(root)
	if err != nil {
		return nil, failure(cmd, opts, exitEnv, "io", err)
	}
	env := &sourcesEnv{root: root, srcRoot: collection.ResolveSourceRoot(root, cfg)}
	for _, a := range args {
		name, err := noteArg(root, cwd, a)
		if err != nil {
			return nil, failure(cmd, opts, exitUsage, "usage", err)
		}
		env.only = append(env.only, name)
	}
	return env, nil
}

// noteArg turns a path (relative to the working directory, to the collection root or to notes/) into the
// file name of a note directly below notes/.
func noteArg(root, cwd, arg string) (string, error) {
	cands := []string{arg, filepath.Join(root, arg), filepath.Join(root, "notes", arg)}
	if !filepath.IsAbs(arg) {
		cands[0] = filepath.Join(cwd, arg)
	}
	notes := filepath.Join(root, "notes")
	for _, c := range cands {
		c = filepath.Clean(c)
		if filepath.Dir(c) != notes || !strings.HasSuffix(c, ".md") {
			continue
		}
		if info, err := os.Lstat(c); err == nil && !info.IsDir() {
			return filepath.Base(c), nil
		}
	}
	return "", fmt.Errorf("%q is not a note of the collection (a .md file directly in notes/)", arg)
}

// sourcesFailure maps the errors of the sources package to the exit status.
func sourcesFailure(cmd *cobra.Command, opts *globalOptions, err error) error {
	switch {
	case errors.Is(err, pin.ErrNoGit), errors.Is(err, pin.ErrNoRepo):
		return failure(cmd, opts, exitEnv, "git", err)
	default:
		return failure(cmd, opts, exitEnv, "io", err)
	}
}

func sourcesCheckCommand(opts *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "check [NOTE...]",
		Short: "Report pinned Source ranges that moved, changed or lost their file (read-only)",
		Long: `Compare the range of each pinned file Source line with the cited file at HEAD, using read-only git
(git diff -U0 from the pinned commit): current, moved (the same text at other lines; the new range is
shown), changed (the text was edited), renamed, missing, or unknown (with the reason: shallow clone,
uncommitted or untracked file, unknown pin). Unpinned lines are only counted. Writes nothing.
Without notes all notes are checked. Exit status 1 when a source is moved, changed, renamed or missing.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := sourcesSetup(cmd, opts, args, false, false)
			if err != nil {
				return err
			}
			rep, err := sources.Check(env.root, env.srcRoot, sourcesGit, env.only)
			if err != nil {
				return sourcesFailure(cmd, opts, err)
			}
			return finishSources(cmd, opts, env.root, "sources check", rep, nil, false)
		},
	}
}

func sourcesPinCommand(opts *globalOptions) *cobra.Command {
	var all, apply bool
	var rev string
	cmd := &cobra.Command{
		Use:   "pin [NOTE... | --all]",
		Short: "Add @<commit> to unpinned file Source lines (plan; --apply writes)",
		Long: `Add the pin @<commit> (12 hex digits) to each unpinned file Source line of the named notes (or --all).
Pinning vouches that the cited range is right now: the range must be valid in the file, the file tracked
and without uncommitted changes, and the pin is HEAD. --rev <commit> pins to a commit you know the range
was right at (the file must exist there with enough lines); check then reports what moved since.
Only the Source line changes. Plans by default; --apply writes all notes or none.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := sourcesSetup(cmd, opts, args, all, true)
			if err != nil {
				return err
			}
			plan, err := sources.PlanPin(env.root, env.srcRoot, sourcesGit, env.only, rev)
			if err != nil {
				return sourcesFailure(cmd, opts, err)
			}
			return finishSources(cmd, opts, env.root, "sources pin", &plan.Report, plan, apply)
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "Act on every note of the collection")
	cmd.Flags().BoolVar(&apply, "apply", false, "Write the planned pins")
	cmd.Flags().StringVar(&rev, "rev", "", "Pin to this commit (7 to 40 lower-case hex digits) instead of HEAD")
	return cmd
}

func sourcesUpdateCommand(opts *globalOptions) *cobra.Command {
	var all, apply bool
	cmd := &cobra.Command{
		Use:   "update [NOTE... | --all]",
		Short: "Rewrite moved ranges and move their pin to HEAD (plan; --apply writes)",
		Long: `For each moved pinned source of the named notes (or --all), plan the new range and move the pin to
HEAD: the cited text is the same, only its lines changed. Changed, renamed and missing sources are never
rewritten; they are listed for review with the hunks that touched the range. Only the Source line changes.
Plans by default; --apply writes all notes or none, after validating the result again. Exit status 1 when
a source still needs review (or, in a plan, is stale).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := sourcesSetup(cmd, opts, args, all, true)
			if err != nil {
				return err
			}
			plan, err := sources.PlanUpdate(env.root, env.srcRoot, sourcesGit, env.only)
			if err != nil {
				return sourcesFailure(cmd, opts, err)
			}
			return finishSources(cmd, opts, env.root, "sources update", &plan.Report, plan, apply)
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "Act on every note of the collection")
	cmd.Flags().BoolVar(&apply, "apply", false, "Write the planned updates")
	return cmd
}

func isStale(s string) bool {
	return s == string(pin.Moved) || s == string(pin.Changed) || s == string(pin.Renamed) || s == string(pin.Missing)
}

// finishSources applies the plan when asked, writes the report and returns the exit status.
func finishSources(cmd *cobra.Command, opts *globalOptions, root, label string, rep *sources.Report, plan *sources.Plan, apply bool) error {
	applied := false
	if plan != nil && apply && len(plan.Edits) > 0 {
		if err := plan.Apply(root); err != nil {
			return failure(cmd, opts, exitEnv, "io", err)
		}
		applied = true
	}
	var doc sourcesDocument
	doc.Version, doc.Command, doc.Notes, doc.Skipped = 1, label, rep.Notes, rep.Skipped
	doc.Sources = []jsonSource{}
	remaining := 0
	for _, r := range rep.Rows {
		js := jsonSource{Note: r.Note, Line: r.Line, Path: r.Source.Path, From: r.Source.From, To: r.Source.To, Pin: r.Source.Pin,
			Status: string(r.Result.Status), Action: r.Action, Reason: r.Reason, Hunks: r.Result.Hunks}
		if r.Result.Status == pin.Moved {
			js.NewFrom, js.NewTo = r.Result.From, r.Result.To
		}
		if r.Result.Status == pin.Renamed {
			js.NewPath = r.Result.NewPath
		}
		doc.Sources = append(doc.Sources, js)
		status := r.Result.Status
		if applied && r.Action == sources.ActionUpdate {
			status = pin.Current // the edit was written
		}
		switch status {
		case pin.Current:
			doc.Counts.Current++
		case pin.Moved:
			doc.Counts.Moved++
		case pin.Changed:
			doc.Counts.Changed++
		case pin.Renamed:
			doc.Counts.Renamed++
		case pin.Missing:
			doc.Counts.Missing++
		case pin.Unknown:
			doc.Counts.Unknown++
		default:
			doc.Counts.Unpinned++
		}
		if isStale(string(status)) {
			remaining++
		}
	}
	exit := remaining > 0 && label != "sources pin"
	doc.OK = !exit
	if plan != nil {
		pd := &sourcesPlanDocument{Mode: "plan", Applied: applied, Edits: []jsonEdit{}, Refused: []jsonRefuse{}}
		if apply {
			pd.Mode = "apply"
		}
		for _, e := range plan.Edits {
			pd.Edits = append(pd.Edits, jsonEdit{File: e.File, Line: e.Line, Rule: e.Rule, Old: e.Old, New: e.New})
		}
		for _, r := range plan.Refused {
			pd.Refused = append(pd.Refused, jsonRefuse{File: r.File, Reason: r.Reason})
		}
		doc.Plan = pd
	}
	if opts.json {
		if err := writeJSON(cmd.OutOrStdout(), doc); err != nil {
			return runtimeFailure{err}
		}
	} else if err := printSourcesHuman(cmd, label, rep, plan, doc, applied, apply); err != nil {
		return runtimeFailure{err}
	}
	if exit {
		return exitError{code: exitFindings}
	}
	return nil
}

func printSourcesHuman(cmd *cobra.Command, label string, rep *sources.Report, plan *sources.Plan, doc sourcesDocument, applied, apply bool) error {
	var b strings.Builder
	for _, s := range doc.Sources {
		switch pin.Status(s.Status) {
		case pin.Current, pin.Unpinned:
			if s.Action != sources.ActionSkip && s.Action != sources.ActionPin {
				continue
			}
		}
		fmt.Fprintf(&b, "%s:%d %s lines %d-%d", s.Note, s.Line, s.Path, s.From, s.To)
		if s.Pin != "" {
			fmt.Fprintf(&b, " @%s", s.Pin)
		}
		switch pin.Status(s.Status) {
		case pin.Moved:
			fmt.Fprintf(&b, ": moved to lines %d-%d", s.NewFrom, s.NewTo)
		case pin.Renamed:
			fmt.Fprintf(&b, ": renamed to %s", s.NewPath)
		case pin.Unpinned:
			fmt.Fprintf(&b, ": unpinned")
		default:
			fmt.Fprintf(&b, ": %s", s.Status)
		}
		if s.Reason != "" && pin.Status(s.Status) != pin.Moved {
			fmt.Fprintf(&b, " (%s)", s.Reason)
		}
		if len(s.Hunks) > 0 {
			fmt.Fprintf(&b, " %s", strings.Join(s.Hunks, " "))
		}
		b.WriteByte('\n')
	}
	for _, msg := range rep.Skipped {
		fmt.Fprintln(cmd.ErrOrStderr(), msg)
	}
	c := doc.Counts
	fmt.Fprintf(&b, "%s: %d notes, %d file sources: %d current, %d moved, %d changed, %d renamed, %d missing, %d unknown, %d unpinned\n",
		label, rep.Notes, len(doc.Sources), c.Current, c.Moved, c.Changed, c.Renamed, c.Missing, c.Unknown, c.Unpinned)
	if plan != nil {
		switch {
		case len(plan.Edits) == 0:
			b.WriteString("nothing to write\n")
		case applied:
			fmt.Fprintf(&b, "applied %s in %s\n", plural(len(plan.Edits), "edit", "edits"), plural(editedSourceFiles(plan), "file", "files"))
		default:
			fmt.Fprintf(&b, "%s in %s planned; --apply writes them\n", plural(len(plan.Edits), "edit", "edits"), plural(editedSourceFiles(plan), "file", "files"))
		}
		if !apply || !applied {
			for _, e := range plan.Edits {
				fmt.Fprintf(&b, "%s:%d [%s]\n- %s\n+ %s\n", e.File, e.Line, e.Rule, e.Old, e.New)
			}
		}
		for _, r := range plan.Refused {
			fmt.Fprintf(&b, "refused: %s: %s\n", r.File, r.Reason)
		}
	}
	_, err := fmt.Fprint(cmd.OutOrStdout(), b.String())
	return err
}

func editedSourceFiles(plan *sources.Plan) int {
	files := map[string]bool{}
	for _, e := range plan.Edits {
		files[e.File] = true
	}
	return len(files)
}
