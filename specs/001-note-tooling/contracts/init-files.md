# Contract: init file set

```text
cscribe [--root DIR] [--json] init [--tool zk|obsidian|quartz]... [--apply] [--overwrite PATH|all]
```

- Without `--apply`: print the plan, one line per file: `write`, `exists (skipped)`, `differs (skipped)` or, when `--overwrite` names a differing file, `overwrite`; write nothing. Exit 0.
- `--apply`: write every file that does not exist. An existing file is never touched unless `--overwrite PATH` names it (or `all`).
- Root: `--root`, else the nearest directory with a `notes/` folder, else the working directory (an empty directory works); `init` does not create `notes/`.
- No `--tool`: all three. Paths are relative to the collection root. Directories are created as needed.
- A rerun after `--apply` plans nothing to write. `--json` prints `{"version":1,"command":"init","files":[{"path","action"}]}`.
- `init` writes files only: it starts no tool and installs no plugin. Existing JSON config is not merged (open point).
- Keys below were checked online (see Verification); embedded file text carries no private path or name.

## Common

`.cscribe.toml` is written under every `--tool`, including `quartz`. The template files belong to their tools: `templates/Note.md` to Obsidian, `.zk/templates/note.md` to zk; their bodies differ in the id placeholder only (`{{date:YYYYMMDDHHmmss}}` against `{{format-date now '%Y%m%d%H%M%S'}}`). `cscribe new` (slice 3) uses an embedded template of the same shape and needs neither file.

| Path | Content |
| --- | --- |
| `.cscribe.toml` | the tag vocabulary of the validate contract (default values) |
| `templates/Note.md` | written only with `--tool obsidian`; see Obsidian below |

Obsidian note template `templates/Note.md`:

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
| `.zk/config.toml` | `[note]`: `filename = "{{title}}"`, `extension = "md"`, `template = "note.md"`, `exclude = ["templates/**", "README.md"]`; `[format.markdown]`: `link-format = "wiki"`, `hashtags = false` (so `#42` in prose is no tag) |
| `.zk/templates/note.md` | frontmatter `id: {{format-date now '%Y%m%d%H%M%S'}}` (not `timestamp`, which has 12 digits; not `{{id}}`, which is random), empty `aliases` and `tags`, `## Links`, `Source:` |
| `.zk/.gitignore` | `notebook.db` |

See Verification below.

## Obsidian (`--tool obsidian`)

| Path | Keys |
| --- | --- |
| `.obsidian/app.json` | `newFileLocation: "folder"`, `newFileFolderPath: "notes"`, `useMarkdownLinks: false`, `newLinkFormat: "shortest"`, `alwaysUpdateLinks: true` |
| `.obsidian/core-plugins.json` | object of core plugin id to boolean with `"templates": true` and the ids seen in real vaults (`file-explorer`, `global-search`, `switcher`, `graph`, `backlink`, `outgoing-link`, `tag-pane`, `page-preview`, `command-palette`, `editor-status`, `outline`, `word-count`, `properties`, `bookmarks`, `canvas`); Templates is off in those vaults, so it must be set |
| `.obsidian/templates.json` | `folder: "templates"` (the template is `templates/Note.md`); `dateFormat` and `timeFormat` are optional defaults, the template names its own format |
| `.obsidian/types.json` | `{"types": {"id": "text", "aliases": "aliases", "tags": "tags"}}` |
| `.obsidian/.gitignore` | `workspace.json`, `workspaces.json`, `cache` (per-user UI state) |

Not written: community
plugins (Linter, Templater are optional suggestions in the docs), `bookmarks`, appearance, hotkeys.

## Quartz (`--tool quartz`)

Quartz v4 (4.5.2 tested, MIT, Node 22 or newer) is a separate checkout that builds the notes into a
static site. Only what lives in the notes repository is written:

| Path | Content |
| --- | --- |
| `.quartz/settings.json` | the values the spike needed: `pageTitle` (collection name), `baseUrl` (host plus subpath, no protocol, e.g. `<owner>.github.io/<repo>`), `analytics: null`; no ignore patterns are needed because only `notes/` is copied (Quartz already ignores `private`, `templates`, `.obsidian` by default) |
| `.quartz/index.md` | home page with `title:` frontmatter, instead of the README (whose repo-relative links are dead in Quartz) |
| `.github/workflows/pages.yml` | builds with Quartz at a pinned commit (4.5.2 is `4923affa7722dfc751f1074348e6dad214fe0c08`): run `cscribe validate` first (Quartz silently renders an unresolved wikilink as a dead link), check out Quartz, copy `notes/` and `index.md` into its `content/`, apply the settings, build, deploy to GitHub Pages. Action versions are pinned by SHA when the file is written; `cscribe` is installed at the version of the binary that ran `init` (module version, else VCS revision), or the placeholder `REPLACE_WITH_CSCRIBE_VERSION` that fails the step until replaced; `go-version: stable` floats |

