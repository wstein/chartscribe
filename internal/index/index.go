// SPDX-License-Identifier: EUPL-1.2

// Package index reads a note collection for the query commands (list, tags,
// links, backlinks). It uses the parser and the link-name rule of validate and
// only reads files.
package index

import (
	"regexp"
	"sort"
	"strings"

	"github.com/wstein/chartscribe/internal/collection"
	"github.com/wstein/chartscribe/internal/note"
)

// Note is one note whose frontmatter parsed.
type Note struct {
	ID      string
	Title   string
	Name    string // file name, for example "Idea.md"
	Path    string // path relative to the root, with forward slashes
	Tags    []string
	Aliases []string
	body    string // without the generated tracking section
}

// Index is the notes of one collection in file-name order.
type Index struct {
	Root    string // the collection root
	Notes   []Note
	Skipped []string // file names whose frontmatter did not parse (validate reports them)
	IDs     []string // the ids of the notes, also of the skipped ones where an id line can be read
	byName  map[string]int
}

var idLine = regexp.MustCompile(`(?m)^id:[ \t]*"?([^"\s]+)`)

// Link is one wikilink of a note; To is nil when the target does not resolve.
type Link struct {
	Target string
	To     *Note
}

// TagCount is a tag and the number of notes that carry it.
type TagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

// Load reads notes/*.md of the collection at root.
func Load(root string) (*Index, error) {
	files, err := collection.ReadNotes(root)
	if err != nil {
		return nil, err
	}
	x := &Index{Root: root, byName: map[string]int{}}
	for _, f := range files {
		fm, body, err := note.Parse(f.Text)
		if err != nil {
			x.Skipped = append(x.Skipped, f.Name)
			if m := idLine.FindStringSubmatch(f.Text); m != nil {
				x.IDs = append(x.IDs, m[1])
			}
			continue
		}
		n := Note{Title: f.Title(), Name: f.Name, Path: "notes/" + f.Name, Tags: []string{}, Aliases: []string{}, body: note.StripTracking(body)}
		if v, ok := fm.Get("id"); ok && !v.IsList {
			n.ID = v.Text
		}
		if v, ok := fm.Get("tags"); ok && v.IsList {
			n.Tags = append(n.Tags, v.List...)
		}
		if v, ok := fm.Get("aliases"); ok && v.IsList {
			n.Aliases = append(n.Aliases, v.List...)
		}
		x.Notes = append(x.Notes, n)
		x.IDs = append(x.IDs, n.ID)
	}
	// As validate resolves names, a later note wins a duplicated title or alias.
	for i, n := range x.Notes {
		x.byName[strings.ToLower(n.Title)] = i
		for _, a := range n.Aliases {
			x.byName[strings.ToLower(a)] = i
		}
	}
	return x, nil
}

// ByName returns the note whose title or alias is name, ignoring case.
func (x *Index) ByName(name string) (Note, bool) {
	i, ok := x.byName[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return Note{}, false
	}
	return x.Notes[i], true
}

// Find returns the note an argument names: a title or alias (any case), a file
// name with or without notes/, or an id.
func (x *Index) Find(arg string) (Note, bool) {
	arg = strings.TrimSpace(arg)
	if i, ok := x.byName[strings.ToLower(arg)]; ok {
		return x.Notes[i], true
	}
	rel := strings.TrimPrefix(arg, "notes/")
	for _, n := range x.Notes {
		if n.Name == rel || n.ID == arg {
			return n, true
		}
	}
	return Note{}, false
}

// Links returns the wikilinks of n in body order, resolved like validate does:
// by title or alias, ignoring case, the |label and the #heading.
func (x *Index) Links(n Note) []Link {
	var out []Link
	for _, target := range note.WikiTargets(n.body) {
		l := Link{Target: target}
		if i, ok := x.byName[note.LinkName(target)]; ok {
			l.To = &x.Notes[i]
		}
		out = append(out, l)
	}
	return out
}

// Backlinks returns the other notes that link to n, in file-name order.
func (x *Index) Backlinks(n Note) []Note {
	out := []Note{}
	for _, m := range x.Notes {
		if m.Name == n.Name {
			continue
		}
		for _, l := range x.Links(m) {
			if l.To != nil && l.To.Name == n.Name {
				out = append(out, m)
				break
			}
		}
	}
	return out
}

// Tags returns every tag with its note count, sorted by tag.
func (x *Index) Tags() []TagCount {
	counts := map[string]int{}
	for _, n := range x.Notes {
		seen := map[string]bool{}
		for _, t := range n.Tags {
			if !seen[t] {
				seen[t] = true
				counts[t]++
			}
		}
	}
	out := []TagCount{}
	for t, c := range counts {
		out = append(out, TagCount{t, c})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Tag < out[j].Tag })
	return out
}
