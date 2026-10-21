// SPDX-License-Identifier: EUPL-1.2

// Package releaseworkflow tests the release pipeline of specs/002-release: the scripts under
// .github/scripts (this file) and the structure of the workflow (workflow_test.go). It has no
// non-test code. Tests that need a tool the host lacks are skipped and say so.
package releaseworkflow

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func script(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join(repoRoot(t), ".github", "scripts", name)
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("missing script: %v", err)
	}
	return p
}

// need skips (never passes) a test whose tools are missing.
func need(t *testing.T, tools ...string) {
	t.Helper()
	for _, tool := range tools {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("SKIPPED, NOT PASSED: %s is not available", tool)
		}
	}
}

// gnuTar skips unless tar is GNU tar, which the packaging script requires.
func gnuTar(t *testing.T) {
	t.Helper()
	need(t, "tar", "gzip")
	out, err := exec.Command("tar", "--version").Output()
	if err != nil || !strings.Contains(string(out), "GNU tar") {
		t.Skip("SKIPPED, NOT PASSED: GNU tar is not available")
	}
}

type result struct {
	code   int
	stdout string
	stderr string
}

func run(t *testing.T, dir string, env []string, name string, args ...string) result {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=T", "GIT_AUTHOR_EMAIL=t@example.test",
		"GIT_COMMITTER_NAME=T", "GIT_COMMITTER_EMAIL=t@example.test")
	cmd.Env = append(cmd.Env, env...)
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	err := cmd.Run()
	code := 0
	if err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("%s %v: %v", name, args, err)
		}
		code = ee.ExitCode()
	}
	return result{code, so.String(), se.String()}
}

func sh(t *testing.T, dir string, env []string, name string, args ...string) result {
	t.Helper()
	need(t, "sh")
	return run(t, dir, env, "sh", append([]string{script(t, name)}, args...)...)
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	need(t, "git")
	r := run(t, dir, nil, "git", args...)
	if r.code != 0 {
		t.Fatalf("git %v: %s%s", args, r.stdout, r.stderr)
	}
	return strings.TrimSpace(r.stdout)
}

func commitFile(t *testing.T, dir, name, subject string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(subject), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", name)
	git(t, dir, "commit", "-q", "-m", subject)
}

// newRepo returns a repository with commit subjects on main, oldest first.
func newRepo(t *testing.T, subjects ...string) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	for i, s := range subjects {
		commitFile(t, dir, "f"+string(rune('a'+i)), s)
	}
	return dir
}

func TestVerifyVersion(t *testing.T) {
	good := []string{"v0.1.0", "v1.20.300", "v10.0.0"}
	bad := []string{"", "0.1.0", "v1", "v1.2", "v01.2.3", "v1.02.3", "v1.2.03", "v1.2.3-rc1", "v1.2.3+b",
		"v1.2.3\nx", " v1.2.3", "V1.2.3", "v1.2.3.4", "v-1.2.3"}
	for _, v := range good {
		if r := sh(t, repoRoot(t), nil, "release-verify.sh", "version", v); r.code != 0 {
			t.Errorf("version %q rejected: %s", v, r.stderr)
		}
	}
	for _, v := range bad {
		if r := sh(t, repoRoot(t), nil, "release-verify.sh", "version", v); r.code == 0 {
			t.Errorf("version %q accepted", v)
		}
	}
}

func TestVerifyDryRunVersion(t *testing.T) {
	for _, v := range []string{"v0.0.0-dryrun", "v1.2.3-rc.1"} {
		if r := sh(t, repoRoot(t), nil, "release-verify.sh", "dryversion", v); r.code != 0 {
			t.Errorf("dry version %q rejected: %s", v, r.stderr)
		}
	}
	for _, v := range []string{"", "v1.2.3", "1.2.3", "v1.2", "v1.2.3-", "v1.2.3-$(id)", "v1.2.3;x", "v1.2.3 x", "v1.2.3\nv1.2.4", "v1.2.3-a/b"} {
		if r := sh(t, repoRoot(t), nil, "release-verify.sh", "dryversion", v); r.code == 0 {
			t.Errorf("dry version %q accepted", v)
		}
	}
}

