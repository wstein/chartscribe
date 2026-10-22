// SPDX-License-Identifier: EUPL-1.2

// Package stale reports cited source files that have a commit newer than the
// note that cites them. It starts git as an external process with fixed
// arguments and only reads (GIT_OPTIONAL_LOCKS=0). It is informational: a
// missing git or a path outside a repository skips the check and never fails.
package stale

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/wstein/chartscribe/internal/note"
	"github.com/wstein/chartscribe/internal/pin"
	"github.com/wstein/chartscribe/internal/sourcecheck"
	"github.com/wstein/chartscribe/internal/validate"
)

// Rule is the name of the warning.
const Rule = "source-stale"

// Runner runs `git <args>` in dir and returns its standard output.
type Runner func(dir string, args ...string) (string, error)

// ExecRunner is the real Runner.
func ExecRunner(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "LC_ALL=C")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil && !errors.Is(err, exec.ErrNotFound) {
		err = fmt.Errorf("%v: %s", err, strings.TrimSpace(stderr.String()))
	}
	return string(out), err
}

// pinGit runs the git commands of pinned sources (package pin); tests may replace it.
var pinGit pin.Git = pin.ExecGit{}

// Warning is one stale source; it never changes the exit status.
type Warning struct {
	Rule    string
	Note    string // file name
	Message string
}

// Result is the outcome of Check. Skipped holds one message per part of the
// check that could not run: all of it (no git, no repository, a shallow clone) or
// one note or source (uncommitted note, no history). Each starts with "source-stale skipped: ".
type Result struct {
	Warnings []Warning
	Skipped  []string
}

// Check compares, for each file and measurement source of notes, the time of
// the last commit of the cited file with that of the note file (root/notes/<name>).
// A source that does not resolve (validate reports it) or a file without a commit
// is left out.
func Check(root, srcRoot string, notes []validate.NoteSources, run Runner) Result {
	const prefix = "source-stale skipped: "
	skip := func(why string) Result { return Result{Skipped: []string{prefix + why}} }
	out, err := run(srcRoot, "rev-parse", "--show-toplevel")
	if errors.Is(err, exec.ErrNotFound) {
		return skip("git not found")
	}
	if err != nil || strings.TrimSpace(out) == "" {
		return skip("the source root is not inside a git repository")
	}
	top, err := filepath.EvalSymlinks(strings.TrimSpace(out))
	if err != nil {
		return skip("the git toplevel cannot be read")
	}
	if shallow, err := run(top, "rev-parse", "--is-shallow-repository"); err == nil && strings.TrimSpace(shallow) == "true" {
		return skip("shallow clone (history incomplete)")
	}
	cache := map[string]int64{}
	commitTime := func(abs string) int64 {
		if t, ok := cache[abs]; ok {
			return t
		}
		cache[abs] = 0
		rel, err := filepath.Rel(top, abs)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return 0
		}
		got, err := run(top, "log", "-1", "--format=%ct", "--", ":(literal)"+filepath.ToSlash(rel))
		if err != nil {
			return 0
		}
		if t, err := strconv.ParseInt(strings.TrimSpace(got), 10, 64); err == nil {
			cache[abs] = t
		}
		return cache[abs]
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return skip("the collection root cannot be read")
	}
	var res Result
	skipped := map[string]bool{}
	skipMsg := func(msg string) {
		if !skipped[msg] {
			skipped[msg] = true
			res.Skipped = append(res.Skipped, prefix+msg)
		}
	}
	dirty := dirtyNotes(run, top, realRoot)
	cls := pinnedClassifier(srcRoot, notes)
	for _, n := range notes {
		// A pinned source is judged by its content (package pin) whatever the state of the note; only an
		// unpinned one needs the commit times of the note and of the file.
		skipUnpinned := false
		var noteTime int64
		if why, ok := dirty[n.Note]; ok {
			skipMsg(n.Note + ": " + why)
			skipUnpinned = true
		} else if noteTime = commitTime(filepath.Join(realRoot, "notes", n.Note)); noteTime == 0 {
			skipMsg(n.Note + ": note has no history in this clone")
			skipUnpinned = true
		}
		for _, s := range n.Sources {
			if !s.Verifiable() {
				continue
			}
			abs, p := sourcecheck.Resolve(srcRoot, s.Path)
			if p != nil {
				continue
			}
			if s.Kind == note.KindFile && s.Pin != "" {
				if cls != nil {
					pinnedWarning(&res, skipMsg, cls, n.Note, s)
				}
				continue
			}
			if skipUnpinned {
				continue
			}
			t := commitTime(abs)
			if t == 0 {
				skipMsg(fmt.Sprintf("%s: cited file %q has no history in this clone", n.Note, s.Path))
				continue
			}
			if t > noteTime {
				res.Warnings = append(res.Warnings, Warning{Rule: Rule, Note: n.Note, Message: fmt.Sprintf(
					"%s %q has a commit (%s) newer than the last commit of this note (%s); check what it cites",
					kindWord(s), s.Path, day(t), day(noteTime))})
			}
		}
	}
	return res
}

