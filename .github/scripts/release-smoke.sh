#!/bin/sh
# SPDX-License-Identifier: EUPL-1.2
#
# Exercise a shipped release archive on its host (specs/002-release).
#
# Usage: [CSCRIBE_RUN='qemu-arm -cpu arm1176'] sh release-smoke.sh VERSION ARCHIVE
#
# ARCHIVE is cscribe_<version>_<os>_<arch>.tar.gz or .zip next to its .sha256 sidecar.
# CSCRIBE_RUN is an optional emulator prefix (word-split) for archives that the host cannot
# execute natively. Checks, in order: the checksum; the archive members; that `cscribe
# version` prints "cscribe VERSION"; `--help`; then, in a temporary collection, `init` (plan,
# --apply, idempotent rerun), `new` (three notes), `list`, `links`, `validate` (0 findings
# once the script has filled the tags, links, Source lines and the map) and `export` of the map
# to a file (the title, a note section and the Sources section; a second export is
# byte-identical). Works with POSIX tools on Linux, macOS and Git Bash on Windows; a zip is
# extracted with PowerShell Expand-Archive when it exists, else with unzip.

set -eu

fail() {
	printf 'release-smoke: %s\n' "$*" >&2
	exit 1
}

[ "$#" -eq 2 ] || {
	printf 'usage: sh release-smoke.sh VERSION ARCHIVE\n' >&2
	exit 2
}
version=$1
archive=$2

[ -f "$archive" ] || fail "archive not found: $archive"
archive=$(cd "$(dirname "$archive")" && pwd)/$(basename "$archive")
file=$(basename "$archive")
case $file in
*.tar.gz) name=${file%.tar.gz} exe=cscribe ;;
*.zip) name=${file%.zip} exe=cscribe.exe ;;
*) fail "not a .tar.gz or .zip: $file" ;;
esac

# Checksum written at build time (sha256sum on Linux and Windows, shasum on macOS).
[ -f "$archive.sha256" ] || fail "checksum sidecar missing: $file.sha256"
if command -v sha256sum >/dev/null 2>&1; then
	(cd "$(dirname "$archive")" && sha256sum -c "$file.sha256" >/dev/null) || fail "checksum mismatch for $file"
else
	(cd "$(dirname "$archive")" && shasum -a 256 -c "$file.sha256" >/dev/null) || fail "checksum mismatch for $file"
fi

# Physical path: macOS TMPDIR is under /var -> /private/var.
d=$(mktemp -d) || fail "cannot create a temp dir"
trap 'rm -rf "$d"' EXIT
tmp=$(cd "$d" && pwd -P) || fail "cannot resolve the temp dir"
case $file in
*.tar.gz) tar -xzf "$archive" -C "$tmp" || fail "cannot extract $file" ;;
*.zip)
	if command -v powershell.exe >/dev/null 2>&1; then
		zipw=$archive
		tmpw=$tmp
		if command -v cygpath >/dev/null 2>&1; then
			zipw=$(cygpath -w "$archive")
			tmpw=$(cygpath -w "$tmp")
		fi
		powershell.exe -NoProfile -NonInteractive -Command "Expand-Archive -LiteralPath '$zipw' -DestinationPath '$tmpw'" || fail "cannot extract $file"
	else
		unzip -q "$archive" -d "$tmp" || fail "cannot extract $file"
	fi
	;;
esac

members=$(cd "$tmp" && find . -mindepth 1 | LC_ALL=C sort | tr '\n' ' ')
expected=$(printf '%s\n' "./$name" "./$name/$exe" ./$name/LICENSE ./$name/README.md ./$name/agent-guide.md ./$name/llms.txt | LC_ALL=C sort | tr '\n' ' ')
[ "$members" = "$expected" ] || fail "unexpected archive members: $members"
bin=$tmp/$name/$exe
[ -x "$bin" ] || fail "$exe is not executable"

# cs runs the shipped binary, through the emulator prefix when there is one.
cs() {
	# shellcheck disable=SC2086
	${CSCRIBE_RUN:-} "$bin" "$@"
}

got=$(cs version) || fail "cscribe version failed"
got=$(printf '%s' "$got" | tr -d '\r')
[ "$got" = "cscribe $version" ] || fail "cscribe version printed '$got', want 'cscribe $version'"
cs --help >/dev/null || fail "cscribe --help failed"

root=$tmp/collection
mkdir -p "$root"
printf '# fixture\n' >"$root/README.md"

cs --root "$root" init >/dev/null || fail "init (plan) failed"
[ ! -e "$root/.cscribe.toml" ] || fail "init without --apply wrote files"
cs --root "$root" init --apply >/dev/null || fail "init --apply failed"
[ -f "$root/.cscribe.toml" ] || fail "init --apply did not write .cscribe.toml"
again=$(cs --root "$root" init --apply) || fail "init --apply rerun failed"
printf '%s\n' "$again" | grep -q 'nothing to write' || fail "init --apply rerun was not idempotent: $again"
grep -qF "$version" "$root/.github/workflows/pages.yml" || fail "the generated Pages workflow does not pin $version"

tags=type/idea,status/open,category/direction
cs --root "$root" new "First note" --tag "$tags" >/dev/null || fail "new (first note) failed"
cs --root "$root" new "Second note" --tag "$tags" >/dev/null || fail "new (second note) failed"
cs --root "$root" new "Map - Direction" --tag type/map,category/direction >/dev/null || fail "new (map) failed"
# fill NAME: keep the five frontmatter lines that new wrote and append the body from stdin.
fill() {
	{ sed -n 1,5p "$root/notes/$1.md"; cat; } >"$root/notes/$1.tmp" && mv "$root/notes/$1.tmp" "$root/notes/$1.md" ||
		fail "cannot fill $1"
}
fill "First note" <<'NOTE'

Body of the first note.

## Links

- [[Second note]]

Source: SMOKE.md, lines 1-2
NOTE
fill "Second note" <<'NOTE'

Body of the second note.

## Links

- [[First note]]

Source: SMOKE.md, lines 3-4
NOTE
fill "Map - Direction" <<'NOTE'

Map of the direction notes.

- [[First note]]: one
- [[Second note]]: two
NOTE

[ "$(cs --root "$root" list | wc -l | tr -d ' ')" = 3 ] || fail "list did not show three notes"
cs --root "$root" list --tag type/map | grep -q 'Map - Direction' || fail "list --tag type/map did not show the map"
cs --root "$root" links "First note" | grep -q 'Second note' || fail "links did not resolve [[Second note]]"
cs --root "$root" validate >/dev/null || fail "validate reported findings on the filled fixture"
cs --root "$root" --json validate | tr -d ' \r\n' | grep -q '"ok":true' || fail "validate --json did not report ok"

cs --root "$root" export "Map - Direction" --out "$tmp/export-a.md" >/dev/null || fail "first export failed"
cs --root "$root" export "Map - Direction" --out "$tmp/export-b.md" >/dev/null || fail "second export failed"
grep -q '^# Map - Direction' "$tmp/export-a.md" || fail "export lacks the document title"
grep -q '^## First note' "$tmp/export-a.md" || fail "export lacks the First note section"
grep -q '^## Sources' "$tmp/export-a.md" || fail "export lacks the Sources section"
cmp "$tmp/export-a.md" "$tmp/export-b.md" >/dev/null || fail "export is not deterministic: two runs differ"

printf 'x\n' >"$root/notes/Broken.md"
if cs --root "$root" validate >/dev/null 2>&1; then
	fail "validate accepted a broken note"
fi
printf 'release-smoke: %s ok\n' "$file"
