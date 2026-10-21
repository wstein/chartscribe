# Tracking: issue links and output

`cscribe tracking` starts `gh issue list --state all --limit 200 --json number,title,state,url,body` once in the
working directory (the repository `gh` resolves). Getting 200 issues back is an error (some may be missing), not a
silent cut; there is no per-note search. `gh` missing, failing, not logged in or printing unreadable JSON exits 3.

**Link rule.** An issue refers to a note when a line of its title or of its body starts with
`Chartroom: <id>` (the 14-digit id, optional `-NNN`) followed by a space or the end of the line; the title after the id is
free text and is not compared. One issue may name several ids. Issues are listed by number.

**Section.** Per [validate rules](validate-rules.md#tracking-section) the section runs from `## Tracking` to the next
`## ` heading or the end of the file. Each issue is `- #<number> <title> (<state>) <url>` with the title on one line and
the state lowercase (`open`, `closed`). A note with issues gets the section replaced, or appended after a blank line;
a note without issues loses it. The result is stable: a second run changes nothing. A note with CRLF line ends or
unparsable frontmatter is not touched (CRLF: warning on stderr).

**Output.** Without `--write` only the plan is printed. Human: one line per changed note `path`, `add|update|remove`,
issue count (tab separated), then `tracking: <N> notes, <M> to change (dry run; --write applies)` (with `--write`:
`<M> changed`). `--json`: `{version: 1, command: "tracking", ok, write, notes, issues, changed, plan: [{id, title, path,
action, issues: [{number, title, state, url}]}]}`; errors use the document of [validate-output.md](validate-output.md).
Exit status: 0 done, 2 usage, 3 environment.
