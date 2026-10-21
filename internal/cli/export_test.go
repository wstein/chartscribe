// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var exportFixture = filepath.Join("..", "..", "testdata", "export", "basic")

func TestExportGolden(t *testing.T) {
	root := exportFixture
	code, out, errOut := run("--root", root, "export", "Map - Harbor")
	if code != 0 {
		t.Fatalf("code %d, stderr %q", code, errOut)
	}
	golden(t, "export.md", out)
	if errOut != "unresolved [[Missing note]] in Anchor\n" {
		t.Errorf("stderr %q", errOut)
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "doc.md")
	code, out, _ = run("--root", root, "--json", "export", "Map - Harbor", "--out", file, "--depth", "2")
	if code != 0 {
		t.Fatalf("json: code %d", code)
	}
	golden(t, "export.json", out)
	got, err := os.ReadFile(file)
	if err != nil || !strings.Contains(string(got), "\n### Lighthouse\n") {
		t.Errorf("file: %v\n%s", err, got)
	}
}

func TestExportOutFile(t *testing.T) {
	root := exportFixture
	dir := t.TempDir()
	file := filepath.Join(dir, "doc.md")
	if code, out, errOut := run("--root", root, "export", "Map - Harbor", "--out", file); code != 0 || out != "" {
		t.Fatalf("code %d out %q err %q", code, out, errOut)
	}
	first, _ := os.ReadFile(file)
	// A rerun is refused and leaves the file alone.
	code, _, errOut := run("--root", root, "export", "Map - Harbor", "--out", file)
	if code != 2 || !strings.Contains(errOut, "--force") {
		t.Fatalf("overwrite: code %d err %q", code, errOut)
	}
	if code, _, _ := run("--root", root, "export", "Map - Harbor", "--out", file, "--force"); code != 0 {
		t.Fatalf("force: code %d", code)
	}
	second, _ := os.ReadFile(file)
	if string(first) != string(second) {
		t.Error("rerun is not byte-identical")
	}
	// Nothing but the one file appeared next to it.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("%d files written", len(entries))
	}
	// The stdout form is the file form.
	_, out, _ := run("--root", root, "export", "Map - Harbor", "--out", "-")
	if out != string(first) {
		t.Error("stdout differs from the file")
	}
}

func TestExportErrors(t *testing.T) {
	root := exportFixture
	dir := t.TempDir()
	tests := []struct {
		name string
		args []string
		code int
	}{
		{"unknown note", []string{"--root", root, "export", "Nope"}, 2},
		{"no argument", []string{"--root", root, "export"}, 2},
		{"format", []string{"--root", root, "export", "Anchor", "--format", "html"}, 2},
		{"negative depth", []string{"--root", root, "export", "Anchor", "--depth", "-1"}, 2},
		{"json to stdout", []string{"--root", root, "--json", "export", "Anchor"}, 2},
		{"missing directory", []string{"--root", root, "export", "Anchor", "--out", filepath.Join(dir, "no", "x.md")}, 3},
		{"out is a directory", []string{"--root", root, "export", "Anchor", "--out", dir, "--force"}, 3},
		{"out is a directory, no force", []string{"--root", root, "export", "Anchor", "--out", dir}, 3},
		{"no collection", []string{"--root", t.TempDir(), "export", "Anchor"}, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out, errOut := run(tt.args...)
			if code != tt.code {
				t.Fatalf("code %d, stdout %q, stderr %q", code, out, errOut)
			}
			if strings.Contains(strings.Join(tt.args, " "), "--json") {
				if !strings.Contains(out, `"ok": false`) {
					t.Errorf("no error document: %q", out)
				}
			} else if out != "" || !strings.HasPrefix(errOut, "Error:") {
				t.Errorf("stdout %q, stderr %q", out, errOut)
			}
		})
	}
}

