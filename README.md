# ChartScribe

ChartScribe is a Go CLI (`cscribe`) for plain-text Zettelkasten collections.

## Note format

- The filename is the note title.
- The frontmatter carries `id` (a 14-digit timestamp), `aliases` and `tags`.
- Links are `[[wikilinks]]`.
- Tags are namespaced: `type/`, `status/`, `category/`.
- A `Chartroom: <id> <title>` line in a note refers to an issue.

## What it does

`cscribe` validates and lints notes and generates issue-tracking sections from
the `Chartroom: <id> <title>` lines. It forwards standard note work (new, list,
search, backlinks, edit) to the external tool [`zk`](https://github.com/zk-org/zk).
ChartScribe only starts `zk` as a process; `zk` is GPL-3.0 and none of its code
is copied or linked.

## Status

Pre-alpha skeleton. The only command is `cscribe version`.

## Design

The design notes are not public yet.

## Development

`make check` runs the formatting check, `go vet` and `go test ./...`.

## Licence

[EUPL-1.2](LICENSE).
