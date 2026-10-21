// SPDX-License-Identifier: EUPL-1.2

package collection

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, text string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ConfigName), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestDefaultVocabularyWithoutConfig(t *testing.T) {
	v, err := LoadVocabulary(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	want := Vocabulary{
		Type:   []string{"idea", "decision", "milestone", "design", "evidence", "question", "process"},
		Status: []string{"open", "decided", "done", "dropped", "reference"},
		Category: []string{"direction", "state", "ratings", "roadmap", "design", "credentials-network",
			"providers", "interfaces", "recovery", "decisions", "delivery", "process"},
	}
	if !reflect.DeepEqual(v, want) {
		t.Errorf("default vocabulary = %#v", v)
	}
}

func TestConfigOverridesPerKey(t *testing.T) {
	root := writeConfig(t, `# vocabulary
[tags]
type = ["note", 'plain']   # inline comment with "quotes"
status = [
  "open",
  "closed", # trailing
]
`)
	v, err := LoadVocabulary(root)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(v.Type, []string{"note", "plain"}) || !reflect.DeepEqual(v.Status, []string{"open", "closed"}) {
		t.Errorf("vocabulary = %#v", v)
	}
	if len(v.Category) != 12 {
		t.Errorf("category keeps its default, got %v", v.Category)
	}
}

func TestConfigErrors(t *testing.T) {
	tests := []struct{ name, text, want string }{
		{"unknown table", "[other]\nx = []\n", "unknown table"},
		{"unknown key", "[tags]\nowner = []\n", "unknown key"},
		{"key outside table", "type = []\n", "outside"},
		{"not an array", "[tags]\ntype = \"idea\"\n", "array"},
		{"not a string item", "[tags]\ntype = [1]\n", "string"},
		{"unterminated array", "[tags]\ntype = [\"a\"\n", "unterminated"},
		{"unterminated string", "[tags]\ntype = [\"a]\n", "string"},
		{"empty value", "[tags]\ntype = [\"\"]\n", "empty"},
		{"duplicate key", "[tags]\ntype = [\"a\"]\ntype = [\"b\"]\n", "duplicate"},
		{"bad line", "[tags]\nnonsense\n", "line 2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadVocabulary(writeConfig(t, tt.text))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want it to contain %q", err, tt.want)
			}
			if err != nil && !strings.Contains(err.Error(), ConfigName) {
				t.Errorf("err %q does not name %s", err, ConfigName)
			}
		})
	}
}