// copyCollection copies the export fixture to a temporary directory so that
// --out can point inside it.
func copyCollection(t *testing.T) string {
	t.Helper()
	dst := t.TempDir()
	err := filepath.WalkDir(exportFixture, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(exportFixture, p)
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

func TestExportRefusesTheCollection(t *testing.T) {
	root := copyCollection(t)
	note := filepath.Join(root, "notes", "Anchor.md")
	before, _ := os.ReadFile(note)
	outside := t.TempDir()
	victim := filepath.Join(outside, "victim.md")
	if err := os.WriteFile(victim, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link.md")
	if err := os.Symlink(victim, link); err != nil {
		t.Skip("no symlinks:", err)
	}
	dirLink := filepath.Join(t.TempDir(), "notes-link")
	if err := os.Symlink(filepath.Join(root, "notes"), dirLink); err != nil {
		t.Skip("no symlinks:", err)
	}
	tests := []struct {
		name string
		out  string
	}{
		{"existing note, force", note},
		{"new file in notes", filepath.Join(root, "notes", "New.md")},
		{"unclean path into notes", filepath.Join(root, "notes", "..", "notes", "New.md")},
		{"symlink to a file outside, force", link},
		{"symlinked parent directory", filepath.Join(dirLink, "New.md")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out, errOut := run("--root", root, "export", "Map - Harbor", "--out", tt.out, "--force")
			if code != 2 || out != "" || !strings.HasPrefix(errOut, "Error:") {
				t.Fatalf("code %d, stdout %q, stderr %q", code, out, errOut)
			}
		})
	}
	if after, _ := os.ReadFile(note); string(after) != string(before) {
		t.Error("a note was changed")
	}
	if got, _ := os.ReadFile(victim); string(got) != "keep" {
		t.Error("the symlink target was written")
	}
	if _, err := os.Stat(filepath.Join(root, "notes", "New.md")); err == nil {
		t.Error("New.md was created")
	}
	entries, _ := os.ReadDir(filepath.Join(root, "notes"))
	if len(entries) != 5 {
		t.Errorf("notes/ has %d entries", len(entries))
	}
}

func TestExportTitleOverride(t *testing.T) {
	code, out, _ := run("--root", exportFixture, "export", "Map - Harbor", "--title", "Harbor Handbook", "--depth", "0")
	if code != 0 || !strings.HasPrefix(out, "# Harbor Handbook\n") {
		t.Fatalf("code %d, out %q", code, out)
	}
}

func TestTouchesNotesByIdentity(t *testing.T) {
	root := t.TempDir()
	notes := filepath.Join(root, "notes")
	if err := os.Mkdir(notes, 0o755); err != nil {
		t.Fatal(err)
	}
	note := filepath.Join(notes, "A.md")
	if err := os.WriteFile(note, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()
	alias := filepath.Join(other, "alias")
	if err := os.Symlink(notes, alias); err != nil {
		t.Skip("no symlinks:", err)
	}
	hard := filepath.Join(other, "hard.md")
	if err := os.Link(note, hard); err != nil {
		t.Skip("no hard links:", err)
	}
	tests := []struct {
		name        string
		dir, target string
		want        bool
	}{
		{"symlink alias of notes", alias, filepath.Join(alias, "New.md"), true},
		{"below an alias", filepath.Join(alias, "sub"), filepath.Join(alias, "sub", "x.md"), true},
		{"hard link of a note", other, hard, true},
		{"unrelated directory", other, filepath.Join(other, "doc.md"), false},
		{"collection root", root, filepath.Join(root, "doc.md"), false},
	}
	for _, tt := range tests {
		got, err := touchesNotes(notes, tt.dir, tt.target)
		if err != nil || got != tt.want {
			t.Errorf("%s: got %v, %v; want %v", tt.name, got, err, tt.want)
		}
	}
}

func TestInsideFold(t *testing.T) {
	sep := string(filepath.Separator)
	base := sep + filepath.Join("c", "Col", "notes")
	for dir, want := range map[string]bool{
		base:                                     true,
		sep + filepath.Join("c", "col", "NOTES"): true,
		sep + filepath.Join("C", "Col", "Notes", "sub"): true,
		sep + filepath.Join("c", "Col", "notes2"):       false,
		sep + filepath.Join("c", "Col"):                 false,
	} {
		if got := insideFold(base, dir); got != want {
			t.Errorf("insideFold(%q) = %v, want %v", dir, got, want)
		}
	}
}

// On a case-insensitive file system NOTES/ names the notes directory; the guard
// must refuse it. Skipped where the temp directory is case-sensitive.
func TestExportRefusesCaseAliasOfNotes(t *testing.T) {
	root := copyCollection(t)
	if _, err := os.Stat(filepath.Join(root, "NOTES")); err != nil {
		t.Skip("case-sensitive file system")
	}
	note := filepath.Join(root, "notes", "Anchor.md")
	before, _ := os.ReadFile(note)
	code, _, errOut := run("--root", root, "export", "Map - Harbor", "--out", filepath.Join(root, "NOTES", "ANCHOR.md"), "--force")
	if code != 2 {
		t.Fatalf("code %d, stderr %q", code, errOut)
	}
	if after, _ := os.ReadFile(note); string(after) != string(before) {
		t.Error("a note was changed")
	}
}

func TestExportWarnsAboutAnUnclosedFence(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "export", "unclosed")
	code, out, errOut := run("--root", root, "export", "Map - U")
	if code != 0 || !strings.Contains(out, "\n## Sources\n") {
		t.Fatalf("code %d, out %q", code, out)
	}
	if !strings.Contains(errOut, "unclosed code fence in A\n") || !strings.Contains(errOut, "unclosed code fence in D\n") {
		t.Errorf("stderr %q", errOut)
	}
	file := filepath.Join(t.TempDir(), "u.md")
	code, jout, _ := run("--root", root, "--json", "export", "Map - U", "--out", file)
	if code != 0 || !strings.Contains(jout, `"warnings": [`) || !strings.Contains(jout, "unclosed code fence in C") {
		t.Errorf("json %q", jout)
	}
	// A clean export carries an empty array, not null.
	code, jout, _ = run("--root", exportFixture, "--json", "export", "Anchor", "--out", filepath.Join(t.TempDir(), "c.md"))
	if code != 0 || !strings.Contains(jout, `"warnings": []`) {
		t.Errorf("json %q", jout)
	}
}
