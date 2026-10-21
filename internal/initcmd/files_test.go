// SPDX-License-Identifier: EUPL-1.2

package initcmd

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wstein/chartscribe/internal/collection"
)

var update = flag.Bool("update", false, "rewrite the golden files")

func paths(files []File) []string {
	var out []string
	for _, f := range files {
		out = append(out, f.Path)
	}
	return out
}

func TestFilesPerTool(t *testing.T) {
	tests := []struct {
		tools []Tool
		want  string
	}{
		{nil, ".cscribe.toml .zk/config.toml .zk/templates/note.md .zk/.gitignore templates/Note.md .obsidian/app.json .obsidian/core-plugins.json .obsidian/templates.json .obsidian/types.json .obsidian/.gitignore .quartz/settings.json .quartz/index.md .github/workflows/pages.yml"},
		{[]Tool{ZK}, ".cscribe.toml .zk/config.toml .zk/templates/note.md .zk/.gitignore"},
		{[]Tool{Obsidian}, ".cscribe.toml templates/Note.md .obsidian/app.json .obsidian/core-plugins.json .obsidian/templates.json .obsidian/types.json .obsidian/.gitignore"},
		{[]Tool{Quartz}, ".cscribe.toml .quartz/settings.json .quartz/index.md .github/workflows/pages.yml"},
		{[]Tool{Quartz, ZK, ZK}, ".cscribe.toml .zk/config.toml .zk/templates/note.md .zk/.gitignore .quartz/settings.json .quartz/index.md .github/workflows/pages.yml"},
	}
	for _, tt := range tests {
		got := strings.Join(paths(Files(tt.tools, "demo")), " ")
		if got != tt.want {
			t.Errorf("Files(%v):\n got %s\nwant %s", tt.tools, got, tt.want)
		}
	}
}

func TestFilesGolden(t *testing.T) {
	for _, f := range Files(nil, "demo") {
		path := filepath.Join("..", "..", "testdata", "init", filepath.FromSlash(f.Path))
		if *update {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(f.Content), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v", f.Path, err)
		}
		if f.Content != string(want) {
			t.Errorf("%s differs from its golden file:\ngot:\n%s\nwant:\n%s", f.Path, f.Content, want)
		}
	}
}

func content(t *testing.T, path string) string {
	t.Helper()
	for _, f := range Files(nil, "demo") {
		if f.Path == path {
			return f.Content
		}
	}
	t.Fatalf("no file %s", path)
	return ""
}

// The golden files pin the bytes; these assert the contract's load-bearing keys
// independently, so a regenerated golden file cannot hide a lost key.
func TestContractKeys(t *testing.T) {
	has := map[string][]string{
		".zk/config.toml": {`filename = "{{title}}"`, `extension = "md"`, `template = "note.md"`,
			`exclude = ["templates/**", "README.md"]`, `link-format = "wiki"`, `hashtags = false`},
		".zk/templates/note.md":       {"id: {{format-date now '%Y%m%d%H%M%S'}}\n", "aliases: []\n", "tags: []\n", "## Links", "Source:\n"},
		".zk/.gitignore":              {"notebook.db"},
		"templates/Note.md":           {"id: {{date:YYYYMMDDHHmmss}}\n", "aliases: []\n", "tags: []\n", "## Links", "Source:\n"},
		".obsidian/.gitignore":        {"workspace.json", "workspaces.json", "cache"},
		".github/workflows/pages.yml": {"/cscribe\" validate", "4923affa7722dfc751f1074348e6dad214fe0c08", "node-version: 22", "npx quartz build"},
	}
	for path, subs := range has {
		c := content(t, path)
		for _, s := range subs {
			if !strings.Contains(c, s) {
				t.Errorf("%s lacks %q", path, s)
			}
		}
	}
	for _, path := range []string{".zk/templates/note.md", "templates/Note.md"} {
		if strings.Contains(content(t, path), "{{id}}") || strings.Contains(content(t, path), "timestamp") {
			t.Errorf("%s uses a wrong id placeholder", path)
		}
	}
	var app map[string]any
	mustJSON(t, content(t, ".obsidian/app.json"), &app)
	wantApp := map[string]any{"newFileLocation": "folder", "newFileFolderPath": "notes", "useMarkdownLinks": false,
		"newLinkFormat": "shortest", "alwaysUpdateLinks": true}
	if len(app) != len(wantApp) {
		t.Errorf("app.json = %v", app)
	}
	for k, v := range wantApp {
		if app[k] != v {
			t.Errorf("app.json %s = %v, want %v", k, app[k], v)
		}
	}
	var plugins map[string]bool
	mustJSON(t, content(t, ".obsidian/core-plugins.json"), &plugins)
	for _, id := range []string{"templates", "file-explorer", "global-search", "switcher", "graph", "backlink", "outgoing-link",
		"tag-pane", "page-preview", "command-palette", "editor-status", "outline", "word-count", "properties", "bookmarks", "canvas"} {
		if !plugins[id] {
			t.Errorf("core plugin %s is not true", id)
		}
	}
	var tmpl map[string]any
	mustJSON(t, content(t, ".obsidian/templates.json"), &tmpl)
	if tmpl["folder"] != "templates" {
		t.Errorf("templates.json = %v", tmpl)
	}
	var types struct{ Types map[string]string }
	mustJSON(t, content(t, ".obsidian/types.json"), &types)
	if got := types.Types; len(got) != 3 || got["id"] != "text" || got["aliases"] != "aliases" || got["tags"] != "tags" {
		t.Errorf("types.json = %v", got)
	}
	var quartz map[string]any
	mustJSON(t, content(t, ".quartz/settings.json"), &quartz)
	if v, ok := quartz["analytics"]; !ok || v != nil || quartz["pageTitle"] != "demo" || quartz["baseUrl"] != "<owner>.github.io/<repo>" {
		t.Errorf("settings.json = %v", quartz)
	}
	if !strings.HasPrefix(content(t, ".quartz/index.md"), "---\ntitle: \"demo\"\n---\n") {
		t.Errorf("index.md = %q", content(t, ".quartz/index.md"))
	}
}

func TestQuartzTitleIsData(t *testing.T) {
	var s map[string]any
	for _, f := range Files([]Tool{Quartz}, "a\"b\\c\n---\nx: y") {
		switch f.Path {
		case ".quartz/settings.json":
			mustJSON(t, f.Content, &s)
			if s["pageTitle"] != "a\"b\\c\n---\nx: y" {
				t.Errorf("pageTitle = %q", s["pageTitle"])
			}
		case ".quartz/index.md":
			if strings.Count(f.Content, "---\n") != 2 {
				t.Errorf("title broke the frontmatter: %q", f.Content)
			}
		}
	}
}

func TestVocabularyFileIsDefault(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, collection.ConfigName), []byte(content(t, ".cscribe.toml")), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := collection.LoadVocabulary(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := collection.DefaultVocabulary()
	if strings.Join(got.Type, ",") != strings.Join(want.Type, ",") ||
		strings.Join(got.Status, ",") != strings.Join(want.Status, ",") ||
		strings.Join(got.Category, ",") != strings.Join(want.Category, ",") {
		t.Errorf("written vocabulary %v differs from the default %v", got, want)
	}
}

func mustJSON(t *testing.T, text string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(text), v); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, text)
	}
}
