# Query output: new, list, tags, links, backlinks

Human output goes to stdout, one record per line, fields separated by a tab; warnings and errors go to stderr.
`--json` writes one document `{version: 1, command, ok, ...}` to stdout; an error is the document of
[validate-output.md](validate-output.md) (`ok: false`, `error`, `message`). Exit status: 0 done, 2 usage (including an
unknown NOTE and a refused title), 3 environment. Query commands never exit 1; unresolved links are data.

| Command | Human line | JSON members |
| --- | --- | --- |
| `new TITLE` | the path of the created file | `root`, `id`, `title`, `path` (relative), `tags` |
| `list [--tag T]` | `id`, `title`, `path`, tags joined by `,` | `notes: [{id, title, path, tags}]` |
| `tags` | `tag`, `count` (sorted by tag) | `tags: [{tag, count}]` |
| `links NOTE` | `[[target]]` and `-> title (path)` or `unresolved` | `note`, `links: [{target, resolved, id, title, path}]`, `unresolved` (count) |
| `backlinks NOTE` | `title`, `path` of each linking note | `note`, `backlinks: [{id, title, path, tags}]` |

Notes are in file-name order; links are in body order, duplicates kept. NOTE is a title or alias (any case), a file name
or an id. A link resolves as in the `wikilink-unresolved` rule; the generated `## Tracking` section is ignored. A note
whose frontmatter does not parse is skipped with `skipped <file>: ...` on stderr.
