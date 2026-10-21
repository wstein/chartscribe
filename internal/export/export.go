// SPDX-License-Identifier: EUPL-1.2

// Package export builds one Markdown document from a note and the notes it
// links to. It only reads the collection and has no clock: the same notes give
// the same bytes.
package export

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/wstein/chartscribe/internal/index"
)

// Included is a note that is part of the document. Level is its heading level.
type Included struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Path  string `json:"path"`
	Level int    `json:"level"`
}

// Unresolved is a wikilink that names no note.
type Unresolved struct {
	Note   string `json:"note"`
	Target string `json:"target"`
}

// Result is the document and what went into it.
type Result struct {
	Markdown   string
	Notes      []Included
	Unresolved []Unresolved
}

var wikilink = regexp.MustCompile(`\[\[([^\]\n]+)\]\]`)

type section struct {
	note  index.Note
	level int
}

// Build exports start and, down to depth levels, the notes its wikilinks name
// (depth 0 is the start note alone). The start note is the title (title when
// not empty); every other note is a heading section at level 1 + its depth, in
// link order (the order of a map), each note once. Wikilinks in code and in
// the Links section name no member.
func Build(x *index.Index, start index.Note, depth int, title string) Result {
	docs := map[string]*parsed{}
	get := func(n index.Note) *parsed {
		if p, ok := docs[n.Name]; ok {
			return p
		}
		p := parse(n.Body())
		docs[n.Name] = p
		return p
	}

	var order []section
	seen := map[string]bool{start.Name: true}
	var visit func(n index.Note, level int)
	visit = func(n index.Note, level int) {
		order = append(order, section{n, level})
		if level-1 >= depth {
			return
		}
		// Claim every direct member before descending, so that a note linked
		// by the start and by a member stays at the start's level.
		var members []index.Note
		for _, target := range get(n).targets {
			if to, ok := x.Resolve(target); ok && !seen[to.Name] {
				seen[to.Name] = true
				members = append(members, to)
			}
		}
		for _, m := range members {
			visit(m, level+1)
		}
	}
	visit(start, 1)

	// First pass: anchors of the titles and of the body headings, in order.
	slugs := map[string]int{}
	anchor := map[string]string{} // note file name -> anchor
	var sources []string
	seenSource := map[string]bool{}
	for _, s := range order {
		heading := s.note.Title
		if s.note.Name == start.Name && title != "" {
			heading = title
		}
		anchor[s.note.Name] = unique(slugs, slug(heading))
		p := get(s.note)
		for _, l := range p.lines {
			if l.kind == kHeading {
				unique(slugs, slug(l.text))
			}
		}
		for _, t := range p.sources {
			if !seenSource[t] {
				seenSource[t] = true
				sources = append(sources, t)
			}
		}
	}

	// Second pass: rewrite the links and write the document.
	var res Result
	seenUnres := map[Unresolved]bool{}
	var out []string
	for _, s := range order {
		level := min(s.level, 6)
		res.Notes = append(res.Notes, Included{s.note.ID, s.note.Title, s.note.Path, s.level})
		heading := s.note.Title
		if s.note.Name == start.Name && title != "" {
			heading = title
		}
		out = append(out, strings.Repeat("#", level)+" "+heading, "")
		p := get(s.note)
		for _, l := range p.lines {
			switch l.kind {
			case kCode:
				out = append(out, l.text)
			case kHeading:
				lvl := min(level+1+l.level-p.shallowest, 6)
				out = append(out, strings.Repeat("#", lvl)+" "+l.text)
			default:
				out = append(out, mapText(l.text, func(text string) string {
					return wikilink.ReplaceAllStringFunc(text, func(m string) string {
						target := m[2 : len(m)-2]
						name, label, _ := strings.Cut(target, "|")
						name, _, _ = strings.Cut(name, "#")
						name = strings.TrimSpace(name)
						if label = strings.TrimSpace(label); label == "" {
							label = name
						}
						to, ok := x.Resolve(target)
						if !ok {
							u := Unresolved{s.note.Title, target}
							if !seenUnres[u] {
								seenUnres[u] = true
								res.Unresolved = append(res.Unresolved, u)
							}
							return label
						}
						if a, in := anchor[to.Name]; in {
							return "[" + label + "](#" + a + ")"
						}
						return label
					})
				}))
			}
		}
		out = append(out, "")
	}
	if len(sources) > 0 {
		out = append(out, "## Sources", "")
		for _, t := range sources {
			out = append(out, "- "+t)
		}
		out = append(out, "")
	}
	res.Markdown = collapse(out)
	return res
}

// collapse joins lines, squeezes runs of blank lines to one and ends the text
// with exactly one newline.
func collapse(lines []string) string {
	var b strings.Builder
	blank := true
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			if !blank {
				b.WriteByte('\n')
			}
			blank = true
			continue
		}
		blank = false
		b.WriteString(strings.TrimRight(l, " \t"))
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// slug is the GitHub heading anchor: lower case, letters, digits, "_" and "-"
// kept, spaces become "-", the rest is dropped.
func slug(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r == ' ':
			b.WriteByte('-')
		case r == '-' || r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		}
	}
	return b.String()
}

// unique returns slug, or slug-N when an earlier heading took it.
func unique(taken map[string]int, s string) string {
	n, dup := taken[s]
	taken[s] = n + 1
	if !dup {
		return s
	}
	for {
		cand := s + "-" + strconv.Itoa(n)
		if _, used := taken[cand]; !used {
			taken[cand] = 1
			return cand
		}
		n++
	}
}
