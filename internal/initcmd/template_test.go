// SPDX-License-Identifier: EUPL-1.2

package initcmd_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wstein/chartscribe/internal/initcmd"
	"github.com/wstein/chartscribe/internal/validate"
)

func initCollection(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := initcmd.Files(nil, "demo")
	es, err := initcmd.Plan(root, files, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := initcmd.Apply(root, files, es); err != nil {
		t.Fatal(err)
	}
	return root
}

func read(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func write(t *testing.T, root, rel, text string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A note made from either generated template (placeholder substituted by the
// tool, tags and Source filled by the author) passes validate; unfilled it does not.
func TestNotesFromTemplatesValidate(t *testing.T) {
	templates := []struct{ path, placeholder string }{
		{"templates/Note.md", "{{date:YYYYMMDDHHmmss}}"},
		{".zk/templates/note.md", "{{format-date now '%Y%m%d%H%M%S'}}"},
	}
	for i, tt := range templates {
		t.Run(tt.path, func(t *testing.T) {
			root := initCollection(t)
			tmpl := read(t, root, tt.path)
			if !strings.Contains(tmpl, tt.placeholder) {
				t.Fatalf("template lacks %s", tt.placeholder)
			}
			note := strings.Replace(tmpl, tt.placeholder, "20261021120"+string(rune('0'+i))+"00", 1)
			write(t, root, "notes/Idea.md", note)
			write(t, root, "notes/Map - Direction.md",
				"---\nid: 20261021130000\ntags: [\"type/map\", \"category/direction\"]\n---\nMap.\n\n- [[Idea]]: summary\n")
			res, err := validate.Run(root)
			if err != nil {
				t.Fatal(err)
			}
			rules := map[string]bool{}
			for _, f := range res.Findings {
				rules[f.Rule] = true
			}
			if !rules["list-type"] && !rules["tag-count"] || !rules["source-missing"] {
				t.Errorf("unfilled template should fail on tags and Source, got %v", res.Findings)
			}
			filled := strings.Replace(note, "tags: []", `tags: ["type/idea", "status/open", "category/direction"]`, 1)
			filled = strings.Replace(filled, "Source:\n", "Source: a book, lines 1-5\n", 1)
			write(t, root, "notes/Idea.md", filled)
			res, err = validate.Run(root)
			if err != nil || len(res.Findings) != 0 {
				t.Errorf("filled note: %v %v", res.Findings, err)
			}
		})
	}
}
