// SPDX-License-Identifier: EUPL-1.2

package pin

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/wstein/chartscribe/internal/note"
	"github.com/wstein/chartscribe/internal/sourcecheck"
)

// Errors of New. A shallow clone is not an error: the Classifier answers Unknown for every pinned source.
var (
	ErrNoRepo = errors.New("the source root is not inside a git repository with a commit")
)

// ShortLen is the number of hex digits of a pin the tool writes.
const ShortLen = 12

var hexRev = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

// ValidRev reports whether s may be passed to git as a commit id.
func ValidRev(s string) bool { return hexRev.MatchString(s) }

// Result is the classification of one source.
type Result struct {
	Status   Status
	From, To int      // Current and Moved: the range in the file now
	NewPath  string   // Renamed: the path now, relative to the source root when it lies below it
	Reason   string   // Unknown, Missing, Renamed and Changed: why
	Hunks    []string // Changed: headers of the hunks that touch the range
}

// change is one entry of the name-status diff between a pin and HEAD.
type change struct {
	status  byte // 'M', 'T', 'D', 'A', 'R'
	newPath string
}

// Classifier classifies pinned file sources against the file at HEAD. It runs one `git diff
// --name-status` per distinct pin and one `git diff -U0` plus one `git cat-file` per changed
// (pin, path) pair, each cached.
type Classifier struct {
	git      Git
	top      string // real git toplevel
	srcRoot  string // real source root
	head     string // full commit id of HEAD
	disabled string // non-empty: why no pinned source can be classified (shallow clone)
	tracked  map[string]bool
	dirty    map[string]bool
	pins     map[string]string            // pin as written -> full id, "" when not found
	changes  map[string]map[string]change // full pin id -> top-relative path -> change
	results  map[string]Result
}

