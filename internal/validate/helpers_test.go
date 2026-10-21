// SPDX-License-Identifier: EUPL-1.2

package validate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// noteOpts mirrors write_note() of chartroom/scripts/test_check_notes.py.
type noteOpts struct {
	category, typ, status string
	links                 []string
	tags                  []string // nil: derived from category, typ, status
	extra                 string   // extra frontmatter lines
	body                  string
}

func (o noteOpts) withDefaults() noteOpts {
	if o.category == "" {
		o.category = "direction"
	}
	if o.typ == "" {
		o.typ = "idea"
	}
	if o.status == "" {
		o.status = "open"
	}
	if o.body == "" {
		o.body = "Body text."
	}
	return o
}

func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeNote(t *testing.T, root, title, id string, opts ...noteOpts) {
	t.Helper()
	var o noteOpts
	if len(opts) > 0 {
		o = opts[0]
	}
	o = o.withDefaults()
	tags := o.tags
	if tags == nil {
		tags = []string{"type/" + o.typ, "status/" + o.status, "category/" + o.category, "alpha"}
	}
	quoted := make([]string, len(tags))
	for i, tag := range tags {
		quoted[i] = fmt.Sprintf("%q", tag)
	}
	var links strings.Builder
	for _, l := range o.links {
		fmt.Fprintf(&links, "- [[%s]]\n", l)
	}
	text := fmt.Sprintf("---\nid: %s\ntags: [%s]\n%s---\n%s\n\n## Links\n\n%s\nSource: IDEAS_NOTES.md §1. Direction, lines 1-5\n",
		id, strings.Join(quoted, ", "), o.extra, o.body, links.String())
	writeFile(t, filepath.Join(root, "notes", title+".md"), text)
}

func writeMap(t *testing.T, root, name, category, id string, titles []string) {
	t.Helper()
	var entries strings.Builder
	for _, title := range titles {
		fmt.Fprintf(&entries, "- [[%s]]: summary\n", title)
	}
	text := fmt.Sprintf("---\nid: %s\ntags: [\"type/map\", \"category/%s\"]\n---\nMap of the %s notes.\n\n%s", id, category, category, entries.String())
	writeFile(t, filepath.Join(root, "notes", "Map - "+name+".md"), text)
}

// newFixture mirrors Fixture.setUp of the Python tests.
func newFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "README.md"), "# chartroom\n")
	writeNote(t, root, "First note", "20261021120000")
	writeNote(t, root, "Second note", "20261021120001", noteOpts{category: "state", links: []string{"First note"}})
	writeMap(t, root, "Direction", "direction", "20261021120100", []string{"First note"})
	writeMap(t, root, "State", "state", "20261021120101", []string{"Second note"})
	return root
}

func edit(t *testing.T, root, title, old, repl string) {
	t.Helper()
	path := filepath.Join(root, "notes", title+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), old) {
		t.Fatalf("%q not in %s", old, title)
	}
	writeFile(t, path, strings.Replace(string(data), old, repl, 1))
}

func check(t *testing.T, root string) []Finding {
	t.Helper()
	res, err := Run(root)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return res.Findings
}

func has(fs []Finding, rule, note string) bool {
	for _, f := range fs {
		if f.Rule == rule && f.Note == note {
			return true
		}
	}
	return false
}

func assertFinding(t *testing.T, root, rule, note string) {
	t.Helper()
	if fs := check(t, root); !has(fs, rule, note) {
		t.Errorf("want finding %s on %q, got %v", rule, note, fs)
	}
}

func assertClean(t *testing.T, root string) {
	t.Helper()
	if fs := check(t, root); len(fs) != 0 {
		t.Errorf("want no findings, got %v", fs)
	}
}
