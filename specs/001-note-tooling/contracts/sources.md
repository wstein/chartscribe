# Contract: Source lines

A note that is not a map has one or more lines that start with `Source: ` (at the line start, after
the `## Links` heading; the generated `## Tracking` section is not read). A note needs at least one
well-formed one (`source-missing`). Rules and examples only.

## Forms

| Kind | Line | Notes |
| --- | --- | --- |
| file | `Source: <path>, lines A-B` | The legacy form, unchanged. It wins over every other form. `<path>` is free text unless `--check-sources` is on. |
| decision | `Source: decision: <free text>, <YYYY-MM-DD>` | The date is required and a real calendar date. |
| url | `Source: url: <https://...>` | `http` or `https` with a host, no space. Never fetched. |
| measurement | `Source: measurement: <path>[, <key>]` | `<path>` is a data file; `<key>` is optional text (the first `, ` separates it, so a path holds no `, `). |

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

With `validate --check-sources` or `check_sources = true` (kinds file and measurement only; nothing is fetched):

| Rule | Finding when |
| --- | --- |
| `source-file-missing` | the path does not exist below the source root |
| `source-not-file` | the path exists but is not a regular file (a directory) |
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
arguments, read-only, with `GIT_OPTIONAL_LOCKS=0`. A missing git or a source root outside a repository skips
the check with `source-stale skipped: <reason>` on stderr (JSON `skipped`) and never fails. A file or note
without a commit is left out. `--rule source-stale` limits the output to the warning.

## Not yet (design only)

No line remapping. A later `cscribe sources pin` may record an optional `@<commit>` after the path
(`Source: <path>@<commit>, lines A-B`); the pinned range could then be remapped to the current file with
`git diff` and reported or updated. Until then a stale warning only says that the file moved on.

`validate --fix` never changes a Source line. `new` and `init` templates list the forms in a comment above
the empty `Source:` line.
