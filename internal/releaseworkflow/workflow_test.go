// SPDX-License-Identifier: EUPL-1.2

package releaseworkflow

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The workflow is checked as text: the module has no YAML dependency and the rules are about
// the presence of lines, not about a parsed tree. Each test names the rule it pins.

// platforms is the release matrix: archive target, GOOS, GOARCH, GOARM and tier. A change to
// the workflow matrix must change this table, the plan and the notes script together.
var platforms = []struct{ target, goos, goarch, goarm, tier string }{
	{"linux_amd64", "linux", "amd64", "", "1"},
	{"linux_arm64", "linux", "arm64", "", "1"},
	{"darwin_arm64", "darwin", "arm64", "", "1"},
	{"darwin_amd64", "darwin", "amd64", "", "1"},
	{"windows_amd64", "windows", "amd64", "", "1"},
	{"windows_arm64", "windows", "arm64", "", "1"},
	{"linux_armv7", "linux", "arm", "7", "1"},
	{"linux_armv6", "linux", "arm", "6", "1"},
	{"freebsd_amd64", "freebsd", "amd64", "", "2"},
	{"freebsd_arm64", "freebsd", "arm64", "", "2"},
	{"openbsd_amd64", "openbsd", "amd64", "", "2"},
	{"openbsd_arm64", "openbsd", "arm64", "", "2"},
	{"netbsd_amd64", "netbsd", "amd64", "", "2"},
	{"netbsd_arm64", "netbsd", "arm64", "", "2"},
	{"linux_riscv64", "linux", "riscv64", "", "2"},
	{"linux_ppc64le", "linux", "ppc64le", "", "2"},
	{"linux_s390x", "linux", "s390x", "", "2"},
	{"linux_loong64", "linux", "loong64", "", "2"},
	{"illumos_amd64", "illumos", "amd64", "", "2"},
}

func readRepo(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot(t), filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

var jobHeader = regexp.MustCompile(`(?m)^  ([a-z-]+):\n`)

// jobs splits the workflow text into its jobs by name.
func jobs(t *testing.T, raw string) map[string]string {
	t.Helper()
	i := strings.Index(raw, "\njobs:\n")
	if i < 0 {
		t.Fatal("no jobs:")
	}
	body := raw[i+len("\njobs:\n"):]
	locs := jobHeader.FindAllStringSubmatchIndex(body, -1)
	out := map[string]string{}
	for n, l := range locs {
		end := len(body)
		if n+1 < len(locs) {
			end = locs[n+1][0]
		}
		out[body[l[2]:l[3]]] = body[l[0]:end]
	}
	return out
}

func TestWorkflowTriggersAreTagAndDispatchOnly(t *testing.T) {
	raw := readRepo(t, ".github/workflows/release.yml")
	head := raw[:strings.Index(raw, "\njobs:")]
	for _, want := range []string{"\non:\n  push:\n    tags: ['v[0-9]+.[0-9]+.[0-9]+']\n  workflow_dispatch:\n", "\npermissions:\n  contents: read\n", "\nconcurrency:\n", "cancel-in-progress: false"} {
		if !strings.Contains(head, want) {
			t.Errorf("workflow lacks %q", want)
		}
	}
	for _, bad := range []string{"pull_request", "branches:", "schedule:", "workflow_run:", "repository_dispatch"} {
		if strings.Contains(raw, bad) {
			t.Errorf("workflow must not contain %q", bad)
		}
	}
	if !strings.HasPrefix(raw, "# SPDX-License-Identifier: EUPL-1.2\n") {
		t.Error("missing SPDX header")
	}
}

var pinned = regexp.MustCompile(`^[a-z0-9-]+/[A-Za-z0-9._-]+@[0-9a-f]{40} # v[0-9][0-9.]*$`)

func TestEveryActionIsPinnedToACommit(t *testing.T) {
	for _, f := range []string{"release.yml", "ci.yml"} {
		raw := readRepo(t, ".github/workflows/"+f)
		n := 0
		for _, line := range strings.Split(raw, "\n") {
			s := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "- "))
			if !strings.HasPrefix(s, "uses:") {
				continue
			}
			n++
			if ref := strings.TrimSpace(strings.TrimPrefix(s, "uses:")); !pinned.MatchString(ref) {
				t.Errorf("%s: action not pinned to a full SHA with a version comment: %s", f, ref)
			}
		}
		if n == 0 {
			t.Errorf("%s: no actions found", f)
		}
	}
}