func TestVerifyTag(t *testing.T) {
	dir := newRepo(t, "one", "two")
	head := git(t, dir, "rev-parse", "HEAD")
	git(t, dir, "tag", "-a", "-m", "release", "v0.1.0")
	git(t, dir, "tag", "v0.2.0") // lightweight
	git(t, dir, "tag", "-a", "-m", "x", "v0.3.0-rc1")

	if r := sh(t, dir, nil, "release-verify.sh", "tag", "v0.1.0", "main"); r.code != 0 || strings.TrimSpace(r.stdout) != head {
		t.Errorf("annotated tag on main must pass and print the commit: %+v", r)
	}
	if r := sh(t, dir, nil, "release-verify.sh", "tag", "v0.2.0", "main"); r.code == 0 || !strings.Contains(r.stderr, "annotated") {
		t.Errorf("lightweight tag must fail naming 'annotated': %+v", r)
	}
	if r := sh(t, dir, nil, "release-verify.sh", "tag", "v0.3.0-rc1", "main"); r.code == 0 {
		t.Errorf("pre-release tag must fail: %+v", r)
	}
	if r := sh(t, dir, nil, "release-verify.sh", "tag", "v9.9.9", "main"); r.code == 0 {
		t.Errorf("missing tag must fail: %+v", r)
	}

	// A tag on a commit that is not reachable from main.
	git(t, dir, "checkout", "-q", "-b", "side")
	commitFile(t, dir, "g", "side")
	git(t, dir, "tag", "-a", "-m", "side", "v0.4.0")
	if r := sh(t, dir, nil, "release-verify.sh", "tag", "v0.4.0", "main"); r.code == 0 || !strings.Contains(r.stderr, "main") {
		t.Errorf("tag off main must fail naming main: %+v", r)
	}
	if r := sh(t, dir, nil, "release-verify.sh", "tag", "v0.1.0"); r.code != 2 {
		t.Errorf("missing argument must exit 2: %+v", r)
	}
	if r := sh(t, dir, nil, "release-verify.sh", "bogus"); r.code != 2 {
		t.Errorf("unknown subcommand must exit 2: %+v", r)
	}
}

func TestVerifyGoVersionConsistency(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(".go-version", "1.27.2\n")
	write("go.mod", "module x\n\ngo 1.27.2\n\nrequire y v1.0.0\n")
	if r := sh(t, dir, nil, "release-verify.sh", "go"); r.code != 0 {
		t.Errorf("matching versions rejected: %+v", r)
	}
	write("go.mod", "module x\n\ngo 1.27.3\n")
	if r := sh(t, dir, nil, "release-verify.sh", "go"); r.code == 0 {
		t.Errorf("mismatch accepted")
	}
	write("go.mod", "module x\n")
	if r := sh(t, dir, nil, "release-verify.sh", "go"); r.code == 0 {
		t.Errorf("missing go line accepted")
	}
	if r := sh(t, repoRoot(t), nil, "release-verify.sh", "go"); r.code != 0 {
		t.Errorf("repository .go-version and go.mod disagree: %+v", r)
	}
}

// realBinary builds cscribe for the host into a temp dir with the given stamped version
// (none when empty) and returns its path. It is a test build of one target, not a distribution.
func realBinary(t *testing.T, version string) string {
	t.Helper()
	need(t, "go")
	exe := "cscribe"
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	out := filepath.Join(t.TempDir(), exe)
	args := []string{"build", "-trimpath"}
	if version != "" {
		args = append(args, "-ldflags", "-X github.com/wstein/chartscribe/internal/cli.Version="+version)
	}
	args = append(args, "-o", out, "./cmd/cscribe")
	r := run(t, repoRoot(t), []string{"CGO_ENABLED=0"}, "go", args...)
	if r.code != 0 {
		t.Fatalf("go build: %s%s", r.stdout, r.stderr)
	}
	return out
}

