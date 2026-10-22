// SPDX-License-Identifier: EUPL-1.2

// Package sources reads the file Source lines of a collection, classifies the pinned ones against the
// cited files with read-only git (package pin) and plans or applies the two edits of `cscribe sources`:
// adding a pin and moving a range with its pin. An edit changes only the Source line it names; every
// other byte of the note stays, and the result is verified by validating it again before anything is
// written. See specs/001-note-tooling/contracts/sources.md.
package sources

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wstein/chartscribe/internal/collection"
	"github.com/wstein/chartscribe/internal/fix"
	"github.com/wstein/chartscribe/internal/note"
	"github.com/wstein/chartscribe/internal/pin"
	"github.com/wstein/chartscribe/internal/validate"
)

// Rule names of the edits in a plan.
const (
	RulePin    = "source-pin"
	RuleUpdate = "source-update"
)

// Actions of a Row.
const (
	ActionNone   = "none"   // nothing to do
	ActionPin    = "pin"    // the line gets a pin
	ActionUpdate = "update" // the line gets a new range and the pin moves to HEAD
	ActionReview = "review" // a human must look: update does not touch the line
	ActionSkip   = "skip"   // not pinned, with the reason
)

// Entry is one valid file Source line of a note.
type Entry struct {
	Note   string // file name, for example "Idea.md"
	Line   int    // 1-based line in the file
	Raw    string // the whole line, "Source: ..."
	Source note.Source
}

// Row is an Entry with its classification and the action planned for it.
type Row struct {
	Entry
	Result  pin.Result
	Action  string
	NewLine string // the line after the edit, "" when there is none
	Reason  string // Review and Skip: why
}

// Report is the classification of the selected notes.
type Report struct {
	Notes   int // notes in the collection
	Rows    []Row
	Skipped []string // one line per part of the check that could not run
}

// Plan is a Report with the edits that would be written.
type Plan struct {
	Report
	Edits   []fix.Edit
	Refused []fix.Refusal
	Rule    string

	orig, final map[string]string
	before      []validate.Finding
	srcRoot     string
}

// Collect returns the valid file Source lines of the notes named in only (all notes when empty), in
// note and line order. Lines inside code fences or frontmatter are not Source lines.
func Collect(root string, only []string) (entries []Entry, notes int, err error) {
	files, err := collection.ReadNotes(root)
	if err != nil {
		return nil, 0, err
	}
	want := map[string]bool{}
	for _, n := range only {
		want[n] = true
	}
	for _, f := range files {
		if len(want) > 0 && !want[f.Name] {
			continue
		}
		entries = append(entries, noteEntries(f)...)
	}
	return entries, len(files), nil
}

func noteEntries(f collection.File) []Entry {
	_, body, err := note.Parse(f.Text)
	if err != nil {
		return nil
	}
	first := strings.Count(f.Text[:len(f.Text)-len(body)], "\n") // index of the first body line
	fenced, _ := note.CodeLines(body)
	var out []Entry
	for i, line := range strings.Split(body, "\n") {
		if i < len(fenced) && fenced[i] || !strings.HasPrefix(line, "Source: ") {
			continue
		}
		s, ok := note.ParseSourceLine(strings.TrimPrefix(line, "Source: "))
		if !ok || s.Kind != note.KindFile || !s.Valid() {
			continue
		}
		out = append(out, Entry{Note: f.Name, Line: first + i + 1, Raw: line, Source: s})
	}
	return out
}

func sourcesOf(entries []Entry) []note.Source {
	out := make([]note.Source, len(entries))
	for i, e := range entries {
		out[i] = e.Source
	}
	return out
}

// Check classifies every file Source line of the selected notes. It writes nothing.
func Check(root, srcRoot string, g pin.Git, only []string) (*Report, error) {
	entries, notes, err := Collect(root, only)
	if err != nil {
		return nil, err
	}
	c, err := pin.New(g, srcRoot, sourcesOf(entries))
	if err != nil {
		return nil, err
	}
	rep := &Report{Notes: notes}
	if why := c.Disabled(); why != "" {
		rep.Skipped = append(rep.Skipped, "sources skipped: "+why)
	}
	for _, e := range entries {
		rep.Rows = append(rep.Rows, classifyRow(c, e))
	}
	return rep, nil
}

