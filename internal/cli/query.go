// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/wstein/chartscribe/internal/collection"
	"github.com/wstein/chartscribe/internal/index"
)

// jsonNote is the record of a note in the JSON output.
type jsonNote struct {
	ID    string   `json:"id"`
	Title string   `json:"title"`
	Path  string   `json:"path"`
	Tags  []string `json:"tags"`
}

func toJSONNote(n index.Note) jsonNote {
	return jsonNote{ID: n.ID, Title: n.Title, Path: n.Path, Tags: n.Tags}
}

func jsonNotes(in []index.Note) []jsonNote {
	out := []jsonNote{}
	for _, n := range in {
		out = append(out, toJSONNote(n))
	}
	return out
}

type listDocument struct {
	Version int        `json:"version"`
	Command string     `json:"command"`
	OK      bool       `json:"ok"`
	Notes   []jsonNote `json:"notes"`
}

type tagsDocument struct {
	Version int              `json:"version"`
	Command string           `json:"command"`
	OK      bool             `json:"ok"`
	Tags    []index.TagCount `json:"tags"`
}

type jsonLink struct {
	Target   string `json:"target"`
	Resolved bool   `json:"resolved"`
	ID       string `json:"id,omitempty"`
	Title    string `json:"title,omitempty"`
	Path     string `json:"path,omitempty"`
}

type linksDocument struct {
	Version    int        `json:"version"`
	Command    string     `json:"command"`
	OK         bool       `json:"ok"`
	Note       jsonNote   `json:"note"`
	Links      []jsonLink `json:"links"`
	Unresolved int        `json:"unresolved"`
}

type backlinksDocument struct {
	Version   int        `json:"version"`
	Command   string     `json:"command"`
	OK        bool       `json:"ok"`
	Note      jsonNote   `json:"note"`
	Backlinks []jsonNote `json:"backlinks"`
}

// loadIndex finds the collection and reads it. Notes whose frontmatter does not
// parse are skipped with a warning on stderr; validate reports them.
func loadIndex(cmd *cobra.Command, opts *globalOptions) (*index.Index, error) {
	_, x, err := loadCollection(cmd, opts)
	return x, err
}

// loadCollection is loadIndex that also returns the collection root.
func loadCollection(cmd *cobra.Command, opts *globalOptions) (string, *index.Index, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", nil, failure(cmd, opts, exitEnv, "io", err)
	}
	root, err := collection.FindRoot(opts.root, cwd)
	if errors.Is(err, collection.ErrNoCollection) {
		return "", nil, failure(cmd, opts, exitEnv, "no-collection", err)
	}
	if err != nil {
		return "", nil, failure(cmd, opts, exitEnv, "io", err)
	}
	x, err := index.Load(root)
	if err != nil {
		return "", nil, failure(cmd, opts, exitEnv, "io", err)
	}
	for _, name := range x.Skipped {
		fmt.Fprintf(cmd.ErrOrStderr(), "skipped %s: frontmatter does not parse (run validate)\n", name)
	}
	return root, x, nil
}

// finish maps the error of a print to a runtime failure.
func finish(err error) error {
	if err != nil {
		return runtimeFailure{err}
	}
	return nil
}

func lines(w io.Writer, rows []string) error {
	for _, r := range rows {
		if _, err := fmt.Fprintln(w, r); err != nil {
			return err
		}
	}
	return nil
}

