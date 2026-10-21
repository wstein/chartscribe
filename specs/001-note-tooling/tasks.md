# Tasks: Note tooling

## FR trace

| FR | Tasks |
| --- | --- |
| 001 | T102-T109 |
| 002 | T101b |
| 003 | T104 |
| 004 | T110 |
| 005 | T109a |
| 006, 007 | T202-T204 |
| 008 | T301 |
| 009 | T302, T303 |
| 010 | T401, T401a, T402, T403 |
| 011 | T205, T206 |
| 012 | T101c |
| 013 | T101 |
| 014 | T101a |
| 015 | T601, T602 |

TDD order: in each group the test task comes first and fails before its implementation task.
`[P]` marks tasks that can run in parallel.

## Slice 1 - validate

- [x] T101 Test: `internal/cli.Run(ctx, version, args, stdout, stderr)` with `version` and an unknown command (exit 2) on a cobra root, one fresh tree per call; add cobra v1.10.2; decide the TOML library.
- [x] T101a Test: the agent hint appears in the root help only; subcommand help stays plain (walk the command tree). Implement the hint. Write `llms.txt` and `docs/agent-guide.md` after `sheetmill/`.
- [x] T101c Test (FR-012): fails when `go.mod` or an import names a `zk-org` module or code starts a `zk` process (scan sources for `exec.Command("zk"`).
- [x] T101b Test: root discovery (FR-002): nearest directory with `notes/` from a subdirectory, `--root`, none found gives exit 3.
- [x] T102 Test: frontmatter parser table (flow list, block list, quoted, plain, single-quoted, comment, `[]`, unterminated, bad line). Implement `internal/note` parser.
- [x] T103 Test: id rules (14 digits, `-NNN`, calendar date, missing, duplicate). Implement.
- [x] T104 [P] Test and implement `field-unknown`, `list-type`, `tag-invalid`, `tag-count` (map without `status/`), vocabulary from `.cscribe.toml`.
- [x] T105 [P] Test and implement `title-char`, `name-duplicate` (case-insensitive, alias against title), `source-missing`.
- [x] T106 Test and implement `wikilink-unresolved` (title, alias, label, heading forms).
- [x] T107 Test and implement the map rules: `map-unlisted`, `map-multiple`, `map-category`, `map-duplicate`, `category-no-map`.
- [x] T108 [P] Test and implement `readme-link` (broken, outside the collection, external ignored).
- [x] T109 Test: the committed chartroom snapshot gives 0 findings; the five mutation probes each give one finding; compare with `check_notes.py` output on every failure fixture.
- [x] T109a Test (FR-005): validate on a read-only copy leaves the tree hash unchanged; `go list -deps ./internal/validate` contains neither `net` nor `os/exec`.
- [x] T110 Test (golden): human and JSON output, `--rule`, exit codes 0/1/2/3, `lint` alias. Implement `validate`.
- [x] T111 Docs: README command section, chartroom-check usage. (The umbrella switch is a separate PR there.)

## Slice 2 - init

- [x] T201 Verified online on 2026-10-21 (sources and gaps in [contracts/init-files.md](contracts/init-files.md#verification)); the contract is corrected where it differed (zk `exclude`, not `ignore`).
- [ ] T202 Test: plan lists every file as `write` in an empty directory and writes nothing. Implement plan.
- [ ] T203 Test: `--apply` writes the missing files; a rerun plans nothing; a pre-existing file stays byte-identical; `--overwrite PATH` replaces only that file. Implement.
- [ ] T204 [P] Per-tool tests (`--tool`), `--json` output, no file outside the root.
- [ ] T205 Test: a note made from `templates/Note.md` (placeholders substituted) and one from `.zk/templates/note.md` pass `validate` after filling tags and `Source:`.
- [ ] T206 Optional CI test: with `zk` installed, `zk list --format json` and `zk graph --format json` on a generated collection find all notes and wikilinks; skipped otherwise.

## Slice 3 - new, list, tags, links, backlinks

- [x] T301 Test and implement `new` (filename = title, id from an injected clock, `-NNN` on collision, refuses existing file and forbidden characters).
- [x] T302 [P] Test and implement `list` (tag filter, JSON record `id`, `title`, `path`, `tags`) and `tags`.
- [x] T303 [P] Test and implement `links` and `backlinks` (alias resolution, unresolved targets reported).
- [x] T304 Golden files for JSON output; update README.

## Slice 4 - tracking

- [x] T401 Test: parse `gh issue list --json` output (fixtures); the `Chartroom: <id> <title>` link rule.
- [x] T401a Test: the query uses `--state all --limit 200`; 200 results is an error; `## Tracking` lines `- #<n> <title> (<state>) <url>`.
- [x] T402 Test: rewrite only the `## Tracking` section, body bytes elsewhere identical (hash); dry-run default; `--write`.
- [x] T403 Test: `gh` missing or failing gives exit 3 with a clear message. Implement with a runner interface.

## Pages (separate PR)

- [ ] T601 `docs/index.md` and a Hugo site configuration over `docs/`; local check by the owner (builds need approval).
- [ ] T602 `.github/workflows/pages.yml` modelled on `sheetmill`: SHA-pinned actions, Hugo pinned with checksum, gofmt/vet/test before deploy, deploy on `main` only, least-privilege permissions, concurrency `pages`.

## Slice 5 - optional (MCP deferred until slices 1-4 are in use)

- [x] T501 `validate --fix` for safe findings (plan by default), body hash unchanged.
