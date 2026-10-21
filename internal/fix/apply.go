// SPDX-License-Identifier: EUPL-1.2

package fix

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wstein/chartscribe/internal/validate"
)

// renameFile is os.Rename; tests replace it to fail a write.
var renameFile = os.Rename

// WriteError says which files an Apply wrote before it stopped.
type WriteError struct {
	Cause      error
	Written    []string // written and not restored
	Restored   []string // written, then put back
	NotWritten []string
}

func (e *WriteError) Error() string {
	var b strings.Builder
	b.WriteString(e.Cause.Error())
	list := func(label string, files []string) {
		if len(files) > 0 {
			fmt.Fprintf(&b, "; %s: %s", label, strings.Join(files, ", "))
		}
	}
	list("written", e.Written)
	list("restored", e.Restored)
	list("not written", e.NotWritten)
	return b.String()
}

func (e *WriteError) Unwrap() error { return e.Cause }

// Apply writes the planned files below root, all or none as far as the file system allows: the whole
// result is verified once more, every target is checked (unchanged since the plan, a regular file inside
// the collection, directory writable), and only then are files written, each through a temporary file
// in its own directory and a rename. If a write fails, the files already written are restored from the
// plan; the error names what was written, restored and not written.
func (p *Plan) Apply(root string) error {
	rootReal, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	files := make([]string, 0, len(p.final))
	for file := range p.final {
		files = append(files, file)
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil
	}
	overlay := validate.Overlay{}
	for _, file := range files {
		overlay[file] = p.final[file]
	}
	res, err := validate.RunOverlay(root, overlay)
	if err != nil {
		return err
	}
	if !better(p.Before, res.Findings) {
		return &WriteError{Cause: errors.New("the result no longer validates better; run again"), NotWritten: files}
	}
	modes := map[string]os.FileMode{}
	for _, file := range files {
		mode, err := check(rootReal, file, p.orig[file])
		if err != nil {
			return &WriteError{Cause: fmt.Errorf("%s: %w", file, err), NotWritten: files}
		}
		modes[file] = mode
	}
	var written []string
	for i, file := range files {
		if err := writeFile(rootReal, file, p.final[file], modes[file]); err != nil {
			werr := &WriteError{Cause: fmt.Errorf("%s: %w", file, err), NotWritten: files[i:]}
			for _, w := range written {
				if rerr := writeFile(rootReal, w, p.orig[w], modes[w]); rerr != nil {
					werr.Written = append(werr.Written, w)
				} else {
					werr.Restored = append(werr.Restored, w)
				}
			}
			return werr
		}
		written = append(written, file)
	}
	return nil
}

func targetPath(rootReal, file string) string {
	return filepath.Join(rootReal, filepath.FromSlash(file))
}

// check verifies that a target is still the planned file and can be replaced; it returns its permissions.
func check(rootReal, file, orig string) (os.FileMode, error) {
	path := targetPath(rootReal, file)
	dirReal, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return 0, err
	}
	if dirReal != filepath.Dir(path) {
		return 0, errors.New("path leaves the collection")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return 0, err
	}
	if !info.Mode().IsRegular() {
		return 0, errors.New("not a regular file (symlinks are refused)")
	}
	current, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	if string(current) != orig {
		return 0, errors.New("changed since the plan; run again")
	}
	probe, err := os.CreateTemp(filepath.Dir(path), ".cscribe-fix-*")
	if err != nil {
		return 0, err
	}
	probe.Close()
	if err := os.Remove(probe.Name()); err != nil {
		return 0, err
	}
	return info.Mode().Perm(), nil
}

// writeFile replaces a file by text through a temporary file in the same directory and a rename.
func writeFile(rootReal, file, text string, mode os.FileMode) error {
	path := targetPath(rootReal, file)
	tmp, err := os.CreateTemp(filepath.Dir(path), ".cscribe-fix-*")
	if err != nil {
		return err
	}
	done := false
	defer func() {
		if !done {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()
	if _, err := tmp.WriteString(text); err != nil {
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := renameFile(tmp.Name(), path); err != nil {
		return err
	}
	done = true
	return nil
}
