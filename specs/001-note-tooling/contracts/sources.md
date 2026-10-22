# Contract: Source lines

A note that is not a map has one or more lines that start with `Source: ` at the line start, outside code
fences and outside the generated `## Tracking` section. A note needs at least one well-formed one
(`source-missing`). Rules and examples only.

Compatibility: a note that earlier versions accepted (a line `Source: <text>, lines A-B` anywhere) stays
accepted, with the same findings. Only lines that were already ignored can now give `source-kind-unknown` or
`source-malformed`, and only in a note without such a legacy line. One deliberate exception: a bare URL with
a range (`Source: https://example.com/x, lines 1-1`) is `source-malformed`, because the range cannot be real.

## Forms

| Kind | Line | Notes |
| --- | --- | --- |
| file | `Source: <path>, lines A-B` | The legacy form, unchanged. It wins over every other form. `<path>` is free text unless `--check-sources` is on. |
| decision | `Source: decision: <free text>, <YYYY-MM-DD>` | The date is required and a real calendar date. Alias: `Source: decision <YYYY-MM-DD> <who or text>`. |
| url | `Source: url: <https://...>` or the bare `Source: https://...` | `http` or `https` with a host, no space, no line range (`url sources take no line range`). Never fetched. |
| measurement | `Source: measurement: <path>[, <key>]` | `<key>` is optional text (the first `, ` separates it, so a path holds no `, `). Alias: `Source: measurement <path or command> <YYYY-MM-DD>`. |

The legacy form wins over every other: `Source: decision 2026-10-21 owner, lines 1-1` is a file source. The
aliases have no colon, so they never clash with the canonical forms. The canonical forms are the ones `new`
and the guide teach.

A measurement text is checked as a file only when it looks like a relative path: one word (no space) that
contains `/` or `.` and does not start with `-`. Anything else (`cargo bench --bench x`) is free text and is
never existence-checked.

```text
Source: docs/design.md, lines 12-30
Source: decision: keep the parser hand-written, 2026-10-22
Source: url: https://example.org/spec
Source: measurement: bench/run1.csv, p95 latency
```

## Rules

Always (no flag):

| Rule | Finding when |
| --- | --- |
| `source-kind-unknown` | a line `Source: <word>: ...` whose word is not `decision`, `url` or `measurement` (kinds are lower-case) and that is not the legacy form |
| `source-malformed` | a `decision:`, `url:` or `measurement:` line that does not parse (no or invalid date, bad URL, empty path) |
| `source-missing` | no well-formed Source line and no line with its own finding above |

With `validate --check-sources` or `check_sources = true` (kind file, and measurement with a path-like text; nothing is fetched):

| Rule | Finding when |
| --- | --- |
| `source-file-missing` | the path does not exist below the source root |
| `source-not-file` | the path exists but is not a regular file; the message says what it is (a directory, a named pipe, a socket, a device) |
| `source-range-invalid` | for `lines A-B`: not `1 <= A <= B <= number of lines` (a last line without a newline counts; an empty file has none) |
| `source-outside-root` | the path is absolute, leaves the source root with `..`, or a symbolic link resolves outside it |

Source root: the `source_root` key of `.cscribe.toml` (relative to the collection root, or absolute), else the
git toplevel of the collection root (the nearest directory above it with a `.git` entry; no process is started),
else the parent of the collection root. Paths are slash paths relative to it.

## Staleness (warning)

With `validate --check-sources --git` or `check_stale = true` (with `check_sources`): `source-stale` is a
warning, not a finding. For a file or measurement source that resolves, it compares `git log -1 --format=%ct`
of the cited file with that of the note file; a newer source commit gives
`warning: <note>: Source "<path>" has a commit (<date>) newer than the last commit of this note (<date>); ...`.
Warnings do not change `ok` or the exit status (1 stays "findings"); the summary appends `, <W> warnings`
and `--json` gets an additive `warnings` array (same shape as `findings`). git runs through exec with fixed
arguments, read-only, with `GIT_OPTIONAL_LOCKS=0`. What cannot be compared is skipped with a line
`source-stale skipped: <reason>` on stderr (JSON `skipped`, an array of those lines) and never fails:
git missing, a source root outside a repository, a shallow clone (`shallow clone (history incomplete)`), a
note with uncommitted changes or not committed (`<note>: note has uncommitted changes`), a note or cited
file without history in the clone. `--rule source-stale` limits the output to the warning.

Cost: one `git log -1` per distinct file (cached) plus one `git status` for the notes folder; about 0.8 s for
1,000 notes citing 300 files.

## Not yet (design only)

No line remapping. A later `cscribe sources pin` may record an optional `@<commit>` after the path
(`Source: <path>@<commit>, lines A-B`); the pinned range could then be remapped to the current file with
`git diff` and reported or updated. Until then a stale warning only says that the file moved on.

`validate --fix` never changes a Source line. `new` and `init` templates list the forms in a comment above
the empty `Source:` line.