func TestVersionIsStampedAndDefaultsToDev(t *testing.T) {
	if r := run(t, "", nil, realBinary(t, ""), "version"); r.stdout != "cscribe dev\n" {
		t.Errorf("unstamped build printed %q, want %q", r.stdout, "cscribe dev\n")
	}
	if r := run(t, "", nil, realBinary(t, "v0.1.0"), "version"); r.stdout != "cscribe v0.1.0\n" {
		t.Errorf("stamped build printed %q", r.stdout)
	}
}

func hostTarget() string { return runtime.GOOS + "_" + runtime.GOARCH }

// pack packages binary as version/target into out, run in the repository root.
func pack(t *testing.T, version, target, binary, out string) result {
	t.Helper()
	return sh(t, repoRoot(t), []string{"SOURCE_DATE_EPOCH=1700000000"}, "release-package.sh", version, target, binary, out)
}

// listing returns the sorted member names of a tar.gz or zip.
func listing(t *testing.T, archive string) []string {
	t.Helper()
	var r result
	if strings.HasSuffix(archive, ".zip") {
		need(t, "unzip")
		r = run(t, "", nil, "unzip", "-Z1", archive)
	} else {
		r = run(t, "", nil, "tar", "-tzf", archive)
	}
	if r.code != 0 {
		t.Fatalf("list %s: %s", archive, r.stderr)
	}
	return strings.Fields(r.stdout)
}

func TestPackageTarLayoutAndDeterminism(t *testing.T) {
	gnuTar(t)
	bin := realBinary(t, "v0.1.0")
	out1, out2 := t.TempDir(), t.TempDir()
	if r := pack(t, "v0.1.0", "linux_armv7", bin, out1); r.code != 0 {
		t.Fatalf("pack: %+v", r)
	}
	if r := pack(t, "v0.1.0", "linux_armv7", bin, out2); r.code != 0 {
		t.Fatalf("pack: %+v", r)
	}
	name := "cscribe_0.1.0_linux_armv7.tar.gz"
	a, err := os.ReadFile(filepath.Join(out1, name))
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(out2, name))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Error("two packagings of the same inputs differ")
	}
	want := []string{"cscribe_0.1.0_linux_armv7/", "cscribe_0.1.0_linux_armv7/LICENSE", "cscribe_0.1.0_linux_armv7/README.md",
		"cscribe_0.1.0_linux_armv7/agent-guide.md", "cscribe_0.1.0_linux_armv7/cscribe", "cscribe_0.1.0_linux_armv7/llms.txt"}
	if got := listing(t, filepath.Join(out1, name)); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("members %v, want %v", got, want)
	}
	if v := run(t, out1, nil, "tar", "-tvzf", name); !strings.Contains(v.stdout, "-rwxr-xr-x 0/0") || !strings.Contains(v.stdout, "2023-11-14") {
		t.Errorf("mode, owner or mtime not normalised:\n%s", v.stdout)
	}
	sum, err := os.ReadFile(filepath.Join(out1, name+".sha256"))
	if err != nil || !strings.HasSuffix(strings.TrimSpace(string(sum)), "  "+name) {
		t.Errorf("sidecar %q: %v", sum, err)
	}
}

func TestPackageZipLayoutAndDeterminism(t *testing.T) {
	need(t, "zip", "unzip")
	bin := realBinary(t, "v0.1.0")
	out1, out2 := t.TempDir(), t.TempDir()
	for _, out := range []string{out1, out2} {
		if r := pack(t, "v0.1.0", "windows_arm64", bin, out); r.code != 0 {
			t.Fatalf("pack: %+v", r)
		}
	}
	name := "cscribe_0.1.0_windows_arm64.zip"
	a, _ := os.ReadFile(filepath.Join(out1, name))
	b, _ := os.ReadFile(filepath.Join(out2, name))
	if len(a) == 0 || !bytes.Equal(a, b) {
		t.Error("two zip packagings of the same inputs differ or are empty")
	}
	d := "cscribe_0.1.0_windows_arm64/"
	want := []string{d, d + "LICENSE", d + "README.md", d + "agent-guide.md", d + "cscribe.exe", d + "llms.txt"}
	if got := listing(t, filepath.Join(out1, name)); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("members %v, want %v", got, want)
	}
	if _, err := os.Stat(filepath.Join(out1, name+".sha256")); err != nil {
		t.Error(err)
	}
}