How the settings reach `quartz.config.ts` (a patch step or a full replacement file for the pinned
version) is open; the spike used the default configuration. The workflow is a proposal until
the Pages PR builds it in CI. The Quartz checkout, its build and the Pages settings stay with the owner.
Slugs are ugly (spaces become `-`, ` - ` becomes `---`) and backlinks, tags and graph exist.

## Verification

Checked online on 2026-10-21 (T201). "Official" means the project's documentation; "vault" means a file
of a real public vault on GitHub; nothing was run in Obsidian.

| Item | Result | Source |
| --- | --- | --- |
| Obsidian: wikilinks setting, automatic link update | verified (UI settings) | <https://obsidian.md/help/links> |
| Obsidian: Templates plugin variables `{{title}}`, `{{date}}`, `{{time}}`, format `{{date:FORMAT}}` with Moment.js tokens, template folder, date and time format settings; inserts into the active note | verified, official | <https://obsidian.md/help/plugins/templates> |
| Obsidian: property types, defaults `tags`, `aliases`, `cssclasses` are lists, vault-wide types | verified, official | <https://obsidian.md/help/Editing+and+formatting/Properties> |
| `.obsidian/app.json` keys `newFileLocation: "folder"`, `newFileFolderPath`, `useMarkdownLinks`, `newLinkFormat` (`shortest`, `relative`), `alwaysUpdateLinks` | key names and value spellings seen in vaults, not documented officially; `absolute` as a third value not seen | e.g. <https://github.com/OmooLab/BioxelNodes/blob/HEAD/.obsidian/app.json>, <https://github.com/bramses/bramses-highly-opinionated-vault-2023/blob/HEAD/.obsidian/app.json> |
| `core-plugins.json` is an object id to boolean; id `templates`; `templates` is `false` in the vaults seen | verified in vaults | e.g. <https://github.com/TfTHacker/DashboardPlusPlus/blob/HEAD/.obsidian/core-plugins.json> |
| `templates.json` keys `folder`, `dateFormat`, `timeFormat` | verified in a vault | <https://github.com/davidvkimball/vaultcms/blob/HEAD/.obsidian/templates.json> |
| `types.json` shape `{"types": {name: type}}`, values `text`, `aliases`, `tags`, `multitext`, `number`, `date` | verified in a vault | <https://github.com/kepano/kepano-obsidian/blob/HEAD/.obsidian/types.json> |
| zk `[note]` keys `filename`, `extension`, `template` (relative to `.zk/templates/`), `exclude`; `ignore` is a legacy alias of `exclude` | verified, official and source | <https://zk-org.github.io/zk/config/config-note.html>, `internal/core/config.go` in <https://github.com/zk-org/zk> |
| zk `[format.markdown]` `link-format` (`markdown`, `wiki`), `hashtags` (default true) | verified, official | `docs/notes/note-format.md` in <https://github.com/zk-org/zk> |
| zk templates: `{{title}}`, `{{now}}`, `{{format-date now FORMAT}}`; named format `timestamp` is `YYYYMMDDHHMM` (12 digits); strftime placeholders allowed | verified, official | `docs/notes/template-creation.md` and `docs/notes/template.md` in <https://github.com/zk-org/zk> |
| zk 0.15.7 reads our format, `id` and `filename` forms work | verified by running zk | spike of 2026-10-21 |
| Quartz 4.5.2 config `pageTitle`, `baseUrl` (no protocol, with subpath on GitHub Pages), `analytics: null`, `ignorePatterns` default `private`, `templates`, `.obsidian` | verified in the source at the pinned commit | `quartz.config.ts`, `docs/configuration.md` at `4923affa7722dfc751f1074348e6dad214fe0c08` of <https://github.com/jackyzha0/quartz> |
| Quartz Pages workflow shape (Node 22, `npm ci`, `npx quartz build`, upload `public`, `deploy-pages`) | verified in the 4.5.2 docs; the hosted docs now describe v5 (other config format) | `docs/hosting.md` at the same commit |

Not confirmed: whether Obsidian accepts `newLinkFormat: "absolute"` (not used); whether Obsidian keeps
all keys when it rewrites `app.json` (it may add its own); that a missing `core-plugins.json` leaves
Templates off (assumed from the vaults, and `init` writes the file anyway); how the Quartz settings reach
`quartz.config.ts` (open point 3). Core plugin ids beyond `templates` come from vaults of different Obsidian versions.
