// SPDX-License-Identifier: EUPL-1.2

package validate

import (
	"os"
	"path/filepath"
	"testing"
)

// caseInsensitive reports whether dir's filesystem folds the case of names.
func caseInsensitive(t *testing.T, dir string) bool {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "probe-case.tmp"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(filepath.Join(dir, "probe-case.tmp"))
	_, err := os.Stat(filepath.Join(dir, "PROBE-CASE.TMP"))
	return err == nil
}