func TestPackageRefusals(t *testing.T) {
	gnuTar(t)
	bin := realBinary(t, "")
	out := t.TempDir()
	cases := []struct {
		name string
		env  []string
		args []string
	}{
		{"bad version", nil, []string{"1.0.0", "linux_amd64", bin, out}},
		{"newline in version", nil, []string{"v1.0.0\nv", "linux_amd64", bin, out}},
		{"bad target", nil, []string{"v1.0.0", "linux/amd64", bin, out}},
		{"missing binary", nil, []string{"v1.0.0", "linux_amd64", filepath.Join(out, "nope"), out}},
		{"no epoch", []string{"SOURCE_DATE_EPOCH="}, []string{"v1.0.0", "linux_amd64", bin, out}},
		{"epoch not digits", []string{"SOURCE_DATE_EPOCH=yesterday"}, []string{"v1.0.0", "linux_amd64", bin, out}},
	}
	for _, c := range cases {
		env := c.env
		if env == nil {
			env = []string{"SOURCE_DATE_EPOCH=1"}
		}
		if r := sh(t, repoRoot(t), env, "release-package.sh", c.args...); r.code == 0 {
			t.Errorf("%s: accepted", c.name)
		}
	}
	if r := sh(t, repoRoot(t), nil, "release-package.sh", "v1.0.0"); r.code != 2 {
		t.Errorf("usage error must exit 2: %+v", r)
	}
	if r := sh(t, t.TempDir(), []string{"SOURCE_DATE_EPOCH=1"}, "release-package.sh", "v1.0.0", "linux_amd64", bin, out); r.code == 0 || !strings.Contains(r.stderr, "not found") {
		t.Errorf("a tree without LICENSE must be refused: %+v", r)
	}
}

func smoke(t *testing.T, env []string, version, archive string) result {
	t.Helper()
	return sh(t, repoRoot(t), env, "release-smoke.sh", version, archive)
}

func TestSmokeWithTheRealBinary(t *testing.T) {
	gnuTar(t)
	if runtime.GOOS == "windows" {
		t.Skip("SKIPPED, NOT PASSED: the host target is packaged as a zip")
	}
	bin := realBinary(t, "v0.1.0")
	out := t.TempDir()
	if r := pack(t, "v0.1.0", hostTarget(), bin, out); r.code != 0 {
		t.Fatalf("pack: %+v", r)
	}
	archive := filepath.Join(out, "cscribe_0.1.0_"+hostTarget()+".tar.gz")
	if r := smoke(t, nil, "v0.1.0", archive); r.code != 0 || !strings.Contains(r.stdout, "ok") {
		t.Fatalf("smoke of a good archive failed: %+v", r)
	}
	if r := smoke(t, nil, "v0.1.1", archive); r.code == 0 || !strings.Contains(r.stderr, "cscribe version printed") {
		t.Errorf("a wrong stamped version must fail: %+v", r)
	}
	// The emulator prefix is applied to every invocation; a prefix that fails must fail the smoke.
	if r := smoke(t, []string{"CSCRIBE_RUN=false"}, "v0.1.0", archive); r.code == 0 {
		t.Errorf("a failing CSCRIBE_RUN prefix must fail the smoke: %+v", r)
	}
	if r := smoke(t, []string{"CSCRIBE_RUN=env"}, "v0.1.0", archive); r.code != 0 {
		t.Errorf("a working CSCRIBE_RUN prefix must pass: %+v", r)
	}
	// A damaged archive fails its checksum.
	data, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archive, append(data, 0), 0o644); err != nil {
		t.Fatal(err)
	}
	if r := smoke(t, nil, "v0.1.0", archive); r.code == 0 || !strings.Contains(r.stderr, "checksum") {
		t.Errorf("a damaged archive must fail its checksum: %+v", r)
	}
}

