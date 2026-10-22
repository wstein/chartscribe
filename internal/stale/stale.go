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

// Warning is one stale source; it never changes the exit status.
type Warning struct {
	Rule    string
	Note    string // file name
	Message string
}

// Result is the outcome of Check. Skipped is a message when the check could not run.
type Result struct {
	Warnings []Warning
	Skipped  string
}

// Check compares, for each file and measurement source of notes, the time of
// the last commit of the cited file with that of the note file (root/notes/<name>).
// A source that does not resolve (validate reports it) or a file without a commit
// is left out.
func Check(root, srcRoot string, notes []validate.NoteSources, run Runner) Result {
	skip := func(why string) Result { return Result{Skipped: "source-stale skipped: " + why} }
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
	for _, n := range notes {
		noteTime := commitTime(filepath.Join(realRoot, "notes", n.Note))
		if noteTime == 0 {
			continue
		}
		for _, s := range n.Sources {
			if !s.Verifiable() {
				continue
			}
			abs, p := sourcecheck.Resolve(srcRoot, s.Path)
			if p != nil {
				continue
			}
			if t := commitTime(abs); t > noteTime {
				res.Warnings = append(res.Warnings, Warning{Rule: Rule, Note: n.Note, Message: fmt.Sprintf(
					"%s source %q has a commit (%s) newer than the last commit of this note (%s); check what it cites",
					kindWord(s), s.Path, day(t), day(noteTime))})
			}
		}
	}
	return res
}

func kindWord(s note.Source) string {
	if s.Kind == note.KindMeasurement {
		return "measurement"
	}
	return "Source"
}

func day(unix int64) string { return time.Unix(unix, 0).UTC().Format("2006-01-02") }
