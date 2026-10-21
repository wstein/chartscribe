// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/spf13/cobra"
	"github.com/wstein/chartscribe/internal/index"
	"github.com/wstein/chartscribe/internal/initcmd"
	"github.com/wstein/chartscribe/internal/note"
)

// now is the clock of new; tests replace it.
var now = time.Now

type newDocument struct {
	Version int      `json:"version"`
	Command string   `json:"command"`
	OK      bool     `json:"ok"`
	Root    string   `json:"root"`
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Path    string   `json:"path"`
	Tags    []string `json:"tags"`
}

// checkTitle refuses a title that cannot be a file name or that validate rejects.
func checkTitle(title string) error {
	if strings.TrimSpace(title) == "" {
		return errors.New("the title is empty")
	}
	if title != strings.TrimSpace(title) {
		return errors.New("the title has leading or trailing space")
	}
	if strings.HasPrefix(title, ".") {
		return errors.New("the title starts with a dot")
	}
	if note.ForbiddenTitle.MatchString(title) {
		return errors.New("forbidden character in the title (one of " + note.ForbiddenTitleChars + ")")
	}
	for _, r := range title {
		if unicode.IsControl(r) {
			return errors.New("control character in the title")
		}
	}
	if len(title)+len(".md") > 255 {
		return errors.New("the title is too long for a file name")
	}
	return nil
}

// freeID returns the id for a note created at t: 14 digits, with -NNN appended
// when a note of the collection already has that id.
func freeID(x *index.Index, t time.Time) string {
	base := t.Format("20060102150405")
	taken := map[string]bool{}
	for _, id := range x.IDs {
		taken[id] = true
	}
	id := base
	for i := 1; taken[id]; i++ {
		id = fmt.Sprintf("%s-%03d", base, i)
	}
	return id
}

func newCommand(opts *globalOptions) *cobra.Command {
	var tags []string
	cmd := &cobra.Command{
		Use:   "new TITLE",
		Short: "Create a note from the template",
		Long: `Create notes/TITLE.md with a fresh id (14 digits from the clock, -NNN when a note
already has it), empty aliases, the --tag tags, a Links heading and an empty Source line.
It refuses an existing file or title and a title with a forbidden character.
The note passes validate once you have filled the tags and the Source line and listed it in a map.
The root is --root, else the nearest directory with a notes/ folder, else the working
directory; notes/ is created when missing. It prints the path of the new file.
Exit status: 0 done, 2 usage error (including a refused title), 3 environment error.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			title := args[0]
			if err := checkTitle(title); err != nil {
				return failure(cmd, opts, exitUsage, "usage", err)
			}
			for _, t := range tags {
				if strings.TrimSpace(t) == "" {
					return failure(cmd, opts, exitUsage, "usage", errors.New("empty --tag"))
				}
			}
			root, err := initRoot(opts.root)
			if err != nil {
				return failure(cmd, opts, exitEnv, "io", err)
			}
			notes := filepath.Join(root, "notes")
			if err := os.MkdirAll(notes, 0o755); err != nil {
				return failure(cmd, opts, exitEnv, "io", err)
			}
			x, err := index.Load(root)
			if err != nil {
				return failure(cmd, opts, exitEnv, "io", err)
			}
			// Every file counts, also one that does not parse and one that differs only by case.
			entries, err := os.ReadDir(notes)
			if err != nil {
				return failure(cmd, opts, exitEnv, "io", err)
			}
			for _, e := range entries {
				if strings.EqualFold(e.Name(), title+".md") {
					return failure(cmd, opts, exitUsage, "usage", fmt.Errorf("notes/%s already exists", e.Name()))
				}
			}
			if n, taken := x.ByName(title); taken {
				return failure(cmd, opts, exitUsage, "usage", fmt.Errorf("%q is already the title or an alias of %s", title, n.Path))
			}
			id := freeID(x, now())
			path := filepath.Join(notes, title+".md")
			f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
			if errors.Is(err, os.ErrExist) {
				return failure(cmd, opts, exitUsage, "usage", fmt.Errorf("notes/%s.md already exists", title))
			}
			if err != nil {
				return failure(cmd, opts, exitEnv, "io", err)
			}
			_, werr := f.WriteString(initcmd.NoteText(id, tags))
			if cerr := f.Close(); werr == nil {
				werr = cerr
			}
			if werr != nil {
				return failure(cmd, opts, exitEnv, "io", werr)
			}
			if opts.json {
				if tags == nil {
					tags = []string{}
				}
				doc := newDocument{Version: 1, Command: "new", OK: true, Root: root, ID: id, Title: title, Path: "notes/" + title + ".md", Tags: tags}
				if err := writeJSON(cmd.OutOrStdout(), doc); err != nil {
					return runtimeFailure{err}
				}
				return nil
			}
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), path); err != nil {
				return runtimeFailure{err}
			}
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&tags, "tag", nil, "Tag for the note (repeatable or comma-separated)")
	return cmd
}
