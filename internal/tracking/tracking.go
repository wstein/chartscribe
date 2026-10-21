// SPDX-License-Identifier: EUPL-1.2

// Package tracking reads GitHub issues through the gh CLI and links them to
// notes. The gh process is started through a Runner so that tests never call it.
package tracking

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/wstein/chartscribe/internal/note"
)

// Limit is the number of issues one query asks for; getting that many back
// means issues may be missing.
const Limit = 200

// ErrGh marks every failure to obtain the issues from gh (exit 3).
var ErrGh = errors.New("gh issue list failed")

// ErrMissing is what a Runner returns when the gh executable does not exist.
var ErrMissing = fmt.Errorf("%w: gh not found (install the GitHub CLI and run gh auth login)", ErrGh)

// ErrLimit means the query returned Limit issues, so some may be missing.
var ErrLimit = fmt.Errorf("%w: the limit of %d issues is reached, some issues are not listed", ErrGh, Limit)

// Runner starts an external command and returns its standard output.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// ExecRunner runs real processes.
type ExecRunner struct{}

// Run starts name with args. A missing executable gives ErrMissing; a failing
// one an error with its exit status and standard error.
func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if errors.Is(err, exec.ErrNotFound) {
		return nil, ErrMissing
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v: %s", ErrGh, err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// Issue is one issue of `gh issue list --json number,title,state,url,body`.
type Issue struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"`
	URL    string `json:"url"`
	Body   string `json:"body"`
}

// Parse reads the JSON of gh issue list.
func Parse(data []byte) ([]Issue, error) {
	var issues []Issue
	if err := json.Unmarshal(data, &issues); err != nil {
		return nil, fmt.Errorf("%w: unreadable output: %v", ErrGh, err)
	}
	return issues, nil
}

// Fetch lists the issues of the repository of the working directory in every
// state. It fails with ErrLimit when Limit issues come back.
func Fetch(ctx context.Context, r Runner) ([]Issue, error) {
	out, err := r.Run(ctx, "gh", "issue", "list", "--state", "all", "--limit", fmt.Sprint(Limit), "--json", "number,title,state,url,body")
	if err != nil {
		if errors.Is(err, ErrGh) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %v: %s", ErrGh, err, strings.TrimSpace(string(out)))
	}
	issues, err := Parse(out)
	if err != nil {
		return nil, err
	}
	if len(issues) >= Limit {
		return nil, ErrLimit
	}
	return issues, nil
}

// link is a line `Chartroom: <id> <title>` of an issue title or body.
var link = regexp.MustCompile(`(?m)^Chartroom:[ \t]+([0-9]{14}(?:-[0-9]{3})?)(?:[ \t].*)?$`)

// ByNote groups the issues by the note ids they link to, each group sorted by
// issue number. An issue links to a note when a line of its title or body
// starts with `Chartroom: <id>`; the title after the id is not compared.
func ByNote(issues []Issue) map[string][]Issue {
	by := map[string][]Issue{}
	for _, is := range issues {
		seen := map[string]bool{}
		for _, text := range []string{is.Title, note.StripCode(strings.ReplaceAll(is.Body, "\r", ""))} {
			for _, m := range link.FindAllStringSubmatch(text, -1) {
				if !seen[m[1]] {
					seen[m[1]] = true
					by[m[1]] = append(by[m[1]], is)
				}
			}
		}
	}
	for _, list := range by {
		sort.Slice(list, func(i, j int) bool { return list[i].Number < list[j].Number })
	}
	return by
}

// Line is the Tracking line of an issue: `- #<n> <title> (<state>) <url>`.
func Line(is Issue) string {
	return fmt.Sprintf("- #%d %s (%s) %s", is.Number, oneLine(is.Title), strings.ToLower(oneLine(is.State)), oneLine(is.URL))
}

// oneLine puts s on one line: control and format characters (ESC, NUL, bidi
// overrides) and any whitespace become single spaces.
func oneLine(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return ' '
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}
