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

Pre-alpha. Implemented: `cscribe validate` (alias `lint`), `cscribe init`, `new`, `list`, `tags`, `links`, `backlinks`, `tracking`, `export` and `version`; the other
commands of the [spec](specs/001-note-tooling/spec.md) follow in later slices.

## Commands

```text
cscribe [--root DIR] [--json] validate [--rule NAME]... [--check-sources [--git]]
cscribe lint ...                 (alias of validate)
cscribe [--root DIR] [--json] validate --fix [--apply] [--rule NAME]...
cscribe [--root DIR] [--json] init [--tool zk|obsidian|quartz]... [--apply] [--overwrite PATH|all]
cscribe [--root DIR] [--json] new TITLE [--tag TAG]...
cscribe [--root DIR] [--json] list [--tag TAG]
cscribe [--root DIR] [--json] tags
cscribe [--root DIR] [--json] links NOTE
cscribe [--root DIR] [--json] backlinks NOTE
cscribe [--root DIR] [--json] tracking [--write]
cscribe [--root DIR] [--json] export NOTE [--format md] [--out FILE|-] [--depth N] [--title TEXT] [--force]
cscribe version
```

`validate` checks `notes/*.md` and `README.md` of a collection against the
[rules](specs/001-note-tooling/contracts/validate-rules.md); it only reads files unless `--fix --apply` is given. The root is `--root`,
else the nearest directory with a `notes/` folder. Findings go to stderr as `<note>: <message>`,
the summary `validate: <N> notes, <M> findings` to stdout; `--json` writes one document to stdout
([contract](specs/001-note-tooling/contracts/validate-output.md)). Exit status: 0 clean, 1 findings,
2 usage error, 3 environment error. A note ends with one or more `Source:` lines: `<path>, lines A-B`, or typed
`decision: <text>, <YYYY-MM-DD>`, `url: <https://...>`, `measurement: <path>[, <key>]`
([sources](specs/001-note-tooling/contracts/sources.md)). `validate --check-sources` also checks that the cited
file exists below the source root (a file, not a directory) and that the line range is real; `--git` adds a
read-only `source-stale` warning (never an error) when a cited file has a newer commit than the note. Nothing is
fetched. `cscribe sources pin|check|update` keep the ranges true when the cited file changes: a pin
`Source: <path>, lines A-B @<commit>` lets `check` tell moved, changed, renamed and missing ranges apart and `update` rewrite the
moved ones (`--apply` to write; see the contract). An optional `.cscribe.toml` at the root replaces the tag vocabulary and may set the source options:

```toml
source_root = ".."        # default: the git toplevel of the collection, else its parent
check_sources = true      # same as --check-sources
check_stale = true        # same as --git (needs check_sources)

[tags]
type = ["idea", "decision"]
status = ["open", "done"]
category = ["direction", "state"]
```

