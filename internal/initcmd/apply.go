// SPDX-License-Identifier: EUPL-1.2

package initcmd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Action is what init does with one file.
type Action string

// The plan actions. Overwrite is a write over a different existing file that
// --overwrite named.
const (
	Write     Action = "write"
	Exists    Action = "exists (skipped)"
	Differs   Action = "differs (skipped)"
	Overwrite Action = "overwrite"
)

// Entry is one planned file.
type Entry struct {
	Path   string `json:"path"`
	Action Action `json:"action"`
}

// Errors of Plan and Apply.
var (
	// ErrUnknownOverwrite means --overwrite names a path outside the file set.
	ErrUnknownOverwrite = errors.New("--overwrite names no file of the selected set")
	// ErrUnsafe means a target or one of its directories is a symlink or not
	// the expected kind of file; init never writes through those.
	ErrUnsafe = errors.New("refusing to write through a symlink or over a directory")
)

func cleanRel(p string) string {
	p = path.Clean(strings.ReplaceAll(p, "\\", "/"))
	return strings.TrimPrefix(p, "./")
}

// Plan decides the action for every file below root without writing anything.
// overwrite lists paths (or "all") that may replace a different existing file.
func Plan(root string, files []File, overwrite []string) ([]Entry, error) {
	all := false
	named := map[string]bool{}
	for _, o := range overwrite {
		if o == "all" {
			all = true
			continue
		}
		named[cleanRel(o)] = true
	}
	for p := range named {
		found := false
		for _, f := range files {
			found = found || f.Path == p
		}
		if !found {
			return nil, fmt.Errorf("%w: %q", ErrUnknownOverwrite, p)
		}
	}
	var entries []Entry
	for _, f := range files {
		state, err := inspect(root, f.Path)
		if err != nil {
			return nil, err
		}
		e := Entry{Path: f.Path}
		switch {
		case !state.exists:
			e.Action = Write
		case state.content == f.Content:
			e.Action = Exists
		case all || named[f.Path]:
			e.Action = Overwrite
		default:
			e.Action = Differs
		}
		entries = append(entries, e)
	}
	return entries, nil
}

type fileState struct {
	exists  bool
	content string
}

// inspect checks that no directory on the way and not the target is a symlink
// and that the target, if present, is a regular file.
func inspect(root, rel string) (fileState, error) {
	cur := root
	parts := strings.Split(rel, "/")
	for i, part := range parts {
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		if errors.Is(err, fs.ErrNotExist) {
			return fileState{}, nil
		}
		if err != nil {
			return fileState{}, err
		}
		last := i == len(parts)-1
		if info.Mode()&os.ModeSymlink != 0 || (last && !info.Mode().IsRegular()) || (!last && !info.IsDir()) {
			return fileState{}, fmt.Errorf("%w: %s", ErrUnsafe, rel)
		}
	}
	data, err := os.ReadFile(cur)
	if err != nil {
		return fileState{}, err
	}
	return fileState{true, string(data)}, nil
}

// Apply performs the Write and Overwrite entries. A new file is created
// exclusively; a replacement goes through a temporary file and a rename, so a
// failure leaves the old file intact. Targets are re-checked for symlinks.
func Apply(root string, files []File, entries []Entry) error {
	content := map[string]string{}
	for _, f := range files {
		content[f.Path] = f.Content
	}
	for _, e := range entries {
		if e.Action != Write && e.Action != Overwrite {
			continue
		}
		text, ok := content[e.Path]
		if !ok {
			return fmt.Errorf("%w: %q", ErrUnknownOverwrite, e.Path)
		}
		if _, err := inspect(root, e.Path); err != nil {
			return err
		}
		full := filepath.Join(root, filepath.FromSlash(e.Path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if _, err := inspect(root, e.Path); err != nil {
			return err
		}
		var err error
		if e.Action == Write {
			err = createNew(full, text)
		} else {
			err = replace(full, text)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func createNew(full, text string) error {
	f, err := os.OpenFile(full, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(text); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func replace(full, text string) error {
	tmp, err := os.CreateTemp(filepath.Dir(full), ".cscribe-init-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.WriteString(text); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, full)
}
