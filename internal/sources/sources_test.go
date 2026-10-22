// SPDX-License-Identifier: EUPL-1.2

package sources

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wstein/chartscribe/internal/gittest"
	"github.com/wstein/chartscribe/internal/pin"
	"github.com/wstein/chartscribe/internal/validate"
)

func numbered(n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	return b.String()
}

func noteText(id, extra, sourceLines string) string {
	return "---\nid: " + id + "\ntags: [\"type/idea\", \"status/open\", \"category/direction\"]\n---\nBody text.\n" + extra + "\n" + sourceLines
}

const mapNote = "---\nid: 20261021120100\ntags: [\"type/map\", \"category/direction\"]\n---\nMap.\n\n- [[Alpha]]\n- [[Beta]]\n"

// fixture is a collection that sits in a git repository next to the files it cites.
type fixture struct {
	*gittest.Repo
	first string // full id of the first commit
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	r := gittest.New(t)
	r.Write("docs/a.md", numbered(20))
	r.Write("docs/b.md", numbered(20))
	r.Write("notes/Map - Direction.md", mapNote)
	r.Write("notes/Alpha.md", noteText("20261021120000", "", "Source: docs/a.md, lines 5-7\nSource: docs/b.md, lines 2-3 @FIRST\nSource: decision: kept, 2026-10-22\n"))
	r.Write("notes/Beta.md", noteText("20261021120001", "```\nSource: docs/a.md, lines 1-1\n```\n", "Source: docs/a.md, lines 10-12 @FIRST\n"))
	first := r.Commit("first")
	// Pins can only name a commit that exists: write the real short id now.
	for _, n := range []string{"Alpha.md", "Beta.md"} {
		p := filepath.Join(r.Dir, "notes", n)
		data, _ := os.ReadFile(p)
		os.WriteFile(p, []byte(strings.ReplaceAll(string(data), "@FIRST", "@"+first[:12])), 0o644)
	}
	r.Commit("pins")
	return &fixture{Repo: r, first: first}
}

func (f *fixture) read(name string) string {
	b, err := os.ReadFile(filepath.Join(f.Dir, filepath.FromSlash(name)))
	if err != nil {
		f.T.Fatal(err)
	}
	return string(b)
}

func sum(s string) [32]byte { return sha256.Sum256([]byte(s)) }

