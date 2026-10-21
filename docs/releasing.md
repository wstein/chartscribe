---
title: "Releasing"
description: "How a ChartScribe release is built, checked and published, and what each release script does."
---
Releases are made by CI from an owner-pushed annotated tag `vMAJOR.MINOR.PATCH`; the design and the
owner steps are in
[specs/002-release/plan.md](https://github.com/wstein/chartscribe/blob/main/specs/002-release/plan.md).
Agents and contributors never create tags, run the workflow or create releases.

The scripts live in `.github/scripts`, are POSIX `sh` and run from the repository root. They read
local files and git objects only (no network, no writes outside their output directory).

| Script | Use |
| --- | --- |
| `release-verify.sh version V`, `dryversion V`, `tag TAG MAIN_REF`, `go` | strict version, dry-run label, annotated tag on `main` (prints the commit), `.go-version` equals `go.mod` |
| `release-package.sh VERSION TARGET BINARY OUTDIR` | deterministic `.tar.gz` (GNU tar) or `.zip` (Info-ZIP) plus `.sha256`; needs `SOURCE_DATE_EPOCH`; `TARGET` is `<os>_<arch>`, with `linux_armv6` and `linux_armv7` for 32-bit ARM |
| `release-inspect.sh GOOS GOARCH BINARY` | ELF, Mach-O or PE header matches the target, without running the binary |
| `release-smoke.sh VERSION ARCHIVE` | checksum, members, `version`, `--help`, `init`, `validate` on a fixture; `CSCRIBE_RUN` prefixes an emulator |
| `release-notes.sh TAG COMMIT GO_VERSION` | release notes on stdout from Conventional Commit subjects since the previous release tag |

`go test ./internal/releaseworkflow` (part of `make check`) tests the scripts and the structure of
`.github/workflows/release.yml`. A test whose tool is missing is reported as skipped, not passed.

A failed `smoke-emulated` job usually means qemu rejected a CPU model or the Go runtime found a
feature the model lacks; reproduce with `CSCRIBE_RUN='qemu-arm -cpu arm1176' sh .github/scripts/release-smoke.sh ...`.
