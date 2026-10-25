# Feature Specification: Note tooling for a Zettelkasten collection

**Branch**: `docs/spec-001-note-tooling` | **Created**: 2026-10-21
**Status**: Draft, independent review pending (round-1 findings addressed in follow-up commits). No code yet.
**Input**: Design notes `ChartScribe Overview`, `Command Surface`, `zk Integration`, `Delivery Plan` and `Obsidian Setup` in the Anchorage `chartroom/` collection, and the owner decisions of 2026-10-21.

## Scope and decisions

`cscribe` maintains the note format of the Anchorage `chartroom/` collection (and later
`<project>-notes` collections): the filename is the title; the frontmatter has only
`id` (14-digit timestamp), `aliases` and `tags`; tags are namespaced `type/`, `status/`,
`category/`; links are `[[wikilinks]]`; a note ends with `## Links` and a `Source:` line;
maps are notes; an issue refers to a note with a line `Chartroom: <id> <title>`; a generated
`## Tracking` section after the `Source:` line lists the issues that refer to a note
(format in the [validate rules](contracts/validate-rules.md#tracking-section)). The rules are in
[contracts/validate-rules.md](contracts/validate-rules.md); their reference implementation
today is `chartroom/scripts/check_notes.py`.

Owner decisions (2026-10-21), taken as given:

1. **Native first.** Trivial, deterministic, file-based functions live in `cscribe`:
   validate/lint, new from template, list, tags, links and backlinks, tracking. There is no
   `zk` pass-through, no wrapper command and no detection of `zk`. `zk` is a separate,
   recommended companion that the user runs directly for complex queries (ranked full-text
   search, fuzzy pick, editor and LSP integration); it is being added to the BundleDock
   catalog. The contract is one-way: `cscribe` writes only formats that `zk` reads unchanged
   (filename = title, frontmatter `id`/`aliases`/`tags`, `[[wikilinks]]`). An optional CI test
   runs `zk` against a generated collection when `zk` is installed.
2. **`cscribe init`** writes the tool configuration for the preferred tools (zk, Obsidian,
   Quartz), idempotently, as a plan by default; it never overwrites an existing file without
   an explicit flag. The file set is in [contracts/init-files.md](contracts/init-files.md).
3. **Humans write notes in Obsidian, configured by files, not by a plugin.** Config files make
   the editor create notes in the right place with wikilinks and the right template. They
   cannot enforce the format. The gate for hand-written notes is `cscribe validate`, run
   before commit and in CI, with JSON output. A custom Obsidian plugin is deferred unless this
   approach proves insufficient. Linter and Templater are optional suggestions; `init` never
   installs a plugin.
4. **Slices**: 1 `validate`, 2 `init`, 3 `new`/`list`/`links`/`backlinks`, 4 `tracking`,
   5 optional `--fix`. MCP: deferred; scope to be decided after slices 1-4 are in use.

Open points for the owner are listed at the end.

## User scenarios and acceptance

### US1 - Gate hand-written notes with validate (P1, slice 1)

A person edits notes in Obsidian or a text editor. `cscribe validate` checks the collection
without network access and reports every rule violation with the rule name, the note and a
message. It exits 0 when clean and 1 when there are findings.

1. The 64 notes of `chartroom/` at the time of writing give 0 findings.
2. Every failure fixture of `chartroom/scripts/test_check_notes.py` gives the matching finding.
3. `--json` prints one document with the same findings ([contracts/validate-output.md](contracts/validate-output.md)).
4. A frontmatter that Obsidian's Properties editor rewrote (block lists, plain scalars) is accepted
   when the values are valid.
5. `lint` is an alias of `validate`.

### US2 - Configure the preferred tools with init (P1, slice 2)

`cscribe init` prints which files it would write for zk, Obsidian and Quartz; with `--apply`
it writes the missing ones and reports the existing ones as skipped.

1. In an empty collection `init --apply` creates the files of the contract; a second run changes nothing and says so.
2. An existing file is never overwritten unless the user names it with `--overwrite PATH` (or `--overwrite all`).
3. A note created by the Obsidian template and a note created by `zk new` with the written config both pass `validate` once the user has filled the tags and the `Source:` line.
4. Quartz: `init` writes only what lives in the notes repository (see the contract); the Quartz checkout is separate.

### US3 - Create and find notes natively (P2, slice 3)

`cscribe new "Title"` writes `notes/Title.md` from the template with a fresh 14-digit id
(`-NNN` suffix on a collision within the same second). `list` prints notes (title, id, tags,
path) filtered by tag; `tags` lists tags with counts; `links NOTE` and `backlinks NOTE` list
outgoing and incoming wikilinks. All have `--json`. The `new` output passes `validate` apart from the
placeholders the user must fill.

### US4 - Keep a Tracking section current (P2, slice 4)

`cscribe tracking` runs `gh issue list --state all --limit 200 --json number,title,state,url,body` once and links issues to
notes by the `Chartroom: <id>` line ([tracking output](contracts/tracking-output.md); `--state all` because the default lists open issues only; 200 matches is an error, not a silent cut)
and rewrites only the generated `## Tracking` section, which shows each issue with its state; dry-run by default,
`--write` applies. `gh` missing or not logged in is an environment error (exit 3); the other commands never need `gh`.

### US5 - Repair mechanical problems (P3, slice 5, optional)

`validate --fix` repairs only safe mechanical findings (missing `id`, tag normalization to the
collection style, field order), never prose, and prints a plan unless `--write` is given. Optional and may be dropped.

## Requirements

| ID | Requirement | Slice |
| --- | --- | --- |
| FR-001 | `validate` implements every rule of the validate contract, with the same findings as `check_notes.py` on the shared fixtures. | 1 |
| FR-002 | The collection root is `--root`, else the nearest directory with a `notes/` folder; a missing root is exit 3. | 1 |
| FR-003 | Tag vocabulary comes from `.cscribe.toml` when present; the built-in default is the chartroom vocabulary. | 1 |
| FR-004 | Findings on stderr and the summary on stdout; `--json` emits one document on stdout; exit codes 0 clean, 1 findings, 2 usage, 3 environment. | 1 |
| FR-005 | `validate` reads files only: no network, no process start, no write. | 1 |
| FR-006 | `init` plans by default, `--apply` writes missing files only, `--overwrite` is explicit and per path; reruns are idempotent. | 2 |
| FR-007 | `init` writes configuration only; it installs no plugin and starts no tool. | 2 |
| FR-008 | `new` creates a note from the template, id from the clock, filename equals the title, refuses an existing filename or a forbidden character. | 3 |
| FR-009 | `list`, `tags`, `links`, `backlinks` read the collection natively and share the parser of `validate`. | 3 |
| FR-010 | `tracking` changes only the `## Tracking` section, defined as: starts at the line `## Tracking` after the `Source:` line, ends at the next `## ` heading or end of file; one line per issue `- #<number> <title> (<state>) <url>`; no matching issue removes the section; a missing section is appended after the last line. Bytes outside it stay identical. `validate` accepts the section and ignores its content. | 4 |
| FR-011 | The notes `cscribe` writes are readable by `zk` unchanged (filename = title, `id`/`aliases`/`tags`, `[[wikilinks]]`). | 2-4 |
| FR-012 | No dependency on `zk` code or binary: a test fails when `go.mod` or an import names a `zk-org` module or `cscribe` starts a `zk` process. | all |
| FR-013 | The CLI uses `github.com/spf13/cobra` at the version pinned by the sibling tools (v1.10.2 in `sourcesteward` and `sheetmill`) with their command-tree and exit-code boundary: `internal/cli.Run(ctx, version, args, stdout, stderr) int` owns a fresh command tree per call and is the only place that maps errors to exit codes. | 1 |
| FR-014 | The root help carries the agent hint in the sibling pattern (below); the repository ships `llms.txt` and `docs/agent-guide.md`; a test asserts the hint in the root help and its absence from subcommand help (as the siblings, which add it to the root help only). | 1 |
| FR-015 | A manual built from `docs/` is published at <https://wstein.github.io/chartscribe/> by a Pages workflow modeled on `sheetmill/.github/workflows/pages.yml`: actions pinned by SHA, Hugo pinned with checksum, deploy only after the checks pass, least-privilege permissions, concurrency group `pages`. A separate PR adds it. The Quartz view of chartroom is separate. | Pages |

## Agent hint and manual

The root help carries the hint of the sibling tools (`sourcesteward` `agentHints`, `sheetmill` `agentHelp`):

```text
Are you an AI? Use these resources ONLY IF your task specifically asks you to:
  Help a human understand, set up or use ChartScribe:
    https://raw.githubusercontent.com/wstein/chartscribe/main/docs/agent-guide.md
  Debug or investigate a problem with ChartScribe:
    https://raw.githubusercontent.com/wstein/chartscribe/main/llms.txt
```

`llms.txt` and `docs/agent-guide.md` follow the layout of `sheetmill/llms.txt` and `sheetmill/docs/agent-guide.md`.
GitHub Pages is enabled for the repository (build type workflow, <https://wstein.github.io/chartscribe/>).

## Obsidian: what config files can and cannot do

Checked online on 2026-10-21 (details and sources in [contracts/init-files.md](contracts/init-files.md)): the Obsidian
settings and their behavior are in the official help; the key names and file shapes of `.obsidian/*.json` are
not documented officially and were confirmed from real vaults on GitHub, not from a running Obsidian.

Config can set:

- where new notes go (`app.json`: `newFileLocation` `folder`, `newFileFolderPath` `notes`);
- link style (`useMarkdownLinks` false for wikilinks, `newLinkFormat` `shortest`, `alwaysUpdateLinks` true);
- the core Templates plugin and its folder, with a template whose frontmatter has
  `id: {{date:YYYYMMDDHHmmss}}`, empty `aliases` and `tags`, and the section skeleton;
- the type of each property in the Properties editor (`types.json`: `id` text, `aliases`, `tags`).

Config cannot:

- enforce anything when a note is created or edited by hand: the tag vocabulary, exactly one tag per
  namespace, the `Source:` line, uniqueness of ids and titles, map listing, link resolution;
- create a note from a template under a given title (the core Templates plugin only inserts into
  the current note; creating and naming stays manual or needs the Templater plugin);
- use the core "Unique note creator", because it names the file after the id, which breaks filename = title.

Hence the gate is `cscribe validate`. Obsidian's Properties editor rewrites frontmatter
(block lists, quoting), so the parser accepts both the flow and the block style.

## Success criteria

- SC-001: `cscribe validate` on the chartroom collection: 0 findings; every failure fixture gives its finding; the five mutation probes (broken wikilink, duplicate id, renamed category, category swapped to another valid one, note removed from a map) each give a finding.
- SC-002: Golden files for human and JSON output; exit codes 0, 1, 2, 3 each tested.
- SC-003: `init` into an empty directory, run twice: the second plan lists nothing to write; a pre-existing file is unchanged byte for byte without `--overwrite`.
- SC-006: The root help contains the agent hint and subcommand help does not (test); `llms.txt` and `docs/agent-guide.md` exist.
- SC-007: The Pages workflow builds the manual from `docs/` after the checks; the deploy job runs on `main` only.
- SC-004: A generated collection passes `validate`; when `zk` is installed an optional CI test runs `zk list --format json` and `zk graph` on it and finds every note and every wikilink.
- SC-005: When slice 1 is accepted, the umbrella's `make chartroom-check` calls `cscribe validate` and the Python checker is removed in the same PR (that PR belongs to the umbrella; tag, release and the BundleDock entry need the owner's approval).

## Non-goals

- No `zk` wrapper, pass-through or detection; no editor, renderer, sync service, index or database.
- No prose edits; `--fix` and `tracking` touch frontmatter or the generated section only.
- No Obsidian plugin (deferred), no installation of community plugins, no Quartz checkout or build.
- No port of `cx notes` features beyond validate/lint and linking (bundles, drift, cognition).
- Other tools (Hugo, VS Code settings, Logseq, Foam) are possible later `init` targets, not in this spec.

## Open points for the owner

1. Whether the tag vocabulary may be built in as the default (FR-003 assumes: file when present, chartroom vocabulary otherwise).
2. Whether `init` may later merge keys into an existing JSON config instead of skipping it (this spec: skip and report).
3. How the Quartz workflow applies the config settings (patch step or a full replacement `quartz.config.ts` for the pinned version); decided in the Pages PR, see the init contract.
4. Whether slice 5 (`--fix`) is wanted. MCP is deferred until slices 1-4 are in use.