func TestCollectFindsFileSourceLines(t *testing.T) {
	f := newFixture(t)
	entries, notes, err := Collect(f.Dir, nil)
	if err != nil || notes != 3 {
		t.Fatal(notes, err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, fmt.Sprintf("%s:%d %s %d-%d pin=%q", e.Note, e.Line, e.Source.Path, e.Source.From, e.Source.To, e.Source.Pin))
	}
	want := []string{
		"Alpha.md:7 docs/a.md 5-7 pin=\"\"",
		"Alpha.md:8 docs/b.md 2-3 pin=\"" + f.first[:12] + "\"",
		"Beta.md:10 docs/a.md 10-12 pin=\"" + f.first[:12] + "\"", // the fenced example is not a source
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if only, _, _ := Collect(f.Dir, []string{"Beta.md"}); len(only) != 1 || only[0].Note != "Beta.md" {
		t.Errorf("only: %+v", only)
	}
}

func TestCheckClassifiesPinnedLines(t *testing.T) {
	f := newFixture(t)
	f.Write("docs/a.md", "new\nnew\n"+numbered(20))                                // shifts every line by two
	f.Write("docs/b.md", strings.Replace(numbered(20), "line 3\n", "edited\n", 1)) // edits a cited line
	f.Commit("edit")
	rep, err := Check(f.Dir, f.Dir, pin.ExecGit{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range rep.Rows {
		got = append(got, fmt.Sprintf("%s:%d %s %d-%d", r.Note, r.Line, r.Result.Status, r.Result.From, r.Result.To))
	}
	want := []string{"Alpha.md:7 unpinned 0-0", "Alpha.md:8 changed 0-0", "Beta.md:10 moved 12-14"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got\n%s", strings.Join(got, "\n"))
	}
}

func TestPlanPinPinsUnpinnedLinesOnly(t *testing.T) {
	f := newFixture(t)
	head := f.Git("rev-parse", "HEAD")
	p, err := PlanPin(f.Dir, f.Dir, pin.ExecGit{}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Edits) != 1 || p.Edits[0].File != "notes/Alpha.md" || p.Edits[0].Line != 7 ||
		p.Edits[0].Old != "Source: docs/a.md, lines 5-7" || p.Edits[0].New != "Source: docs/a.md, lines 5-7 @"+head[:12] {
		t.Fatalf("edits: %+v", p.Edits)
	}
	if f.read("notes/Alpha.md") == "" || strings.Contains(f.read("notes/Alpha.md"), "@"+head[:12]) {
		t.Error("planning wrote")
	}
	before := map[string][32]byte{}
	for _, n := range []string{"Alpha.md", "Beta.md", "Map - Direction.md"} {
		before[n] = sum(f.read("notes/" + n))
	}
	orig := f.read("notes/Alpha.md")
	if err := p.Apply(f.Dir); err != nil {
		t.Fatal(err)
	}
	now := f.read("notes/Alpha.md")
	if strings.ReplaceAll(now, " @"+head[:12], "") != orig {
		t.Errorf("bytes outside the Source line changed:\n%s", now)
	}
	if sum(f.read("notes/Beta.md")) != before["Beta.md"] || sum(f.read("notes/Map - Direction.md")) != before["Map - Direction.md"] {
		t.Error("an unedited note changed")
	}
	// The result validates with --check-sources and the new pin is current.
	res, err := validate.RunWith(f.Dir, nil, validate.Options{CheckSources: true, SourceRoot: f.Dir})
	if err != nil || len(res.Findings) != 0 {
		t.Errorf("findings %v %v", res.Findings, err)
	}
	f.Commit("pinned")
	rep, err := Check(f.Dir, f.Dir, pin.ExecGit{}, nil)
	if err != nil || rep.Rows[0].Result.Status != pin.Current {
		t.Errorf("%+v %v", rep.Rows, err)
	}
}

func TestPlanPinSkipsWithAReason(t *testing.T) {
	f := newFixture(t)
	f.Write("docs/a.md", "changed\n"+numbered(20)) // uncommitted
	p, err := PlanPin(f.Dir, f.Dir, pin.ExecGit{}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Edits) != 0 || p.Rows[0].Action != ActionSkip || p.Rows[0].Reason != "the file has uncommitted changes" {
		t.Errorf("%+v", p.Rows[0])
	}
}

func TestPlanPinAtAnotherRevision(t *testing.T) {
	f := newFixture(t)
	f.Write("docs/a.md", "new\n"+numbered(20))
	f.Commit("shift")
	p, err := PlanPin(f.Dir, f.Dir, pin.ExecGit{}, nil, f.first[:7])
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Edits) != 1 || !strings.HasSuffix(p.Edits[0].New, "lines 5-7 @"+f.first[:12]) {
		t.Fatalf("%+v", p.Edits)
	}
	if err := p.Apply(f.Dir); err != nil {
		t.Fatal(err)
	}
	// Pinned to the commit before the shift, the line is reported as moved.
	rep, _ := Check(f.Dir, f.Dir, pin.ExecGit{}, []string{"Alpha.md"})
	if rep.Rows[0].Result.Status != pin.Moved || rep.Rows[0].Result.From != 6 {
		t.Errorf("%+v", rep.Rows[0])
	}
}

func TestPlanUpdateRewritesMovedLinesOnly(t *testing.T) {
	f := newFixture(t)
	f.Write("docs/a.md", "new\nnew\n"+numbered(20))
	f.Write("docs/b.md", strings.Replace(numbered(20), "line 3\n", "edited\n", 1))
	f.Commit("edit")
	head := f.Git("rev-parse", "HEAD")
	p, err := PlanUpdate(f.Dir, f.Dir, pin.ExecGit{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Edits) != 1 || p.Edits[0].File != "notes/Beta.md" || p.Edits[0].Line != 10 ||
		p.Edits[0].New != "Source: docs/a.md, lines 12-14 @"+head[:12] {
		t.Fatalf("edits %+v", p.Edits)
	}
	if p.Rows[1].Action != ActionReview || len(p.Rows[1].Result.Hunks) != 1 {
		t.Errorf("the changed line must be listed for review: %+v", p.Rows[1])
	}
	alpha, mapText := f.read("notes/Alpha.md"), f.read("notes/Map - Direction.md")
	beta := f.read("notes/Beta.md")
	if err := p.Apply(f.Dir); err != nil {
		t.Fatal(err)
	}
	if f.read("notes/Alpha.md") != alpha || f.read("notes/Map - Direction.md") != mapText {
		t.Error("an unedited note changed")
	}
	if got := strings.Replace(f.read("notes/Beta.md"), "lines 12-14 @"+head[:12], "lines 10-12 @"+f.first[:12], 1); got != beta {
		t.Errorf("bytes outside the Source line changed:\n%s", f.read("notes/Beta.md"))
	}
	// The moved line holds the same text now; check is current for it, changed stays for review.
	f.Commit("updated")
	rep, _ := Check(f.Dir, f.Dir, pin.ExecGit{}, nil)
	var got []string
	for _, r := range rep.Rows {
		got = append(got, string(r.Result.Status))
	}
	if strings.Join(got, ",") != "unpinned,changed,current" {
		t.Errorf("got %v", got)
	}
	if res, _ := validate.RunWith(f.Dir, nil, validate.Options{CheckSources: true, SourceRoot: f.Dir}); len(res.Findings) != 0 {
		t.Errorf("findings %v", res.Findings)
	}
}

func TestEditsRefuseCRLFAndSymlinkedNotes(t *testing.T) {
	f := newFixture(t)
	f.Write("docs/a.md", "new\n"+numbered(20))
	f.Commit("shift")
	alpha := f.read("notes/Alpha.md")
	f.Write("notes/Beta.md", strings.ReplaceAll(f.read("notes/Beta.md"), "\n", "\r\n"))
	p, err := PlanUpdate(f.Dir, f.Dir, pin.ExecGit{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Edits) != 0 || len(p.Refused) != 1 || p.Refused[0].File != "notes/Beta.md" || p.Refused[0].Reason != "has CR line endings" {
		t.Fatalf("edits %+v refused %+v", p.Edits, p.Refused)
	}
	if p.Rows[2].Action != ActionSkip {
		t.Errorf("%+v", p.Rows[2])
	}
	// A symbolic link as a note.
	f.Write("notes/Beta.md", strings.ReplaceAll(f.read("notes/Beta.md"), "\r\n", "\n"))
	real := filepath.Join(f.Dir, "elsewhere.md")
	if err := os.Rename(filepath.Join(f.Dir, "notes", "Beta.md"), real); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, filepath.Join(f.Dir, "notes", "Beta.md")); err != nil {
		t.Fatal(err)
	}
	p, err = PlanUpdate(f.Dir, f.Dir, pin.ExecGit{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Edits) != 0 || len(p.Refused) != 1 || p.Refused[0].Reason != "is a symlink" {
		t.Fatalf("edits %+v refused %+v", p.Edits, p.Refused)
	}
	if f.read("notes/Alpha.md") != alpha {
		t.Error("planning wrote")
	}
}

func TestApplyRefusesANoteChangedSinceThePlan(t *testing.T) {
	f := newFixture(t)
	f.Write("docs/a.md", "new\n"+numbered(20))
	f.Commit("shift")
	p, err := PlanUpdate(f.Dir, f.Dir, pin.ExecGit{}, nil)
	if err != nil || len(p.Edits) == 0 {
		t.Fatal(err, p)
	}
	f.Write("notes/Beta.md", f.read("notes/Beta.md")+"\nmore\n")
	changed := f.read("notes/Beta.md")
	if err := p.Apply(f.Dir); err == nil || !strings.Contains(err.Error(), "changed since the plan") {
		t.Errorf("err = %v", err)
	}
	if f.read("notes/Beta.md") != changed {
		t.Error("the note was written")
	}
}

func TestVerifyRejectsAnEditThatAddsAFinding(t *testing.T) {
	f := newFixture(t)
	p, err := PlanPin(f.Dir, f.Dir, pin.ExecGit{}, nil, "")
	if err != nil || len(p.Edits) != 1 {
		t.Fatal(err, p)
	}
	byNote := map[string][]int{"Alpha.md": {0}}
	p.Rows[0].NewLine = "Source: docs/a.md, lines 5-99 @" + f.first[:12]
	text := rewrite(p.orig["notes/Alpha.md"], p.rows([]int{0}))
	if err := p.verify(f.Dir, map[string]string{"notes/Alpha.md": text}, byNote); err == nil || !strings.Contains(err.Error(), "source-range-invalid") {
		t.Errorf("err = %v", err)
	}
	// An edit outside the Source line is caught as well.
	p.Rows[0].NewLine = "Source: docs/a.md, lines 5-7 @" + f.first[:12]
	text = strings.Replace(rewrite(p.orig["notes/Alpha.md"], p.rows([]int{0})), "Body text.", "Body text!", 1)
	if err := p.verify(f.Dir, map[string]string{"notes/Alpha.md": text}, byNote); err == nil || !strings.Contains(err.Error(), "outside the Source lines") {
		t.Errorf("err = %v", err)
	}
}

func TestShallowCloneIsSkippedWithAReason(t *testing.T) {
	f := newFixture(t)
	f.Write("docs/a.md", "new\n"+numbered(20))
	f.Commit("shift")
	clone := filepath.Join(t.TempDir(), "clone")
	f.Git("clone", "-q", "--no-local", "--depth", "1", "file://"+f.Dir, clone)
	rep, err := Check(clone, clone, pin.ExecGit{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Skipped) != 1 || rep.Skipped[0] != "sources skipped: shallow clone (history incomplete)" {
		t.Errorf("skipped %v", rep.Skipped)
	}
	for _, r := range rep.Rows {
		if r.Source.Pin != "" && r.Result.Status != pin.Unknown {
			t.Errorf("%+v", r)
		}
	}
}
