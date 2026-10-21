// SPDX-License-Identifier: EUPL-1.2

package fix

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Apply writes the planned files below root. Each file is written to a temporary file in its own
// directory and renamed over the original. A file that changed since planning, a symlink, or a path
// that leaves the collection stops the write of that file with an error; files written before stay written.
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
	for _, file := range files {
		if err := writeOne(rootReal, file, p.orig[file], p.final[file]); err != nil {
			return fmt.Errorf("%s: %w", file, err)
		}
	}
	return nil
}

func writeOne(rootReal, file, orig, text string) error {
	path := filepath.Join(rootReal, filepath.FromSlash(file))
	dirReal, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return err
	}
	if dirReal != filepath.Dir(path) {
		return errors.New("path leaves the collection")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("not a regular file (symlinks are refused)")
	}
	current, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if string(current) != orig {
		return errors.New("changed since the plan; run again")
	}
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
	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return err
	}
	done = true
	return nil
}
