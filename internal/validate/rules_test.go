// SPDX-License-Identifier: EUPL-1.2

package validate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The tests below port chartroom/scripts/test_check_notes.py rule by rule.

func TestValidCollection(t *testing.T) {
	root := newFixture(t)
	assertClean(t, root)
	res, err := Run(root)
	if err != nil || res.Notes != 4 {
		t.Errorf("notes = %d, err = %v", res.Notes, err)
	}
}

func TestFrontmatterRules(t *testing.T) {
	t.Run("missing frontmatter", func(t *testing.T) {
		root := newFixture(t)
		writeFile(t, filepath.Join(root, "notes", "First note.md"), "just text\n")
		assertFinding(t, root, "frontmatter", "First note.md")
	})
	t.Run("unterminated and bad line", func(t *testing.T) {
		for _, text := range []string{"---\nid: 1\n", "---\nid 1\n---\nx\n"} {
			root := newFixture(t)
			writeFile(t, filepath.Join(root, "notes", "First note.md"), text)
			assertFinding(t, root, "frontmatter", "First note.md")
		}
	})
	t.Run("id format", func(t *testing.T) {
		for _, bad := range []string{"abc", "202610211200", "2026102112000"} {
			root := newFixture(t)
			writeNote(t, root, "First note", bad)
			assertFinding(t, root, "id-invalid", "First note.md")
		}
	})
	t.Run("id suffix is accepted", func(t *testing.T) {
		root := newFixture(t)
		writeNote(t, root, "First note", "20261021120000-001")
		assertClean(t, root)
	})
	t.Run("id must be a calendar date", func(t *testing.T) {
		for _, bad := range []string{"20261321120000", "20261032120000", "20261021250000", "20261021126100"} {
			root := newFixture(t)
			writeNote(t, root, "First note", bad)
			assertFinding(t, root, "id-invalid", "First note.md")
		}
	})
	t.Run("missing id", func(t *testing.T) {
		root := newFixture(t)
		edit(t, root, "First note", "id: 20261021120000\n", "")
		assertFinding(t, root, "id-missing", "First note.md")
	})
	t.Run("duplicate ids", func(t *testing.T) {
		root := newFixture(t)
		writeNote(t, root, "Second note", "20261021120000", noteOpts{category: "state", links: []string{"First note"}})
		assertFinding(t, root, "id-duplicate", "")
	})
	t.Run("unknown field", func(t *testing.T) {
		root := newFixture(t)
		writeNote(t, root, "First note", "20261021120000", noteOpts{extra: "title: X\n"})
		assertFinding(t, root, "field-unknown", "First note.md")
	})
	t.Run("tags must be a list of strings", func(t *testing.T) {
		root := newFixture(t)
		edit(t, root, "First note", `tags: ["type/idea"`, `tags: "type/idea" #`)
		assertFinding(t, root, "list-type", "First note.md")
	})
	t.Run("missing tags is a list-type finding", func(t *testing.T) {
		root := newFixture(t)
		writeFile(t, filepath.Join(root, "notes", "First note.md"), "---\nid: 20261021120000\n---\nBody\n\nSource: x, lines 1-2\n")
		assertFinding(t, root, "list-type", "First note.md")
	})
	t.Run("aliases must be a list", func(t *testing.T) {
		root := newFixture(t)
		writeNote(t, root, "First note", "20261021120000", noteOpts{extra: "aliases: Primus\n"})
		assertFinding(t, root, "list-type", "First note.md")
	})
	t.Run("block lists are accepted", func(t *testing.T) {
		root := newFixture(t)
		writeNote(t, root, "First note", "20261021120000", noteOpts{extra: "aliases:\n  - One\n"})
		assertClean(t, root)
	})
	t.Run("obsidian style frontmatter is accepted", func(t *testing.T) {
		root := newFixture(t)
		writeFile(t, filepath.Join(root, "notes", "First note.md"),
			"---\nid: '20261021120000'\naliases: []\ntags:\n  - type/idea\n  - status/open\n  - category/direction\n---\nBody.\n\nSource: x, lines 1-2\n")
		assertClean(t, root)
	})
	t.Run("invalid namespaced tags", func(t *testing.T) {
		for _, c := range [][2]string{{"type/idea", "type/essay"}, {"status/open", "status/wip"}, {"category/direction", "category/misc"}} {
			root := newFixture(t)
			edit(t, root, "First note", c[0], c[1])
			assertFinding(t, root, "tag-invalid", "First note.md")
		}
	})
	t.Run("each namespace exactly once", func(t *testing.T) {
		all := []string{"type/idea", "status/open", "category/direction"}
		for i, ns := range []string{"type", "status", "category"} {
			root := newFixture(t)
			var tags []string
			for j, tag := range all {
				if j != i {
					tags = append(tags, tag)
				}
			}
			writeNote(t, root, "First note", "20261021120000", noteOpts{tags: tags})
			fs := check(t, root)
			if !has(fs, "tag-count", "First note.md") {
				t.Errorf("%s: want tag-count, got %v", ns, fs)
			}
		}
	})
	t.Run("two tags of one namespace", func(t *testing.T) {
		root := newFixture(t)
		writeNote(t, root, "First note", "20261021120000", noteOpts{tags: []string{"type/idea", "type/design", "status/open", "category/direction"}})
		assertFinding(t, root, "tag-count", "First note.md")
	})
	t.Run("a map needs no status tag but one category tag", func(t *testing.T) {
		root := newFixture(t)
		assertClean(t, root)
		edit(t, root, "Map - State", `, "category/state"`, "")
		assertFinding(t, root, "tag-count", "Map - State.md")
	})
	t.Run("source line required", func(t *testing.T) {
		root := newFixture(t)
		edit(t, root, "First note", "Source: IDEAS_NOTES.md §1. Direction, lines 1-5", "")
		assertFinding(t, root, "source-missing", "First note.md")
	})
	t.Run("source line needs the lines range", func(t *testing.T) {
		root := newFixture(t)
		edit(t, root, "First note", ", lines 1-5", "")
		assertFinding(t, root, "source-missing", "First note.md")
	})
}