func TestSmokeRejectsAnExtraMemberAndAMissingSidecar(t *testing.T) {
	gnuTar(t)
	if runtime.GOOS == "windows" {
		t.Skip("SKIPPED, NOT PASSED: the host target is packaged as a zip")
	}
	bin := realBinary(t, "v0.1.0")
	out := t.TempDir()
	if r := pack(t, "v0.1.0", hostTarget(), bin, out); r.code != 0 {
		t.Fatalf("pack: %+v", r)
	}
	name := "cscribe_0.1.0_" + hostTarget()
	archive := filepath.Join(out, name+".tar.gz")
	if err := os.Remove(archive + ".sha256"); err != nil {
		t.Fatal(err)
	}
	if r := smoke(t, nil, "v0.1.0", archive); r.code == 0 || !strings.Contains(r.stderr, "sidecar") {
		t.Errorf("a missing sidecar must fail: %+v", r)
	}
	// Rebuild the archive with an extra file and a valid sidecar.
	work := t.TempDir()
	if r := run(t, work, nil, "tar", "-xzf", archive); r.code != 0 {
		t.Fatal(r.stderr)
	}
	if err := os.WriteFile(filepath.Join(work, name, "extra"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	extra := filepath.Join(out, name+".tar.gz")
	if r := run(t, work, nil, "tar", "-czf", extra, name); r.code != 0 {
		t.Fatal(r.stderr)
	}
	if r := run(t, out, nil, "sh", "-c", "sha256sum "+name+".tar.gz > "+name+".tar.gz.sha256"); r.code != 0 {
		t.Skipf("SKIPPED, NOT PASSED: sha256sum unavailable: %s", r.stderr)
	}
	if r := smoke(t, nil, "v0.1.0", extra); r.code == 0 || !strings.Contains(r.stderr, "unexpected archive members") {
		t.Errorf("an extra member must fail: %+v", r)
	}
}

func TestInspectChecksTheHeader(t *testing.T) {
	bin := realBinary(t, "")
	if r := sh(t, repoRoot(t), nil, "release-inspect.sh", runtime.GOOS, runtime.GOARCH, bin); r.code != 0 {
		t.Errorf("host binary rejected for its own target: %+v", r)
	}
	wrongArch := "amd64"
	if runtime.GOARCH == "amd64" {
		wrongArch = "arm64"
	}
	if r := sh(t, repoRoot(t), nil, "release-inspect.sh", runtime.GOOS, wrongArch, bin); r.code != 1 {
		t.Errorf("wrong architecture must exit 1: %+v", r)
	}
	wrongOS := "darwin"
	if runtime.GOOS == "darwin" {
		wrongOS = "windows"
	}
	if r := sh(t, repoRoot(t), nil, "release-inspect.sh", wrongOS, runtime.GOARCH, bin); r.code != 1 {
		t.Errorf("wrong format must exit 1: %+v", r)
	}
	junk := filepath.Join(t.TempDir(), "junk")
	if err := os.WriteFile(junk, []byte("not a binary at all, only text"), 0o755); err != nil {
		t.Fatal(err)
	}
	if r := sh(t, repoRoot(t), nil, "release-inspect.sh", "linux", "amd64", junk); r.code != 1 {
		t.Errorf("junk must exit 1: %+v", r)
	}
	if r := sh(t, repoRoot(t), nil, "release-inspect.sh", "linux", "amd64"); r.code != 2 {
		t.Errorf("usage error must exit 2: %+v", r)
	}
}

func TestInspectPEAndMachO(t *testing.T) {
	// Synthetic headers: the checks read bytes only.
	dir := t.TempDir()
	pe := make([]byte, 0x90)
	copy(pe, "MZ")
	pe[0x3c] = 0x80
	copy(pe[0x80:], "PE\x00\x00")
	pe[0x84], pe[0x85] = 0x64, 0xaa // arm64
	pePath := filepath.Join(dir, "pe")
	macho := []byte{0xcf, 0xfa, 0xed, 0xfe, 0x07, 0, 0, 1, 0, 0, 0, 0} // x86_64
	machoPath := filepath.Join(dir, "macho")
	for p, b := range map[string][]byte{pePath: pe, machoPath: macho} {
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	root := repoRoot(t)
	for _, c := range []struct {
		os, arch, path string
		code           int
	}{
		{"windows", "arm64", pePath, 0}, {"windows", "amd64", pePath, 1},
		{"darwin", "amd64", machoPath, 0}, {"darwin", "arm64", machoPath, 1},
	} {
		if r := sh(t, root, nil, "release-inspect.sh", c.os, c.arch, c.path); r.code != c.code {
			t.Errorf("%s/%s: exit %d, want %d: %s", c.os, c.arch, r.code, c.code, r.stderr)
		}
	}
}

func TestNotes(t *testing.T) {
	dir := newRepo(t, "feat: first", "fix: second")
	git(t, dir, "tag", "-a", "-m", "r", "v0.1.0")
	git(t, dir, "tag", "-a", "-m", "r", "v0.0.9", "HEAD~1") // an older stable tag: not the baseline
	commitFile(t, dir, "n", "feat(cli): third")
	git(t, dir, "tag", "-a", "-m", "r", "v0.1.5-rc1") // a newer pre-release: never the baseline
	commitFile(t, dir, "m", "fix: fourth")
	commitFile(t, dir, "k", "docs: fifth")
	commitFile(t, dir, "j", "chore: sixth")
	git(t, dir, "tag", "-a", "-m", "r", "v0.2.0")
	head := git(t, dir, "rev-parse", "HEAD")

	r := sh(t, dir, nil, "release-notes.sh", "v0.2.0", head, "1.27.2")
	if r.code != 0 {
		t.Fatalf("notes failed: %+v", r)
	}
	for _, want := range []string{"ChartScribe v0.2.0", "Commit: " + head, "Go 1.27.2", "cscribe_0.2.0_<os>_<arch>",
		"sha256sum --check --ignore-missing SHA256SUMS", "gh attestation verify", "not signed with a certificate or notarized",
		"Changes since v0.1.0", "### Features\n\n- feat(cli): third (", "### Fixes\n\n- fix: fourth (", "### Documentation\n\n- docs: fifth (",
		"### Other\n\n- chore: sixth ("} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("notes lack %q:\n%s", want, r.stdout)
		}
	}
	for _, unwanted := range []string{"- fix: second", "First release", "v0.1.5-rc1", "Changes since v0.0.9"} {
		if strings.Contains(r.stdout, unwanted) {
			t.Errorf("notes must not contain %q:\n%s", unwanted, r.stdout)
		}
	}
	lower := strings.ToLower(r.stdout)
	for _, word := range []string{"exciting", "amazing", "powerful", "blazing", "!"} {
		if strings.Contains(lower, word) {
			t.Errorf("notes contain marketing wording %q", word)
		}
	}

	first := newRepo(t, "feat: only")
	git(t, first, "tag", "-a", "-m", "r", "v0.1.0")
	r = sh(t, first, nil, "release-notes.sh", "v0.1.0", git(t, first, "rev-parse", "HEAD"), "1.27.2")
	if r.code != 0 || !strings.Contains(r.stdout, "First release.") || strings.Contains(r.stdout, "- feat: only") {
		t.Errorf("first release notes wrong: %+v", r)
	}
	if r := sh(t, first, nil, "release-notes.sh", "v0.1.0"); r.code != 2 {
		t.Errorf("usage error must exit 2: %+v", r)
	}
}
