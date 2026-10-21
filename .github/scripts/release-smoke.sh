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
# version` prints "cscribe VERSION"; `--help`; and, in a temporary collection, that `init
# --apply` writes files and a rerun writes none, that `validate` accepts a two-note fixture and
# `validate --json` reports ok, and that a broken note makes `validate` exit 1. The commands
# `new`, `list` and `export` do not exist yet; add them here when they ship. Works with POSIX
# tools on Linux, macOS and Git Bash on Windows.

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
	if command -v unzip >/dev/null 2>&1; then
		unzip -q "$archive" -d "$tmp" || fail "cannot extract $file"
	elif command -v 7z >/dev/null 2>&1; then
		7z x -y "-o$tmp" "$archive" >/dev/null || fail "cannot extract $file"
	else
		fail "no unzip or 7z to extract $file"
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
mkdir -p "$root/notes"
printf '# fixture\n' >"$root/README.md"
cat >"$root/notes/First note.md" <<'NOTE'
---
id: 20261021120000
tags: ["type/idea", "status/open", "category/direction"]
---
Body text.

## Links

- [[Map - Direction]]

Source: SMOKE.md, lines 1-2
NOTE
cat >"$root/notes/Map - Direction.md" <<'NOTE'
---
id: 20261021120100
tags: ["type/map", "category/direction"]
---
Map of the direction notes.

- [[First note]]: summary
NOTE

cs --root "$root" init >/dev/null || fail "init (plan) failed"
[ ! -e "$root/.cscribe.toml" ] || fail "init without --apply wrote files"
cs --root "$root" init --apply >/dev/null || fail "init --apply failed"
[ -f "$root/.cscribe.toml" ] || fail "init --apply did not write .cscribe.toml"
again=$(cs --root "$root" init --apply) || fail "init --apply rerun failed"
printf '%s\n' "$again" | grep -q 'nothing to write' || fail "init --apply rerun was not idempotent: $again"
grep -qF "$version" "$root/.github/workflows/pages.yml" || fail "the generated Pages workflow does not pin $version"
cs --root "$root" validate >/dev/null || fail "validate rejected the fixture"
cs --root "$root" --json validate | tr -d ' \r\n' | grep -q '"ok":true' || fail "validate --json did not report ok"
printf 'x\n' >"$root/notes/Broken.md"
if cs --root "$root" validate >/dev/null 2>&1; then
	fail "validate accepted a broken note"
fi
printf 'release-smoke: %s ok\n' "$file"