func TestTitleRules(t *testing.T) {
	t.Run("forbidden filename characters", func(t *testing.T) {
		root := newFixture(t)
		data, _ := os.ReadFile(filepath.Join(root, "notes", "First note.md"))
		writeFile(t, filepath.Join(root, "notes", "Bad: title.md"), string(data))
		assertFinding(t, root, "title-char", "Bad: title.md")
	})
	t.Run("characters that cut a wikilink", func(t *testing.T) {
		for _, c := range []string{"#", "[", "]", "^"} {
			root := newFixture(t)
			data, _ := os.ReadFile(filepath.Join(root, "notes", "First note.md"))
			name := "Bad" + c + " title.md"
			writeFile(t, filepath.Join(root, "notes", name), string(data))
			assertFinding(t, root, "title-char", name)
		}
	})
	t.Run("duplicate via alias", func(t *testing.T) {
		root := newFixture(t)
		writeNote(t, root, "Second note", "20261021120001", noteOpts{category: "state", links: []string{"First note"}, extra: "aliases: [\"First note\"]\n"})
		assertFinding(t, root, "name-duplicate", "")
	})
	t.Run("duplicate differing in case only", func(t *testing.T) {
		root := newFixture(t)
		if caseInsensitive(t, root) {
			t.Skip("case-insensitive filesystem cannot hold names that differ only by case")
		}
		writeNote(t, root, "FIRST NOTE", "20261021120002")
		writeMap(t, root, "Direction", "direction", "20261021120100", []string{"First note", "FIRST NOTE"})
		assertFinding(t, root, "name-duplicate", "")
	})
}