`validate --fix` plans the safe fixes of the findings and prints each exact edit (`file:line [rule]`, `-` old
and `+` new lines; `--json` adds a `fix` object); nothing is written until `--apply` is added. A fix is
planned only when one value is certain, and it is kept only if re-running validate on the result shows fewer
findings and no new one; otherwise the file is listed as refused. Note bodies stay byte-identical: only
frontmatter fields and the link lines of a map or of `README.md` change. Files are written through a temporary
file and a rename inside the collection; symlinks, files with CR line endings and files changed since the plan
are refused. A second run plans nothing. Fixable rules and the rules that are deliberately not fixed:
[validate rules](specs/001-note-tooling/contracts/validate-rules.md#fixes).

`init` prints a plan for the zk, Obsidian and Quartz configuration files (one line per file: `write`,
`exists (skipped)`, `differs (skipped)` or `overwrite`) and writes nothing. `--apply` writes the files that
do not exist; an existing file stays untouched unless `--overwrite PATH` (relative to the root) or
`--overwrite all` names it. `--tool` limits the set (default all three; `.cscribe.toml` is always written).
A rerun plans nothing to write. The root is `--root`, else the nearest directory with a `notes/` folder,
else the working directory. `init` writes files only and starts no tool; the file list is the
[init contract](specs/001-note-tooling/contracts/init-files.md). The Quartz workflow is a proposal.

`new TITLE` writes `notes/TITLE.md` (creating `notes/` when missing) with the id from the clock
(`YYYYMMDDHHMMSS`, `-001`, `-002`... when a note already has it), empty `aliases`, the `--tag` tags, a `## Links`
heading and an empty `Source:` line, and prints the path. It exits 2 for an existing file or title, an empty
title or a forbidden character (`\ / : * ? " < > | # [ ] ^`, control characters, a leading dot). The note passes `validate`
once you fill the tags and the `Source: <text>, lines A-B` line and list it in a map.
`list` prints `id`, title, path and tags per note (tab separated; `--tag` filters); `tags` prints each tag and its
note count; `links NOTE` prints the wikilinks of a note and `backlinks NOTE` the notes that link to it. NOTE is a
title, alias, file name or id; links resolve like `validate` resolves them (title or alias, any case, `|label` and
`#heading` ignored) and unresolved targets are printed as `unresolved`. These commands only read files, skip
notes whose frontmatter does not parse (with a warning on stderr) and exit 2 for an unknown NOTE.
`--json` output is specified in [query output](specs/001-note-tooling/contracts/query-output.md).

`tracking` reads the issues of the current repository with `gh issue list --state all --limit 200` and rewrites only the
`## Tracking` section of each note whose id appears in a `Chartroom: <id> <title>` line of an issue title or body (lines
`- #<n> <title> (<state>) <url>`). It prints the plan and writes nothing unless `--write` is given; bytes outside the
section stay identical. `gh` missing or failing, or 200 issues returned (some may be missing), exits 3. It is the only
command that starts a process; it never uses the network itself. See [tracking output](specs/001-note-tooling/contracts/tracking-output.md).
`export NOTE` generates one Markdown document from a note, usually a map: the knowledge base stays notes and
documents are generated from them. The title is the start note; the notes its wikilinks name follow in link
(map) order as heading sections (`--depth`, default 1; each note once), without frontmatter, `## Links` section
and `Source:` lines. A wikilink to a note in the document becomes an anchor link, any other one its text; the
`Source:` lines are collected, without duplicates, into a final `## Sources` section. The output has no
timestamps, so a rerun gives the same bytes. It goes to stdout or `--out FILE`, which is never replaced without
`--force` and never inside `notes/` or at a symbolic link; nothing else is written. Wikilinks in code and in the `## Links` section do not pull notes in. `--title TEXT` replaces the document title. Only Markdown is generated (`--format md`): render it with Hugo, Quartz or
pandoc. `--json` (with `--out`) lists the included notes and the unresolved links; see
[export contract](specs/001-note-tooling/contracts/export.md).

The collection of the Anchorage workspace is checked with `cscribe --root chartroom validate`; the
umbrella switches its `make chartroom-check` from `chartroom/scripts/check_notes.py` in a separate pull request.
`.cscribe.toml` is read by a small built-in parser (a `[tags]` table of string arrays and the three top-level source keys), so the module
depends on cobra only.

## Releases and install

Releases are built by CI from a version tag and listed under
[Releases](https://github.com/wstein/chartscribe/releases). Pick the archive for your system
(`cscribe_<version>_<os>_<arch>`; `.zip` on Windows, `.tar.gz` elsewhere):

| System | Archive |
| --- | --- |
| Linux x86-64 / ARM 64-bit | `linux_amd64` / `linux_arm64` |
| macOS Apple silicon / Intel | `darwin_arm64` / `darwin_amd64` |
| Windows x86-64 / ARM 64-bit | `windows_amd64` / `windows_arm64` |
| Raspberry Pi 3, 4, 5 and Zero 2 W with a 64-bit OS | `linux_arm64` |
| Raspberry Pi 2 and later with a 32-bit OS (check with `uname -m`: `armv7l`) | `linux_armv7` |
| Raspberry Pi 1, Zero and Zero W (`armv6l`) | `linux_armv6` |
| Linux riscv64, ppc64le, s390x, loong64 | `linux_<arch>`; run under qemu-user by the release workflow |
| FreeBSD, OpenBSD, NetBSD (amd64, arm64); illumos amd64 | `<os>_<arch>`; built and header-checked, not run by CI |

Verify a download in the directory with the archives and `SHA256SUMS`, then check where it was built:

```sh
sha256sum --check --ignore-missing SHA256SUMS                                  # Linux
grep ' cscribe_0.1.0_darwin_arm64.tar.gz$' SHA256SUMS | shasum -a 256 --check   # macOS, one archive
gh attestation verify cscribe_0.1.0_linux_amd64.tar.gz --repo wstein/chartscribe
```

The release binary, checked as above, is the recommended way to install. Building from source with
`go install github.com/wstein/chartscribe/cmd/cscribe@latest` works from v0.1.1 on; v0.1.0 cannot be
fetched through the Go module proxy (its test fixtures held file names that Go module zips refuse),
so `@v0.1.0` fails and the v0.1.0 release archives are the way to get that version.

The binaries are not signed with a certificate or notarized. A browser-downloaded binary on
macOS carries a quarantine flag and Gatekeeper refuses it until you run `xattr -d com.apple.quarantine cscribe`;
a file fetched with `curl` or built with `go install` is unaffected. On Windows, SmartScreen may warn
about the unrecognized download ("More info", then "Run anyway"). The checksum and the attestation
are the proof of origin. See [docs/releasing.md](docs/releasing.md).

## Design

The design notes are not public yet.

## Development

`make check` runs the formatting check, `go vet` and `go test ./...`. The parity test compares
the findings with `chartroom/scripts/check_notes.py` of the Anchorage workspace on the fixtures under
`testdata/validate` when `python3` and the script are available (`CHECK_NOTES_PY=<path>`; `CHARTROOM_COLLECTION=<dir>`
adds a collection) and is skipped otherwise. The fixtures are synthetic: they are shaped like the real notes but contain no
real note text.

## License

[EUPL-1.2](LICENSE).
