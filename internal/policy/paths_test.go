// SPDX-License-Identifier: EUPL-1.2

package policy

import (
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
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

// moduleFilePathProblem reimplements golang.org/x/mod/module.CheckFilePath (v0.42.0, module.go:
// CheckFilePath, checkPath, checkElem, fileNameOK), the rule that module zip creation applies to
// every file of a module. The Go module proxy cannot serve a version that holds a path it
// rejects, and `go install pkg@version` then fails with "malformed file path". It returns why a
// path is rejected, or "". x/mod is not a dependency of this module; the rule is small and is
// proven against the real package by the check recorded in specs/002-release.
func moduleFilePathProblem(p string) string {
	if !utf8.ValidString(p) {
		return "invalid UTF-8"
	}
	if p == "" || strings.Contains(p, "//") || strings.HasSuffix(p, "/") {
		return "empty path, double slash or trailing slash"
	}
	for _, elem := range strings.Split(p, "/") {
		if strings.Count(elem, ".") == len(elem) {
			return "path element " + strconv.Quote(elem) + " is empty or only dots"
		}
		if strings.HasSuffix(elem, ".") {
			return "trailing dot in a path element"
		}
		for _, r := range elem {
			if !moduleFileNameOK(r) {
				return "invalid char " + strconv.QuoteRune(r)
			}
		}
		short, _, _ := strings.Cut(elem, ".")
		for _, bad := range []string{"CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
			"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9"} {
			if strings.EqualFold(bad, short) {
				return "reserved Windows name " + strconv.Quote(short)
			}
		}
	}
	return ""
}

// moduleFileNameOK is fileNameOK of x/mod/module: ASCII letters, digits and !#$%&()+,-.=@[]^_{}~
// and the space, and any non-ASCII Unicode letter. Symbols such as the em dash are not letters.
func moduleFileNameOK(r rune) bool {
	if r < utf8.RuneSelf {
		return '0' <= r && r <= '9' || 'A' <= r && r <= 'Z' || 'a' <= r && r <= 'z' || strings.ContainsRune("!#$%&()+,-.=@[]^_{}~ ", r)
	}
	return unicode.IsLetter(r)
}

func TestModulePathRule(t *testing.T) {
	for _, bad := range []string{"notes/M0 — x.md", "a/b c", "a/Bad: t.md", "a/it's.md", "a/x*.md", "a/é.md™", "dir./x", "a//b", "a/", "", "a/..", "a/CON.txt", "a/b\xff"} {
		if moduleFilePathProblem(bad) == "" {
			t.Errorf("%q must be rejected", bad)
		}
	}
	for _, ok := range []string{"notes/M0 - Établir les contrats.md", "a/b c.go", "x/%E2%80%94.md", "go.mod", ".github/x.yml", "a/münchen.md", "a/{b}~[c]^=@.md"} {
		if p := moduleFilePathProblem(ok); p != "" {
			t.Errorf("%q rejected: %s", ok, p)
		}
	}
}

// TestTrackedPathsAreValidInAModuleZip keeps `go install github.com/wstein/chartscribe/cmd/cscribe@version`
// working: no tracked path may break module zip creation (v0.1.0 holds notes named with an em dash and
// the Go proxy cannot serve it). Store such a name percent-encoded (%E2%80%94) and decode it at test time.
func TestTrackedPathsAreValidInAModuleZip(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("SKIPPED, NOT PASSED: git is not available")
	}
	cmd := exec.Command("git", "ls-files", "-z")
	cmd.Dir = "../.."
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("SKIPPED, NOT PASSED: not a git checkout: %v", err)
	}
	n := 0
	for _, p := range strings.Split(strings.TrimRight(string(out), "\x00"), "\x00") {
		if p == "" {
			continue
		}
		n++
		if why := moduleFilePathProblem(p); why != "" {
			t.Errorf("tracked path %q breaks the Go module zip (go install @version): %s", p, why)
		}
	}
	if n == 0 {
		t.Error("git ls-files listed nothing")
	}
}
