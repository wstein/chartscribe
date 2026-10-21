// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/wstein/chartscribe/internal/export"
)

type exportDocument struct {
	Version    int                 `json:"version"`
	Command    string              `json:"command"`
	OK         bool                `json:"ok"`
	Format     string              `json:"format"`
	Depth      int                 `json:"depth"`
	Start      jsonNote            `json:"start"`
	Notes      []export.Included   `json:"notes"`
	Unresolved []export.Unresolved `json:"unresolved"`
	Warnings   []string            `json:"warnings"`
}

func exportCommand(opts *globalOptions) *cobra.Command {
	var format, out, title string
	var depth int
	var force bool
	cmd := &cobra.Command{
		Use:   "export NOTE",
		Short: "Generate one Markdown document from a note and the notes it links to",
		Long: `Generate one Markdown document from NOTE (a title, alias, file name or id), usually a
map. The title is the note's; the notes its wikilinks name follow, in link order (map order),
each once, as heading sections: level 2 for the first --depth level (default 1), level 3 for
the next. Frontmatter, the Links section and the Source lines are left out; the Source lines
are collected, without duplicates, in a final Sources section. A wikilink to a note in the
document becomes an anchor link; any other wikilink becomes its text. The output has no
timestamps: the same notes give the same bytes. Only Markdown (--format md) is generated;
render it with Hugo, Quartz or pandoc.

--title TEXT replaces the document title. Wikilinks in code and in the Links section do not
make a note a member.

The document goes to stdout, or with --out FILE to that file, which is never overwritten
without --force. --out is refused (exit 2) inside the collection's notes/ tree and at a
symbolic link, even with --force; a missing directory or a directory as FILE is exit 3.
Nothing else is written. Unresolved links and unclosed code fences are warned on stderr. With --json
(which needs --out FILE) one document lists the included notes and the unresolved links.
Exit status: 0 done, 2 usage error (including a refused overwrite), 3 environment error.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if format != "md" {
				return failure(cmd, opts, exitUsage, "usage", fmt.Errorf("unsupported --format %q (only md)", format))
			}
			if depth < 0 {
				return failure(cmd, opts, exitUsage, "usage", errors.New("--depth must be 0 or more"))
			}
			toStdout := out == "" || out == "-"
			if opts.json && toStdout {
				return failure(cmd, opts, exitUsage, "usage", errors.New("--json needs --out FILE: stdout would carry both documents"))
			}
			root, x, err := loadCollection(cmd, opts)
			if err != nil {
				return err
			}
			start, err := findNote(cmd, opts, x, args[0])
			if err != nil {
				return err
			}
			target := ""
			if !toStdout {
				if target, err = checkOut(root, out); err != nil {
					return outFailure(cmd, opts, err)
				}
			}
			res := export.Build(x, start, depth, title)
			if toStdout {
				if _, err := fmt.Fprint(cmd.OutOrStdout(), res.Markdown); err != nil {
					return runtimeFailure{err}
				}
			} else if err := writeExport(target, res.Markdown, force); err != nil {
				if errors.Is(err, os.ErrExist) {
					return failure(cmd, opts, exitUsage, "usage", fmt.Errorf("%s exists (use --force to replace it)", out))
				}
				return failure(cmd, opts, exitEnv, "io", err)
			}
			if opts.json {
				doc := exportDocument{1, "export", true, "md", depth, toJSONNote(start), res.Notes, res.Unresolved, res.Warnings}
				if doc.Warnings == nil {
					doc.Warnings = []string{}
				}
				if doc.Unresolved == nil {
					doc.Unresolved = []export.Unresolved{}
				}
				return finish(writeJSON(cmd.OutOrStdout(), doc))
			}
			for _, u := range res.Unresolved {
				fmt.Fprintf(cmd.ErrOrStderr(), "unresolved [[%s]] in %s\n", u.Target, u.Note)
			}
			for _, w := range res.Warnings {
				fmt.Fprintln(cmd.ErrOrStderr(), w)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "md", "Output format (md)")
	cmd.Flags().StringVar(&out, "out", "-", "Write to FILE instead of stdout (- is stdout)")
	cmd.Flags().StringVar(&title, "title", "", "Document title (default: the title of the note)")
	cmd.Flags().IntVar(&depth, "depth", 1, "Levels of links to follow from the note")
	cmd.Flags().BoolVar(&force, "force", false, "Replace an existing --out file")
	return cmd
}

// usageError marks a refused --out (exit 2); other errors are environment errors.
type usageError struct{ error }

func outFailure(cmd *cobra.Command, opts *globalOptions, err error) error {
	var u usageError
	if errors.As(err, &u) {
		return failure(cmd, opts, exitUsage, "usage", u.error)
	}
	return failure(cmd, opts, exitEnv, "io", err)
}

// checkOut resolves --out to the path to write and refuses what export must
// never touch: anything inside the collection's notes/ tree (after cleaning and
// resolving symlinks of the parent directory) and a symlink as the target, with
// or without --force. A missing parent directory or a directory as the target
// is an environment error.
func checkOut(root, out string) (string, error) {
	abs, err := filepath.Abs(out)
	if err != nil {
		return "", err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return "", err
	}
	target := filepath.Join(parent, filepath.Base(abs))
	notes, err := filepath.EvalSymlinks(filepath.Join(root, "notes"))
	if err != nil {
		return "", err
	}
	if hit, err := touchesNotes(notes, parent, target); err != nil {
		return "", err
	} else if hit {
		return "", usageError{fmt.Errorf("--out %s is inside the notes of the collection; export never writes there", out)}
	}
	info, err := os.Lstat(target)
	switch {
	case err == nil && info.Mode()&os.ModeSymlink != 0:
		return "", usageError{fmt.Errorf("--out %s is a symbolic link; export does not follow or replace links", out)}
	case err == nil && info.IsDir():
		return "", fmt.Errorf("--out %s is a directory", out)
	case err != nil && !errors.Is(err, os.ErrNotExist):
		return "", err
	}
	return target, nil
}

// touchesNotes reports whether writing target (in dir) would land in the notes
// directory or on a note. It decides by file identity (os.SameFile on dir and
// each of its existing ancestors against notes, and on target against every
// note file), so aliases through links or a case-insensitive file system are
// caught; the case-insensitive path comparison is a second guard.
func touchesNotes(notes, dir, target string) (bool, error) {
	if insideFold(notes, dir) {
		return true, nil
	}
	ninfo, err := os.Stat(notes)
	if err != nil {
		return false, err
	}
	for d := dir; ; {
		if info, err := os.Stat(d); err == nil && os.SameFile(info, ninfo) {
			return true, nil
		}
		up := filepath.Dir(d)
		if up == d {
			break
		}
		d = up
	}
	tinfo, err := os.Lstat(target)
	if err != nil {
		return false, nil // not there yet
	}
	entries, err := os.ReadDir(notes)
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		if info, err := os.Stat(filepath.Join(notes, e.Name())); err == nil && os.SameFile(info, tinfo) {
			return true, nil
		}
	}
	return false, nil
}

// insideFold reports whether dir is base or below it, comparing the path
// components without regard to case.
func insideFold(base, dir string) bool {
	b := strings.Split(filepath.Clean(base), string(filepath.Separator))
	d := strings.Split(filepath.Clean(dir), string(filepath.Separator))
	if len(d) < len(b) {
		return false
	}
	for i := range b {
		if !strings.EqualFold(b[i], d[i]) {
			return false
		}
	}
	return true
}

// writeExport writes text to path (already checked, never a link or a
// directory). Without force it creates the file and refuses an existing one;
// with force it writes a temporary file in the same directory and renames it
// over the target, so the replacement is atomic and nothing else remains.
func writeExport(path, text string, force bool) error {
	if !force {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return err
		}
		_, werr := f.WriteString(text)
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		return werr
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".cscribe-export-*")
	if err != nil {
		return err
	}
	_, werr := tmp.WriteString(text)
	if cerr := tmp.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Chmod(tmp.Name(), 0o644)
	}
	if werr == nil {
		werr = os.Rename(tmp.Name(), path)
	}
	if werr != nil {
		os.Remove(tmp.Name())
	}
	return werr
}
