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

var (
	wikilink = regexp.MustCompile(`\[\[([^\]\n]+)\]\]`)
	heading  = regexp.MustCompile(`^(#{1,6}) +(.*?)\s*$`)
	source   = regexp.MustCompile(`^Source: (.+)$`)
	fence    = regexp.MustCompile("^\\s*(```|~~~)")
)

// line is one body line; Code is true inside a fenced block.
type line struct {
	text string
	code bool
}

type section struct {
	note  index.Note
	level int
	lines []line
}

// Build exports start and, down to depth levels, the notes its wikilinks name
// (depth 0 is the start note alone). The start note is the title; every other
// note is a heading section at level 1 + its depth, in link order (the order
// of a map), each note once.
func Build(x *index.Index, start index.Note, depth int) Result {
	var order []section
	seen := map[string]bool{start.Name: true}
	var visit func(n index.Note, level int)
	visit = func(n index.Note, level int) {
		order = append(order, section{note: n, level: level})
		if level-1 >= depth {
			return
		}
		// Claim every direct member before descending, so that a note linked
		// by the start and by a member stays at the start's level.
		var members []index.Note
		for _, l := range x.Links(n) {
			if l.To != nil && !seen[l.To.Name] {
				seen[l.To.Name] = true
				members = append(members, *l.To)
			}
		}
		for _, m := range members {
			visit(m, level+1)
		}
	}
	visit(start, 1)

	// First pass: lines, headings and their anchors.
	slugs := map[string]int{}
	anchor := map[string]string{} // note file name -> anchor
	var sources []string
	seenSource := map[string]bool{}
	for i := range order {
		s := &order[i]
		s.level = min(s.level, 6)
		a := unique(slugs, slug(s.note.Title))
		anchor[s.note.Name] = a
		inFence, skipLinks := false, false
		for _, raw := range strings.Split(strings.TrimRight(s.note.Body(), "\n"), "\n") {
			if fence.MatchString(raw) {
				inFence = !inFence
				s.lines = append(s.lines, line{raw, true})
				continue
			}
			if inFence {
				s.lines = append(s.lines, line{raw, true})
				continue
			}
			if m := heading.FindStringSubmatch(raw); m != nil {
				skipLinks = len(m[1]) == 2 && m[2] == "Links"
				if skipLinks {
					continue
				}
				lvl := min(s.level+len(m[1])-1, 6)
				unique(slugs, slug(m[2]))
				s.lines = append(s.lines, line{strings.Repeat("#", lvl) + " " + m[2], false})
				continue
			}
			if m := source.FindStringSubmatch(raw); m != nil {
				if t := strings.TrimSpace(m[1]); !seenSource[t] {
					seenSource[t] = true
					sources = append(sources, t)
				}
				continue
			}
			if skipLinks {
				continue
			}
			s.lines = append(s.lines, line{raw, false})
		}
	}

	// Second pass: rewrite the links and write the document.
	var res Result
	seenUnres := map[Unresolved]bool{}
	var out []string
	for _, s := range order {
		res.Notes = append(res.Notes, Included{s.note.ID, s.note.Title, s.note.Path, s.level})
		out = append(out, strings.Repeat("#", s.level)+" "+s.note.Title, "")
		for _, l := range s.lines {
			text := l.text
			if !l.code {
				text = wikilink.ReplaceAllStringFunc(text, func(m string) string {
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
			}
			out = append(out, text)
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
		cand := s + "-" + itoa(n)
		if _, used := taken[cand]; !used {
			taken[cand] = 1
			return cand
		}
		n++
	}
}

func itoa(n int) string { return strconv.Itoa(n) }
