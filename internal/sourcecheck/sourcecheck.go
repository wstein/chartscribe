// SPDX-License-Identifier: EUPL-1.2

// Package sourcecheck verifies that a cited source file exists below the
// source root and that its line range is real. It only reads the file system:
// no process, no network.
package sourcecheck

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/wstein/chartscribe/internal/note"
)

// Problem is one failed check; Rule is a validate rule name.
type Problem struct {
	Rule    string
	Message string
}

// Check verifies a file or measurement source against srcRoot. Other kinds are
// never checked (nothing is fetched). A nil result means the source holds.
func Check(srcRoot string, s note.Source) *Problem {
	if !s.Verifiable() {
		return nil
	}
	abs, p := Resolve(srcRoot, s.Path)
	if p != nil || s.Kind != note.KindFile {
		return p
	}
	n, err := countLines(abs)
	if err != nil {
		return &Problem{"source-file-missing", fmt.Sprintf("Source file %q cannot be read: %v", s.Path, err)}
	}
	switch {
	case s.From < 1:
		return &Problem{"source-range-invalid", fmt.Sprintf("Source %q lines %d-%d: line numbers start at 1", s.Path, s.From, s.To)}
	case s.From > s.To:
		return &Problem{"source-range-invalid", fmt.Sprintf("Source %q lines %d-%d: the range starts after its end", s.Path, s.From, s.To)}
	case s.To > n:
		return &Problem{"source-range-invalid", fmt.Sprintf("Source %q lines %d-%d: the file has %d lines", s.Path, s.From, s.To, n)}
	}
	return nil
}

// Resolve returns the real absolute path of rel (a slash path relative to
// srcRoot), or the problem: outside the root (a "..", an absolute path or a
// symbolic link that leaves it), missing, or not a regular file.
func Resolve(srcRoot, rel string) (string, *Problem) {
	outside := &Problem{"source-outside-root", fmt.Sprintf("Source path %q is outside the source root", rel)}
	if filepath.IsAbs(rel) || strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, `\`) || filepath.VolumeName(rel) != "" {
		return "", outside
	}
	clean := filepath.Clean(filepath.FromSlash(rel))
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", outside
	}
	missing := func(err error) (string, *Problem) {
		if os.IsNotExist(err) {
			return "", &Problem{"source-file-missing", fmt.Sprintf("Source file %q not found under the source root", rel)}
		}
		return "", &Problem{"source-file-missing", fmt.Sprintf("Source file %q cannot be read: %v", rel, err)}
	}
	realRoot, err := filepath.EvalSymlinks(srcRoot)
	if err != nil {
		return missing(err)
	}
	real, err := filepath.EvalSymlinks(filepath.Join(realRoot, clean))
	if err != nil {
		return missing(err)
	}
	if r, err := filepath.Rel(realRoot, real); err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return "", outside
	}
	info, err := os.Stat(real)
	if err != nil {
		return missing(err)
	}
	if !info.Mode().IsRegular() {
		kind := "not a regular file"
		switch m := info.Mode(); {
		case m.IsDir():
			kind = "a directory"
		case m&os.ModeNamedPipe != 0:
			kind = "a named pipe"
		case m&os.ModeSocket != 0:
			kind = "a socket"
		case m&os.ModeDevice != 0:
			kind = "a device"
		}
		return "", &Problem{"source-not-file", fmt.Sprintf("Source path %q is %s, not a file", rel, kind)}
	}
	return real, nil
}

// countLines returns the number of lines of the file: the newlines plus a last
// line without one. An empty file has none.
func countLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	buf := make([]byte, 64*1024)
	n := 0
	var last byte = '\n'
	for {
		k, err := f.Read(buf)
		for _, b := range buf[:k] {
			if b == '\n' {
				n++
			}
		}
		if k > 0 {
			last = buf[k-1]
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, err
		}
	}
	if last != '\n' {
		n++
	}
	return n, nil
}