func classifyRow(c *pin.Classifier, e Entry) Row {
	r := Row{Entry: e, Action: ActionNone}
	r.Result = c.Classify(e.Source)
	switch r.Result.Status {
	case pin.Changed, pin.Renamed, pin.Missing:
		r.Action, r.Reason = ActionReview, r.Result.Reason
	case pin.Unknown:
		r.Reason = r.Result.Reason
	}
	return r
}

// PlanPin plans a pin for every unpinned file Source line whose range holds at rev (HEAD when "").
func PlanPin(root, srcRoot string, g pin.Git, only []string, rev string) (*Plan, error) {
	return build(root, srcRoot, g, only, RulePin, func(c *pin.Classifier, e Entry) Row {
		r := Row{Entry: e, Action: ActionNone, Result: pin.Result{Status: pin.Unpinned}}
		if e.Source.Pin != "" {
			return classifyRow(c, e)
		}
		short, why := c.Target(e.Source, rev)
		if why != "" {
			r.Action, r.Reason = ActionSkip, why
			return r
		}
		r.Action = ActionPin
		r.NewLine = note.FormatFile(e.Source.Path, e.Source.From, e.Source.To, short)
		return r
	})
}

// PlanUpdate plans, for every moved pinned line, the new range and the pin at HEAD. Changed, renamed and
// missing lines are listed for review and never rewritten.
func PlanUpdate(root, srcRoot string, g pin.Git, only []string) (*Plan, error) {
	return build(root, srcRoot, g, only, RuleUpdate, func(c *pin.Classifier, e Entry) Row {
		r := classifyRow(c, e)
		if r.Result.Status == pin.Moved {
			r.Action = ActionUpdate
			r.NewLine = note.FormatFile(e.Source.Path, r.Result.From, r.Result.To, c.Head())
		}
		return r
	})
}

func build(root, srcRoot string, g pin.Git, only []string, rule string, decide func(*pin.Classifier, Entry) Row) (*Plan, error) {
	entries, notes, err := Collect(root, only)
	if err != nil {
		return nil, err
	}
	c, err := pin.New(g, srcRoot, sourcesOf(entries))
	if err != nil {
		return nil, err
	}
	files, err := collection.ReadNotes(root)
	if err != nil {
		return nil, err
	}
	before, err := validate.RunWith(root, nil, validate.Options{CheckSources: true, SourceRoot: srcRoot})
	if err != nil {
		return nil, err
	}
	p := &Plan{Report: Report{Notes: notes}, Rule: rule, orig: map[string]string{}, final: map[string]string{},
		before: before.Findings, srcRoot: srcRoot}
	if why := c.Disabled(); why != "" {
		p.Skipped = append(p.Skipped, "sources skipped: "+why)
	}
	texts := map[string]string{}
	for _, f := range files {
		texts[f.Name] = f.Text
	}
	byNote := map[string][]int{} // note -> indexes into Rows with an edit
	for _, e := range entries {
		row := decide(c, e)
		if row.NewLine != "" {
			byNote[e.Note] = append(byNote[e.Note], len(p.Rows))
		}
		p.Rows = append(p.Rows, row)
	}
	names := make([]string, 0, len(byNote))
	for n := range byNote {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		rel := "notes/" + name
		if why := fix.UnsafeFile(filepath.Join(root, "notes", name)); why != "" {
			p.refuse(rel, why, byNote[name])
			continue
		}
		p.orig[rel] = texts[name]
		p.final[rel] = rewrite(texts[name], p.rows(byNote[name]))
	}
	if err := p.verifyAll(root, byNote); err != nil {
		return nil, err
	}
	p.finish(byNote)
	return p, nil
}

func (p *Plan) rows(idx []int) []Row {
	out := make([]Row, len(idx))
	for i, k := range idx {
		out[i] = p.Rows[k]
	}
	return out
}

// refuse drops the planned edits of the rows idx, with the reason.
func (p *Plan) refuse(file, why string, idx []int) {
	p.Refused = append(p.Refused, fix.Refusal{File: file, Reason: why})
	for _, k := range idx {
		p.Rows[k].Action, p.Rows[k].NewLine = ActionSkip, ""
		if p.Rows[k].Reason == "" {
			p.Rows[k].Reason = why
		}
	}
}

