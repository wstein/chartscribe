# Plan: 002 release pipeline (v0.1.0)

Status: proposed, modelled on SheetMill's feature 019. Nothing here has been run: the first
run is the owner's dry run.

## Decisions

- An owner-pushed annotated tag `vMAJOR.MINOR.PATCH` on a commit of `main` is the publication
  approval for that tag only. A manual `workflow_dispatch` is a dry run and never publishes.
  There is no standing approval; every release needs its own tag push by the owner.
- Jobs: `verify` -> `build` (all targets) -> `smoke` (tier 1 native) and `smoke-emulated`
  (every Linux archive that is not native, under qemu-user) -> `publish` (tag push only, environment `release`).
- verify: version and tag format, the run was started by the repository owner, annotated tag
  on `main` naming the run's commit, a green `ci` push run for that commit, no release for
  the tag, `.go-version` equals `go.mod`, `make check`, `go mod verify`.
- build: `CGO_ENABLED=0`, `-trimpath`, `GOTOOLCHAIN=local`, `-mod=readonly`, version stamped
  with `-X .../internal/cli.Version=<tag>` (`cscribe version` prints `cscribe v0.1.0`; an
  unstamped build prints `cscribe dev`). Built twice with separate empty caches and compared
  byte-for-byte; the header is checked by `release-inspect.sh`; then packaged.
- Archives `cscribe_<version without v>_<os>_<arch>.tar.gz` (`.zip` for windows) with
  `cscribe[.exe]`, `LICENSE`, `README.md`, `llms.txt`, `agent-guide.md`, sorted, owner 0,
  mtime `SOURCE_DATE_EPOCH` (commit time), plus a `.sha256` sidecar.
  The arm port is named by GOARM: `linux_armv6` (GOARM=6), `linux_armv7` (GOARM=7);
  `arm64` stays `arm64`.
- publish: re-checks owner and actor, no release or draft for the tag, the remote tag still
  names the verified commit; writes `SHA256SUMS` over all archives; attests build provenance
  (`actions/attest-build-provenance`, pinned; `attestations: write` and `id-token: write` only
  in this job); `gh release create --verify-tag`; draft when the repository variable
  `CHARTSCRIBE_RELEASE_DRAFT` is `true`. Release notes come from `release-notes.sh`
  (Conventional Commit subjects since the previous release tag; no CHANGELOG file).
- All actions are pinned by SHA with a version comment; each job has explicit least-privilege
  permissions and a timeout; the concurrency group never cancels a release.
- No certificate signing and no notarization (README documents the Gatekeeper and SmartScreen
  consequence). No SBOM: the pinned toolchain has no trivial generator, and every binary
  already embeds its module list (`go version -m cscribe`). Adding a pinned
  `cyclonedx-gomod` step is a later, separate decision.
- `make` gains no build targets. The scripts are tested offline by `go test ./internal/releaseworkflow`
  (part of `make check`): script behaviour on temporary git repos and a real host build, plus
  text rules on `release.yml` (pins, permissions, matrix equals the table below, archive count).
- The smoke runs the shipped archive on every smoke platform (native and qemu): checksum and
  members, `version`, `--help`, `init` (plan, `--apply`, idempotent rerun), `new` (two notes and
  a map), `list`, `links`, `validate` (0 findings after the script fills tags, links, Source
  lines and the map), `export` of the map to a file twice (title, a note section, Sources;
  byte-identical) and a broken note failing with exit 1. The BSD and illumos archives cannot run
  under qemu-user (not Linux system calls) and get build, reproducibility and header check only.
  The qemu job exists only in the release workflow (tag or dispatch), never on commits or pull
  requests.

## Platform matrix (Go 1.27.2 from `.go-version`; verified with `go tool dist list`)

| Tier | Targets | Gate |
| --- | --- | --- |
| 1, native smoke | linux/amd64 (ubuntu-24.04), linux/arm64 (ubuntu-24.04-arm), darwin/arm64 (macos-15), darwin/amd64 (macos-15-intel), windows/amd64 (windows-2025), windows/arm64 (windows-11-arm) | build, reproducible, header check, shipped archive run |
| 1, emulated smoke | linux/arm GOARM=7 `linux_armv7` (Pi 2 and later, 32-bit OS), linux/arm GOARM=6 `linux_armv6` (Pi 1, Zero) | same, run under `qemu-arm` (`cortex-a15` / `arm1176`) on ubuntu-24.04; the arm64 runner is not assumed to run AArch32 |
| 2, emulated smoke | linux/riscv64 (`qemu-riscv64`), linux/ppc64le (`qemu-ppc64le -cpu power9`), linux/s390x (`qemu-s390x -cpu max`), linux/loong64 (`qemu-loongarch64`) | same, qemu-user of the Ubuntu 24.04 archive (qemu 8.2.2 ships all four binaries; the same smoke also passed locally with qemu 10) |
| 2, build only | freebsd/amd64, freebsd/arm64, openbsd/amd64, openbsd/arm64, netbsd/amd64, netbsd/arm64, illumos/amd64 | build, reproducible, header check |

19 archives; the publish job requires exactly 19. Other 32-bit targets are not shipped (the
owner asked for 64-bit, plus Raspberry Pi). SPARC is not supported by Go any more.
Follow-up after the first dry run (not in this change): FreeBSD, OpenBSD, NetBSD and illumos
amd64 could be smoke-tested in KVM virtual machines through a SHA-pinned cross-platform VM
action; the owner approves each third-party action first. The arm64 BSDs stay build-only.
All targets gate the release: one failed build or smoke stops publication.

## Owner setup (not done by this change)

1. Merge the PR: a tag push runs the workflow file of the tagged commit, so it must be on `main`.
2. Create the `release` environment (Settings, Environments) if absent; add required
   reviewers if wanted. Optional: variable `CHARTSCRIBE_RELEASE_DRAFT=true`.
3. Optional: a tag ruleset for `v*` (no update, no deletion) after the release exists.
4. Confirm the runner labels exist for this public repository (`macos-15-intel`,
   `windows-11-arm`, `ubuntu-24.04-arm`); a missing label fails that smoke job and blocks
   publication, which is the intended fail-closed behaviour.

## Release v0.1.0 (owner only)

1. Pick the `main` commit `<sha>` whose `ci` push run is green.
2. Dry run: Actions, release, Run workflow on `main` (default label `v0.0.0-dryrun`). Expect
   verify (without the tag-only steps), 19 builds, 6 native and 6 emulated smokes, and no publish. Fix and re-run
   until green; a dry run publishes nothing.
3. Tag and push:

   ```sh
   git fetch origin main
   git tag -a v0.1.0 -m "ChartScribe v0.1.0" <sha>
   git push origin v0.1.0
   ```

4. The workflow verifies, builds, smokes and publishes the release (a draft when the
   variable is set; publish it by hand then).
5. Check: the release has 19 archives and `SHA256SUMS`; download one, then
   `sha256sum --check --ignore-missing SHA256SUMS` (macOS: `grep ' <archive>$' SHA256SUMS | shasum -a 256 --check`), `gh attestation verify <archive> --repo wstein/chartscribe`,
   and `cscribe version` prints `cscribe v0.1.0`; `go install github.com/wstein/chartscribe/cmd/cscribe@v0.1.0`
   works once the module proxy has the tag.

Scripts and failure hints: [docs/releasing.md](../../docs/releasing.md).
