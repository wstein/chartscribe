// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden files")

const fixtures = "../../testdata/validate"

func fixture(kind, name string) string { return filepath.Join(fixtures, kind, name) }

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "golden", name)
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("%s differs:\ngot:\n%s\nwant:\n%s", name, got, want)
	}
}

func TestValidateHumanOutput(t *testing.T) {
	tests := []struct {
		name, dir string
		code      int
	}{
		{"human-clean", fixture("pass", "base"), 0},
		{"human-findings", fixture("fail", "wikilink-unresolved-links"), 1},
		{"human-collection-level", fixture("fail", "id-duplicate"), 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out, errOut := run("--root", tt.dir, "validate")
			if code != tt.code {
				t.Errorf("exit = %d, want %d", code, tt.code)
			}
			golden(t, tt.name+".stdout", out)
			golden(t, tt.name+".stderr", errOut)
		})
	}
}

func TestValidateJSONOutput(t *testing.T) {
	tests := []struct {
		name, dir string
		code      int
	}{
		{"json-clean", fixture("pass", "base"), 0},
		{"json-findings", fixture("fail", "map-category"), 1},
		{"json-collection-level", fixture("fail", "id-duplicate"), 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out, errOut := run("--json", "--root", tt.dir, "validate")
			if code != tt.code {
				t.Errorf("exit = %d, want %d", code, tt.code)
			}
			if errOut != "" {
				t.Errorf("stderr must be empty under --json, got %q", errOut)
			}
			golden(t, tt.name+".json", out)
			var doc struct {
				Version  int               `json:"version"`
				Command  string            `json:"command"`
				OK       bool              `json:"ok"`
				Notes    int               `json:"notes"`
				Findings []json.RawMessage `json:"findings"`
			}
			if err := json.Unmarshal([]byte(out), &doc); err != nil {
				t.Fatalf("stdout is not one JSON document: %v", err)
			}
			if doc.Version != 1 || doc.Command != "validate" || doc.OK != (len(doc.Findings) == 0) || doc.Findings == nil {
				t.Errorf("document = %+v", doc)
			}
		})
	}
}

func TestValidateRuleFilter(t *testing.T) {
	dir := fixture("fail", "tag-invalid-category") // tag-invalid, map-category, category-no-map
	code, out, errOut := run("--root", dir, "validate", "--rule", "tag-invalid")
	if code != 1 || !strings.Contains(errOut, "invalid tag") || strings.Contains(errOut, "differs from its map") {
		t.Errorf("filtered: %d %q", code, errOut)
	}
	if out != "validate: 4 notes, 1 findings\n" {
		t.Errorf("summary = %q", out)
	}
	// The other rules still ran: filtering a rule that has no finding is clean.
	if code, _, errOut := run("--root", dir, "validate", "--rule", "title-char"); code != 0 || errOut != "" {
		t.Errorf("clean filter: %d %q", code, errOut)
	}
	// Repeated and comma-separated rule names.
	if code, _, _ := run("--root", dir, "validate", "--rule", "title-char", "--rule", "tag-invalid"); code != 1 {
		t.Errorf("repeated --rule: %d", code)
	}
	if code, _, _ := run("--root", dir, "validate", "--rule", "title-char,tag-invalid"); code != 1 {
		t.Errorf("comma --rule: %d", code)
	}
}

func TestLintAlias(t *testing.T) {
	dir := fixture("fail", "map-unlisted")
	c1, o1, e1 := run("--root", dir, "validate")
	c2, o2, e2 := run("--root", dir, "lint")
	if c1 != 1 || c1 != c2 || o1 != o2 || e1 != e2 {
		t.Errorf("lint differs from validate: %d/%d %q/%q %q/%q", c1, c2, o1, o2, e1, e2)
	}
}

func TestValidateExitCodes(t *testing.T) {
	empty := t.TempDir()
	badConfig := t.TempDir()
	if err := os.MkdirAll(filepath.Join(badConfig, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(badConfig, ".cscribe.toml"), []byte("[nope]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		args     []string
		code     int
		errToken string // in stderr (human mode)
	}{
		{"clean", []string{"--root", fixture("pass", "base"), "validate"}, 0, ""},
		{"findings", []string{"--root", fixture("fail", "id-missing"), "validate"}, 1, "missing id"},
		{"unknown rule", []string{"--root", fixture("pass", "base"), "validate", "--rule", "nope"}, 2, "unknown rule"},
		{"unknown flag", []string{"validate", "--nope"}, 2, "unknown flag"},
		{"extra argument", []string{"--root", fixture("pass", "base"), "validate", "x"}, 2, "unknown command"},
		{"missing root", []string{"--root", filepath.Join(empty, "gone"), "validate"}, 3, "no collection"},
		{"root without notes", []string{"--root", empty, "validate"}, 3, "no collection"},
		{"bad config", []string{"--root", badConfig, "validate"}, 3, ".cscribe.toml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out, errOut := run(tt.args...)
			if code != tt.code {
				t.Errorf("exit = %d, want %d (stdout %q stderr %q)", code, tt.code, out, errOut)
			}
			if !strings.Contains(errOut, tt.errToken) {
				t.Errorf("stderr = %q, want %q", errOut, tt.errToken)
			}
			if tt.code >= 2 && out != "" {
				t.Errorf("stdout = %q on an error", out)
			}
		})
	}
}

func TestValidateJSONErrors(t *testing.T) {
	empty := t.TempDir()
	badConfig := t.TempDir()
	_ = os.MkdirAll(filepath.Join(badConfig, "notes"), 0o755)
	_ = os.WriteFile(filepath.Join(badConfig, ".cscribe.toml"), []byte("[nope]\n"), 0o644)
	tests := []struct {
		name string
		args []string
		code int
		kind string
	}{
		{"unknown rule", []string{"--json", "--root", fixture("pass", "base"), "validate", "--rule", "nope"}, 2, "usage"},
		{"unknown flag", []string{"--json", "validate", "--nope"}, 2, "usage"},
		{"no collection", []string{"--json", "--root", empty, "validate"}, 3, "no-collection"},
		{"bad config", []string{"--json", "--root", badConfig, "validate"}, 3, "io"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out, errOut := run(tt.args...)
			if code != tt.code || errOut != "" {
				t.Errorf("exit = %d stderr = %q", code, errOut)
			}
			var doc map[string]any
			if err := json.Unmarshal([]byte(out), &doc); err != nil {
				t.Fatalf("stdout %q: %v", out, err)
			}
			if doc["version"] != float64(1) || doc["command"] != "validate" || doc["ok"] != false || doc["error"] != tt.kind || doc["message"] == "" {
				t.Errorf("document = %v", doc)
			}
		})
	}
}

func TestValidateDiscoversTheRootFromASubdirectory(t *testing.T) {
	abs, err := filepath.Abs(fixture("pass", "base"))
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(filepath.Join(abs, "notes"))
	if code, out, errOut := run("validate"); code != 0 || out != "validate: 4 notes, 0 findings\n" {
		t.Errorf("exit = %d stdout = %q stderr = %q", code, out, errOut)
	}
	t.Chdir(t.TempDir())
	if code, _, errOut := run("validate"); code != 3 || !strings.Contains(errOut, "no collection") {
		t.Errorf("no collection: %d %q", code, errOut)
	}
}

func TestValidateHelpIsPlain(t *testing.T) {
	_, out, _ := run("validate", "--help")
	for _, want := range []string{"--rule", "lint"} {
		if !strings.Contains(out, want) {
			t.Errorf("validate help lacks %q:\n%s", want, out)
		}
	}
}
