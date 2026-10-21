# Contract: export

`cscribe export NOTE [--format md] [--out FILE|-] [--depth N] [--title TEXT] [--force]` writes one Markdown
document. It only reads notes and writes only `--out`.

## Rules

- NOTE is a title, alias, file name or id. Unknown: exit 2.
- Included notes: NOTE, then the notes its resolved wikilinks name in body order (map order), each once,
  (wikilinks in code and in the `## Links` section are not members: the Links section is "related", not content;
  see Code below)
  down to `--depth` levels (default 1; 0 is NOTE alone). A note's direct members are all taken before
  deeper ones, so a note linked from the start and from a member stays at the start's level.
- Title: `# <title of NOTE>`, or `--title TEXT`. Every other note is a heading `#`x(1+level) with its title
  (level 6 at most). Body headings move so that the shallowest one is one level below the note's own heading
  (body `# H1` under `## Note` becomes `###`; level 6 at most). The body of NOTE follows its title.
- Code: fenced blocks (found by the scanner `validate` and `tracking` share, `note.CodeLines`: container prefixes
  of quotes and list items, the backtick info-string rule; a fence still open at the end of the note runs to the
  end, so the rest of that note is copied as code and no link in it is rewritten),
  indented code (4 spaces or a tab after a blank line, not inside a list) and inline code spans (a backtick run
  closed by a run of the same length, within one line) are copied as they are: no wikilink in them is a member,
  rewritten or reported, and a `#` line in them is no heading.
- Relative Markdown links and images in note bodies are not rewritten (a known limitation).
- Left out of each body: frontmatter, the generated `## Tracking` section, the `## Links` section and the
  `Source:` lines. Fenced code blocks are copied as they are (wikilinks and headings in them untouched).
- Wikilinks `[[T]]`, `[[T|label]]`, `[[T#Heading]]` (T a title or alias): to a note in the document
  `[label or T](#anchor)`; any other one (outside the export or unresolved) its label or T as plain text.
  Anchor: the GitHub heading slug of the title (lower case; letters, digits, `-` and `_` kept; spaces to
  `-`; `-1`, `-2` for a repeat).
- `## Sources`: the distinct `Source:` texts in first-appearance order, one `- <text>` each; omitted when none.
- Blank runs collapse to one line; the text ends with one newline. No timestamp, id or path: the same notes
  give the same bytes.
- `--out FILE` creates the file and refuses an existing one (exit 2) unless `--force`; with `--force` the text
  is written to a temporary file in the same directory and renamed over FILE. `--out` is refused (exit 2), even
  with `--force`, when it resolves (cleaned, symbolic links of its directory resolved) inside the collection's
  `notes/` tree (decided by file identity of the directory and its ancestors and of FILE against every note, so links and a case-insensitive file system are covered, and by a case-insensitive path comparison), and when FILE is a symbolic link (links are neither followed nor replaced). A missing directory
  or a directory as FILE is exit 3. `-` or no `--out` is stdout. Only md is generated (`--format` other: exit 2).
- Unresolved links: `unresolved [[target]] in <title>` on stderr, exit 0.
- `--json` needs `--out FILE` (exit 2 otherwise) and prints one document on stdout:
  `{version, command, ok, format, depth, start: {id, title, path, tags}, notes: [{id, title, path, level}],
  unresolved: [{note, target}]}`; an error is `{version, command, ok: false, error, message}`.

## Example

Collection `testdata/export/basic`; `cscribe --root testdata/export/basic export "Map - Harbor"`:

````markdown
# Map - Harbor

Notes about the harbor.

- [Anchor](#anchor): holds the ship
- [Chain](#chain): connects it
- [Buoy](#buoy): marks the spot

## Anchor

An anchor is held by a [chain link](#chain) and marked by a [Buoy](#buoy).
See also Lighthouse and Missing note.

### Details

Heavier than the [Hook](#anchor) suggests.

## Chain

A chain joins the [Anchor](#anchor) to the ship.

```
[[Not a link in code]]
## Not a heading
```

## Buoy

A buoy floats above the [Anchor](#anchor).

## Sources

- HARBOR.md §1. Anchors, lines 1-9
- HARBOR.md §2. Buoys, lines 10-14
````

stderr: `unresolved [[Missing note]] in Anchor`. The golden file is `testdata/golden/export.md`.
