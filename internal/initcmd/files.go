// SPDX-License-Identifier: EUPL-1.2

// Package initcmd holds the embedded tool configuration of `cscribe init` and
// the plan/apply logic that writes it (specs/001-note-tooling/contracts/init-files.md).
package initcmd

import (
	"encoding/json"
	"strings"
)

// Tool selects one tool's file set.
type Tool string

// The tools init can configure.
const (
	ZK       Tool = "zk"
	Obsidian Tool = "obsidian"
	Quartz   Tool = "quartz"
)

// AllTools is the default selection, in output order.
var AllTools = []Tool{ZK, Obsidian, Quartz}

// File is one file of the set. Path is relative to the collection root and
// uses forward slashes.
type File struct {
	Path    string
	Content string
}

// QuartzCommit is the pinned Quartz 4.5.2 commit.
const QuartzCommit = "4923affa7722dfc751f1074348e6dad214fe0c08"

// Files returns the files for the selected tools (nil or empty: all), in a
// fixed order and without duplicates. .cscribe.toml comes first and is
// written under every tool. title is the collection name for the Quartz files.
func Files(tools []Tool, title string) []File {
	if len(tools) == 0 {
		tools = AllTools
	}
	files := []File{{".cscribe.toml", cscribeToml}}
	for _, tool := range AllTools {
		if !selected(tools, tool) {
			continue
		}
		switch tool {
		case ZK:
			files = append(files,
				File{".zk/config.toml", zkConfig},
				File{".zk/templates/note.md", noteTemplate("{{format-date now '%Y%m%d%H%M%S'}}")},
				File{".zk/.gitignore", "notebook.db\n"})
		case Obsidian:
			files = append(files,
				File{"templates/Note.md", noteTemplate("{{date:YYYYMMDDHHmmss}}")},
				File{".obsidian/app.json", obsidianApp},
				File{".obsidian/core-plugins.json", corePlugins()},
				File{".obsidian/templates.json", "{\n  \"folder\": \"templates\"\n}\n"},
				File{".obsidian/types.json", "{\n  \"types\": {\n    \"id\": \"text\",\n    \"aliases\": \"aliases\",\n    \"tags\": \"tags\"\n  }\n}\n"},
				File{".obsidian/.gitignore", "workspace.json\nworkspaces.json\ncache\n"})
		case Quartz:
			files = append(files,
				File{".quartz/settings.json", quartzSettings(title)},
				File{".quartz/index.md", "---\ntitle: " + jsonString(title) + "\n---\n\nWelcome to " + singleLine(title) + ".\n"},
				File{".github/workflows/pages.yml", pagesWorkflow})
		}
	}
	return files
}

func selected(tools []Tool, t Tool) bool {
	for _, s := range tools {
		if s == t {
			return true
		}
	}
	return false
}

func noteTemplate(id string) string {
	return "---\nid: " + id + "\naliases: []\ntags: []\n---\n\n## Links\n\nSource:\n"
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func singleLine(s string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(s)
}

func quartzSettings(title string) string {
	return "{\n  \"pageTitle\": " + jsonString(title) +
		",\n  \"baseUrl\": \"<owner>.github.io/<repo>\",\n  \"analytics\": null\n}\n"
}

func corePlugins() string {
	ids := []string{"file-explorer", "global-search", "switcher", "graph", "backlink", "outgoing-link",
		"tag-pane", "page-preview", "templates", "command-palette", "editor-status", "outline",
		"word-count", "properties", "bookmarks", "canvas"}
	var b strings.Builder
	b.WriteString("{\n")
	for i, id := range ids {
		b.WriteString("  \"" + id + "\": true")
		if i < len(ids)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("}\n")
	return b.String()
}

const cscribeToml = `[tags]
type = ["idea", "decision", "milestone", "design", "evidence", "question", "process"]
status = ["open", "decided", "done", "dropped", "reference"]
category = ["direction", "state", "ratings", "roadmap", "design", "credentials-network", "providers", "interfaces", "recovery", "decisions", "delivery", "process"]
`

const zkConfig = `[note]
filename = "{{title}}"
extension = "md"
template = "note.md"
exclude = ["templates/**", "README.md"]

[format.markdown]
link-format = "wiki"
hashtags = false
`

const obsidianApp = `{
  "newFileLocation": "folder",
  "newFileFolderPath": "notes",
  "useMarkdownLinks": false,
  "newLinkFormat": "shortest",
  "alwaysUpdateLinks": true
}
`

// pagesWorkflow is a proposal (contract: Quartz). Action versions are pinned by SHA.
const pagesWorkflow = `# Proposal: publishes the notes with Quartz 4.5.2 to GitHub Pages.
# Set baseUrl in .quartz/settings.json and enable Pages (source: GitHub Actions) first.
name: pages
on:
  push:
    branches: [main]
  workflow_dispatch:
permissions:
  contents: read
concurrency:
  group: pages
  cancel-in-progress: false
jobs:
  build:
    runs-on: ubuntu-latest
    timeout-minutes: 15
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          persist-credentials: false
      - uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0
        with:
          go-version: stable
      - name: Validate the notes
        run: go install github.com/wstein/chartscribe/cmd/cscribe@main && "$(go env GOPATH)/bin/cscribe" validate
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          repository: jackyzha0/quartz
          ref: ` + QuartzCommit + `
          path: quartz
          persist-credentials: false
      - uses: actions/setup-node@949feb2413d6458794dcd2491c4babbbce0c15c1 # v7.1.0
        with:
          node-version: 22
      - name: Copy the notes and apply the settings
        run: |
          set -euo pipefail
          rm -rf quartz/content && mkdir quartz/content
          cp -r notes quartz/content/notes
          cp .quartz/index.md quartz/content/index.md
          node -e '
            const fs = require("fs");
            const s = JSON.parse(fs.readFileSync(".quartz/settings.json", "utf8"));
            const f = "quartz/quartz.config.ts";
            let t = fs.readFileSync(f, "utf8");
            t = t.replace(/pageTitle: "[^"]*"/, "pageTitle: " + JSON.stringify(s.pageTitle))
                 .replace(/baseUrl: "[^"]*"/, "baseUrl: " + JSON.stringify(s.baseUrl))
                 .replace(/analytics: \{[^}]*\}/, "analytics: null");
            fs.writeFileSync(f, t);
          '
      - name: Build
        working-directory: quartz
        run: npm ci && npx quartz build
      - uses: actions/upload-pages-artifact@fc324d3547104276b827a68afc52ff2a11cc49c9 # v5.0.0
        with:
          path: quartz/public
  deploy:
    needs: build
    if: github.ref == 'refs/heads/main'
    runs-on: ubuntu-latest
    timeout-minutes: 10
    permissions:
      pages: write
      id-token: write
    environment:
      name: github-pages
      url: ${{ steps.deployment.outputs.page_url }}
    steps:
      - id: deployment
        uses: actions/deploy-pages@368f82528645a54fb793d4d04e342629a3f51346 # v5.0.1
`
