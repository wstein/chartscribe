// SPDX-License-Identifier: EUPL-1.2

package fix

import (
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wstein/chartscribe/internal/validate"
)

const source = "Source: IDEAS_NOTES.md §1. Direction, lines 1-5\n"

// collectionWith writes a clean base collection (two maps, two notes) and the given extra files
// (slash path relative to the root -> text). A value overrides a base file.
func collectionWith(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	base := map[string]string{
		"notes/Map - Direction.md": "---\nid: 20261021120100\ntags: [\"type/map\", \"category/direction\"]\n---\nMap of the direction notes.\n\n- [[First note]]: summary\n",
		"notes/Map - State.md":     "---\nid: 20261021120101\ntags: [\"type/map\", \"category/state\"]\n---\nMap of the state notes.\n\n- [[Second note]]: summary\n",
		"notes/First note.md":      "---\nid: 20261021120000\ntags: [\"type/idea\", \"status/open\", \"category/direction\"]\n---\nBody text.\n\n" + source,
		"notes/Second note.md":     "---\nid: 20261021120001\ntags: [\"type/idea\", \"status/open\", \"category/state\"]\n---\nBody text.\n\n" + source,
	}
	for k, v := range files {
		base[k] = v
	}
	for k, v := range base {
		if v == "" {
			continue
		}
		path := filepath.Join(root, filepath.FromSlash(k))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(v), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func read(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func findings(t *testing.T, root string) []validate.Finding {
	t.Helper()
	res, err := validate.Run(root)
	if err != nil {
		t.Fatal(err)
	}
	return res.Findings
}

func bodyOf(text string) string {
	_, after, _ := strings.Cut(strings.TrimPrefix(text, "---\n"), "\n---\n")
	return after
}

func build(t *testing.T, root string, rules ...string) *Plan {
	t.Helper()
	p, err := Build(root, rules)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// fixAndCheck plans, applies, and asserts: findings drop, a second plan is empty, the body bytes of every
// note not named in appended stay identical.
func fixAndCheck(t *testing.T, root string) *Plan {
	t.Helper()
	before := findings(t, root)
	hashes := map[string][32]byte{}
	entries, _ := os.ReadDir(filepath.Join(root, "notes"))
	for _, e := range entries {
		hashes[e.Name()] = sha256.Sum256([]byte(bodyOf(read(t, root, "notes/"+e.Name()))))
	}
	p := build(t, root)
	if len(p.Edits) == 0 {
		t.Fatal("no edits planned")
	}
	if got := findings(t, root); len(got) != len(before) {
		t.Fatal("planning changed the collection")
	}
	if err := p.Apply(root); err != nil {
		t.Fatal(err)
	}
	after := findings(t, root)
	if len(after) >= len(before) {
		t.Errorf("findings %d -> %d, want a drop", len(before), len(after))
	}
	if len(after) != len(p.After) {
		t.Errorf("plan predicted %d findings, got %d", len(p.After), len(after))
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "Map") {
			continue
		}
		if h := sha256.Sum256([]byte(bodyOf(read(t, root, "notes/"+e.Name())))); h != hashes[e.Name()] {
			t.Errorf("%s: body changed", e.Name())
		}
	}
	if again := build(t, root); len(again.Edits) != 0 {
		t.Errorf("second plan has %d edits: %+v", len(again.Edits), again.Edits)
	}
	left, _ := filepath.Glob(filepath.Join(root, "notes", ".cscribe-fix-*"))
	if len(left) != 0 {
		t.Errorf("temp files left: %v", left)
	}
	return p
}

func TestTagsCaseFoldAndDedupe(t *testing.T) {
	root := collectionWith(t, map[string]string{
		"notes/First note.md": "---\nid: 20261021120000\ntags:\n  - type/idea\n  - \"status/Open\"\n  - status/open\n  - category/direction\n  - alpha\n---\nBody text.\n\n" + source,
	})
	p := fixAndCheck(t, root)
	if len(findings(t, root)) != 0 {
		t.Errorf("findings left: %v", findings(t, root))
	}
	want := "---\nid: 20261021120000\ntags: [\"type/idea\", \"status/open\", \"category/direction\", \"alpha\"]\n---\n"
	if got := read(t, root, "notes/First note.md"); !strings.HasPrefix(got, want) {
		t.Errorf("got %q", got)
	}
	e := p.Edits[0]
	if e.File != "notes/First note.md" || e.Line != 3 || e.Rule != "tag-invalid" || !strings.Contains(e.Old, "status/Open") || !strings.HasPrefix(e.New, "tags: [") {
		t.Errorf("edit = %+v", e)
	}
}

func TestInvalidTagFoldedOnlyWhenInVocabulary(t *testing.T) {
	root := collectionWith(t, map[string]string{
		"notes/First note.md": "---\nid: 20261021120000\ntags: [\"type/idea\", \"status/Done\", \"category/direction\"]\n---\nBody text.\n\n" + source,
	})
	fixAndCheck(t, root)
	// An unknown value stays: no certain fix.
	root = collectionWith(t, map[string]string{
		"notes/First note.md": "---\nid: 20261021120000\ntags: [\"type/idea\", \"status/Whatever\", \"category/direction\"]\n---\nBody text.\n\n" + source,
	})
	if p := build(t, root); len(p.Edits) != 0 {
		t.Errorf("planned %+v", p.Edits)
	}
}

func TestScalarTagsAndAliasesBecomeLists(t *testing.T) {
	root := collectionWith(t, map[string]string{
		"notes/First note.md": "---\nid: 20261021120000\naliases: First\ntags: type/idea\n---\nBody text.\n\n" + source,
	})
	p := build(t, root)
	if len(p.Edits) != 2 || p.Edits[0].New != `aliases: ["First"]` || p.Edits[1].New != `tags: ["type/idea"]` ||
		p.Edits[0].Rule != "list-type" || p.Edits[1].Rule != "list-type" {
		t.Fatalf("edits = %+v refused = %+v", p.Edits, p.Refused)
	}
}

func TestScalarTagsFixedWhenFindingsDrop(t *testing.T) {
	root := collectionWith(t, map[string]string{
		"notes/Map - State.md": "---\nid: 20261021120101\ntags: category/state\n---\nMap of the state notes.\n\n- [[Second note]]: summary\n",
	})
	fixAndCheck(t, root)
}

func TestAliasEqualToTitleRemoved(t *testing.T) {
	root := collectionWith(t, map[string]string{
		"notes/First note.md": "---\nid: 20261021120000\naliases: [\"first NOTE\", \"Other\", \"other\"]\ntags: [\"type/idea\", \"status/open\", \"category/direction\"]\n---\nBody text.\n\n" + source,
	})
	fixAndCheck(t, root)
	if got := read(t, root, "notes/First note.md"); !strings.Contains(got, "aliases: [\"Other\"]\n") {
		t.Errorf("got %q", got)
	}
}

func TestIDDerivedFromSeparators(t *testing.T) {
	root := collectionWith(t, map[string]string{
		"notes/First note.md": "---\nid: \"2026-10-21 12:00:00\"\ntags: [\"type/idea\", \"status/open\", \"category/direction\"]\n---\nBody text.\n\n" + source,
	})
	fixAndCheck(t, root)
	if got := read(t, root, "notes/First note.md"); !strings.HasPrefix(got, "---\nid: 20261021120000\n") {
		t.Errorf("got %q", got)
	}
}

func TestIDNotFixedWhenAmbiguous(t *testing.T) {
	for _, id := range []string{"2026102112000", "202610211200000", "20261301120000", "abc", "20261021120001"} {
		root := collectionWith(t, map[string]string{
			"notes/First note.md": "---\nid: " + id + "\ntags: [\"type/idea\", \"status/open\", \"category/direction\"]\n---\nBody text.\n\n" + source,
		})
		if p := build(t, root); len(p.Edits) != 0 {
			t.Errorf("id %q: planned %+v", id, p.Edits)
		}
	}
}

func TestMissingIDNotFixed(t *testing.T) {
	root := collectionWith(t, map[string]string{
		"notes/First note.md": "---\ntags: [\"type/idea\", \"status/open\", \"category/direction\"]\n---\nBody text.\n\n" + source,
	})
	if p := build(t, root); len(p.Edits) != 0 {
		t.Errorf("planned %+v", p.Edits)
	}
}

func TestMapUnlistedAppendsToTheOnlyMapOfTheCategory(t *testing.T) {
	root := collectionWith(t, map[string]string{
		"notes/Third note.md":  "---\nid: 20261021120002\ntags: [\"type/idea\", \"status/open\", \"category/state\"]\n---\nBody text.\n\n" + source,
		"notes/Fourth note.md": "---\nid: 20261021120003\ntags: [\"type/idea\", \"status/open\", \"category/state\"]\n---\nBody text.\n\n" + source,
	})
	p := fixAndCheck(t, root)
	got := read(t, root, "notes/Map - State.md")
	if !strings.HasSuffix(got, "- [[Second note]]: summary\n- [[Fourth note]]\n- [[Third note]]\n") {
		t.Errorf("got %q", got)
	}
	for _, e := range p.Edits {
		if e.Rule != "map-unlisted" || e.File != "notes/Map - State.md" || e.Old != "" {
			t.Errorf("edit = %+v", e)
		}
	}
	if len(findings(t, root)) != 0 {
		t.Errorf("findings left: %v", findings(t, root))
	}
}

func TestMapUnlistedNotFixedWithoutExactlyOneMap(t *testing.T) {
	// two maps for the category
	root := collectionWith(t, map[string]string{
		"notes/Third note.md":    "---\nid: 20261021120002\ntags: [\"type/idea\", \"status/open\", \"category/state\"]\n---\nBody text.\n\n" + source,
		"notes/Map - State 2.md": "---\nid: 20261021120102\ntags: [\"type/map\", \"category/state\"]\n---\nMap.\n",
	})
	if p := build(t, root); len(p.Edits) != 0 {
		t.Errorf("two maps: planned %+v", p.Edits)
	}
	// no map for the category
	root = collectionWith(t, map[string]string{
		"notes/Third note.md": "---\nid: 20261021120002\ntags: [\"type/idea\", \"status/open\", \"category/roadmap\"]\n---\nBody text.\n\n" + source,
	})
	if p := build(t, root); len(p.Edits) != 0 {
		t.Errorf("no map: planned %+v", p.Edits)
	}
}

func TestFixThatDoesNotValidateIsRefused(t *testing.T) {
	// The only links of the map sit in a generated tracking section, which validate ignores: a link
	// added there would not list the note.
	root := collectionWith(t, map[string]string{
		"notes/Map - State.md": "---\nid: 20261021120101\ntags: [\"type/map\", \"category/state\"]\n---\nMap.\n\n## Tracking\n\n- [[Second note]]\n",
		"notes/Third note.md":  "---\nid: 20261021120002\ntags: [\"type/idea\", \"status/open\", \"category/state\"]\n---\nBody text.\n\n" + source,
	})
	before := read(t, root, "notes/Map - State.md")
	p := build(t, root)
	if len(p.Edits) != 0 || len(p.Refused) != 1 || p.Refused[0].File != "notes/Map - State.md" {
		t.Fatalf("edits = %+v refused = %+v", p.Edits, p.Refused)
	}
	if err := p.Apply(root); err != nil {
		t.Fatal(err)
	}
	if read(t, root, "notes/Map - State.md") != before {
		t.Error("refused fix was written")
	}
}

func TestFixThatAddsAFindingIsRefused(t *testing.T) {
	// The fix lowers status/Open and removes the repeated type/idea, so tag-invalid goes, but
	// "found 3" becomes "found 2" for type/: fewer findings, one of them new.
	root := collectionWith(t, map[string]string{
		"notes/First note.md": "---\nid: 20261021120000\ntags: [\"type/idea\", \"type/idea\", \"type/decision\", \"status/Open\", \"category/direction\"]\n---\nBody text.\n\n" + source,
	})
	p := build(t, root)
	if len(p.Edits) != 0 || len(p.Refused) != 1 {
		t.Fatalf("edits = %+v refused = %+v", p.Edits, p.Refused)
	}
}

func TestBetter(t *testing.T) {
	a := validate.Finding{Rule: "tag-count", Note: "A.md", Message: "x"}
	b := validate.Finding{Rule: "tag-count", Note: "B.md", Message: "y"}
	c := validate.Finding{Rule: "tag-count", Note: "C.md", Message: "z"}
	for _, tt := range []struct {
		name          string
		before, after []validate.Finding
		want          bool
	}{
		{"fewer", []validate.Finding{a, b}, []validate.Finding{a}, true},
		{"same", []validate.Finding{a, b}, []validate.Finding{a, b}, false},
		{"fewer but new", []validate.Finding{a, b}, []validate.Finding{c}, false},
		{"all fixed", []validate.Finding{a}, nil, true},
	} {
		if got := better(tt.before, tt.after); got != tt.want {
			t.Errorf("%s: %v", tt.name, got)
		}
	}
}

func TestSameDerivedIDIsFixedOnce(t *testing.T) {
	tags := "tags: [\"type/idea\", \"status/open\", \"category/direction\"]"
	root := collectionWith(t, map[string]string{
		"notes/Gamma.md":           "---\nid: \"2026-10-21 13:00:00\"\n" + tags + "\n---\nBody text.\n\n" + source,
		"notes/Hotel.md":           "---\nid: 20261021130000x\n" + tags + "\n---\nBody text.\n\n" + source,
		"notes/Map - Direction.md": "---\nid: 20261021120100\ntags: [\"type/map\", \"category/direction\"]\n---\nMap.\n\n- [[First note]]\n- [[Gamma]]\n- [[Hotel]]\n",
	})
	// Hotel's id has a letter: not derivable. Make it derive the same value instead.
	putFile(t, root, "notes/Hotel.md", "---\nid: \"2026/10/21 13:00:00\"\n"+tags+"\n---\nBody text.\n\n"+source)
	p := build(t, root)
	ids := 0
	for _, e := range p.Edits {
		if e.Rule == "id-invalid" {
			ids++
		}
	}
	if ids > 1 {
		t.Fatalf("both notes were given the same id: %+v", p.Edits)
	}
	if err := p.Apply(root); err != nil {
		t.Fatal(err)
	}
	for _, f := range findings(t, root) {
		if f.Rule == "id-duplicate" {
			t.Errorf("duplicate id: %v", f)
		}
	}
}

func putFile(t *testing.T, root, rel, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(rel)), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestOnlyNamedTagsChange(t *testing.T) {
	root := collectionWith(t, map[string]string{
		"notes/First note.md": "---\nid: 20261021120000\ntags: [\"type/idea\", \"status/Open\", \"category/direction\", \"Type/Idea\", \"Free\", \"Free\"]\n---\nBody text.\n\n" + source,
	})
	p := build(t, root)
	want := `tags: ["type/idea", "status/open", "category/direction", "Type/Idea", "Free", "Free"]`
	if len(p.Edits) != 1 || p.Edits[0].New != want || p.Edits[0].Rule != "tag-invalid" {
		t.Errorf("edits = %+v", p.Edits)
	}
}

func TestMapLinkGoesToTheListNotTheEndOfTheFile(t *testing.T) {
	root := collectionWith(t, map[string]string{
		"notes/Map - State.md": "---\nid: 20261021120101\ntags: [\"type/map\", \"category/state\"]\n---\nMap.\n\n* [[Second note]]\n* [[Fifth note]]\n\nClosing words.\n",
		"notes/Fifth note.md":  "---\nid: 20261021120005\ntags: [\"type/idea\", \"status/open\", \"category/state\"]\n---\nBody text.\n\n" + source,
		"notes/Third note.md":  "---\nid: 20261021120002\ntags: [\"type/idea\", \"status/open\", \"category/state\"]\n---\nBody text.\n\n" + source,
	})
	fixAndCheck(t, root)
	want := "* [[Second note]]\n* [[Fifth note]]\n* [[Third note]]\n\nClosing words.\n"
	if got := read(t, root, "notes/Map - State.md"); !strings.HasSuffix(got, want) {
		t.Errorf("got %q", got)
	}
}

func TestMapLinkNotFixedWithoutOneListOrWithUnknownMaps(t *testing.T) {
	third := "---\nid: 20261021120002\ntags: [\"type/idea\", \"status/open\", \"category/state\"]\n---\nBody text.\n\n" + source
	for name, files := range map[string]map[string]string{
		"no list":     {"notes/Map - State.md": "---\nid: 20261021120101\ntags: [\"type/map\", \"category/state\"]\n---\nMap.\n", "notes/Third note.md": third},
		"two lists":   {"notes/Map - State.md": "---\nid: 20261021120101\ntags: [\"type/map\", \"category/state\"]\n---\n- [[Second note]]\n\ntext\n\n- [[First note]]\n", "notes/Third note.md": third},
		"fenced link": {"notes/Map - State.md": "---\nid: 20261021120101\ntags: [\"type/map\", \"category/state\"]\n---\n```\n- [[Second note]]\n```\n", "notes/Third note.md": third},
		"broken map":  {"notes/Map - State 2.md": "no frontmatter\n", "notes/Third note.md": third},
	} {
		root := collectionWith(t, files)
		if p := build(t, root); len(p.Edits) != 0 {
			t.Errorf("%s: planned %+v", name, p.Edits)
		}
	}
}
func TestReadmeLinkCaseFold(t *testing.T) {
	root := collectionWith(t, map[string]string{
		"README.md":     "# Notes\n\nSee [a](Docs/Guide.MD#top), [b](DOCS/guide.md) and [c](https://example.org/X).\n[gone](nothing.md)\n",
		"docs/guide.md": "x\n",
	})
	p := build(t, root)
	if len(p.Edits) != 1 || p.Edits[0].File != "README.md" || p.Edits[0].Rule != "readme-link" || p.Edits[0].Line != 3 {
		t.Fatalf("edits = %+v", p.Edits)
	}
	if err := p.Apply(root); err != nil {
		t.Fatal(err)
	}
	want := "# Notes\n\nSee [a](docs/guide.md#top), [b](docs/guide.md) and [c](https://example.org/X).\n[gone](nothing.md)\n"
	if got := read(t, root, "README.md"); got != want {
		t.Errorf("got %q", got)
	}
	left := findings(t, root)
	if len(left) != 1 || left[0].Rule != "readme-link" {
		t.Errorf("left = %v", left)
	}
	if again := build(t, root); len(again.Edits) != 0 {
		t.Errorf("second plan: %+v", again.Edits)
	}
}

func TestAmbiguousCaseFoldNotFixed(t *testing.T) {
	root := collectionWith(t, map[string]string{
		"README.md":     "[a](docs/GUIDE.md)\n",
		"docs/guide.md": "x\n",
		"docs/Guide.md": "y\n",
	})
	if entries, _ := os.ReadDir(filepath.Join(root, "docs")); len(entries) < 2 {
		t.Skip("case-insensitive file system")
	}
	if p := build(t, root); len(p.Edits) != 0 {
		t.Errorf("planned %+v", p.Edits)
	}
}

func TestUnfixableRulesStayFindings(t *testing.T) {
	root := collectionWith(t, map[string]string{
		"notes/First note.md": "---\nid: 20261021120000\ntags: [\"type/idea\", \"status/open\", \"category/direction\"]\n---\nSee [[Missing]].\n",
		"notes/Bad: title.md": "---\nid: 20261021120009\ntags: [\"type/idea\", \"status/open\", \"category/direction\"]\n---\nBody text.\n\n" + source,
	})
	if p := build(t, root); len(p.Edits) != 0 || len(p.Refused) != 0 {
		t.Errorf("edits = %+v refused = %+v", p.Edits, p.Refused)
	}
}

func TestRuleFilter(t *testing.T) {
	root := collectionWith(t, map[string]string{
		"notes/First note.md": "---\nid: \"2026-10-21 12:00:00\"\naliases: [\"First note\"]\ntags: [\"type/idea\", \"status/open\", \"category/direction\"]\n---\nBody text.\n\n" + source,
	})
	p := build(t, root, "id-invalid")
	if len(p.Edits) != 1 || p.Edits[0].Rule != "id-invalid" {
		t.Errorf("edits = %+v", p.Edits)
	}
}

func TestSymlinkNoteRefused(t *testing.T) {
	root := collectionWith(t, map[string]string{
		"outside.md": "---\nid: 20261021120000\ntags: [\"type/idea\", \"status/Open\", \"category/direction\"]\n---\nBody text.\n\n" + source,
	})
	link := filepath.Join(root, "notes", "First note.md")
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "outside.md"), link); err != nil {
		t.Fatal(err)
	}
	before := read(t, root, "outside.md")
	p := build(t, root)
	if len(p.Edits) != 0 || len(p.Refused) != 1 || !strings.Contains(p.Refused[0].Reason, "symlink") {
		t.Fatalf("edits = %+v refused = %+v", p.Edits, p.Refused)
	}
	_ = p.Apply(root)
	if read(t, root, "outside.md") != before {
		t.Error("wrote through a symlink")
	}
}

func TestCRLFNoteRefused(t *testing.T) {
	text := "---\r\nid: 20261021120000\r\ntags: [\"type/idea\", \"status/Open\", \"category/direction\"]\r\n---\r\nBody text.\r\n\r\n" + source
	root := collectionWith(t, map[string]string{"notes/First note.md": text})
	p := build(t, root)
	if len(p.Edits) != 0 || len(p.Refused) != 1 {
		t.Fatalf("edits = %+v refused = %+v", p.Edits, p.Refused)
	}
}

func TestApplyRefusesAChangedFile(t *testing.T) {
	root := collectionWith(t, map[string]string{
		"notes/First note.md": "---\nid: 20261021120000\ntags: [\"type/idea\", \"status/Open\", \"category/direction\"]\n---\nBody text.\n\n" + source,
	})
	p := build(t, root)
	if len(p.Edits) == 0 {
		t.Fatal("no edits")
	}
	changed := "---\nid: 20261021120000\ntags: [\"type/idea\", \"status/Open\", \"category/direction\"]\n---\nEdited meanwhile.\n\n" + source
	if err := os.WriteFile(filepath.Join(root, "notes", "First note.md"), []byte(changed), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := p.Apply(root); err == nil {
		t.Fatal("want an error")
	}
	if read(t, root, "notes/First note.md") != changed {
		t.Error("changed file was overwritten")
	}
}

func TestCleanCollectionPlansNothing(t *testing.T) {
	root := collectionWith(t, nil)
	p := build(t, root)
	if len(p.Edits) != 0 || len(p.Refused) != 0 || len(p.Before) != 0 || len(p.After) != 0 {
		t.Errorf("%+v", p)
	}
}

func TestFileModeKept(t *testing.T) {
	root := collectionWith(t, map[string]string{
		"notes/First note.md": "---\nid: 20261021120000\ntags: [\"type/idea\", \"status/Open\", \"category/direction\"]\n---\nBody text.\n\n" + source,
	})
	path := filepath.Join(root, "notes", "First note.md")
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := build(t, root).Apply(root); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Errorf("mode = %v", info.Mode())
	}
}

// twoFileCollection needs a note fix and a README fix.
func twoFileCollection(t *testing.T) (root string, p *Plan) {
	t.Helper()
	root = collectionWith(t, map[string]string{
		"notes/First note.md": "---\nid: 20261021120000\ntags: [\"type/idea\", \"status/Open\", \"category/direction\"]\n---\nBody text.\n\n" + source,
		"README.md":           "[a](DOCS/guide.md)\n",
		"docs/guide.md":       "x\n",
	})
	p = build(t, root)
	if len(p.Edits) != 2 {
		t.Fatalf("edits = %+v", p.Edits)
	}
	return root, p
}

func TestApplyWritesNothingWhenADirectoryIsReadOnly(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory modes")
	}
	root, p := twoFileCollection(t)
	readme, note := read(t, root, "README.md"), read(t, root, "notes/First note.md")
	if err := os.Chmod(filepath.Join(root, "notes"), 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(filepath.Join(root, "notes"), 0o755)
	err := p.Apply(root)
	var werr *WriteError
	if !errors.As(err, &werr) || len(werr.NotWritten) != 2 || len(werr.Written) != 0 {
		t.Fatalf("err = %v", err)
	}
	if read(t, root, "README.md") != readme || read(t, root, "notes/First note.md") != note {
		t.Error("a file was written")
	}
}

func TestApplyRestoresWhenALaterWriteFails(t *testing.T) {
	root, p := twoFileCollection(t)
	readme, note := read(t, root, "README.md"), read(t, root, "notes/First note.md")
	calls := 0
	renameFile = func(from, to string) error {
		calls++
		if calls == 2 {
			return errors.New("disk full")
		}
		return os.Rename(from, to)
	}
	defer func() { renameFile = os.Rename }()
	err := p.Apply(root)
	var werr *WriteError
	if !errors.As(err, &werr) || len(werr.Restored) != 1 || werr.Restored[0] != "README.md" ||
		len(werr.NotWritten) != 1 || len(werr.Written) != 0 || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("err = %v", err)
	}
	if read(t, root, "README.md") != readme || read(t, root, "notes/First note.md") != note {
		t.Error("files were not restored")
	}
	left, _ := filepath.Glob(filepath.Join(root, "notes", ".cscribe-fix-*"))
	if len(left) != 0 {
		t.Errorf("temp files left: %v", left)
	}
}

func TestApplyRefusesASymlinkSwappedInAfterThePlan(t *testing.T) {
	root, p := twoFileCollection(t)
	path := filepath.Join(root, "notes", "First note.md")
	copyPath := filepath.Join(root, "copy.md")
	if err := os.Rename(path, copyPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(copyPath, path); err != nil {
		t.Fatal(err)
	}
	err := p.Apply(root)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(read(t, root, "copy.md"), "status/open") {
		t.Error("wrote through the symlink")
	}
}

func TestApplyRefusesADirectorySwappedForASymlink(t *testing.T) {
	root, p := twoFileCollection(t)
	if err := os.Rename(filepath.Join(root, "notes"), filepath.Join(root, "real-notes")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "real-notes"), filepath.Join(root, "notes")); err != nil {
		t.Fatal(err)
	}
	err := p.Apply(root)
	if err == nil || !strings.Contains(err.Error(), "leaves the collection") {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(read(t, root, "real-notes/First note.md"), "status/open") {
		t.Error("wrote through the symlinked directory")
	}
}