// rewrite replaces the Source lines of rows in text; nothing else changes.
func rewrite(text string, rows []Row) string {
	lines := strings.Split(text, "\n")
	for _, r := range rows {
		lines[r.Line-1] = r.NewLine
	}
	return strings.Join(lines, "\n")
}

// verifyAll checks the result of all edits at once and, if that fails, file by file; a file whose edit does
// not verify is refused and left out.
func (p *Plan) verifyAll(root string, byNote map[string][]int) error {
	if len(p.final) == 0 {
		return nil
	}
	if err := p.verify(root, p.final, byNote); err == nil {
		return nil
	}
	files := make([]string, 0, len(p.final))
	for f := range p.final {
		files = append(files, f)
	}
	sort.Strings(files)
	for _, f := range files {
		one := map[string]string{f: p.final[f]}
		if err := p.verify(root, one, byNote); err != nil {
			p.refuse(f, "the edit does not verify: "+err.Error(), byNote[strings.TrimPrefix(f, "notes/")])
			delete(p.final, f)
			delete(p.orig, f)
		}
	}
	if len(p.final) > 0 {
		return p.verify(root, p.final, byNote)
	}
	return nil
}

// verify validates the result as an overlay: no finding that the collection does not have now, every edited
// line is the planned one, and every other line of an edited note is as it was.
func (p *Plan) verify(root string, final map[string]string, byNote map[string][]int) error {
	for file, text := range final {
		name := strings.TrimPrefix(file, "notes/")
		old, now := strings.Split(p.orig[file], "\n"), strings.Split(text, "\n")
		if len(old) != len(now) {
			return fmt.Errorf("%s: the line count changed", file)
		}
		edited := map[int]string{}
		for _, k := range byNote[name] {
			edited[p.Rows[k].Line-1] = p.Rows[k].NewLine
		}
		for i := range old {
			if want, ok := edited[i]; ok {
				if now[i] != want {
					return fmt.Errorf("%s:%d is not the planned line", file, i+1)
				}
				if s, ok := note.ParseSourceLine(strings.TrimPrefix(now[i], "Source: ")); !ok || !s.Valid() || s.Kind != note.KindFile {
					return fmt.Errorf("%s:%d does not parse as a file source", file, i+1)
				}
			} else if old[i] != now[i] {
				return fmt.Errorf("%s:%d changed outside the Source lines", file, i+1)
			}
		}
	}
	return verifyOverlay(root, p.srcRoot, p.before, final)
}

func verifyOverlay(root, srcRoot string, before []validate.Finding, overlay validate.Overlay) error {
	res, err := validate.RunWith(root, overlay, validate.Options{CheckSources: true, SourceRoot: srcRoot})
	if err != nil {
		return err
	}
	known := map[validate.Finding]bool{}
	for _, f := range before {
		known[f] = true
	}
	for _, f := range res.Findings {
		if !known[f] {
			return fmt.Errorf("a new finding: %s: %s", f.Rule, f.Message)
		}
	}
	return nil
}

// finish fills Edits from the rows that kept their edit.
func (p *Plan) finish(byNote map[string][]int) {
	for _, r := range p.Rows {
		if r.NewLine == "" {
			continue
		}
		p.Edits = append(p.Edits, fix.Edit{File: "notes/" + r.Note, Line: r.Line, Rule: p.Rule, Old: r.Raw, New: r.NewLine})
	}
	sort.SliceStable(p.Edits, func(i, j int) bool {
		a, b := p.Edits[i], p.Edits[j]
		if a.File != b.File {
			return a.File < b.File
		}
		return a.Line < b.Line
	})
	sort.Slice(p.Refused, func(i, j int) bool { return p.Refused[i].File < p.Refused[j].File })
}

// Apply writes the planned notes, all or none as far as the file system allows, with the guarantees of
// fix.ApplyFiles: the result is verified once more, the targets must be unchanged regular files inside the
// collection, and a failed write restores the files already written.
func (p *Plan) Apply(root string) error {
	if len(p.final) == 0 {
		return nil
	}
	byNote := map[string][]int{}
	for k, r := range p.Rows {
		if r.NewLine != "" {
			byNote[r.Note] = append(byNote[r.Note], k)
		}
	}
	return fix.ApplyFiles(root, p.orig, p.final, func(overlay validate.Overlay) error {
		if err := p.verify(root, overlay, byNote); err != nil {
			return errors.New("the result no longer verifies: " + err.Error())
		}
		return nil
	})
}
