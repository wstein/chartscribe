#!/bin/sh
# SPDX-License-Identifier: EUPL-1.2
#
# Release notes for a tag on stdout (specs/002-release). There is no CHANGELOG file: the
# change list is generated from the commit subjects.
#
# Usage: sh release-notes.sh TAG COMMIT GO_VERSION
#
# Run in a repository with full history and tags. The change list is the non-merge
# commit subjects between the nearest earlier strict release tag (by version order)
# and COMMIT, grouped by Conventional Commit type, or just "First release." when there is
# no earlier tag.

set -eu

[ "$#" -eq 3 ] || {
	printf 'usage: sh release-notes.sh TAG COMMIT GO_VERSION\n' >&2
	exit 2
}
tag=$1
commit=$2
gover=$3
plain=${tag#v}

prev=
if git rev-parse -q --verify "$commit^" >/dev/null; then
	prev=$(git tag --merged "$commit^" --list 'v*' --sort=-version:refname |
		grep -E '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$' | grep -vx "$tag" | head -n 1 || true)
fi

cat <<TEXT
ChartScribe $tag

Commit: $commit
Built with Go $gover, CGO disabled, -trimpath, version stamped as $tag.

Archives are named cscribe_${plain}_<os>_<arch>.tar.gz (.zip on Windows). Each holds
cscribe, LICENSE, README.md, llms.txt and agent-guide.md.

- Tier 1 (run natively on a GitHub runner before release): linux/amd64, linux/arm64,
  darwin/arm64, darwin/amd64, windows/amd64, windows/arm64.
- Raspberry Pi (32-bit OS; run under qemu-user before release): linux_armv7 (Pi 2 and
  later) and linux_armv6 (Pi 1 and Zero). Pi 3, 4 and 5 with a 64-bit OS use linux_arm64.
- Tier 2 (cross-built, reproducible, header-checked, not run): freebsd, openbsd and netbsd
  on amd64 and arm64; linux on riscv64, ppc64le, s390x and loong64; illumos/amd64.

Verify a download in the directory that holds the archives and SHA256SUMS:

    sha256sum --check --ignore-missing SHA256SUMS
    # macOS, for one archive: grep ' ARCHIVE\$' SHA256SUMS | shasum -a 256 --check
    gh attestation verify ARCHIVE --repo wstein/chartscribe

then extract and run \`cscribe version\`, which prints \`cscribe $tag\`.

The binaries are not signed with a certificate or notarized. The build provenance
attestation shows that GitHub Actions built the archive from this repository;
SHA256SUMS detects a damaged or wrong download.

TEXT
if [ -z "$prev" ]; then
	printf '## Changes\n\nFirst release.\n'
	exit 0
fi
printf '## Changes since %s\n\n' "$prev"
range=$prev..$commit
subjects=$(git log --no-merges --format='%s (%h)' "$range")
if [ -z "$subjects" ]; then
	printf 'No changes.\n'
	exit 0
fi
section() { # section TITLE ERE
	lines=$(printf '%s\n' "$subjects" | grep -E "$2" || true)
	if [ -n "$lines" ]; then
		printf '### %s\n\n' "$1"
		printf '%s\n' "$lines" | sed 's/^/- /'
		printf '\n'
	fi
}
section Features '^feat(\(.+\))?!?:'
section Fixes '^fix(\(.+\))?!?:'
section Documentation '^docs(\(.+\))?!?:'
rest=$(printf '%s\n' "$subjects" | grep -Ev '^(feat|fix|docs)(\(.+\))?!?:' || true)
if [ -n "$rest" ]; then
	printf '### Other\n\n'
	printf '%s\n' "$rest" | sed 's/^/- /'
fi
