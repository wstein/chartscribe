// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestQueryOutputGolden(t *testing.T) {
	alias := fixture("pass", "alias-link")
	tests := []struct {
		golden string
		args   []string
		code   int
	}{
		{"list.stdout", []string{"--root", alias, "list"}, 0},
		{"list.json", []string{"--root", alias, "--json", "list"}, 0},
		{"list-tag.stdout", []string{"--root", alias, "list", "--tag", "category/state"}, 0},
		{"list-tag.json", []string{"--root", alias, "--json", "list", "--tag", "nothing"}, 0},
		{"tags.stdout", []string{"--root", alias, "tags"}, 0},
		{"tags.json", []string{"--root", alias, "--json", "tags"}, 0},
		{"links.stdout", []string{"--root", alias, "links", "Second note"}, 0},
		{"links.json", []string{"--root", alias, "--json", "links", "Second note"}, 0},
		{"links-unresolved.stdout", []string{"--root", fixture("fail", "wikilink-unresolved-links"), "links", "Second note"}, 0},
		{"links-unresolved.json", []string{"--root", fixture("fail", "wikilink-unresolved-links"), "--json", "links", "Second note"}, 0},
		{"backlinks.stdout", []string{"--root", alias, "backlinks", "Primus"}, 0},
		{"backlinks.json", []string{"--root", alias, "--json", "backlinks", "First note"}, 0},
		{"links-label-heading.stdout", []string{"--root", fixture("pass", "link-label-heading"), "links", "Second note"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.golden, func(t *testing.T) {
			code, out, errOut := run(tt.args...)
			if code != tt.code || errOut != "" {
				t.Fatalf("code %d, stderr %q", code, errOut)
			}
			golden(t, tt.golden, out)
		})
	}
}

func TestQueryErrors(t *testing.T) {
	alias := fixture("pass", "alias-link")
	tests := []struct {
		name string
		args []string
		code int
	}{
		{"unknown note", []string{"--root", alias, "links", "Nope"}, 2},
		{"unknown note backlinks", []string{"--root", alias, "backlinks", "Nope"}, 2},
		{"missing argument", []string{"--root", alias, "links"}, 2},
		{"no collection", []string{"--root", t.TempDir(), "list"}, 3},
		{"extra argument", []string{"--root", alias, "tags", "x"}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out, errOut := run(tt.args...)
			if code != tt.code || out != "" || !strings.HasPrefix(errOut, "Error:") {
				t.Errorf("code %d, stdout %q, stderr %q", code, out, errOut)
			}
		})
	}
	code, out, _ := run("--root", alias, "--json", "links", "Nope")
	if code != 2 || !strings.Contains(out, `"ok": false`) || !strings.Contains(out, `"command": "links"`) {
		t.Errorf("json error: code %d, out %s", code, out)
	}
}

func TestQuerySkipsUnparsableNotesWithAWarning(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "notes"), 0o755)
	os.WriteFile(filepath.Join(root, "notes", "Bad.md"), []byte("no frontmatter\n"), 0o644)
	code, out, errOut := run("--root", root, "list")
	if code != 0 || out != "" || !strings.Contains(errOut, "Bad.md") {
		t.Errorf("code %d, stdout %q, stderr %q", code, out, errOut)
	}
}

func TestQueryDoesNotWrite(t *testing.T) {
	root := fixture("pass", "alias-link")
	before := tree(t, root)
	for _, args := range [][]string{{"list"}, {"tags"}, {"links", "Second note"}, {"backlinks", "First note"}} {
		run(append([]string{"--root", root}, args...)...)
	}
	if after := tree(t, root); after != before {
		t.Errorf("tree changed:\n%s\n---\n%s", before, after)
	}
}
