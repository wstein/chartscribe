// SPDX-License-Identifier: EUPL-1.2

package fix

import (
	"crypto/sha256"
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
		"notes/First note.md": "---\nid: 20261021120000\ntags:\n  - \"Type/Idea\"\n  - type/idea\n  - '#status/open'\n  - category/direction\n  - alpha\n---\nBody text.\n\n" + source,
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
	if e.File != "notes/First note.md" || e.Line != 3 || e.Rule != "tag-count" || !strings.Contains(e.Old, "Type/Idea") || !strings.HasPrefix(e.New, "tags: [") {
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
	// The map ends in a generated tracking section, which validate ignores: a link added there
	// would not list the note.
	root := collectionWith(t, map[string]string{
		"notes/Map - State.md": "---\nid: 20261021120101\ntags: [\"type/map\", \"category/state\"]\n---\nMap.\n\n- [[Second note]]\n\n## Tracking\n\n- #1 x (open) https://example.org/1\n",
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
		"outside.md": "---\nid: 20261021120000\ntags: [\"Type/idea\", \"status/open\", \"category/direction\"]\n---\nBody text.\n\n" + source,
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
	text := "---\r\nid: 20261021120000\r\ntags: [\"Type/idea\", \"status/open\", \"category/direction\"]\r\n---\r\nBody text.\r\n\r\n" + source
	root := collectionWith(t, map[string]string{"notes/First note.md": text})
	p := build(t, root)
	if len(p.Edits) != 0 || len(p.Refused) != 1 {
		t.Fatalf("edits = %+v refused = %+v", p.Edits, p.Refused)
	}
}

func TestApplyRefusesAChangedFile(t *testing.T) {
	root := collectionWith(t, map[string]string{
		"notes/First note.md": "---\nid: 20261021120000\ntags: [\"Type/idea\", \"status/open\", \"category/direction\"]\n---\nBody text.\n\n" + source,
	})
	p := build(t, root)
	if len(p.Edits) == 0 {
		t.Fatal("no edits")
	}
	changed := "---\nid: 20261021120000\ntags: [\"Type/idea\", \"status/open\", \"category/direction\"]\n---\nEdited meanwhile.\n\n" + source
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
		"notes/First note.md": "---\nid: 20261021120000\ntags: [\"Type/idea\", \"status/open\", \"category/direction\"]\n---\nBody text.\n\n" + source,
	})
	path := filepath.Join(root, "notes", "First note.md")
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := build(t, root).Apply(root); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v", info.Mode())
	}
}
