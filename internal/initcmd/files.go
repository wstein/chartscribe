// SPDX-License-Identifier: EUPL-1.2

// Package initcmd holds the embedded tool configuration of `cscribe init` and
// the plan/apply logic that writes it (specs/001-note-tooling/contracts/init-files.md).
package initcmd

import (
	"encoding/json"
	"regexp"
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

// VersionPlaceholder is written into the workflow when the running binary has
// no version; the workflow step fails while it is present.
const VersionPlaceholder = "REPLACE_WITH_CSCRIBE_VERSION"

var versionOK = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*$`)

// WorkflowVersion is what the workflow pins: version if it is a plausible
// module version or revision, else VersionPlaceholder.
func WorkflowVersion(version string) string {
	if versionOK.MatchString(version) && version != "(devel)" {
		return version
	}
	return VersionPlaceholder
}

// Files returns the files for the selected tools (nil or empty: all), in a
// fixed order and without duplicates. .cscribe.toml comes first and is
// written under every tool. title is the collection name for the Quartz files;
// version is the cscribe version the Quartz workflow installs (see WorkflowVersion).
func Files(tools []Tool, title, version string) []File {
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
				File{".github/workflows/pages.yml", pagesWorkflow(WorkflowVersion(version))})
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

// quartzPatch applies .quartz/settings.json to the pinned quartz.config.ts. Each
// pattern must match exactly once, replacements are literal, and the baseUrl
// placeholder stops the build.
const quartzPatch = `import json, re, sys
s = json.load(open(".quartz/settings.json", encoding="utf-8"))
path = "quartz/quartz.config.ts"
t = open(path, encoding="utf-8").read()
def patch(pattern, repl):
    global t
    if len(re.findall(pattern, t)) != 1:
        sys.exit("quartz.config.ts: %r does not match exactly once" % pattern)
    t = re.sub(pattern, lambda m: repl, t)
if "<" in s["baseUrl"]:
    sys.exit("set baseUrl in .quartz/settings.json: it still holds the placeholder")
patch(r'pageTitle: "[^"]*"', "pageTitle: " + json.dumps(s["pageTitle"]))
patch(r'baseUrl: "[^"]*"', "baseUrl: " + json.dumps(s["baseUrl"]))
patch(r'analytics: \{[^}]*\}', "analytics: null")
open(path, "w", encoding="utf-8").write(t)
`

// pagesWorkflow is a proposal (contract: Quartz). Action versions are pinned by
// SHA; cscribe is installed at version, the one of the binary that ran init.
func pagesWorkflow(version string) string {
	indented := "            " + strings.ReplaceAll(strings.TrimSuffix(quartzPatch, "\n"), "\n", "\n            ") + "\n"
	return strings.NewReplacer("@VERSION@", version, "@PATCH@", indented, "@QUARTZ@", QuartzCommit).Replace(pagesTemplate)
}

const pagesTemplate = `# Proposal: publishes the notes with Quartz 4.5.2 to GitHub Pages.
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
        env:
          CSCRIBE_VERSION: @VERSION@
        run: |
          set -euo pipefail
          case "$CSCRIBE_VERSION" in
            REPLACE_WITH_*)
              echo "::error::Set CSCRIBE_VERSION in .github/workflows/pages.yml to a cscribe version or commit"
              exit 1;;
          esac
          go install "github.com/wstein/chartscribe/cmd/cscribe@$CSCRIBE_VERSION"
          "$(go env GOPATH)/bin/cscribe" validate
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          repository: jackyzha0/quartz
          ref: @QUARTZ@
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
          python3 - <<'PY'
@PATCH@          PY
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
