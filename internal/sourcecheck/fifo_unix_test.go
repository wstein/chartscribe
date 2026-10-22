// SPDX-License-Identifier: EUPL-1.2

//go:build unix

package sourcecheck

import (
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestNamedPipeIsNamed(t *testing.T) {
	root := tree(t)
	if err := syscall.Mkfifo(filepath.Join(root, "docs", "pipe"), 0o644); err != nil {
		t.Skip("mkfifo unavailable")
	}
	p := Check(root, file("docs/pipe", 1, 1))
	if p == nil || p.Rule != "source-not-file" || !strings.Contains(p.Message, "a named pipe") {
		t.Errorf("got %+v", p)
	}
}
