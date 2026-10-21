// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"errors"
	"fmt"
	"os"

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
}

func exportCommand(opts *globalOptions) *cobra.Command {
	var format, out string
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

The document goes to stdout, or with --out FILE to that file, which is never overwritten
without --force. Nothing else is written. Unresolved links are warned on stderr. With --json
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
			x, err := loadIndex(cmd, opts)
			if err != nil {
				return err
			}
			start, err := findNote(cmd, opts, x, args[0])
			if err != nil {
				return err
			}
			res := export.Build(x, start, depth)
			if toStdout {
				if _, err := fmt.Fprint(cmd.OutOrStdout(), res.Markdown); err != nil {
					return runtimeFailure{err}
				}
			} else if err := writeExport(out, res.Markdown, force); err != nil {
				if errors.Is(err, os.ErrExist) {
					return failure(cmd, opts, exitUsage, "usage", fmt.Errorf("%s exists (use --force to replace it)", out))
				}
				return failure(cmd, opts, exitEnv, "io", err)
			}
			if opts.json {
				doc := exportDocument{1, "export", true, "md", depth, toJSONNote(start), res.Notes, res.Unresolved}
				if doc.Unresolved == nil {
					doc.Unresolved = []export.Unresolved{}
				}
				return finish(writeJSON(cmd.OutOrStdout(), doc))
			}
			for _, u := range res.Unresolved {
				fmt.Fprintf(cmd.ErrOrStderr(), "unresolved [[%s]] in %s\n", u.Target, u.Note)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "md", "Output format (md)")
	cmd.Flags().StringVar(&out, "out", "-", "Write to FILE instead of stdout (- is stdout)")
	cmd.Flags().IntVar(&depth, "depth", 1, "Levels of links to follow from the note")
	cmd.Flags().BoolVar(&force, "force", false, "Replace an existing --out file")
	return cmd
}

// writeExport creates path with text; an existing file is replaced only with force.
func writeExport(path, text string, force bool) error {
	flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if force {
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}
	f, err := os.OpenFile(path, flags, 0o644)
	if err != nil {
		return err
	}
	_, werr := f.WriteString(text)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	return werr
}
