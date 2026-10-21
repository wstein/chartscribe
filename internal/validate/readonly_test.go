// SPDX-License-Identifier: EUPL-1.2

package validate

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func treeHash(t *testing.T, root string) string {
	t.Helper()
	h := sha256.New()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		info, err := d.Info()
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%s %v %d\n", rel, info.Mode(), info.ModTime().UnixNano())
		if !d.IsDir() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			h.Write(data)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// FR-005: validate only reads. A read-only collection validates and stays unchanged.
func TestValidateLeavesTheTreeUnchanged(t *testing.T) {
	root := newFixture(t)
	writeNote(t, root, "Broken", "bad", noteOpts{body: "[[Ghost]]"})
	// Make the tree read-only so that any write attempt fails loudly.
	var dirs []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			dirs = append(dirs, path)
		} else {
			_ = os.Chmod(path, 0o444)
		}
		return nil
	})
	for i := len(dirs) - 1; i >= 0; i-- {
		_ = os.Chmod(dirs[i], 0o555)
	}
	t.Cleanup(func() {
		for _, d := range dirs {
			_ = os.Chmod(d, 0o755)
		}
	})
	readOnlyBefore := treeHash(t, root)
	if fs := check(t, root); len(fs) == 0 {
		t.Fatal("want findings on the broken note")
	}
	if after := treeHash(t, root); after != readOnlyBefore {
		t.Error("validate changed the tree")
	}
}

// FR-005: the validator has no network and no process start in its dependency closure.
func TestValidateDependencies(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".").CombinedOutput()
	if err != nil {
		t.Skipf("go list unavailable: %v\n%s", err, out)
	}
	for _, pkg := range strings.Fields(string(out)) {
		if pkg == "net" || strings.HasPrefix(pkg, "net/") || pkg == "os/exec" {
			t.Errorf("internal/validate depends on %s", pkg)
		}
	}
}