func TestWikilinkRules(t *testing.T) {
	t.Run("alias resolves", func(t *testing.T) {
		root := newFixture(t)
		writeNote(t, root, "First note", "20261021120000", noteOpts{extra: "aliases: [\"Primus\"]\n"})
		writeNote(t, root, "Second note", "20261021120001", noteOpts{category: "state", links: []string{"Primus"}})
		assertClean(t, root)
	})
	t.Run("label and heading", func(t *testing.T) {
		root := newFixture(t)
		writeNote(t, root, "Second note", "20261021120001", noteOpts{category: "state", links: []string{"First note#Part|label"}})
		assertClean(t, root)
	})
	t.Run("case insensitive", func(t *testing.T) {
		root := newFixture(t)
		writeNote(t, root, "Second note", "20261021120001", noteOpts{category: "state", links: []string{"first NOTE"}})
		assertClean(t, root)
	})
	t.Run("broken link in the links list", func(t *testing.T) {
		root := newFixture(t)
		writeNote(t, root, "Second note", "20261021120001", noteOpts{category: "state", links: []string{"Ghost"}})
		assertFinding(t, root, "wikilink-unresolved", "Second note.md")
	})
	t.Run("broken link in the body", func(t *testing.T) {
		root := newFixture(t)
		writeNote(t, root, "Second note", "20261021120001", noteOpts{category: "state", body: "See [[Ghost]]."})
		assertFinding(t, root, "wikilink-unresolved", "Second note.md")
	})
	t.Run("link inside a code block is checked", func(t *testing.T) {
		root := newFixture(t)
		writeNote(t, root, "Second note", "20261021120001", noteOpts{category: "state", body: "```\n[[Ghost]]\n```"})
		assertFinding(t, root, "wikilink-unresolved", "Second note.md")
	})
	t.Run("markdown links in notes are not checked", func(t *testing.T) {
		root := newFixture(t)
		writeNote(t, root, "First note", "20261021120000", noteOpts{body: "See [x](workharbor/specs/a.md)."})
		assertClean(t, root)
	})
	t.Run("tracking section is ignored", func(t *testing.T) {
		root := newFixture(t)
		path := filepath.Join(root, "notes", "First note.md")
		data, _ := os.ReadFile(path)
		writeFile(t, path, string(data)+"\n## Tracking\n\n- #7 Fix [[Ghost]] (open) https://example.org/7\n")
		assertClean(t, root)
	})
	t.Run("links after the tracking section are checked", func(t *testing.T) {
		root := newFixture(t)
		path := filepath.Join(root, "notes", "First note.md")
		data, _ := os.ReadFile(path)
		writeFile(t, path, string(data)+"\n## Tracking\n\n- #7 x (open) u\n\n## More\n\n[[Ghost]]\n")
		assertFinding(t, root, "wikilink-unresolved", "First note.md")
	})
}

func TestReadmeRules(t *testing.T) {
	cases := []struct {
		name, readme, rule string
	}{
		{"outside", "[outside](../Makefile)\n", "readme-link"},
		{"absolute path", "[abs](/etc/hosts)\n", "readme-link"},
		{"broken", "[gone](notes/Gone.md)\n", "readme-link"},
		{"inside and fragment", "[a](notes) [r](.) [n](notes/../notes#x)\n", ""},
		{"external and anchor", "[s](https://example.org/x.md) [a](#top) [m](mailto:a@b.c)\n", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := newFixture(t)
			writeFile(t, filepath.Join(root, "README.md"), c.readme)
			fs := check(t, root)
			if c.rule == "" {
				if len(fs) != 0 {
					t.Errorf("want no findings, got %v", fs)
				}
				return
			}
			if !has(fs, c.rule, "README.md") {
				t.Errorf("want %s, got %v", c.rule, fs)
			}
		})
	}
	t.Run("no readme", func(t *testing.T) {
		root := newFixture(t)
		_ = os.Remove(filepath.Join(root, "README.md"))
		assertClean(t, root)
	})
}

