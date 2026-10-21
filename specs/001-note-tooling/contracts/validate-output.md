# Contract: validate output

```text
cscribe [--root DIR] [--json] validate [--rule NAME]...
cscribe lint ...                  (alias)
```

`--rule` limits the output to the named rules (the whole collection is still read).

## Human output

Findings on stderr, one per line: `<note>: <message>`; collection-level findings without the prefix.
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

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | no findings |
| 1 | findings |
| 2 | usage error (unknown flag or rule name) |
| 3 | environment error (no collection found, unreadable directory) |
