# Contract: validate rules

Reference implementation: `chartroom/scripts/check_notes.py`. Each rule has a stable name used in
the output. A finding has `rule`, `note` (file name, empty for collection-level findings) and `message`.
All findings are errors; there are no warnings in slice 1.

## Collection

- Notes are `notes/*.md` (not recursive). The optional `README.md` at the collection root is checked only by `readme-*`.
- Vocabulary: `.cscribe.toml` (optional; a duplicate key or an unknown table or key is an environment error) sets the allowed values:

  ```toml
  [tags]
  type = ["idea", "decision", "milestone", "design", "evidence", "question", "process"]
  status = ["open", "decided", "done", "dropped", "reference"]
  category = ["direction", "state", "ratings", "roadmap", "design", "credentials-network", "providers", "interfaces", "recovery", "decisions", "delivery", "process"]
  ```

  The values above are the default when the file is absent. `map` is always allowed as a `type/` value.

## Frontmatter parser

A YAML subset: `key: value` lines, plain, double-quoted (JSON string) and single-quoted scalars,
flow lists (`["a", "b"]`, `[a, b]`, `[]`) and block lists (`- item`), `#` comments after a value. Anything else is
`frontmatter`. An unquoted `id` is read as text (it is digits).

## Rules

| Rule | Finding when |
| --- | --- |
| `frontmatter` | the file does not start with `---`, the block is unterminated, or a line cannot be parsed |
| `field-unknown` | a key other than `id`, `aliases`, `tags` |
| `id-missing` | no `id` |
| `id-invalid` | `id` is not `^\d{14}(-\d{3})?$` or its first 14 digits are not a calendar date and time (`YYYYMMDDHHMMSS`) |
| `id-duplicate` | two notes share an id |
| `list-type` | `aliases` (if present) or `tags` is not a list of strings; a missing `tags` counts |
| `tag-invalid` | a `type/`, `status/` or `category/` tag has a value outside the vocabulary |
| `tag-count` | not exactly one tag of a namespace; a note with `type/map` needs no `status/` tag |
| `title-char` | the filename stem contains one of `\ / : * ? " < > \| # [ ] ^` (the last four cut a wikilink target; a difference from the Python reference) |
| `name-duplicate` | two titles or aliases are equal, case-insensitively (title against alias included) |
| `source-missing` | a note that is not a map has no line `Source: <text>, lines A-B` |
| `wikilink-unresolved` | a `[[target]]` in a body does not resolve to a title or alias (case-insensitive); the label (`\|`) and heading (`#`) parts are ignored for resolution |
| `map-unlisted` | a non-map note is linked from no map |
| `map-multiple` | a non-map note is linked from more than one map (several links to it from the same map count once) |
| `map-category` | the one map linking a note has a different category than the note |
| `map-duplicate` | more than one map for a category |
| `category-no-map` | a category in use has no map |
| `readme-link` | a Markdown link in `README.md` is broken or points outside the collection (external schemes and `#anchors` ignored) |

Markdown links in notes are not checked. Wikilinks inside code blocks are checked like any other
(same as the reference); a later spec may change that.

## Tracking section

A note may end with a generated section written by `cscribe tracking`: it starts at the line `## Tracking`
after the `Source:` line and ends at the next `## ` heading or at the end of the file. Each line is
`- #<number> <title> (<state>) <url>`. `validate` accepts the section, and ignores its content: no
`wikilink-unresolved` finding comes from it (an issue title may contain `[[...]]`).

## Differences from the reference

A note linked several times from one map counts once for `map-multiple` (the reference reports it as in more than one map).

Single-quoted scalars, plain flow items and block list items without indentation are accepted (Obsidian and other YAML writers produce them; the reference reports `frontmatter` or `id-invalid` for them), and the `## Tracking` section is skipped. The parity test records each difference in `testdata/validate/diff`. Everything
else is the reference behaviour, including its quirks, until a fixture says otherwise.
