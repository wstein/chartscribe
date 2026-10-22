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
| file | `Source: <path>, lines A-B` or `Source: <path>, lines A-B @<commit>` | The legacy form is unchanged; the pin is optional (see Pins). It wins over every other form. `<path>` is free text unless `--check-sources` is on. |
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

## Pins and range repair

A file source may carry the commit its range was cited at:

```text
Source: <path>, lines A-B @<commit>
```

Grammar: `<commit>` is 7 to 40 lower-case hex digits after ` @` at the end of the line. A line without a pin is
the legacy form and behaves as before (`validate` accepts both; a pin that is not 7 to 40 lower-case hex digits is
`source-malformed`, but only in a note that has no legacy line). The tool writes 12 digits. A pin is resolved
with `git rev-parse --verify <commit>^{commit}` after it has been checked as hex.

Remap algorithm (`git diff -U0 <pin> HEAD -- <path>`, hunks `@@ -a,b +c,d @@`; with `b = 0` the hunk is an insertion
after old line `a`). Map the pinned range `[A,B]` through the hunks in order, with `shift = 0`:

1. A hunk with `b > 0` that covers a line of `[A,B]` (`a <= B` and `a+b-1 >= A`): the range is `changed`.
2. A hunk with `b > 0` that ends before `A`: `shift += d - b`. A hunk that starts after `B`: no effect.
3. An insertion after old line `x` with `A <= x < B` (between two lines of the range): `changed`.
4. An insertion with `x < A`: `shift += d`. With `x >= B`: no effect.
5. Without a `changed` hunk: `shift = 0` is `current`, otherwise `moved` to `[A+shift, B+shift]`.
6. Safety net: for `current` and `moved` the text of the pinned lines (`git cat-file blob <pin>:<path>`) must equal the
   text of the mapped lines in the file; if not the result is `changed`. So `current` and `moved` always mean the same text.

Statuses of `sources check` (each pinned file source; unpinned ones are only counted as `unpinned`):

| Status | Meaning |
| --- | --- |
| `current` | the range holds the pinned text where it was |
| `moved` | the same text at other lines (`new_from`, `new_to`) |
| `changed` | a hunk touches the range: manual review; the hunk headers are listed |
| `renamed` | git (`-M`) sees the file under another path (`new_path`); `update` never changes the path |
| `missing` | the file does not exist now (deleted since the pin, or never there) |
| `unknown` | git cannot tell, with the reason: pin not a commit of this clone, shallow clone, the file has uncommitted changes, is not tracked, the pin has no such file or fewer lines, the path leaves the source root or passes a symbolic link |

Comparison is always pinned commit against HEAD; a cited file with uncommitted changes is `unknown`, never judged by
the working tree. A line-ending or whitespace change is a change (no `--ignore-*` option is used). A binary or
type-changed file is `changed`.

### Commands

`cscribe sources check [NOTE...]` reads only. Exit status 1 when a source is `moved`, `changed`, `renamed` or `missing`
(`unknown` and `unpinned` do not fail), 2 usage error, 3 environment error (git missing, source root not in a
repository with a commit). stdout: one line per source that needs attention
(`<note>:<line> <path> lines A-B @<pin>: moved to lines A'-B'`) and the summary
`sources check: <N> notes, <S> file sources: <c> current, <m> moved, ...`; skipped parts go to stderr as
`sources skipped: <reason>`. `--json` writes `{version, command: "sources check", ok, notes, counts, sources[], skipped?}`;
each source has `note, line, path, from, to, pin?, status, new_from?, new_to?, new_path?, action, reason?, hunks?`.

`cscribe sources pin [NOTE... | --all] [--rev <commit>] [--apply]` adds ` @<12 hex>` to each unpinned file source. Pinning
vouches that the cited range is right at that commit: at HEAD (default) the range must be valid in the file, the file
tracked and free of uncommitted changes; with `--rev` the file must exist at that commit with at least `B` lines.
Other lines are `skip` with the reason. Exit 0.

`cscribe sources update [NOTE... | --all] [--apply]` rewrites each `moved` source to `A'-B'` and moves its pin to HEAD.
`changed`, `renamed` and `missing` sources are listed with `action: review` and the hunks, and never rewritten. Exit 1
while a source is still stale or needs review (in a plan, the moved ones count as stale; after `--apply` they do not).

`pin` and `update` plan by default (`sources pin: 1 edit in 1 file planned; --apply writes them`, each edit as
`notes/<file>:<line> [source-pin|source-update]`, `- old`, `+ new`); `--apply` plans and writes in one run, all notes or none, with the
atomic write and restore-on-failure of `validate --fix --apply`. Before writing, the result is validated with
`--check-sources`: it must add no finding, every edited line must parse as the planned file source and every other
byte of the note must be as before. A symbolic link, a note with CR line endings or a note changed since the plan is
refused. `--json` has the same document plus `plan: {mode, applied, edits[], refused[]}` and `command` `sources pin|update`.

git runs through exec with fixed arguments and no shell, read-only (`GIT_OPTIONAL_LOCKS=0`), with
`core.quotepath=off`, `core.autocrlf=false`, `diff.renames=true`, `--no-ext-diff`, `--no-textconv`, `--no-pager`; the
environment variables `GIT_EXTERNAL_DIFF`, `GIT_DIFF_OPTS` and `GIT_PAGER` are dropped, paths follow `--` as literal
pathspecs and commit ids are checked as hex first. A shallow clone is skipped (`unknown`, reason). Cost: one
`git diff --name-status -M` per distinct pin, and for a changed file one `git diff -U0` and one `git cat-file`
(each cached); the diff output is read as a stream and only hunk headers are kept.

### With `validate --check-sources --git`

A pinned source is judged by content, not by "any later commit": `moved` gives
`Source "<path>" lines A-B (pinned @<pin>) now holds the same text at lines A'-B'; run cscribe sources update`, `changed`
and `renamed` give a warning too, `unknown` a `source-stale skipped:` line. This does not depend on the state of the
note. An unpinned source keeps the commit-time comparison above.

## Not yet

An anchor hash of the cited text and the remapping of a renamed file are not done. `validate --fix` never changes a
Source line. `new` and `init` templates list the forms, with `[@<commit>]`, in a comment above the empty `Source:` line.
