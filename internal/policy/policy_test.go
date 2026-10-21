// SPDX-License-Identifier: EUPL-1.2

// Package policy holds repository-wide source checks. It has no code.
package policy

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// zkProcess matches exec.Command("zk"...) and exec.CommandContext(ctx, "zk"...).
var zkProcess = regexp.MustCompile(`exec\.Command(\(|Context\(\w+,\s*)"zk"`)

// TestNoZkDependency enforces FR-012: no zk-org module in go.mod or in an
// import, and no zk process started by the sources. zk is GPL-3.0 and is only
// ever a separate program the user runs.
func TestNoZkDependency(t *testing.T) {
	root := filepath.Join("..", "..")
	mod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(mod), "zk-org") {
		t.Error("go.mod names a zk-org module")
	}
	self := "policy_test.go"
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "testdata" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || filepath.Base(path) == self {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(src), "zk-org") {
			t.Errorf("%s names zk-org", path)
		}
		if zkProcess.Match(src) {
			t.Errorf("%s starts a zk process", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestPolicyDetectsViolations proves the scanner matches what it must catch.
func TestPolicyDetectsViolations(t *testing.T) {
	for _, src := range []string{`exec.Command("zk", "list")`, `exec.CommandContext(ctx, "zk")`} {
		if !zkProcess.MatchString(src) {
			t.Errorf("pattern misses %s", src)
		}
	}
	if zkProcess.MatchString(`exec.Command("gh", "zk")`) {
		t.Error("pattern matches a later argument")
	}
}
