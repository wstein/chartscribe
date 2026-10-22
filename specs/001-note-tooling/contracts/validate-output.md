# Contract: validate output

```text
cscribe [--root DIR] [--json] validate [--rule NAME]... [--check-sources [--git]]
cscribe lint ...                  (alias)
```

`--rule` limits the output to the named rules (the whole collection is still read).

## Human output

Findings on stderr (as `check_notes.py` does), one per line: `<note>: <message>`; collection-level findings without the prefix.
Summary on stdout: `validate: <N> notes, <M> findings`. No colour when not a terminal.

## JSON output (`--json`)

One document on stdout; nothing else on stdout; human text is suppressed.

```json
{
  "version": 1,
  "command": "validate",
  "ok": false,
  "notes": 64,
  "findings": [
    {"rule": "wikilink-unresolved", "note": "Network policy.md", "message": "wikilink [[Missing]] does not resolve to a note title or alias"}
  ]
}
```

- `findings` is always present, sorted by note then rule, without duplicates; `note` is `""` for collection-level findings.
- `ok` is true exactly when `findings` is empty.
- A usage or environment error under `--json` prints `{"version":1,"command":"validate","ok":false,"error":"<code>","message":"..."}` with `error` one of `usage`, `no-collection`, `io`.

## Source checks

`--check-sources` (or `check_sources = true` in `.cscribe.toml`) adds the file rules of [sources](sources.md) to the
findings; `--git` (or `check_stale = true`) adds `source-stale` warnings. `--git` without check-sources and
`--check-sources` or `--git` with `--fix` are usage errors. Warnings are not findings: with `--json` they are an additive
`"warnings": [{"rule","note","message"}]` array (present only when not empty) and `"skipped": ["<reason>", ...]` for what
the staleness check could not compare; `ok` and the exit status depend on `findings` only.

## Fix mode (`--fix`, `--apply`)

`validate --fix` plans and `validate --fix --apply` writes the fixes of [validate rules](validate-rules.md#fixes);
`--apply` without `--fix` is a usage error. `--rule` also limits the fixes to the named rules. Findings and the exit
status describe the collection as it is now (plan) or as it is left (apply). Human output adds on stdout, after the
summary: `fix: <K> edits in <F> files planned; findings <M> -> <A>; --apply writes them`, then per edit
`<file>:<line> [<rule>]` with `- <old line>` and `+ <new line>` lines; `fix: applied <K> edits in <F> files` with
`--apply`; `fix: nothing to fix`; `fix refused: <file>: <reason>`. With `--json` the document gets one more key:

```json
"fix": {"mode": "plan", "applied": false, "findings_before": 2, "findings_after": 0,
        "edits": [{"file": "notes/A.md", "line": 3, "rule": "tag-invalid", "old": "tags: [...]", "new": "tags: [...]"}],
        "refused": [{"file": "notes/B.md", "reason": "has CR line endings"}]}
```

`mode` is `plan` or `apply`; `findings_after` is the predicted count in plan mode and the real one after `--apply`.
`old` is empty for an inserted line. `--apply` verifies the whole result and checks every target (unchanged since the plan, regular file inside the collection,
directory writable) before the first write. A write error or a file that changed since the plan is an environment error
(`io`) whose message names the files written, restored (put back from the plan after a later write failed) and not written.

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | no findings |
| 1 | findings |
| 2 | usage error (unknown flag or rule name) |
| 3 | environment error (no collection found, unreadable directory) |
