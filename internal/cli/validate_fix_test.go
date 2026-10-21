// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fixSource = "Source: IDEAS_NOTES.md §1. Direction, lines 1-5\n"

// fixCollection writes a collection with one fixable note: a mixed-case duplicate status tag.
func fixCollection(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"notes/Map - Direction.md": "---\nid: 20261021120100\ntags: [\"type/map\", \"category/direction\"]\n---\nMap.\n\n- [[First note]]\n",
		"notes/First note.md":      "---\nid: 20261021120000\ntags: [\"type/idea\", \"status/Open\", \"status/open\", \"category/direction\"]\n---\nBody text.\n\n" + fixSource,
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

func TestFixPlanIsDefaultAndWritesNothing(t *testing.T) {
	root := fixCollection(t)
	path := filepath.Join(root, "notes", "First note.md")
	before, _ := os.ReadFile(path)
	code, out, errOut := run("--root", root, "validate", "--fix")
	if code != 1 {
		t.Errorf("exit = %d, want 1 (findings exist)", code)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Error("plan wrote a file")
	}
	for _, want := range []string{
		"validate: 2 notes, 2 findings\n",
		"fix: 1 edit in 1 file planned; findings 2 -> 0; --apply writes them\n",
		"notes/First note.md:3 [tag-invalid]\n",
		"- tags: [\"type/idea\", \"status/Open\", \"status/open\", \"category/direction\"]\n",
		"+ tags: [\"type/idea\", \"status/open\", \"category/direction\"]\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout lacks %q:\n%s", want, out)
		}
	}
	if !strings.Contains(errOut, "First note.md: ") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestFixApplyWritesAndIsIdempotent(t *testing.T) {
	root := fixCollection(t)
	code, out, errOut := run("--root", root, "validate", "--fix", "--apply")
	if code != 0 || errOut != "" {
		t.Errorf("apply: %d %q", code, errOut)
	}
	if !strings.Contains(out, "fix: applied 1 edit in 1 file\n") || !strings.HasSuffix(out, "validate: 2 notes, 0 findings\n") {
		t.Errorf("stdout = %q", out)
	}
	code, out, _ = run("--root", root, "validate", "--fix")
	if code != 0 || !strings.Contains(out, "fix: nothing to fix\n") {
		t.Errorf("second run: %d %q", code, out)
	}
}

func TestFixJSON(t *testing.T) {
	root := fixCollection(t)
	code, out, errOut := run("--json", "--root", root, "validate", "--fix")
	if code != 1 || errOut != "" {
		t.Fatalf("plan: %d %q", code, errOut)
	}
	var doc struct {
		OK       bool              `json:"ok"`
		Findings []json.RawMessage `json:"findings"`
		Fix      struct {
			Mode    string `json:"mode"`
			Applied bool   `json:"applied"`
			Before  int    `json:"findings_before"`
			After   int    `json:"findings_after"`
			Edits   []struct {
				File string `json:"file"`
				Line int    `json:"line"`
				Rule string `json:"rule"`
				Old  string `json:"old"`
				New  string `json:"new"`
			} `json:"edits"`
			Refused []json.RawMessage `json:"refused"`
		} `json:"fix"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	f := doc.Fix
	if doc.OK || len(doc.Findings) != 2 || f.Mode != "plan" || f.Applied || f.Before != 2 || f.After != 0 ||
		len(f.Edits) != 1 || f.Edits[0].File != "notes/First note.md" || f.Edits[0].Line != 3 || f.Edits[0].Rule != "tag-invalid" ||
		!strings.HasPrefix(f.Edits[0].New, "tags: [") || f.Refused == nil {
		t.Errorf("doc = %+v\n%s", doc, out)
	}
	code, out, _ = run("--json", "--root", root, "validate", "--fix", "--apply")
	if code != 0 || !strings.Contains(out, `"mode": "apply"`) || !strings.Contains(out, `"applied": true`) || !strings.Contains(out, `"ok": true`) {
		t.Errorf("apply: %d\n%s", code, out)
	}
}

func TestFixPlainValidateHasNoFixKey(t *testing.T) {
	_, out, _ := run("--json", "--root", fixCollection(t), "validate")
	if strings.Contains(out, `"fix"`) {
		t.Errorf("validate without --fix has a fix key:\n%s", out)
	}
}

func TestApplyNeedsFix(t *testing.T) {
	root := fixCollection(t)
	if code, _, errOut := run("--root", root, "validate", "--apply"); code != 2 || !strings.Contains(errOut, "--apply needs --fix") {
		t.Errorf("exit = %d %q", code, errOut)
	}
	code, out, _ := run("--json", "--root", root, "validate", "--apply")
	if code != 2 || !strings.Contains(out, `"error": "usage"`) {
		t.Errorf("json: %d %s", code, out)
	}
}

func TestFixRuleFilterLimitsFixes(t *testing.T) {
	root := fixCollection(t)
	code, out, _ := run("--root", root, "validate", "--fix", "--rule", "id-invalid")
	if code != 0 || !strings.Contains(out, "fix: nothing to fix\n") {
		t.Errorf("%d %q", code, out)
	}
}

func TestFixNoCollection(t *testing.T) {
	code, out, _ := run("--json", "--root", t.TempDir(), "validate", "--fix")
	if code != 3 || !strings.Contains(out, `"no-collection"`) {
		t.Errorf("%d %s", code, out)
	}
}