func listCommand(opts *globalOptions) *cobra.Command {
	var tag string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the notes",
		Long: `List the notes of the collection in file-name order, one per line:
id, title, path and tags separated by tabs. --tag keeps the notes that carry
that exact tag. With --json each note is {id, title, path, tags}.
Notes whose frontmatter does not parse are skipped with a warning on stderr.
Exit status: 0 done, 2 usage error, 3 environment error.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			x, err := loadIndex(cmd, opts)
			if err != nil {
				return err
			}
			var notes []index.Note
			for _, n := range x.Notes {
				if tag == "" || contains(n.Tags, tag) {
					notes = append(notes, n)
				}
			}
			if opts.json {
				return finish(writeJSON(cmd.OutOrStdout(), listDocument{1, "list", true, jsonNotes(notes)}))
			}
			var rows []string
			for _, n := range notes {
				rows = append(rows, strings.Join([]string{n.ID, n.Title, n.Path, strings.Join(n.Tags, ",")}, "\t"))
			}
			return finish(lines(cmd.OutOrStdout(), rows))
		},
	}
	cmd.Flags().StringVar(&tag, "tag", "", "Only the notes with this tag")
	return cmd
}

func tagsCommand(opts *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "tags",
		Short: "List the tags with their note counts",
		Long: `List every tag of the collection sorted by name, one per line: the tag and the
number of notes that carry it, separated by a tab. With --json: {tags: [{tag, count}]}.
Exit status: 0 done, 2 usage error, 3 environment error.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			x, err := loadIndex(cmd, opts)
			if err != nil {
				return err
			}
			tags := x.Tags()
			if opts.json {
				return finish(writeJSON(cmd.OutOrStdout(), tagsDocument{1, "tags", true, tags}))
			}
			var rows []string
			for _, t := range tags {
				rows = append(rows, fmt.Sprintf("%s\t%d", t.Tag, t.Count))
			}
			return finish(lines(cmd.OutOrStdout(), rows))
		},
	}
}

// findNote resolves the NOTE argument of links and backlinks.
func findNote(cmd *cobra.Command, opts *globalOptions, x *index.Index, arg string) (index.Note, error) {
	n, ok := x.Find(arg)
	if !ok {
		return n, failure(cmd, opts, exitUsage, "usage", fmt.Errorf("no note %q (a title, alias, file name or id)", arg))
	}
	return n, nil
}

func linksCommand(opts *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "links NOTE",
		Short: "List the wikilinks of a note",
		Long: `List the wikilinks of NOTE (a title, alias, file name or id) in body order. A link
resolves like validate resolves it: by title or alias, ignoring case, the |label and the
#heading. Each line is [[target]], a tab and "-> title (path)" or "unresolved"; unresolved
targets are reported, not an error (validate reports them as findings). With --json:
{note, links: [{target, resolved, id, title, path}], unresolved}.
Exit status: 0 done, 2 usage error (including an unknown note), 3 environment error.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			x, err := loadIndex(cmd, opts)
			if err != nil {
				return err
			}
			n, err := findNote(cmd, opts, x, args[0])
			if err != nil {
				return err
			}
			doc := linksDocument{Version: 1, Command: "links", OK: true, Note: toJSONNote(n), Links: []jsonLink{}}
			var rows []string
			for _, l := range x.Links(n) {
				jl := jsonLink{Target: l.Target}
				row := "[[" + l.Target + "]]\tunresolved"
				if l.To != nil {
					jl.Resolved, jl.ID, jl.Title, jl.Path = true, l.To.ID, l.To.Title, l.To.Path
					row = "[[" + l.Target + "]]\t-> " + l.To.Title + " (" + l.To.Path + ")"
				} else {
					doc.Unresolved++
				}
				doc.Links = append(doc.Links, jl)
				rows = append(rows, row)
			}
			if opts.json {
				return finish(writeJSON(cmd.OutOrStdout(), doc))
			}
			return finish(lines(cmd.OutOrStdout(), rows))
		},
	}
}

func backlinksCommand(opts *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "backlinks NOTE",
		Short: "List the notes that link to a note",
		Long: `List the other notes whose wikilinks resolve to NOTE (a title, alias, file name or
id), in file-name order: title and path separated by a tab. With --json:
{note, backlinks: [{id, title, path, tags}]}.
Exit status: 0 done, 2 usage error (including an unknown note), 3 environment error.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			x, err := loadIndex(cmd, opts)
			if err != nil {
				return err
			}
			n, err := findNote(cmd, opts, x, args[0])
			if err != nil {
				return err
			}
			from := x.Backlinks(n)
			if opts.json {
				return finish(writeJSON(cmd.OutOrStdout(), backlinksDocument{1, "backlinks", true, toJSONNote(n), jsonNotes(from)}))
			}
			var rows []string
			for _, m := range from {
				rows = append(rows, m.Title+"\t"+m.Path)
			}
			return finish(lines(cmd.OutOrStdout(), rows))
		},
	}
}
