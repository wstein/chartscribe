// SPDX-License-Identifier: EUPL-1.2

// Package fixtures supports tests that read the collections under testdata/validate.
// It is used by tests only.
package fixtures

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Materialize returns dir itself, or, when a file name in it holds %XX escapes, a temporary
// copy with the names decoded. A fixture file whose name Windows cannot hold (a colon in
// "Bad: title.md") is stored as "Bad%3A title.md" and created at test time, and likewise a name with an em dash (%E2%80%94), which the Go module zip rejects; a filesystem that
// cannot create it skips the test with a message instead of failing.
func Materialize(t *testing.T, dir string) string {
	t.Helper()
	escaped := false
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err == nil && strings.Contains(d.Name(), "%") {
			escaped = true
		}
		return nil
	})
	if !escaped {
		return dir
	}
	dst := t.TempDir()
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		parts := strings.Split(rel, string(filepath.Separator))
		for i, p := range parts {
			if dec, derr := url.PathUnescape(p); derr == nil {
				parts[i] = dec
			}
		}
		target := filepath.Join(append([]string{dst}, parts...)...)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			t.Skipf("SKIPPED, NOT PASSED: this filesystem cannot create %q: %v", filepath.Join(parts...), err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}
