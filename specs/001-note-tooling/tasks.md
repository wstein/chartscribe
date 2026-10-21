# Tasks: Note tooling

TDD order: in each group the test task comes first and fails before its implementation task.
`[P]` marks tasks that can run in parallel.

## Slice 1 - validate

- [ ] T101 Test: `internal/cli.Run` with `version` and an unknown command (exit 2); decide the TOML library.
- [ ] T102 Test: frontmatter parser table (flow list, block list, quoted, plain, single-quoted, comment, `[]`, unterminated, bad line). Implement `internal/note` parser.
- [ ] T103 Test: id rules (14 digits, `-NNN`, calendar date, missing, duplicate). Implement.
- [ ] T104 [P] Test and implement `field-unknown`, `list-type`, `tag-invalid`, `tag-count` (map without `status/`), vocabulary from `.cscribe.toml`.
- [ ] T105 [P] Test and implement `title-char`, `name-duplicate` (case-insensitive, alias against title), `source-missing`.
- [ ] T106 Test and implement `wikilink-unresolved` (title, alias, label, heading forms).
- [ ] T107 Test and implement the map rules: `map-unlisted`, `map-multiple`, `map-category`, `map-duplicate`, `category-no-map`.
- [ ] T108 [P] Test and implement `readme-link` (broken, outside the collection, external ignored).
- [ ] T109 Test: the committed chartroom snapshot gives 0 findings; the five mutation probes each give one finding; compare with `check_notes.py` output on every failure fixture.
- [ ] T110 Test (golden): human and JSON output, `--rule`, exit codes 0/1/2/3, `lint` alias. Implement `validate`.
- [ ] T111 Docs: README command section, chartroom-check usage. (The umbrella switch is a separate PR there.)

## Slice 2 - init

- [ ] T201 Verify by hand in Obsidian and with zk 0.15.7 every key of [contracts/init-files.md](contracts/init-files.md); record results in the contract; fix the proposal where wrong.
- [ ] T202 Test: plan lists every file as `write` in an empty directory and writes nothing. Implement plan.
- [ ] T203 Test: `--apply` writes the missing files; a rerun plans nothing; a pre-existing file stays byte-identical; `--overwrite PATH` replaces only that file. Implement.
- [ ] T204 [P] Per-tool tests (`--tool`), `--json` output, no file outside the root.
- [ ] T205 Test: a note made from `templates/Note.md` (placeholders substituted) and one from `.zk/templates/note.md` pass `validate` after filling tags and `Source:`.
- [ ] T206 Optional CI test: with `zk` installed, `zk list --format json` and `zk graph --format json` on a generated collection find all notes and wikilinks; skipped otherwise.

## Slice 3 - new, list, tags, links, backlinks

- [ ] T301 Test and implement `new` (filename = title, id from an injected clock, `-NNN` on collision, refuses existing file and forbidden characters).
- [ ] T302 [P] Test and implement `list` (tag filter, JSON record `id`, `title`, `path`, `tags`) and `tags`.
- [ ] T303 [P] Test and implement `links` and `backlinks` (alias resolution, unresolved targets reported).
- [ ] T304 Golden files for JSON output; update README.

## Slice 4 - tracking

- [ ] T401 Test: parse `gh issue list --json` output (fixtures); the `Chartroom: <id> <title>` link rule.
- [ ] T402 Test: rewrite only the `## Tracking` section, body bytes elsewhere identical (hash); dry-run default; `--write`.
- [ ] T403 Test: `gh` missing or failing gives exit 3 with a clear message. Implement with a runner interface.

## Slice 5 - optional

- [ ] T501 `validate --fix` for safe findings (plan by default), body hash unchanged.
- [ ] T502 MCP server over stdio with the read commands.
