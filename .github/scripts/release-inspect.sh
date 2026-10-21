#!/bin/sh
# SPDX-License-Identifier: EUPL-1.2
#
# Header sanity check of a cross-built binary that no runner executes (specs/002-release).
#
# Usage: sh release-inspect.sh GOOS GOARCH BINARY
#
# Reads the executable header with od and checks that the format matches GOOS (ELF for
# linux, the BSDs and illumos; Mach-O for darwin; PE for windows), the machine matches
# GOARCH, and that ELF class and byte order match the architecture. It does not run the
# binary. Exit 0 when the header fits, 1 when it does not, 2 on a usage error.

set -eu

fail() {
	printf 'release-inspect: %s\n' "$*" >&2
	exit 1
}

[ "$#" -eq 3 ] || {
	printf 'usage: sh release-inspect.sh GOOS GOARCH BINARY\n' >&2
	exit 2
}
goos=$1
goarch=$2
bin=$3
[ -f "$bin" ] || fail "binary not found: $bin"

# hex OFFSET COUNT: lowercase hex of COUNT bytes at OFFSET, no separators.
hex() {
	od -An -tx1 -v -j "$1" -N "$2" "$bin" | tr -d ' \n'
}

case $goos in
linux | freebsd | openbsd | netbsd | illumos)
	[ "$(hex 0 4)" = 7f454c46 ] || fail "$goos/$goarch: not an ELF file"
	# class (1 = 32-bit, 2 = 64-bit), data (1 = little, 2 = big endian), e_machine at 18
	case $goarch in
	amd64) want="02 01 3e00" ;;
	arm64) want="02 01 b700" ;;
	arm) want="01 01 2800" ;;
	riscv64) want="02 01 f300" ;;
	ppc64le) want="02 01 1500" ;;
	s390x) want="02 02 0016" ;;
	loong64) want="02 01 0201" ;;
	*) fail "unknown ELF architecture: $goarch" ;;
	esac
	got="$(hex 4 1) $(hex 5 1) $(hex 18 2)"
	[ "$got" = "$want" ] || fail "$goos/$goarch: ELF class/data/machine '$got', want '$want'"
	;;
darwin)
	[ "$(hex 0 4)" = cffaedfe ] || fail "darwin/$goarch: not a 64-bit Mach-O file"
	case $goarch in
	amd64) want=07000001 ;;
	arm64) want=0c000001 ;;
	*) fail "unknown Mach-O architecture: $goarch" ;;
	esac
	got=$(hex 4 4)
	[ "$got" = "$want" ] || fail "darwin/$goarch: Mach-O cpu type '$got', want '$want'"
	;;
windows)
	[ "$(hex 0 2)" = 4d5a ] || fail "windows/$goarch: not a PE file (no MZ)"
	le=$(hex 60 4)
	off=$(printf '%d' "0x$(printf '%s' "$le" | cut -c7-8)$(printf '%s' "$le" | cut -c5-6)$(printf '%s' "$le" | cut -c3-4)$(printf '%s' "$le" | cut -c1-2)")
	[ "$(hex "$off" 4)" = 50450000 ] || fail "windows/$goarch: no PE signature at offset $off"
	case $goarch in
	amd64) want=6486 ;;
	arm64) want=64aa ;;
	*) fail "unknown PE architecture: $goarch" ;;
	esac
	got=$(hex $((off + 4)) 2)
	[ "$got" = "$want" ] || fail "windows/$goarch: PE machine '$got', want '$want'"
	;;
*) fail "unknown GOOS: $goos" ;;
esac
printf 'release-inspect: %s/%s header ok\n' "$goos" "$goarch"
