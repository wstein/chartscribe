// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixedClock(t *testing.T, s string) {
	t.Helper()
	at, err := time.ParseInLocation("20060102150405", s, time.Local)
	if err != nil {
		t.Fatal(err)
	}
	old := now
	now = func() time.Time { return at }
	t.Cleanup(func() { now = old })
}

func emptyCollection(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestNewWritesTheNote(t *testing.T) {
	fixedClock(t, "20261021093015")
	root := emptyCollection(t)
	code, out, errOut := run("--root", root, "new", "My idea")
	if code != 0 || errOut != "" {
		t.Fatalf("code %d, stderr %q", code, errOut)
	}
	path := filepath.Join(root, "notes", "My idea.md")
	if out != path+"\n" {
		t.Errorf("stdout = %q, want %q", out, path+"\n")
	}
	data, _ := os.ReadFile(path)
	want := "---\nid: 20261021093015\naliases: []\ntags: []\n---\n\n## Links\n\n" + "<!-- Source forms: <path>, lines A-B | decision: <text>, YYYY-MM-DD | url: <https://...> | measurement: <path>[, <key>] -->\nSource:\n"
	if string(data) != want {
		t.Errorf("note = %q, want %q", data, want)
	}
}

func TestNewTagsAndJSON(t *testing.T) {
	fixedClock(t, "20261021093015")
	root := emptyCollection(t)
	code, out, _ := run("--root", root, "--json", "new", "Tagged", "--tag", "type/idea", "--tag", "a,b")
	if code != 0 {
		t.Fatalf("code %d, out %s", code, out)
	}
	golden(t, "new.json", strings.ReplaceAll(out, filepath.ToSlash(root), "<root>"))
	data, _ := os.ReadFile(filepath.Join(root, "notes", "Tagged.md"))
	if !strings.Contains(string(data), `tags: ["type/idea", "a", "b"]`) {
		t.Errorf("note = %q", data)
	}
}

func TestNewIDCollisionGetsASuffix(t *testing.T) {
	fixedClock(t, "20261021093015")
	root := emptyCollection(t)
	for i, want := range []string{"20261021093015", "20261021093015-001", "20261021093015-002"} {
		title := []string{"One", "Two", "Three"}[i]
		if code, _, errOut := run("--root", root, "new", title); code != 0 {
			t.Fatalf("%s: code %d %s", title, code, errOut)
		}
		data, _ := os.ReadFile(filepath.Join(root, "notes", title+".md"))
		if !strings.Contains(string(data), "id: "+want+"\n") {
			t.Errorf("%s: %q lacks id %s", title, data, want)
		}
	}
}

func TestNewRefuses(t *testing.T) {
	fixedClock(t, "20261021093015")
	root := emptyCollection(t)
	run("--root", root, "new", "Taken")
	tests := []struct{ name, title string }{
		{"existing file", "Taken"},
		{"existing file in other case", "TAKEN"},
		{"slash", "a/b"},
		{"colon", "a: b"},
		{"star", "a*"},
		{"empty", " "},
		{"control", "a\nb"},
		{"hidden", ".hidden"},
		{"hash", "a#b"},
		{"bracket", "a[b"},
		{"caret", "a^b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out, errOut := run("--root", root, "new", tt.title)
			if code != 2 || out != "" || !strings.HasPrefix(errOut, "Error:") {
				t.Errorf("code %d, stdout %q, stderr %q", code, out, errOut)
			}
		})
	}
	entries, _ := os.ReadDir(filepath.Join(root, "notes"))
	if len(entries) != 1 {
		t.Errorf("notes/ has %d files, want 1", len(entries))
	}
	code, out, _ := run("--root", root, "--json", "new", "a|b")
	if code != 2 || !strings.Contains(out, `"error": "usage"`) {
		t.Errorf("json: code %d, out %s", code, out)
	}
}

func TestNewCreatesNotesFolderAndPassesValidate(t *testing.T) {
	fixedClock(t, "20261021093015")
	root := t.TempDir() // as after `init --apply`: no notes/ yet
	if code, _, errOut := run("--root", root, "new", "Fresh"); code != 0 {
		t.Fatalf("code %d %s", code, errOut)
	}
	if _, err := os.Stat(filepath.Join(root, "notes", "Fresh.md")); err != nil {
		t.Fatal(err)
	}
	// Filled in as the user would: tags, Source line and a map that lists it.
	path := filepath.Join(root, "notes", "Fresh.md")
	data, _ := os.ReadFile(path)
	filled := strings.Replace(string(data), "tags: []", `tags: ["type/idea", "status/open", "category/state"]`, 1)
	filled = strings.Replace(filled, "Source:\n", "Source: manual, lines 1-2\n", 1)
	os.WriteFile(path, []byte(filled), 0o644)
	os.WriteFile(filepath.Join(root, "notes", "Map.md"), []byte("---\nid: 20261021093016\naliases: []\ntags: [\"type/map\", \"category/state\"]\n---\n- [[Fresh]]\n"), 0o644)
	os.WriteFile(filepath.Join(root, "README.md"), []byte("# x\n"), 0o644)
	code, out, errOut := run("--root", root, "validate")
	if code != 0 {
		t.Errorf("validate: code %d, out %s, err %s", code, out, errOut)
	}
}

func TestNewMessagesAndUnparsableNotes(t *testing.T) {
	fixedClock(t, "20261021093015")
	root := emptyCollection(t)
	notes := filepath.Join(root, "notes")
	os.WriteFile(filepath.Join(notes, "Broken.md"), []byte("---\nid: 20261021093015\nbad line\n---\n"), 0o644)
	run("--root", root, "new", "Plain")
	tests := []struct{ title, want string }{
		{"a#b", "forbidden character"},
		{"BROKEN", "notes/Broken.md already exists"}, // a note that does not parse still counts, by file name
		{"PLAIN", "already exists"},
	}
	for _, tt := range tests {
		code, _, errOut := run("--root", root, "new", tt.title)
		if code != 2 || !strings.Contains(errOut, tt.want) {
			t.Errorf("%q: code %d, stderr %q, want %q", tt.title, code, errOut, tt.want)
		}
	}
	// The id of the note that does not parse is taken too.
	run("--root", root, "new", "Next")
	data, _ := os.ReadFile(filepath.Join(notes, "Next.md"))
	if !strings.Contains(string(data), "id: 20261021093015-002\n") {
		t.Errorf("note = %q", data)
	}
	// A title that equals an id is a fine title unless the name is taken.
	if code, _, errOut := run("--root", root, "new", "20261021093015"); code != 0 {
		t.Errorf("id-like title: code %d, stderr %q", code, errOut)
	}
}