func TestEveryJobHasATimeoutAndExplicitPermissions(t *testing.T) {
	js := jobs(t, readRepo(t, ".github/workflows/release.yml"))
	for _, name := range []string{"verify", "build", "smoke", "smoke-emulated", "publish"} {
		j, ok := js[name]
		if !ok {
			t.Fatalf("job %s missing", name)
		}
		if !strings.Contains(j, "    timeout-minutes: ") || !strings.Contains(j, "    permissions:\n") {
			t.Errorf("job %s lacks timeout-minutes or permissions", name)
		}
	}
	if len(js) != 5 {
		t.Errorf("jobs %v, want exactly verify, build, smoke, smoke-emulated, publish", keys(js))
	}
}

func keys(m map[string]string) []string {
	var k []string
	for n := range m {
		k = append(k, n)
	}
	sort.Strings(k)
	return k
}

func TestOnlyPublishCanWriteAndAttest(t *testing.T) {
	raw := readRepo(t, ".github/workflows/release.yml")
	js := jobs(t, raw)
	for _, perm := range []string{"contents: write", "attestations: write", "id-token: write"} {
		if strings.Count(raw, perm) != 1 || !strings.Contains(js["publish"], perm) {
			t.Errorf("%q must appear once, in the publish job", perm)
		}
	}
	if strings.Contains(raw, "write-all") || strings.Contains(raw, "secrets.") {
		t.Error("write-all and secrets are not used")
	}
	p := js["publish"]
	for _, want := range []string{
		"    environment: release\n",
		"needs.verify.outputs.dry_run == 'false' && github.event_name == 'push' && github.ref_type == 'tag'",
		"needs: [verify, build, smoke, smoke-emulated]",
		"actions/attest-build-provenance@",
		"--verify-tag", "vars.CHARTSCRIBE_RELEASE_DRAFT",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("publish lacks %q", want)
		}
	}
	for _, rule := range []string{`test "$ACTOR" = "$OWNER"`, `test "$PUSHER" = "$OWNER"`, "releases\" --paginate", "git ls-remote", "SHA256SUMS", "sha256sum --check"} {
		if !strings.Contains(p, rule) {
			t.Errorf("publish does not recheck %q", rule)
		}
	}
	if strings.Contains(p, "workflow_dispatch") {
		t.Error("publish must not mention dispatch")
	}
}

func TestVerifyJobGates(t *testing.T) {
	v := jobs(t, readRepo(t, ".github/workflows/release.yml"))["verify"]
	for _, want := range []string{
		"release-verify.sh version", "release-verify.sh dryversion", "release-verify.sh tag", "release-verify.sh go",
		`test "$ACTOR" = "$OWNER"`, `test "$PUSHER" = "$OWNER"`,
		"actions/workflows/ci.yml/runs?head_sha=", "releases/tags/$TAG", "make check", "go mod verify",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("verify lacks %q", want)
		}
	}
	if got := strings.Count(v, "if: steps.resolve.outputs.dry_run == 'false'"); got != 3 {
		t.Errorf("%d tag-only steps in verify, want 3 (annotated tag, green ci, no release)", got)
	}
}

func TestBuildIsReproducibleAndStatic(t *testing.T) {
	b := jobs(t, readRepo(t, ".github/workflows/release.yml"))["build"]
	for _, want := range []string{
		"CGO_ENABLED: '0'", "GOTOOLCHAIN: local", "GOFLAGS: -mod=readonly", "-trimpath",
		"-X github.com/wstein/chartscribe/internal/cli.Version=$VERSION",
		"GOCACHE=\"$RUNNER_TEMP/gocache-first\"", "GOCACHE=\"$RUNNER_TEMP/gocache-second\"", "cmp ",
		"release-inspect.sh", "release-package.sh", "SOURCE_DATE_EPOCH=", "cache: false", "retention-days: 1", "if-no-files-found: error",
		"GOARM: ${{ matrix.goarm }}",
	} {
		if !strings.Contains(b, want) {
			t.Errorf("build lacks %q", want)
		}
	}
}

func matrixTargets(job string) []string {
	var out []string
	for _, line := range strings.Split(job, "\n") {
		if s := strings.TrimSpace(line); strings.HasPrefix(s, "target: ") || strings.HasPrefix(s, "- target: ") {
			out = append(out, strings.TrimPrefix(strings.TrimPrefix(s, "- "), "target: "))
		}
	}
	return out
}

