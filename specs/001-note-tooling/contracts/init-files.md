# Contract: init file set

```text
cscribe [--root DIR] [--json] init [--tool zk|obsidian|quartz]... [--apply] [--overwrite PATH|all]
```

- Without `--apply`: print the plan, one line per file: `write`, `exists (skipped)` or `differs (skipped)`; write nothing. Exit 0.
- `--apply`: write every file that does not exist. An existing file is never touched unless `--overwrite PATH` names it (or `all`).
- No `--tool`: all three. Paths are relative to the collection root. Directories are created as needed.
- A rerun after `--apply` plans nothing to write. `--json` prints `{"version":1,"command":"init","files":[{"path","action"}]}`.
- `init` writes files only: it starts no tool and installs no plugin. Existing JSON config is not merged (open point).
- Every key below is a proposal; task T201 verifies the Obsidian and zk keys against the tools before they are fixed in code. Embedded file text carries no private path or name.

## Common

| Path | Content |
| --- | --- |
| `.cscribe.toml` | the tag vocabulary of the validate contract (default values) |
| `templates/Note.md` | the template body below, used by Obsidian and (as `.zk/templates/note.md`) by zk |

Note template (Obsidian syntax):

```markdown
---
id: {{date:YYYYMMDDHHmmss}}
aliases: []
tags: []
---

## Links

Source:
```

`tags: []` and the empty `Source:` fail `validate` on purpose until the author fills them.

## zk (`--tool zk`)

| Path | Content |
| --- | --- |
| `.zk/config.toml` | `[note]`: `filename = "{{title}}"`, `extension = "md"`, `template = "note.md"`, ignore `templates/**` and `README.md`; wikilinks (`link-format = "wiki"`), hashtags off |
| `.zk/templates/note.md` | frontmatter `id: {{format-date now '%Y%m%d%H%M%S'}}` (not `timestamp`, which has 12 digits; not `{{id}}`, which is random), empty `aliases` and `tags`, `## Links`, `Source:` |
| `.zk/.gitignore` | `notebook.db` |

Verified in the 2026-10-21 spike with zk 0.15.7: the `id` and `filename` template forms, wikilink
resolution by filename stem. Not verified: the exact `ignore` and `link-format` keys.

## Obsidian (`--tool obsidian`)

| Path | Keys |
| --- | --- |
| `.obsidian/app.json` | `newFileLocation: "folder"`, `newFileFolderPath: "notes"`, `useMarkdownLinks: false`, `newLinkFormat: "shortest"`, `alwaysUpdateLinks: true` |
| `.obsidian/core-plugins.json` | the core Templates plugin enabled (object form `{"templates": true, ...}` of current Obsidian; older versions used a list) |
| `.obsidian/templates.json` | `folder: "templates"` (the template is `templates/Note.md`) |
| `.obsidian/types.json` | property types: `id` text, `aliases` aliases, `tags` tags |
| `.obsidian/.gitignore` | `workspace.json`, `workspaces.json`, `cache` (per-user UI state) |

Not verified offline: the key names and value spellings above, the `core-plugins.json` shape,
and whether a missing `core-plugins.json` keeps Templates enabled. Not written: community
plugins (Linter, Templater are optional suggestions in the docs), `bookmarks`, appearance, hotkeys.

## Quartz (`--tool quartz`)

Quartz v4 (4.5.2 tested, MIT, Node 22 or newer) is a separate checkout that builds the notes into a
static site. Only what lives in the notes repository is written:

| Path | Content |
| --- | --- |
| `.quartz/settings.json` | the values the spike needed: `pageTitle` (collection name), `baseUrl` (placeholder for the Pages host), `analytics: null`, ignore patterns `scripts`, `templates` |
| `.quartz/index.md` | home page with `title:` frontmatter, instead of the README (whose repo-relative links are dead in Quartz) |
| `.github/workflows/pages.yml` | builds with Quartz at a pinned commit (4.5.2 is `4923affa7722dfc751f1074348e6dad214fe0c08`): run `cscribe validate` first (Quartz silently renders an unresolved wikilink as a dead link), check out Quartz, copy `notes/` and `index.md` into its `content/`, apply the settings, build, deploy to GitHub Pages. Action versions are pinned by SHA when the file is written |

How the settings reach `quartz.config.ts` (a patch step or a full replacement file for the pinned
version) is open; the spike used the default configuration. The workflow is a proposal until
the Pages PR builds it in CI. The Quartz checkout, its build and the Pages settings stay with the owner.
Slugs are ugly (spaces become `-`, ` - ` becomes `---`) and backlinks, tags and graph exist.
