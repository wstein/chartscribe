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
var ErrNoCollection = errors.New("no collection found (a directory with a zettel/ or notes/ folder)")

// NotesDir returns the relative name of the active note directory.
// A new collection keeps the legacy notes/ default.
func NotesDir(root string) string {
	info, err := os.Stat(filepath.Join(root, "zettel"))
	if err == nil && info.IsDir() {
		return "zettel"
	}
	// Preserve access errors so they cannot silently switch writes to notes/.
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "zettel"
	}
	return "notes"
}

func hasNotes(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, NotesDir(dir)))
	return err == nil && info.IsDir()
}

// FindRoot returns the collection root: explicit when given, else the nearest
// directory from cwd upwards that has a zettel/ or notes/ folder.
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

// ReadNotes reads *.md from the active note directory of the collection (not recursive), sorted by name.
func ReadNotes(root string) ([]File, error) {
	dir := filepath.Join(root, NotesDir(root))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []File
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		// Like the reference (Python read_text), treat \r\n and a lone \r as \n.
		text := strings.ReplaceAll(strings.ReplaceAll(string(data), "\r\n", "\n"), "\r", "\n")
		files = append(files, File{Name: e.Name(), Text: text})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	return files, nil
}
