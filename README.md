# ChartScribe

ChartScribe is a Go CLI (`cscribe`) for plain-text Zettelkasten collections.

## Note format

- The filename is the note title.
- The frontmatter carries `id` (a 14-digit timestamp), `aliases` and `tags`.
- Links are `[[wikilinks]]`.
- Tags are namespaced: `type/`, `status/`, `category/`.
- A `Chartroom: <id> <title>` line in a note refers to an issue.

## What it does

`cscribe` validates and lints notes, creates, lists and links them, writes the
configuration for the preferred tools (`init`) and generates issue-tracking sections
from the `Chartroom: <id> <title>` lines. Everything file-based is native. [`zk`](https://github.com/zk-org/zk)
is a recommended separate companion for complex queries; `cscribe` never starts it and
only writes formats it reads unchanged. Manual (planned): <https://wstein.github.io/chartscribe/>. See [specs/001-note-tooling](specs/001-note-tooling/spec.md).

## Status

Pre-alpha. Implemented: `cscribe validate` (alias `lint`), `cscribe init`, `new`, `list`, `tags`, `links`, `backlinks`, `tracking` and `version`; the other
commands of the [spec](specs/001-note-tooling/spec.md) follow in later slices.

## Commands

```text
cscribe [--root DIR] [--json] validate [--rule NAME]...
cscribe lint ...                 (alias of validate)
cscribe [--root DIR] [--json] init [--tool zk|obsidian|quartz]... [--apply] [--overwrite PATH|all]
cscribe [--root DIR] [--json] new TITLE [--tag TAG]...
cscribe [--root DIR] [--json] list [--tag TAG]
cscribe [--root DIR] [--json] tags
cscribe [--root DIR] [--json] links NOTE
cscribe [--root DIR] [--json] backlinks NOTE
cscribe [--root DIR] [--json] tracking [--write]
cscribe version
```

`validate` checks `notes/*.md` and `README.md` of a collection against the
[rules](specs/001-note-tooling/contracts/validate-rules.md); it only reads files. The root is `--root`,
else the nearest directory with a `notes/` folder. Findings go to stderr as `<note>: <message>`,
the summary `validate: <N> notes, <M> findings` to stdout; `--json` writes one document to stdout
([contract](specs/001-note-tooling/contracts/validate-output.md)). Exit status: 0 clean, 1 findings,
2 usage error, 3 environment error. An optional `.cscribe.toml` at the root replaces the tag vocabulary:

```toml
[tags]
type = ["idea", "decision"]
status = ["open", "done"]
category = ["direction", "state"]
```

`init` prints a plan for the zk, Obsidian and Quartz configuration files (one line per file: `write`,
`exists (skipped)`, `differs (skipped)` or `overwrite`) and writes nothing. `--apply` writes the files that
do not exist; an existing file stays untouched unless `--overwrite PATH` (relative to the root) or
`--overwrite all` names it. `--tool` limits the set (default all three; `.cscribe.toml` is always written).
A rerun plans nothing to write. The root is `--root`, else the nearest directory with a `notes/` folder,
else the working directory. `init` writes files only and starts no tool; the file list is the
[init contract](specs/001-note-tooling/contracts/init-files.md). The Quartz workflow is a proposal.

`new TITLE` writes `notes/TITLE.md` (creating `notes/` when missing) with the id from the clock
(`YYYYMMDDHHMMSS`, `-001`, `-002`... when a note already has it), empty `aliases`, the `--tag` tags, a `## Links`
heading and an empty `Source:` line, and prints the path. It exits 2 for an existing file or title, an empty
title or a forbidden character (`\ / : * ? " < > | # [ ] ^`, control characters, a leading dot). The note passes `validate`
once you fill the tags and the `Source: <text>, lines A-B` line and list it in a map.
`list` prints `id`, title, path and tags per note (tab separated; `--tag` filters); `tags` prints each tag and its
note count; `links NOTE` prints the wikilinks of a note and `backlinks NOTE` the notes that link to it. NOTE is a
title, alias, file name or id; links resolve like `validate` resolves them (title or alias, any case, `|label` and
`#heading` ignored) and unresolved targets are printed as `unresolved`. These commands only read files, skip
notes whose frontmatter does not parse (with a warning on stderr) and exit 2 for an unknown NOTE.
`--json` output is specified in [query output](specs/001-note-tooling/contracts/query-output.md).

`tracking` reads the issues of the current repository with `gh issue list --state all --limit 200` and rewrites only the
`## Tracking` section of each note whose id appears in a `Chartroom: <id> <title>` line of an issue title or body (lines
`- #<n> <title> (<state>) <url>`). It prints the plan and writes nothing unless `--write` is given; bytes outside the
section stay identical. `gh` missing or failing, or 200 issues returned (some may be missing), exits 3. It is the only
command that starts a process; it never uses the network itself. See [tracking output](specs/001-note-tooling/contracts/tracking-output.md).

The collection of the Anchorage workspace is checked with `cscribe --root chartroom validate`; the
umbrella switches its `make chartroom-check` from `chartroom/scripts/check_notes.py` in a separate pull request.
`.cscribe.toml` is read by a small built-in parser (a `[tags]` table of string arrays), so the module
depends on cobra only.

## Design

The design notes are not public yet.

## Development

`make check` runs the formatting check, `go vet` and `go test ./...`. The parity test compares
the findings with `chartroom/scripts/check_notes.py` of the Anchorage workspace on the fixtures under
`testdata/validate` when `python3` and the script are available (`CHECK_NOTES_PY=<path>`; `CHARTROOM_COLLECTION=<dir>`
adds a collection) and is skipped otherwise. The fixtures are synthetic: they are shaped like the real notes but contain no
real note text.

## Licence

[EUPL-1.2](LICENSE).
