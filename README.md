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

Pre-alpha. Implemented: `cscribe validate` (alias `lint`), `cscribe init` and `cscribe version`; the other
commands of the [spec](specs/001-note-tooling/spec.md) follow in later slices.

## Commands

```text
cscribe [--root DIR] [--json] validate [--rule NAME]...
cscribe lint ...                 (alias of validate)
cscribe [--root DIR] [--json] init [--tool zk|obsidian|quartz]... [--apply] [--overwrite PATH|all]
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
