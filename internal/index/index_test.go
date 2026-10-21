// SPDX-License-Identifier: EUPL-1.2

package index

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func collectionOf(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(root, "notes", name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func noteText(id, aliases, tags, body string) string {
	return "---\nid: " + id + "\naliases: " + aliases + "\ntags: " + tags + "\n---\n" + body
}

func load(t *testing.T) *Index {
	t.Helper()
	root := collectionOf(t, map[string]string{
		"Alpha.md": noteText("20260101000000", `["First"]`, `["type/idea", "x"]`, "See [[Beta]], [[first|label]], [[Beta#Head]] and [[Missing]].\n"),
		"Beta.md":  noteText("20260101000001", `[]`, `["x"]`, "Back to [[Alpha]] and [[Beta]].\n\n## Tracking\n\n- #1 [[Hidden]]\n"),
		"Bad.md":   "no frontmatter\n",
	})
	x, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	return x
}

func TestLoadSkipsUnparsableNotes(t *testing.T) {
	x := load(t)
	if len(x.Notes) != 2 || x.Notes[0].Title != "Alpha" || x.Notes[0].Path != "notes/Alpha.md" || x.Notes[0].ID != "20260101000000" {
		t.Errorf("notes = %+v", x.Notes)
	}
	if !reflect.DeepEqual(x.Skipped, []string{"Bad.md"}) {
		t.Errorf("skipped = %v", x.Skipped)
	}
}

func TestFind(t *testing.T) {
	x := load(t)
	for _, arg := range []string{"Alpha", "alpha", "First", "Alpha.md", "notes/Alpha.md", "20260101000000"} {
		if n, ok := x.Find(arg); !ok || n.Title != "Alpha" {
			t.Errorf("Find(%q) = %v, %v", arg, n.Title, ok)
		}
	}
	if _, ok := x.Find("Nope"); ok {
		t.Error("Find(Nope) found a note")
	}
}

func TestLinks(t *testing.T) {
	x := load(t)
	a, _ := x.Find("Alpha")
	var got []string
	for _, l := range x.Links(a) {
		to := "?"
		if l.To != nil {
			to = l.To.Title
		}
		got = append(got, l.Target+"="+to)
	}
	want := []string{"Beta=Beta", "first|label=Alpha", "Beta#Head=Beta", "Missing=?"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("links = %v, want %v", got, want)
	}
	b, _ := x.Find("Beta")
	if n := len(x.Links(b)); n != 2 { // the Tracking section is ignored
		t.Errorf("Beta has %d links, want 2", n)
	}
}

func TestBacklinks(t *testing.T) {
	x := load(t)
	b, _ := x.Find("Beta")
	if got := x.Backlinks(b); len(got) != 1 || got[0].Title != "Alpha" { // not itself
		t.Errorf("backlinks of Beta = %+v", got)
	}
	a, _ := x.Find("Alpha")
	if got := x.Backlinks(a); len(got) != 1 || got[0].Title != "Beta" {
		t.Errorf("backlinks of Alpha = %+v", got)
	}
}

func TestTagCounts(t *testing.T) {
	x := load(t)
	want := []TagCount{{"type/idea", 1}, {"x", 2}}
	if got := x.Tags(); !reflect.DeepEqual(got, want) {
		t.Errorf("tags = %v, want %v", got, want)
	}
}
