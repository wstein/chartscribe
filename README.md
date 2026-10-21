# ChartScribe

ChartScribe is a Go CLI (`cscribe`) for plain-text Zettelkasten collections in
the cx-style note format used by the Anchorage `chartroom/` collection: the
filename is the title, the frontmatter carries `id` (a 14-digit timestamp),
`aliases` and `tags`, links are `[[wikilinks]]`, and tags are namespaced
(`type/`, `status/`, `category/`).

It validates and lints notes, generates issue-tracking sections from
`Chartroom: <id>` lines, and forwards standard note work (new, list, search,
backlinks, edit) to the external tool [`zk`](https://github.com/zk-org/zk).
ChartScribe only starts `zk` as a process; `zk` is GPL-3.0 and none of its code
is copied or linked.

## Status

Pre-alpha skeleton. The only command is `cscribe version`.

## Design

The design lives in the chartroom notes of
[wstein/anchorage-workspace](https://github.com/wstein/anchorage-workspace)
(`chartroom/notes/`). Those notes still use the working name Cairn.

## Development

`make check` runs the formatting check, `go vet` and `go test ./...`.

## Licence

[EUPL-1.2](LICENSE).
