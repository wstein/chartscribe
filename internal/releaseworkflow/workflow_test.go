// SPDX-License-Identifier: EUPL-1.2

package releaseworkflow

import (
	"fmt"
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
	// Every archive is smoked natively or under qemu, unless it is on the explicit build-only list.
	native := matrixTargets(js["smoke"])
	emulated := matrixTargets(js["smoke-emulated"])
	for _, p := range platforms {
		inNative := contains(native, p.target)
		inEmulated := contains(emulated, p.target)
		reason, buildOnly := buildOnly[p.target]
		switch {
		case buildOnly && (inNative || inEmulated):
			t.Errorf("%s is on the build-only list (%s) but is smoked", p.target, reason)
		case !buildOnly && !inNative && !inEmulated:
			t.Errorf("%s has neither a native nor an emulated smoke and is not on the build-only list", p.target)
		case inNative && inEmulated:
			t.Errorf("%s is smoked twice", p.target)
		}
		if inEmulated && p.goos != "linux" {
			t.Errorf("%s: qemu-user runs Linux binaries only", p.target)
		}
	}
	for _, n := range append(append([]string{}, native...), emulated...) {
		known := false
		for _, p := range platforms {
			known = known || p.target == n
		}
		if !known {
			t.Errorf("smoke target %s is not in the platform table", n)
		}
	}
	for tg, cpu := range map[string]string{"linux_armv7": "qemu-arm -cpu cortex-a15", "linux_armv6": "qemu-arm -cpu arm1176",
		"linux_riscv64": "qemu-riscv64", "linux_ppc64le": "qemu-ppc64le -cpu power9", "linux_s390x": "qemu-s390x -cpu max", "linux_loong64": "qemu-loongarch64"} {
		if !strings.Contains(js["smoke-emulated"], "target: "+tg+"\n            ext: tar.gz\n            run: "+cpu+"\n") {
			t.Errorf("emulated smoke of %s does not run %q", tg, cpu)
		}
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

// buildOnly lists the archives that no job runs, each with its reason.
var buildOnly = map[string]string{
	"freebsd_amd64": "BSD system calls: qemu-user runs Linux only", "freebsd_arm64": "BSD system calls: qemu-user runs Linux only",
	"openbsd_amd64": "BSD system calls: qemu-user runs Linux only", "openbsd_arm64": "BSD system calls: qemu-user runs Linux only",
	"netbsd_amd64": "BSD system calls: qemu-user runs Linux only", "netbsd_arm64": "BSD system calls: qemu-user runs Linux only",
	"illumos_amd64": "illumos system calls: qemu-user runs Linux only",
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
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
	for _, want := range []string{"linux_armv7", "linux_armv6", "linux_arm64", "Raspberry Pi", "gh attestation verify", "go install github.com/wstein/chartscribe/cmd/cscribe@latest", "Gatekeeper", "SmartScreen"} {
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

// step is one workflow step: its text and the body of its run: block (empty without one).
type step struct{ text, run string }

var stepStart = regexp.MustCompile(`(?m)^      - `)

// stepsOf splits a job's text into steps. A run: block runs until the first line indented
// less than its body.
func stepsOf(job string) []step {
	locs := stepStart.FindAllStringIndex(job, -1)
	var out []step
	for n, l := range locs {
		end := len(job)
		if n+1 < len(locs) {
			end = locs[n+1][0]
		}
		text := job[l[0]:end]
		st := step{text: text}
		lines := strings.Split(text, "\n")
		for i, line := range lines {
			t := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "- "))
			if !strings.HasPrefix(t, "run:") {
				continue
			}
			rest := strings.TrimSpace(strings.TrimPrefix(t, "run:"))
			if rest != "|" {
				st.run = rest
				break
			}
			var body []string
			for _, b := range lines[i+1:] {
				if strings.TrimSpace(b) != "" && !strings.HasPrefix(b, "          ") {
					break
				}
				body = append(body, strings.TrimPrefix(b, "          "))
			}
			st.run = strings.TrimSpace(strings.Join(body, "\n"))
			break
		}
		out = append(out, st)
	}
	return out
}

// allowedActions is every action the workflow may use, with its exact pin.
var allowedActions = map[string]bool{
	"actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1":                true,
	"actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0":                true,
	"actions/upload-artifact@cf430e030ddbb5b0abf93d22962f4752f3646cd9 # v7.0.2":         true,
	"actions/download-artifact@9000827ccba6bdab643e8b6fd33ac0654aef8333 # v8.0.2":       true,
	"actions/attest-build-provenance@4d101475d8b20a2381f78447822ac1eab6504dd8 # v4.2.2": true,
}

var (
	swallowRe = regexp.MustCompile(`\|\|\s*(true|:)(\s|$|;)`)
	disarmRe  = regexp.MustCompile(`(?m)(^|\s)(exit 0|set \+e)(\s|$)|\|\|\s*echo\b`)
	// qemuInstall is the one permitted package installation: the emulator of the Raspberry Pi smoke.
	qemuInstall = "sudo apt-get install -y --no-install-recommends qemu-user"
)

// violations lists the rule breaks of a release workflow text.
func violations(raw string) []string {
	var out []string
	add := func(format string, a ...any) { out = append(out, strings.TrimSpace(sprintf(format, a...))) }
	jt := jobsOf(raw)
	for name, job := range jt {
		for _, st := range stepsOf(job) {
			if i := strings.Index(st.text, "uses: "); i >= 0 {
				ref := strings.TrimSpace(strings.SplitN(st.text[i+len("uses: "):], "\n", 2)[0])
				if !allowedActions[ref] {
					add("job %s uses an action that is not on the allowed list: %s", name, ref)
				}
				if strings.HasPrefix(ref, "actions/checkout@") && !strings.Contains(st.text, "persist-credentials: false") {
					add("job %s checkout must set persist-credentials: false", name)
				}
				if strings.HasPrefix(ref, "actions/upload-artifact@") && name != "build" {
					add("only build may upload artifacts, job %s does", name)
				}
				if strings.HasPrefix(ref, "actions/download-artifact@") && name != "smoke" && name != "smoke-emulated" && name != "publish" {
					add("only the smoke and publish jobs may download artifacts, job %s does", name)
				}
			}
			run := st.run
			if run == "" {
				continue
			}
			if strings.Contains(run, "\n") && !strings.HasPrefix(run, "set -euo pipefail\n") {
				add("job %s multi-line script must start with set -euo pipefail", name)
			}
			if strings.Contains(run, "${{") {
				add("job %s interpolates an expression inside a run script; pass it through env", name)
			}
			if swallowRe.MatchString(run) {
				add("job %s script ignores a failure with || true or || :", name)
			}
			if disarmRe.MatchString(run) {
				add("job %s script can succeed past a failure (exit 0, set +e, || echo)", name)
			}
			checked := run
			if name == "smoke-emulated" {
				// Only the exact install lines are allowed, whole line, nothing appended.
				var kept []string
				for _, line := range strings.Split(run, "\n") {
					if l := strings.TrimSpace(line); l != "sudo apt-get update" && l != qemuInstall {
						kept = append(kept, line)
					}
				}
				checked = strings.Join(kept, "\n")
			}
			for _, banned := range []string{"curl ", "wget ", "pip ", "npm ", "go install", "apt-get", "brew "} {
				if strings.Contains(checked, banned) {
					add("job %s run script fetches or installs a tool (%s)", name, strings.TrimSpace(banned))
				}
			}
		}
	}
	if regexp.MustCompile(`(?m)^\s*(?:- )?uses:\s*\S+@(?:v\d|main|master)\b`).MatchString(raw) {
		add("a uses: line names a mutable ref")
	}
	sort.Strings(out)
	return out
}

func sprintf(format string, a ...any) string { return fmt.Sprintf(format, a...) }

// jobsOf is jobs without a testing.T.
func jobsOf(raw string) map[string]string {
	i := strings.Index(raw, "\njobs:\n")
	if i < 0 {
		return nil
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

func TestReleaseWorkflowHasNoViolations(t *testing.T) {
	if v := violations(readRepo(t, ".github/workflows/release.yml")); len(v) != 0 {
		t.Errorf("violations:\n%s", strings.Join(v, "\n"))
	}
}

func TestWeakeningTheWorkflowIsCaught(t *testing.T) {
	raw := readRepo(t, ".github/workflows/release.yml")
	smokeCheckout := "  smoke:\n"
	cases := []struct{ name, from, to, want string }{
		{"persist-credentials dropped", "      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1\n        with:\n          persist-credentials: false\n      - uses: actions/download-artifact@9000827ccba6bdab643e8b6fd33ac0654aef8333 # v8.0.2\n        with:\n          name: release-${{ matrix.target }}\n          path: ${{ runner.temp }}/dist\n      - name: Smoke-test",
			"      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1\n      - uses: actions/download-artifact@9000827ccba6bdab643e8b6fd33ac0654aef8333 # v8.0.2\n        with:\n          name: release-${{ matrix.target }}\n          path: ${{ runner.temp }}/dist\n      - name: Smoke-test", "persist-credentials"},
		{"persist-credentials true", "persist-credentials: false", "persist-credentials: true", "persist-credentials"},
		{"expression in run", `          sh .github/scripts/release-verify.sh go`, "", ""},
		{"swallowed failure", `sh .github/scripts/release-smoke.sh "$VERSION" "$dist/cscribe_${plain}_${TARGET}.$EXT"`, `sh .github/scripts/release-smoke.sh "$VERSION" "$dist/cscribe_${plain}_${TARGET}.$EXT" || true`, "|| true"},
		{"unlisted action", "actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0", "actions/setup-go@0000000000000000000000000000000000000000 # v7.0.0", "allowed list"},
		{"package appended to the qemu install", "--no-install-recommends qemu-user\n", "--no-install-recommends qemu-user curl\n", "apt-get"},
		{"other apt-get", "sudo apt-get update\n", "sudo apt-get update\n          sudo apt-get install -y curl\n", "apt-get"},
		{"no strict mode", "          set -euo pipefail\n          sudo apt-get update", "          sudo apt-get update", "set -euo pipefail"},
	}
	_ = smokeCheckout
	for _, c := range cases {
		from, to, want := c.from, c.to, c.want
		if c.name == "expression in run" {
			from = `plain=${VERSION#v}`
			to = `plain=${{ needs.verify.outputs.version }}`
			want = "expression"
		}
		if !strings.Contains(raw, from) {
			t.Fatalf("%s: the mutation target is not in the workflow", c.name)
		}
		mutated := strings.Replace(raw, from, to, 1)
		v := strings.Join(violations(mutated), "\n")
		if !strings.Contains(v, want) {
			t.Errorf("%s: not caught (want %q), violations:\n%s", c.name, want, v)
		}
	}
}

func TestPublishGatesOnTheSmokeJobs(t *testing.T) {
	p := jobs(t, readRepo(t, ".github/workflows/release.yml"))["publish"]
	if !strings.Contains(p, "needs: [verify, build, smoke, smoke-emulated]") {
		t.Error("publish must wait for the native and the emulated smoke jobs")
	}
}

func TestOnlyTheReleaseWorkflowRunsReleaseJobsAndQemu(t *testing.T) {
	rel := readRepo(t, ".github/workflows/release.yml")
	if i := strings.Index(rel, "\non:\n"); i < 0 || !strings.HasPrefix(rel[i:], "\non:\n  push:\n    tags: ['v[0-9]+.[0-9]+.[0-9]+']\n  workflow_dispatch:\n    inputs:\n") {
		t.Error("release.yml triggers must be exactly a tag push and workflow_dispatch")
	}
	entries, err := os.ReadDir(filepath.Join(repoRoot(t), ".github", "workflows"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() == "release.yml" {
			continue
		}
		raw := strings.ToLower(readRepo(t, ".github/workflows/"+e.Name()))
		for _, bad := range []string{"qemu", "release.yml", "smoke-emulated", "release-smoke", "release-package"} {
			if strings.Contains(raw, bad) {
				t.Errorf("%s must not reference %q: only the release workflow runs release jobs", e.Name(), bad)
			}
		}
	}
}