func TestMapRules(t *testing.T) {
	t.Run("note missing from maps", func(t *testing.T) {
		root := newFixture(t)
		writeMap(t, root, "State", "state", "20261021120101", nil)
		assertFinding(t, root, "map-unlisted", "Second note.md")
	})
	t.Run("note listed in two maps", func(t *testing.T) {
		root := newFixture(t)
		writeMap(t, root, "State", "state", "20261021120101", []string{"Second note", "First note"})
		assertFinding(t, root, "map-multiple", "First note.md")
	})
	t.Run("several links from one map count once", func(t *testing.T) {
		root := newFixture(t)
		writeMap(t, root, "Direction", "direction", "20261021120100", []string{"First note", "First note"})
		assertClean(t, root)
	})
	t.Run("alias and title links from one map count once", func(t *testing.T) {
		root := newFixture(t)
		writeNote(t, root, "First note", "20261021120000", noteOpts{extra: "aliases: [\"Primus\"]\n"})
		writeMap(t, root, "Direction", "direction", "20261021120100", []string{"First note", "Primus"})
		assertClean(t, root)
	})
	t.Run("map category must match note category", func(t *testing.T) {
		root := newFixture(t)
		writeMap(t, root, "Direction", "direction", "20261021120100", []string{"First note", "Second note"})
		writeMap(t, root, "State", "state", "20261021120101", nil)
		assertFinding(t, root, "map-category", "Second note.md")
	})
	t.Run("category without map", func(t *testing.T) {
		root := newFixture(t)
		_ = os.Remove(filepath.Join(root, "notes", "Map - State.md"))
		assertFinding(t, root, "category-no-map", "")
		assertFinding(t, root, "map-unlisted", "Second note.md")
	})
	t.Run("two maps for one category", func(t *testing.T) {
		root := newFixture(t)
		writeMap(t, root, "State again", "state", "20261021120102", nil)
		assertFinding(t, root, "map-duplicate", "")
	})
	t.Run("a note linked from two maps of one category counts twice", func(t *testing.T) {
		root := newFixture(t)
		writeMap(t, root, "State again", "state", "20261021120102", []string{"Second note"})
		assertFinding(t, root, "map-multiple", "Second note.md")
	})
	t.Run("a map is not required to be listed", func(t *testing.T) {
		root := newFixture(t)
		assertClean(t, root)
	})
}

func TestVocabularyFromConfig(t *testing.T) {
	root := newFixture(t)
	writeFile(t, filepath.Join(root, ".cscribe.toml"), "[tags]\ntype = [\"thought\"]\n")
	assertFinding(t, root, "tag-invalid", "First note.md")
	edit(t, root, "First note", "type/idea", "type/thought")
	edit(t, root, "Second note", "type/idea", "type/thought")
	assertClean(t, root)
}

func TestBadConfigIsAnError(t *testing.T) {
	root := newFixture(t)
	writeFile(t, filepath.Join(root, ".cscribe.toml"), "[nope]\n")
	if _, err := Run(root); err == nil {
		t.Error("want an error for an invalid .cscribe.toml")
	}
}

func TestFindingsAreSortedAndUnique(t *testing.T) {
	root := newFixture(t)
	writeNote(t, root, "Second note", "20261021120001", noteOpts{category: "state", body: "[[Ghost]] [[Ghost]] [[Aaa]]"})
	fs := check(t, root)
	for i := 1; i < len(fs); i++ {
		a, b := fs[i-1], fs[i]
		if a.Note > b.Note || (a.Note == b.Note && a.Rule > b.Rule) || a == b {
			t.Errorf("findings not sorted and unique at %d: %v then %v", i, a, b)
		}
	}
	if n := countRule(fs, "wikilink-unresolved"); n != 2 {
		t.Errorf("wikilink-unresolved findings = %d, want 2 (Ghost deduplicated)", n)
	}
}

func countRule(fs []Finding, rule string) int {
	n := 0
	for _, f := range fs {
		if f.Rule == rule {
			n++
		}
	}
	return n
}

func TestRuleNames(t *testing.T) {
	want := []string{"frontmatter", "field-unknown", "id-missing", "id-invalid", "id-duplicate", "list-type", "tag-invalid", "tag-count",
		"title-char", "name-duplicate", "source-missing", "wikilink-unresolved", "map-unlisted", "map-multiple", "map-category",
		"map-duplicate", "category-no-map", "readme-link",
		"source-kind-unknown", "source-malformed", "source-file-missing", "source-not-file", "source-range-invalid", "source-outside-root"}
	if strings.Join(Rules, ",") != strings.Join(want, ",") {
		t.Errorf("Rules = %v", Rules)
	}
}
