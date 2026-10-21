# Implementation plan: Note tooling

Go, module `github.com/wstein/chartscribe`, pinned version in `.go-version`. Few direct
dependencies: `github.com/spf13/cobra` (v1.10.2, as `sourcesteward` and `sheetmill`; model:
`sourcesteward/internal/cli/root.go`) and a TOML library for `.cscribe.toml` (decide in T101); the standard
library covers the frontmatter subset and JSON.

## Layout

```text
cmd/cscribe/            main: calls internal/cli.Run
internal/cli/           cobra command tree, output, exit codes (Run(ctx, version, args, stdout, stderr) int; one fresh tree per call)
internal/note/          filename, frontmatter parser, wikilinks, id
internal/collection/    load a collection, config (.cscribe.toml)
internal/validate/      rules (one file per rule group), findings
internal/initcmd/       embedded file set, plan and apply
testdata/               fixture collections: valid, one per failing rule, chartroom snapshot
```

## Slices and PRs

One PR (or a short stack) per slice; each ends with `make check` green.

1. `validate`: parser, rules, outputs. Fixtures from `chartroom/scripts/test_check_notes.py`; a snapshot of the 64 chartroom notes is committed under `testdata/` (no private data; notes are the owner's public design notes, confirm before commit).
2. `init`: embedded templates, plan/apply, per-tool tests; the key verification is recorded in the init contract.
3. `new`, `list`, `tags`, `links`, `backlinks` on the slice-1 parser.
4. `tracking`: `gh` behind an interface; tests use a fake runner, one optional test uses real `gh`.
5. Optional: `--fix`. MCP is deferred until slices 1-4 are in use.

## Agent resources and manual

The root help carries the agent hint (subcommands plain, as the siblings); `llms.txt` and `docs/agent-guide.md` ship in slice 1 (layout of `sheetmill/`).
A separate small PR adds `docs/` and a Pages workflow modelled on `sheetmill/.github/workflows/pages.yml`
(SHA-pinned actions, pinned Hugo with checksum, checks before deploy, least privilege, concurrency `pages`).
Unlike `sheetmill`, `cscribe` has no site generator of its own, so the manual is plain Hugo over `docs/`.

## Verification

`make check` (gofmt, vet, tests) per PR. Optional `zk` compatibility test (skipped when `zk` is absent). No build or release targets; builds, tags, releases and the BundleDock entry need the owner's approval.
