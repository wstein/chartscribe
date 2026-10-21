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
