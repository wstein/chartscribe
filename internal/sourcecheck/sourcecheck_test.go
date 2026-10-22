// SPDX-License-Identifier: EUPL-1.2

package sourcecheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wstein/chartscribe/internal/note"
)

func tree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(root, "docs", "dir"), 0o755))
	must(os.WriteFile(filepath.Join(root, "docs", "a.md"), []byte("one\ntwo\nthree\n"), 0o644))
	must(os.WriteFile(filepath.Join(root, "docs", "nonl.md"), []byte("one\ntwo"), 0o644))
	must(os.WriteFile(filepath.Join(root, "docs", "empty.md"), nil, 0o644))
	return root
}

func file(path string, from, to int) note.Source {
	return note.Source{Kind: note.KindFile, Path: path, From: from, To: to}
}

func TestCheck(t *testing.T) {
	root := tree(t)
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkErr := os.Symlink(outside, filepath.Join(root, "docs", "link.txt"))
	tests := []struct {
		name string
		src  note.Source
		rule string
	}{
		{"ok", file("docs/a.md", 1, 3), ""},
		{"ok single line", file("docs/a.md", 2, 2), ""},
		{"last line without newline", file("docs/nonl.md", 1, 2), ""},
		{"past the end", file("docs/a.md", 1, 4), "source-range-invalid"},
		{"past the end without newline", file("docs/nonl.md", 1, 3), "source-range-invalid"},
		{"from zero", file("docs/a.md", 0, 2), "source-range-invalid"},
		{"reversed", file("docs/a.md", 3, 2), "source-range-invalid"},
		{"empty file", file("docs/empty.md", 1, 1), "source-range-invalid"},
		{"missing", file("docs/none.md", 1, 1), "source-file-missing"},
		{"directory", file("docs/dir", 1, 1), "source-not-file"},
		{"dotdot", file("../x.md", 1, 1), "source-outside-root"},
		{"dotdot inside", file("docs/../docs/a.md", 1, 3), ""},
		{"dotdot out and back", file("docs/../../x", 1, 1), "source-outside-root"},
		{"absolute", file(outside, 1, 1), "source-outside-root"},
		{"measurement ok", note.Source{Kind: note.KindMeasurement, Path: "docs/a.md"}, ""},
		{"measurement missing", note.Source{Kind: note.KindMeasurement, Path: "docs/none.csv"}, "source-file-missing"},
		{"measurement directory", note.Source{Kind: note.KindMeasurement, Path: "docs/dir"}, "source-not-file"},
	}
	if linkErr == nil {
		tests = append(tests, struct {
			name string
			src  note.Source
			rule string
		}{"symlink escape", file("docs/link.txt", 1, 1), "source-outside-root"})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Check(root, tt.src)
			switch {
			case tt.rule == "" && p != nil:
				t.Errorf("unexpected %+v", p)
			case tt.rule != "" && (p == nil || p.Rule != tt.rule):
				t.Errorf("got %+v, want %s", p, tt.rule)
			}
			if p != nil && strings.Contains(p.Message, root) {
				t.Errorf("message leaks the absolute root: %s", p.Message)
			}
		})
	}
}

func TestCheckSymlinkInsideRootIsFine(t *testing.T) {
	root := tree(t)
	if err := os.Symlink(filepath.Join(root, "docs", "a.md"), filepath.Join(root, "docs", "alias.md")); err != nil {
		t.Skip("symlinks unavailable")
	}
	if p := Check(root, file("docs/alias.md", 1, 3)); p != nil {
		t.Errorf("got %+v", p)
	}
}

func TestResolveReturnsRealPath(t *testing.T) {
	root := tree(t)
	abs, p := Resolve(root, "docs/a.md")
	if p != nil || !strings.HasSuffix(abs, filepath.Join("docs", "a.md")) {
		t.Errorf("abs=%s p=%v", abs, p)
	}
}