// pinnedClassifier prepares the content-based check of the pinned file sources, or returns nil when there are none.
func pinnedClassifier(srcRoot string, notes []validate.NoteSources) *pin.Classifier {
	var pinned []note.Source
	for _, n := range notes {
		for _, s := range n.Sources {
			if s.Kind == note.KindFile && s.Pin != "" && s.Valid() {
				pinned = append(pinned, s)
			}
		}
	}
	if len(pinned) == 0 {
		return nil
	}
	c, err := pin.New(pinGit, srcRoot, pinned)
	if err != nil {
		return nil
	}
	return c
}

// pinnedWarning warns about a pinned source whose range moved or whose text changed since the pin; a source
// that cannot be judged is skipped with the reason. A file that is gone is left to source-file-missing.
func pinnedWarning(res *Result, skipMsg func(string), c *pin.Classifier, name string, s note.Source) {
	r := c.Classify(s)
	warn := func(format string, a ...any) {
		res.Warnings = append(res.Warnings, Warning{Rule: Rule, Note: name, Message: fmt.Sprintf(format, a...)})
	}
	switch r.Status {
	case pin.Moved:
		warn("Source %q lines %d-%d (pinned @%s) now holds the same text at lines %d-%d; run cscribe sources update",
			s.Path, s.From, s.To, s.Pin, r.From, r.To)
	case pin.Changed:
		warn("Source %q lines %d-%d (pinned @%s): %s; check what the note says", s.Path, s.From, s.To, s.Pin, r.Reason)
	case pin.Renamed:
		warn("Source %q (pinned @%s) was renamed to %q; update the path", s.Path, s.Pin, r.NewPath)
	case pin.Unknown:
		skipMsg(fmt.Sprintf("%s: pinned source %q: %s", name, s.Path, r.Reason))
	}
}

func kindWord(s note.Source) string {
	if s.Kind == note.KindMeasurement {
		return "measurement"
	}
	return "Source"
}

func day(unix int64) string { return time.Unix(unix, 0).UTC().Format("2006-01-02") }

// dirtyNotes maps the file name of each note under realRoot/notes that has
// uncommitted changes or is not tracked to the reason it is skipped. One
// `git status` call covers all notes; if it fails nothing is reported dirty.
func dirtyNotes(run Runner, top, realRoot string) map[string]string {
	out := map[string]string{}
	rel, err := filepath.Rel(top, filepath.Join(realRoot, "notes"))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return out
	}
	got, err := run(top, "status", "--porcelain", "-z", "--no-renames", "--", ":(literal)"+filepath.ToSlash(rel))
	if err != nil {
		return out
	}
	for _, rec := range strings.Split(got, "\x00") {
		if len(rec) < 4 {
			continue
		}
		name := filepath.Base(filepath.FromSlash(rec[3:]))
		if rec[:2] == "??" {
			out[name] = "note is not committed"
		} else {
			out[name] = "note has uncommitted changes"
		}
	}
	return out
}
