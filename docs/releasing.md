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
| `release-smoke.sh VERSION ARCHIVE` | checksum, members, `version`, `--help`, `init`, `new`, `list`, `links`, `validate` and a twice-run `export` on a fixture collection; `CSCRIBE_RUN` prefixes an emulator; zips are extracted with PowerShell `Expand-Archive` |
| `release-notes.sh TAG COMMIT GO_VERSION` | release notes on stdout from Conventional Commit subjects since the previous release tag |

`go test ./internal/releaseworkflow` (part of `make check`) tests the scripts and the structure of
`.github/workflows/release.yml`. A test whose tool is missing is reported as skipped, not passed.

A failed `smoke-emulated` job usually means qemu rejected a CPU model or the Go runtime found a
feature the model lacks; reproduce with `CSCRIBE_RUN='qemu-arm -cpu arm1176' sh .github/scripts/release-smoke.sh ...`.

## Verifying a download

```sh
sha256sum --check --ignore-missing SHA256SUMS                       # Linux
grep ' ARCHIVE$' SHA256SUMS | shasum -a 256 --check                 # macOS, one archive
gh attestation verify ARCHIVE --repo wstein/chartscribe
```

## The qemu smoke gate

`smoke-emulated` (the Raspberry Pi and other non-native Linux archives under qemu-user) runs only in this workflow, never on commits
or pull requests, and `publish` lists it in `needs`. To demote it after the first dry run, if the
emulation proves flaky, replace the `if:` of `publish` by
`!cancelled() && needs.verify.result == 'success' && needs.build.result == 'success' && needs.smoke.result == 'success' && <the existing condition>`
so that a failed `smoke-emulated` no longer blocks publication. The owner decides; the structure
test `TestPublishGatesOnTheSmokeJobs` must then be changed in the same pull request.

## v0.1.0 cannot be fetched with `go install`

The `v0.1.0` tag holds test fixtures named with an em dash (`M0 — ...md`). Go module zips accept only
letters, digits and `!#$%&()+,-.=@[]^_{}~` and the space in file names (`module.CheckFilePath`), so the
Go proxy cannot build the zip and `go install ...@v0.1.0` fails with "malformed file path". Tags are
permanent, so v0.1.0 stays unfetchable through the proxy; its release archives are fine. The fixtures are
now stored percent-encoded (`%E2%80%94`) and decoded at test time, and `internal/policy` fails if any tracked
path would break a module zip or a Windows checkout. `go install ...@latest` works from v0.1.1.
