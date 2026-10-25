# Contract: validate rules

Reference implementation: `chartroom/scripts/check_notes.py`. Each rule has a stable name used in
the output. A finding has `rule`, `note` (file name, empty for collection-level findings) and `message`.
All findings are errors. The only warning is `source-stale` (see [sources](sources.md)); it never changes the exit status.

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
| `source-missing` | a note that is not a map has no well-formed Source line (`Source: <text>, lines A-B` or a typed form, see [sources](sources.md)) |
| `source-kind-unknown`, `source-malformed` | a Source line with an unknown kind prefix or a malformed typed form ([sources](sources.md)) |
| `source-file-missing`, `source-not-file`, `source-range-invalid`, `source-outside-root` | only with `--check-sources`: the cited file or range does not hold ([sources](sources.md)) |
| `wikilink-unresolved` | a `[[target]]` in a body does not resolve to a title or alias (case-insensitive); the label (`\|`) and heading (`#`) parts are ignored for resolution |
| `map-unlisted` | a non-map note is linked from no map |
| `map-multiple` | a non-map note is linked from more than one map (several links to it from the same map count once) |
| `map-category` | the one map linking a note has a different category than the note |
| `map-duplicate` | more than one map for a category |
| `category-no-map` | a category in use has no map |
| `readme-link` | a Markdown link in `README.md` is broken or points outside the collection (external schemes and `#anchors` ignored) |

Markdown links in notes are not checked. Wikilinks inside code blocks are checked like any other
(same as the reference); a later spec may change that.

## Fixes

`validate --fix` (plan) and `validate --fix --apply` (write) fix a finding only when one value is certain. Each
fix is verified: validate runs again on the result, and the fix is kept only if the findings drop and none is new.
Only frontmatter fields and the link lines of a map or of `README.md` change; the note body stays byte-identical.

| Rule | Fix |
| --- | --- |
| `id-invalid` | an id of exactly 14 digits with `-`, `:`, `_`, space or `T` between them that is a real date and time becomes the 14 digits, unless another note has it |
| `list-type` | a scalar `tags` (one tag without space or comma) or `aliases` (one alias without comma) becomes a one-item list |
| `tag-invalid` | the value of a `type/`, `status/` or `category/` tag is lower-cased when the lower-case value is in the vocabulary; no other tag changes |
| `tag-count` | a repeated `type/`, `status/` or `category/` tag (also after lower-casing) is removed; repeated free tags stay |
| `name-duplicate` | an alias equal to its own title, or repeated in the same note, is removed |
| `map-unlisted` | `- [[Title]]` is added after the list of links of the one map of the note's category, only when the note has exactly one category tag, exactly one map has it, the map has exactly one list of `- [[...` lines, and every note's frontmatter parses |
| `readme-link` | a broken relative link is rewritten when exactly one path matches after ignoring case, one component at a time; the `#anchor` stays |

Not fixed, because no single value is certain or the fix needs a decision: `frontmatter`, `field-unknown`,
`id-missing` (no id can be derived), `id-duplicate`, `title-char`, `source-missing` and the other `source-` rules, `wikilink-unresolved`,
`map-multiple`, `map-category`, `map-duplicate`, `category-no-map`, a missing `tags` or `aliases` field (adding
`tags: []` would trade one finding for three `tag-count` findings; a missing `aliases` is no finding), and list
formatting or ordering (no finding names it, so nothing changes). A list that is rewritten is written as `["a", "b"]`.

## Tracking section

A note may end with a generated section written by `cscribe tracking`: it starts at the line `## Tracking`
after the `Source:` line and ends at the next `## ` heading or at the end of the file. Each line is
`- #<number> <title> (<state>) <url>`. `validate` accepts the section, and ignores its content: no
`wikilink-unresolved` finding comes from it (an issue title may contain `[[...]]`).

## Differences from the reference

A note linked several times from one map counts once for `map-multiple` (the reference reports it as in more than one map).

Single-quoted scalars, plain flow items and block list items without indentation are accepted (Obsidian and other YAML writers produce them; the reference reports `frontmatter` or `id-invalid` for them), and the `## Tracking` section is skipped. The parity test records each difference in `testdata/validate/diff`. Everything
else is the reference behavior, including its quirks, until a fixture says otherwise.