func TestMatrixMatchesThePlatformTable(t *testing.T) {
	raw := readRepo(t, ".github/workflows/release.yml")
	js := jobs(t, raw)
	var want []string
	var tier1 []string
	for _, p := range platforms {
		want = append(want, p.target)
		if p.tier == "1" {
			tier1 = append(tier1, p.target)
		}
		block := "          - os: " + p.goos + "\n            arch: " + p.goarch + "\n"
		if p.goarm != "" {
			block += "            goarm: '" + p.goarm + "'\n"
		}
		block += "            target: " + p.target + "\n            tier: " + p.tier + "\n"
		if !strings.Contains(js["build"], block) {
			t.Errorf("build matrix lacks the entry for %s:\n%s", p.target, block)
		}
	}
	if got := matrixTargets(js["build"]); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("build targets %v, want %v", got, want)
	}
	// Every tier 1 target is smoked, natively or under qemu; no tier 2 target is.
	smoked := append(matrixTargets(js["smoke"]), matrixTargets(js["smoke-emulated"])...)
	sort.Strings(smoked)
	sort.Strings(tier1)
	if strings.Join(smoked, " ") != strings.Join(tier1, " ") {
		t.Errorf("smoked %v, want the tier 1 targets %v", smoked, tier1)
	}
	if e := matrixTargets(js["smoke-emulated"]); strings.Join(e, " ") != "linux_armv7 linux_armv6" {
		t.Errorf("emulated smoke targets %v, want the Raspberry Pi archives", e)
	}
	if !strings.Contains(js["smoke-emulated"], "qemu-arm -cpu arm1176") || !strings.Contains(js["smoke-emulated"], "qemu-arm -cpu cortex-a15") {
		t.Error("emulated smoke does not name the qemu cpus")
	}
	// The expected archive count of the publish job follows the table.
	if !strings.Contains(js["publish"], "          n="+itoa(len(platforms))+"\n") {
		t.Errorf("publish does not expect %d archives", len(platforms))
	}
	for _, p := range platforms {
		if p.tier == "1" && p.goos == "windows" && !strings.Contains(js["smoke"], "target: "+p.target+"\n            runner: ") {
			t.Errorf("no native runner for %s", p.target)
		}
	}
}

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}

func TestMatrixPairsAreSupportedAndSixtyFourBitOrArm(t *testing.T) {
	need(t, "go")
	out, err := exec.Command("go", "tool", "dist", "list").Output()
	if err != nil {
		t.Skipf("SKIPPED, NOT PASSED: go tool dist list: %v", err)
	}
	have := map[string]bool{}
	for _, l := range strings.Fields(string(out)) {
		have[l] = true
	}
	for _, p := range platforms {
		if !have[p.goos+"/"+p.goarch] {
			t.Errorf("%s/%s is not supported by the pinned Go", p.goos, p.goarch)
		}
		if p.goarch == "arm" && (p.goos != "linux" || (p.goarm != "6" && p.goarm != "7")) {
			t.Errorf("%s: 32-bit arm is only the Raspberry Pi linux GOARM 6 and 7", p.target)
		}
		if p.goarch == "386" || p.goarch == "mips" || p.goarch == "mipsle" {
			t.Errorf("%s: other 32-bit targets are not shipped", p.target)
		}
		wantTarget := p.goos + "_" + p.goarch
		if p.goarch == "arm" {
			wantTarget = p.goos + "_armv" + p.goarm
		}
		if p.target != wantTarget {
			t.Errorf("target %s, want %s", p.target, wantTarget)
		}
	}
}

func TestNotesAndReadmeNameEveryPlatformFamily(t *testing.T) {
	notes := readRepo(t, ".github/scripts/release-notes.sh")
	readme := readRepo(t, "README.md")
	plan := readRepo(t, "specs/002-release/plan.md")
	for _, p := range platforms {
		if p.tier == "1" && p.goarch != "arm" && !strings.Contains(notes, p.goos+"/"+p.goarch) {
			t.Errorf("release notes do not list %s/%s", p.goos, p.goarch)
		}
		if !strings.Contains(plan, p.goos+"/"+p.goarch) {
			t.Errorf("plan does not list %s/%s", p.goos, p.goarch)
		}
	}
	for _, want := range []string{"linux_armv7", "linux_armv6", "linux_arm64", "Raspberry Pi", "gh attestation verify", "go install github.com/wstein/chartscribe/cmd/cscribe@v0.1.0", "Gatekeeper", "SmartScreen"} {
		if !strings.Contains(readme, want) {
			t.Errorf("README lacks %q", want)
		}
	}
}

func TestMakefileStaysFreeOfBuildTargets(t *testing.T) {
	mk := readRepo(t, "Makefile")
	for _, bad := range []string{"go build", "goreleaser", "release:", "dist:", "build:"} {
		if strings.Contains(mk, bad) {
			t.Errorf("Makefile must have no build or release target: found %q", bad)
		}
	}
}
