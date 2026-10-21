// SPDX-License-Identifier: EUPL-1.2

package collection

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ErrNoCollection means no collection root was found.
var ErrNoCollection = errors.New("no collection found (a directory with a notes/ folder)")

func hasNotes(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "notes"))
	return err == nil && info.IsDir()
}

// FindRoot returns the collection root: explicit when given, else the nearest
// directory from cwd upwards that has a notes/ folder. A root without a notes/
// folder is ErrNoCollection.
func FindRoot(explicit, cwd string) (string, error) {
	if explicit != "" {
		abs, err := filepath.Abs(explicit)
		if err != nil {
			return "", err
		}
		if !hasNotes(abs) {
			return "", fmt.Errorf("%w: %s", ErrNoCollection, explicit)
		}
		return abs, nil
	}
	dir, err := filepath.Abs(cwd)
	if err != nil {
		return "", err
	}
	for {
		if hasNotes(dir) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ErrNoCollection
		}
		dir = parent
	}
}

// File is one note file as read from disk.
type File struct {
	Name string // file name, for example "Idea.md"
	Text string
}

// Title is the note title: the file name without .md.
func (f File) Title() string { return strings.TrimSuffix(f.Name, ".md") }

// ReadNotes reads notes/*.md of the collection (not recursive), sorted by name.
func ReadNotes(root string) ([]File, error) {
	entries, err := os.ReadDir(filepath.Join(root, "notes"))
	if err != nil {
		return nil, err
	}
	var files []File
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, "notes", e.Name()))
		if err != nil {
			return nil, err
		}
		files = append(files, File{Name: e.Name(), Text: string(data)})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	return files, nil
}
