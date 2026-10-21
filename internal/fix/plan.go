// SPDX-License-Identifier: EUPL-1.2

// Package fix plans and applies the safe, deterministic fixes of validate findings. A fix is planned
// only when one value is certain, is kept only when re-running validate on the result shows fewer
// findings and no new one, and changes only frontmatter fields and the map and README link lines the
// rule names; the note body bytes stay identical. Planning reads files and writes nothing.
package fix

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wstein/chartscribe/internal/collection"
	"github.com/wstein/chartscribe/internal/note"
	"github.com/wstein/chartscribe/internal/validate"
)

// Rules lists the rules validate --fix can fix, in contract order.
var Rules = []string{"id-invalid", "list-type", "tag-invalid", "tag-count", "name-duplicate", "map-unlisted", "readme-link"}

// Edit is one exact change: the lines Old (empty for an insertion) at Line of File become New.
type Edit struct {
	File string // slash path relative to the root
	Line int    // 1-based line in the file as it is now
	Rule string
	Old  string
	New  string
}

// Refusal names a file with a planned fix that was not kept, and why.
type Refusal struct {
	File   string
	Reason string
}

// Plan is the outcome of Build.
type Plan struct {
	Notes   int
	Edits   []Edit
	Refused []Refusal
	Before  []validate.Finding // findings now
	After   []validate.Finding // findings once every edit is applied

	orig  map[string]string // file -> text now
	final map[string]string // file -> text after the edits, for the files with edits
	kept  map[string][]lineEdit
}

type fileState struct {
	text     string
	problem  string // non-empty: the file must not be written
	isReadme bool
}

// Build plans the fixes of the collection at root. only limits the plan to the named rules (all when empty).
func Build(root string, only []string) (*Plan, error) {
	vocab, err := collection.LoadVocabulary(root)
	if err != nil {
		return nil, err
	}
	files, err := collection.ReadNotes(root)
	if err != nil {
		return nil, err
	}
	base, err := validate.Run(root)
	if err != nil {
		return nil, err
	}
	p := &Plan{Notes: base.Notes, Before: base.Findings, orig: map[string]string{}, final: map[string]string{}, kept: map[string][]lineEdit{}}
	states := map[string]*fileState{}
	for _, f := range files {
		rel := "notes/" + f.Name
		st := &fileState{text: f.Text}
		st.problem = unsafeFile(filepath.Join(root, "notes", f.Name))
		states[rel] = st
		p.orig[rel] = f.Text
	}
	readmePath := filepath.Join(root, "README.md")
	if info, err := os.Lstat(readmePath); err == nil && !info.IsDir() {
		raw, err := os.ReadFile(readmePath)
		if err != nil {
			return nil, err
		}
		st := &fileState{text: string(raw), isReadme: true, problem: unsafeFile(readmePath)}
		states["README.md"] = st
		p.orig["README.md"] = st.text
	}

	run := &runner{root: root, plan: p, states: states, overlay: validate.Overlay{}, cur: base.Findings, only: only, refused: map[string]bool{}}

	// Phase 1: frontmatter fixes of each note.
	usedIDs := map[string]bool{}
	for _, f := range files {
		if fm, _, err := note.Parse(f.Text); err == nil {
			if v, ok := fm.Get("id"); ok && !v.IsList && note.ValidID(v.Text) {
				usedIDs[v.Text] = true
			}
		}
	}
	rules := noteRules(base.Findings)
	for _, f := range files {
		rel := "notes/" + f.Name
		if edits := noteEdits(rel, f.Name, f.Text, vocab, usedIDs, rules[f.Name]); len(edits) > 0 {
			if err := run.try(rel, edits); err != nil {
				return nil, err
			}
		}
	}

	// Phase 2: map links and README links, decided on the result of phase 1.
	texts := map[string]string{}
	for _, f := range files {
		texts[f.Name] = run.text("notes/" + f.Name)
	}
	byFile := map[string][]lineEdit{}
	for _, e := range mapEdits(run.cur, texts, texts0(files)) {
		byFile[e.file] = append(byFile[e.file], e)
	}
	names := make([]string, 0, len(byFile))
	for file := range byFile {
		names = append(names, file)
	}
	sort.Strings(names)
	for _, file := range names {
		if err := run.try(file, byFile[file]); err != nil {
			return nil, err
		}
	}
	if st, ok := states["README.md"]; ok {
		if edits := readmeEdits(root, st.text); len(edits) > 0 {
			if err := run.try("README.md", edits); err != nil {
				return nil, err
			}
		}
	}

	p.After = run.cur
	p.finish(run)
	return p, nil
}

