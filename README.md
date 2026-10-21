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

Pre-alpha skeleton. The only command is `cscribe version`.

## Design

The design notes are not public yet.

## Development

`make check` runs the formatting check, `go vet` and `go test ./...`.

## Licence

[EUPL-1.2](LICENSE).
