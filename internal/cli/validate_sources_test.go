// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// sourcesCollection writes a collection whose one note cites a file that does not exist.
func sourcesCollection(t *testing.T, config string) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"notes/Map - Direction.md": "---\nid: 20261021120100\ntags: [\"type/map\", \"category/direction\"]\n---\nMap.\n\n- [[First note]]\n",
		"notes/First note.md": "---\nid: 20261021120000\ntags: [\"type/idea\", \"status/open\", \"category/direction\"]\n---\nBody.\n\n" +
			"Source: docs/gone.md, lines 1-2\nSource: decision: keep it, 2026-10-22\n",
		"docs/real.md": "one\ntwo\n",
	}
	if config != "" {
		files[".cscribe.toml"] = config
	}
	for name, text := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestValidateWithoutFlagsIgnoresMissingSources(t *testing.T) {
	root := sourcesCollection(t, "source_root = \".\"\n")
	if code, out, errOut := run("--root", root, "validate"); code != 0 || errOut != "" || !strings.Contains(out, "0 findings") {
		t.Errorf("code=%d out=%q err=%q", code, out, errOut)
	}
}

func TestCheckSourcesFlag(t *testing.T) {
	root := sourcesCollection(t, "source_root = \".\"\n")
	code, out, errOut := run("--root", root, "validate", "--check-sources")
	if code != 1 || !strings.Contains(out, "1 findings") || !strings.Contains(errOut, `First note.md: Source file "docs/gone.md" not found`) {
		t.Errorf("code=%d out=%q err=%q", code, out, errOut)
	}
	code, out, _ = run("--json", "--root", root, "validate", "--check-sources")
	var doc struct {
		OK       bool `json:"ok"`
		Findings []struct{ Rule string }
		Warnings json.RawMessage
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil || code != 1 || doc.OK || len(doc.Findings) != 1 ||
		doc.Findings[0].Rule != "source-file-missing" || doc.Warnings != nil {
		t.Errorf("code=%d doc=%+v err=%v\n%s", code, doc, err, out)
	}
}

func TestCheckSourcesFromConfig(t *testing.T) {
	root := sourcesCollection(t, "source_root = \".\"\ncheck_sources = true\n")
	if code, _, errOut := run("--root", root, "validate"); code != 1 || !strings.Contains(errOut, "not found") {
		t.Errorf("code=%d err=%q", code, errOut)
	}
	// A clean source passes.
	if err := os.WriteFile(filepath.Join(root, "notes", "First note.md"), []byte("---\nid: 20261021120000\ntags: [\"type/idea\", \"status/open\", \"category/direction\"]\n---\nBody.\n\nSource: docs/real.md, lines 1-2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, errOut := run("--root", root, "validate"); code != 0 {
		t.Errorf("code=%d err=%q", code, errOut)
	}
}

func TestDefaultSourceRootIsTheGitToplevel(t *testing.T) {
	top := t.TempDir()
	if err := os.Mkdir(filepath.Join(top, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(top, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(top, "docs", "real.md"), []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := sourcesCollection(t, "")
	// move the collection into the fake repository
	coll := filepath.Join(top, "chartroom")
	if err := os.Rename(root, coll); err != nil {
		t.Skip("cannot move across file systems")
	}
	_ = os.Remove(filepath.Join(coll, "docs", "real.md"))
	if code, _, errOut := run("--root", coll, "validate", "--check-sources"); code != 1 || strings.Contains(errOut, "real.md") {
		t.Errorf("code=%d err=%q", code, errOut)
	}
}

func TestSourceFlagUsageErrors(t *testing.T) {
	root := sourcesCollection(t, "source_root = \".\"\n")
	for _, args := range [][]string{
		{"validate", "--git"},
		{"validate", "--fix", "--check-sources"},
		{"validate", "--fix", "--git"},
	} {
		code, _, errOut := run(append([]string{"--root", root}, args...)...)
		if code != 2 || !strings.Contains(errOut, "--") {
			t.Errorf("%v: code=%d err=%q", args, code, errOut)
		}
	}
}

func TestRuleFilterAcceptsSourceStale(t *testing.T) {
	root := sourcesCollection(t, "source_root = \".\"\n")
	if code, _, errOut := run("--root", root, "validate", "--rule", "source-stale"); code != 0 {
		t.Errorf("code=%d err=%q", code, errOut)
	}
}

func TestGitStalenessWarnsWithoutFailing(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := sourcesCollection(t, "source_root = \".\"\n")
	g := func(date string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.org", "GIT_COMMITTER_NAME=t",
			"GIT_COMMITTER_EMAIL=t@example.org", "GIT_AUTHOR_DATE="+date, "GIT_COMMITTER_DATE="+date,
			"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	note := filepath.Join(root, "notes", "First note.md")
	if err := os.WriteFile(note, []byte("---\nid: 20261021120000\ntags: [\"type/idea\", \"status/open\", \"category/direction\"]\n---\nBody.\n\nSource: docs/real.md, lines 1-2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	g("2026-01-01T00:00:00Z", "init", "-q")
	g("2026-01-01T00:00:00Z", "add", "notes", ".cscribe.toml")
	g("2026-01-01T00:00:00Z", "commit", "-q", "-m", "n")
	g("2026-03-01T00:00:00Z", "add", "docs")
	g("2026-03-01T00:00:00Z", "commit", "-q", "-m", "s")

	code, out, errOut := run("--root", root, "validate", "--check-sources", "--git")
	if code != 0 || !strings.Contains(out, "0 findings, 1 warnings") ||
		!strings.Contains(errOut, `warning: First note.md: Source "docs/real.md" has a commit (2026-03-01)`) {
		t.Errorf("code=%d out=%q err=%q", code, out, errOut)
	}
	code, out, _ = run("--json", "--root", root, "validate", "--check-sources", "--git")
	var doc struct {
		OK       bool
		Warnings []struct{ Rule, Note string }
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil || code != 0 || !doc.OK || len(doc.Warnings) != 1 || doc.Warnings[0].Rule != "source-stale" {
		t.Errorf("code=%d doc=%+v err=%v\n%s", code, doc, err, out)
	}
}

func TestGitStalenessSkipsOutsideARepository(t *testing.T) {
	root := sourcesCollection(t, "source_root = \".\"\ncheck_sources = true\ncheck_stale = true\n")
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(root))
	os.Remove(filepath.Join(root, "notes", "First note.md"))
	if err := os.WriteFile(filepath.Join(root, "notes", "First note.md"), []byte("---\nid: 20261021120000\ntags: [\"type/idea\", \"status/open\", \"category/direction\"]\n---\nBody.\n\nSource: docs/real.md, lines 1-2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, errOut := run("--root", root, "validate")
	if code != 0 || !strings.Contains(errOut, "source-stale skipped: ") {
		t.Errorf("code=%d err=%q", code, errOut)
	}
}