func texts0(files []collection.File) map[string]string {
	out := map[string]string{}
	for _, f := range files {
		out[f.Name] = f.Text
	}
	return out
}

// unsafeFile returns why a file must not be rewritten, or "".
func unsafeFile(path string) string {
	info, err := os.Lstat(path)
	if err != nil {
		return err.Error()
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "is a symlink"
	}
	if !info.Mode().IsRegular() {
		return "is not a regular file"
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err.Error()
	}
	if strings.Contains(string(raw), "\r") {
		return "has CR line endings"
	}
	return ""
}

type runner struct {
	root    string
	plan    *Plan
	states  map[string]*fileState
	overlay validate.Overlay
	cur     []validate.Finding
	only    []string
	refused map[string]bool
}

func (r *runner) text(file string) string {
	if t, ok := r.overlay[file]; ok {
		return t
	}
	return r.states[file].text
}

func (r *runner) refuse(file, reason string) {
	if r.refused[file] {
		return
	}
	r.refused[file] = true
	r.plan.Refused = append(r.plan.Refused, Refusal{File: file, Reason: reason})
}

// try keeps the edits of one file when the result validates better: first all together, else one by one.
func (r *runner) try(file string, edits []lineEdit) error {
	var wanted []lineEdit
	for _, e := range edits {
		if len(r.only) == 0 || contains(r.only, e.rule) {
			wanted = append(wanted, e)
		}
	}
	if len(wanted) == 0 {
		return nil
	}
	if problem := r.states[file].problem; problem != "" {
		r.refuse(file, problem)
		return nil
	}
	ok, err := r.accept(file, wanted)
	if err != nil || ok || len(wanted) == 1 {
		if err == nil && !ok {
			r.refuse(file, "the fix does not reduce the findings without new ones")
		}
		return err
	}
	any := false
	for _, e := range wanted {
		ok, err := r.accept(file, []lineEdit{e})
		if err != nil {
			return err
		}
		any = any || ok
	}
	if !any {
		r.refuse(file, "the fix does not reduce the findings without new ones")
	}
	return nil
}

func (r *runner) accept(file string, edits []lineEdit) (bool, error) {
	all := append(append([]lineEdit{}, r.plan.kept[file]...), edits...)
	text, ok := applyEdits(r.states[file].text, all)
	if !ok {
		return false, nil
	}
	if !bodyKept(all, r.states[file].text, text) {
		return false, nil
	}
	next := validate.Overlay{}
	for k, v := range r.overlay {
		next[k] = v
	}
	next[file] = text
	res, err := validate.RunOverlay(r.root, next)
	if err != nil {
		return false, err
	}
	if !better(r.cur, res.Findings) {
		return false, nil
	}
	r.overlay = next
	r.cur = res.Findings
	r.plan.kept[file] = all
	return true, nil
}

// bodyKept checks that a file whose edits are all frontmatter edits keeps its body bytes.
func bodyKept(edits []lineEdit, orig, text string) bool {
	for _, e := range edits {
		if e.rule == "map-unlisted" || e.rule == "readme-link" {
			return true
		}
	}
	return body(orig) == body(text)
}

// better reports whether after has fewer findings than before and none that before lacks.
func better(before, after []validate.Finding) bool {
	if len(after) >= len(before) {
		return false
	}
	known := map[validate.Finding]bool{}
	for _, f := range before {
		known[f] = true
	}
	for _, f := range after {
		if !known[f] {
			return false
		}
	}
	return true
}

func (p *Plan) finish(r *runner) {
	files := make([]string, 0, len(p.kept))
	for file := range p.kept {
		files = append(files, file)
	}
	sort.Strings(files)
	for _, file := range files {
		edits := p.kept[file]
		text, _ := applyEdits(r.states[file].text, edits)
		p.final[file] = text
		lines := strings.Split(r.states[file].text, "\n")
		for _, e := range edits {
			p.Edits = append(p.Edits, Edit{File: file, Line: e.start + 1, Rule: e.rule,
				Old: strings.Join(lines[e.start:e.end], "\n"), New: strings.Join(e.repl, "\n")})
		}
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

func (p *Plan) String() string { return fmt.Sprintf("%d edits", len(p.Edits)) }
