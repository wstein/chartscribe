// SPDX-License-Identifier: EUPL-1.2

package policy

import (
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

var reservedWindowsName = regexp.MustCompile(`(?i)^(con|prn|aux|nul|com[1-9]|lpt[1-9])$`)

// windowsPathProblem returns why a repository path cannot be checked out on Windows, or "".
func windowsPathProblem(p string) string {
	for _, part := range strings.Split(p, "/") {
		if strings.ContainsAny(part, `<>:"\|?*`) {
			return "contains one of < > : \" \\ | ? *"
		}
		if strings.IndexFunc(part, func(r rune) bool { return r < 0x20 }) >= 0 {
			return "contains a control character"
		}
		if strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return "ends in a dot or a space"
		}
		if reservedWindowsName.MatchString(strings.SplitN(part, ".", 2)[0]) {
			return "uses a reserved device name"
		}
	}
	return ""
}

func TestWindowsPathRules(t *testing.T) {
	for _, bad := range []string{"a/Bad: title.md", "a/b?.md", "dir./x", "x /y", "notes/CON.md", "aux", "a/com1.txt", "a/LPT9", "a\\b", "a/\x01"} {
		if windowsPathProblem(bad) == "" {
			t.Errorf("%q should be invalid on Windows", bad)
		}
	}
	for _, ok := range []string{"notes/M0 — Establish contracts.md", "a/b.go", ".github/x.yml", "a/console.md", "a/com10.txt"} {
		if p := windowsPathProblem(ok); p != "" {
			t.Errorf("%q rejected: %s", ok, p)
		}
	}
}

// TestTrackedPathsCheckOutOnWindows keeps the repository cloneable on Windows, where the
// release smoke job checks it out: no tracked path may hold a character, name or ending
// that Windows refuses, and no two paths may differ only in case. Generate such a file at
// test time instead (see internal/validate fixtures: a %XX in a fixture name is decoded).
func TestTrackedPathsCheckOutOnWindows(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("SKIPPED, NOT PASSED: git is not available")
	}
	cmd := exec.Command("git", "ls-files", "-z")
	cmd.Dir = "../.."
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("SKIPPED, NOT PASSED: not a git checkout: %v", err)
	}
	seen := map[string]string{}
	n := 0
	for _, p := range strings.Split(strings.TrimRight(string(out), "\x00"), "\x00") {
		if p == "" {
			continue
		}
		n++
		if why := windowsPathProblem(p); why != "" {
			t.Errorf("tracked path %q cannot be checked out on Windows: %s", p, why)
		}
		if other, dup := seen[strings.ToLower(p)]; dup {
			t.Errorf("tracked paths %q and %q differ only in case", p, other)
		}
		seen[strings.ToLower(p)] = p
	}
	if n == 0 {
		t.Error("git ls-files listed nothing")
	}
}
