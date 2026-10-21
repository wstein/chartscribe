# Implementation plan: Note tooling

Go, module `github.com/wstein/chartscribe`, pinned version in `.go-version`. Few direct
dependencies; the standard library covers the frontmatter subset, JSON and TOML is the only
candidate for a library (decide in T101).

## Layout

```text
cmd/cscribe/            main: calls internal/cli.Run
internal/cli/           flags, output, exit codes (testable Run(args, stdout, stderr))
internal/note/          filename, frontmatter parser, wikilinks, id
internal/collection/    load a collection, config (.cscribe.toml)
internal/validate/      rules (one file per rule group), findings
internal/initcmd/       embedded file set, plan and apply
testdata/               fixture collections: valid, one per failing rule, chartroom snapshot
```

## Slices and PRs

One PR (or a short stack) per slice; each ends with `make check` green.

1. `validate`: parser, rules, outputs. Fixtures from `chartroom/scripts/test_check_notes.py`; a snapshot of the 64 chartroom notes is committed under `testdata/` (no private data; notes are the owner's public design notes, confirm before commit).
2. `init`: embedded templates, plan/apply, per-tool tests; T201 verifies Obsidian and zk keys by hand.
3. `new`, `list`, `tags`, `links`, `backlinks` on the slice-1 parser.
4. `tracking`: `gh` behind an interface; tests use a fake runner, one optional test uses real `gh`.
5. Optional: `--fix`, MCP.

## Verification

`make check` (gofmt, vet, tests) per PR. Optional `zk` compatibility test (skipped when `zk` is absent). No build or release targets; builds, tags, releases and the BundleDock entry need the owner's approval.