// New prepares the classification of sources (every kind may be passed; only pinned file sources count).
// srcRoot is the source root of the collection.
func New(g Git, srcRoot string, sources []note.Source) (*Classifier, error) {
	realRoot, err := filepath.EvalSymlinks(srcRoot)
	if err != nil {
		return nil, fmt.Errorf("the source root cannot be read: %w", err)
	}
	out, err := g.Output(realRoot, "rev-parse", "--show-toplevel")
	if errors.Is(err, ErrNoGit) {
		return nil, ErrNoGit
	}
	if err != nil || strings.TrimSpace(out) == "" {
		return nil, ErrNoRepo
	}
	top, err := filepath.EvalSymlinks(strings.TrimSpace(out))
	if err != nil {
		return nil, ErrNoRepo
	}
	head, err := g.Output(top, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil || !ValidRev(strings.TrimSpace(head)) { // SHA-1 repositories only
		return nil, ErrNoRepo
	}
	c := &Classifier{git: g, top: top, srcRoot: realRoot, head: strings.TrimSpace(head),
		tracked: map[string]bool{}, dirty: map[string]bool{}, pins: map[string]string{},
		changes: map[string]map[string]change{}, results: map[string]Result{}}
	if shallow, err := g.Output(top, "rev-parse", "--is-shallow-repository"); err == nil && strings.TrimSpace(shallow) == "true" {
		c.disabled = "shallow clone (history incomplete)"
		return c, nil
	}
	wanted := map[string]map[string]bool{} // full pin -> top-relative paths
	all := map[string]bool{}
	for _, s := range sources {
		if s.Kind != note.KindFile || s.Pin == "" || !s.Valid() {
			continue
		}
		rel, ok := c.rel(s.Path)
		if !ok {
			continue
		}
		all[rel] = true
		full := c.resolve(s.Pin)
		if full == "" {
			continue
		}
		if wanted[full] == nil {
			wanted[full] = map[string]bool{}
		}
		wanted[full][rel] = true
	}
	if len(all) == 0 {
		return c, nil
	}
	c.loadIndex(all)
	for full, paths := range wanted {
		c.loadChanges(full, paths)
	}
	return c, nil
}

// Head returns the pin the tool writes for HEAD.
func (c *Classifier) Head() string { return c.head[:ShortLen] }

// Disabled returns why pinned sources cannot be classified at all, or "".
func (c *Classifier) Disabled() string { return c.disabled }

// rel returns the path of a source relative to the git toplevel (a slash path), and false when it
// leaves the source root.
func (c *Classifier) rel(p string) (string, bool) {
	if filepath.IsAbs(p) || strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\`) || filepath.VolumeName(p) != "" {
		return "", false
	}
	clean := filepath.Clean(filepath.FromSlash(p))
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == "." {
		return "", false
	}
	r, err := filepath.Rel(c.top, filepath.Join(c.srcRoot, clean))
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(r), true
}

// resolve returns the full id of the commit a pin names, or "" (cached).
func (c *Classifier) resolve(pin string) string {
	if full, ok := c.pins[pin]; ok {
		return full
	}
	c.pins[pin] = ""
	if !ValidRev(pin) {
		return ""
	}
	out, err := c.git.Output(c.top, "rev-parse", "--verify", "--quiet", pin+"^{commit}")
	if err != nil {
		return ""
	}
	c.pins[pin] = strings.TrimSpace(out)
	return c.pins[pin]
}

// loadIndex records which of the wanted paths git tracks and which of them have uncommitted changes.
func (c *Classifier) loadIndex(wanted map[string]bool) {
	_ = c.git.Stream(c.top, []string{"ls-files", "-z", "--cached"}, 0, func(p string) {
		if wanted[p] {
			c.tracked[p] = true
		}
	})
	_ = c.git.Stream(c.top, []string{"status", "--porcelain=v1", "-z", "--no-renames", "--untracked-files=no"}, 0, func(rec string) {
		// Records are "XY path"; with --no-renames there are no second paths.
		if len(rec) > 3 && wanted[rec[3:]] {
			c.dirty[rec[3:]] = true
		}
	})
}

// loadChanges records the name-status entries of the wanted paths between pin and HEAD.
func (c *Classifier) loadChanges(full string, wanted map[string]bool) {
	m := map[string]change{}
	c.changes[full] = m
	var status string
	var first string
	step := 0 // 0: expect a status, 1: first path, 2: second path of a rename or copy
	_ = c.git.Stream(c.top, []string{"diff", "--name-status", "-z", "-M", "--no-color", "--no-ext-diff", "--no-textconv", full, c.head}, 0, func(rec string) {
		switch step {
		case 0:
			status = rec
			step = 1
		case 1:
			if status != "" && (status[0] == 'R' || status[0] == 'C') {
				first, step = rec, 2
				return
			}
			step = 0
			if wanted[rec] && status != "" {
				m[rec] = change{status: status[0]}
			}
		case 2:
			step = 0
			if wanted[first] && status[0] == 'R' {
				m[first] = change{status: 'R', newPath: rec}
			}
		}
	})
}

func lineSplit(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// Classify classifies one source. A source that is not a valid file source with a pin is Unpinned.
func (c *Classifier) Classify(s note.Source) Result {
	if s.Kind != note.KindFile || !s.Valid() || s.Pin == "" {
		return Result{Status: Unpinned}
	}
	key := fmt.Sprintf("%s\x00%s\x00%d\x00%d", s.Pin, s.Path, s.From, s.To)
	if r, ok := c.results[key]; ok {
		return r
	}
	r := c.classify(s)
	c.results[key] = r
	return r
}

func unknown(format string, a ...any) Result {
	return Result{Status: Unknown, Reason: fmt.Sprintf(format, a...)}
}

func (c *Classifier) classify(s note.Source) Result {
	if c.disabled != "" {
		return unknown("%s", c.disabled)
	}
	rel, ok := c.rel(s.Path)
	if !ok {
		return unknown("the path leaves the source root")
	}
	full := c.resolve(s.Pin)
	if full == "" {
		return unknown("pin @%s is not a commit of this clone", s.Pin)
	}
	ch, changed := c.changes[full][rel]
	abs, prob := sourcecheck.Resolve(c.srcRoot, s.Path)
	if prob != nil {
		if prob.Rule != "source-file-missing" {
			return unknown("%s", prob.Message)
		}
		switch {
		case changed && ch.status == 'R':
			return c.renamed(s, ch.newPath)
		case changed && ch.status == 'D':
			return Result{Status: Missing, Reason: "the file was deleted since @" + s.Pin}
		}
		return Result{Status: Missing, Reason: "the file is not found below the source root"}
	}
	if r, err := filepath.Rel(c.top, abs); err != nil || filepath.ToSlash(r) != rel {
		return unknown("the path goes through a symbolic link")
	}
	if !c.tracked[rel] {
		return unknown("the file is not tracked by git")
	}
	if c.dirty[rel] {
		return unknown("the file has uncommitted changes")
	}
	if changed {
		switch ch.status {
		case 'R':
			return c.renamed(s, ch.newPath)
		case 'D':
			return Result{Status: Missing, Reason: "the file was deleted since @" + s.Pin}
		case 'A':
			return unknown("the file does not exist at the pin")
		}
	} else {
		return Result{Status: Current, From: s.From, To: s.To}
	}
	// M or T: map the range through the hunks, then check the text itself.
	if ch.status == 'T' {
		return Result{Status: Changed, Reason: "the file type changed since @" + s.Pin}
	}
	hunks, binary, err := c.hunks(full, rel)
	if err != nil {
		return unknown("git diff failed: %v", err)
	}
	if binary {
		return Result{Status: Changed, Reason: "binary file changed since @" + s.Pin}
	}
	pinned, err := c.git.Output(c.top, "cat-file", "blob", full+":"+rel)
	if err != nil {
		return unknown("the file does not exist at the pin")
	}
	pinLines := lineSplit(pinned)
	if s.To > len(pinLines) {
		return unknown("lines %d-%d lie outside the file at the pin (%d lines)", s.From, s.To, len(pinLines))
	}
	m := MapRange(hunks, s.From, s.To)
	if m.Status == Changed {
		r := Result{Status: Changed, Reason: "the cited lines were edited since @" + s.Pin}
		for _, h := range m.Overlap {
			r.Hunks = append(r.Hunks, h.Header())
		}
		return r
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return unknown("the file cannot be read: %v", err)
	}
	cur := lineSplit(string(data))
	if m.From < 1 || m.To > len(cur) || !equalLines(pinLines[s.From-1:s.To], cur[m.From-1:m.To]) {
		return Result{Status: Changed, Reason: "the text at the mapped lines differs from the pinned text"}
	}
	return Result{Status: m.Status, From: m.From, To: m.To}
}

func equalLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (c *Classifier) renamed(s note.Source, newTop string) Result {
	np := newTop
	if r, err := filepath.Rel(c.srcRoot, filepath.Join(c.top, filepath.FromSlash(newTop))); err == nil && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		np = filepath.ToSlash(r)
	}
	return Result{Status: Renamed, NewPath: path.Clean(np), Reason: "the file was renamed since @" + s.Pin}
}

// hunks returns the hunk headers of `git diff -U0 full HEAD -- rel` and whether git calls the file binary.
func (c *Classifier) hunks(full, rel string) (hunks []Hunk, binary bool, err error) {
	args := []string{"diff", "-U0", "--no-color", "--no-ext-diff", "--no-textconv", "--no-renames", full, c.head, "--", ":(literal)" + rel}
	err = c.git.Stream(c.top, args, '\n', func(line string) {
		switch {
		case strings.HasPrefix(line, "@@ "):
			if h, ok := ParseHunkHeader(line); ok {
				hunks = append(hunks, h)
			}
		case strings.HasPrefix(line, "Binary files "), line == "GIT binary patch":
			binary = true
		}
	})
	return hunks, binary, err
}

// Target returns the pin to write for source s: the abbreviation of rev (HEAD when rev is ""), or why
// the source cannot be pinned. Pinning vouches that the cited range is right at that commit: for HEAD
// the range must be valid in the file, the file tracked and without uncommitted changes; for another
// commit the file must exist there with at least To lines.
func (c *Classifier) Target(s note.Source, rev string) (short, reason string) {
	if c.disabled != "" {
		return "", c.disabled
	}
	if s.Kind != note.KindFile || !s.Valid() {
		return "", "not a file source"
	}
	rel, ok := c.rel(s.Path)
	if !ok {
		return "", "the path leaves the source root"
	}
	if rev == "" {
		if p := sourcecheck.Check(c.srcRoot, s); p != nil {
			return "", p.Message
		}
		abs, _ := sourcecheck.Resolve(c.srcRoot, s.Path)
		if r, err := filepath.Rel(c.top, abs); err != nil || filepath.ToSlash(r) != rel {
			return "", "the path goes through a symbolic link"
		}
		if !c.trackedNow(rel) {
			return "", "the file is not tracked by git"
		}
		if c.dirtyNow(rel) {
			return "", "the file has uncommitted changes"
		}
		return c.head[:ShortLen], ""
	}
	if !ValidRev(rev) {
		return "", "the revision must be 7 to 40 lower-case hex digits"
	}
	full := c.resolve(rev)
	if full == "" {
		return "", fmt.Sprintf("revision %s is not a commit of this clone", rev)
	}
	blob, err := c.git.Output(c.top, "cat-file", "blob", full+":"+rel)
	if err != nil {
		return "", fmt.Sprintf("the file does not exist at %s", rev)
	}
	if n := len(lineSplit(blob)); s.To > n {
		return "", fmt.Sprintf("lines %d-%d lie outside the file at %s (%d lines)", s.From, s.To, rev, n)
	}
	return full[:ShortLen], ""
}

// trackedNow and dirtyNow answer for paths that New did not know (unpinned sources) with one git call each.
func (c *Classifier) trackedNow(rel string) bool {
	if c.tracked[rel] {
		return true
	}
	out, err := c.git.Output(c.top, "ls-files", "-z", "--cached", "--", ":(literal)"+rel)
	if err == nil && strings.TrimRight(out, "\x00") == rel {
		c.tracked[rel] = true
	}
	return c.tracked[rel]
}

func (c *Classifier) dirtyNow(rel string) bool {
	if c.dirty[rel] {
		return true
	}
	out, err := c.git.Output(c.top, "status", "--porcelain=v1", "-z", "--no-renames", "--untracked-files=no", "--", ":(literal)"+rel)
	return err != nil || out != ""
}
