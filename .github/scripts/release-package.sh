#!/bin/sh
# SPDX-License-Identifier: EUPL-1.2
#
# Package one release binary (specs/002-release).
#
# Usage: SOURCE_DATE_EPOCH=N sh release-package.sh VERSION TARGET BINARY OUTDIR
#
# TARGET is the archive platform: <os>_<arch>, where the arm port is split by GOARM into
# linux_armv6 and linux_armv7. Run in the repository root (LICENSE, README.md, llms.txt and
# docs/agent-guide.md are read from there). Writes
# OUTDIR/cscribe_<version without v>_<target>.tar.gz (.zip for windows) and its .sha256
# sidecar. The archive holds one top directory with cscribe (cscribe.exe on windows, 0755),
# LICENSE, README.md, llms.txt and agent-guide.md (0644), in sorted order, owner 0, mtime
# SOURCE_DATE_EPOCH, no gzip name or time, so the same inputs always give the same bytes.
# tar.gz needs GNU tar; zip needs Info-ZIP zip.

set -eu

fail() {
	printf 'release-package: %s\n' "$*" >&2
	exit 1
}

[ "$#" -eq 4 ] || {
	printf 'usage: SOURCE_DATE_EPOCH=N sh release-package.sh VERSION TARGET BINARY OUTDIR\n' >&2
	exit 2
}
version=$1
target=$2
binary=$3
out=$4

case $version$target in
*'
'*) fail "newline in an argument" ;;
esac
printf '%s\n' "$version" | grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.]+)?$' ||
	fail "version must look like v1.2.3 or v1.2.3-dryrun: $version"
printf '%s\n' "$target" | grep -Eq '^[a-z0-9]+_[a-z0-9]+$' || fail "bad target (want os_arch): $target"
: "${SOURCE_DATE_EPOCH:?release-package: SOURCE_DATE_EPOCH is required}"
printf '%s\n' "$SOURCE_DATE_EPOCH" | grep -Eq '^[0-9]+$' || fail "SOURCE_DATE_EPOCH must be digits"
[ -f "$binary" ] || fail "binary not found: $binary"
for f in LICENSE README.md llms.txt docs/agent-guide.md; do
	[ -f "$f" ] || fail "$f not found in $(pwd)"
done

exe=cscribe
ext=tar.gz
case $target in
windows_*)
	exe=cscribe.exe
	ext=zip
	command -v zip >/dev/null 2>&1 || fail "zip is required for windows archives"
	;;
*) tar --version 2>/dev/null | grep -q 'GNU tar' || fail "GNU tar is required" ;;
esac

name=cscribe_${version#v}_${target}
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

mkdir "$work/$name"
chmod 0755 "$work/$name"
cp "$binary" "$work/$name/$exe"
cp LICENSE README.md llms.txt "$work/$name/"
cp docs/agent-guide.md "$work/$name/agent-guide.md"
chmod 0755 "$work/$name/$exe"
chmod 0644 "$work/$name/LICENSE" "$work/$name/README.md" "$work/$name/llms.txt" "$work/$name/agent-guide.md"

mkdir -p "$out"
out=$(cd "$out" && pwd)
archive=$out/$name.$ext
rm -f "$archive"
if [ "$ext" = zip ]; then
	# Info-ZIP stores local time with 2 s resolution: pin the zone and every mtime.
	(
		cd "$work"
		for f in "$name" "$name"/*; do touch -d "@$SOURCE_DATE_EPOCH" "$f"; done
		{ printf '%s/\n' "$name"; ls -1 "$name" | LC_ALL=C sort | sed "s|^|$name/|"; } |
			TZ=UTC zip -X -q -9 "$archive" -@
	)
else
	tar --sort=name --owner=0 --group=0 --numeric-owner --mtime="@$SOURCE_DATE_EPOCH" \
		--format=ustar -C "$work" -cf - "$name" | gzip -n -9 >"$archive"
fi
(cd "$out" && sha256sum "$name.$ext" >"$name.$ext.sha256")
printf '%s\n' "$archive"
