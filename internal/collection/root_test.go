// SPDX-License-Identifier: EUPL-1.2

package collection

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func mkdir(t *testing.T, parts ...string) string {
	t.Helper()
	p := filepath.Join(parts...)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFindRootFromSubdirectory(t *testing.T) {
	base := t.TempDir()
	mkdir(t, base, "coll", "notes")
	sub := mkdir(t, base, "coll", "docs", "deep")
	got, err := FindRoot("", sub)
	if err != nil || got != filepath.Join(base, "coll") {
		t.Errorf("FindRoot = %q, %v", got, err)
	}
}

func TestFindRootNearestWins(t *testing.T) {
	base := t.TempDir()
	mkdir(t, base, "outer", "notes")
	inner := mkdir(t, base, "outer", "inner")
	mkdir(t, inner, "notes")
	got, err := FindRoot("", inner)
	if err != nil || got != inner {
		t.Errorf("FindRoot = %q, %v", got, err)
	}
}

func TestFindRootExplicit(t *testing.T) {
	base := t.TempDir()
	mkdir(t, base, "coll", "notes")
	elsewhere := mkdir(t, base, "elsewhere")
	got, err := FindRoot(filepath.Join(base, "coll"), elsewhere)
	if err != nil || got != filepath.Join(base, "coll") {
		t.Errorf("FindRoot = %q, %v", got, err)
	}
}

func TestFindRootNone(t *testing.T) {
	base := t.TempDir()
	mkdir(t, base, "x", "y")
	for _, explicit := range []string{"", filepath.Join(base, "missing"), filepath.Join(base, "x")} {
		if _, err := FindRoot(explicit, filepath.Join(base, "x", "y")); !errors.Is(err, ErrNoCollection) {
			t.Errorf("FindRoot(%q) err = %v, want ErrNoCollection", explicit, err)
		}
	}
}

func TestReadNotes(t *testing.T) {
	root := t.TempDir()
	notes := mkdir(t, root, "notes")
	for name, text := range map[string]string{"b.md": "B", "a.md": "A", "skip.txt": "x", "Ü.md": "U"} {
		if err := os.WriteFile(filepath.Join(notes, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mkdir(t, notes, "dir.md") // a directory is not a note
	mkdir(t, notes, "sub")
	if err := os.WriteFile(filepath.Join(notes, "sub", "n.md"), []byte("N"), 0o644); err != nil {
		t.Fatal(err)
	}
	files, err := ReadNotes(root)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range files {
		got = append(got, f.Name+"="+f.Text)
	}
	want := []string{"a.md=A", "b.md=B", "Ü.md=U"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got %v want %v", got, want)
		}
	}
	if files[0].Title() != "a" {
		t.Errorf("Title = %q", files[0].Title())
	}
}

func TestReadNotesNormalisesLineEndings(t *testing.T) {
	root := t.TempDir()
	notes := mkdir(t, root, "notes")
	if err := os.WriteFile(filepath.Join(notes, "n.md"), []byte("a\r\nb\rc\nd"), 0o644); err != nil {
		t.Fatal(err)
	}
	files, err := ReadNotes(root)
	if err != nil || len(files) != 1 || files[0].Text != "a\nb\nc\nd" {
		t.Errorf("got %#v, %v", files, err)
	}
}
